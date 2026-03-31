#!/usr/bin/env bash
# Generate TLS certificates for Flower SuperLink and create Kubernetes Secrets.
#
# Usage:
#   ./hack/generate-certs.sh [--hub-ip <IP>] [--namespace <ns>] [--days <validity>]
#
# This creates two Secrets:
#   flower-tls-ca         - CA cert + key (used to verify server identity)
#   flower-superlink-tls  - SuperLink server cert + key + CA cert

set -euo pipefail

NAMESPACE="${NAMESPACE:-flower-system}"
DAYS=365
HUB_IPS=()
CERT_DIR=""

usage() {
  echo "Usage: $0 [--hub-ip <IP>]... [--namespace <ns>] [--days <n>]"
  echo ""
  echo "Options:"
  echo "  --hub-ip <IP>      Hub node IP to include in server cert SANs (repeatable)"
  echo "  --namespace <ns>   Kubernetes namespace (default: flower-system)"
  echo "  --days <n>         Certificate validity in days (default: 365)"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --hub-ip)
      [[ -z "${2:-}" ]] && { echo "Error: --hub-ip requires a value"; exit 1; }
      HUB_IPS+=("$2")
      shift 2
      ;;
    --namespace)
      [[ -z "${2:-}" ]] && { echo "Error: --namespace requires a value"; exit 1; }
      NAMESPACE="$2"
      shift 2
      ;;
    --days)
      [[ -z "${2:-}" ]] && { echo "Error: --days requires a value"; exit 1; }
      DAYS="$2"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    *)
      echo "Unknown option: $1"
      usage
      ;;
  esac
done

cleanup() {
  if [[ -n "${CERT_DIR}" && -d "${CERT_DIR}" ]]; then
    rm -rf "${CERT_DIR}"
  fi
}
trap cleanup EXIT

CERT_DIR=$(mktemp -d)

echo "==> Generating CA certificate..."
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "${CERT_DIR}/ca.key" \
  -out "${CERT_DIR}/ca.crt" \
  -days "${DAYS}" \
  -subj "/CN=Flower CA/O=flower-addon"

echo "==> Generating SuperLink server certificate..."

# Build SAN extension
SAN="DNS:superlink,DNS:superlink.${NAMESPACE},DNS:superlink.${NAMESPACE}.svc,DNS:superlink.${NAMESPACE}.svc.cluster.local"
for ip in "${HUB_IPS[@]}"; do
  SAN="${SAN},IP:${ip}"
done

cat > "${CERT_DIR}/server-ext.cnf" <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name

[req_distinguished_name]

[v3_req]
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = ${SAN}
EOF

openssl req -newkey rsa:2048 -nodes \
  -keyout "${CERT_DIR}/server.key" \
  -out "${CERT_DIR}/server.csr" \
  -subj "/CN=superlink/O=flower-addon"

openssl x509 -req \
  -in "${CERT_DIR}/server.csr" \
  -CA "${CERT_DIR}/ca.crt" \
  -CAkey "${CERT_DIR}/ca.key" \
  -CAcreateserial \
  -out "${CERT_DIR}/server.pem" \
  -days "${DAYS}" \
  -extensions v3_req \
  -extfile "${CERT_DIR}/server-ext.cnf"

echo "==> Creating namespace ${NAMESPACE} (if not exists)..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

echo "==> Creating Secret flower-tls-ca..."
kubectl create secret generic flower-tls-ca \
  --namespace="${NAMESPACE}" \
  --from-file=ca.crt="${CERT_DIR}/ca.crt" \
  --from-file=ca.key="${CERT_DIR}/ca.key" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "==> Creating Secret flower-superlink-tls..."
kubectl create secret generic flower-superlink-tls \
  --namespace="${NAMESPACE}" \
  --from-file=server.pem="${CERT_DIR}/server.pem" \
  --from-file=server.key="${CERT_DIR}/server.key" \
  --from-file=ca.crt="${CERT_DIR}/ca.crt" \
  --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "TLS certificates created successfully!"
echo "  Namespace:  ${NAMESPACE}"
echo "  CA Secret:  flower-tls-ca"
echo "  TLS Secret: flower-superlink-tls"
echo "  SANs:       ${SAN}"
echo "  Validity:   ${DAYS} days"
echo ""
echo "Next: helm install flower-addon ./charts/flower-addon --set tls.enabled=true ..."
