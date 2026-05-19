# -----------------------------------------------------------------------------
# flock-addon Makefile
#
# Drives the addon's lifecycle and CI gates without depending on a hub-side
# operator. Targets are grouped into sections that `make help` prints:
#
#   Deployment   - public `make deploy-*`, `make update-task`, `make undeploy`,
#                  `make enable-addon`, `make disable-addon`
#   CI Targets   - contract with the addon-contrib CI pipeline (verify,
#                  build, test-unit, test-integration, test-e2e, test-chart)
#   Development  - `make lint`, `make template`, `make status`
#   Help         - `make help`
#
# `_deploy-*` targets are composition helpers: they assume their inputs are
# already validated by a public deploy-* wrapper and are intentionally
# underscored so end users avoid calling them directly.
# -----------------------------------------------------------------------------

# -----------------------------------------------------------------------------
# Recipe shell
#
# GNU make's default `/bin/sh` is dash on Debian/Ubuntu, which rejects
# `set -o pipefail` and a few other niceties this Makefile (and the
# embedded entrypoint script under templates/addon-template.yaml) relies
# on. Pinning bash with the strict flags is the canonical fix and matches
# what every recipe is already written against. `.SHELLFLAGS` is GNU
# make >= 3.82, available on every distro the addon targets.
# -----------------------------------------------------------------------------
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

# -----------------------------------------------------------------------------
# Tool binaries
#
# Kept overrideable so the same recipes work on hosts that need `sudo docker`
# (no rootless setup, no docker group), inside CI where helm/kubectl come
# from a custom path, and during local dev where the user may want to point
# at kind/k3d-vended versions. CHAIN_DOCKER_COMPOSE below inspects the first
# word of $(DOCKER) to detect the sudo case, so callers MUST keep the
# `sudo docker` form (not `docker sudo`, not aliases).
# -----------------------------------------------------------------------------
KUBECTL ?= kubectl
HELM ?= helm
DOCKER ?= docker
BATS ?= bats

# DOCKER_WITH_ENV is the env-preserving variant of $(DOCKER), used by the
# recipes below that hand secrets to docker via `-e VAR` env-inheritance
# (no `=value`) — start-local-s3-compatible, the model upload step, and
# the undeploy-local-chain bucket cleanup. When DOCKER='sudo docker',
# `sudo`'s default policy strips the parent environment before exec'ing
# docker, so `-e ACCESS_KEY -e SECRET_KEY` would see empty values and
# MinIO would silently start with empty credentials (readiness loop
# fails for 30s with no actionable signal). Mapping the first word
# `sudo` → `sudo -E` preserves envp for the docker child without
# affecting argv-only invocations (`docker ps`, `docker rm`, etc.) that
# stay on plain $(DOCKER). For non-sudo callers DOCKER_WITH_ENV is a
# no-op (== $(DOCKER)).
DOCKER_WITH_ENV ?= $(if $(filter sudo,$(word 1,$(DOCKER))),sudo -E $(wordlist 2,$(words $(DOCKER)),$(DOCKER)),$(DOCKER))

# -----------------------------------------------------------------------------
# OCM and chart identity
# -----------------------------------------------------------------------------
NAMESPACE ?= open-cluster-management
RELEASE_NAME ?= flock-addon
CHART_PATH ?= charts/flock-addon

# Test fixture locations. Both directories are checked in and discovered
# automatically by the test runners (helm-unittest scans
# `<chart>/tests/*_test.yaml`, bats picks up every *.bats below
# tests/entrypoint/).
TESTS_CHART_DIR ?= $(CHART_PATH)/tests
TESTS_ENTRYPOINT_DIR ?= tests/entrypoint
TESTS_MAKEFILE_DIR   ?= tests/makefile

# -----------------------------------------------------------------------------
# Image coordinates
#
# FLOCK_ALLIANCE_IMAGE is the single source of truth for the workload image.
# When callers override it (e.g. with a privately mirrored repo), the deploy
# recipes split it back into image.repository / image.tag for Helm using
# `rev | cut -d: -f1`. We split at the LAST colon — using `cut -d: -f1`
# without `rev` would mis-split values like `registry.local:5000/foo:tag`,
# which is the exact reason rev/cut/rev is used everywhere image=... appears.
# -----------------------------------------------------------------------------
IMAGE_REGISTRY ?= ghcr.io
IMAGE_OWNER ?= flock-io
IMAGE_NAME ?= fl-alliance-client
IMAGE_TAG ?= latest
IMAGE_PULL_POLICY ?= Always
FLOCK_ALLIANCE_IMAGE ?= $(IMAGE_REGISTRY)/$(IMAGE_OWNER)/$(IMAGE_NAME):$(IMAGE_TAG)
IMAGE_PULL_SECRET ?=

# -----------------------------------------------------------------------------
# Local-chain mode prerequisites
#
# Required only by the hub-managed local-chain deploy recipes
# (deploy-local-chain-s3*). They intentionally have no defaults so the
# recipes can `$(error ...)` with an actionable message instead of silently
# pointing at a non-existent path or contacting the wrong RPC host.
# -----------------------------------------------------------------------------
FL_ALLIANCE_CLIENT_DIR ?=
MODEL_ARCHIVE ?=
MODEL_HASH ?=
RPC_HOST ?=

# -----------------------------------------------------------------------------
# Compose forwarding
#
# When DOCKER='sudo docker', `make chain` in FL-Alliance-Client must invoke
# `sudo -E docker compose ...` (note `-E`: the chain target reads variables
# like MODEL_DEFINITION_HASH from the environment, which sudo strips by
# default). When DOCKER='docker', plain `docker compose` is enough. We
# detect by checking whether the first word of $(DOCKER) is `sudo`.
# -----------------------------------------------------------------------------
CHAIN_DOCKER_COMPOSE ?= $(if $(filter sudo,$(word 1,$(DOCKER))),sudo -E $(wordlist 2,$(words $(DOCKER)),$(DOCKER)) compose,$(DOCKER) compose)

# -----------------------------------------------------------------------------
# Local MinIO defaults (nami storage backend)
#
# These back the hub-managed MinIO instance used by the
# deploy-local-chain-s3-compatible flow. The container is bound to
# host-network ports so managed clusters can reach it as
# http://<RPC_HOST>:$(S3_COMPAT_PORT).
#
# Credentials and bucket name are intentionally task-scoped:
#
#   * deploy-local-chain-s3-compatible generates a fresh access/secret pair
#     (openssl rand) per invocation and passes them down to MinIO + the
#     AddOnDeploymentConfig in lockstep. Every FL client on every managed
#     cluster therefore receives the SAME credentials via OCM customizedVariable
#     substitution — multi-client coordination is preserved exactly as before;
#     only the value being propagated changes from the well-known
#     `minioadmin/minioadmin` to a per-task random pair.
#
#   * The bucket name is derived from MODEL_HASH (`flock-task-<sha256[:12]>`),
#     so concurrent tasks on the same hub never clobber each other's artifacts
#     and `make undeploy-local-chain` can recycle a task's bucket without
#     touching others.
#
# Operator-facing overrides (use either to pin known values, e.g. for CI):
#   MINIO_ACCESS_KEY=... MINIO_SECRET_KEY=...  -> skip random generation
#   MINIO_BUCKET=...                            -> skip MODEL_HASH derivation
#
# The S3_COMPAT_* variables below are still honored when running
# `start-local-s3-compatible` standalone (e.g. for manual `mc cp` testing
# from an operator laptop) — those defaults intentionally stay on the
# legacy `minioadmin` values so quick smoke tests don't require a fresh
# generation step.
# -----------------------------------------------------------------------------
MINIO_IMAGE ?= quay.io/minio/minio
MINIO_MC_IMAGE ?= quay.io/minio/mc
S3_COMPAT_CONTAINER_NAME ?= flock-minio
S3_COMPAT_DATA_DIR ?= /srv/flock-minio/data
S3_COMPAT_PORT ?= 9000
S3_COMPAT_CONSOLE_PORT ?= 9001
S3_COMPAT_ACCESS_KEY ?= minioadmin
S3_COMPAT_SECRET_KEY ?= minioadmin
S3_COMPAT_BUCKET ?= flock-dev
S3_COMPAT_REGION ?= us-east-1
S3_COMPAT_ADDRESSING_STYLE ?= path
S3_COMPAT_VERIFY_SSL ?= false

# -----------------------------------------------------------------------------
# Export the credential-bearing S3_COMPAT_* variables to recipe shells (and to
# any sub-`make` they invoke) so recipes can reference them via `$$S3_COMPAT_*`
# and have bash expand them at runtime instead of `$(S3_COMPAT_*)` having Make
# expand them at recipe-generation time. The latter would bake the literal
# values into the bash `-c "<recipe text>"` argv that GNU make hands to the
# recipe shell — making secrets visible to anyone running `ps -ef` /
# `cat /proc/<pid>/cmdline` on the host for the entire ~30s startup +
# readiness loop window. With the runtime-expansion form, the secret never
# leaves envp (which is mode 0400 to the process owner under `/proc/<pid>/
# environ` on Linux) and never enters argv anywhere. Behavior is unchanged
# for all standalone invocation paths because GNU make still uses these as
# Make variables in `--set` style command-line callers; `export` only widens
# the scope from "Make-internal" to "Make-internal + recipe-shell-env".
# -----------------------------------------------------------------------------
export S3_COMPAT_ACCESS_KEY S3_COMPAT_SECRET_KEY S3_COMPAT_BUCKET

# -----------------------------------------------------------------------------
# READ_CONTRACT_ADDR
#
# Inline python helper used by the `_deploy-local-chain-*` recipes to extract
# `<contract>.address` from FL-Alliance-Client's `data/contracts.json`. We
# define it once as a Make variable and `export` it so both the parent recipe
# shell and any sub-make see it via `$$READ_CONTRACT_ADDR`. Reasons for the
# helper to be a multi-line script (vs the previous one-liner inside `-c`):
#
#   * `try/except` around `open()` and `json.load()` turns I/O / parse errors
#     into a single-line `[flock-addon] ERROR: ...` on stderr (exit 1) instead
#     of a four-line Python traceback that drowns the cause. The shell-side
#     existence check (`if [ ! -f "$$CONTRACTS" ]`) catches the common case,
#     but a permission-error or partial-write JSON (`make chain` killed
#     mid-write) would still crash the previous one-liner with a Python
#     traceback. This helper handles both consistently.
#
#   * `.get(<contract>, {}).get('address', '')` preserves the existing empty-
#     string-on-missing-key behaviour that the surrounding shell `if [ -z ... ]`
#     guard relies on, so a chain that ran but did not register the contract
#     still hits the friendly "did not populate ... .address" error path.
#
#   * `argv[1]` = path to contracts.json, `argv[2]` = contract name. Two args
#     so the same helper covers both FlockToken and FlockTask without
#     duplicating the script.
#
# This is the addon's only inline-python in a hot path; the rest of the
# Makefile shells out to `jq`-free coreutils so we do not need to add yet
# another runtime dependency just for JSON parsing.
# -----------------------------------------------------------------------------
define READ_CONTRACT_ADDR
import json, sys
path, contract = sys.argv[1], sys.argv[2]
try:
    with open(path) as fh:
        data = json.load(fh)
except OSError as exc:
    print(f"[flock-addon] ERROR: cannot open {path}: {exc}", file=sys.stderr)
    sys.exit(1)
except ValueError as exc:
    print(f"[flock-addon] ERROR: {path} is not valid JSON: {exc}", file=sys.stderr)
    sys.exit(1)
print(data.get(contract, {}).get("address", ""))
endef
export READ_CONTRACT_ADDR

# Operator overrides for `make deploy-local-chain-s3-compatible`. Empty
# defaults trigger random generation inside the recipe.
MINIO_ACCESS_KEY ?=
MINIO_SECRET_KEY ?=
MINIO_BUCKET ?=

# -----------------------------------------------------------------------------
# Per-deploy resource overrides (FLockAlliance container)
#
# values.yaml ships conservative defaults (2Gi limit / 512Mi request) sized
# for FLock's small reference models so `kind`/`k3d` smoke tests fit on a
# laptop. Real LLM workloads (7B+ LoRA, MLX fine-tune, anything with a
# tokenizer warm-up bigger than a few hundred MB) blow past 2Gi instantly
# and the kubelet OOM-kills the container with exit 137.
#
# Rather than baking large numbers into values.yaml — which would refuse to
# schedule on small dev clusters — expose four optional Make vars. When set,
# they emit `--set agent.resources.flockAlliance.{requests,limits}.{cpu,memory}`
# below; when empty, values.yaml defaults win. This pattern matches the
# IMAGE_PULL_SECRET_ARG approach a few lines down.
#
# Sizing examples (host RAM only — VRAM is governed by GPU_RESOURCE_*):
#   7B LoRA  on GPU : MEMORY_REQUEST=8Gi  MEMORY_LIMIT=24Gi CPU_REQUEST=2 CPU_LIMIT=4
#   13B LoRA on GPU : MEMORY_REQUEST=16Gi MEMORY_LIMIT=48Gi CPU_REQUEST=2 CPU_LIMIT=4
#   7B MLX   on CPU : MEMORY_REQUEST=24Gi MEMORY_LIMIT=64Gi CPU_REQUEST=4 CPU_LIMIT=16
# -----------------------------------------------------------------------------
MEMORY_REQUEST ?=
MEMORY_LIMIT ?=
CPU_REQUEST ?=
CPU_LIMIT ?=

# -----------------------------------------------------------------------------
# Conditional pull-secret injection
#
# Helm rejects an empty entry inside the pullSecrets list (it would render
# a Pod with `imagePullSecrets: [{name: ""}]`, which fails admission). Only
# emit the --set flag when the user actually supplied a Secret name; this
# keeps the default render valid for users on public registries.
# -----------------------------------------------------------------------------
ifneq ($(strip $(IMAGE_PULL_SECRET)),)
IMAGE_PULL_SECRET_ARG := --set image.pullSecrets[0]='$(IMAGE_PULL_SECRET)'
endif

# -----------------------------------------------------------------------------
# Conditional resource overrides
#
# Build up a single $(RESOURCE_OVERRIDES) string that every deploy recipe
# splices in unconditionally. Each branch contributes nothing when its var
# is empty so render output is byte-identical to the previous behaviour
# unless the operator explicitly opts in.
# -----------------------------------------------------------------------------
RESOURCE_OVERRIDES :=
ifneq ($(strip $(MEMORY_REQUEST)),)
RESOURCE_OVERRIDES += --set agent.resources.flockAlliance.requests.memory='$(MEMORY_REQUEST)'
endif
ifneq ($(strip $(MEMORY_LIMIT)),)
RESOURCE_OVERRIDES += --set agent.resources.flockAlliance.limits.memory='$(MEMORY_LIMIT)'
endif
ifneq ($(strip $(CPU_REQUEST)),)
RESOURCE_OVERRIDES += --set agent.resources.flockAlliance.requests.cpu='$(CPU_REQUEST)'
endif
ifneq ($(strip $(CPU_LIMIT)),)
RESOURCE_OVERRIDES += --set agent.resources.flockAlliance.limits.cpu='$(CPU_LIMIT)'
endif

.PHONY: all
all: help

##@ Deployment

# -----------------------------------------------------------------------------
# Public deploy targets
#
# Three public entrypoints with intentionally different authority models:
#
#   make deploy
#       Install the chart on the hub with default values.yaml settings
#       (Manual install strategy, storage.backend=s3, empty chain
#       addresses). Nodes supply TASK_ADDRESS / TOKEN_ADDRESS /
#       BLOCKCHAIN_RPC / PRIVATE_KEY via their per-node `.env`. This is
#       the canonical OCM flow most operators expect:
#           make deploy
#           make enable-addon CLUSTER=m1
#           make enable-addon CLUSTER=m2
#           ...
#
#   make deploy-testnet TASK_ADDRESS=0x... [TOKEN_ADDRESS=0x...]
#       Same layout but the hub centrally pins task (and optionally
#       token) contract addresses so `make update-task` can rotate them
#       without touching every node's `.env`. Use on FLock's public
#       testnet when operators want fleet-wide control of the task.
#
#   make deploy-local-chain-s3 / deploy-local-chain-s3-compatible
#       Hub bootstraps its own FlockToken/FlockTask on a local RPC (and,
#       in the -compatible variant, a local MinIO), then calls the
#       internal `_deploy-local-chain-*` recipes to install the chart
#       with every hub-side address already filled in. For unfunded
#       local-chain smoke tests and fully offline demos.
#
# `_deploy-*` targets are underscored because they assume their inputs
# are already validated by the outer deploy-local-chain-* flows; end
# users should never call the underscored variants directly.
# -----------------------------------------------------------------------------

# -----------------------------------------------------------------------------
# deploy
#
# The bare minimum: install the chart with defaults. Chain addresses and
# PRIVATE_KEY come from each managed cluster's `.env`; the entrypoint
# script's hub-vs-.env precedence rule means operators can later run any
# `deploy-testnet` or `update-task` variant to take hub-side control
# without re-enabling the addon per cluster. Storage backend stays on the
# values.yaml default (`s3`) so FLock's testnet signer-authenticated
# uploads work out of the box.
# -----------------------------------------------------------------------------
.PHONY: deploy
deploy: ## Install the addon control plane on the hub with default values (enable per cluster with `make enable-addon`)
	$(HELM) upgrade --install $(RELEASE_NAME) $(CHART_PATH) \
		--set image.repository=$$(echo $(FLOCK_ALLIANCE_IMAGE) | rev | cut -d: -f2- | rev) \
		--set image.tag=$$(echo $(FLOCK_ALLIANCE_IMAGE) | rev | cut -d: -f1 | rev) \
		--set image.pullPolicy='$(IMAGE_PULL_POLICY)' \
		$(IMAGE_PULL_SECRET_ARG) \
		$(RESOURCE_OVERRIDES)

.PHONY: deploy-testnet
deploy-testnet: ## Deploy addon for testnet mode (usage: make deploy-testnet TASK_ADDRESS=0x... [TOKEN_ADDRESS=0x...])
ifndef TASK_ADDRESS
	$(error TASK_ADDRESS is not set. Usage: make deploy-testnet TASK_ADDRESS=0x... [TOKEN_ADDRESS=0x...])
endif
	$(HELM) upgrade --install $(RELEASE_NAME) $(CHART_PATH) \
		--set addon.installStrategy=Manual \
		--set placement.gpu.enabled=false \
		--set placement.all.enabled=false \
		--set image.repository=$$(echo $(FLOCK_ALLIANCE_IMAGE) | rev | cut -d: -f2- | rev) \
		--set image.tag=$$(echo $(FLOCK_ALLIANCE_IMAGE) | rev | cut -d: -f1 | rev) \
		--set image.pullPolicy='$(IMAGE_PULL_POLICY)' \
		$(IMAGE_PULL_SECRET_ARG) \
		$(RESOURCE_OVERRIDES) \
		--set deploymentConfig.blockchain.tokenAddress='$(TOKEN_ADDRESS)' \
		--set deploymentConfig.blockchain.taskAddress='$(TASK_ADDRESS)' \
		--set deploymentConfig.storage.backend='s3'

.PHONY: _deploy-local-chain-s3-compatible
# -----------------------------------------------------------------------------
# _deploy-local-chain-s3-compatible
#
# The S3 access/secret keys are passed to helm via --set-file (not --set)
# so they never appear in the helm process argv. With --set, the rendered
# command is `helm upgrade ... --set ...accessKey=AKxxx ...secretKey=SKyyy`,
# which leaves both credentials visible to any local user via `ps -ef` or
# `cat /proc/<pid>/cmdline` for the ~5s the helm process runs. With
# --set-file, helm reads the value from a chmod-600 file in a per-recipe
# tempdir, so the only argv leak left is the bucket name (not a credential)
# and the file paths themselves (which contain only random tempdir suffixes).
#
# Tempdir lifecycle:
#   * `mktemp -d` creates a unique dir (e.g. /tmp/tmp.AbC123) with mode 700.
#   * `umask 077` before each `printf > file` ensures the value file is
#     created with mode 600 (rw for owner only).
#   * `trap 'rm -rf "$tmp"' EXIT INT TERM HUP` removes the dir when the
#     recipe shell exits — successfully OR via a kubelet/operator signal —
#     so we never leave plaintext credentials on disk after helm returns.
#   * The trap fires AFTER helm has read the files (helm reads --set-file
#     values synchronously during argument parsing, before any network
#     call), so the cleanup order is safe.
#
# Note: the bucket name remains on `--set` because it is not a secret
# (it is a freshly minted random name like `flock-1234abc` per task), and
# keeping it on the CLI keeps the recipe diffable against the s3 variant.
# -----------------------------------------------------------------------------
_deploy-local-chain-s3-compatible:
ifndef RPC
	$(error RPC is not set. Usage: make _deploy-local-chain-s3-compatible RPC=http://<hub-ip>:8545 [TOKEN_ADDRESS=0x...] [TASK_ADDRESS=0x...])
endif
	@set -eu; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT INT TERM HUP; \
	(umask 077; printf '%s' "$$S3_COMPAT_ACCESS_KEY" > "$$tmp/ak"); \
	(umask 077; printf '%s' "$$S3_COMPAT_SECRET_KEY" > "$$tmp/sk"); \
	$(HELM) upgrade --install $(RELEASE_NAME) $(CHART_PATH) \
		--set addon.installStrategy=Manual \
		--set placement.gpu.enabled=false \
		--set placement.all.enabled=false \
		--set image.repository=$$(echo $(FLOCK_ALLIANCE_IMAGE) | rev | cut -d: -f2- | rev) \
		--set image.tag=$$(echo $(FLOCK_ALLIANCE_IMAGE) | rev | cut -d: -f1 | rev) \
		--set image.pullPolicy='$(IMAGE_PULL_POLICY)' \
		$(IMAGE_PULL_SECRET_ARG) \
		$(RESOURCE_OVERRIDES) \
		--set deploymentConfig.blockchain.rpc='$(RPC)' \
		--set deploymentConfig.blockchain.tokenAddress='$(TOKEN_ADDRESS)' \
		--set deploymentConfig.blockchain.taskAddress='$(TASK_ADDRESS)' \
		--set deploymentConfig.storage.s3Compat.endpointUrl='$(S3_COMPAT_ENDPOINT_URL)' \
		--set deploymentConfig.storage.s3Compat.bucket="$$S3_COMPAT_BUCKET" \
		--set-file deploymentConfig.storage.s3Compat.accessKey="$$tmp/ak" \
		--set-file deploymentConfig.storage.s3Compat.secretKey="$$tmp/sk" \
		--set deploymentConfig.storage.s3Compat.region='$(S3_COMPAT_REGION)' \
		--set deploymentConfig.storage.s3Compat.addressingStyle='$(S3_COMPAT_ADDRESSING_STYLE)' \
		--set deploymentConfig.storage.s3Compat.verifySsl='$(S3_COMPAT_VERIFY_SSL)' \
		--set deploymentConfig.storage.backend='nami'

.PHONY: _deploy-local-chain-s3
_deploy-local-chain-s3:
ifndef RPC
	$(error RPC is not set. Usage: make _deploy-local-chain-s3 RPC=http://<hub-ip>:8545 [TOKEN_ADDRESS=0x...] [TASK_ADDRESS=0x...])
endif
	$(HELM) upgrade --install $(RELEASE_NAME) $(CHART_PATH) \
		--set addon.installStrategy=Manual \
		--set placement.gpu.enabled=false \
		--set placement.all.enabled=false \
		--set image.repository=$$(echo $(FLOCK_ALLIANCE_IMAGE) | rev | cut -d: -f2- | rev) \
		--set image.tag=$$(echo $(FLOCK_ALLIANCE_IMAGE) | rev | cut -d: -f1 | rev) \
		--set image.pullPolicy='$(IMAGE_PULL_POLICY)' \
		$(IMAGE_PULL_SECRET_ARG) \
		$(RESOURCE_OVERRIDES) \
		--set deploymentConfig.blockchain.rpc='$(RPC)' \
		--set deploymentConfig.blockchain.tokenAddress='$(TOKEN_ADDRESS)' \
		--set deploymentConfig.blockchain.taskAddress='$(TASK_ADDRESS)' \
		--set deploymentConfig.storage.backend='s3'

# -----------------------------------------------------------------------------
# Local MinIO lifecycle
#
# The readiness loop below uses `mc ready` (not a fixed sleep) because
# MinIO cold-start time varies wildly between SSD and rotational backing
# stores. `mc ready` polls /minio/health/ready, which only succeeds once
# MinIO has finished initialising disks and elected its quorum — a
# stronger guarantee than /minio/health/live. The 30-second cap is
# generous enough for the slowest disks we have observed in CI but still
# fails fast on misconfig.
#
# Both stdout AND stderr from the per-attempt `mc alias set` and
# `mc ready` calls are sunk to /dev/null. Reason: `mc alias set`
# round-trips to the server to validate credentials, so during the
# sub-second window where the docker port mapping is live but the MinIO
# main process has not finished claiming the listener, mc emits a noisy
# `connection reset by peer` to stderr on every retry. The error is
# benign (the loop catches it and tries again on the next tick), but
# leaking it to the operator's terminal looks like a deploy failure.
# The genuine 30-second timeout branch dumps the container's last 40
# log lines so we lose no diagnostic signal when something is really
# wrong.
# -----------------------------------------------------------------------------

.PHONY: start-local-s3-compatible
start-local-s3-compatible: ## Start a local MinIO instance on the hub and ensure the bucket exists
	@set -euo pipefail; \
	mkdir -p '$(S3_COMPAT_DATA_DIR)'; \
	echo "[flock-addon] ensuring local MinIO container: $(S3_COMPAT_CONTAINER_NAME)"; \
	if $(DOCKER) ps -a --format '{{.Names}}' | grep -qx '$(S3_COMPAT_CONTAINER_NAME)'; then \
		echo "[flock-addon] removing pre-existing container $(S3_COMPAT_CONTAINER_NAME)"; \
		$(DOCKER) rm -f '$(S3_COMPAT_CONTAINER_NAME)' >/dev/null; \
	fi; \
	: 'Source S3 credentials from the recipe-shell environment (set up by the top-of-file `export S3_COMPAT_*` directive) instead of `$(S3_COMPAT_*)` Make-time expansion. Make-time expansion would bake literal values into the bash `-c "<recipe text>"` argv visible via `ps -ef` for the entire ~30s startup + 30-iter readiness loop. Runtime expansion keeps argv clean — secrets only live in envp, which is `0400 owner` under `/proc/<pid>/environ` on Linux. The aliasing onto MINIO_ROOT_USER/MINIO_ROOT_PASSWORD lets the MinIO server container also use docker `-e VAR` (no =value) inheritance, sealing the docker argv channel. The same env vars are also forwarded to the mc containers, where the inner `sh -ec` expands the single-quoted `"$$ACCESS_KEY"` / `"$$SECRET_KEY"` placeholders inside the container — the host-side docker argv only carries the literal $-prefixed names.'; \
	ACCESS_KEY="$$S3_COMPAT_ACCESS_KEY"; \
	SECRET_KEY="$$S3_COMPAT_SECRET_KEY"; \
	BUCKET="$$S3_COMPAT_BUCKET"; \
	MINIO_ROOT_USER="$$ACCESS_KEY"; \
	MINIO_ROOT_PASSWORD="$$SECRET_KEY"; \
	export ACCESS_KEY SECRET_KEY BUCKET MINIO_ROOT_USER MINIO_ROOT_PASSWORD; \
	$(DOCKER_WITH_ENV) run -d \
		--name '$(S3_COMPAT_CONTAINER_NAME)' \
		-p '$(S3_COMPAT_PORT):9000' \
		-p '$(S3_COMPAT_CONSOLE_PORT):9001' \
		-e MINIO_ROOT_USER -e MINIO_ROOT_PASSWORD \
		-v '$(S3_COMPAT_DATA_DIR):/data' \
		'$(MINIO_IMAGE)' server /data --console-address ":9001" >/dev/null; \
	echo "[flock-addon] MinIO listening at http://127.0.0.1:$(S3_COMPAT_PORT) (console http://127.0.0.1:$(S3_COMPAT_CONSOLE_PORT))"; \
	for i in $$(seq 1 30); do \
		if $(DOCKER_WITH_ENV) run --rm --network host -e ACCESS_KEY -e SECRET_KEY --entrypoint /bin/sh '$(MINIO_MC_IMAGE)' -ec \
			'mc alias set local http://127.0.0.1:$(S3_COMPAT_PORT) "$$ACCESS_KEY" "$$SECRET_KEY" >/dev/null 2>&1 && mc ready local >/dev/null 2>&1' >/dev/null 2>&1; then \
			echo "[flock-addon] MinIO ready after $$i second(s)"; \
			break; \
		fi; \
		if [ $$i -eq 30 ]; then \
			echo "[flock-addon] ERROR: MinIO did not become ready within 30s" >&2; \
			$(DOCKER) logs --tail=40 '$(S3_COMPAT_CONTAINER_NAME)' >&2 || true; \
			exit 1; \
		fi; \
		sleep 1; \
	done; \
	$(DOCKER_WITH_ENV) run --rm --network host -e ACCESS_KEY -e SECRET_KEY -e BUCKET --entrypoint /bin/sh '$(MINIO_MC_IMAGE)' -ec '\
		mc alias set local http://127.0.0.1:$(S3_COMPAT_PORT) "$$ACCESS_KEY" "$$SECRET_KEY" >/dev/null && \
		mc mb --ignore-existing "local/$$BUCKET" >/dev/null'; \
	echo "[flock-addon] bucket ensured: $$BUCKET"

.PHONY: stop-local-s3-compatible
stop-local-s3-compatible: ## Stop and remove the local MinIO container (leaves data dir intact)
	@set -eu; \
	if $(DOCKER) ps -a --format '{{.Names}}' | grep -qx '$(S3_COMPAT_CONTAINER_NAME)'; then \
		echo "[flock-addon] removing container $(S3_COMPAT_CONTAINER_NAME)"; \
		$(DOCKER) rm -f '$(S3_COMPAT_CONTAINER_NAME)' >/dev/null; \
	else \
		echo "[flock-addon] no container named $(S3_COMPAT_CONTAINER_NAME); nothing to do"; \
	fi

# -----------------------------------------------------------------------------
# Hub-managed local-chain flows
#
# `make chain` in FL-Alliance-Client deploys FlockToken/FlockTask and writes
# their addresses to data/contracts.json. We parse that JSON with python3
# (already required as a runtime dependency of FL-Alliance-Client) instead
# of jq to avoid pulling another tool onto the hub host.
# -----------------------------------------------------------------------------

.PHONY: deploy-local-chain-s3
deploy-local-chain-s3: ## Auto-start local chain on the Hub using a pre-uploaded MODEL_HASH, then deploy the addon with the original signer-based S3 storage
ifndef FL_ALLIANCE_CLIENT_DIR
	$(error FL_ALLIANCE_CLIENT_DIR is not set. Usage: make deploy-local-chain-s3 FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client MODEL_HASH=<sha256> RPC_HOST=<hub-ip>)
endif
ifndef MODEL_HASH
	$(error MODEL_HASH is not set. Usage: make deploy-local-chain-s3 FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client MODEL_HASH=<sha256> RPC_HOST=<hub-ip>)
endif
ifndef RPC_HOST
	$(error RPC_HOST is not set. Usage: make deploy-local-chain-s3 FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client MODEL_HASH=<sha256> RPC_HOST=<hub-ip>)
endif
	@set -euo pipefail; \
	echo "[flock-addon] MODEL_HASH=$(MODEL_HASH)"; \
	$(MAKE) -C '$(FL_ALLIANCE_CLIENT_DIR)' chain \
		$(if $(strip $(CHAIN_DOCKER_COMPOSE)),DOCKER_COMPOSE='$(CHAIN_DOCKER_COMPOSE)') \
		MODEL_DEFINITION_HASH='$(MODEL_HASH)' \
		$(if $(strip $(ROUNDS)),ROUNDS='$(ROUNDS)') \
		$(if $(strip $(MIN_PARTICIPANTS)),MIN_PARTICIPANTS='$(MIN_PARTICIPANTS)') \
		$(if $(strip $(TIME_TO_PROPOSE)),TIME_TO_PROPOSE='$(TIME_TO_PROPOSE)') \
		$(if $(strip $(TIME_TO_VOTE)),TIME_TO_VOTE='$(TIME_TO_VOTE)'); \
	CONTRACTS='$(FL_ALLIANCE_CLIENT_DIR)/data/contracts.json'; \
	if [ ! -f "$$CONTRACTS" ]; then \
		echo "[flock-addon] ERROR: $$CONTRACTS not found; did 'make chain' fail?" >&2; \
		exit 1; \
	fi; \
	TOKEN_ADDRESS=$$(python3 -c "$$READ_CONTRACT_ADDR" "$$CONTRACTS" FlockToken) || exit $$?; \
	TASK_ADDRESS=$$(python3 -c "$$READ_CONTRACT_ADDR" "$$CONTRACTS" FlockTask) || exit $$?; \
	if [ -z "$$TOKEN_ADDRESS" ] || [ -z "$$TASK_ADDRESS" ]; then \
		echo "[flock-addon] ERROR: FL-Alliance-Client chain deployment did not populate FlockToken.address / FlockTask.address in $$CONTRACTS" >&2; \
		exit 1; \
	fi; \
	echo "[flock-addon] TOKEN_ADDRESS=$$TOKEN_ADDRESS"; \
	echo "[flock-addon] TASK_ADDRESS=$$TASK_ADDRESS"; \
	$(MAKE) _deploy-local-chain-s3 \
		RPC='http://$(RPC_HOST):8545' \
		TOKEN_ADDRESS="$$TOKEN_ADDRESS" \
		TASK_ADDRESS="$$TASK_ADDRESS"; \
	echo ""; \
	echo "============================================================"; \
	echo "[flock-addon] local-chain + signer-S3 deployment complete"; \
	echo "  TASK_ADDRESS  = $$TASK_ADDRESS"; \
	echo "  TOKEN_ADDRESS = $$TOKEN_ADDRESS"; \
	echo "  CHAIN_RPC     = http://$(RPC_HOST):8545"; \
	echo "  STORAGE       = signer-authenticated S3 (FLock testnet)"; \
	echo "  MODEL_HASH    = $(MODEL_HASH)"; \
	echo "============================================================"; \
	echo "Next steps:"; \
	echo "  - Already-enabled clusters pick up the new task/chain config"; \
	echo "    automatically (OCM addon-manager reconciles the new"; \
	echo "    AddOnDeploymentConfig into each cluster's ManifestWork)."; \
	echo "    Verify with:"; \
	echo "       kubectl -n <cluster> get manifestwork addon-flock-addon-deploy-0"; \
	echo "       kubectl --context=<managed> -n flock-system get pod -l app.kubernetes.io/name=flock-addon"; \
	echo "  - For clusters NOT YET enabled, opt them in once:"; \
	echo "       make enable-addon CLUSTER=<name>"; \
	echo "  - Tear everything down when the task is finished:"; \
	echo "       make undeploy-local-chain FL_ALLIANCE_CLIENT_DIR='$(FL_ALLIANCE_CLIENT_DIR)'"; \
	echo "============================================================"

# -----------------------------------------------------------------------------
# deploy-local-chain-s3-compatible
#
# The model's sha256 doubles as the on-chain MODEL_DEFINITION_HASH and as
# the S3 object key. Computing it once locally keeps the upload step and
# the `make chain` step in agreement on the same identifier without an
# extra RPC round-trip. The model directory is bind-mounted read-only so
# `mc cp` cannot accidentally mutate the operator's source artefacts.
# -----------------------------------------------------------------------------

.PHONY: deploy-local-chain-s3-compatible
deploy-local-chain-s3-compatible: ## Auto-start local chain + local S3-compatible storage on the Hub, upload MODEL_ARCHIVE, then deploy the addon
ifndef FL_ALLIANCE_CLIENT_DIR
	$(error FL_ALLIANCE_CLIENT_DIR is not set. Usage: make deploy-local-chain-s3-compatible FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client MODEL_ARCHIVE=/path/to/model.tar.gz RPC_HOST=<hub-ip>)
endif
ifndef MODEL_ARCHIVE
	$(error MODEL_ARCHIVE is not set. Usage: make deploy-local-chain-s3-compatible FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client MODEL_ARCHIVE=/path/to/model.tar.gz RPC_HOST=<hub-ip>)
endif
ifndef RPC_HOST
	$(error RPC_HOST is not set. Usage: make deploy-local-chain-s3-compatible FL_ALLIANCE_CLIENT_DIR=/path/to/FL-Alliance-Client MODEL_ARCHIVE=/path/to/model.tar.gz RPC_HOST=<hub-ip>)
endif
	@set -euo pipefail; \
	if [ ! -f '$(MODEL_ARCHIVE)' ]; then \
		echo "[flock-addon] ERROR: MODEL_ARCHIVE does not exist: $(MODEL_ARCHIVE)" >&2; \
		exit 1; \
	fi; \
	if ! command -v openssl >/dev/null 2>&1; then \
		echo "[flock-addon] ERROR: openssl is required to generate per-task MinIO credentials" >&2; \
		exit 1; \
	fi; \
	if command -v sha256sum >/dev/null 2>&1; then \
		MODEL_HASH=$$(sha256sum '$(MODEL_ARCHIVE)' | cut -d' ' -f1); \
	elif command -v shasum >/dev/null 2>&1; then \
		MODEL_HASH=$$(shasum -a 256 '$(MODEL_ARCHIVE)' | awk '{print $$1}'); \
	else \
		echo "[flock-addon] ERROR: need sha256sum (Linux coreutils) or shasum (macOS / perl-based) on PATH to compute MODEL_HASH" >&2; \
		exit 1; \
	fi; \
	MODEL_DIR=$$(cd "$$(dirname '$(MODEL_ARCHIVE)')" && pwd); \
	MODEL_BASENAME=$$(basename '$(MODEL_ARCHIVE)'); \
	S3_ENDPOINT='http://$(RPC_HOST):$(S3_COMPAT_PORT)'; \
	ACCESS_KEY='$(MINIO_ACCESS_KEY)'; [ -n "$$ACCESS_KEY" ] || ACCESS_KEY=$$(openssl rand -hex 12); \
	SECRET_KEY='$(MINIO_SECRET_KEY)'; [ -n "$$SECRET_KEY" ] || SECRET_KEY=$$(openssl rand -hex 24); \
	BUCKET='$(MINIO_BUCKET)'; [ -n "$$BUCKET" ] || BUCKET=flock-task-$$(echo "$$MODEL_HASH" | cut -c1-12); \
	echo "[flock-addon] MODEL_HASH=$$MODEL_HASH"; \
	echo "[flock-addon] task-scoped bucket: $$BUCKET"; \
	echo "[flock-addon] task-scoped MinIO credentials: ACCESS_KEY=$$ACCESS_KEY (SECRET_KEY hidden; use 'make minio-credentials' to fetch)"; \
	: 'Hand S3 credentials to the sub-make via shell inline-env (`VAR=val cmd args` is POSIX shell syntax — VAR enters cmds envp but never argv). Combined with the top-of-file `export S3_COMPAT_*` directive, sub-make propagates them to its recipe shell, which then references them as `$$S3_COMPAT_*` (bash-runtime expansion, not Make-time expansion). Putting the same overrides AFTER `$(MAKE)` would push them into sub-make argv → `ps -ef` leak for the entire ~30s sub-make lifetime. No `-e` flag needed: `export` already makes envp the source of truth for those names.'; \
	S3_COMPAT_ACCESS_KEY="$$ACCESS_KEY" S3_COMPAT_SECRET_KEY="$$SECRET_KEY" S3_COMPAT_BUCKET="$$BUCKET" \
		$(MAKE) start-local-s3-compatible; \
	echo "[flock-addon] uploading $(MODEL_ARCHIVE) to bucket $$BUCKET with key $$MODEL_HASH"; \
	: 'Pass MinIO credentials by env-inheritance, not by argv. `docker -e VAR` (without =value) tells docker to inherit the named variable from its own environment, so the secret never appears in argv and is therefore invisible to ps auxwwe / /proc/<pid>/cmdline during the few seconds this docker run is alive. The matching `export` makes the recipe-shell-local variables visible to docker; without it, the container would see empty values and mc would fail loudly on its first call (caught failure, not silent regression).'; \
	export MODEL_BASENAME MODEL_HASH BUCKET ACCESS_KEY SECRET_KEY; \
	$(DOCKER_WITH_ENV) run --rm --network host --entrypoint /bin/sh \
		-e MODEL_BASENAME -e MODEL_HASH -e BUCKET -e ACCESS_KEY -e SECRET_KEY \
		-v "$$MODEL_DIR:/upload:ro" '$(MINIO_MC_IMAGE)' -ec '\
		mc alias set local http://127.0.0.1:$(S3_COMPAT_PORT) "$$ACCESS_KEY" "$$SECRET_KEY" >/dev/null && \
		mc cp "/upload/$${MODEL_BASENAME}" "local/$${BUCKET}/$${MODEL_HASH}" >/dev/null'; \
	$(MAKE) -C '$(FL_ALLIANCE_CLIENT_DIR)' chain \
		$(if $(strip $(CHAIN_DOCKER_COMPOSE)),DOCKER_COMPOSE='$(CHAIN_DOCKER_COMPOSE)') \
		MODEL_DEFINITION_HASH="$$MODEL_HASH" \
		$(if $(strip $(ROUNDS)),ROUNDS='$(ROUNDS)') \
		$(if $(strip $(MIN_PARTICIPANTS)),MIN_PARTICIPANTS='$(MIN_PARTICIPANTS)') \
		$(if $(strip $(TIME_TO_PROPOSE)),TIME_TO_PROPOSE='$(TIME_TO_PROPOSE)') \
		$(if $(strip $(TIME_TO_VOTE)),TIME_TO_VOTE='$(TIME_TO_VOTE)'); \
	CONTRACTS='$(FL_ALLIANCE_CLIENT_DIR)/data/contracts.json'; \
	if [ ! -f "$$CONTRACTS" ]; then \
		echo "[flock-addon] ERROR: $$CONTRACTS not found; did 'make chain' fail?" >&2; \
		exit 1; \
	fi; \
	TOKEN_ADDRESS=$$(python3 -c "$$READ_CONTRACT_ADDR" "$$CONTRACTS" FlockToken) || exit $$?; \
	TASK_ADDRESS=$$(python3 -c "$$READ_CONTRACT_ADDR" "$$CONTRACTS" FlockTask) || exit $$?; \
	if [ -z "$$TOKEN_ADDRESS" ] || [ -z "$$TASK_ADDRESS" ]; then \
		echo "[flock-addon] ERROR: FL-Alliance-Client chain deployment did not populate FlockToken.address / FlockTask.address in $$CONTRACTS" >&2; \
		exit 1; \
	fi; \
	echo "[flock-addon] TOKEN_ADDRESS=$$TOKEN_ADDRESS"; \
	echo "[flock-addon] TASK_ADDRESS=$$TASK_ADDRESS"; \
	: 'Same inline-env idiom as the start-local-s3-compatible call above: secret-bearing overrides (BUCKET/ACCESS_KEY/SECRET_KEY) are pre-pended to $(MAKE) so they enter the sub-make environment, not its argv. Non-secret overrides (RPC, TOKEN_ADDRESS, TASK_ADDRESS, ENDPOINT, REGION, ADDRESSING_STYLE, VERIFY_SSL) stay as conventional make variable assignments after `$(MAKE)` to keep `make -n` output diff-friendly. The top-of-file `export S3_COMPAT_*` directive ensures the sub-makes recipe shell inherits these as env vars without needing the `-e` flag.'; \
	S3_COMPAT_BUCKET="$$BUCKET" S3_COMPAT_ACCESS_KEY="$$ACCESS_KEY" S3_COMPAT_SECRET_KEY="$$SECRET_KEY" \
		$(MAKE) _deploy-local-chain-s3-compatible \
			RPC='http://$(RPC_HOST):8545' \
			TOKEN_ADDRESS="$$TOKEN_ADDRESS" \
			TASK_ADDRESS="$$TASK_ADDRESS" \
			S3_COMPAT_ENDPOINT_URL="$$S3_ENDPOINT" \
			S3_COMPAT_REGION='$(S3_COMPAT_REGION)' \
			S3_COMPAT_ADDRESSING_STYLE='$(S3_COMPAT_ADDRESSING_STYLE)' \
			S3_COMPAT_VERIFY_SSL='$(S3_COMPAT_VERIFY_SSL)'; \
	echo ""; \
	echo "============================================================"; \
	echo "[flock-addon] local-chain + S3-compatible deployment complete"; \
	echo "  TASK_ADDRESS  = $$TASK_ADDRESS"; \
	echo "  TOKEN_ADDRESS = $$TOKEN_ADDRESS"; \
	echo "  CHAIN_RPC     = http://$(RPC_HOST):8545"; \
	echo "  S3_ENDPOINT   = $$S3_ENDPOINT"; \
	echo "  S3_BUCKET     = $$BUCKET"; \
	echo "  ACCESS_KEY    = $$ACCESS_KEY"; \
	echo "  SECRET_KEY    = (hidden — run 'make minio-credentials')"; \
	echo "  MODEL_KEY     = $$BUCKET/$$MODEL_HASH"; \
	echo "============================================================"; \
	echo "Next steps:"; \
	echo "  - Already-enabled clusters pick up the new task, chain, bucket,"; \
	echo "    and credentials automatically (OCM addon-manager reconciles"; \
	echo "    the new AddOnDeploymentConfig into each cluster's"; \
	echo "    ManifestWork). Verify with:"; \
	echo "       kubectl -n <cluster> get manifestwork addon-flock-addon-deploy-0"; \
	echo "       kubectl --context=<managed> -n flock-system get pod -l app.kubernetes.io/name=flock-addon"; \
	echo "  - For clusters NOT YET enabled, opt them in once:"; \
	echo "       make enable-addon CLUSTER=<name>"; \
	echo "  - Recover the MinIO credentials at any time:"; \
	echo "       make minio-credentials"; \
	echo "  - Tear everything down when the task is finished:"; \
	echo "       make undeploy-local-chain FL_ALLIANCE_CLIENT_DIR='$(FL_ALLIANCE_CLIENT_DIR)'"; \
	echo "============================================================"

# -----------------------------------------------------------------------------
# Lifecycle helpers
#
# `--reuse-values` keeps every other field at its current rendered value so
# operators can rotate task addresses without re-supplying RPC, image, or
# storage settings. Each managed cluster picks the new value up on its next
# AddOnDeploymentConfig reconcile (see docs/troubleshooting.md for
# force-rollout). OCM's addon-manager owns the per-cluster Namespace and
# Deployment objects via the AddOnTemplate; deleting the hub-side
# ClusterManagementAddOn is what triggers GC on the managed clusters, so
# `helm uninstall` is the correct (and only) way to fully remove the addon.
# -----------------------------------------------------------------------------

.PHONY: update-task
update-task: ## Update the shared task address only (usage: make update-task TASK_ADDRESS=0x...)
ifndef TASK_ADDRESS
	$(error TASK_ADDRESS is not set. Usage: make update-task TASK_ADDRESS=0x...)
endif
	$(HELM) upgrade $(RELEASE_NAME) $(CHART_PATH) \
		--reuse-values \
		--set deploymentConfig.blockchain.taskAddress='$(TASK_ADDRESS)'

.PHONY: undeploy
undeploy: ## Remove the FLock addon Helm release (managed cluster workloads are garbage-collected by OCM)
	@echo "[flock-addon] uninstalling Helm release $(RELEASE_NAME)"
	$(HELM) uninstall $(RELEASE_NAME) || true

.PHONY: undeploy-all
undeploy-all: undeploy stop-local-s3-compatible ## Full local cleanup: helm uninstall + stop local MinIO container

# -----------------------------------------------------------------------------
# Local-chain task helpers
#
# `make deploy-local-chain-s3-compatible` materialises three task-scoped
# pieces of state that the chart's defaults do not encode: the MinIO
# bucket, the access/secret pair, and the FlockToken/FlockTask addresses.
# All three live inside the chart's AddOnDeploymentConfig as plain
# customizedVariable entries because OCM substitutes them into the FL
# client Pod env on every managed cluster — that's also what guarantees
# every cluster sees the same values, which is the foundation of
# multi-client coordination.
#
# `minio-credentials` reads them back out via kubectl jsonpath so an
# operator can drive `mc` from their laptop without re-running the deploy.
#
# `undeploy-local-chain` is the reverse of `deploy-local-chain-*`:
#   1. Read the task's bucket + creds from the live ADC (best effort —
#      may be empty if the chart was never installed).
#   2. Drop the bucket via `mc rb --force` so we don't leak per-task data
#      into /srv/flock-minio when the next task reuses the hub.
#   3. helm uninstall — OCM addon-manager garbage-collects the per-cluster
#      ManifestWork and the FL client Deployments cascade away.
#   4. Stop the MinIO container (idempotent; matches the ephemeral model).
#   5. Optionally stop the chain when FL_ALLIANCE_CLIENT_DIR is provided —
#      we call `make chain-stop` defensively (|| true) because the upstream
#      target name has changed historically and the operator shouldn't have
#      a half-failed undeploy block them from creating the next task.
# -----------------------------------------------------------------------------

.PHONY: minio-credentials
minio-credentials: ## Print the current task's MinIO credentials, bucket, and endpoint URL (read from the live AddOnDeploymentConfig)
	@set -eu; \
	if ! $(KUBECTL) -n $(NAMESPACE) get addondeploymentconfig $(RELEASE_NAME)-config >/dev/null 2>&1; then \
		echo "[flock-addon] ERROR: AddOnDeploymentConfig $(RELEASE_NAME)-config not found in namespace $(NAMESPACE)." >&2; \
		echo "[flock-addon] Hint: run 'make deploy-local-chain-s3-compatible ...' first." >&2; \
		exit 1; \
	fi; \
	get_var() { \
		$(KUBECTL) -n $(NAMESPACE) get addondeploymentconfig $(RELEASE_NAME)-config \
			-o jsonpath="{range .spec.customizedVariables[?(@.name==\"$$1\")]}{.value}{end}"; \
	}; \
	BACKEND=$$(get_var STORAGE_BACKEND); \
	BUCKET=$$(get_var S3_COMPAT_BUCKET); \
	if [ "$$BACKEND" != "nami" ] || [ -z "$$BUCKET" ]; then \
		echo "[flock-addon] No active MinIO-backed task found (STORAGE_BACKEND=$$BACKEND, BUCKET=$$BUCKET)."; \
		echo "[flock-addon] The local-chain MinIO flow is only used by 'make deploy-local-chain-s3-compatible'."; \
		exit 0; \
	fi; \
	ENDPOINT=$$(get_var S3_COMPAT_ENDPOINT_URL); \
	ACCESS_KEY=$$(get_var S3_COMPAT_ACCESS_KEY); \
	SECRET_KEY=$$(get_var S3_COMPAT_SECRET_KEY); \
	echo "ENDPOINT_URL=$$ENDPOINT"; \
	echo "BUCKET=$$BUCKET"; \
	echo "ACCESS_KEY=$$ACCESS_KEY"; \
	echo "SECRET_KEY=$$SECRET_KEY"; \
	echo ""; \
	echo "# Explore the bucket from this hub:"; \
	echo "mc alias set hubminio $$ENDPOINT $$ACCESS_KEY $$SECRET_KEY"; \
	echo "mc ls hubminio/$$BUCKET/"

.PHONY: undeploy-local-chain
undeploy-local-chain: ## Tear down a local-chain task: drop bucket + helm uninstall + stop MinIO + optional chain stop (pass FL_ALLIANCE_CLIENT_DIR=...)
	@set -eu; \
	if $(KUBECTL) -n $(NAMESPACE) get addondeploymentconfig $(RELEASE_NAME)-config >/dev/null 2>&1; then \
		get_var() { \
			$(KUBECTL) -n $(NAMESPACE) get addondeploymentconfig $(RELEASE_NAME)-config \
				-o jsonpath="{range .spec.customizedVariables[?(@.name==\"$$1\")]}{.value}{end}"; \
		}; \
		BACKEND=$$(get_var STORAGE_BACKEND); \
		BUCKET=$$(get_var S3_COMPAT_BUCKET); \
		ACCESS_KEY=$$(get_var S3_COMPAT_ACCESS_KEY); \
		SECRET_KEY=$$(get_var S3_COMPAT_SECRET_KEY); \
		if [ "$$BACKEND" = "nami" ] && [ -n "$$BUCKET" ] && [ -n "$$ACCESS_KEY" ] && [ -n "$$SECRET_KEY" ]; then \
			if $(DOCKER) ps --format '{{.Names}}' | grep -qx '$(S3_COMPAT_CONTAINER_NAME)'; then \
				echo "[flock-addon] removing task bucket: $$BUCKET"; \
				: 'Same env-inheritance pattern as the upload step above: `docker -e VAR` (no =value) keeps ACCESS_KEY/SECRET_KEY out of docker argv, and `export` makes the recipe-shell-local vars visible to docker. See the longer comment around the upload recipe.'; \
				export BUCKET ACCESS_KEY SECRET_KEY; \
				$(DOCKER_WITH_ENV) run --rm --network host --entrypoint /bin/sh \
					-e BUCKET -e ACCESS_KEY -e SECRET_KEY \
					'$(MINIO_MC_IMAGE)' -ec '\
					mc alias set local http://127.0.0.1:$(S3_COMPAT_PORT) "$$ACCESS_KEY" "$$SECRET_KEY" >/dev/null && \
					mc rb --force --dangerous "local/$$BUCKET" >/dev/null 2>&1' \
					|| echo "[flock-addon] WARN: bucket cleanup failed (already gone or mc error); continuing"; \
			else \
				echo "[flock-addon] MinIO container is not running; skipping bucket cleanup"; \
			fi; \
		else \
			echo "[flock-addon] no MinIO task scope detected in ADC; skipping bucket cleanup"; \
		fi; \
	else \
		echo "[flock-addon] AddOnDeploymentConfig not found; skipping bucket cleanup"; \
	fi; \
	echo "[flock-addon] uninstalling Helm release $(RELEASE_NAME)"; \
	$(HELM) uninstall $(RELEASE_NAME) || true; \
	$(MAKE) stop-local-s3-compatible; \
	if [ -n "$(FL_ALLIANCE_CLIENT_DIR)" ]; then \
		if [ -d "$(FL_ALLIANCE_CLIENT_DIR)" ]; then \
			echo "[flock-addon] stopping chain via $(FL_ALLIANCE_CLIENT_DIR)"; \
			if $(MAKE) --no-print-directory -C '$(FL_ALLIANCE_CLIENT_DIR)' --dry-run chain-stop >/dev/null 2>&1; then \
				$(MAKE) -C '$(FL_ALLIANCE_CLIENT_DIR)' chain-stop \
					$(if $(strip $(CHAIN_DOCKER_COMPOSE)),DOCKER_COMPOSE='$(CHAIN_DOCKER_COMPOSE)') \
					|| { echo "[flock-addon] ERROR: 'chain-stop' failed in $(FL_ALLIANCE_CLIENT_DIR) (CHAIN_DOCKER_COMPOSE=$(CHAIN_DOCKER_COMPOSE)); the local chain may still be running. Stop it manually before redeploying or you will hit a port conflict." >&2; exit 1; }; \
			else \
				echo "[flock-addon] WARN: 'chain-stop' target not present in $(FL_ALLIANCE_CLIENT_DIR); stop the chain manually."; \
			fi; \
		else \
			echo "[flock-addon] WARN: FL_ALLIANCE_CLIENT_DIR=$(FL_ALLIANCE_CLIENT_DIR) does not exist; skipping chain stop."; \
		fi; \
	else \
		echo "[flock-addon] FL_ALLIANCE_CLIENT_DIR not set; leaving the chain running. Pass it to also stop the chain."; \
	fi; \
	echo "[flock-addon] local-chain teardown complete"

# -----------------------------------------------------------------------------
# enable-addon
#
# Builds the ManagedClusterAddOn manifest with `printf '%s\n'` rather than
# a heredoc. Heredocs in Make recipes are fragile because each
# backslash-newline turns the recipe into a single shell line: the heredoc
# terminator ends up on the same physical line as the body and the shell
# tries to interpret subsequent words as filenames. `printf` with one line
# per argument keeps every output line independent and pipes cleanly to
# `kubectl apply -f -`. The GPU label dispatch tolerates missing labels so
# unlabelled clusters fall through to the CPU variant.
# -----------------------------------------------------------------------------

.PHONY: enable-addon
enable-addon: ## Enable the addon on a managed cluster; auto-selects GPU/CPU template via managedcluster label `gpu=true` (override with CONFIG=/TEMPLATE=)
ifndef CLUSTER
	$(error CLUSTER is not set. Usage: make enable-addon CLUSTER=cluster1 [CONFIG=flock-addon-config] [TEMPLATE=flock-addon])
endif
	@set -eu; \
	GPU_LABEL=$$($(KUBECTL) get managedcluster $(CLUSTER) -o jsonpath='{.metadata.labels.gpu}' 2>/dev/null || true); \
	if [ -n "$(TEMPLATE)" ]; then \
		TEMPLATE_NAME='$(TEMPLATE)'; \
	elif [ "$$GPU_LABEL" = "true" ]; then \
		TEMPLATE_NAME='$(RELEASE_NAME)-gpu'; \
	else \
		TEMPLATE_NAME='$(RELEASE_NAME)'; \
	fi; \
	if [ -n "$(CONFIG)" ]; then \
		CONFIG_NAME='$(CONFIG)'; \
	elif [ "$$GPU_LABEL" = "true" ]; then \
		CONFIG_NAME='$(RELEASE_NAME)-gpu-config'; \
	else \
		CONFIG_NAME='$(RELEASE_NAME)-config'; \
	fi; \
	echo "[flock-addon] enabling addon on cluster=$(CLUSTER) template=$$TEMPLATE_NAME config=$$CONFIG_NAME"; \
	printf '%s\n' \
		'apiVersion: addon.open-cluster-management.io/v1alpha1' \
		'kind: ManagedClusterAddOn' \
		'metadata:' \
		'  name: $(RELEASE_NAME)' \
		'  namespace: $(CLUSTER)' \
		'spec:' \
		'  configs:' \
		'    - group: addon.open-cluster-management.io' \
		'      resource: addontemplates' \
		"      name: $$TEMPLATE_NAME" \
		'    - group: addon.open-cluster-management.io' \
		'      resource: addondeploymentconfigs' \
		"      name: $$CONFIG_NAME" \
		'      namespace: $(NAMESPACE)' \
		| $(KUBECTL) apply -f -

.PHONY: disable-addon
disable-addon: ## Disable the addon on a managed cluster (usage: make disable-addon CLUSTER=cluster1)
ifndef CLUSTER
	$(error CLUSTER is not set. Usage: make disable-addon CLUSTER=cluster1)
endif
	@echo "[flock-addon] disabling addon on cluster=$(CLUSTER)"
	$(KUBECTL) delete managedclusteraddon $(RELEASE_NAME) -n $(CLUSTER) --ignore-not-found

.PHONY: status
status: ## Print addon-related objects (hub-side)
	$(KUBECTL) get clustermanagementaddons
	$(KUBECTL) get addontemplates
	$(KUBECTL) get addondeploymentconfigs -A
	$(KUBECTL) get managedclusteraddons -A

##@ CI Targets

# -----------------------------------------------------------------------------
# CI contract
#
# The targets in this section satisfy the contract that addon-contrib's CI
# pipeline expects from every addon under this repo: verify, vendor, build,
# test-unit, test-integration, test-e2e, test-chart, image, image-push,
# image-manifest. Most are no-ops for this addon because we ship a Helm
# chart only — the actual container image is built and published from the
# FL-Alliance-Client repository — but the targets MUST exist or CI fails.
# -----------------------------------------------------------------------------

# -----------------------------------------------------------------------------
# verify
#
# This is the addon-contrib CI contract entry point (see
# `.github/workflows/test.yml`, job `reviewable`). That workflow runs four
# separate steps in sequence: `make verify`, `make build`, `make test-unit`,
# `make test-integration`. The runner image for `reviewable` only sets up
# Go and DevSpace — it does not install Helm plugins or bats — so verify
# itself MUST stay tool-light: only `helm` is assumed to be on PATH (it is
# pre-installed on `ubuntu-latest`). Heavier checks (chart unit tests,
# entrypoint bats suites, Makefile secret-leak gate) live behind the
# `test-unit` / `test-integration` steps, each of which auto-installs its
# own runner so a dev cloning the repo can `make test-unit` without first
# learning helm-unittest exists.
#
# Locally `make verify` is still a useful smoke gate — it catches typos in
# values references, missing `if` branches, and chart lint errors before a
# slower test-unit/test-integration run. Developers who want the full
# offline gate in one command can run `make verify-full`.
# -----------------------------------------------------------------------------
.PHONY: verify
verify: lint template ## Tool-light CI gate: helm lint + render both install paths (no plugins required)
	@echo "[flock-addon] verify complete"

.PHONY: verify-full
verify-full: verify test-unit test-integration ## Full offline gate including chart unit tests and entrypoint shell tests (auto-installs helm-unittest and bats if missing)
	@echo "[flock-addon] verify-full complete"

.PHONY: vendor
vendor: ## No-op vendor target (Helm-only addon)
	@echo "[flock-addon] no vendor step required"

.PHONY: build
build: ## No-op build target (Helm-only addon; images are built in source repositories)
	@echo "[flock-addon] no build required"

# -----------------------------------------------------------------------------
# test-unit / test-integration
#
# Chart unit tests run helm-unittest against the rendered manifests so any
# change to a template, value, or fail-guard is caught before deployment.
# Integration tests exercise the embedded entrypoint script with bats,
# extracting it from a fresh `helm template`, replacing `exec` with a
# printf so argv can be captured, and asserting hub-vs-.env precedence,
# storage-backend guards, and the python argv shape — none of which need
# a real cluster.
#
# Both runners — `helm-unittest` (a Helm plugin) and `bats` (a Bash test
# harness) — are NOT pre-installed on the addon-contrib CI runner image
# (`.github/workflows/test.yml` only sets up Go + DevSpace for the
# `reviewable` job and Helm + chart-testing for the `test-chart` job).
# Rather than push the install responsibility into the shared workflow
# (which would couple it to one addon's needs), each test target probes
# for its runner and **auto-installs it on demand** the first time it is
# needed. The auto-install paths are:
#
#   * `helm plugin install https://github.com/helm-unittest/helm-unittest
#     --version 1.0.3` — pinned for reproducibility; no sudo/root required;
#     installs into the user-owned $HELM_PLUGINS dir; idempotent.
#   * `apt-get install -y bats` (Linux/CI) or `brew install bats-core`
#     (macOS dev) — both single-package installs from the OS default repo.
#     If neither package manager is present (e.g. Alpine, Windows WSL
#     without sudo), the recipe prints the install URL and fails so the
#     dev knows exactly what to do.
#
# A note on idempotence: every probe-and-install block here is a no-op if
# the tool is already on PATH / already a registered plugin, so running
# `make test-unit` repeatedly costs only the probe (~10ms) on warm runs.
# -----------------------------------------------------------------------------

.PHONY: test-unit
test-unit: ensure-helm-unittest ## Chart unit tests via helm-unittest (covers all template branches and fail-guards)
	$(HELM) unittest $(CHART_PATH)

.PHONY: test-integration
test-integration: ensure-bats ## Entrypoint + Makefile bats suites (hub-vs-.env precedence, storage guards, argv shape, secret-leak gate)
	@HELM=$(HELM) $(BATS) $(TESTS_ENTRYPOINT_DIR)
	@$(BATS) $(TESTS_MAKEFILE_DIR)

# -----------------------------------------------------------------------------
# test-makefile
#
# Standalone alias for the Makefile secret-leak regression gate. The same
# bats suite runs as part of `test-integration` (see above) so CI picks it
# up automatically; this target exists so a developer can iterate on a
# single suite without paying the entrypoint-suite cost.
#
# Regression invariants enforced:
#   1. Marker test — feeding a canary string into S3_COMPAT_*_KEY and
#      running `make -n` on the leaky recipes never echoes the canary,
#      proving the recipes use bash-runtime expansion (`$$VAR`) and not
#      Make-time expansion (`$(VAR)`) for credentials.
#   2. Shape test — the Makefile source contains no sub-`make` call of
#      the form `$(MAKE) target VAR=val` for any S3_COMPAT_*_KEY/BUCKET,
#      which would put the value into the sub-make argv.
#
# Hermetic (no real cluster, no docker daemon) and fast (~1s).
# -----------------------------------------------------------------------------
.PHONY: test-makefile
test-makefile: ensure-bats ## Makefile secret-leak regression gate (no recipe argv may carry S3 credentials)
	@$(BATS) $(TESTS_MAKEFILE_DIR)

# -----------------------------------------------------------------------------
# ensure-helm-unittest / ensure-bats
#
# Auto-bootstrap helpers for the test runners. Split out as their own
# targets so a developer can pre-warm them with `make ensure-bats` before
# a hack session, and so the install logic lives in one place rather than
# being copy-pasted into every test-* recipe.
#
# Both helpers are silent on the happy path (tool already present) and
# verbose only when they actually have to install something — that gives
# clear CI logs without spamming `[flock-addon] bats already installed`
# on every local re-run.
# -----------------------------------------------------------------------------
.PHONY: ensure-helm-unittest
ensure-helm-unittest: ## Install helm-unittest plugin if missing (idempotent, no sudo)
	@if ! $(HELM) plugin list 2>/dev/null | awk '{print $$1}' | grep -qx unittest; then \
		echo "[flock-addon] helm-unittest plugin not found, installing v1.0.3..." >&2; \
		$(HELM) plugin install https://github.com/helm-unittest/helm-unittest --version 1.0.3 >&2 || { \
			echo "[flock-addon] auto-install failed; install manually:" >&2; \
			echo "    $(HELM) plugin install https://github.com/helm-unittest/helm-unittest" >&2; \
			exit 1; \
		}; \
	fi

.PHONY: ensure-bats
ensure-bats: ## Install bats if missing (apt on Linux/CI, brew on macOS)
	@if ! command -v $(BATS) >/dev/null 2>&1; then \
		echo "[flock-addon] bats not found, attempting to install..." >&2; \
		if command -v apt-get >/dev/null 2>&1; then \
			if [ "$$(id -u)" = "0" ]; then \
				apt-get update -qq && apt-get install -y -qq bats >&2; \
			else \
				sudo -n apt-get update -qq >&2 && sudo -n apt-get install -y -qq bats >&2; \
			fi; \
		elif command -v brew >/dev/null 2>&1; then \
			brew install bats-core >&2; \
		else \
			echo "[flock-addon] no apt-get or brew available; install bats manually:" >&2; \
			echo "    https://github.com/bats-core/bats-core" >&2; \
			exit 1; \
		fi; \
	fi

# e2e tests require a live OCM hub and at least one managed cluster, so
# they live outside the offline `verify` chain. The slot is kept so CI
# can opt in once a kind-based fixture is added.
.PHONY: test-e2e
test-e2e: ## No-op e2e tests (would require a live OCM hub; see tests/README.md for the design)
	@echo "[flock-addon] no e2e tests yet; see tests/README.md"

.PHONY: test-chart
test-chart: lint test-unit ## Helm chart validation used by CI test-chart job (Helm + helm-unittest auto-installed)

.PHONY: image
image: ## No-op image target; images are built in source repositories
	@echo "[flock-addon] use source repositories to build images"

.PHONY: image-push
image-push: ## No-op image push target
	@echo "[flock-addon] use source repositories to push images"

.PHONY: image-manifest
image-manifest: ## No-op multi-arch manifest target
	@echo "[flock-addon] use source repositories to publish manifests"

##@ Development

.PHONY: lint
lint: ## Lint the Helm chart
	$(HELM) lint $(CHART_PATH)

# -----------------------------------------------------------------------------
# template
#
# Render twice: the default (Manual install strategy, no Placements) and
# the Placements path with both placements enabled. Together they exercise
# every conditional branch in the chart, so a template-only smoke test
# catches typos that helm lint alone would miss (e.g. a misnamed value
# referenced only inside `if .Values.placement.gpu.enabled`).
# -----------------------------------------------------------------------------

.PHONY: template
template: ## Render templates locally (useful for diffing across modes)
	@$(HELM) template $(RELEASE_NAME) $(CHART_PATH) >/dev/null
	@echo "[flock-addon] default render OK"
	@$(HELM) template $(RELEASE_NAME) $(CHART_PATH) \
		--set addon.installStrategy=Placements \
		--set placement.gpu.enabled=true \
		--set placement.all.enabled=true >/dev/null
	@echo "[flock-addon] Placements render OK"

##@ Help

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-34s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
