
```
podman build -t quay.io/dynamic-scoring/dynamic-scoring-framework-mcp:latest .
kind load docker-image quay.io/dynamic-scoring/dynamic-scoring-framework-mcp:latest --name hub
kubectl apply -f deployment.yaml
kubectl port-forward -n dynamic-scoring pod/$(kubectl get pods -n dynamic-scoring -l app=dynamic-scoring-framework-mcp -o name | head -1 | cut -d/ -f2) 8338:8338
```