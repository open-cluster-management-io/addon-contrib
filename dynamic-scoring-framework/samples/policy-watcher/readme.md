
```
podman build -t quay.io/dynamic-scoring/policy-watcher:v0.1.0 .
kind load docker-image quay.io/dynamic-scoring/policy-watcher:v0.1.0 --name cluster1
kubectl delete -f cluster1/deployment.yaml --context kind-cluster1
kubectl apply -f cluster1/deployment.yaml --context kind-cluster1
kind load docker-image quay.io/dynamic-scoring/policy-watcher:v0.1.0 --name cluster2
kubectl delete -f cluster2/deployment.yaml --context kind-cluster2
kubectl apply -f cluster2/deployment.yaml --context kind-cluster2
```