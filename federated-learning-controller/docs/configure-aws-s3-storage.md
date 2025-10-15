# Configure AWS S3 Storage

This guide walks through configuring an Amazon S3 bucket as the model storage backend for the federated learning controller. The server runs on the hub cluster, so you only need to complete these steps on the hub cluster.

## Prerequisites

- An AWS account with permissions to create or reuse an S3 bucket.
- An existing S3 bucket dedicated to model artifacts (the controller does not create it for you).
- A Kubernetes cluster with cluster-admin access.
- `kubectl` and `helm` configured against the hub cluster.
- Network connectivity from cluster nodes to the S3 endpoint that hosts your bucket.

> **Tip:** This guide uses static credentials stored in a Kubernetes secret to authenticate to S3.

## 1. Create the AWS Credential Secret

The CSI driver reads AWS credentials from a secret that must exist before the driver starts. Create `aws-secret.yaml` with the following content (replace the placeholders with your credentials) and apply it to the `kube-system` namespace.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-secret
  namespace: kube-system
stringData:
  key_id: <your-access-key-id>
  access_key: <your-secret-access-key>
```

Apply the manifest:

```bash
kubectl apply -f aws-secret.yaml
```

## 2. Install the AWS S3 CSI Driver

Install the [aws-s3-csi-driver](https://github.com/awslabs/mountpoint-s3-csi-driver) on the hub cluster. The driver runs in the `kube-system` namespace.

```bash
helm repo add aws-mountpoint-s3-csi-driver https://awslabs.github.io/mountpoint-s3-csi-driver
helm repo update

helm upgrade --install aws-mountpoint-s3-csi-driver \
    --namespace kube-system \
    aws-mountpoint-s3-csi-driver/aws-mountpoint-s3-csi-driver
```

Verify that the controller and node pods are running:

```bash
kubectl -n kube-system get pods -l app.kubernetes.io/name=aws-mountpoint-s3-csi-driver
```

For advanced installation and configuration scenarios, refer to the upstream Mountpoint S3 CSI driver documentation: [INSTALL.md](https://github.com/awslabs/mountpoint-s3-csi-driver/blob/main/docs/INSTALL.md) and [CONFIGURATION.md](https://github.com/awslabs/mountpoint-s3-csi-driver/blob/main/docs/CONFIGURATION.md).

## 3. Configure the Federated Learning Resource

With the driver in place, update the federated learning specification to reference S3-backed storage. The controller automatically creates a `PersistentVolume` and `PersistentVolumeClaim` that target the bucket.

```yaml
apiVersion: federation-ai.open-cluster-management.io/v1alpha1
kind: FederatedLearning
metadata:
  name: federated-learning-sample
spec:
  framework: flower
  server:
    image: <REGISTRY>/flower-app-torch:<IMAGE_TAG>
    rounds: 3
    minAvailableClients: 2
    listeners:
      - name: server-listener
        port: 8080
        type: NodePort
    storage:
      type: S3Bucket
      name: s3-pvc
      path: /data/models
      size: 2Gi
      s3:
        bucketName: <your-bucket-name>
        region: us-east-1            # optional but recommended
        prefix: models/round-1/      # optional logical folder within the bucket
  client:
    image: <REGISTRY>/flower-app-torch:<IMAGE_TAG>
    placement:
      clusterSets:
        - global
      predicates:
        - requiredClusterSelector:
            claimSelector:
              matchExpressions:
                - key: federated-learning-sample.client-data
                  operator: Exists

```

Key points:

- `bucketName` is required and must match the existing bucket.
- `region` adds a CSI mount option so the driver talks to the correct AWS endpoint.
- `prefix` scopes writes to a folder-like prefix (omit it to use the bucket root).
- The controller creates PV/PVC pairs named after `name` (for example `s3-pvc` and `s3-pvc-pv`); do not pre-create them.

After applying the CR, confirm that the controller provisioned the volume and claim:

```bash
kubectl get pv | grep s3
kubectl get pvc s3-pvc
```
