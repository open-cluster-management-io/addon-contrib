# hellospoke-addon

 hellospoke-addon is an example addon that showcase the capability of syncing a CR from the managed cluster to the hub cluster.

## Install the hellospoke-addon to the Hub cluster

Switch context to Hub cluster.

```
make deploy
```

You can check the addon manager status by:
```
$ kubectl -n open-cluster-management get deploy hellospoke-addon-manager
NAME                       READY   UP-TO-DATE   AVAILABLE   AGE
hellospoke-addon-manager   1/1     1            1           2m17s

kubectl -n cluster1 get managedclusteraddon hellospoke-addon # Replace 'cluster1' with the managed cluster name
NAME               AVAILABLE   DEGRADED   PROGRESSING
hellospoke-addon   True                   
```

## Verify the hellospoke-addon agent is installed on the Managed cluster and create a HelloSpoke CR

Switch context to Managed cluster.

```
$ kubectl -n open-cluster-management-agent-addon get deploy hellospoke-addon-agent
NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
hellospoke-addon-agent   1/1     1            1           4m23s
```

```
make deploy-hellospoke-cr-sample
```

## Verify the HelloSpoke CR is created on the Hub cluster

Switch context to Hub cluster.

```
$ kubectl -n cluster1 get hellospoke # Replace 'cluster1' with the managed cluster name
NAME         AGE
hellospoke   5m35s
```

## Update the HelloSpoke status on the Managed cluster

Using a tool such as [kubectl-edit-status](https://github.com/ulucinar/kubectl-edit-status), 
modify the HelloSpoke CR status to have the following:

```
status:
  spokeURL: hello
```

## Verify the  HelloSpoke CR status is updated on the Hub cluster

```
$ kubectl -n cluster1 get hellospoke hellospoke -o yaml
apiVersion: example.open-cluster-management.io/v1alpha1
kind: HelloSpoke
metadata:
...
  name: hellospoke
  namespace: cluster1
...
status:
  spokeURL: hello
```
