# Deploying Jupyter Notebook on OpenShift

## Building the Jupyter Notebook Image

OpenShift runs containers using randomized, non-root UIDs for enhanced security. However, the default Jupyter base images assume the `jovyan` user (UID 1000) has write permissions to `/home/jovyan`. When OpenShift assigns a high UID, Jupyter may attempt to fix the user mismatch but still lacks the necessary permissions for `/home/jovyan`.

### Solution: Custom Image with Relaxed Permissions

Build and push a custom Jupyter image with permissions compatible with OpenShift:

```bash
docker build -t quay.io/myan/scipy-notebook:2024-12-23-ocp . -f ocp.Dockerfile
docker push quay.io/myan/scipy-notebook:2024-12-23-ocp
```

Deploy the custom image to OpenShift:

```bash
cat <<EOF | oc apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jupyter-notebook
spec:
  replicas: 1
  selector:
    matchLabels:
      app: jupyter-notebook
  template:
    metadata:
      labels:
        app: jupyter-notebook
    spec:
      containers:
      - name: jupyter
        image: quay.io/myan/scipy-notebook:2024-12-23-ocp
        ports:
        - containerPort: 8888
        env:
        - name: JUPYTER_ENABLE_LAB
          value: "yes"
        volumeMounts:
        - name: model-volume
          mountPath: /data/models # Matches the path used by your server
      volumes:
      - name: model-volume
        persistentVolumeClaim:
          claimName: model-pvc # Same PVC used by FederatedLearning
EOF
```

Optional
```bash
cat <<EOF | oc apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: model-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi  
  storageClassName: gp3-csi  
EOF
```

---

## Accessing the Jupyter Notebook Locally

To access the Jupyter Notebook:

1. Forward the Jupyter Notebook port:
   ```bash
   kubectl port-forward deployment/jupyter-notebook 8888:8888
   ```

2. Open your browser and navigate to:
   ```
   http://localhost:8888
   ```

## Accessing the Jupyter Notebook by Route

```bash
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Route
metadata:
  name: jupyter-notebook
  labels:
    app: jupyter-notebook
spec:
  to:
    kind: Service
    name: jupyter-notebook
    weight: 100
  port:
    targetPort: "8888-tcp"
  tls:
    termination: edge
    insecureEdgeTerminationPolicy: Redirect
---
apiVersion: v1
kind: Service
metadata:
  name: jupyter-notebook
  labels:
    app: jupyter-notebook
spec:
  ports:
  - name: "8888-tcp"
    protocol: TCP
    port: 8888
    targetPort: 8888
  selector:
    app: jupyter-notebook
  type: ClusterIP
EOF
```

---

This setup ensures compatibility with OpenShift security policies and integrates seamlessly with your existing PVC for model storage.