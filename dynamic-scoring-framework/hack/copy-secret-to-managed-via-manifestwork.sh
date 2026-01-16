#!/usr/bin/env bash
set -euo pipefail

# Copy a Secret from each managed cluster namespace on the hub into a target namespace
# on the managed cluster using ManifestWork.
#
# Requirements:
# - Run against the HUB cluster (where ManagedCluster and ManifestWork LIVE).
# - You need permission to:
#   - list managedclusters
#   - get secrets in each managed cluster namespace
#   - create/update manifestworks in each managed cluster namespace
#
# This script is intentionally conservative:
# - Default is --dry-run (prints generated ManifestWork YAML only)
# - Use --apply to actually create/update ManifestWorks
#
# Notes:
# - ManifestWork namespace MUST be the managed cluster name (ManagedCluster name).
# - The Secret manifest embedded in spec.workload.manifests can target any namespace
#   on the managed cluster (dst-namespace).

usage() {
  cat <<'EOF'
Usage:
  hack/copy-secret-to-managed-via-manifestwork.sh \
    --secret-name <name> \
    [--dst-secret-name <name>] \
    --src-namespace-template <template> \
    --dst-namespace <namespace> \
    [--manifestwork-name <name>] \
    [--kubeconfig <path>] \
    [--context <ctx>] \
    [--apply] \
    [--label-selector <selector>] \
    [--clusters <c1,c2,...>]

Examples:
  # Dry-run (default): show YAML for all managed clusters
  hack/copy-secret-to-managed-via-manifestwork.sh \
    --secret-name token \
    --src-namespace-template '{{CLUSTER}}' \
    --dst-namespace dynamic-scoring

  # Apply to only specific clusters
  hack/copy-secret-to-managed-via-manifestwork.sh \
    --secret-name token \
    --src-namespace-template '{{CLUSTER}}' \
    --dst-namespace dynamic-scoring \
    --clusters cluster1,cluster2 \
    --apply

Options:
  --secret-name                Source Secret name on hub side per cluster namespace.
  --dst-secret-name            Secret name to create on managed clusters (default: same as --secret-name).
  --src-namespace-template     Template to build source namespace on hub.
                               Use '{{CLUSTER}}' as placeholder for managed cluster name.
                               Common choice: '{{CLUSTER}}' (i.e. namespace == cluster name).
  --dst-namespace              Namespace on managed cluster where the Secret will be created.
  --manifestwork-name          ManifestWork name to create/update. Default: copy-secret-<secret-name>
  --label-selector             Label selector for managedclusters (filters list).
  --clusters                   Comma-separated managed cluster names (overrides list).
  --kubeconfig                 kubeconfig path for oc.
  --context                    kube context for oc.
  --apply                      Actually create/update ManifestWork (default: dry-run).
  -h, --help                   Show help.
EOF
}

command -v jq >/dev/null 2>&1 || {
  echo "ERROR: 'jq' is required but not installed (or not in PATH)." >&2
  exit 127
}

SECRET_NAME=""
DST_SECRET_NAME=""
SRC_NS_TEMPLATE=""
DST_NAMESPACE=""
MANIFESTWORK_NAME=""
LABEL_SELECTOR=""
CLUSTERS_CSV=""
APPLY=0
KUBECONFIG=""
KUBE_CONTEXT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --secret-name)
      SECRET_NAME="$2"; shift 2;;
    --dst-secret-name)
      DST_SECRET_NAME="$2"; shift 2;;
    --src-namespace-template)
      SRC_NS_TEMPLATE="$2"; shift 2;;
    --dst-namespace)
      DST_NAMESPACE="$2"; shift 2;;
    --manifestwork-name)
      MANIFESTWORK_NAME="$2"; shift 2;;
    --label-selector)
      LABEL_SELECTOR="$2"; shift 2;;
    --clusters)
      CLUSTERS_CSV="$2"; shift 2;;
    --apply)
      APPLY=1; shift 1;;
    --kubeconfig)
      KUBECONFIG="$2"; shift 2;;
    --context)
      KUBE_CONTEXT="$2"; shift 2;;
    -h|--help)
      usage; exit 0;;
    *)
      echo "Unknown arg: $1" >&2
      usage
      exit 2;;
  esac
done

if [[ -z "$SECRET_NAME" || -z "$SRC_NS_TEMPLATE" || -z "$DST_NAMESPACE" ]]; then
  echo "ERROR: --secret-name, --src-namespace-template, --dst-namespace are required." >&2
  usage
  exit 2
fi

if [[ -z "$DST_SECRET_NAME" ]]; then
  DST_SECRET_NAME="$SECRET_NAME"
fi

if [[ -z "$MANIFESTWORK_NAME" ]]; then
  # Keep within DNS label limits (ManifestWork name max 253, but keep it simple)
  MANIFESTWORK_NAME="copy-secret-${SECRET_NAME}"
fi

OC=(oc)
if [[ -n "$KUBECONFIG" ]]; then
  OC+=(--kubeconfig "$KUBECONFIG")
fi
if [[ -n "$KUBE_CONTEXT" ]]; then
  OC+=(--context "$KUBE_CONTEXT")
fi

log() { printf '%s\n' "$*" >&2; }

# Build list of clusters
clusters=()
if [[ -n "$CLUSTERS_CSV" ]]; then
  IFS=',' read -r -a clusters <<< "$CLUSTERS_CSV"
else
  args=(get managedclusters -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
  if [[ -n "$LABEL_SELECTOR" ]]; then
    args=(get managedclusters -l "$LABEL_SELECTOR" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
  fi
  mapfile -t clusters < <("${OC[@]}" "${args[@]}")
fi

if [[ ${#clusters[@]} -eq 0 ]]; then
  log "No managedclusters found."
  exit 0
fi

# Generate ManifestWork YAML for a given cluster.
# We embed the Secret as a raw manifest. We keep metadata.namespace=dstNamespace.
# We keep type and data as-is.
render_manifestwork() {
  local cluster="$1"
  local src_ns
  src_ns="${SRC_NS_TEMPLATE//\{\{CLUSTER\}\}/$cluster}"

  # Fetch Secret from hub side namespace
  if ! secret_json=$(${OC[@]} get secret "$SECRET_NAME" -n "$src_ns" -o json 2>/dev/null); then
    log "WARN: secret '$SECRET_NAME' not found in namespace '$src_ns' (cluster '$cluster'), skipping."
    return 1
  fi

  # Extract required fields with jq.
  local secret_type
  secret_type=$(jq -r '.type // ""' <<<"$secret_json")

  # data must be base64 strings already; keep as map.
  local secret_data_json
  secret_data_json=$(jq -c '.data // {}' <<<"$secret_json")

  # Build YAML. We intentionally _do not_ copy metadata.uid/resourceVersion/etc.
  # Convert the Secret's data map to YAML-like lines with proper indentation.
  # (We output lines: key: value). Keys are safe as Secret data keys are strings.
  local secret_data_yaml
  secret_data_yaml=$(jq -r 'to_entries[] | "\(.key): \(.value)"' <<<"$secret_data_json")

  cat <<EOF
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: ${MANIFESTWORK_NAME}
  namespace: ${cluster}
  labels:
    app.kubernetes.io/managed-by: ocm-dynamic-scoring
    ocm-dynamic-scoring/secret-copy: "true"
spec:
  workload:
    manifests:
      - apiVersion: v1
        kind: Secret
        metadata:
          name: ${DST_SECRET_NAME}
          namespace: ${DST_NAMESPACE}
        type: "${secret_type}"
        data:
$(printf '%s\n' "$secret_data_yaml" | sed 's/^/          /')
EOF
}

apply_manifestwork() {
  local cluster="$1"
  local yaml_in="$2"

  # Create/update in hub namespace == cluster
  # We use apply to be idempotent.
  printf '%s' "$yaml_in" | "${OC[@]}" apply -f - >/dev/null
  log "OK: applied ManifestWork '${MANIFESTWORK_NAME}' in namespace '${cluster}'"
}

log "Target Secret: ${SECRET_NAME}"
log "Destination Secret name (managed): ${DST_SECRET_NAME}"
log "Source namespace template (hub): ${SRC_NS_TEMPLATE}"
log "Destination namespace (managed): ${DST_NAMESPACE}"
log "ManifestWork name: ${MANIFESTWORK_NAME}"
log "Mode: $([[ $APPLY -eq 1 ]] && echo apply || echo dry-run)"

failures=0
for c in "${clusters[@]}"; do
  [[ -z "$c" ]] && continue
  if ! mw_yaml=$(render_manifestwork "$c"); then
    failures=$((failures+1))
    continue
  fi

  if [[ $APPLY -eq 1 ]]; then
    apply_manifestwork "$c" "$mw_yaml"
  else
    # Print separator + YAML to stdout for inspection
    printf '\n# --- cluster: %s ---\n' "$c"
    printf '%s\n' "$mw_yaml"
  fi
done

if [[ $failures -gt 0 ]]; then
  log "Finished with ${failures} skipped/failed clusters." 
fi
