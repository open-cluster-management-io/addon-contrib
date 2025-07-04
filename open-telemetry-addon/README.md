# OtelCollector-Addon

## What is OtelCollector-Addon?

OtelCollector Addon is a pluggable addon working on the extensibility provided by [addon-framework](https://github.com/open-cluster-management-io/addon-framework)
which automates the installation of otelCollector on the managed clusters.


### Prerequisite

The otel-addon depends on prometheus-stack installed on the hub cluster. Before you get started, you must have a OCM environment setuped. You can also follow our recommended [quick start guide](https://open-cluster-management.io/docs/getting-started/quick-start/) to set up a playgroud OCM environment.

#### Add Helm Repo

```shell
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
```

#### Install Prometheus

1. You need to create `monitoring` namespace in hub cluster with this command:
```
kubectl --context kind-hub create namespace monitoring
```

2. You have to run the script to generate the certs before installing prometheus with mTLS enabled. The script do the following things:
- Generate root ca and key
- Generate client ca and key
- Generate server cert and key
- Create prometheus-tls secret in monitoring namespace
- Create otel-signer secret in open-cluster-management-hub namespace

```shell
cd hack/certs
bash ./generate-certs.sh
cd ../..
```

3. Install Prometheus with the following command:

```shell
helm --kube-context kind-hub install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  -f ./hack/prom/values.yaml
```

### Installing the otel-addon

```shell
kubectl --context kind-hub create namespace open-cluster-management-addon
kubectl --context kind-hub apply -k deploy
```

### Verifying the opentelemetry-collector in the managed clusters
```
kubectl --context kind-cluster1 get pod -n open-cluster-management-agent-addon
NAME                                       READY   STATUS    RESTARTS   AGE
opentelemetry-collector-66fcdc8779-zvhff   1/1     Running   0          27s
```

### Verify container advisor metrics
```
curl -k https://hub-control-plane:30090/api/v1/query?query=container_fs_inodes_total | jq .
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "__name__": "container_fs_inodes_total",
          "beta_kubernetes_io_arch": "amd64",
          "beta_kubernetes_io_os": "linux",
          "cluster_name": "cluster1",
          "device": "overlay_0-157",
          "id": "/",
          "instance": "cluster1-control-plane",
          "job": "opentelemetry-collector",
          "kubernetes_io_arch": "amd64",
          "kubernetes_io_hostname": "cluster1-control-plane",
          "kubernetes_io_os": "linux"
        },
        "value": [
          1751596235.174,
          "104840504"
        ]
      },
      {
        "metric": {
          "__name__": "container_fs_inodes_total",
          "beta_kubernetes_io_arch": "amd64",
          "beta_kubernetes_io_os": "linux",
          "cluster_name": "cluster2",
          "device": "overlay_0-313",
          "id": "/",
          "instance": "cluster2-control-plane",
          "job": "opentelemetry-collector",
          "kubernetes_io_arch": "amd64",
          "kubernetes_io_hostname": "cluster2-control-plane",
          "kubernetes_io_os": "linux"
        },
        "value": [
          1751596235.174,
          "104840504"
        ]
      }
      ...
    ]
  }
}
```