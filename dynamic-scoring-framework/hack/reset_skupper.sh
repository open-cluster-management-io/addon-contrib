#!/bin/bash

NAMESPACE=${NAMESPACE:-dynamic-scoring}
HUB_NODE_IP=${HUB_NODE_IP:-10.89.1.16}
HUB_CONTEXT=kind-hub01
CLUSTER1_NAME=worker01
CLUSTER1_CONTEXT=kind-worker01
CLUSTER2_NAME=worker02
CLUSTER2_CONTEXT=kind-worker02


# delete skupper sites and secrets
kubectl delete -f ./deploy/skupper/skupper-site-hub.yaml -n $NAMESPACE --context $HUB_CONTEXT
kubectl delete -f ./deploy/skupper/token-request.yaml -n $NAMESPACE --context $HUB_CONTEXT


CLUSTER_NAME=$CLUSTER1_NAME envsubst < deploy/skupper/skupper-site-ingress-none.yaml | kubectl delete -f - -n $NAMESPACE --context $CLUSTER1_CONTEXT
kubectl delete -f ./secrets/token-$NAMESPACE.yaml -n $NAMESPACE --context $CLUSTER1_CONTEXT

CLUSTER_NAME=$CLUSTER2_NAME envsubst < deploy/skupper/skupper-site-ingress-none.yaml | kubectl delete -f - -n $NAMESPACE --context $CLUSTER2_CONTEXT
kubectl delete -f ./secrets/token-$NAMESPACE.yaml -n $NAMESPACE --context $CLUSTER2_CONTEXT

echo "Waiting for skupper sites to be deleted..."
sleep 10
echo "Waiting for skupper sites to be deleted... done. Proceeding to create new skupper sites."

# create skupper sites and connect them

kubectl create namespace $NAMESPACE --context $HUB_CONTEXT
envsubst < deploy/skupper/skupper-site-hub.yaml | kubectl apply -f - -n $NAMESPACE --context $HUB_CONTEXT

kubectl create namespace $NAMESPACE --context $CLUSTER1_CONTEXT
CLUSTER_NAME=$CLUSTER1_NAME envsubst < deploy/skupper/skupper-site-ingress-none.yaml | kubectl apply -f - -n $NAMESPACE --context $CLUSTER1_CONTEXT

kubectl create namespace $NAMESPACE --context $CLUSTER2_CONTEXT
CLUSTER_NAME=$CLUSTER2_NAME envsubst < deploy/skupper/skupper-site-ingress-none.yaml | kubectl apply -f - -n $NAMESPACE --context $CLUSTER2_CONTEXT

echo "Waiting for skupper sites to be created..."
sleep 10
echo "Waiting for skupper sites to be created... done. Proceeding to connect skupper sites."

kubectl apply -f ./deploy/skupper/token-request.yaml -n $NAMESPACE --context $HUB_CONTEXT
sleep 5
kubectl get secret -o yaml skupper-connection-secret -n $NAMESPACE --context $HUB_CONTEXT > ./secrets/token-$NAMESPACE.yaml

kubectl apply -f ./secrets/token-$NAMESPACE.yaml -n $NAMESPACE --context $CLUSTER1_CONTEXT

kubectl apply -f ./secrets/token-$NAMESPACE.yaml -n $NAMESPACE --context $CLUSTER2_CONTEXT

skupper status -n $NAMESPACE --context $HUB_CONTEXT