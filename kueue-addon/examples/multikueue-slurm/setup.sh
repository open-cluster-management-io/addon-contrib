#!/usr/bin/env bash
# Hub-side setup: label the workers by pod scheduler, create Placements and queues, and
# push the worker LocalQueues. Run create-slurm-cluster.sh first.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context ${HUB_CTX:-kind-local-cluster}"
KUBE_WORKER="${KUBE_WORKER:-cluster1}"
SLURM_WORKER="${SLURM_CLUSTER_NAME:-slurm-cluster}"

# The examples run on the ClusterProfile environment from build/setup-env.sh --clusterprofile.
cp_mode=$($H get deploy kueue-addon-controller -n open-cluster-management-addon \
  -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="ENABLE_CLUSTERPROFILE")].value}' 2>/dev/null || true)
if [ "$cp_mode" != "true" ]; then
  echo "The kueue-addon is not running in ClusterProfile mode." >&2
  echo "Create the environment with: build/setup-env.sh --clusterprofile" >&2
  exit 1
fi

echo "Waiting for the ClusterProfile-backed MultiKueueClusters"
for c in "$KUBE_WORKER" "$SLURM_WORKER"; do
  until [ "$($H get multikueuecluster "$c" \
    -o jsonpath='{.spec.clusterSource.clusterProfileRef.name}/{.status.conditions[?(@.type=="Active")].status}' \
    2>/dev/null)" = "$c/True" ]; do sleep 2; done
done
$H get clusterprofile -n kueue-system "$KUBE_WORKER" "$SLURM_WORKER"

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
# Readiness comes from the Placement's actual decisions, not a hardcoded count: the
# hpc-workers Placement selects every cluster with a "scheduler" label, so a worker left
# labelled from an earlier run (e.g. rerunning with KUBE_WORKER=cluster2) makes it select
# more than two. summary.available is the count of available ManifestWorks: wait until
# every selected worker's ManifestWork is available.
selected_workers() {
  $H get placementdecisions -n kueue-system -l cluster.open-cluster-management.io/placement=hpc-workers \
    -o jsonpath='{range .items[*].status.decisions[*]}{.clusterName}{"\n"}{end}' 2>/dev/null | sort -u
}
mwrs_summary() { $H get manifestworkreplicaset hpc-worker-queues -n kueue-system \
  -o jsonpath='{.status.summary.available}/{.status.summary.total}' 2>/dev/null; }
mwrs_all_available() {
  local n
  n=$(selected_workers | wc -l)
  [ "$n" -gt 0 ] && [ "$(mwrs_summary)" = "$n/$n" ]
}
deadline=$(( $(date +%s) + 300 ))
until mwrs_all_available; do
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "ERROR: timed out waiting for the hpc-worker-queues ManifestWorks." >&2
    echo "Selected workers: $(selected_workers | tr '\n' ' ')" >&2
    echo "ManifestWorkReplicaSet summary (available/total): $(mwrs_summary)" >&2
    $H get manifestworkreplicaset hpc-worker-queues -n kueue-system -o yaml >&2 || true
    exit 1
  fi
  sleep 2
done

echo "Waiting for the kueue-addon to generate the MultiKueueConfigs"
for p in kube-batch slurm; do
  until $H get multikueueconfig "$p" >/dev/null 2>&1; do sleep 2; done
done
$H get multikueueconfig kube-batch slurm -o 'custom-columns=MKCONFIG:.metadata.name,CLUSTERS:.spec.clusters'
