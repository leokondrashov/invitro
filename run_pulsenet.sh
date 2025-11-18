#!/bin/bash

run_experiment() {
    EXP=$1
    MASTER_NODE=10.0.1.1
    func=$2
    config=config_$3.json

    export EXPERIMENT="$EXP"_$func
    export FUNC=$func
    start_time=`date --rfc-3339=seconds | sed 's/ /T/'`

    mkdir data/out/$EXPERIMENT
    cat cmd/$config | envsubst '$EXPERIMENT','$FUNC' > cmd/config_tmp.json
    go run cmd/loader.go -config cmd/config_tmp.json -verbosity debug | tee data/out/$EXPERIMENT/loader.log

    ./download_logs.sh $MASTER_NODE knative-serving autoscaler data/out/$EXPERIMENT $start_time
    ./download_logs.sh $MASTER_NODE knative-serving activator data/out/$EXPERIMENT $start_time

    i=10
    while [ "$(ssh $MASTER_NODE 'curl -XPOST http://localhost:9090/api/v1/admin/tsdb/snapshot | jq ".status"')" != '"success"' ] && [ $i -gt 0 ]; do
	    echo retry
	    let i=$i-1
	    sleep 10
    done
    mkdir data/out/$EXPERIMENT/prometheus_snapshot
    kubectl cp -n monitoring prometheus-prometheus-kube-prometheus-prometheus-0:/prometheus/snapshots/ -c prometheus data/out/$EXPERIMENT/prometheus_snapshot

    make clean
}

reset_relay() {
	for n in {3..8}
	do
		ssh 10.0.1.$n "tmux send-keys -t relay C-c"
		ssh 10.0.1.$n "tmux send -t relay 'sudo ./relay -ss proxy -snapshots remote -dbg -netPoolSize 1 -endpoint 10.0.1.$n:8080 2>&1 > ~/relay.log' ENTER"
	done
}

# baseline
kubectl patch deployment autoscaler -n knative-serving -p '{"spec": {"template": {"spec": {"containers": [{"name": "autoscaler", "image": "lkondras/autoscaler-12c0fa24db31956a7cfa673210e4fa13:synchronous"}]}}}}'
kubectl patch deployment activator -n knative-serving -p '{"spec": {"template": {"spec": {"containers": [{"name": "activator", "image": "lkondras/activator-ecd51ca5034883acbe737fde417a3d86:pulsenet-400"}]}}}}'
kubectl patch configmap config-autoscaler -n knative-serving -p '{"data": {"container-concurrency-target-percentage": "100"}}'
kubectl patch configmap config-autoscaler -n knative-serving -p '{"data": {"stable-window": "60s"}}'

reset_relay

sleep 60

cp workloads/container/trace_func_go_1.yaml workloads/container/trace_func_go.yaml

sed -i -e 's/"CPULimit": .*/"CPULimit": "1vCPU",/' cmd/config_knative.json

run_experiment "pulsenet" "400" "knative"

#sed -i -e 's/"CPULimit": .*/"CPULimit": "1vCPU",/' cmd/config_knative.json

#run_experiment "1cc_full" "400" "knative"

#sed -i -e 's/"CPULimit": .*/"CPULimit": "0_1vCPU",/' cmd/config_knative.json

#run_experiment "1cc_fraction" "400" "knative"

cp workloads/container/trace_func_go_2.yaml workloads/container/trace_func_go.yaml

#sed -i -e 's/"CPULimit": .*/"CPULimit": "1vCPU",/' cmd/config_knative.json

#run_experiment "2cc" "400" "knative"

sed -i -e 's/"CPULimit": .*/"CPULimit": "2vCPU",/' cmd/config_knative.json

run_experiment "pulsenet_2cc_full" "400" "knative"

#sed -i -e 's/"CPULimit": .*/"CPULimit": "0_2vCPU",/' cmd/config_knative.json

#run_experiment "2cc_fraction" "400" "knative"

cp workloads/container/trace_func_go_4.yaml workloads/container/trace_func_go.yaml

#sed -i -e 's/"CPULimit": .*/"CPULimit": "1vCPU",/' cmd/config_knative.json

#run_experiment "4cc" "400" "knative"

sed -i -e 's/"CPULimit": .*/"CPULimit": "4vCPU",/' cmd/config_knative.json

run_experiment "pulsenet_4cc_full" "400" "knative"

#sed -i -e 's/"CPULimit": .*/"CPULimit": "0_4vCPU",/' cmd/config_knative.json

#run_experiment "4cc_fraction" "400" "knative"

cp workloads/container/trace_func_go_5.yaml workloads/container/trace_func_go.yaml

#sed -i -e 's/"CPULimit": .*/"CPULimit": "1vCPU",/' cmd/config_knative.json

#run_experiment "5cc" "400" "knative"

sed -i -e 's/"CPULimit": .*/"CPULimit": "5vCPU",/' cmd/config_knative.json

run_experiment "pulsenet_5cc_full" "400" "knative"

#sed -i -e 's/"CPULimit": .*/"CPULimit": "0_5vCPU",/' cmd/config_knative.json

#run_experiment "5cc_fraction" "400" "knative"

cp workloads/container/trace_func_go_10.yaml workloads/container/trace_func_go.yaml

#sed -i -e 's/"CPULimit": .*/"CPULimit": "1vCPU",/' cmd/config_knative.json

#run_experiment "10cc" "400" "knative"

sed -i -e 's/"CPULimit": .*/"CPULimit": "10vCPU",/' cmd/config_knative.json

run_experiment "pulsenet_10cc_full" "400" "knative"

#sed -i -e 's/"CPULimit": .*/"CPULimit": "0_10vCPU",/' cmd/config_knative.json

#run_experiment "10cc_fraction" "400" "knative"

