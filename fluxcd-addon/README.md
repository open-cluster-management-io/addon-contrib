This addon is a fork of https://github.com/kluster-manager/fluxcd-addon

https://open-cluster-management.io/developer-guides/addon/

https://github.com/open-cluster-management-io/addon-framework

https://github.com/kluster-management/addon-contrib/tree/main


```bash
> kubebuilder init --domain open-cluster-management.io --skip-go-version-check
> kubebuilder create api --group fluxcd --version v1alpha1 --kind FluxCDConfig
```

## Enable in Hub

```
kubectl apply -f api/config/samples/fluxcd_v1alpha1_fluxcdconfig.yaml
```

## Deploy to spoke clusters

```bash
clusteradm addon enable --names fluxcd-addon --namespace flux-system --clusters c1
```
