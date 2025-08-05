# Configure Environment for Observability

This document provides a solution for monitoring sustainability and performance metrics across a multi-cluster Kubernetes environment managed by [Open Cluster Management (OCM)](https://open-cluster-management.io/).

It leverages [Kepler](https://github.com/sustainable-computing-io/kepler) to export energy consumption metrics, an [OpenTelemetry (OTel) Collector](https://opentelemetry.io/docs/collector/) to aggregate and forward these metrics, and [Prometheus](https://prometheus.io/) for centralized storage and querying.

## Architecture

The monitoring system is designed with the architecture:

![Architecture Diagram](../assets/images/obs_architecture.png)

*   **Prometheus**: A time-series database for storing and querying metrics, which will be installed on the hub cluster.
*   **OpenTelemetry Collector (otel-collector)**: A component that collects metrics from the managed clusters and forwards them to the Prometheus instance on the hub cluster. It will be installed on all managed clusters.
*   **Kepler**: A tool for measuring energy consumption of Kubernetes pods. It will be installed on all managed clusters.

## Prerequisites

Before you begin, ensure you have the following tools installed:

*   [Kind (Kubernetes in Docker)](https://kind.sigs.k8s.io/)
*   `kubectl`
*   `git`
*   `helm`

### Enable local-cluster

To collect metrics from the hub cluster itself, we first need to register it as a managed cluster, a common practice known as `local-cluster`.

We assume that you have already installed the environment with 2 managed clusters according to the [Set Up the Environment](../README.md#set-up-the-environment) guide.

```bash
# join command
joincmd=$(clusteradm get token --context kind-hub | grep clusteradm)

# join hub cluster as local-cluster
$(echo ${joincmd} --force-internal-endpoint-lookup --wait --context kind-hub | sed "s/<cluster_name>/local-cluster/g")

# accept local-cluster
clusteradm accept --context kind-hub --clusters local-cluster --wait
```

### Verify the environment

Now you can verify that the clusters are registered with the hub:

```bash
$ kubectl --context kind-hub get managedclusters
NAME            HUB ACCEPTED   MANAGED CLUSTER URLS                  JOINED   AVAILABLE   AGE
cluster1        true           https://cluster1-control-plane:6443   True     True        3h36m
cluster2        true           https://cluster2-control-plane:6443   True     True        3h36m
local-cluster   true           https://hub-control-plane:6443        True     True        3h35m
```

---

## Deployment Guide

Follow these steps to deploy the monitoring stack and the OpenTelemetry add-on.

### 1. Generate TLS Certificates

First, create a `monitoring` namespace on the hub cluster for Prometheus.

```bash
kubectl --context kind-hub create namespace monitoring
```

Next, run the certificate generation script. This script automates the creation of the necessary Certificate Authorities (CAs) and TLS certificates for securing communication (mTLS) between Prometheus and the OTel collectors. It will also create the required Kubernetes secrets.

```bash
cd federated-learning-controller/deploy/otel_addon/hack/certs
./generate-certs.sh
cd ../..
```

### 2. Install Prometheus

This step installs the `kube-prometheus-stack`, which includes Prometheus, on the hub cluster. The configuration enables the remote write receiver, allowing it to ingest metrics from the OTel collectors.

First, add the Prometheus community Helm repository:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
```

Now, install Prometheus using the provided Helm values file:

```bash
helm --kube-context kind-hub install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  -f ./hack/prom/values.yaml
```

### 3. Install the OpenTelemetry Add-on

This add-on deploys the necessary components (Kepler exporter and OTel collector) to the managed clusters via OCM's add-on framework.

First, create a namespace for the add-on on all clusters:
```bash
kubectl --context kind-hub create namespace open-cluster-management-addon
```

Then, apply the Kustomization to deploy the add-on resources:

```bash
kubectl --context kind-hub apply -k deploy
```

The OCM add-on manager will now distribute the OTel collector and Kepler to the managed clusters as defined by the `Placement` resource.

---

## Verification

After all components are deployed, you can verify that metrics are being collected from all clusters by querying Prometheus on the hub cluster.

Run a query for a Kepler metric, such as `kepler_container_joules_total`. The results should show time series with distinct `cluster_name` labels (`hub`, `cluster1`, `cluster2`), confirming that metrics are being successfully aggregated from all clusters.

**NOTE**: Please replace `hub-control-plane` with the actual address of the hub cluster. If you are using Kind in macOS or Windows, you can configure port mapping according to [extra port mapping](https://kind.sigs.k8s.io/docs/user/configuration/#extra-port-mappings).

```bash
$ curl -ksS https://hub-control-plane:30090/api/v1/query?query=kepler_container_joules_total | jq         
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "__name__": "kepler_container_joules_total",
          "cluster_name": "local-cluster",
          "container_id": "037ac5839960d853b607d77a8501c6ddc244cbe22f3e6994508662b5ea1376eb",
          "container_name": "klusterlet",
          "container_namespace": "open-cluster-management",
          "instance": "10.244.0.42:9102",
          "job": "kepler",
          "k8s_container_name": "kepler-exporter",
          "k8s_daemonset_name": "kepler-exporter",
          "k8s_namespace_name": "open-cluster-management-agent-addon",
          "k8s_node_name": "hub-control-plane",
          "k8s_pod_name": "kepler-exporter-sdljk",
          "k8s_pod_uid": "9c846e33-5e18-4150-9b9e-1517104b6972",
          "mode": "dynamic",
          "pod_name": "klusterlet-7d8bd449cc-zftcw",
          "server_address": "10.244.0.42",
          "server_port": "9102",
          "service_instance_id": "10.244.0.42:9102",
          "service_name": "kepler",
          "url_scheme": "http"
        },
        "value": [
          1753535725.037,
          "5.778"
        ]
      },
      {
        "metric": {
          "__name__": "kepler_container_joules_total",
          "cluster_name": "local-cluster",
          "container_id": "8abc271d1180f59ee706546513f07e34f2dac64f3c9d91d3641007e90c5fefc3",
          "container_name": "klusterlet",
          "container_namespace": "open-cluster-management",
          "instance": "10.244.0.42:9102",
          "job": "kepler",
          "k8s_container_name": "kepler-exporter",
          "k8s_daemonset_name": "kepler-exporter",
          "k8s_namespace_name": "open-cluster-management-agent-addon",
          "k8s_node_name": "hub-control-plane",
          "k8s_pod_name": "kepler-exporter-sdljk",
          "k8s_pod_uid": "9c846e33-5e18-4150-9b9e-1517104b6972",
          "mode": "idle",
          "pod_name": "klusterlet-7d8bd449cc-cj52x",
          "server_address": "10.244.0.42",
          "server_port": "9102",
          "service_instance_id": "10.244.0.42:9102",
          "service_name": "kepler",
          "url_scheme": "http"
        },
        "value": [
          1753535725.037,
          "0"
        ]
      },
      ...
    ]
  }
}
```