#!/usr/bin/env bash
# Live demo: one hub, two workers with different pod schedulers.
#   cluster1       default kube-scheduler
#   slurm-cluster  Slurm (slurm-bridge) schedules the pods
# The same Job is submitted to the hub; only its queue label differs.
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
H="kubectl --context ${HUB_CTX:-kind-local-cluster}"
KW="kubectl --context kind-${KUBE_WORKER:-cluster1}"
SW="kubectl --context kind-${SLURM_CLUSTER_NAME:-slurm-cluster}"
B=$'\e[1m'; C=$'\e[36m'; G=$'\e[32m'; Y=$'\e[33m'; N=$'\e[0m'
PAUSE=${PAUSE:-3}

step() { echo; echo "${B}${C}════ $* ════${N}"; sleep "$PAUSE"; }
run()  { echo "${Y}\$ $*${N}"; eval "$@"; }
slurm() { $SW exec -n slurm slurm-controller-0 -c slurmctld -- "$@"; }

pods() {   # $1 = kubectl command for a worker
  $1 get pods -n hpc --no-headers \
    -o 'custom-columns=P:.metadata.name,S:.spec.schedulerName,N:.spec.nodeName,PH:.status.phase' 2>/dev/null \
    | awk '{printf "  %-26s scheduler=%-23s node=%-23s %s\n",$1,$2,$3,$4}'
}

snapshot() {
  echo "${B}[hub] Kueue workloads${N}"
  $H get workloads -n hpc --no-headers --sort-by=.metadata.creationTimestamp \
    -o 'custom-columns=W:.metadata.name,Q:.spec.queueName,A:.status.conditions[?(@.type=="Admitted")].status,F:.status.conditions[?(@.type=="Finished")].status,C:.status.clusterName' 2>/dev/null \
    | awk 'BEGIN{printf "  %-30s %-12s %-9s %-9s %s\n","WORKLOAD","QUEUE","ADMITTED","FINISHED","WORKER"} {printf "  %-30s %-12s %-9s %-9s %s\n",$1,$2,$3,$4,$5}'
  echo "${B}[cluster1] pods, placed by kube-scheduler${N}"; pods "$KW"
  echo "${B}[slurm-cluster] pods, placed by Slurm${N}"; pods "$SW"
  echo "${B}[slurm-cluster] squeue${N}"
  slurm squeue -o '%.6i %.12j %.9T %.22N %.8M %r' 2>/dev/null | sed 's/^/  /'
}

watch_until_finished() {   # $1 = expected finished workloads, $2 = title
  for i in $(seq 1 60); do
    clear; echo "${B}${C}$2${N}   (refresh $i)"; snapshot
    done_n=$($H get workloads -n hpc -o 'jsonpath={range .items[*]}{.status.conditions[?(@.type=="Finished")].status}{"\n"}{end}' | grep -c True)
    [ "$done_n" -ge "$1" ] && { echo; echo "${G}All $1 workloads finished.${N}"; sleep "$PAUSE"; return; }
    sleep 3
  done
}

step "0. Clean slate"
run "$H delete jobs -n hpc --all --wait=true"

step "1. Two workers, labelled by the scheduler that places their pods"
run "$H get managedclusters -L scheduler | grep -E 'NAME|kube-scheduler|slurm'"
echo; echo "Each Placement becomes a MultiKueueConfig, and each has its own queue:"
run "$H get multikueueconfig kube-batch slurm -o 'custom-columns=MKCONFIG:.metadata.name,CLUSTERS:.spec.clusters'"
run "$H get localqueue -n hpc"

step "2. On slurm-cluster, Slurm owns the batch nodes"
run "$SW get nodes -L scheduler.slinky.slurm.net/external-node"
echo; echo "slurm-bridge registered those Kubernetes nodes in Slurm:"
run "slurm sinfo -N -o '%N %P %c %m %T'"
sleep "$PAUSE"

step "3. One Job, two schedulers: the files differ only in the queue label"
run "diff $DIR/workloads/01-job-kube-scheduler.yaml $DIR/workloads/02-job-slurm.yaml"
sleep "$PAUSE"

step "4. Submit one Job to each queue on the hub"
run "$H create -f $DIR/workloads/01-job-kube-scheduler.yaml"
run "$H create -f $DIR/workloads/02-job-slurm.yaml"
watch_until_finished 2 "Step 4: same Job → kube-scheduler on cluster1, Slurm on slurm-cluster"

step "5. Kueue admits, Slurm places: submit 4 more Slurm jobs for 2 Slurm nodes"
echo "Kueue's quota allows all 4. Slurm gives each pod a whole node, so 2 wait in squeue."
for i in 1 2 3 4; do run "$H create -f $DIR/workloads/02-job-slurm.yaml"; done
watch_until_finished 6 "Step 5: Kueue admitted all 4; Slurm queues what doesn't fit"

step "6. The hub saw every Job finish"
run "$H get jobs -n hpc"
echo; echo "slurm-bridge ran each pod as a stand-in Slurm job, cancelled when the pod finished:"
run "slurm scontrol show job | grep -oE 'JobId=[0-9]+|JobName=[^ ]+|JobState=[A-Z]+|^   NodeList=[^ ]+' | paste - - - - | sed 's/^/  /'"
echo; echo "${G}${B}Demo complete.${N}"
