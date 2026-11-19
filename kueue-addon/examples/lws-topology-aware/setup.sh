#!/usr/bin/env bash
# Prepares the kind environment from build/setup-env.sh --clusterprofile for the LWS topology-aware example.
#   1. Installs KWOK on each worker and adds fake GPU nodes labelled with that provider's topology.
#   2. Labels the managed clusters with their provider.
#   3. Creates the hub Placements, Topologies and queues, and the ManifestWorkReplicaSets that
#      push each provider's Topology and queues to its clusters.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context ${HUB_CTX:-kind-local-cluster}"
KWOK_VERSION=${KWOK_VERSION:-v0.8.0}
declare -A PROVIDER=([cluster1]=ibm [cluster2]=coreweave [cluster3]=onprem)

# The examples run on the ClusterProfile environment from build/setup-env.sh --clusterprofile.
cp_mode=$($H get deploy kueue-addon-controller -n open-cluster-management-addon \
  -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="ENABLE_CLUSTERPROFILE")].value}' 2>/dev/null || true)
if [ "$cp_mode" != "true" ]; then
  echo "The kueue-addon is not running in ClusterProfile mode." >&2
  echo "Create the environment with: build/setup-env.sh --clusterprofile" >&2
  exit 1
fi

# Shared deadline (seconds) for every wait below. Normal startup delays stay within it;
# expiry prints the relevant resource conditions and fails the setup.
WAIT_TIMEOUT=${WAIT_TIMEOUT:-600}

# wait_until <description> <check-cmd> [args...] -- <diagnostics-cmd> [args...]
# Retries <check-cmd> until it succeeds or WAIT_TIMEOUT passes. On expiry runs the
# diagnostics command (if given) and exits nonzero.
wait_until() {
  local desc=$1 check=() diag=()
  shift
  while [ $# -gt 0 ] && [ "$1" != "--" ]; do check+=("$1"); shift; done
  if [ "${1:-}" = "--" ]; then shift; fi
  diag=("$@")
  local deadline=$(( $(date +%s) + WAIT_TIMEOUT ))
  until "${check[@]}"; do
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "ERROR: timed out after ${WAIT_TIMEOUT}s waiting for $desc" >&2
      if [ ${#diag[@]} -gt 0 ]; then "${diag[@]}" >&2 || true; fi
      exit 1
    fi
    sleep 2
  done
}

echo "Waiting for the ClusterProfile-backed MultiKueueClusters"
mkc_active() { [ "$($H get multikueuecluster "$1" \
  -o jsonpath='{.spec.clusterSource.clusterProfileRef.name}/{.status.conditions[?(@.type=="Active")].status}' \
  2>/dev/null)" = "$1/True" ]; }
for c in cluster1 cluster2 cluster3; do
  wait_until "MultiKueueCluster $c to be Active" mkc_active "$c" -- \
    $H get multikueuecluster "$c" -o yaml
done
$H get clusterprofile -n kueue-system cluster1 cluster2 cluster3

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
  work_controller_deployed() { $H get deploy -n open-cluster-management-hub cluster-manager-work-controller >/dev/null 2>&1; }
  wait_until "the cluster-manager-work-controller deployment" work_controller_deployed -- \
    $H get deploy -n open-cluster-management-hub cluster-manager-work-controller -o yaml
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
# summary.available is the count of available ManifestWorks: all of them (one per selected
# worker) must be available before the example can run.
mwrs_ready() { local s; s=$($H get manifestworkreplicaset "tas-$1-worker-config" -n kueue-system \
  -o jsonpath='{.status.summary.available}/{.status.summary.total}' 2>/dev/null)
  [[ "$s" =~ ^[0-9]+/[0-9]+$ && "${s%/*}" -gt 0 && "${s%/*}" == "${s#*/}" ]]; }
mwrs_diag() { $H get manifestworkreplicaset "tas-$1-worker-config" -n kueue-system -o yaml; }
for p in ibm coreweave onprem; do
  wait_until "ManifestWorkReplicaSet tas-$p-worker-config to be available on every selected worker" \
    mwrs_ready "$p" -- mwrs_diag "$p"
done

echo "Waiting for the kueue-addon to generate the MultiKueueConfigs"
for p in ibm coreweave onprem gpu-cloud; do
  wait_until "MultiKueueConfig tas-$p" $H get multikueueconfig "tas-$p" -- \
    $H get placement -n kueue-system "tas-$p" -o yaml
done
$H get multikueueconfig tas-ibm tas-coreweave tas-onprem tas-gpu-cloud \
  -o 'custom-columns=MKCONFIG:.metadata.name,CLUSTERS:.spec.clusters'
echo
for c in cluster1 cluster2 cluster3; do
  echo "[$c] $(kubectl --context kind-$c get topology -o name | tr '\n' ' ')$(kubectl --context kind-$c get nodes -l type=kwok --no-headers | wc -l) fake nodes"
done
