#!/usr/bin/env bash
# Prepares the kind environment from build/setup-env.sh for the LWS multi-provider example.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
HUB_CTX="${HUB_CTX:-kind-local-cluster}"
declare -A PROVIDER=([cluster1]=ibm [cluster2]=coreweave [cluster3]=onprem)

echo "Labelling managed clusters with their provider"
for c in "${!PROVIDER[@]}"; do
  kubectl --context "$HUB_CTX" label managedcluster "$c" "provider=${PROVIDER[$c]}" --overwrite
done

echo "Creating Placements, AdmissionChecks, ClusterQueues and LocalQueues on the hub"
kubectl --context "$HUB_CTX" apply -f "$DIR/hub/placements.yaml" -f "$DIR/hub/queues.yaml"

echo "Creating matching LocalQueues on the workers"
for c in "${!PROVIDER[@]}"; do
  kubectl --context "kind-$c" apply -f "$DIR/workers/queues.yaml"
done

echo "Waiting for the kueue-addon to generate the MultiKueueConfigs"
for p in ibm coreweave onprem gpu-cloud; do
  until kubectl --context "$HUB_CTX" get multikueueconfig "$p" >/dev/null 2>&1; do sleep 2; done
done
kubectl --context "$HUB_CTX" get multikueueconfig ibm coreweave onprem gpu-cloud \
  -o 'custom-columns=MKCONFIG:.metadata.name,CLUSTERS:.spec.clusters'
