# Configuration and Overrides

This guide explains where `flock-addon` runs, how runtime values are resolved, and how to override defaults for specific clusters.

## What Runs Where

### Hub cluster

- Stores `ClusterManagementAddOn`, `AddOnTemplate`, and `AddOnDeploymentConfig`
- Deploys and updates shared addon settings
- Enables the addon on selected managed clusters (Manual) or auto-installs via `Placement` (Placements)

### Managed cluster

- Receives `ManifestWork` from the hub addon manager
- Runs the `flock-agent` Deployment in namespace `flock-system`

### Managed cluster node

- Provides the mounted host path, usually `/data/flock-client`
- Stores `.env` and any local datasets or files used by `FLockAlliance`

## Runtime Model

Each managed cluster gets one Pod with one container:

- Deployment: `flock-agent`
- Container: `flock-alliance-client`
- Container mount path: `/data`
- Default node path: `/data/flock-client`
- Default env file inside container: `/data/.env`
- Effective env file on node: `/data/flock-client/.env`

Path rules:

- do not use `~` in `hostPath`; always use an absolute path such as `/data/flock-client`
- the same host path must exist on every node that may schedule the Pod
- if your GPU nodes use taints or dedicated labels, set `agent.tolerations` and/or `agent.nodeSelector` via Helm

## Update Task Address

When a new onchain task is created, update only `TASK_ADDRESS`:

```bash
# [Hub]
make update-task TASK_ADDRESS='0x<NEW_TASK_ADDRESS>'
```

Check:

```bash
# [Hub]
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config -o yaml | rg -A1 'name: TASK_ADDRESS\b'
```

If the addon is already enabled, the managed cluster workload should reconcile automatically because the AddOnDeploymentConfig has changed. If you want to force a refresh immediately:

```bash
# [Hub]
make disable-addon CLUSTER=<cluster-name>
make enable-addon CLUSTER=<cluster-name>
```

## Parameter Flow

1. Helm values render shared `AddOnDeploymentConfig` objects (CPU + GPU variants).
2. `ManagedClusterAddOn` selects one `AddOnTemplate` and one `AddOnDeploymentConfig` per cluster.
3. OCM injects `customizedVariables` into the template placeholders (`{{FLOCK_ALLIANCE_IMAGE}}`, `{{TASK_ADDRESS}}`, ...).
4. The Pod receives those values as environment variables (`TASK_ADDRESS`, `USE_GPU`, `HOST_DATA_PATH`, ...).
5. The container entrypoint:
   - snapshots every hub-pushed value into an `OCM_*` shadow variable
   - sources `FLOCK_ALLIANCE_ENV_FILE` (default `/data/.env`) so per-node secrets become available
   - **re-exports every `OCM_*` value that is non-empty** so hub-pushed values always win, regardless of storage backend
   - validates `STORAGE_BACKEND` is one of `s3|local|nami` and warns when `TASK_ADDRESS` is empty
   - builds and `exec`'s `python -u main.py --config config/conf.yaml --override ...`

Effective authority rules:

- hub value non-empty → hub wins (for `TASK_ADDRESS`, `USE_GPU`, `STORAGE_BACKEND`, `NO_INCENTIVE`, `NUM_PARTICIPANTS`, `STAKE`, `BLOCKCHAIN_RPC`, `TOKEN_ADDRESS`, `LOCAL_STORAGE_DIR`, and all `S3_COMPAT_*`)
- hub value empty → the value from the node `.env` (if any) is used
- neither set → the FLockAlliance YAML default applies

This means:

- in testnet mode, the hub leaves `BLOCKCHAIN_RPC` empty so each node `.env` provides it; the hub stays authoritative for `TASK_ADDRESS`, `STORAGE_BACKEND`, and GPU selection
- in hub-managed local-chain modes, the hub pushes `BLOCKCHAIN_RPC`, `TOKEN_ADDRESS`, and - when running `nami` - every `S3_COMPAT_*` setting, and those values always win

## Per-Cluster Override

If one cluster needs different defaults, create a dedicated `AddOnDeploymentConfig` and reference it from that cluster's `ManagedClusterAddOn`.

> ⚠️ When you reference a per-cluster `AddOnDeploymentConfig` from a `ManagedClusterAddOn`, OCM uses **only** the variables it lists — it does NOT merge with the chart-managed `flock-addon-config`. That means a sparse per-cluster config silently drops anything the chart was forwarding (image, storage backend, GPU flag, num_participants, …). The safest pattern is to start by exporting the chart-managed default and editing only the fields you need to override, instead of writing the per-cluster config from scratch.

```bash
# [Hub] export the chart-managed default as a starting point
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config -o yaml \
  | yq 'del(.metadata.creationTimestamp, .metadata.resourceVersion, .metadata.uid, .metadata.generation, .status) | .metadata.name = "flock-addon-config-<cluster-name>"' \
  > /tmp/flock-addon-config-<cluster-name>.yaml

# Edit /tmp/flock-addon-config-<cluster-name>.yaml — change only the
# customizedVariables you actually need to override; keep every other
# entry verbatim so the rendered AddOnTemplate still receives them.
kubectl apply -f /tmp/flock-addon-config-<cluster-name>.yaml
```

If you must hand-author the per-cluster config (no `yq` available, or you are bootstrapping from a fresh hub), the example below covers every customized variable the chart's `AddOnTemplate` consumes. Empty strings let the node `.env` win for that field; non-empty values are hub-authoritative per the precedence rule described in the [Runtime Model](#runtime-model) section above.

> ⚠️ `AddOnTemplate.customizedVariables` is a flat key/value mechanism — it does **not** support `valueFrom: secretKeyRef`. Anything populated under `S3_COMPAT_ACCESS_KEY` / `S3_COMPAT_SECRET_KEY` here lands in the AddOnDeploymentConfig in plaintext, readable by any subject with `get/list addondeploymentconfigs` in this namespace. Leave them empty when each node's `.env` (mode `0600` on disk) is the authoritative source; populate them only when the hub-authoritative path (`make deploy-local-chain-s3-compatible`) owns the credential and the "Security: S3 Credential Exposure Model" section of [deployment-modes.md](deployment-modes.md) applies.

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: AddOnDeploymentConfig
metadata:
  name: flock-addon-config-<cluster-name>
  namespace: open-cluster-management
spec:
  agentInstallNamespace: flock-system
  customizedVariables:
    # ----- image (must be non-empty; rendered into the agent image field) -----
    - name: FLOCK_ALLIANCE_IMAGE
      value: ghcr.io/<image-owner>/fl-alliance-client:<git-sha-or-release-tag>
    # ----- node-local files -----
    - name: FLOCK_ALLIANCE_ENV_FILE
      value: /data/.env
    - name: DATA_PATH
      value: /data
    - name: HOST_DATA_PATH
      value: /data/flock-client
    # ----- training knobs (chart defaults forwarded so blank cells
    #       fall through to FLockAlliance's YAML defaults via the
    #       entrypoint's per-field guard, not to the empty string) -----
    - name: USE_GPU
      value: "false"
    - name: NUM_PARTICIPANTS
      value: ""
    - name: NO_INCENTIVE
      value: "false"
    - name: STAKE
      value: ""
    # ----- storage (must be one of s3|local|nami) -----
    - name: STORAGE_BACKEND
      value: s3
    # LOCAL_STORAGE_DIR is consumed only when STORAGE_BACKEND=local; the
    # entrypoint forwards it as `--override storage.local.shared_dir=...`.
    # Leave empty when STORAGE_BACKEND != local so FLockAlliance's YAML
    # default applies; set a host path when local storage is the chosen
    # backend on this cluster.
    - name: LOCAL_STORAGE_DIR
      value: ""
    # ----- chain (leave empty when this cluster's node `.env`
    #       provides them; set explicitly when the hub should win) -----
    - name: BLOCKCHAIN_RPC
      value: ""
    - name: TOKEN_ADDRESS
      value: ""
    - name: TASK_ADDRESS
      value: ""
    # ----- S3-compatible (only when STORAGE_BACKEND=nami; the
    #       SECURITY MODEL block in deployment-modes.md applies if
    #       you populate accessKey/secretKey here) -----
    - name: S3_COMPAT_ENDPOINT_URL
      value: ""
    - name: S3_COMPAT_BUCKET
      value: ""
    # S3_COMPAT_ACCESS_KEY / S3_COMPAT_SECRET_KEY are rendered into the
    # AddOnDeploymentConfig in plaintext (see the warning above this
    # block). Leave empty to let each node's `.env` provide them; set
    # them only when the hub-authoritative deploy path owns the
    # credential.
    - name: S3_COMPAT_ACCESS_KEY
      value: ""
    - name: S3_COMPAT_SECRET_KEY
      value: ""
    - name: S3_COMPAT_REGION
      value: us-east-1
    - name: S3_COMPAT_ADDRESSING_STYLE
      value: path
    - name: S3_COMPAT_VERIFY_SSL
      value: "false"
```

Example `ManagedClusterAddOn` reference. The install namespace is provided by the referenced `AddOnDeploymentConfig.spec.agentInstallNamespace` (see the `agentInstallNamespace: flock-system` line in the per-cluster ADC above) — no annotation is needed on the `ManagedClusterAddOn` itself. If you override `agent.namespace` in `values.yaml`, set the same value in `agentInstallNamespace` of any per-cluster ADC so the two stay in sync.

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: flock-addon
  namespace: <cluster-name>
spec:
  configs:
    - group: addon.open-cluster-management.io
      resource: addontemplates
      name: flock-addon
    - group: addon.open-cluster-management.io
      resource: addondeploymentconfigs
      name: flock-addon-config-<cluster-name>
      namespace: open-cluster-management
```

Because the addon entrypoint treats any non-empty hub value as authoritative, leaving a field empty in a per-cluster `AddOnDeploymentConfig` lets the node `.env` provide it. Set the field to an explicit value when you want the hub to win for that cluster.

## Direct CLI Mapping

Old direct run:

```bash
python main.py \
  --task-address 0x<task-address> \
  --dataset /path/to/dataset \
  --hf-token <hf-token> \
  --gpu
```

Addon mapping:

- `--task-address` → `deploymentConfig.blockchain.taskAddress` (hub) or `TASK_ADDRESS` in node `.env`
- `--dataset`      → `DATA_PATH` (hub, pointing at the in-container mount of `agent.dataVolume.*`)
- `--hf-token`     → `HF_TOKEN` in node `.env`
- `--gpu`          → GPU `AddOnTemplate` + `AddOnDeploymentConfig` selected automatically by `make enable-addon` when the managed cluster has label `gpu=true`

Effective priority inside the client, highest to lowest:

1. CLI `--override` flags built by the addon entrypoint
2. Environment variables (hub `customizedVariables` with the "hub-wins-when-non-empty" rule described above, plus node `.env`)
3. `config/conf.yaml` defaults shipped with FLockAlliance
