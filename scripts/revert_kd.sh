#!/bin/bash

MASTER_NODE=$(kubectl get nodes \
  -l node-role.kubernetes.io/control-plane \
  -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')

kubectl -n kube-system get cm kubeadm-config \
  -o jsonpath='{.data.ClusterConfiguration}' \
  | yq '
      .imageRepository = "registry.k8s.io" |
      .kubernetesVersion = "v1.32.0" |
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
    echo "Restoring kubelet on $node"

    ssh "$node" '
        set -e
        sudo systemctl stop kubelet
        sudo install -o root -g root -m 0755 \
            /tmp/kubelet.stock /usr/bin/kubelet
        sudo systemctl start kubelet
        sudo systemctl is-active --quiet kubelet
    '

    # Wait until Kubernetes sees the node healthy again.
    kubectl wait \
        --for=condition=Ready \
        "node/$node" \
        --timeout=120s
done

