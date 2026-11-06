#!/usr/bin/env bash
# Live demo: provider-aware placement of LeaderWorkerSet / disaggregated serving
# with OCM Placement + Kueue MultiKueue.
#   cluster1 = provider=ibm   cluster2 = provider=coreweave   cluster3 = provider=onprem
# cluster3 starts unlabelled and "joins" the on-prem pool halfway through.
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context kind-local-cluster"
declare -A PROV=([cluster1]=ibm [cluster2]=coreweave [cluster3]=onprem)
B=$'\e[1m'; C=$'\e[36m'; G=$'\e[32m'; Y=$'\e[33m'; R=$'\e[31m'; N=$'\e[0m'
PAUSE=${PAUSE:-3}

step() { echo; echo "${B}${C}════ $* ════${N}"; sleep "$PAUSE"; }
run()  { echo "${Y}\$ $*${N}"; eval "$@"; }

snapshot() {
  echo "${B}[hub] Kueue workloads (one per LWS group)${N}"
  $H get workloads -n inference --no-headers --sort-by=.metadata.name \
    -o 'custom-columns=W:.metadata.name,Q:.spec.queueName,A:.status.conditions[?(@.type=="Admitted")].status,C:.status.clusterName' 2>/dev/null \
    | awk 'BEGIN{printf "  %-40s %-17s %-9s %s\n","WORKLOAD","QUEUE","ADMITTED","WORKER"} {printf "  %-40s %-17s %-9s %s\n",$1,$2,$3,$4}'
  for c in cluster1 cluster2 cluster3; do
    label=$($H get managedcluster "$c" -o jsonpath='{.metadata.labels.provider}')
    echo "${B}[$c / ${label:-<no provider label>}] pods${N}"
    kubectl --context kind-$c get pods -n inference --no-headers \
      -o 'custom-columns=P:.metadata.name,APP:.metadata.labels.app,ROLE:.metadata.labels.role,TYPE:.metadata.labels.pod-type,S:.status.phase' 2>/dev/null \
      | awk '{printf "  %-22s app=%-11s role=%-8s %-7s %s\n",$1,$2,$3,$4,$5}'
  done
}

running_pods() {
  local n=0
  for c in cluster1 cluster2 cluster3; do
    n=$((n + $(kubectl --context kind-$c get pods -n inference --no-headers 2>/dev/null | grep -c Running)))
  done
  echo "$n"
}

watch_admitted() {   # $1 = expected admitted workloads, $2 = title
  for i in $(seq 1 30); do
    clear; echo "${B}${C}$2${N}   (refresh $i)"; snapshot
    adm=$($H get workloads -n inference -o 'jsonpath={range .items[*]}{.status.conditions[?(@.type=="Admitted")].status}{"\n"}{end}' | grep -c True)
    ready=$(running_pods)
    [ "$adm" -ge "$1" ] && [ "$ready" -ge $(( $1 * 2 )) ] && { echo; echo "${G}$adm workloads admitted, $ready pods running.${N}"; sleep "$PAUSE"; return; }
    sleep 3
  done
}

step "0. Clean slate: cluster3 is registered with OCM but not yet in the on-prem pool"
run "$H delete lws -n inference --all --wait=true"
run "$H label managedcluster cluster3 provider- 2>/dev/null || true"
sleep 5

step "1. Clusters are labelled by provider (cluster3 has no label yet)"
run "$H get managedclusters -L provider"

step "2. One OCM Placement per provider (plus a multi-provider 'gpu-cloud')"
run "$H get placement -n kueue-system ibm coreweave onprem gpu-cloud -o 'custom-columns=PLACEMENT:.metadata.name,PROVIDERS:.spec.predicates[0].requiredClusterSelector.labelSelector.matchExpressions[0].values,SELECTED:.status.numberOfSelectedClusters'"
echo; echo "kueue-addon turns each Placement into a MultiKueueConfig (none for onprem yet):"
run "$H get multikueueconfig -o 'custom-columns=MKCONFIG:.metadata.name,CLUSTERS:.spec.clusters' | grep -v default"
echo; echo "A ManifestWorkReplicaSet already pushed the LocalQueues to every worker, cluster3 included:"
run "$H get manifestworkreplicaset -n kueue-system lws-worker-queues -o jsonpath='{.status.summary}'; echo"
run "kubectl --context kind-cluster3 get localqueue -n inference"
sleep "$PAUSE"

step "3. Disaggregated serving on CoreWeave (prefill LWS + decode LWS → coreweave-queue)"
run "grep -E 'name:|queue-name|replicas:|size:' $DIR/workloads/01-ds-coreweave.yaml | head -12"
sleep "$PAUSE"
run "$H apply -f $DIR/workloads/01-ds-coreweave.yaml"
watch_admitted 3 "Step 3: llama-ds (prefill + decode) → CoreWeave only"

step "4. Submit regulated work to onprem-queue, but no cluster is on-prem yet"
run "$H apply -f $DIR/workloads/02-ds-onprem.yaml"
for i in 1 2 3 4; do
  clear; echo "${B}${C}Step 4: granite-ds waits: there is no on-prem cluster${N}   (refresh $i)"; snapshot
  echo; echo "${R}onprem-queue: $($H get clusterqueue cq-onprem -o jsonpath='{.status.conditions[?(@.type=="Active")].message}')${N}"
  sleep 3
done

step "5. Onboard the on-prem cluster: one label, no queue edits"
run "$H label managedcluster cluster3 provider=onprem"
for i in $(seq 1 10); do
  cl=$($H get multikueueconfig onprem -o jsonpath='{.spec.clusters}' 2>/dev/null)
  echo "  MultiKueueConfig onprem: ${cl:-<not found>}"; [ -n "$cl" ] && break; sleep 1
done
watch_admitted 5 "Step 5: cluster3 joined the on-prem pool → waiting granite-ds dispatched"

step "6. Multi-node LWS serving on IBM Cloud (ibm-queue)"
run "$H apply -f $DIR/workloads/03-lws-ibm.yaml"
watch_admitted 7 "Step 6: mixtral → IBM only"

step "7. Burst to ANY public cloud (gpu-cloud-queue = IBM or CoreWeave, never on-prem)"
run "$H apply -f $DIR/workloads/04-lws-gpu-cloud.yaml"
watch_admitted 9 "Step 7: burst-a / burst-b → IBM or CoreWeave (MultiKueue picks)"

step "8. What the hub sees"
run "$H get lws -n inference"
run "$H get pods -n inference"
echo "(no pods on the hub: it only holds the LWS objects; workers run them)"

step "9. Delete on the hub → MultiKueue removes the copies from the workers"
run "$H delete lws -n inference --all --wait=true"
sleep 8
for c in cluster1 cluster2 cluster3; do run "kubectl --context kind-$c get lws -n inference"; done
echo; echo "${G}${B}Demo complete.${N} cluster3 keeps its provider=onprem label."
