#!/usr/bin/env bash
# Prepares the kind environment from build/setup-env.sh for the LWS topology-aware example.
#   1. Installs KWOK on each worker and adds fake GPU nodes labelled with that provider's topology.
#   2. Labels the managed clusters with their provider.
#   3. Creates the hub Placements, Topologies and queues, and the ManifestWorkReplicaSets that
#      push each provider's Topology and queues to its clusters.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context ${HUB_CTX:-kind-local-cluster}"
KWOK_VERSION=${KWOK_VERSION:-v0.8.0}
declare -A PROVIDER=([cluster1]=ibm [cluster2]=coreweave [cluster3]=onprem)

# fake_node <name> <label>=<value>...
# 2 CPU / 8Gi per node, so a group of four 1-CPU pods needs exactly two nodes.
fake_node() {
  local name=$1; shift
  cat <<EOF
---
apiVersion: v1
kind: Node
metadata:
  name: $name
  annotations:
    kwok.x-k8s.io/node: fake
    node.alpha.kubernetes.io/ttl: "0"
  labels:
    type: kwok
    kubernetes.io/hostname: $name
$(for l in "$@"; do echo "    ${l%%=*}: \"${l#*=}\""; done)
spec:
  taints:
  - {key: kwok.x-k8s.io/node, value: fake, effect: NoSchedule}
status:
  allocatable: {cpu: "2", memory: 8Gi, pods: "110"}
  capacity: {cpu: "2", memory: 8Gi, pods: "110"}
EOF
}

# Each provider labels its topology differently, and the trees have different shapes.
nodes_cluster1() {   # IBM Cloud: zone -> rack -> node   (2 zones x 2 racks x 2 nodes)
  for z in 1 2; do for r in 1 2; do for n in 1 2; do
    fake_node "ibm-z$z-r$r-n$n" ibm.example.com/zone="z$z" ibm.example.com/rack="z$z-r$r"
  done; done; done
}
nodes_cluster2() {   # CoreWeave: superpod -> InfiniBand leaf -> node   (2 x 2 x 2)
  for s in 1 2; do for l in 1 2; do for n in 1 2; do
    fake_node "cw-sp$s-leaf$l-n$n" coreweave.example.com/superpod="sp$s" coreweave.example.com/ib-leaf="sp$s-leaf$l"
  done; done; done
}
nodes_cluster3() {   # On-prem: rack -> node   (3 racks x 2 nodes)
  for r in a01 a02 a03; do for n in 1 2; do
    fake_node "dc-$r-n$n" dc.example.com/rack="$r"
  done; done
}

if ! $H get deploy -n open-cluster-management-hub cluster-manager-work-controller >/dev/null 2>&1; then
  echo "Enabling the ManifestWorkReplicaSet feature on the hub"
  $H patch clustermanager cluster-manager --type=merge \
    -p '{"spec":{"workConfiguration":{"workDriver":"kube","featureGates":[{"feature":"ManifestWorkReplicaSet","mode":"Enable"}]}}}'
  until $H get deploy -n open-cluster-management-hub cluster-manager-work-controller >/dev/null 2>&1; do sleep 2; done
  $H rollout status -n open-cluster-management-hub deploy/cluster-manager-work-controller --timeout=180s
fi

for c in cluster1 cluster2 cluster3; do
  W="kubectl --context kind-$c"
  echo "[$c] Installing KWOK $KWOK_VERSION and adding fake ${PROVIDER[$c]} GPU nodes"
  $W apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/$KWOK_VERSION/kwok.yaml" >/dev/null
  $W apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/$KWOK_VERSION/stage-fast.yaml" >/dev/null
  $W rollout status -n kube-system deploy/kwok-controller --timeout=180s
  "nodes_$c" | $W apply -f - >/dev/null
  $W wait --for=condition=Ready node -l type=kwok --timeout=120s >/dev/null
done

echo "Labelling managed clusters with their provider"
for c in "${!PROVIDER[@]}"; do
  $H label managedcluster "$c" "provider=${PROVIDER[$c]}" --overwrite
done

echo "Creating Placements, Topologies, AdmissionChecks, ClusterQueues and LocalQueues on the hub"
$H apply -f "$DIR/hub/placements.yaml" -f "$DIR/hub/queues.yaml"

echo "Pushing each provider's Topology and queues to its clusters with ManifestWorkReplicaSets"
$H apply -f "$DIR/hub/worker-topology.yaml"
mwrs_ready() { local s; s=$($H get manifestworkreplicaset "tas-$1-worker-config" -n kueue-system \
  -o jsonpath='{.status.summary.available}/{.status.summary.total}' 2>/dev/null)
  [[ "$s" =~ ^[0-9]+/[0-9]+$ && "${s%/*}" -gt 0 && "${s%/*}" == "${s#*/}" ]]; }
for p in ibm coreweave onprem; do until mwrs_ready "$p"; do sleep 2; done; done

echo "Waiting for the kueue-addon to generate the MultiKueueConfigs"
for p in ibm coreweave onprem gpu-cloud; do
  until $H get multikueueconfig "tas-$p" >/dev/null 2>&1; do sleep 2; done
done
$H get multikueueconfig tas-ibm tas-coreweave tas-onprem tas-gpu-cloud \
  -o 'custom-columns=MKCONFIG:.metadata.name,CLUSTERS:.spec.clusters'
echo
for c in cluster1 cluster2 cluster3; do
  echo "[$c] $(kubectl --context kind-$c get topology -o name | tr '\n' ' ')$(kubectl --context kind-$c get nodes -l type=kwok --no-headers | wc -l) fake nodes"
done
