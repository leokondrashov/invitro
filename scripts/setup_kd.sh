#!/bin/bash

cd "$HOME/kubedirect-ae/kubernetes"

# Same tag used by their build script.
if ! git tag -l | grep -qx 'v1.32.0-kubedirect'; then
    git tag v1.32.0-kubedirect
fi

make WHAT=cmd/kubelet

KD_KUBELET="$PWD/_output/bin/kubelet"

cd "$HOME/kubedirect-ae/cmd/kubelet"

go build

for node in $(kubectl get nodes -o jsonpath='{.items[*].metadata.name}'); do
    if [[ $node =~ fake ]] then
      kubectl annotate node $node kubedirect/kubelet-service-override=true
      kubectl annotate node $node kubedirect/kubelet-service-addr=10.0.1.3:25010 --overwrite
      continue
    fi
    echo "Updating custom kubelet on $node"

    kubectl annotate $node kubedirect/kubelet-service-addr-

    scp kubelet $node:~/kubelet.custom
    ssh $node '
      tmux new -s kubelet -d
      tmux send-keys -t kubelet C-c
      sleep 10
      tmux send-keys -t kubelet "sudo ~/kubelet.custom -snapshotter proxy -dbg -ready-after 0 2>&1 | tee ~/kubelet.log" ENTER
    '
done

MASTER_NODE=$(kubectl get nodes \
  -l node-role.kubernetes.io/control-plane \
  -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')

kubectl -n kube-system get cm kubeadm-config \
  -o jsonpath='{.data.ClusterConfiguration}' \
  | yq '
      .imageRepository = "shengqipku" |
      .dns.imageRepository = "registry.k8s.io" |
      .etcd.local.imageRepository = "registry.k8s.io" |
      .apiServer.extraArgs = (
        (.apiServer.extraArgs // []) |
        map(select(.name != "feature-gates")) +
        [{"name": "feature-gates", "value": "AuthorizeNodeWithSelectors=false"}]
      ) |
      .kubernetesVersion = "v1.32.0-kubedirect" |
      .controlPlaneEndpoint = "'$MASTER_NODE':6443"
    ' > /tmp/clusterconfig.yaml

kubectl -n kube-system patch cm kubeadm-config \
  --type merge \
  -p "$(jq -n --rawfile cfg /tmp/clusterconfig.yaml \
    '{data:{ClusterConfiguration:$cfg}}')"

ssh "$MASTER_NODE" '
sudo kubeadm upgrade apply v1.32.0-kubedirect \
    --allow-experimental-upgrades \
    --force \
    --yes
'

for node in $(kubectl get nodes -o jsonpath='{.items[*].metadata.name}'); do
    [[ $node =~ fake ]] && continue
    
    echo "Updating kubelet on $node"

    scp "$KD_KUBELET" "$node:/tmp/kubelet.kubedirect"

    ssh "$node" '
        set -e
        sudo systemctl stop kubelet
        sudo cp /usr/bin/kubelet /tmp/kubelet.stock
        sudo install -o root -g root -m 0755 \
            /tmp/kubelet.kubedirect /usr/bin/kubelet
        sudo systemctl start kubelet
        sudo systemctl is-active --quiet kubelet
    '

    # Wait until Kubernetes sees the node healthy again.
    kubectl wait \
        --for=condition=Ready \
        "node/$node" \
        --timeout=120s
done

sleep 60
