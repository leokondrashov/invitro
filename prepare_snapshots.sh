#!/bin/bash

TRACE=$1
RELAY_ARGS=$2

mc ls minio/snapshots

reset_relay() {
	for n in {3..8}
	do
		ssh 10.0.1.$n "tmux send-keys -t relay C-c"
	done
    sleep 5
	for n in {3..8}
    do
    	ssh 10.0.1.$n "tmux send -t relay 'sudo ./relay -ss proxy -snapshots remote -dbg -netPoolSize 1 -endpoint 10.0.1.$n:8080 $RELAY_ARGS 2>&1 > ~/relay.log' ENTER"
    done
    sleep 5
}

get_relay_logs() {
    for n in {3..8}
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

make clean

reset_relay

pushd tools/trace_synthesizer; python3 . generate --existing-trace $TRACE -o warming_up -b 1 -t 1 -s 1 -dur 1440 -m 0; popd
cp $TRACE/mapper_output.json tools/trace_synthesizer/warming_up/

go run cmd/loader.go -config cmd/config_warming_up.json -verbosity debug -justDeploy

for n in 1 2 3; do # go over three times to make sure there are all snapshot and working sets
    for url in `kn service list | awk 'NR>1 {print $2}'`; do
        echo "Warming up functions at $url"
        # normalize url: strip protocol, everything after the first period, and after the last dash
        name="${url#*://}"
        name="${name%%.*}"
        name="${name%-*}"

        if mc stat minio/snapshots/$name/working_set_pages >/dev/null 2>&1; then
            echo "Skipping $name as snapshot is already available"
            continue
        fi
        timeout 30 ../vhive/bin/grpcurl -import-path ../vhive/function-images/springboot/proto -proto helloworld.proto -plaintext -d '{"name": "record"}' ${url#*://}:80 helloworld.Greeter/SayHello 2>&1 > /dev/null || echo "Function $name did not respond"
    done
done

get_relay_logs

sleep 30

reset_relay

make clean

mc ls minio/snapshots
