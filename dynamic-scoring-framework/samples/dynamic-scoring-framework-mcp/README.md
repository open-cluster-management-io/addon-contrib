# Dynamic Scoring Framework MCP Server

An Operation API server for the Dynamic Scoring Framework that provides REST endpoints and an MCP (Model Context Protocol) interface for querying AddOnPlacementScores, running workload placement optimization, and managing generated resources.  
It uses [PuLP](https://coin-or.github.io/pulp/) for linear programming-based optimization and [FastAPI-MCP](https://fastapi-mcp.tadata.com/getting-started/welcome) to expose endpoints as MCP tools, enabling AI-assisted optimization through VS Code Copilot.

> This component is used in the [Optimization Using DSF](../../docs/optimization-using-dsf.md) workflow.

## Directory Structure

```
dynamic-scoring-framework-mcp/
├── Dockerfile              # Container image definition
├── README.md               # This file
├── main.py                 # FastAPI application with MCP integration
├── opt.py                  # Standalone optimization script (for local testing)
├── pyproject.toml          # Python project metadata and dependencies
├── deployment.yaml         # Kubernetes Deployment/RBAC/Service manifest
├── examples/
│   ├── params.json         # Sample optimization parameters
│   ├── params.yaml         # Sample optimization parameters (YAML format)
│   ├── output.json         # Sample optimization output
│   └── output.yaml         # Sample optimization output (YAML format)
├── manifests/
│   ├── clustersetbindings.yaml             # ManagedClusterSetBinding
│   ├── mwrs-app01.yaml                     # Sample ManifestWorkReplicaSet (app01)
│   ├── mwrs-app02.yaml                     # Sample ManifestWorkReplicaSet (app02)
│   ├── policy-disable-mig-cluster1.yaml    # Sample Policy: disable MIG on cluster1
│   ├── policy-disable-mig-cluster2.yaml    # Sample Policy: disable MIG on cluster2
│   ├── policy-enable-mig-2g-cluster1.yaml  # Sample Policy: enable MIG 2g on cluster1
│   ├── policy-enable-mig-2g-cluster2.yaml  # Sample Policy: enable MIG 2g on cluster2
│   ├── policy-enable-mig-3g-cluster1.yaml  # Sample Policy: enable MIG 3g on cluster1
│   └── policy-enable-mig-3g-cluster2.yaml  # Sample Policy: enable MIG 3g on cluster2
└── templates/
    ├── placement-app.yaml        # Jinja2 template for app Placement
    ├── placement-policy.yaml     # Jinja2 template for policy Placement
    └── placementbinding.yaml     # Jinja2 template for PlacementBinding
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/addonplacementscores` | Retrieve AddOnPlacementScores from the hub cluster |
| `POST` | `/optimize` | Run workload placement optimization and generate Placement/PlacementBinding resources |
| `POST` | `/reset` | Delete all generated Placement/PlacementBinding resources |
| `GET` | `/mcp` | MCP (Model Context Protocol) endpoint for AI integration |

## Quick Start (podman)

### Build

```bash
cd samples/dynamic-scoring-framework-mcp
podman build -t dynamic-scoring-framework-mcp .
```

### Run (local development with kubeconfig)

```bash
podman run -d -p 8338:8338 \
  --name dynamic-scoring-framework-mcp \
  -v $HOME/.kube/config:/root/.kube/config:ro \
  --replace \
  dynamic-scoring-framework-mcp
```

### Test

Get AddOnPlacementScores:

```bash
curl -sS http://localhost:8338/addonplacementscores | jq
```

Run optimization:

```bash
curl -sS -X POST http://localhost:8338/optimize \
  -H "Content-Type: application/json" \
  -d @examples/params.json | jq
```

Reset generated resources:

```bash
curl -sS -X POST http://localhost:8338/reset \
  -H "Content-Type: application/json" \
  -d '{"namespace": "default"}' | jq
```

## Deploy to Hub Cluster

```bash
podman build -t quay.io/dynamic-scoring/dynamic-scoring-framework-mcp:latest .
kind load docker-image quay.io/dynamic-scoring/dynamic-scoring-framework-mcp:latest --name hub
kubectl apply -f deployment.yaml --context kind-hub
```

Access via port-forward:

```bash
kubectl port-forward -n dynamic-scoring \
  pod/$(kubectl get pods -n dynamic-scoring -l app=dynamic-scoring-framework-mcp --context kind-hub -o name | head -1 | cut -d/ -f2) \
  8338:8338 --context kind-hub
```

## Optimization Flow

1. The `/optimize` endpoint receives parameters defining clusters, available policies, target workloads, and scoring preferences.
2. It retrieves AddOnPlacementScores and policy/workload metadata from the hub cluster.
3. A linear programming model (PuLP) is solved to maximize the total score while satisfying:
   - GPU capacity constraints (each cluster's policy determines available GPU resources).
   - Exactly one policy per cluster.
   - Workload demand constraints (GPU resource requests from MWRS labels).
4. The solution generates Placement and PlacementBinding resources (from Jinja2 templates) and applies them to the hub cluster.

## MCP Integration (VS Code Copilot)

Configure VS Code to use the MCP server by adding to `.vscode/mcp.json`:

```json
{
  "servers": {
    "dynamic-scoring-framework-mcp": {
      "url": "http://localhost:8338/mcp",
      "type": "http"
    }
  }
}
```

Example prompts:
- "Get AddOnPlacementScores and summarize them"
- "Optimize for performance"
- "Optimize for power consumption"

## RBAC

The `deployment.yaml` includes the required RBAC resources:

- **ClusterRole** — `get`, `list`, `watch` on `addonplacementscores`, `dynamicscorers`, `policies`, `manifestworkreplicasets`; `get`, `list`, `watch`, `create`, `update`, `delete` on `placements` and `placementbindings`.
- **ServiceAccount** — `dynamic-scoring-framework-mcp-sa` in `dynamic-scoring` namespace.
- **ClusterRoleBinding** — Binds the above.