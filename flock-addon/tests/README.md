# flock-addon Tests

Three offline test suites guard the addon against regression. All run on a
laptop without any cluster and are wired into the `test-unit` /
`test-integration` make targets that addon-contrib CI calls directly.

## Layout

```text
flock-addon/
├── charts/flock-addon/tests/      # chart unit tests (helm-unittest)
│   ├── addon-deployment-config_test.yaml
│   ├── addon-template_test.yaml
│   ├── cluster-management-addon_test.yaml
│   ├── guards_test.yaml
│   └── placement_test.yaml
└── tests/
    ├── entrypoint/                # entrypoint shell tests (bats)
    │   ├── argv.bats
    │   ├── extract-entrypoint.sh
    │   ├── guards.bats
    │   ├── precedence.bats
    │   └── test_helper.bash
    └── makefile/                  # Makefile secret-leak regression gate (bats)
        └── secret_leak.bats
```

## Chart unit tests — what they prove

`helm-unittest` renders the chart with controlled value overrides and
asserts on the resulting Kubernetes object tree. The suites cover the
fields that are easy to break and hard to debug from a Pod log:

| Suite                                      | Coverage |
| ------------------------------------------ | --- |
| `cluster-management-addon_test.yaml`       | Lifecycle annotation, supportedConfig defaults, Manual vs Placements installStrategy, per-placement config refs (CPU vs GPU template/AddOnDeploymentConfig). |
| `guards_test.yaml`                         | The three `fail` guards in `cluster-management-addon.yaml` (bad strategy, bad backend, Placements without any enabled placement) plus the positive cases. |
| `addon-deployment-config_test.yaml`        | Two-document render, USE_GPU is the only divergent field, every blockchain/storage/S3-compat customizedVariable forwards verbatim, FLOCK_ALLIANCE_IMAGE renders as `repo:tag`. |
| `addon-template_test.yaml`                 | CPU vs GPU variant naming and labels, `nvidia.com/gpu` only on GPU variant, `Recreate` strategy, `terminationGracePeriodSeconds: 30`, `revisionHistoryLimit: 3`, volume precedence (PVC → hostPath → emptyDir), `imagePullSecrets` omitted when empty, env list contains the OCM placeholder values, entrypoint script contains the `ocm_wins` precedence machinery and the `STORAGE_BACKEND is empty` guard. |
| `placement_test.yaml`                      | No output when neither placement is enabled, single placement + binding when only one is enabled, the `gpu=true NotIn` exclusion appears on the `all` placement when both are enabled (preventing both placements from binding the same gpu cluster), custom ClusterSet names propagate. |

Run them directly:

```bash
make test-unit
# or
helm unittest charts/flock-addon
```

## Entrypoint shell tests — what they prove

`bats` exercises the actual `/bin/sh -ec` script the chart embeds in the
`AddOnTemplate`. Test setup pulls the script out of a fresh `helm template`
render via `extract-entrypoint.sh`, swaps the final `exec "$@"` for a
`printf 'CMD: ...'` so the python argv can be captured, then drives the
script with the same env-var contract OCM uses at runtime.

| Suite               | Coverage |
| ------------------- | --- |
| `precedence.bats`   | The hub-vs-`.env` authority rule end to end: hub non-empty wins for `TASK_ADDRESS`, `TOKEN_ADDRESS`, `BLOCKCHAIN_RPC`, `STORAGE_BACKEND`; `.env` wins when hub is silent; missing `.env` is a warning, not an error; missing path with hub silent leaves all overrides absent. |
| `guards.bats`       | `STORAGE_BACKEND` validation: invalid value or empty exits 2 with a clear error; `s3`, `local`, `nami` all pass; missing `TASK_ADDRESS` is only a warning. |
| `argv.bats`         | Exact python argv shape: the always-present `--override` block, conditional appends for `LOCAL_STORAGE_DIR` / `NUM_PARTICIPANTS`, GPU variant forwarding, S3-compat fields stay out of `--override` (they are env-only), and the `effective:` debugging summary lines that the troubleshooting guide depends on. |

Run them directly:

```bash
make test-integration
# or
HELM=helm bats tests/entrypoint
```

## Why these layers and not others

* **Chart unit tests vs `helm template`.** A bare `helm template` smoke
  test only proves rendering does not error. The unit suites assert on
  the exact field values that the API server, OCM addon-manager and the
  managed-cluster kubelet care about. A regression that changes
  `terminationGracePeriodSeconds` from 30 to 0 would still pass `helm
  template` but is caught immediately here.
* **Entrypoint shell tests vs Python e2e.** The hub-vs-`.env` precedence
  rule lives in shell, not in FLockAlliance, so a Python e2e would test
  the wrong layer. Driving the real script with controlled env input is
  the cheapest way to keep the documented authority rule honest. The
  tests run in under two seconds and need neither python nor a cluster.
* **Makefile secret-leak gate.** A bats suite under `tests/makefile/`
  asserts that no S3 credential ever lands in a recipe's argv (neither
  via Make-time `$(VAR)` expansion nor via sub-`make ... VAR=val` overrides).
  It runs `make -n` and greps the recipe text, so it is hermetic and
  catches regressions of the `ps`-leak class before they ship.
* **No cluster e2e yet.** A future kind+OCM e2e would extend the matrix
  to deployment, label-based GPU/CPU dispatch and `make update-task`
  rollout. It is intentionally out of the offline gate because the full
  fixture takes minutes to bring up; `make test-e2e` is reserved for it.

## CI hook

addon-contrib's shared CI workflow (`.github/workflows/test.yml`) calls
four make targets in sequence on the `reviewable` job:

```text
make verify          # lint + template render (helm-only, no plugins)
make build           # no-op for this Helm-only addon
make test-unit       # chart unit tests; auto-installs helm-unittest if missing
make test-integration # entrypoint + Makefile bats suites; auto-installs bats if missing
```

Both `test-unit` and `test-integration` probe for their runner and
install it on demand, so the workflow does not need addon-specific setup
steps. `make test-chart` (called by the chart-test job) runs
`lint + test-unit`, reusing the same auto-install path. Locally, the
shortcut `make verify-full` chains all of the above for a one-shot
offline gate.
