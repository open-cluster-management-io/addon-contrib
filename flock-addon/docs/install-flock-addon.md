# Install FLock Addon

This guide walks through the recommended first deployment for `flock-addon`: `Local chain + local S3-compatible`, with one `flock-agent` running on each enabled managed cluster.

Before using this guide, complete [Prepare Multi-Cluster Environment](prepare-multicluster-environment.md). In particular, make sure:

- the hub and managed clusters are separate Kubernetes clusters
- OCM registration is complete and `ManagedCluster` objects are `Joined=True` and `Available=True`
- a simple `ManifestWork` can already reach the managed clusters
- single-node clusters have their control-plane taints removed if they need to run workloads on the only node

## What Gets Deployed

- Hub cluster:
  - `ClusterManagementAddOn`
  - `AddOnTemplate`
  - `AddOnDeploymentConfig`
- Managed cluster:
  - namespace `flock-system`
  - Deployment `flock-agent`
  - container `flock-alliance-client`
- Managed cluster node:
  - mounted host path, usually `/data/flock-client`
  - `.env` and local data files used by `FLockAlliance`

## Prerequisites

- OCM hub and managed clusters are already available and healthy
- `kubectl`, `helm`, and `make` are installed on the hub
- Every node that may run the addon Pod has a shared host path, usually `/data/flock-client`
- (Optional) `ripgrep` is installed on the hub for the verification commands shown below; substitute `grep` if you prefer. On Ubuntu/Debian: `sudo apt install -y ripgrep`
- This repository is checked out on the hub machine. All snippets below assume the working directory is the repo root (`/path/to/addon-contrib`). Anchor your shell once at the start of the runbook so the later `cd flock-addon` steps land in the right place:

```bash
# Run once. The repo root anchor is required because every later
# `cd flock-addon` snippet is written as a relative path FROM the
# repo root, not from wherever the previous snippet left the shell.
# Re-run this anchor (or `cd ..` back from `flock-addon`) before
# each deploy snippet if you mix in unrelated commands between them.
cd /path/to/addon-contrib
```

For the recommended default path, also prepare:

- a checkout of `FL-Alliance-Client` on the hub machine
- a local model archive such as `/absolute/path/to/model.tar.gz`
- a hub IP or hostname reachable from managed clusters for `RPC_HOST`

If you need a different deployment path, use [Deployment Modes](deployment-modes.md). If you need a custom or private image, use [Image Management](image-management.md) before enabling the addon.

If you only want the canonical OCM flow (chart on the hub with defaults, each managed cluster supplies its own `.env`), use `make deploy` instead — see [Deployment Modes → Bare Install Mode](deployment-modes.md#bare-install-mode).

## Step 1: Prepare the Node Path

Run on every managed cluster node that may host the addon Pod.

```bash
# [Each Managed Cluster Node]
sudo mkdir -p /data/flock-client
sudo chmod 755 /data
sudo chown -R <login-user>:<login-group> /data/flock-client
sudo chmod -R u+rwX /data/flock-client
```

Check:

```bash
# [Each Managed Cluster Node]
ls -ld /data /data/flock-client
```

Should see:

- `/data` exists
- `/data/flock-client` exists
- your login user can read and write `/data/flock-client`

If your workflow depends on node-local input files (per-client dataset shards, demo fixtures, model inputs), copy them into `/data/flock-client` now. This directory is mounted into the container at `/data`.

Concrete example for a sharded federated dataset where each managed cluster gets a different client slice:

```bash
# [Each Managed Cluster Node]
# Replace <path-to-client-shard> with the absolute path on this node that
# holds the FL client slice you want this cluster to train on
# (e.g. a per-client folder unpacked from a demo dataset archive).
cp <path-to-client-shard>/* /data/flock-client/
ls -la /data/flock-client/
```


## Step 2: Create the Node `.env`

Create this file on every managed cluster node:

```text
/data/flock-client/.env
```

Recommended `.env` for `Local chain + local S3-compatible`:

```dotenv
PRIVATE_KEY=<private-key>
HF_TOKEN=<hf-token>
```

Ignore any secrets shown in historical testing notes. The important part is the variable layout, not the sample values.

Check (deliberately printing only the variable names, never their values, so this snippet is safe to paste into shared logs):

```bash
# [Each Managed Cluster Node]
ls -l /data/flock-client/.env
# Confirm the expected variable names are present without revealing secrets:
grep -E '^[A-Z_][A-Z0-9_]*=' /data/flock-client/.env | awk -F= '{print $1"=<redacted>"}'
```

Should see:

- `.env` exists at `/data/flock-client/.env`
- `PRIVATE_KEY` and `HF_TOKEN` are present (printed as `<redacted>`)

In the recommended default mode, blockchain RPC, task address, token address, and S3-compatible storage settings are pushed from the hub and override any stale values that may still be present in the node `.env`. Node `.env` only needs node-local secrets.

If you are using one of the other supported modes instead, use the matching `.env` shape:

`Testnet`

```dotenv
PRIVATE_KEY=<private-key>
HF_TOKEN=<hf-token>
BLOCKCHAIN_RPC=<testnet-rpc-url>
TOKEN_ADDRESS=0x<token-address>
```

Use this when the managed cluster should connect to an existing external blockchain endpoint and external S3-oriented workflow.

`Local chain + original S3`

```dotenv
PRIVATE_KEY=<private-key>
HF_TOKEN=<hf-token>
```

Use this when the hub will host the local chain, but model storage still depends on an already uploaded external S3 artifact.

## Step 3: Deploy the Addon Definition on the Hub

Deploy the shared addon definition from the hub using the recommended self-contained mode:

```bash
# [Hub] (run from repo root — see the prerequisite `cd /path/to/addon-contrib` above)
cd flock-addon
make deploy-local-chain-s3-compatible \
  FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client \
  MODEL_ARCHIVE=/absolute/path/to/model.tar.gz \
  RPC_HOST=<hub-ip> \
  DOCKER='sudo docker' \
  S3_COMPAT_DATA_DIR='<local-minio-data-dir>'
```

If your user can already access Docker without `sudo`, you can omit `DOCKER='sudo docker'`.

If you prefer the default MinIO data directory, create it first:

```bash
sudo mkdir -p /srv/flock-minio/data
sudo chown -R "$USER":"$(id -gn)" /srv/flock-minio
```

Optional image overrides:

```bash
# [Hub]
export IMAGE_REGISTRY='ghcr.io'
export IMAGE_OWNER='<image-owner>'
export IMAGE_NAME='fl-alliance-client'
export IMAGE_TAG='<git-sha-or-release-tag>'
export IMAGE_PULL_POLICY='Always'
export FLOCK_ALLIANCE_IMAGE="${IMAGE_REGISTRY}/${IMAGE_OWNER}/${IMAGE_NAME}:${IMAGE_TAG}"
```

If the selected image is private, create the managed-cluster pull secret first and set `IMAGE_PULL_SECRET`. The full flow is in [Image Management](image-management.md).

If the addon definition exists on the hub but workloads never reach managed clusters, stop here and validate the OCM pipeline with [Prepare Multi-Cluster Environment](prepare-multicluster-environment.md).

Check:

```bash
# [Hub]
kubectl get clustermanagementaddon flock-addon
kubectl get addontemplate flock-addon flock-addon-gpu
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config flock-addon-gpu-config -o yaml | rg -A1 'name: (TASK_ADDRESS|BLOCKCHAIN_RPC|S3_COMPAT_ENDPOINT_URL|FLOCK_ALLIANCE_IMAGE)\b'
```

Should see:

- `clustermanagementaddon/flock-addon` exists (cluster-scoped)
- `addontemplate/flock-addon` and `addontemplate/flock-addon-gpu` exist (cluster-scoped)
- `addondeploymentconfig/flock-addon-config` exists in `open-cluster-management`
- `addondeploymentconfig/flock-addon-gpu-config` exists in `open-cluster-management`
- `TASK_ADDRESS` matches the hub-generated value
- `BLOCKCHAIN_RPC` points to the hub-hosted local chain
- `S3_COMPAT_ENDPOINT_URL` points to the hub-hosted local S3-compatible service

## Alternative Step 3 Paths

If you are not using the recommended default mode, replace Step 3 with one of the following hub-side deploy commands.

### Option A: Testnet

Use this only when you already have:

- an existing on-chain task on testnet
- a managed-cluster `.env` that includes `BLOCKCHAIN_RPC`
- the external S3-oriented workflow already in place

```bash
# [Hub] (run from repo root — re-anchor with `cd /path/to/addon-contrib` if needed)
cd flock-addon
make deploy-testnet TASK_ADDRESS='0x<task-address>'
```

Check:

```bash
# [Hub]
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config -o yaml | rg -A1 'name: (TASK_ADDRESS|BLOCKCHAIN_RPC|STORAGE_BACKEND)\b'
```

Should see:

- `TASK_ADDRESS` matches the value you passed
- `STORAGE_BACKEND` is `s3`
- `BLOCKCHAIN_RPC` is not forced from the hub for this mode

### Option B: Local Chain + Original S3

Use this when you want the hub to host the local chain, but your model artifact already exists in external S3 and you have its hash.

```bash
# [Hub] (run from repo root — re-anchor with `cd /path/to/addon-contrib` if needed)
cd flock-addon
make deploy-local-chain-s3 \
  FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client \
  MODEL_HASH=<sha256> \
  RPC_HOST=<hub-ip> \
  DOCKER='sudo docker'
```

Check:

```bash
# [Hub]
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config -o yaml | rg -A1 'name: (TASK_ADDRESS|BLOCKCHAIN_RPC|STORAGE_BACKEND)\b'
```

Should see:

- `STORAGE_BACKEND` is `s3`
- `BLOCKCHAIN_RPC` points to the hub-hosted local chain
- `TASK_ADDRESS` matches the hub-generated local-chain task

After any of the Step 3 variants above, continue with Step 4 and Step 5 unchanged.

## Step 4: Enable the Addon on a Managed Cluster

GPU/CPU template selection follows the hub-side `managedcluster` label `gpu=true`.

```bash
# [Hub]
make enable-addon CLUSTER=<cluster-name>
```

Check:

```bash
# [Hub]
kubectl -n <cluster-name> get managedclusteraddon flock-addon -o yaml
kubectl -n <cluster-name> get manifestwork
```

Should see:

- `managedclusteraddon/flock-addon` exists
- `spec.configs` selects the GPU template/config on `gpu=true` clusters
- `spec.configs` selects the CPU template/config on other clusters
- a `ManifestWork` appears

To enable multiple clusters, repeat the same command:

```bash
# [Hub]
make enable-addon CLUSTER=<cluster-a>
make enable-addon CLUSTER=<cluster-b>
make enable-addon CLUSTER=<cluster-c>
```

## Step 5: Verify Runtime on the Managed Cluster

```bash
# [Managed Cluster context]
kubectl -n flock-system get deploy,pod
kubectl -n flock-system logs deploy/flock-agent -c flock-alliance-client --tail=100
kubectl -n flock-system get pod -l app.kubernetes.io/name=flock-addon -o jsonpath='{range .items[*]}{.metadata.name}{"\trequest="}{.spec.containers[0].resources.requests.nvidia\.com/gpu}{"\tlimit="}{.spec.containers[0].resources.limits.nvidia\.com/gpu}{"\n"}{end}'
kubectl get node -o custom-columns=NAME:.metadata.name,GPU_ALLOCATABLE:.status.allocatable.nvidia\\.com/gpu
```

Should see:

- `deployment/flock-agent` exists
- the Pod becomes `Running`
- logs show `FLockAlliance` startup
- logs include the local chain and S3-compatible runtime path instead of missing RPC or storage errors
- on `gpu=true` clusters, Pod resources show `request=1` and `limit=1` for `nvidia.com/gpu`
- on CPU clusters, the GPU request fields are empty and the Pod still runs

## Sizing for LLM Workloads

The chart ships conservative defaults (`requests: 200m / 512Mi`, `limits: 2 cores / 2Gi`) so the Pod schedules on `kind`/`k3d` smoke clusters. Real LLM training (7B+ LoRA, MLX fine-tune, anything with a tokenizer warm-up bigger than a few hundred MB) blows past 2Gi instantly and the kubelet OOM-kills the container with exit code 137:

```
NAME                               READY   STATUS      RESTARTS      AGE
pod/flock-agent-6879cf7f75-dhxf9   0/1     OOMKilled   3 (85s ago)   4m28s
```

Override per deploy with the `MEMORY_*` / `CPU_*` Make vars — they propagate to every `make deploy*` recipe and OCM auto-reconciles the running Pods:

```bash
# Example: 7B LoRA on GPU
make deploy-local-chain-s3-compatible \
  FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client \
  MODEL_ARCHIVE=/path/to/model.tar.gz \
  RPC_HOST=<hub-ip> \
  DOCKER='sudo docker' \
  MEMORY_REQUEST=8Gi MEMORY_LIMIT=24Gi \
  CPU_REQUEST=2     CPU_LIMIT=4
```

Or, for a release that's already deployed, patch resources in place without touching anything else:

```bash
helm upgrade flock-addon charts/flock-addon \
  --reuse-values \
  --set agent.resources.flockAlliance.requests.memory=8Gi \
  --set agent.resources.flockAlliance.limits.memory=24Gi
```

Sizing reference (host RAM only — VRAM is governed by the GPU template variant):

| Workload | `MEMORY_REQUEST` | `MEMORY_LIMIT` | `CPU_REQUEST` | `CPU_LIMIT` |
|----------|------------------|----------------|---------------|-------------|
| 7B LoRA fine-tune on GPU  | `8Gi`  | `24Gi` | `2` | `4`  |
| 13B LoRA fine-tune on GPU | `16Gi` | `48Gi` | `2` | `4`  |
| 7B MLX fine-tune on CPU   | `24Gi` | `64Gi` | `4` | `16` |
| 7B FP16 inference on GPU  | `4Gi`  | `16Gi` | `1` | `2`  |
| 7B INT4 inference on CPU  | `6Gi`  | `12Gi` | `2` | `8`  |

To dial in an unfamiliar model: set `MEMORY_LIMIT` to a deliberately oversized value (e.g. `128Gi`) for one run, then read the peak with `kubectl -n flock-system top pod` (or `cat /sys/fs/cgroup/.../memory.peak` on the node) and use `peak * 1.4` as the production limit, `peak * 0.7` as the request.

## Cleanup

```bash
# [Hub]
# Disable the addon on each cluster first so OCM garbage-collects the managed-cluster workload.
make disable-addon CLUSTER=<cluster-name>

# Remove the Helm release from the hub.
make undeploy

# Optional: if you also started the hub-hosted local MinIO via
# `deploy-local-chain-s3-compatible`, stop it with one of:
make stop-local-s3-compatible   # removes only the MinIO container
make undeploy-all               # helm uninstall + stop-local-s3-compatible

# Recommended for the local-chain flow: this drops the per-task bucket,
# uninstalls the Helm release, stops MinIO, and optionally stops the chain.
make undeploy-local-chain FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client
```

The local MinIO data directory (`S3_COMPAT_DATA_DIR`, default `/srv/flock-minio/data`) is intentionally left in place so you can reuse uploaded artifacts across re-deploys. Per-task buckets inside it are dropped by `make undeploy-local-chain`; delete the host directory manually if you also want to free the disk.

## Next Steps

- Use [Deployment Modes](deployment-modes.md) for testnet or external-S3 workflows
- Use [Image Management](image-management.md) for public/private registry setups and custom image publishing
- Use [Configuration and Overrides](configuration-and-overrides.md) for task updates, path rules, and per-cluster customization
- Use [Troubleshooting](troubleshooting.md) if the rollout reaches the hub but not the managed cluster
