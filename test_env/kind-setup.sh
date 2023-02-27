#!/bin/bash

# kind delete cluster kind
# kind create cluster --config kind.yml

LATEST_TAG=$(git tag | tail -n 1)
LATEST_TAG=${LATEST_TAG:1}
REPOSITORY=docker.yektanet.tech/tools/redis-backup-operator
NAMESPACE=redis-backup-operator

# helm install redis-backup-operator charts/redis-backup-operator --set image.repository=$REPOSITORY \
# --set image.tag=$LATEST_TAG --set imagePullSecrets[0].name=docker.yektanet.tech --create-namespace --namespace  --debug

kubectl apply -f minio.yml -n $NAMESPACE
kubectl apply -f redis.yml -n $NAMESPACE
kubectl apply -f redisbackup.yml -n $NAMESPACE