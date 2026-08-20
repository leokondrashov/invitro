#!/bin/bash

cd "$HOME/kubedirect-ae/kubernetes"

# Same tag used by their build script.
if ! git tag -l | grep -qx 'v1.32.0-kubedirect'; then
    git tag v1.32.0-kubedirect
fi

make WHAT=cmd/kubelet

KD_KUBELET="$PWD/_output/bin/kubelet"

MASTER_NODE=$(kubectl get nodes \
  -l node-role.kubernetes.io/control-plane \
  -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')

kubectl -n kube-system get cm kubeadm-config \
  -o jsonpath='{.data.ClusterConfiguration}' \
  | yq '
      .imageRepository = "shengqipku" |
      .kubernetesVersion = "v1.32.0-kubedirect" |
      .controlPlaneEndpoint = "'$MASTER_NODE':6443"
    ' > /tmp/clusterconfig.yaml

kubectl -n kube-system patch cm kubeadm-config \
  --type merge \
  -p "$(jq -n --rawfile cfg /tmp/clusterconfig.yaml \
    '{data:{ClusterConfiguration:$cfg}}')"

scp /tmp/clusterconfig.yaml $MASTER_NODE:/tmp/clusterconfig.yaml
ssh $MASTER_NODE '
sudo kubeadm init phase control-plane all --config /tmp/clusterconfig.yaml
sudo kubeadm init phase addon kube-proxy --config /tmp/clusterconfig.yaml
'

for node in $(kubectl get nodes -o jsonpath='{.items[*].metadata.name}'); do
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
