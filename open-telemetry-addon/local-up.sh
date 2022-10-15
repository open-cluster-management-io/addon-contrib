#!/bin/bash

cd $(dirname ${BASH_SOURCE})

set -e

hub=${HUB:-hub}
c1=${CLUSTER1:-cluster1}

hubctx="kind-${hub}"
c1ctx="kind-${c1}"

kind create cluster --name "${hub}"
kind create cluster --name "${c1}"

kubectl config use ${hubctx}
echo "Initialize the ocm hub cluster"
joincmd=$(clusteradm init --use-bootstrap-token --image-registry docker.io/nitishchauhan0022 --bundle-version=latest --wait | grep clusteradm)

kubectl config use ${c1ctx}
echo "Join cluster1 to hub"
$(echo ${joincmd} --image-registry docker.io/nitishchauhan0022 --bundle-version=latest --force-internal-endpoint-lookup --wait | sed "s/<cluster_name>/$c1/g")

kubectl config use ${hubctx}
echo "Accept join of cluster1"
clusteradm accept --clusters ${c1} --wait

