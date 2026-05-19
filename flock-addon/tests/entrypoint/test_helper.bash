#!/usr/bin/env bash
# Shared bats fixtures for the entrypoint suites.
#
# The fixtures perform two jobs:
#
#   1. Extract a fresh copy of the embedded entrypoint script for each test
#      run. Doing this in setup_file (not setup) keeps `helm template` to a
#      single invocation per file, which is what dominates wall-clock time.
#   2. Substitute the final `exec "$@"` for `printf 'CMD: %s\n' "$@"` so the
#      test can capture the assembled python argv without actually trying
#      to run python (which is not available in the addon image used in CI
#      and would mask precedence bugs as ImagePullBackOff later anyway).
#
# Using `printf` (not `echo`) preserves embedded spaces in argv elements so
# the assertions below can inspect the exact argv shape, including override
# values that contain `=` and slashes.

THIS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENTRYPOINT_SRC="${BATS_FILE_TMPDIR:-/tmp}/entrypoint.sh"
ENTRYPOINT_TEST="${BATS_FILE_TMPDIR:-/tmp}/entrypoint_test.sh"
ENV_FILE="${BATS_FILE_TMPDIR:-/tmp}/node.env"

setup_file() {
  # Fixtures generated here are required by every test in the file, so any
  # failure must abort setup_file rather than be hidden behind a later
  # cryptic "command not found" or "no such file" inside an individual
  # test. `set -e` triggers the abort on the first failing command and the
  # explicit `|| return 1` on each fixture step keeps the failure mode
  # working even on shells that disable -e inside a function.
  set -e
  HELM="${HELM:-helm}" "$THIS_DIR/extract-entrypoint.sh" cpu "$ENTRYPOINT_SRC" || return 1
  awk '
    /^exec "\$@"$/ { print "printf '\''CMD:'\''; for a in \"$@\"; do printf '\'' %s'\'' \"$a\"; done; printf '\''\\n'\''"; next }
    { print }
  ' "$ENTRYPOINT_SRC" > "$ENTRYPOINT_TEST" || return 1
  cat > "$ENV_FILE" <<'EOF' || return 1
PRIVATE_KEY=0xnodeprivatekey
HF_TOKEN=hf_node
TASK_ADDRESS=0xENV_TASK
TOKEN_ADDRESS=0xENV_TOKEN
BLOCKCHAIN_RPC=http://node-env-rpc:8545
EOF
}

# run_entrypoint sets the OCM-injected env vars to whatever the caller
# overrides via local exports and then runs the doctored script under sh.
# Every variable in the AddOnDeploymentConfig customizedVariables list is
# present here as an empty string default so tests can opt in to setting
# only the fields they care about — matching the shape OCM actually
# delivers, where every customizedVariable is exported even when blank.
#
# We use `${VAR-default}` (no colon) instead of `${VAR:-default}` so a
# caller that exports `VAR=""` keeps the empty value: that distinction
# matters for the empty-STORAGE_BACKEND guard test which deliberately
# probes the script's behaviour when OCM delivers an empty placeholder.
run_entrypoint() {
  USE_GPU="${USE_GPU-false}" \
  STORAGE_BACKEND="${STORAGE_BACKEND-s3}" \
  NO_INCENTIVE="${NO_INCENTIVE-false}" \
  NUM_PARTICIPANTS="${NUM_PARTICIPANTS-1}" \
  STAKE="${STAKE-0}" \
  DATA_PATH="${DATA_PATH-/data}" \
  TASK_ADDRESS="${TASK_ADDRESS-}" \
  TOKEN_ADDRESS="${TOKEN_ADDRESS-}" \
  BLOCKCHAIN_RPC="${BLOCKCHAIN_RPC-}" \
  LOCAL_STORAGE_DIR="${LOCAL_STORAGE_DIR-}" \
  S3_COMPAT_ENDPOINT_URL="${S3_COMPAT_ENDPOINT_URL-}" \
  S3_COMPAT_BUCKET="${S3_COMPAT_BUCKET-}" \
  S3_COMPAT_ACCESS_KEY="${S3_COMPAT_ACCESS_KEY-}" \
  S3_COMPAT_SECRET_KEY="${S3_COMPAT_SECRET_KEY-}" \
  S3_COMPAT_REGION="${S3_COMPAT_REGION-}" \
  S3_COMPAT_ADDRESSING_STYLE="${S3_COMPAT_ADDRESSING_STYLE-}" \
  S3_COMPAT_VERIFY_SSL="${S3_COMPAT_VERIFY_SSL-}" \
  FLOCK_ALLIANCE_ENV_FILE="${FLOCK_ALLIANCE_ENV_FILE-$ENV_FILE}" \
    sh "$ENTRYPOINT_TEST"
}

# cmd_line greps the captured `CMD: ...` line from the entrypoint output.
cmd_line() {
  printf '%s\n' "$1" | awk '/^CMD:/ {print; exit}'
}
