#!/usr/bin/env bash
# Removes everything setup.sh added. Provider labels on the managed clusters are left in place
# because the lws-topology-aware example uses them too.
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context ${HUB_CTX:-kind-local-cluster}"
rc=0

# Best-effort cleanup: run every step even if one fails, then exit nonzero if any did.
# Resources that are already absent (--ignore-not-found) count as cleaned up.
cleanup() { "$@" || { echo "WARNING: cleanup step failed: $*" >&2; rc=1; }; }

# Deleting on the hub makes MultiKueue remove the copies from the workers.
cleanup $H delete lws -n inference --all --wait=true --ignore-not-found
cleanup $H delete -f "$DIR/hub/worker-queues.yaml" --ignore-not-found   # removes worker LocalQueues and namespace
# Delete hub/queues.yaml piece by piece: it also holds the shared default-flavor
# ResourceFlavor that other examples use.
for p in ibm coreweave onprem gpu-cloud; do
  cleanup $H delete localqueue -n inference "$p-queue" --ignore-not-found
  cleanup $H delete clusterqueue "cq-$p" --ignore-not-found
  cleanup $H delete admissioncheck "mk-$p" "ocm-$p" --ignore-not-found
done
cleanup $H delete namespace inference --ignore-not-found
cleanup $H delete -f "$DIR/hub/placements.yaml" --ignore-not-found

exit $rc
