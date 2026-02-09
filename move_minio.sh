#!/usr/bin/env bash

cd ~/vhive/configs/storage/minio

kubectl delete -f service.yaml
kubectl delete -f deployment.yaml
kubectl delete -f pv-claim.yaml
kubectl delete -f pv.yaml

MINIO_NODE_NAME=$(hostname) MINIO_PATH=$1 envsubst < pv.yaml | kubectl apply -f -
kubectl apply -f pv-claim.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml