# OtelCollector-Addon

## What is OtelCollector-Addon?

OtelCollector Addon is a pluggable addon working on the extensibility provided by [addon-framework](https://github.com/open-cluster-management-io/addon-framework)
which automates the installation of otelCollector on both hub cluster and managed clusters and jaeget-all-in-one on hub cluster for processing and storing the traces.

OtelCollecotr Addon consists of two components:

- __Addon-Manager__: Manages the installation of hub components for setting up the Addon

- __Addon-Agent__: Manages the installation of collector agents in the managed clustres.

The overall architecture is shown below:

![Arch](./hack/picture/arch.png)


### Installing via Helm Chart :

```shell
$ helm install \
    -n open-cluster-management-addon --create-namespace \
    otel-collector charts/otel
```

### Verifying the addOn :
```
$ kubectl -n open-cluster-management-addon get pod
NAME                                            READY   STATUS    RESTARTS   AGE
jaeger-all-in-one-57dcf4b5-ltt46                1/1     Running   0          65s
otel-collector-75fd748b4c-cbt7k                 1/1     Running   0          65s
otel-collector-addon-manager-568885c78b-pl9db   1/1     Running   0          67s
```

