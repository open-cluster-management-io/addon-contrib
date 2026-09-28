# cert-manager-addon

cert-manager-addon is an OCM addon that deploys [cert-manager](https://cert-manager.io/) to managed clusters using the [AddOnTemplate](https://open-cluster-management.io/concepts/addon/#addontemplate) framework.

## Why this addon?

Managing a fleet of clusters means cert-manager needs to be installed on every managed cluster — individually. Without this addon, a platform team must run `helm install` (or equivalent) once per cluster, track versions per cluster, and repeat the process for every new cluster that joins. At scale (tens or hundreds of clusters) that becomes error-prone and operationally expensive.

This addon solves that by letting a platform team deploy and configure cert-manager across **all managed clusters from the hub**, once:

- A newly joined cluster gets cert-manager automatically when it matches the `Placement`
- Version upgrades are done by updating one `AddOnDeploymentConfig` on the hub, not by touching each cluster
- Different cluster groups can run different cert-manager versions or use different registries, all controlled from the hub

cert-manager itself is a prerequisite for many workloads that need TLS certificates (ingress controllers, service meshes, internal PKI, webhook servers). This addon makes that prerequisite available fleet-wide without per-cluster manual steps.

## Overview

This addon deploys cert-manager directly using upstream Kubernetes manifests — **no OLM required**. Works on any Kubernetes distribution (Kind, GKE, EKS, AKS, OpenShift, etc.).

**Key Features:**

- CRDs installed via Job (downloads from official cert-manager releases)
- Works on any Kubernetes distribution
- Configurable via `AddOnDeploymentConfig` (image registry, version, CRDs URL)
- Label-based automatic deployment via `Placement`



## Quick Start



### 1. Install the addon on the Hub cluster

**Option A: Direct from GitHub (no clone needed)**

```bash
kubectl apply -k https://github.com/open-cluster-management-io/addon-contrib/cert-manager-addon/deploy
```

**Option B: Clone and apply**

```bash
git clone https://github.com/open-cluster-management-io/addon-contrib.git
cd addon-contrib/cert-manager-addon
kubectl apply -k deploy/
```



### 2. Enable on a managed cluster

Simply label the managed cluster:

```bash
kubectl label managedcluster <cluster-name> addon.open-cluster-management.io/cert-manager=enabled
```

That's it! The addon-manager will automatically deploy cert-manager to the labeled cluster.

### 3. Verify deployment

```bash
# Check addon status on hub
kubectl get managedclusteraddon cert-manager-addon -n <cluster-name>

# Check cert-manager on managed cluster
kubectl get pods -n cert-manager
```

### 4. Confirm cert-manager is working (end-to-end)

On the **managed cluster**, create a self-signed issuer and a certificate:

```bash
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-cert
  namespace: default
spec:
  secretName: example-tls
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
  dnsNames:
    - example.com
EOF
```

Wait for it to be issued:

```bash
kubectl wait certificate example-cert -n default --for=condition=Ready --timeout=60s
kubectl get secret example-tls -n default
```

`READY=True` confirms the full stack is working: CRDs are installed, the webhook is functional, the controller is running, and certificate issuance is operational.



## Architecture

The addon uses a **Job-based CRD installation** approach:

1. **CRD Installation Job** — Downloads and applies CRDs from official cert-manager GitHub releases
2. **Components** — Deploys cert-manager controller, webhook, and cainjector

This avoids Kubernetes object size limits (~256KB) that would prevent embedding large CRDs directly.

## Files

```
cert-manager-addon/
├── README.md
├── OWNERS
└── deploy/
    ├── kustomization.yaml            # Enables kubectl apply -k
    ├── clustermanagementaddon.yaml   # Registers addon with addon-manager
    ├── addontemplate.yaml            # CRD Job + components
    ├── addondeploymentconfig.yaml    # Default configuration values
    └── placement.yaml                # Auto-deploy to labeled clusters
```



## Configuration

The addon supports customization via `AddOnDeploymentConfig`:


| Variable             | Default                           | Description                                             |
| -------------------- | --------------------------------- | ------------------------------------------------------- |
| `certManagerVersion` | `v1.21.1`                         | cert-manager version (used for images); must match `certManagerCrdsUrl` |
| `certManagerCrdsUrl` | `https://github.com/cert-manager/cert-manager/releases/download/v1.21.1/cert-manager.crds.yaml` | URL to download CRDs; must match `certManagerVersion`  |
| `imageRegistry`      | `quay.io/jetstack`                | Container registry prefix for cert-manager              |
| `kubectlImage`       | `registry.k8s.io/kubectl:v1.31.4` | Image for the CRD install Job (mirror for disconnected) |


> **Note:** Webhook/controller replicas cannot use `AddOnDeploymentConfig` variables (OCM variables are strings; `replicas` requires int32). Edit `replicas` in `addontemplate.yaml` directly, or reference a cluster-specific AddOnTemplate.

> **Namespace:** `AddOnDeploymentConfig` sets `agentInstallNamespace: ""` so workloads deploy to the `cert-manager` namespace defined in the AddOnTemplate. Without this, OCM defaults to `open-cluster-management-agent-addon`.



### Webhook replicas (HA)

`AddOnDeploymentConfig` cannot set Deployment replicas (integer fields). To run multiple webhook replicas, edit `deploy/addontemplate.yaml`:

```yaml
# cert-manager-webhook Deployment
spec:
  replicas: 3
```

Then re-apply:

```bash
kubectl apply -f cert-manager-addon/deploy/addontemplate.yaml
kubectl delete manifestwork addon-cert-manager-addon-deploy-0 -n <cluster-name>
```

For per-cluster HA, create a separate `AddOnTemplate` (e.g. `cert-manager-addon-ha`) with higher replicas and reference it in that cluster's `ManagedClusterAddOn` configs.

### Custom configuration per cluster

Create a cluster-specific config and reference it in the `ManagedClusterAddOn`:

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: AddOnDeploymentConfig
metadata:
  name: cert-manager-addon-config-v1.20
  namespace: open-cluster-management
spec:
  customizedVariables:
    - name: certManagerVersion
      value: "v1.20.3"
    - name: certManagerCrdsUrl
      value: "https://github.com/cert-manager/cert-manager/releases/download/v1.20.3/cert-manager.crds.yaml"
    - name: imageRegistry
      value: "quay.io/jetstack"
---
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: cert-manager-addon
  namespace: prod-cluster
spec:
  installNamespace: cert-manager
  configs:
    - group: addon.open-cluster-management.io
      resource: addondeploymentconfigs
      name: cert-manager-addon-config-v1.20
      namespace: open-cluster-management
```



### Air-gapped / Disconnected environments

For disconnected environments, you need to mirror both images and CRDs:

**Step 1:** Mirror container images to your internal registry:

- `quay.io/jetstack/cert-manager-controller:v1.21.1`
- `quay.io/jetstack/cert-manager-cainjector:v1.21.1`
- `quay.io/jetstack/cert-manager-webhook:v1.21.1`
- `registry.k8s.io/kubectl:v1.31.4` (CRD install Job)

**Step 2:** Download and host the CRDs YAML on an internal server:

```bash
curl -L https://github.com/cert-manager/cert-manager/releases/download/v1.21.1/cert-manager.crds.yaml \
  -o /path/to/internal-server/cert-manager-crds.yaml
```

**Step 3:** Create a custom `AddOnDeploymentConfig`:

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: AddOnDeploymentConfig
metadata:
  name: cert-manager-addon-config-disconnected
  namespace: open-cluster-management
spec:
  customizedVariables:
    - name: certManagerVersion
      value: "v1.21.1"
    - name: certManagerCrdsUrl
      value: "https://internal-mirror.example.com/cert-manager-crds.yaml"  # Your internal URL
    - name: imageRegistry
      value: "my-registry.example.com/jetstack"  # Your internal registry
    - name: kubectlImage
      value: "my-registry.example.com/kubectl:v1.31.4"  # Mirrored kubectl image
```

**Step 4:** Reference it in the `ManagedClusterAddOn`:

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: cert-manager-addon
  namespace: disconnected-cluster
spec:
  installNamespace: cert-manager
  configs:
    - group: addon.open-cluster-management.io
      resource: addondeploymentconfigs
      name: cert-manager-addon-config-disconnected
      namespace: open-cluster-management
```



## Manual Enablement (Alternative to Placement)

If you prefer manual control instead of label-based auto-deployment:

1. Edit `clustermanagementaddon.yaml` to use `type: Manual`:

```yaml
installStrategy:
  type: Manual
```

1. Create `ManagedClusterAddOn` per cluster:

```bash
kubectl apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: cert-manager-addon
  namespace: <cluster-name>
spec:
  installNamespace: cert-manager
EOF
```



## What Gets Deployed

When enabled on a managed cluster:

### CRD Installation (via Job)

- Downloads CRDs from `https://github.com/cert-manager/cert-manager/releases/download/<version>/cert-manager.crds.yaml`
- Installs: `certificates`, `certificaterequests`, `issuers`, `clusterissuers`, `challenges`, `orders`

### Components

- **Namespace**: `cert-manager` with Pod Security Standards
- **Deployments**: `cert-manager`, `cert-manager-cainjector`, `cert-manager-webhook`
- **Services**: `cert-manager`, `cert-manager-webhook`
- **RBAC**: ClusterRoles, ClusterRoleBindings, Roles, RoleBindings

### Verifying everything works

Listing objects that were created doesn't prove cert-manager is functional. The real test is issuing a certificate. Run this on the **managed cluster** after the addon shows `Available`:

```bash
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-cert
  namespace: default
spec:
  secretName: example-tls
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
  dnsNames:
    - example.com
EOF

kubectl wait certificate example-cert -n default --for=condition=Ready --timeout=60s
```

Expected output:
```
certificate.cert-manager.io/example-cert condition met
```

This single result proves the full stack is operational:

| What `READY=True` confirms | Which component |
|---|---|
| cert-manager CRDs are installed | CRD install Job |
| cert-manager resource operations are accepted | Webhook |
| Certificate lifecycle is being processed | Controller |
| TLS Secret was written | Controller |

Clean up when done:
```bash
kubectl delete certificate example-cert -n default
kubectl delete clusterissuer selfsigned-issuer
```



## Troubleshooting



### Check addon status

```bash
kubectl get managedclusteraddon cert-manager-addon -n <cluster-name> -o yaml
```



### Check CRD installation Job

```bash
# On managed cluster
kubectl get jobs -n cert-manager
kubectl logs -n cert-manager job/cert-manager-crd-install
```



### Check ManifestWork

```bash
kubectl get manifestwork -n <cluster-name> | grep cert-manager
```



### Common issues


| Symptom                        | Cause              | Solution                                           |
| ------------------------------ | ------------------ | -------------------------------------------------- |
| CRD Job failed                 | No internet access | Mirror CRDs for air-gapped environments            |
| cert-manager pods not starting | CRDs not installed | Check CRD Job logs                                 |
| Webhook timeout errors         | Webhook not ready  | Wait for webhook pod; check certificate generation |




## Upgrading cert-manager

1. Update both version variables in `addondeploymentconfig.yaml` — `certManagerVersion` and `certManagerCrdsUrl` must always refer to the same release, because CRDs are installed separately by the Job and must match the controller images:

```yaml
- name: certManagerVersion
  value: "v1.21.1"
- name: certManagerCrdsUrl
  value: "https://github.com/cert-manager/cert-manager/releases/download/v1.21.1/cert-manager.crds.yaml"
```

2. Re-apply:

```bash
kubectl apply -k deploy/
```

3. Delete the completed CRD install Job on each managed cluster to trigger re-download with the new CRDs:

```bash
kubectl delete job cert-manager-crd-install -n cert-manager --context <cluster>
```

## Related Links

- [cert-manager documentation](https://cert-manager.io/docs/)
- [OCM AddOnTemplate documentation](https://open-cluster-management.io/concepts/addon/#addontemplate)
- [cert-manager releases](https://github.com/cert-manager/cert-manager/releases)

