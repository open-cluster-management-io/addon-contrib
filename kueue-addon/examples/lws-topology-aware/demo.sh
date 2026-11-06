#!/usr/bin/env bash
# Live demo: Kueue Topology Aware Scheduling (TAS) for LeaderWorkerSet across clusters whose
# topology labels differ, with OCM Placement + Kueue MultiKueue.
#   cluster1 = ibm        zone -> rack -> node
#   cluster2 = coreweave  superpod -> ib-leaf -> node
#   cluster3 = onprem     rack -> node   (starts unlabelled and joins halfway through)
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context kind-local-cluster"
NS=inference-tas
declare -A KEYS=(
  [cluster1]="ibm.example.com/zone ibm.example.com/rack"
  [cluster2]="coreweave.example.com/superpod coreweave.example.com/ib-leaf"
  [cluster3]="dc.example.com/rack")
B=$'\e[1m'; C=$'\e[36m'; G=$'\e[32m'; Y=$'\e[33m'; R=$'\e[31m'; N=$'\e[0m'
PAUSE=${PAUSE:-3}

step() { echo; echo "${B}${C}════ $* ════${N}"; sleep "$PAUSE"; }
run()  { echo "${Y}\$ $*${N}"; eval "$@"; }

# node name -> "zone/rack" (or the provider's equivalent) for one cluster
node_domains() {
  local c=$1 cols="N:.metadata.name" k
  for k in ${KEYS[$c]}; do cols+=",${k##*/}:.metadata.labels.${k//./\\.}"; done
  kubectl --context kind-$c get nodes -l type=kwok --no-headers -o "custom-columns=$cols" 2>/dev/null \
    | awk '{d=$2; for (i=3;i<=NF;i++) d=d"/"$i; print $1, d}'
}

snapshot() {
  echo "${B}[hub] Kueue workloads (one per LWS group)${N}"
  $H get workloads -n $NS --no-headers --sort-by=.metadata.name \
    -o 'custom-columns=W:.metadata.name,Q:.spec.queueName,A:.status.conditions[?(@.type=="Admitted")].status,C:.status.clusterName' 2>/dev/null \
    | awk 'BEGIN{printf "  %-40s %-17s %-9s %s\n","WORKLOAD","QUEUE","ADMITTED","WORKER"} {printf "  %-40s %-17s %-9s %s\n",$1,$2,$3,$4}'
  for c in cluster1 cluster2 cluster3; do
    label=$($H get managedcluster "$c" -o jsonpath='{.metadata.labels.provider}')
    levels=$(echo ${KEYS[$c]} | sed 's#[^ ]*/##g; s# #/#g')
    echo "${B}[$c / ${label:-<no provider label>}] pods → node → ${levels}${N}"
    kubectl --context kind-$c get pods -n $NS --no-headers \
      -o 'custom-columns=P:.metadata.name,TYPE:.metadata.labels.pod-type,NODE:.spec.nodeName,S:.status.phase' 2>/dev/null \
      | awk -v map="$(node_domains $c)" 'BEGIN{n=split(map,l,"\n"); for(i=1;i<=n;i++){split(l[i],f," "); d[f[1]]=f[2]}}
          {printf "  %-20s %-7s %-20s %-16s %s\n",$1,$2,$3,d[$3],$4}' | sort -k4,4 -k1,1
  done
}

running_pods() {
  local n=0 c
  for c in cluster1 cluster2 cluster3; do
    n=$((n + $(kubectl --context kind-$c get pods -n $NS --no-headers 2>/dev/null | grep -c Running)))
  done
  echo "$n"
}

watch_admitted() {   # $1 = expected admitted workloads, $2 = expected running pods, $3 = title
  local i adm ready
  for i in $(seq 1 40); do
    clear; echo "${B}${C}$3${N}   (refresh $i)"; snapshot
    adm=$($H get workloads -n $NS -o 'jsonpath={range .items[*]}{.status.conditions[?(@.type=="Admitted")].status}{"\n"}{end}' | grep -c True)
    ready=$(running_pods)
    [ "$adm" -ge "$1" ] && [ "$ready" -ge "$2" ] && { echo; echo "${G}$adm workloads admitted, $ready pods running.${N}"; sleep "$PAUSE"; return; }
    sleep 3
  done
}

step "0. Clean slate: cluster3 is registered with OCM but not yet in the on-prem pool"
run "$H delete lws -n $NS --all --wait=true"
run "$H label managedcluster cluster3 provider- 2>/dev/null || true"
sleep 5

step "1. Three providers, three ways of labelling topology"
for c in cluster1 cluster2 cluster3; do
  cols=$(for k in ${KEYS[$c]}; do printf -- "-L %s " "$k"; done)
  run "kubectl --context kind-$c get nodes -l type=kwok $cols"
done
sleep "$PAUSE"

step "2. The hub has one Topology per provider, behind that provider's queue"
run "$H get topology ibm coreweave onprem gpu-cloud -o 'custom-columns=TOPOLOGY:.metadata.name,LEVELS:.spec.levels[*].nodeLabel'"
echo; echo "A ManifestWorkReplicaSet per provider pushes the same Topology to that provider's clusters:"
for c in cluster1 cluster2 cluster3; do
  run "kubectl --context kind-$c get topology -o 'custom-columns=TOPOLOGY:.metadata.name,LEVELS:.spec.levels[*].nodeLabel' 2>&1"
done
echo "(cluster3 has none yet: it isn't labelled as on-prem)"
sleep "$PAUSE"

step "3. Multi-node serving on IBM Cloud: each group of 4 pods must share one IBM rack"
run "grep -E 'name: mixtral|queue-name|replicas:|size:|required-topology' $DIR/workloads/01-lws-ibm.yaml | head -6"
sleep "$PAUSE"
run "$H apply -f $DIR/workloads/01-lws-ibm.yaml"
watch_admitted 2 8 "Step 3: mixtral → IBM, one rack per group"

step "4. Disaggregated serving on CoreWeave: each group behind one InfiniBand leaf"
run "grep -E 'required-topology' $DIR/workloads/02-ds-coreweave.yaml | sort -u"
run "$H apply -f $DIR/workloads/02-ds-coreweave.yaml"
watch_admitted 5 16 "Step 4: llama prefill + decode → CoreWeave, one ib-leaf per group"

step "5. A mistake: IBM's rack label on the CoreWeave queue"
run "grep -E 'queue-name|required-topology' $DIR/workloads/03-wrong-label.yaml | sort -u"
run "$H apply -f $DIR/workloads/03-wrong-label.yaml"
sleep 5
wl=$($H get workloads -n $NS -o name | grep llama-typo | head -1)
echo "${R}$($H get -n $NS "$wl" -o jsonpath='{.status.conditions[?(@.type=="QuotaReserved")].message}')${N}"
echo "The hub rejects it before anything is sent to a worker."
sleep "$PAUSE"
run "$H delete -f $DIR/workloads/03-wrong-label.yaml --wait=true"

step "6. Regulated serving on onprem-queue, but no cluster is on-prem yet"
run "$H apply -f $DIR/workloads/04-lws-onprem.yaml"
for i in 1 2 3 4; do
  clear; echo "${B}${C}Step 6: granite waits: there is no on-prem cluster${N}   (refresh $i)"; snapshot
  echo; echo "${R}onprem-queue: $($H get clusterqueue cq-tas-onprem -o jsonpath='{.status.conditions[?(@.type=="Active")].message}')${N}"
  sleep 3
done

step "7. Onboard the on-prem cluster: one label brings its topology, queues and routing"
run "$H label managedcluster cluster3 provider=onprem"
for i in $(seq 1 30); do
  cl=$($H get multikueueconfig tas-onprem -o jsonpath='{.spec.clusters}' 2>/dev/null)
  tp=$(kubectl --context kind-cluster3 get topology onprem -o jsonpath='{.spec.levels[*].nodeLabel}' 2>/dev/null)
  echo "  MultiKueueConfig tas-onprem: ${cl:-<not found>}   cluster3 Topology onprem: ${tp:-<not yet>}"
  [ -n "$cl" ] && [ -n "$tp" ] && break; sleep 2
done
watch_admitted 6 20 "Step 7: cluster3 joined the on-prem pool → granite placed in one data-center rack"

step "8. Burst to any public cloud: no shared rack label, so prefer the level they share (the node)"
run "grep -E 'queue-name|preferred-topology' $DIR/workloads/05-lws-gpu-cloud.yaml | sort -u"
run "$H apply -f $DIR/workloads/05-lws-gpu-cloud.yaml"
watch_admitted 8 24 "Step 8: burst-a / burst-b → IBM or CoreWeave, each group on one node"

step "9. Delete on the hub → MultiKueue removes the copies from the workers"
run "$H delete lws -n $NS --all --wait=true"
sleep 8
for c in cluster1 cluster2 cluster3; do run "kubectl --context kind-$c get lws -n $NS"; done
echo; echo "${G}${B}Demo complete.${N} cluster3 keeps its provider=onprem label."
