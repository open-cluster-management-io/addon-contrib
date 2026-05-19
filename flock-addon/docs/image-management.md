# Image Management

Use this guide to choose the runtime image for `flock-addon`, publish a custom build, and verify the managed cluster is pulling the expected image.

## Default Image Resolution

Chart fallback image (from `values.yaml`):

- `ghcr.io/flock-io/fl-alliance-client:latest`

> The `latest` tag is convenient for `kind`/`k3d` smoke tests but defeats Kubernetes' ability to detect that the image actually changed across rollouts. For anything beyond local development, override `IMAGE_TAG` (or `FLOCK_ALLIANCE_IMAGE`) with an immutable reference such as a release tag or short git SHA, and consider switching `IMAGE_PULL_POLICY=IfNotPresent` to skip the registry round-trip on every Pod restart.

Environment-variable based overrides supported by the `Makefile`:

- `IMAGE_REGISTRY`, default `ghcr.io`
- `IMAGE_OWNER`, default `flock-io`
- `IMAGE_NAME`, default `fl-alliance-client`
- `IMAGE_TAG`, default `latest`
- `IMAGE_PULL_POLICY`, default `Always`
- `IMAGE_PULL_SECRET`, optional managed-cluster image pull secret name
- `FLOCK_ALLIANCE_IMAGE`, overrides all of the above

Example using exported environment variables (useful when the same overrides apply to several `make` invocations in the same shell session):

```bash
# [Hub]
export IMAGE_REGISTRY='ghcr.io'
export IMAGE_OWNER='<image-owner>'
export IMAGE_NAME='fl-alliance-client'
export IMAGE_TAG='<git-sha-or-release-tag>'
export IMAGE_PULL_POLICY='Always'
export FLOCK_ALLIANCE_IMAGE="${IMAGE_REGISTRY}/${IMAGE_OWNER}/${IMAGE_NAME}:${IMAGE_TAG}"
```

Use it for deployment. Replace the deploy command with the mode you are actually using. For exact mode-specific commands, see [Install FLock Addon](install-flock-addon.md) and [Deployment Modes](deployment-modes.md).

```bash
# [Hub]
make <deploy-command> <mode-specific-args>
```

Equivalent inline shorthand (useful for one-shot deploys, e.g. pinning a specific git SHA tag or attaching a pull secret for a single rollout, without polluting the shell environment):

```bash
# [Hub]
make deploy \
  IMAGE_OWNER=<image-owner> \
  IMAGE_TAG=<git-sha-or-release-tag> \
  IMAGE_PULL_SECRET=ghcr-pull \
  IMAGE_PULL_POLICY=Always
```

Both forms set the same Make variables; pick whichever matches your workflow.

Check:

```bash
# [Hub]
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config -o yaml | rg -A1 'name: FLOCK_ALLIANCE_IMAGE\b'
```

Should see:

- `FLOCK_ALLIANCE_IMAGE` matches the image you intend to deploy

## Public Image Repository

Use this path when the image already exists publicly in GHCR, for example:

- `ghcr.io/flock-io/fl-alliance-client:<release-tag>`

How to operate:

- do not clone `FL-Alliance-Client` unless you want to rebuild the image
- do not create an image pull secret
- deploy directly from `flock-addon`

Example:

```bash
# [Hub]
unset IMAGE_PULL_SECRET
export IMAGE_OWNER='flock-io'
export IMAGE_TAG='<release-tag>'
make <deploy-command> <mode-specific-args>
```

## Private Image Repository

Use this path when the image is private, for example:

- `ghcr.io/<image-owner>/fl-alliance-client:<git-sha-or-release-tag>`

How to operate:

1. Publish the image before addon deployment.
2. Create a pull secret on every managed cluster.
3. Set `IMAGE_PULL_SECRET` on the hub before deploy.

Create the registry secret on each managed cluster. We avoid `kubectl create secret docker-registry --docker-password="$GHCR_PAT"` because the PAT lands in argv (visible via `ps -ef` / `/proc/<pid>/cmdline`) for the lifetime of that command. Instead, materialize a `dockerconfigjson` file in `mktemp` at mode `0600`, feed it to `kubectl --from-file`, and `rm` it on exit — the PAT only lives in envp and on a short-lived `0600` file:

```bash
# [Managed Cluster context]
# Requires $GHCR_USER and $GHCR_PAT exported in the current shell.
auth=$(printf '%s:%s' "$GHCR_USER" "$GHCR_PAT" | base64 | tr -d '\n')
tmp=$(mktemp); chmod 600 "$tmp"
trap 'rm -f "$tmp"' EXIT
cat >"$tmp" <<EOF
{"auths":{"ghcr.io":{"username":"$GHCR_USER","password":"$GHCR_PAT","auth":"$auth"}}}
EOF
kubectl -n flock-system create secret generic ghcr-pull \
  --type=kubernetes.io/dockerconfigjson \
  --from-file=.dockerconfigjson="$tmp" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Deploy from the hub:

```bash
# [Hub]
export IMAGE_OWNER='<image-owner>'
export IMAGE_TAG='<git-sha-or-release-tag>'
export IMAGE_PULL_SECRET='ghcr-pull'
export IMAGE_PULL_POLICY='Always'
make <deploy-command> <mode-specific-args>
```

Check:

```bash
# [Managed Cluster context]
kubectl -n flock-system get secret ghcr-pull
kubectl -n flock-system get deploy flock-agent -o yaml | rg -n "imagePullSecrets|ghcr-pull"
```

Should see:

- `ghcr-pull` exists
- `deployment/flock-agent` references `imagePullSecrets`

## Publish a Custom Image

You only need the `FL-Alliance-Client` source repository if you want to build or publish your own image.

Source repository:

- [FL-Alliance-Client](https://github.com/FLock-io/FL-Alliance-Client.git)

Clone it before local image build or manual image push:

```bash
# [Hub or image-build machine]
cd ~
git clone https://github.com/FLock-io/FL-Alliance-Client.git
cd FL-Alliance-Client
```

Example publish flow. We pass `$GHCR_USER` / `$GHCR_PAT` through the environment (via `export`) rather than appending them as `make NAME=VALUE` arguments — the FL-Alliance-Client Makefile declares `GHCR_USER ?=` / `GHCR_PAT ?=`, so `make`'s conditional assignment picks them up from envp when no command-line override is present. This narrows the argv exposure window for the PAT from "the entire `make` run (~30s)" down to "the brief `printf | docker login` recipe shell (~1s)"; the upstream `image-login` recipe still uses Make-time expansion of `$(GHCR_PAT)`, so a fully argv-free path requires an upstream change in FL-Alliance-Client (track separately). `docker login --password-stdin` ensures the docker daemon's auth cache itself never sees the PAT on argv:

```bash
# [FL-Alliance-Client workspace]
export IMAGE_SHA=$(git rev-parse --short=12 HEAD)
export GHCR_USER GHCR_PAT                     # in envp, picked up by `?=`
echo "$GHCR_PAT" | docker login ghcr.io -u "$GHCR_USER" --password-stdin
make image-publish \
  DOCKER='sudo docker' \
  IMAGE_OWNER="$GHCR_USER" \
  IMAGE_TAG="$IMAGE_SHA" \
  IMAGE_IMMUTABLE_TAG="$IMAGE_SHA"
# (drop GHCR_USER=/GHCR_PAT= make args — FL-Alliance-Client's Makefile
# reads them from envp via `?=` when exported, and the docker daemon
# already holds the auth cache from --password-stdin above.)
```

If your environment does not need `sudo` for Docker, you can omit `DOCKER='sudo docker'`.

Check:

```bash
# [Hub]
export IMAGE_OWNER="$GHCR_USER"
export IMAGE_TAG="$IMAGE_SHA"
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config -o yaml | rg -A1 'name: FLOCK_ALLIANCE_IMAGE\b'
```

Should see:

- addon deployment uses an immutable tag such as `$IMAGE_SHA`

If you use GitHub Actions instead of local push, wait for the publish workflow to finish successfully before deploying the addon.

## Verify the Deployed Image

```bash
# [Hub]
kubectl -n open-cluster-management get addondeploymentconfig flock-addon-config -o yaml | rg -A1 'name: FLOCK_ALLIANCE_IMAGE\b'

# [Managed Cluster context]
kubectl -n flock-system describe pod -l app.kubernetes.io/name=flock-addon
```

If you republish the same tag, redeploy with an explicit image override and re-enable the addon:

```bash
# [Hub]
IMAGE_OWNER='<image-owner>' IMAGE_TAG='<git-sha-or-release-tag>' IMAGE_PULL_POLICY='Always' make <deploy-command> <mode-specific-args>
make disable-addon CLUSTER=<cluster-name>
make enable-addon CLUSTER=<cluster-name>
```

If you also rotated credentials for a private registry, recreate the secret in place and trigger a rolling restart of the addon Deployment. Use `rollout restart` rather than `kubectl delete pod --all`: the latter wipes every Pod in the namespace (including any sidecar Pods OCM may have placed there) and produces a hard outage instead of a controlled rollout. Use `apply` rather than `delete && create`: the latter opens a window where pod restarts cannot pull from the private registry, which causes spurious `ImagePullBackOff` events on any pod that happens to crash during the rotation.

```bash
# [Managed Cluster context]
# Atomic in-place update — the dry-run-and-apply idiom replaces the
# secret without ever leaving the cluster without one. Pods that
# happen to restart during this command keep using the previous
# secret until apply completes (kubectl apply is non-disruptive for
# Secrets), so there is no `ImagePullBackOff` window.
#
# Uses the same `mktemp + chmod 600 + --from-file` pattern as the
# initial create (above) so the rotated PAT never appears in argv.
auth=$(printf '%s:%s' "$GHCR_USER" "$GHCR_PAT" | base64 | tr -d '\n')
tmp=$(mktemp); chmod 600 "$tmp"
trap 'rm -f "$tmp"' EXIT
cat >"$tmp" <<EOF
{"auths":{"ghcr.io":{"username":"$GHCR_USER","password":"$GHCR_PAT","auth":"$auth"}}}
EOF
kubectl -n flock-system create secret generic ghcr-pull \
  --type=kubernetes.io/dockerconfigjson \
  --from-file=.dockerconfigjson="$tmp" \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart only the addon Deployment. `rollout restart` is label-scoped
# under the hood (it bumps the Deployment's pod-template-hash), so
# it cannot evict unrelated workloads or OCM-placed sidecars that
# happen to share the namespace.
kubectl -n flock-system rollout restart deploy/flock-agent
kubectl -n flock-system rollout status deploy/flock-agent --timeout=5m
```
