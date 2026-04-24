#!/bin/bash

TRACE=$1
RELAY_ARGS=$2

mc ls minio/snapshots

reset_relay() {
	for n in {3..10}
	do
		ssh 10.0.1.$n "tmux send-keys -t relay C-c"
	done
    sleep 5
	for n in {3..10}
    do
    	ssh 10.0.1.$n "tmux send -t relay 'sudo ./relay -ss proxy -snapshots remote -dbg -netPoolSize 1 -endpoint 10.0.1.$n:8080 $RELAY_ARGS 2>&1 > ~/relay.log' ENTER"
    done
    sleep 5
}

get_relay_logs() {
    for n in {3..10}
    do
        scp 10.0.1.$n:~/relay.log data/out/relay_$n.log
    done
}

if ! mc stat minio/snapshots/base 2>&1 >/dev/null; then
    ssh 10.0.1.3 "tmux send-keys -t relay C-c"
    sleep 10
    ssh 10.0.1.3 "tmux send -t relay 'sudo ./relay -ss proxy -snapshots remote -dbg -netPoolSize 1 -endpoint 10.0.1.3:8080 $RELAY_ARGS 2>&1 > ~/relay.log' ENTER"
    sleep 10
fi

# make clean

reset_relay

TRACE=$(echo "$TRACE" | sed 's/\//\\\//g')
sed 's/"TracePath": ".*",/"TracePath": "'$TRACE'",/' cmd/config_warming_up.json > cmd/config_warming_up_tmp.json
# pushd tools/trace_synthesizer; python3 . generate --existing-trace $TRACE -o warming_up -b 1 -t 1 -s 1 -dur 1440 -m 0; popd
# cp $TRACE/mapper_output.json tools/trace_synthesizer/warming_up/

go run cmd/loader.go -config cmd/config_warming_up_tmp.json -verbosity debug -justDeploy

killall server
for n in 1 2 3 4 5 6; do # go over three times to make sure there are all snapshot and working sets
    unset covered
    declare -A covered
    for url in `kn service list | awk 'NR>1 {print $2}'`; do
        echo "Warming up functions at $url"
        # normalize url: strip protocol, everything after the first period, and after the last dash
        host="${url#*://}"
        name="${host%%.*}"
        name="${name%-*}"
        canonicalName="${name%-*}" # get the name without the version suffix
        if [[ -n "${covered[$canonicalName]+x}" ]]; then
            continue
        fi
        covered[$canonicalName]=1

        # if mc stat minio/snapshots/$name/working_set_pages >/dev/null 2>&1; then
        #     echo "Skipping $name as snapshot is already available"
        #     continue
        # fi
        fn_name=$(jq -r '."'$canonicalName'".InvocationParams.FunctionName' workloads/container/yamls/deploy_info.json)
        generator=$(jq -r '."'$canonicalName'".InvocationParams.Generator' workloads/container/yamls/deploy_info.json)
        value=$(jq -r '."'$canonicalName'".InvocationParams.Value' workloads/container/yamls/deploy_info.json)
        lower=$(jq -r '."'$canonicalName'".InvocationParams.LowerBound' workloads/container/yamls/deploy_info.json)
        upper=$(jq -r '."'$canonicalName'".InvocationParams.UpperBound' workloads/container/yamls/deploy_info.json)
        method=$(jq -r '."'$canonicalName'".InvocationParams.FunctionMethod' workloads/container/yamls/deploy_info.json)
        ../vswarm/tools/relay/server --addr=localhost:50051 --function-endpoint-url=passthrough:///$host --function-endpoint-port=80 --function-name=$fn_name --function-method=$method --generator=$generator --value=$value --lowerBound=$lower --upperBound=$upper &
        pid=$!
        sleep 1
        timeout 30 ../vhive/bin/grpcurl -import-path ../vhive/function-images/springboot/proto -proto helloworld.proto -plaintext -d '{"name": "record"}' localhost:50051 helloworld.Greeter/SayHello 2>&1 > /dev/null || echo "Function $name did not respond"
        kill $pid
    done
done

get_relay_logs

sleep 30

reset_relay

# make clean

mc ls minio/snapshots
