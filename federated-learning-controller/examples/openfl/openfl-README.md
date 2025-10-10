### OpenFL Example App – Containerization and Usage Guide

### Overview
This document describes the default OpenFL workspace layout, the modifications made for this example, how to build the container image, how OpenFL is containerized here, and how to run aggregator and collaborator containers using the custom entrypoint.

### Default Workspace Layout (OpenFL-generated baseline)
A typical OpenFL workspace contains:
- `plan/`: federation configuration (e.g., `plan.yaml`) and auxiliary files.
- `save/`: model checkpoints and artifacts (e.g., best/last state).
- `cert/`: TLS/certificates if used.
- `data/`: local data mount point for collaborators.
- `logs/`: runtime logs.
- `local_state/`: local runtime/cache state.
- `src/`: model and task runner code.
- `requirements.txt`: Python dependencies for the workspace.

In this example, the app directory is `openfl-app/` and includes all the above.

### What We Changed in This Example
- Added a workspace-specific Dockerfile (`Dockerfile.workspace`) to build a runnable image from `ghcr.io/securefederatedai/openfl:latest`, copy the workspace assets, and set the entrypoint.
- Introduced an explicit Python entrypoint (`entrypoint.py`) that:
  - Updates federation settings in `plan/plan.yaml` at container start based on CLI flags (e.g., aggregator IP/port, training rounds, model directory).
  - Writes collaborator names into `plan/cols.yaml` and appends collaborator-to-data mapping into `plan/data.yaml`.
  - Starts the desired role via `fx` (aggregator or collaborator) after configuration is updated.
- Included a simple Keras CNN task runner in `src/taskrunner.py` as the demo model.
- Added `requirements.txt` to pin training dependencies (Keras/TensorFlow) used by the example.
- Optionally builds Gramine enclaves during the image build (requires a signing key provided as a Docker secret).

### How the Image Is Built
You can build via the provided `Makefile` targets or directly with `docker build`.

- Single-arch local build (Makefile):
```bash
cd federated-learning-controller/examples/openfl
make build-app-image REGISTRY=quay.io/open-cluster-management IMAGE_TAG=latest
```
This:
- Generates `signer-key.pem` for enclave signing.
- Builds from `openfl-app/Dockerfile.workspace`.
- Passes the signing key as a Docker secret (`--secret id=signer-key,src=../signer-key.pem`).

- Push built image:
```bash
make push-app-image REGISTRY=quay.io/open-cluster-management IMAGE_TAG=latest
```

Notes:
- The Dockerfile copies `plan/`, `src/`, `save/`, `cert/`, and installs `requirements.txt`.
- It runs enclave build steps using the secret signer key and then drops privileges to `user`.
- Environment variables `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` are set in the Dockerfile; override them at runtime if needed.

### How OpenFL Is Containerized Here
- Base image: `ghcr.io/securefederatedai/openfl:latest` (already includes OpenFL CLI `fx` and runtime).
- Workspace content is copied under `/workspace/` inside the image.
- A Python entrypoint (`/workspace/entrypoint.py`) becomes the container entrypoint to:
  - Adjust `plan.yaml`, `data.yaml`, and `cols.yaml` dynamically from CLI flags.
  - Exec into the correct OpenFL role via `fx` (aggregator or collaborator).
- Optional enclave build: If the signing key secret is provided at build time, Gramine enclaves are generated for runtime attestation use cases.

### How to Run Containers

#### Aggregator (Server)
Minimum example:
```bash
docker run --rm -p 8080:8080 \
  quay.io/open-cluster-management/openfl-app:latest \
  server --server-ip 172.17.0.1 --server-port 8080 --num-rounds 3 --cols client1,client2 --model-dir save
```

- Key flags:
  - `--server-ip`: aggregator bind address shared to collaborators (if omitted, only port is set).
  - `--server-port`: TCP port for the aggregator.
  - `--num-rounds`: number of training rounds.
  - `--cols`: comma-separated collaborator names; persisted to `plan/cols.yaml`.
  - `--model-dir`: directory for `best_state_path` and `last_state_path`.

What happens:
- `entrypoint.py` updates `plan/plan.yaml` (`use_tls` disabled by default here) and writes `plan/cols.yaml`.
- Process `exec`s into `fx aggregator start`.

#### Collaborator (Client)
Start one collaborator container per client. Use logical data shards and pass a shard identifier via `--data-path` (no volume mounts required).
```bash
# client1
docker run --rm \
  quay.io/open-cluster-management/openfl-app:latest \
  client --name client1 --data-path 1 \
         --server-ip 172.17.0.1 --server-port 8080 --num-rounds 3 --model-dir save

# client2
docker run --rm \
  quay.io/open-cluster-management/openfl-app:latest \
  client --name client2 --data-path 2 \
         --server-ip 172.17.0.1 --server-port 8080 --num-rounds 3 --model-dir save
```

- Key flags:
  - `--name`: collaborator name (must match one in `--cols` used by the aggregator).
  - `--data-path`: logical shard ID understood by the OpenFL data loader.
  - `--server-ip`, `--server-port`: address/port of the running aggregator.
  - `--num-rounds`, `--model-dir`: optional overrides aligned with the aggregator configuration.

What happens:
- `entrypoint.py` appends a row to `plan/data.yaml` with `name,data_path` (where `data_path` is the shard ID).
- `entrypoint.py` updates network and training fields in `plan/plan.yaml` and `exec`s into `fx collaborator start -n <name>`.

### Entrypoint Design and Behavior
- Location: `/workspace/entrypoint.py`
- Subcommands:
  - `server`: updates aggregator config and collaborator list; starts `fx aggregator start`.
  - `client`: appends dataset mapping and updates network/training config; starts `fx collaborator start -n <name>`.
- Plan updates:
  - `network.settings.use_tls = False` (TLS disabled by default here).
  - `network.settings.agg_addr`, `agg_port` set from flags if provided.
  - `aggregator.settings.rounds_to_train` set from `--num-rounds` if provided.
  - `aggregator.settings.best_state_path` and `last_state_path` optionally redirected under `--model-dir`.
- Files touched:
  - `plan/plan.yaml` (read/write)
  - `plan/cols.yaml` (server mode; collaborator names)
  - `plan/data.yaml` (client mode; `<name>,<data_path>` lines)
- Execution model:
  - Uses `os.execvp` to replace the Python process with the `fx` process, making `fx` PID 1 in the container.



### Deploy an OpenFL Federated Learning Instance

This example targets OCM-based multi-cluster deployment. At a high level:

1) Build and push image
- Build and push the example image to a registry reachable by all managed clusters (see build steps above).

2) Set data shard ClusterClaims on managed clusters
- Each collaborator cluster should declare a logical shard ID via a `ClusterClaim` named `federated-learning-sample.client-data`.
- Samples are provided:
  - `examples/openfl/samples/cluster-claim-cluster1-openfl.yaml` → shard "1"
  - `examples/openfl/samples/cluster-claim-cluster2-openfl.yaml` → shard "2"
- Apply on each respective managed cluster context:
```bash
kubectl --context <cluster1> apply -f examples/openfl/samples/cluster-claim-cluster1-openfl.yaml
kubectl --context <cluster2> apply -f examples/openfl/samples/cluster-claim-cluster2-openfl.yaml
```

3) Apply your FederatedLearning manifest (ofl.yaml) on the hub
- Create an OpenFL job using the controller’s `FederatedLearning` CR (similar to the Flower sample in the repository root README).
- In your `ofl.yaml`:
  - Set the aggregator to use the OpenFL image and pass server flags like `--server-ip`, `--server-port`, `--num-rounds`, `--cols`, `--model-dir`.
  - Configure collaborators to use the same image and pass client flags `--name` and `--data-path` where `--data-path` corresponds to the shard ID (from the `ClusterClaim`).
- Then apply on the hub:
```bash
kubectl --context <hub> apply -f ofl.yaml
```

Notes
- Ensure your hub/managed clusters can pull the image.
- The shard ID provided via `ClusterClaim` should match the `--data-path` each collaborator uses.
- Refer to the root README’s Flower example for general OCM workflows; this OpenFL sample follows the same pattern with different entrypoint args.

Inline example manifest (OpenFL):

```yaml
apiVersion: federation-ai.open-cluster-management.io/v1alpha1
kind: FederatedLearning
metadata:
  name: federated-learning-sample
spec:
  framework: openfl
  server:
    image: <your-registry>/openfl-app:latest
    rounds: 3
    minAvailableClients: 2
    listeners:
      - name: server-listener
        port: 31531
        type: NodePort
    storage:
      type: PersistentVolumeClaim
      name: model-pvc
      # If you are using the OpenFL framework, make sure to mount the model directory under /workspace
      path: /workspace/models
      size: 2Gi
  client:
    image: <your-registry>/openfl-app:latest
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

Replace `<your-registry>` with your actual image registry/paths. Ensure the server and clients use the same OpenFL image. The data shard comes from the `ClusterClaim` value and is passed to clients as `--data-path` automatically by the controller. Apply this manifest on the hub with your preferred workflow (e.g., `kubectl apply -f -`).

