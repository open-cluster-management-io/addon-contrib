# Enable TLS

By default, SuperLink and SuperNode communicate over insecure gRPC. This guide enables TLS-secured connections between them.

**What gets configured:**
- **SuperLink** on hub cluster starts with `--ssl-certfile`/`--ssl-keyfile` for server-side TLS
- **SuperNode** on managed clusters connects with `--root-certificates` to verify the SuperLink identity
- **CA certificate** is automatically distributed to managed clusters via the OCM AddOnTemplate

## 1. Generate Certificates

```bash
cd flower-addon

# Generate CA + server cert (include hub node IP in SANs for NodePort access)
./hack/generate-certs.sh --hub-ip <HUB_NODE_IP>
```

This creates two Kubernetes Secrets in `flower-system` (default namespace). If you override `superlink.namespace` in Helm values, pass the same namespace here:

```bash
./hack/generate-certs.sh --hub-ip <HUB_NODE_IP> --namespace <YOUR_NAMESPACE>
```

- `flower-tls-ca` — CA certificate and key (used to verify server identity)
- `flower-superlink-tls` — SuperLink server certificate, key, and CA cert

The server certificate SANs include:
- `superlink`, `superlink.flower-system`, `superlink.flower-system.svc`, `superlink.flower-system.svc.cluster.local`
- Any hub node IPs passed via `--hub-ip` (repeatable for multiple IPs)

## 2. Deploy with TLS Enabled

```bash
helm install flower-addon ./charts/flower-addon \
  --set tls.enabled=true \
  --set deploymentConfig.superlinkAddress=<HUB_NODE_IP>
```

When `tls.enabled=true`:
- SuperLink mounts the server cert secret and starts with `--ssl-ca-certfile`, `--ssl-certfile`, `--ssl-keyfile`
- The CA certificate is read from the `flower-tls-ca` Secret (via Helm `lookup`) and embedded in the AddOnTemplate
- SuperNodes on managed clusters receive the CA cert as a Secret and start with `--root-certificates`

## 3. Run FL Jobs with TLS

When submitting FL jobs via `flwr run`, configure TLS in `~/.flwr/config.toml`:

```toml
[superlink.ocm-deployment]
address = "<HUB_NODE_IP>:30093"
insecure = false
root-certificates = "/path/to/ca.crt"
```

Extract the CA cert from the cluster:

```bash
# Use the same namespace as your SuperLink deployment (default: flower-system)
kubectl get secret flower-tls-ca -n flower-system -o jsonpath='{.data.ca\.crt}' | base64 -d > ca.crt
```

Then run:

```bash
flwr run . ocm-deployment --stream
```
