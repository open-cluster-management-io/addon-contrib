<p align="center">
  <img src="docs/assets/flock-logo-full.svg" alt="FLock" width="320" />
</p>

# FLock Addon for Decentralized Federated AI

Integrate [FLock FL Alliance](https://github.com/FLock-io/FL-Alliance-Client) (`FLockAlliance`) with Open Cluster Management (OCM) to automate decentralized federated learning across multi-cluster and multi-cloud environments, with blockchain-backed coordination and incentive mechanisms for participating nodes.

## Key Characteristics

- OCM deploys the decentralized `FLockAlliance` client to managed clusters as a direct participant runtime
- One-command, hub-managed deployment flows for testnet and local-chain development, including an optional hub-hosted S3-compatible object store
- Preserves the protocol's incentive-driven workflow, including on-chain task coordination and reward-oriented participation
- Runtime mode is fixed to `local`, so each cluster joins the protocol with its own node-local data and secrets
- `FLocKit` is not deployed as a separate addon workload; training runs inside the same `flock-alliance-client` container process
- Each enabled managed cluster runs one `flock-agent` Deployment in `flock-system`
- Runtime configuration, datasets, and model inputs are loaded from a mounted node directory, usually `/data/flock-client`

## Features

| Capability | Description |
| --- | --- |
| Decentralized FL | Runs `FLockAlliance`, a blockchain-backed federated learning client with on-chain task coordination and incentive-aware participation |
| Deployment | Uses OCM primitives such as `ClusterManagementAddOn`, `AddOnTemplate`, and `AddOnDeploymentConfig` for declarative multi-cluster rollout |
| Runtime Architecture | Keeps the addon simple with one direct `flock-alliance-client` workload per managed cluster and no separate `FLocKit` addon component |
| Placement | Automatically selects a CPU or GPU addon template from the managed cluster label `gpu=true` |
| Data Locality | Reads `.env`, datasets, and model inputs from node-mounted storage so each cluster trains on its own local resources |
| Configuration Authority | Hub-pushed values (task address, storage backend, hub-managed RPC/S3) always win over stale values in node `.env`, regardless of storage backend |
| Modes | Supports four deployment flows: bare install (node-managed `.env`), testnet (hub-pinned task), local chain + original S3, and local chain + local S3-compatible storage |

## Supported Deployment Modes

| Mode                              | Command | Best for |
|-----------------------------------| --- | --- |
| Bare install (node-managed)       | `make deploy` | Canonical OCM flow: install the chart with default values and let each managed cluster supply `TASK_ADDRESS` / `TOKEN_ADDRESS` / `BLOCKCHAIN_RPC` / `PRIVATE_KEY` from its own `.env`. Follow with `make enable-addon CLUSTER=<name>` per cluster. |
| Testnet (hub-pinned task)         | `make deploy-testnet TASK_ADDRESS=0x... [TOKEN_ADDRESS=0x...]` | Public testnet when the hub should centrally pin the task (and optionally token) contract addresses so `make update-task` can rotate them without touching every node's `.env`. |
| Local chain + original S3         | `make deploy-local-chain-s3` | Hub bootstraps its own local RPC + FlockToken/FlockTask, then deploys the addon. External S3 still backs model artefacts. |
| Local chain + local S3-compatible | `make deploy-local-chain-s3-compatible` | Fully self-contained: hub bootstraps local RPC, FlockToken/FlockTask, and a local MinIO with task-scoped random credentials and a `flock-task-<sha256[:12]>` bucket. Recommended for offline demos and unfunded smoke tests. Tear down with `make undeploy-local-chain`. |

Pick `make deploy` when individual nodes already own their task address and keys; pick one of the hub-managed variants when the hub should be the single source of truth for fleet-wide configuration. The entrypoint script's hub-vs-`.env` precedence rule guarantees that switching between the two styles never requires re-enabling the addon per cluster.

Full mode details are in [Deployment Modes](docs/deployment-modes.md).

## Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│                        Hub Cluster                          │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  ClusterManagementAddOn + AddOnTemplate (CPU / GPU)    │ │
│  │  AddOnDeploymentConfig  (CPU / GPU)                    │ │
│  │  [optional] local anvil chain + MinIO (self-contained) │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Managed Cluster                         │
│  namespace: flock-system                                    │
│  Deployment: flock-agent                                    │
│  Container:  flock-alliance-client                          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  Managed Cluster Node                       │
│  hostPath: /data/flock-client                               │
│  files:    .env, datasets, model inputs                     │
└─────────────────────────────────────────────────────────────┘
```

## Documentation

- [Prepare Multi-Cluster Environment](docs/prepare-multicluster-environment.md) - build Kubernetes clusters, install OCM, register managed clusters, and verify ManifestWork distribution
- [Install FLock Addon](docs/install-flock-addon.md) - first deployment path for the recommended default workflow
- [Deployment Modes](docs/deployment-modes.md) - compare and run the four supported deployment modes
- [Image Management](docs/image-management.md) - choose public/private images and publish custom builds
- [Configuration and Overrides](docs/configuration-and-overrides.md) - runtime model, path rules, task updates, and per-cluster overrides
- [Troubleshooting](docs/troubleshooting.md) - image pull, OCM distribution, GPU mapping, and log collection
- [Tests](tests/README.md) - chart unit tests and entrypoint shell tests that gate every change

## Related Projects

- [FLock FL Alliance Client](https://github.com/FLock-io/FL-Alliance-Client) - the direct client runtime deployed by this addon
- [FLock FLocKit](https://github.com/FLock-io/FLocKit) - the federated learning training logic template
- [Open Cluster Management](https://open-cluster-management.io) - multi-cluster management for Kubernetes
