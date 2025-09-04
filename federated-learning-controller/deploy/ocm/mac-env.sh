#!/bin/bash

cd $(dirname ${BASH_SOURCE})

set -e

hub=${HUB:-hub}
c1=${CLUSTER1:-cluster1}
c2=${CLUSTER2:-cluster2}

hubctx="kind-${hub}"
c1ctx="kind-${c1}"
c2ctx="kind-${c2}"

cat <<EOF | kind create cluster --name "${hub}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30090
    hostPort: 30090
    protocol: TCP
EOF

kind create cluster --name "${c1}"
kind create cluster --name "${c2}"

echo "Initialize the ocm hub cluster"
clusteradm init --wait --context ${hubctx}
joincmd=$(clusteradm get token --context ${hubctx} | grep clusteradm)

echo "Join clusters to hub"
eval "${joincmd//<cluster_name>/local-cluster} --force-internal-endpoint-lookup --wait --context ${hubctx}"
eval "${joincmd//<cluster_name>/${c1}} --force-internal-endpoint-lookup --wait --context ${c1ctx}"
eval "${joincmd//<cluster_name>/${c2}} --force-internal-endpoint-lookup --wait --context ${c2ctx}"

echo "Accept join of clusters"
clusteradm accept --context ${hubctx} --clusters ${c1},${c2},local-cluster --wait

# label local-cluster
kubectl label managedclusters local-cluster local-cluster=true --context ${hubctx}
kubectl get managedclusters --all-namespaces --context ${hubctx}