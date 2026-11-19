#!/usr/bin/env bash
# Removes the example from the hub and deletes slurm-cluster.
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context ${HUB_CTX:-kind-local-cluster}"
CLUSTER="${SLURM_CLUSTER_NAME:-slurm-cluster}"

$H delete jobs -n hpc --all --wait=true
$H delete -f "$DIR/hub/worker-queues.yaml" --ignore-not-found
# Delete hub/queues.yaml piece by piece: it also holds the shared default-flavor
# ResourceFlavor that other examples use.
for kind in localqueue/kube-queue localqueue/slurm-queue; do $H delete -n hpc "$kind" --ignore-not-found; done
$H delete clusterqueue cq-kube-batch cq-slurm --ignore-not-found
$H delete admissioncheck mk-kube-batch mk-slurm ocm-kube-batch ocm-slurm --ignore-not-found
$H delete namespace hpc --ignore-not-found
$H delete -f "$DIR/hub/placements.yaml" --ignore-not-found
$H label managedcluster "${KUBE_WORKER:-cluster1}" scheduler- 2>/dev/null || true
$H delete managedcluster "$CLUSTER" --ignore-not-found --wait=true
# The hub keeps the cluster's namespace after the ManagedCluster is deleted. Remove it so a
# later create-slurm-cluster.sh starts from a clean registration.
$H delete namespace "$CLUSTER" --ignore-not-found --wait=true --timeout=180s
kind delete cluster --name "$CLUSTER"
