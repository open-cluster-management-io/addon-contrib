#!/usr/bin/env bash
# Creates "slurm-cluster": a kind cluster whose batch nodes are scheduled by Slurm through
# slurm-bridge, installs Kueue on it, and joins it to the OCM hub from
# build/setup-env.sh --clusterprofile.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
CLUSTER="${SLURM_CLUSTER_NAME:-slurm-cluster}"
CTX="kind-${CLUSTER}"
HUB_CTX="${HUB_CTX:-kind-local-cluster}"
NODE_IMAGE="${NODE_IMAGE:-kindest/node:v1.35.0}"   # slurm-bridge 1.2.x needs Kubernetes >= 1.35
SLINKY_VERSION="1.2.4"                             # slurm-operator, slurm chart and slurm-bridge
CERT_MANAGER_VERSION="v1.21.2"
KUEUE_VERSION="v0.20.7"
K="kubectl --context $CTX"
HELM="helm --kube-context $CTX"
SLINKY_CHARTS="oci://ghcr.io/slinkyproject/charts"

log() { echo; echo "==> $*"; }

# slurm-cluster joins the ClusterProfile environment from build/setup-env.sh --clusterprofile.
cp_mode=$(kubectl --context "$HUB_CTX" get deploy kueue-addon-controller -n open-cluster-management-addon \
  -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="ENABLE_CLUSTERPROFILE")].value}' 2>/dev/null || true)
if [ "$cp_mode" != "true" ]; then
  echo "The kueue-addon on $HUB_CTX is not running in ClusterProfile mode." >&2
  echo "Create the environment with: build/setup-env.sh --clusterprofile" >&2
  exit 1
fi

# Each kind node uses inotify instances; the default of 128 (or 512) runs out quickly.
if [ "$(sysctl -n fs.inotify.max_user_instances)" -lt 1024 ]; then
  echo "fs.inotify.max_user_instances is $(sysctl -n fs.inotify.max_user_instances)." >&2
  echo "Raise it first:  sudo sysctl fs.inotify.max_user_instances=2048" >&2
  exit 1
fi

log "Creating kind cluster $CLUSTER ($NODE_IMAGE)"
if ! kind get clusters | grep -Fxq "$CLUSTER"; then
  kind create cluster --name "$CLUSTER" --image "$NODE_IMAGE" --config "$DIR/slurm-cluster/kind.yaml"
fi
# kind configs cannot set node annotations. This one tells slurm-bridge which Slurm
# partition to register each external node in.
$K annotate nodes -l scheduler.slinky.slurm.net/external-node=true \
  scheduler.slinky.slurm.net/external-node-partitions=slurm-bridge --overwrite
$K wait --for=condition=Ready nodes --all --timeout=300s

log "Installing cert-manager $CERT_MANAGER_VERSION"
$HELM upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --version "$CERT_MANAGER_VERSION" -n cert-manager --create-namespace \
  --set crds.enabled=true --wait

log "Installing slurm-operator $SLINKY_VERSION"
$HELM upgrade --install slurm-operator-crds "$SLINKY_CHARTS/slurm-operator-crds" --version "$SLINKY_VERSION"
$HELM upgrade --install slurm-operator "$SLINKY_CHARTS/slurm-operator" --version "$SLINKY_VERSION" \
  -n slinky --create-namespace --wait
echo "Waiting for the slurm-operator webhook to accept requests"
until $K create --dry-run=server -n slinky -f - >/dev/null 2>&1 <<'EOF'
apiVersion: slinky.slurm.net/v1beta1
kind: RestApi
metadata:
  generateName: webhook-check-
spec:
  controllerRef:
    name: webhook-check
EOF
do sleep 2; done

log "Installing the Slurm control plane (slurmctld + slurmrestd)"
$HELM upgrade --install slurm "$SLINKY_CHARTS/slurm" --version "$SLINKY_VERSION" \
  -n slurm --create-namespace -f "$DIR/slurm-cluster/slurm-values.yaml" --wait --timeout 10m
until $K get pod -n slurm slurm-controller-0 >/dev/null 2>&1; do sleep 2; done
$K wait --for=condition=Ready pod/slurm-controller-0 -n slurm --timeout=600s

log "Installing slurm-bridge $SLINKY_VERSION"
$K apply -f "$DIR/slurm-cluster/token.yaml"
until $K get secret -n slurm slurm-bridge-token >/dev/null 2>&1; do sleep 2; done
$K create namespace hpc --dry-run=client -o yaml | $K apply -f -
$HELM upgrade --install slurm-bridge "$SLINKY_CHARTS/slurm-bridge" --version "$SLINKY_VERSION" \
  -n slurm -f "$DIR/slurm-cluster/bridge-values.yaml" --wait --timeout 5m
echo "Waiting for slurm-bridge to register the Kubernetes nodes in Slurm"
until [ "$($K exec -n slurm slurm-controller-0 -c slurmctld -- sinfo -h -N -o %N 2>/dev/null | wc -l)" -ge 2 ]; do sleep 3; done
$K exec -n slurm slurm-controller-0 -c slurmctld -- sinfo -N -o '%N %P %c %m %T'

log "Installing Kueue $KUEUE_VERSION"
$K apply --server-side -f "https://github.com/kubernetes-sigs/kueue/releases/download/${KUEUE_VERSION}/manifests.yaml" >/dev/null
$K wait --for=condition=Available deploy/kueue-controller-manager -n kueue-system --timeout=300s
"$DIR/slurm-cluster/install-integration-crds.sh" "$CTX"

log "Joining $CLUSTER to the OCM hub ($HUB_CTX)"
HK="kubectl --context $HUB_CTX"
# The hub does not clear ManagedClusterJoined when a worker cluster is deleted and recreated,
# so also require this worker's registration agent before trusting the condition.
agent_registered() {
  $K get deploy klusterlet-registration-agent -n open-cluster-management-agent >/dev/null 2>&1 &&
    $K get secret hub-kubeconfig-secret -n open-cluster-management-agent >/dev/null 2>&1
}
joined() {
  [ "$($HK get managedcluster "$CLUSTER" -o jsonpath='{.status.conditions[?(@.type=="ManagedClusterJoined")].status}' 2>/dev/null)" = "True" ] && agent_registered
}
# The agent's heartbeat renews the cluster's lease on the hub; a renewTime newer than $1
# proves this worker's agent registered, not a leftover Joined condition from a previous
# cluster with the same name.
lease_renewed_since() {
  local rt ts
  rt=$($HK get lease managed-cluster-lease -n "$CLUSTER" -o jsonpath='{.spec.renewTime}' 2>/dev/null)
  [ -n "$rt" ] || return 1
  ts=$(date -d "$rt" +%s 2>/dev/null) || { echo "ERROR: cannot parse lease renewTime '$rt'" >&2; exit 1; }
  [ "$ts" -ge "$1" ]
}
# Approve only CSRs from this cluster's OCM registration agent, the same validation the hub's
# CSR-approving controller does: the signer must be the kube-apiserver-client signer, the
# requester must be a bootstrap user or this cluster's agent, and the requested identity must
# be the agent's (O=system:open-cluster-management:<cluster>,
# CN=system:open-cluster-management:<cluster>:<agent>). A pending CSR has no conditions yet
# (approved ones have Approved, denied ones Denied). Any other CSR is left unapproved, even
# when it carries this cluster's name label.
approve_registration_csrs() {
  local csr signer requester subject ok
  for csr in $($HK get csr -l "open-cluster-management.io/cluster-name=$CLUSTER" --no-headers \
      -o 'custom-columns=NAME:.metadata.name,COND:.status.conditions[*].type' 2>/dev/null \
      | awk '$2=="<none>"{print $1}'); do
    signer=$($HK get csr "$csr" -o jsonpath='{.spec.signerName}')
    requester=$($HK get csr "$csr" -o jsonpath='{.spec.username}')
    subject=$($HK get csr "$csr" -o jsonpath='{.spec.request}' \
      | base64 -d 2>/dev/null | openssl req -noout -subject -nameopt RFC2253 2>/dev/null)
    ok=false
    if [ "$signer" = "kubernetes.io/kube-apiserver-client" ]; then
      case "$requester" in
        "system:bootstrap:"*|"system:open-cluster-management:$CLUSTER:"*)
          if echo "$subject" | grep -Eq "(^|,)O=system:open-cluster-management:$CLUSTER(,|$)" \
             && echo "$subject" | grep -q "CN=system:open-cluster-management:$CLUSTER"; then
            ok=true
          fi
          ;;
      esac
    fi
    if [ "$ok" = true ]; then
      $HK certificate approve "$csr" >/dev/null 2>&1 || true
    else
      echo "Leaving CSR $csr unapproved (requester=${requester:-?}, signer=$signer, subject=${subject:-unparseable})" >&2
    fi
  done
}
if ! joined; then
  # Require a fresh registration result below, not the hub's possibly stale Joined condition.
  join_started=$(date +%s)
  joincmd=$(clusteradm get token --context "$HUB_CTX" | grep clusteradm)
  eval "${joincmd//<cluster_name>/$CLUSTER} --force-internal-endpoint-lookup --context $CTX"
fi
# Accept the cluster. The hub auto-approves the first join of a cluster name, but not a
# rejoin under the same name, so approve this cluster's pending CSRs explicitly (the same
# thing "clusteradm accept" does).
while :; do
  approve_registration_csrs
  $HK patch managedcluster "$CLUSTER" --type=merge -p '{"spec":{"hubAcceptsClient":true}}' >/dev/null 2>&1 || true
  # Either the cluster was already registered by this worker, or (after a join in this
  # run) the heartbeat lease was renewed since the join started.
  if joined && { [ -z "${join_started:-}" ] || lease_renewed_since "$join_started"; }; then break; fi
  sleep 5
done
$HK get managedcluster "$CLUSTER"
echo "Waiting for OCM to publish the ClusterProfile with a cluster-proxy access provider"
until [ -n "$($HK get clusterprofile "$CLUSTER" -n kueue-system \
  -o jsonpath='{.status.accessProviders[*].name}' 2>/dev/null)" ]; do sleep 5; done
echo "Waiting for managed-serviceaccount to sync the ${CLUSTER}-multikueue token"
until $HK get secret "${CLUSTER}-multikueue" -n kueue-system >/dev/null 2>&1; do sleep 5; done
echo "Waiting for the kueue-addon to create the MultiKueueCluster from the ClusterProfile"
until [ "$($HK get multikueuecluster "$CLUSTER" \
  -o jsonpath='{.spec.clusterSource.clusterProfileRef.name}/{.status.conditions[?(@.type=="Active")].status}' \
  2>/dev/null)" = "$CLUSTER/True" ]; do sleep 5; done
$HK get clusterprofile "$CLUSTER" -n kueue-system
$HK get multikueuecluster "$CLUSTER"
