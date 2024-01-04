# fluid-addon

fluid-addon is an addon that integrates the [fluid](https://github.com/fluid-cloudnative/fluid) project into OCM.

## Install the fluid-addon to the Hub cluster

Switch context to Hub cluster.

```
$ kubectl apply -f deploy/addon
```

You can check if the fluid addon was deployed by:

```
$ kubectl get clustermanagementaddon fluid
$ kubectl get addontemplate fluid-0.0.1
```

## Enable the fluid addon for a managed cluster

Then apply a managedclusteraddon to enable the fluid for a managed cluster(eg cluster1) by:

```
# Replace 'cluster1' with the managed cluster name

$ MANAGED_CLUSTER=cluster1 \
  sed -e "s,MANAGED_CLUSTER,${MANAGED_CLUSTER}," deploy/sample/mca-fluid.yaml | \
  kubectl apply -f -
```

OR use the [clusteradm](https://github.com/open-cluster-management-io/clusteradm/) cli:

```
clusteradm addon enable --names=fluid --clusters=cluster1
```

You can check if the fluid addon was enabled by:

```
$ kubectl -n cluster1 get managedclusteraddon fluid # Replace 'cluster1' with the managed cluster name
NAME    AVAILABLE   DEGRADED   PROGRESSING
fluid   True                   False
```

## Verify the fluid components are installed on the Managed cluster

Switch context to the Managed cluster.

```
$ kubectl get pod -n fluid-system
NAME                                     READY   STATUS      RESTARTS   AGE
csi-nodeplugin-fluid-bw4qv               2/2     Running     0          16m
dataset-controller-64cf69f489-hcx7s      1/1     Running     0          16m
fluid-crds-upgrade-0.9.2-02f70ac-w7tft   0/1     Completed   0          16m
fluid-webhook-5998fb4c9-bxmxt            1/1     Running     0          16m
fluidapp-controller-6c59d668cf-pxhjc     1/1     Running     0          16m
```

## Verify the fluid addon is functioning

Switch context to the Managed cluster.

Please refer to the [Get Started of the fluid doc to crate a dataset](https://github.com/fluid-cloudnative/fluid/blob/v0.9.2/docs/en/userguide/get_started.md#create-a-dataset) to verify that the fluid is functioning properly.
