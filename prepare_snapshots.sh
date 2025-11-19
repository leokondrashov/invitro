#!/bin/bash

TRACE=$1
RELAY_ARGS=$2

mc ls minio/snapshots

reset_relay() {
	for n in {3..8}
	do
		ssh 10.0.1.$n "tmux send-keys -t relay C-c"
	done
    sleep 2
	for n in {3..8}
    do
    	ssh 10.0.1.$n "tmux send -t relay 'sudo ./relay -ss proxy -snapshots remote -dbg -netPoolSize 1 -endpoint 10.0.1.$n:8080 $RELAY_ARGS 2>&1 > ~/relay.log' ENTER"
    done
}

get_relay_logs() {
    for n in {3..8}
    do
        scp 10.0.1.$n:~/relay.log data/out/relay_$n.log
    done
}

reset_relay

pushd tools/trace_synthesizer; python3 . generate --existing-trace $TRACE -o warming_up -b 1 -t 1 -s 1 -dur 1440 -m 0; popd
cp $TRACE/mapper_output.json tools/trace_synthesizer/warming_up/

go run cmd/loader.go -config cmd/config_warming_up.json -verbosity debug

get_relay_logs

reset_relay

mc ls minio/snapshots