# Prepare Multi-Cluster Environment

This guide captures the real preconditions used in deployment testing before `flock-addon` itself is installed.

`flock-addon` assumes you already have:

- one Kubernetes cluster that acts as the hub
- one or more separate Kubernetes clusters that act as managed clusters
- OCM installed on the hub
- each managed cluster registered to the hub and able to receive `ManifestWork`

For best results, keep hub and managed clusters on the same LAN. If they communicate over WAN, plan for TLS and stable API reachability.

## 1. Build Separate Kubernetes Clusters

Create one cluster on the hub machine and one independent cluster on each managed machine. Do not join the managed machines into the hub's kubeadm cluster. This setup is multi-cluster, not multi-node.

Common host preparation on every machine:

```bash
sudo swapoff -a
sudo sed -i '/ swap / s/^/#/' /etc/fstab

cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
EOF
sudo modprobe overlay
sudo modprobe br_netfilter

cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF
sudo sysctl --system
```

Install containerd and Kubernetes packages:

```bash
sudo apt-get update
sudo apt-get install -y containerd
sudo mkdir -p /etc/containerd
sudo containerd config default | sudo tee /etc/containerd/config.toml > /dev/null
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
sudo systemctl restart containerd
sudo systemctl enable containerd

sudo apt-get update
sudo apt-get install -y apt-transport-https ca-certificates curl gpg
sudo mkdir -p -m 755 /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.30/deb/Release.key \
  | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.30/deb/ /" \
  | sudo tee /etc/apt/sources.list.d/kubernetes.list
sudo apt-get update
sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl
```

Initialize each single-node cluster with kubeadm and install a CNI. Calico was used in testing:

```bash
sudo kubeadm init --pod-network-cidr=192.168.0.0/16

mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config

curl -LO https://raw.githubusercontent.com/projectcalico/calico/v3.31.4/manifests/calico.yaml
kubectl apply -f calico.yaml
kubectl get pods -n kube-system -w
kubectl get nodes -w
```

## 2. Remove Single-Node Control-Plane Taints

For single-node clusters, remove the default taint so OCM agents and addon workloads can schedule on the only node.

Run on the hub and on every single-node managed cluster:

```bash
kubectl taint nodes --all node-role.kubernetes.io/control-plane:NoSchedule- || true
kubectl taint nodes --all node-role.kubernetes.io/master:NoSchedule- || true
```

This is especially important on the hub if `cluster-manager` Pods stay `Pending` with `FailedScheduling`.

## 3. Install OCM and Register Managed Clusters

Install `clusteradm` on the machine where you will run OCM bootstrap commands:

```bash
curl -fsSL https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/main/install.sh | bash
clusteradm version
```

Set a stable hub context name if needed:

```bash
kubectl config get-contexts
kubectl config rename-context kubernetes-admin@kubernetes <hub-context>
export CTX_HUB=<hub-context>
```

Initialize the hub:

```bash
clusteradm init --context "${CTX_HUB}"
```

If you later need a new join token:

```bash
clusteradm get token --context "${CTX_HUB}"
```

For each managed cluster:

1. Make sure the node is `Ready`, the CNI is healthy, and the control-plane taint is gone.
2. Give the managed cluster context a stable name such as `<cluster-a>`, `<cluster-b>`, or `<cluster-c>`.
3. Run `clusteradm join` against that context.

Example:

```bash
kubectl config rename-context kubernetes-admin@kubernetes <managed-context>
export CTX_MANAGED_CLUSTER=<managed-context>

clusteradm join \
  --hub-token <token-from-clusteradm-init-or-get-token> \
  --hub-apiserver https://<hub-apiserver>:6443 \
  --cluster-name "${CTX_MANAGED_CLUSTER}" \
  --context "${CTX_MANAGED_CLUSTER}"
```

If registration fails partway through on a managed cluster, clean up the OCM agent namespaces before retrying:

```bash
kubectl delete ns open-cluster-management-agent --wait=true
kubectl delete ns open-cluster-management-agent-addon --wait=true 2>/dev/null || true
```

## 4. Approve CSRs and Accept the Cluster on the Hub

If `clusteradm accept` reports `no csr is approved yet`, approve the registration CSRs on the hub first.

List the CSRs:

```bash
kubectl get csr -o custom-columns=NAME:.metadata.name,REQUESTER:.spec.username,STATUS:.status.conditions[*].type --no-headers
```

Approve the latest CSR for that cluster:

```bash
kubectl certificate approve <csr-name>
```

Then accept the managed cluster:

```bash
clusteradm accept --clusters <managed-cluster-name> --context "${CTX_HUB}"
```

Verify:

```bash
kubectl get managedcluster
kubectl get managedcluster <managed-cluster-name> -o yaml | sed -n '/status:/,$p'
```

You want `Joined=True` and `Available=True`.

## 5. Verify ManifestWork Distribution Before Installing FLock Addon

Before debugging `flock-addon`, make sure the hub-to-managed work pipeline is already healthy.

Create a minimal `ManifestWork` in the namespace that matches a managed cluster name:

```yaml
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: demo-nginx
  namespace: <managed-cluster-name>
spec:
  workload:
    manifests:
      - apiVersion: v1
        kind: Namespace
        metadata:
          name: demo
      - apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: nginx
          namespace: demo
        spec:
          replicas: 1
          selector:
            matchLabels:
              app: nginx
          template:
            metadata:
              labels:
                app: nginx
            spec:
              containers:
                - name: nginx
                  image: nginx:stable
                  ports:
                    - containerPort: 80
```

Apply it from the hub:

```bash
kubectl apply -f mw-nginx-<managed-cluster-name>.yaml
kubectl -n <managed-cluster-name> get manifestwork demo-nginx -o yaml | sed -n '/status:/,$p'
```

Verify on the managed cluster:

```bash
kubectl --context=<managed-context> get ns demo
kubectl --context=<managed-context> -n demo get deploy nginx
```

If this test fails, fix OCM registration or network reachability before moving on to `flock-addon`.

## 6. Install Hub-Side Tools Used by FLock Addon

On the hub machine:

```bash
sudo apt update
sudo apt install -y make

curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh
helm version
```

Docker is required for local-chain modes and for custom image publishing. The tested setup used Docker Engine plus the Compose plugin:

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg
echo \
"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
$(. /etc/os-release && echo \"$VERSION_CODENAME\") stable" | \
sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
docker ps
```

If your user is not already allowed to access Docker directly:

```bash
sudo usermod -aG docker $USER
newgrp docker
docker ps
```

## 7. Prepare GPU-Capable Managed Clusters

Only do this on managed clusters that should run the GPU addon template.

Verify the host GPU first:

```bash
nvidia-smi
dpkg -l | grep -E 'nvidia-container-toolkit|nvidia-container-toolkit-base'
which nvidia-ctk
```

If needed, install the NVIDIA container toolkit and configure containerd:

```bash
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | \
  sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=containerd --set-as-default
sudo systemctl restart containerd
sudo systemctl restart kubelet
```

Install the NVIDIA device plugin:

```bash
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.19.0/deployments/static/nvidia-device-plugin.yml
kubectl get ds -A | grep -i nvidia
kubectl get pods -A | grep -i nvidia
kubectl -n kube-system logs -l name=nvidia-device-plugin-ds --tail=100
kubectl get node -o custom-columns=NAME:.metadata.name,GPU:.status.allocatable.nvidia\\.com/gpu
```

Label the managed clusters on the hub so `make enable-addon` will select the GPU template:

```bash
kubectl label managedcluster <cluster-a> gpu=true --overwrite
kubectl label managedcluster <cluster-b> gpu=true --overwrite
kubectl label managedcluster <cluster-c> gpu=true --overwrite
```

## 8. Prepare Managed-Cluster Data Mounts

On every managed cluster node:

```bash
sudo mkdir -p /data/flock-client
sudo chmod 755 /data
sudo chown -R <login-user>:<login-group> /data/flock-client
sudo chmod -R u+rwX /data/flock-client
```

> The `/data/flock-client` path used here **must match** the chart value `agent.dataVolume.hostPath` in `flock-addon/charts/flock-addon/values.yaml`. If you change one, change the other (or override with `--set agent.dataVolume.hostPath=...` on every `make deploy*` invocation), otherwise the container will mount an empty path and FLockAlliance will fail to find `.env` and the per-node dataset.

Everything in `/data/flock-client` is mounted into the addon container at `/data`. Put node-local runtime files there:

- `.env`
- input datasets
- unpacked demo files or model inputs required by your workflow

## Next Step

After the hub, managed clusters, and ManifestWork pipeline are healthy, continue with [Install FLock Addon](install-flock-addon.md).
