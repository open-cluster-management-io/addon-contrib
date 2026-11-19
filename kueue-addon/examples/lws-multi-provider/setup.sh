#!/usr/bin/env bash
# Prepares the kind environment from build/setup-env.sh --clusterprofile for the LWS multi-provider example.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context ${HUB_CTX:-kind-local-cluster}"
declare -A PROVIDER=([cluster1]=ibm [cluster2]=coreweave [cluster3]=onprem)

# The examples run on the ClusterProfile environment from build/setup-env.sh --clusterprofile.
cp_mode=$($H get deploy kueue-addon-controller -n open-cluster-management-addon \
  -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="ENABLE_CLUSTERPROFILE")].value}' 2>/dev/null || true)
if [ "$cp_mode" != "true" ]; then
  echo "The kueue-addon is not running in ClusterProfile mode." >&2
  echo "Create the environment with: build/setup-env.sh --clusterprofile" >&2
  exit 1
fi

echo "Waiting for the ClusterProfile-backed MultiKueueClusters"
for c in cluster1 cluster2 cluster3; do
  until [ "$($H get multikueuecluster "$c" \
    -o jsonpath='{.spec.clusterSource.clusterProfileRef.name}/{.status.conditions[?(@.type=="Active")].status}' \
    2>/dev/null)" = "$c/True" ]; do sleep 2; done
done
$H get clusterprofile -n kueue-system cluster1 cluster2 cluster3

if ! $H get deploy -n open-cluster-management-hub cluster-manager-work-controller >/dev/null 2>&1; then
  echo "Enabling the ManifestWorkReplicaSet feature on the hub"
  $H patch clustermanager cluster-manager --type=merge \
    -p '{"spec":{"workConfiguration":{"workDriver":"kube","featureGates":[{"feature":"ManifestWorkReplicaSet","mode":"Enable"}]}}}'
  until $H get deploy -n open-cluster-management-hub cluster-manager-work-controller >/dev/null 2>&1; do sleep 2; done
  $H rollout status -n open-cluster-management-hub deploy/cluster-manager-work-controller --timeout=180s
fi

echo "Labelling managed clusters with their provider"
for c in "${!PROVIDER[@]}"; do
  $H label managedcluster "$c" "provider=${PROVIDER[$c]}" --overwrite
done

echo "Creating Placements, AdmissionChecks, ClusterQueues and LocalQueues on the hub"
$H apply -f "$DIR/hub/placements.yaml" -f "$DIR/hub/queues.yaml"

echo "Distributing LocalQueues to the workers with a ManifestWorkReplicaSet"
$H apply -f "$DIR/hub/worker-queues.yaml"
mwrs_summary() { $H get manifestworkreplicaset lws-worker-queues -n kueue-system \
  -o jsonpath='{.status.summary.available}/{.status.summary.total}' 2>/dev/null; }
mwrs_ready() { local s; s=$(mwrs_summary); [[ "$s" =~ ^[0-9]+/[0-9]+$ && "${s%/*}" -gt 0 && "${s%/*}" == "${s#*/}" ]]; }
until mwrs_ready; do sleep 2; done
echo "LocalQueues available on $(mwrs_summary) worker clusters"

echo "Waiting for the kueue-addon to generate the MultiKueueConfigs"
for p in ibm coreweave onprem gpu-cloud; do
  until $H get multikueueconfig "$p" >/dev/null 2>&1; do sleep 2; done
done
$H get multikueueconfig ibm coreweave onprem gpu-cloud \
  -o 'custom-columns=MKCONFIG:.metadata.name,CLUSTERS:.spec.clusters'
