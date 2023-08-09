#!/usr/bin/env bash

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/.." ; pwd -P)"
API_PKG="open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
OUTPUT_PKG="open-cluster-management-io/addon-contrib/device-addon/pkg/client"

set -o errexit
set -o nounset
set -o pipefail

set -x

GOBIN=${REPO_DIR}/bin

$GOBIN/controller-gen crd:allowDangerousTypes=true \
    paths="${REPO_DIR}/pkg/apis/v1alpha1" \
    output:crd:artifacts:config="${REPO_DIR}/contrib/deploy/crds"
