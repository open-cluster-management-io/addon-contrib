#!/usr/bin/env bash
# Removes everything setup.sh added. Provider labels on the managed clusters are left in place
# because the lws-multi-provider example uses them too.
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context ${HUB_CTX:-kind-local-cluster}"
KWOK_VERSION=${KWOK_VERSION:-v0.8.0}
rc=0

# Best-effort cleanup: run every step even if one fails (e.g. a cluster is unreachable),
# then exit nonzero if any did. Resources that are already absent count as cleaned up.
cleanup() { "$@" || { echo "WARNING: cleanup step failed: $*" >&2; rc=1; }; }

cleanup $H delete lws -n inference-tas --all --wait=true --ignore-not-found
cleanup $H delete -f "$DIR/hub/worker-topology.yaml" --ignore-not-found   # removes worker Topologies and queues
cleanup $H delete -f "$DIR/hub/queues.yaml" -f "$DIR/hub/placements.yaml" --ignore-not-found

for c in cluster1 cluster2 cluster3; do
  W="kubectl --context kind-$c"
  cleanup $W delete node -l type=kwok --ignore-not-found
  cleanup $W delete -f "https://github.com/kubernetes-sigs/kwok/releases/download/$KWOK_VERSION/stage-fast.yaml" --ignore-not-found
  cleanup $W delete -f "https://github.com/kubernetes-sigs/kwok/releases/download/$KWOK_VERSION/kwok.yaml" --ignore-not-found
done

exit $rc
