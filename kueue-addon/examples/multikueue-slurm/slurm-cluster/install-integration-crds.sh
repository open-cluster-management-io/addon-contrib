#!/usr/bin/env bash
# Installs ONLY the CRDs (no controllers) for every job integration the hub's Kueue enables.
# MultiKueue opens a watch on each worker for every enabled integration, and marks the
# MultiKueueCluster inactive ("no matches for kind ...") if any of these types is missing,
# even when that worker never runs that kind of job.
# Versions match build/setup-env.sh.
set -euo pipefail
CTX="${1:-kind-slurm-cluster}"

# Keep only CustomResourceDefinitions from a multi-document manifest and drop conversion
# webhooks, which would point at controllers that are not installed here.
crds_only() {
  python3 -c '
import sys, yaml
for d in yaml.safe_load_all(sys.stdin):
    if d and d.get("kind") == "CustomResourceDefinition":
        d["spec"].pop("conversion", None)
        print("---"); print(yaml.safe_dump(d, sort_keys=False))'
}
apply_url() { curl -sSfL "$1" | crds_only | kubectl --context "$CTX" apply --server-side -f - >/dev/null; echo "  $2"; }
apply_kustomize() { kubectl --context "$CTX" kustomize "$1" | crds_only | kubectl --context "$CTX" apply --server-side -f - >/dev/null; echo "  $2"; }

echo "Installing job integration CRDs on $CTX"
apply_url https://github.com/kubernetes-sigs/jobset/releases/download/v0.13.0/manifests.yaml "JobSet v0.13.0"
apply_url https://github.com/kubernetes-sigs/lws/releases/download/v1.0.0/manifests.yaml "LeaderWorkerSet v1.0.0"
apply_url https://raw.githubusercontent.com/kubeflow/mpi-operator/master/deploy/v2beta1/mpi-operator.yaml "MPIJob"
apply_url https://github.com/project-codeflare/appwrapper/releases/download/v1.1.2/install.yaml "AppWrapper v1.1.2"
apply_kustomize "github.com/ray-project/kuberay/ray-operator/config/crd?ref=v1.3.0" "RayJob, RayCluster, RayService v1.3.0"
apply_kustomize "github.com/kubeflow/trainer.git/manifests/base/crds?ref=v2.1.0" "TrainJob v2.1.0"
