#!/bin/bash

cd $(dirname ${BASH_SOURCE})

set -euo pipefail

# Parse command line arguments
CLEAN=false
E2E_MODE=false
KUEUE_VERSION="v0.11.9"
while [[ $# -gt 0 ]]; do
  case $1 in
    --clean)
      CLEAN=true
      shift
      ;;
    --e2e)
      E2E_MODE=true
      shift
      ;;
    --kueue-version)
      KUEUE_VERSION="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      echo "Usage: $0 [--clean] [--e2e] [--kueue-version VERSION]"
      exit 1
      ;;
  esac
done

hub=${HUB:-local-cluster}
c1=${CLUSTER1:-cluster1}
c2=${CLUSTER2:-cluster2}
c3=${CLUSTER3:-cluster3}

hubctx="kind-${hub}"
c1ctx="kind-${c1}"
c2ctx="kind-${c2}"
c3ctx="kind-${c3}"

spoke_clusters=(${c1} ${c2} ${c3})
all_clusters=(${hub} ${spoke_clusters[@]})
spoke_ctx=(${c1ctx} ${c2ctx} ${c3ctx})
all_ctx=(${hubctx} ${spoke_ctx[@]})

kueue_manifest="https://github.com/kubernetes-sigs/kueue/releases/download/${KUEUE_VERSION}/manifests.yaml"
jobset_manifest="https://github.com/kubernetes-sigs/jobset/releases/download/v0.7.1/manifests.yaml"
mpi_operator_manifest="https://raw.githubusercontent.com/kubeflow/mpi-operator/master/deploy/v2beta1/mpi-operator.yaml"
training_operator_kustomize="github.com/kubeflow/training-operator.git/manifests/overlays/standalone?ref=v1.8.1"
ray_operator_crd_manifest="github.com/ray-project/kuberay/ray-operator/config/crd?ref=v1.3.0"
appwrapper_manifest="https://github.com/project-codeflare/appwrapper/releases/download/v1.1.2/install.yaml"

# Function to create kind clusters
create_clusters() {
  if [[ "$CLEAN" == "true" ]]; then
    echo "Deleting existing clusters due to --clean flag..."
    for cluster in "${all_clusters[@]}"; do
      kind delete cluster --name "$cluster" || true
    done
  fi

  echo "Prepare kind clusters"
  for cluster in "${all_clusters[@]}"; do
    kind create cluster --name "$cluster" --image kindest/node:v1.29.0 || true
  done
}

# Function to setup OCM
setup_ocm() {
  echo "Initialize the ocm hub cluster"
  clusteradm init --wait --context ${hubctx}
  joincmd=$(clusteradm get token --context ${hubctx} | grep clusteradm)

  echo "Join clusters to hub"
  eval "${joincmd//<cluster_name>/${hub}} --force-internal-endpoint-lookup --wait --context ${hubctx}"
  eval "${joincmd//<cluster_name>/${c1}} --force-internal-endpoint-lookup --wait --context ${c1ctx}"
  eval "${joincmd//<cluster_name>/${c2}} --force-internal-endpoint-lookup --wait --context ${c2ctx}"
  eval "${joincmd//<cluster_name>/${c3}} --force-internal-endpoint-lookup --wait --context ${c3ctx}"

  echo "Accept join of clusters"
  clusteradm accept --context ${hubctx} --clusters ${hub},${c1},${c2},${c3} --wait

  # label local-cluster
  kubectl label managedclusters ${hub} local-cluster=true --context ${hubctx}
  kubectl get managedclusters --all-namespaces --context ${hubctx}
}

# Function to install Kueue, jobset, workflow
install_kueue() {
  for ctx in "${all_ctx[@]}"; do
      echo "Install Kueue, Jobset on $ctx"
      kubectl apply --server-side -f "$kueue_manifest" --context "$ctx"
      echo "waiting for kueue-system pods to be ready"
      kubectl wait --for=condition=Ready pods --all -n kueue-system --timeout=300s --context "$ctx"
      kubectl apply --server-side -f "$jobset_manifest" --context "$ctx"
  done

  for ctx in "${spoke_ctx[@]}"; do
      echo "Install Kubeflow MPI Operator, Training Operator on $ctx"
      kubectl apply --server-side -f "$mpi_operator_manifest" --context "$ctx" || true
      kubectl apply --server-side -f "$appwrapper_manifest" --context "$ctx" || true
      kubectl apply --server-side -k "$training_operator_kustomize" --context "$ctx" || true
      kubectl apply --server-side -k "$ray_operator_crd_manifest" --context "$ctx" || true
  done
}

# Function to install OCM addons
install_ocm_addons() {
  kubectl config use-context ${hubctx}

  echo "Add ocm helm repo"
  helm repo add ocm https://open-cluster-management.io/helm-charts/
  helm repo update

  echo "Install managed-serviceaccount"
  helm upgrade --install \
     -n open-cluster-management-addon --create-namespace \
     managed-serviceaccount ocm/managed-serviceaccount \
     --set featureGates.ephemeralIdentity=true \
     --set enableAddOnDeploymentConfig=true \
     --set hubDeployMode=AddOnTemplate

  echo "Install cluster-proxy"
  helm upgrade --install \
    -n open-cluster-management-addon --create-namespace \
    cluster-proxy ocm/cluster-proxy \
    --set installByPlacement.placementName=global \
    --set installByPlacement.placementNamespace=open-cluster-management-addon

  echo "Install cluster-permission"
  helm upgrade --install \
    -n open-cluster-management --create-namespace \
     cluster-permission ocm/cluster-permission \
    --set global.imageOverrides.cluster_permission=quay.io/open-cluster-management/cluster-permission:latest

  if [[ "$E2E_MODE" == "true" ]]; then
  echo "Install kueue-addon from local chart"
  helm upgrade --install \
      -n open-cluster-management-addon --create-namespace \
      kueue-addon ../charts/kueue-addon \
      --set image.tag=e2e
  else
    echo "Install kueue-addon"
    helm upgrade --install \
      -n open-cluster-management-addon --create-namespace \
      kueue-addon ocm/kueue-addon
  fi

  echo "Install resource-usage-collect-addon"
  git clone https://github.com/open-cluster-management-io/addon-contrib.git || true
  cd addon-contrib/resource-usage-collect-addon
  helm install resource-usage-collect-addon chart/ \
    -n open-cluster-management-addon --create-namespace \
    --set skipClusterSetBinding=true \
    --set global.image.repository=quay.io/haoqing/resource-usage-collect-addon
  cd -

  rm -rf addon-contrib
}

# Function to setup fake GPU
setup_fake_gpu() {
  echo "Setup fake GPU on the spoke clusters"
  kubectl label managedcluster cluster2 accelerator=nvidia-tesla-t4 --context ${hubctx}
  kubectl label managedcluster cluster3 accelerator=nvidia-tesla-t4 --context ${hubctx}

  kubectl patch node cluster2-control-plane --subresource=status --type='merge' --patch='{
    "status": {
      "capacity": {
        "nvidia.com/gpu": "3"
      },
      "allocatable": {
        "nvidia.com/gpu": "3"
      }
    }
  }' --context ${c2ctx}

  kubectl patch node cluster3-control-plane --subresource=status --type='merge' --patch='{
    "status": {
      "capacity": {
        "nvidia.com/gpu": "3"
      },
      "allocatable": {
        "nvidia.com/gpu": "3"
      }
    }
  }' --context ${c3ctx}

  echo "Fake GPU resources added successfully to cluster2 and cluster3!"
}

# Function to load e2e images
load_e2e_images() {
  if [[ "$E2E_MODE" == "true" ]]; then
    echo "Loading e2e images to hub cluster"
    echo "Loading kueue-addon:e2e image to hub cluster"
    kind load docker-image --name="${hub}" quay.io/open-cluster-management/kueue-addon:e2e || echo "Warning: Failed to load image to ${hub}, continuing..."
  fi
}

# Main execution
create_clusters
setup_ocm
load_e2e_images
install_kueue
install_ocm_addons
setup_fake_gpu
