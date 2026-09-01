#!/usr/bin/env bash
#
# Update the kubeadm-managed configuration with the control-plane settings in
# config/kubeadm_init.yaml. The existing ConfigMap contents are the source of
# truth, so unrelated kubeadm settings are preserved.

set -euo pipefail

readonly KUBEADM_INIT_CONFIG="config/kubeadm_init.yaml"
readonly KUBE_SYSTEM_NAMESPACE="kube-system"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

config_value() {
	local expression=$1
	yq eval -r "$expression" "$KUBEADM_INIT_CONFIG"
}

set_extra_arg() {
	local config_file=$1
	local component_path=$2
	local argument_name=$3
	local argument_value=$4

	ARGUMENT_NAME="$argument_name" ARGUMENT_VALUE="$argument_value" \
		yq eval -i "${component_path}.extraArgs |= ((. // []) | map(select(.name != strenv(ARGUMENT_NAME))) + [{\"name\": strenv(ARGUMENT_NAME), \"value\": strenv(ARGUMENT_VALUE)}])" "$config_file"
}

patch_config_map_data() {
	local config_map=$1
	local data_key=$2
	local config_file=$3
	local patch_file="$work_dir/${config_map}-patch.json"

	kubectl create configmap "$config_map" --namespace "$KUBE_SYSTEM_NAMESPACE" \
		--from-file="${data_key}=${config_file}" --dry-run=client --output=json |
		jq '{data: .data}' > "$patch_file"
	kubectl patch configmap "$config_map" --namespace "$KUBE_SYSTEM_NAMESPACE" \
		--type=merge --patch-file "$patch_file"
}

cluster_config="$work_dir/ClusterConfiguration.yaml"
kubectl get configmap kubeadm-config --namespace "$KUBE_SYSTEM_NAMESPACE" \
	--output='go-template={{index .data "ClusterConfiguration"}}' > "$cluster_config"

# Preserve every existing kubeadm option and only replace the managed
# control-plane arguments from kubeadm_init.yaml.
set_extra_arg "$cluster_config" '.apiServer' max-mutating-requests-inflight \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .apiServer.extraArgs[] | select(.name == "max-mutating-requests-inflight") | .value')"
set_extra_arg "$cluster_config" '.apiServer' max-requests-inflight \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .apiServer.extraArgs[] | select(.name == "max-requests-inflight") | .value')"
set_extra_arg "$cluster_config" '.apiServer' feature-gates \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .apiServer.extraArgs[] | select(.name == "feature-gates") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' bind-address \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "bind-address") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' kube-api-qps \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "kube-api-qps") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' kube-api-burst \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "kube-api-burst") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' concurrent-deployment-syncs \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "concurrent-deployment-syncs") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' concurrent-endpoint-syncs \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "concurrent-endpoint-syncs") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' concurrent-replicaset-syncs \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "concurrent-replicaset-syncs") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' concurrent-service-endpoint-syncs \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "concurrent-service-endpoint-syncs") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' endpoint-updates-batch-period \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "endpoint-updates-batch-period") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' endpointslice-updates-batch-period \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "endpointslice-updates-batch-period") | .value')"
set_extra_arg "$cluster_config" '.controllerManager' feature-gates \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .controllerManager.extraArgs[] | select(.name == "feature-gates") | .value')"
set_extra_arg "$cluster_config" '.scheduler' bind-address \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .scheduler.extraArgs[] | select(.name == "bind-address") | .value')"
set_extra_arg "$cluster_config" '.etcd.local' listen-metrics-urls \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .etcd.local.extraArgs[] | select(.name == "listen-metrics-urls") | .value')"
set_extra_arg "$cluster_config" '.etcd.local' quota-backend-bytes \
	"$(config_value 'select(.kind == "ClusterConfiguration") | .etcd.local.extraArgs[] | select(.name == "quota-backend-bytes") | .value')"
patch_config_map_data kubeadm-config ClusterConfiguration "$cluster_config"

proxy_config="$work_dir/kube-proxy-config.yaml"
kubectl get configmap kube-proxy --namespace "$KUBE_SYSTEM_NAMESPACE" \
	--output='go-template={{index .data "config.conf"}}' > "$proxy_config"
METRICS_BIND_ADDRESS="$(config_value 'select(.kind == "KubeProxyConfiguration") | .metricsBindAddress')" \
	yq eval -i '.metricsBindAddress = strenv(METRICS_BIND_ADDRESS)' "$proxy_config"
patch_config_map_data kube-proxy config.conf "$proxy_config"
