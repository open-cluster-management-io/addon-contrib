#!/usr/bin/env bash

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/.." ; pwd -P)"
API_PKG="open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
OUTPUT_PKG="open-cluster-management-io/addon-contrib/device-addon/pkg/client"

set -o errexit
set -o nounset
set -o pipefail

set -x

GOBIN=${REPO_DIR}/bin

rm -rf ${REPO_DIR}/pkg/client

$GOBIN/deepcopy-gen -O zz_generated.deepcopy \
    --go-header-file="${REPO_DIR}/hack/boilerplate.go.txt" \
    --input-dirs="${API_PKG}"

$GOBIN/openapi-gen -O zz_generated.swagger_doc_generated \
    --go-header-file="${REPO_DIR}/hack/boilerplate.go.txt" \
    --input-dirs="k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/runtime,k8s.io/apimachinery/pkg/version,${API_PKG}" \
    --output-package="${API_PKG}"

$GOBIN/client-gen --go-header-file="${REPO_DIR}/hack/boilerplate.go.txt" \
    --clientset-name="versioned" \
    --input-base="" \
    --input="${API_PKG}" \
    --output-package="${OUTPUT_PKG}/clientset"

$GOBIN/lister-gen --go-header-file="${REPO_DIR}/hack/boilerplate.go.txt" \
    --input-dirs=${API_PKG} \
    --output-package="${OUTPUT_PKG}/listers"

$GOBIN/informer-gen --go-header-file="${REPO_DIR}/hack/boilerplate.go.txt" \
    --input-dirs="${API_PKG}" \
    --versioned-clientset-package="${OUTPUT_PKG}/clientset/versioned" \
    --listers-package="${OUTPUT_PKG}/listers" \
    --output-package="${OUTPUT_PKG}/informers"
