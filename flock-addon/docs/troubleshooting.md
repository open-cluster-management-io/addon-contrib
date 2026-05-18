# Troubleshooting

Use this guide when `flock-addon` deploys on the hub but does not behave as expected on managed clusters.

> The snippets below assume `rg` ([ripgrep](https://github.com/BurntSushi/ripgrep)) is installed for log filtering. Every `rg <pattern>` is equivalent to `grep -E '<pattern>'` — substitute one for the other on hosts where ripgrep is not available (`sudo apt install -y ripgrep` on Debian/Ubuntu, `brew install ripgrep` on macOS).

## Image Pull Problems

If the managed cluster Pod is stuck in `ImagePullBackOff` or `ErrImagePull`, inspect Pod events first.

```bash
# [Managed Cluster context]
kubectl -n flock-system describe pod -l app.kubernetes.io/name=flock-addon
```

Check the hub-side configured image:

```bash
# [Hub]
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config -o yaml | rg -A1 'name: FLOCK_ALLIANCE_IMAGE\b'
```

If the image looks wrong or stale, redeploy with an explicit override and re-enable the addon:

```bash
# [Hub]
IMAGE_OWNER='<image-owner>' IMAGE_TAG='<git-sha-or-release-tag>' IMAGE_PULL_POLICY='Always' make <deploy-command> <mode-specific-args>
make disable-addon CLUSTER=<cluster-name>
make enable-addon CLUSTER=<cluster-name>
```

If Pod events show `unauthorized` or `denied`, the registry needs credentials. Follow the pull-secret flow in [Image Management](image-management.md).

## OCM Distribution Problems

If the hub-side addon deploy succeeds but nothing runs on the managed cluster, check the OCM distribution chain.

Hub-side checks:

```bash
# [Hub]
kubectl get clustermanagementaddon flock-addon
kubectl -n <cluster-name> get managedclusteraddon flock-addon -o yaml
kubectl -n <cluster-name> get manifestwork
```

Should see:

- `clustermanagementaddon/flock-addon` exists
- `managedclusteraddon/flock-addon` exists in the managed cluster namespace on the hub
- a `ManifestWork` exists for `flock-addon`

If `ManagedClusterAddOn` exists but no `ManifestWork` appears, re-enable the addon:

```bash
# [Hub]
make disable-addon CLUSTER=<cluster-name>
make enable-addon CLUSTER=<cluster-name>
```

Managed-cluster checks:

```bash
# [Managed Cluster context]
kubectl get ns flock-system
kubectl -n flock-system get deploy,pod
```

If the managed cluster itself is missing on the hub, go one layer earlier and verify OCM registration:

```bash
# [Hub]
kubectl get managedcluster
kubectl get csr -o custom-columns=NAME:.metadata.name,REQUESTER:.spec.username,STATUS:.status.conditions[*].type --no-headers
```

If `clusteradm accept` reports `no csr is approved yet`, approve the latest CSR for that cluster and accept it again:

```bash
# [Hub]
kubectl certificate approve <csr-name>
clusteradm accept --clusters <cluster-name> --context "${CTX_HUB}"
```

If `clusteradm join` failed previously on a managed cluster, clean up and retry:

```bash
# [Managed Cluster context]
kubectl delete ns open-cluster-management-agent --wait=true
kubectl delete ns open-cluster-management-agent-addon --wait=true 2>/dev/null || true
```

If hub-side OCM Pods are stuck in `Pending`, check whether the single-node hub still has a control-plane taint:

```bash
# [Hub]
kubectl get events -n open-cluster-management --sort-by=.lastTimestamp | tail -n 50
kubectl taint nodes --all node-role.kubernetes.io/control-plane:NoSchedule- || true
kubectl taint nodes --all node-role.kubernetes.io/master:NoSchedule- || true
kubectl get pods -n open-cluster-management
kubectl get pods -n open-cluster-management-hub
```

From a managed cluster, also verify the hub API is reachable:

```bash
# [Managed Cluster context]
curl -k https://<hub-apiserver>:6443/healthz
curl -k https://<hub-apiserver>:6443/version
```

## GPU Mapping Problems

If training is unexpectedly slow, verify the addon Pod is actually bound to GPU resources.

```bash
# [Managed Cluster context]
kubectl -n flock-system get pod -l app.kubernetes.io/name=flock-addon -o wide
kubectl -n flock-system describe pod -l app.kubernetes.io/name=flock-addon | rg -n "nvidia.com/gpu|Node:|Warning|FailedScheduling"
kubectl get node -o custom-columns=NAME:.metadata.name,GPU_ALLOCATABLE:.status.allocatable.nvidia\\.com/gpu
kubectl get ds -A | rg -i "nvidia|gpu|device-plugin"
kubectl -n flock-system logs deploy/flock-agent -c flock-alliance-client --tail=80 | rg -n "NVIDIA device files|nvidia-smi|No NVIDIA device files"
```

Then inspect device selection inside the client subprocess logs:

```bash
# [Managed Cluster context]
POD=$(kubectl -n flock-system get pod -l app.kubernetes.io/component=agent -o jsonpath='{.items[0].metadata.name}')
kubectl -n flock-system exec "$POD" -c flock-alliance-client -- sh -lc 'f=$(ls -1t /app/output/task_outputs/process_*.log | head -n1); echo "LOG=$f"; rg -n "CUDA is available|CUDA not available|Using device/backend|device=" "$f" || true'
```

If your cluster dedicates GPU nodes with labels or taints, deploy with node placement hints:

```bash
# [Hub]
helm upgrade --install flock-addon charts/flock-addon \
  --set agent.nodeSelector.gpu=true \
  --set 'agent.tolerations[0].key=nvidia.com/gpu' \
  --set 'agent.tolerations[0].operator=Exists' \
  --set 'agent.tolerations[0].effect=NoSchedule'
```

## Client and FLocKit Logs

When using OCM plus direct client mode, `FLocKit` is started as a subprocess inside the same `flock-alliance-client` container. There is no separate `FLocKit` Pod.

### 1. Find the running Pod

```bash
# [Managed Cluster context]
POD=$(kubectl -n flock-system get pod -l app.kubernetes.io/component=agent -o jsonpath='{.items[0].metadata.name}')
echo "$POD"
kubectl -n flock-system get pod "$POD"
```

### 2. Read `FLockAlliance` logs

```bash
# [Managed Cluster context]
kubectl -n flock-system logs "$POD" -c flock-alliance-client --tail=200
kubectl -n flock-system logs "$POD" -c flock-alliance-client --tail=200 | rg -n "Model process logs:|Step 3/5|timed out|Process crashed"
```

### 3. Read `FLocKit` subprocess logs

```bash
# [Managed Cluster context]
kubectl -n flock-system exec "$POD" -c flock-alliance-client -- sh -lc 'ls -lt /app/output/task_outputs | head -n 20'
kubectl -n flock-system exec "$POD" -c flock-alliance-client -- sh -lc 'f=$(ls -1t /app/output/task_outputs/process_*.log | head -n1); echo "LOG=$f"; tail -n 200 "$f"'
```

### 4. Follow subprocess logs live

```bash
# [Managed Cluster context]
kubectl -n flock-system exec "$POD" -c flock-alliance-client -- sh -lc 'f=$(ls -1t /app/output/task_outputs/process_*.log | head -n1); tail -f "$f"'
```

## Confirm Hub Values Reached the Client

The entrypoint logs the raw hub values before sourcing `.env`, the `.env` load itself, and the effective values that will be passed to `FLockAlliance`. To verify the authoritative pipeline is working:

```bash
# [Managed Cluster context]
POD=$(kubectl -n flock-system get pod -l app.kubernetes.io/component=agent -o jsonpath='{.items[0].metadata.name}')
kubectl -n flock-system logs "$POD" -c flock-alliance-client --tail=200 | rg -n "^\[flock-addon\]"
```

Should see the following lines, in order:

- `startup: image entrypoint beginning`
- `vars: USE_GPU=... STORAGE_BACKEND=... TASK_ADDRESS=...` (raw hub-pushed values)
- `loading env file: /data/.env` (or a warning if it is missing)
- `effective: STORAGE_BACKEND=... TASK_ADDRESS=...` (final values after OCM-wins reassertion; sensitive fields are rendered as `<set>` or `<empty>` so secrets never reach the log)
- `exec: python -u main.py ...`

If `effective: TASK_ADDRESS=<empty>`, either the hub is not pushing the value or a stale `.env` is setting it to an empty string. Check the hub-side `AddOnDeploymentConfig`:

```bash
# [Hub]
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config -o yaml | rg -A1 'name: (TASK_ADDRESS|BLOCKCHAIN_RPC|STORAGE_BACKEND)\b'
```

The startup wrapper also fails fast (exit code 2) with `ERROR: unsupported STORAGE_BACKEND=...` when the hub is misconfigured. That message is preferable to a Python stack trace later.

## Validate the Chart

```bash
# [Hub]
make verify
make test-chart
make status
```
