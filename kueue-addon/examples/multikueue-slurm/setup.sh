#!/usr/bin/env bash
# Hub-side setup: label the workers by pod scheduler, create Placements and queues, and
# push the worker LocalQueues. Run create-slurm-cluster.sh first.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context ${HUB_CTX:-kind-local-cluster}"
KUBE_WORKER="${KUBE_WORKER:-cluster1}"
SLURM_WORKER="${SLURM_CLUSTER_NAME:-slurm-cluster}"

if ! $H get deploy -n open-cluster-management-hub cluster-manager-work-controller >/dev/null 2>&1; then
  echo "Enabling the ManifestWorkReplicaSet feature on the hub"
  $H patch clustermanager cluster-manager --type=merge \
    -p '{"spec":{"workConfiguration":{"workDriver":"kube","featureGates":[{"feature":"ManifestWorkReplicaSet","mode":"Enable"}]}}}'
  until $H get deploy -n open-cluster-management-hub cluster-manager-work-controller >/dev/null 2>&1; do sleep 2; done
  $H rollout status -n open-cluster-management-hub deploy/cluster-manager-work-controller --timeout=180s
fi

echo "Labelling workers by pod scheduler"
$H label managedcluster "$KUBE_WORKER" scheduler=kube-scheduler --overwrite
$H label managedcluster "$SLURM_WORKER" scheduler=slurm --overwrite

echo "Creating Placements, AdmissionChecks, ClusterQueues and LocalQueues on the hub"
$H apply -f "$DIR/hub/placements.yaml" -f "$DIR/hub/queues.yaml"

echo "Distributing the hpc LocalQueues to the workers with a ManifestWorkReplicaSet"
$H apply -f "$DIR/hub/worker-queues.yaml"
until [ "$($H get manifestworkreplicaset hpc-worker-queues -n kueue-system \
  -o jsonpath='{.status.summary.available}' 2>/dev/null)" = "2" ]; do sleep 2; done

echo "Waiting for the kueue-addon to generate the MultiKueueConfigs"
for p in kube-batch slurm; do
  until $H get multikueueconfig "$p" >/dev/null 2>&1; do sleep 2; done
done
$H get multikueueconfig kube-batch slurm -o 'custom-columns=MKCONFIG:.metadata.name,CLUSTERS:.spec.clusters'
