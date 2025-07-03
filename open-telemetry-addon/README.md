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
cd hack/cert
bash ./generate-certs.sh
```

3. Install Prometheus with the following command:

```shell
helm --kube-context kind-hub install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  -f ./hack/prom/values.yaml
```

### Installing the otel-addon

```shell
oc apply -k deploy
```

### Verifying the addOn
```
$ kubectl -n open-cluster-management-addon get pod
NAME                                            READY   STATUS    RESTARTS   AGE
jaeger-all-in-one-57dcf4b5-ltt46                1/1     Running   0          65s
otel-collector-75fd748b4c-cbt7k                 1/1     Running   0          65s
otel-collector-addon-manager-568885c78b-pl9db   1/1     Running   0          67s
```

