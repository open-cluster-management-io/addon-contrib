#!/usr/bin/env bash

REPO_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})/../.." ; pwd -P)"

demo_dir=${REPO_DIR}/contrib/demo
kubeconfig=${REPO_DIR}/_output/clusters/edge-demo-kind.kubeconfig

cluster="edge"

source ${demo_dir}/demo_magic

export KUBECONFIG=${kubeconfig}
comment "managed cluster and device addon on hub cluster"
pe "kubectl get managedclusters"
pe "kubectl -n ${cluster} get managedclusteraddons"

comment "enable the opcua driver on the edge cluster"
pe "kubectl -n ${cluster} apply -f ${demo_dir}/resources/opcua/driver.yaml"
pe "kubectl -n ${cluster} get drivers opcua -oyaml"

comment "enable a opcua device on the edge cluster"
pe "kubectl -n ${cluster} apply -f ${demo_dir}/resources/opcua/device.yaml"
pe "kubectl -n ${cluster} get devices opcua-s001 -oyaml"

comment "You can receive the data of device opcua-s001 with MQTT topic devices/+/data/+ on tcp://127.0.0.1:1883"

unset KUBECONFIG
