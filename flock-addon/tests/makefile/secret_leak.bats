#!/usr/bin/env bats
# ============================================================================
# Regression gate: S3 credentials must never enter recipe argv.
#
# Threat model (shared-host hub, Linux):
#   `ps -ef` and `cat /proc/<pid>/cmdline` are world-readable for every
#   process owned by any user. Any value that lands in argv of `make`,
#   `bash -c "<recipe text>"`, `docker run`, or `helm` is therefore visible
#   to other local users for the lifetime of that process.
#
# Two complementary checks are enforced here:
#
#   1. Marker test (dynamic). Feed a unique canary string into S3_COMPAT_*
#      and run `make -n` on the credential-touching recipes. The canary
#      must NOT appear in the dry-run output. Appearance means the recipe
#      uses `$(VAR)` Make-time expansion, which bakes the literal value
#      into the bash `-c "<recipe text>"` argv — leaking it via `ps -ef`
#      for the entire ~30s `start-local-s3-compatible` recipe lifetime.
#      The required form is `"$$VAR"` (bash-runtime expansion).
#
#   2. Shape test (static). Grep the Makefile source itself for sub-`make`
#      calls of the leaky shape `$(MAKE) target VAR=val` where VAR is a
#      credential. Such calls put VAR=val into the sub-make's argv,
#      leaking via `ps -ef` for the entire sub-make lifetime. The required
#      shape is `VAR=val $(MAKE) target` (POSIX inline-env), which puts
#      VAR into envp instead of argv. The marker test cannot catch this
#      regression directly because the sub-make's argv is one level removed
#      from the parent's `make -n` output.
#
# Out of scope (tracked as follow-up):
#   - Moving credentials into a Kubernetes Secret with `secretRef` in the
#     chart (this would also remove the `AddOnDeploymentConfig`-as-CR
#     exposure documented in `docs/deployment-modes.md` Security section).
#     That is a larger redesign of the chart values contract and is out
#     of scope for this regression gate, which guards only the local
#     `ps -ef` / argv channels around the Makefile recipes.
#
# Note: `helm upgrade --set ...secretKey=<value>` USED to leak via helm's
# own argv. The `_deploy-local-chain-s3-compatible` recipe now passes
# accessKey/secretKey via `--set-file` (chmod 600 file in a `trap`-cleaned
# tempdir), so helm's argv carries only file PATHS, not credential values.
# The shape test below pins this idiom in place.
# ============================================================================

setup() {
  # Distinct, non-overlapping markers so a substring match cannot mask a
  # leak. Hex-only so they collide with no Make/shell metacharacter and
  # so a maintainer reading them in test output recognises the source.
  LEAK_AK='SECRETCANARYAKID00b8f3e9a1c0d4e5f6'
  LEAK_SK='SECRETCANARYSKID00d7c2f4b8a9e1f0c2'
  REPO_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
}

# ----------------------------------------------------------------------------
# Marker tests
# ----------------------------------------------------------------------------

@test "marker: S3_COMPAT_*_KEY values never appear in start-local-s3-compatible recipe text" {
  cd "$REPO_ROOT"
  run make -n start-local-s3-compatible \
    S3_COMPAT_ACCESS_KEY="$LEAK_AK" \
    S3_COMPAT_SECRET_KEY="$LEAK_SK" \
    S3_COMPAT_BUCKET=secret-leak-test-bucket
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  if echo "$output" | grep -qF "$LEAK_AK"; then
    echo "ACCESS_KEY value leaked into recipe text:"
    echo "$output" | grep -F "$LEAK_AK"
    false
  fi
  if echo "$output" | grep -qF "$LEAK_SK"; then
    echo "SECRET_KEY value leaked into recipe text:"
    echo "$output" | grep -F "$LEAK_SK"
    false
  fi
}

@test "marker: S3_COMPAT_*_KEY values never appear in _deploy-local-chain-s3-compatible recipe text" {
  cd "$REPO_ROOT"
  run make -n _deploy-local-chain-s3-compatible \
    RPC=http://secret-leak-test:8545 \
    S3_COMPAT_ACCESS_KEY="$LEAK_AK" \
    S3_COMPAT_SECRET_KEY="$LEAK_SK" \
    S3_COMPAT_BUCKET=secret-leak-test-bucket
  [ "$status" -eq 0 ] || { echo "$output"; false; }
  if echo "$output" | grep -qF "$LEAK_AK"; then
    echo "ACCESS_KEY value leaked into recipe text:"
    echo "$output" | grep -F "$LEAK_AK"
    false
  fi
  if echo "$output" | grep -qF "$LEAK_SK"; then
    echo "SECRET_KEY value leaked into recipe text:"
    echo "$output" | grep -F "$LEAK_SK"
    false
  fi
}

# ----------------------------------------------------------------------------
# Shape test
# ----------------------------------------------------------------------------

@test "shape: credential-bearing sub-make calls use inline-env (VAR=val \$(MAKE) target), not argv-override (\$(MAKE) target VAR=val)" {
  cd "$REPO_ROOT"
  # Collapse `\\\n` line continuations so each logical recipe line is one
  # physical line; then a per-line grep can look for `$(MAKE) ... VAR=`
  # without false negatives from continuations splitting the inline-env
  # tokens away from the `$(MAKE)` token.
  joined=$(awk '
    /\\$/ { sub(/\\$/, ""); buf = buf $0; next }
          { buf = buf $0; print buf; buf = "" }
    END   { if (buf != "") print buf }
  ' Makefile)
  leaks=$(printf '%s\n' "$joined" \
    | grep -E '\$\(MAKE\)[^;#]*\bS3_COMPAT_(ACCESS_KEY|SECRET_KEY|BUCKET)=' \
    || true)
  [ -z "$leaks" ] || {
    echo "Leaky sub-make shape detected (S3_COMPAT_* override appears AFTER \$(MAKE)):"
    echo "$leaks"
    echo
    echo "Required shape: 'S3_COMPAT_X=\$\$VAL \$(MAKE) target ...' (inline-env, BEFORE make)."
    echo "Argv-override puts the value into sub-make argv, visible via 'ps -ef' for the sub-make lifetime."
    false
  }
}

# ----------------------------------------------------------------------------
# Sanity: the export directive that makes the runtime form work is in place.
# Without it, `make X=Y target` standalone would NOT propagate X to the
# recipe shells envp, breaking the `"$$X"` expansion silently (recipe sees
# empty value, mc fails with InvalidAccessKeyId — caught failure, but
# noisy and ambiguous). This sanity check protects against accidental
# removal of the export directive during refactors.
# ----------------------------------------------------------------------------

@test "sanity: Makefile exports S3_COMPAT_ACCESS_KEY/SECRET_KEY/BUCKET to recipe shells" {
  cd "$REPO_ROOT"
  grep -qE '^export[[:space:]].*\bS3_COMPAT_ACCESS_KEY\b' Makefile \
    || { echo "Missing 'export S3_COMPAT_ACCESS_KEY' directive in Makefile"; false; }
  grep -qE '^export[[:space:]].*\bS3_COMPAT_SECRET_KEY\b' Makefile \
    || { echo "Missing 'export S3_COMPAT_SECRET_KEY' directive in Makefile"; false; }
  grep -qE '^export[[:space:]].*\bS3_COMPAT_BUCKET\b' Makefile \
    || { echo "Missing 'export S3_COMPAT_BUCKET' directive in Makefile"; false; }
}

# ----------------------------------------------------------------------------
# Shape test: helm credential idiom
#
# A regression that re-introduces `--set ...accessKey="$$S3_COMPAT_ACCESS_KEY"`
# (or the secretKey equivalent) would NOT be caught by the marker test
# above, because make -n does not run bash, so `$$VAR` in recipe text
# expands to the literal `$VAR` string and the canary never appears in
# the dry-run output. The leak in that scenario surfaces only at runtime,
# inside helm's own argv, where `ps -ef` can read it for the lifetime of
# the helm process (~5s).
#
# This test pins the safer idiom in source: any --set token referencing
# accessKey or secretKey is a regression. The required form is
# `--set-file deploymentConfig.storage.s3Compat.accessKey=$$tmp/ak`,
# which puts a file path (not the credential value) into helm's argv.
# ----------------------------------------------------------------------------
@test "shape: helm never receives S3 access/secret keys via --set (must use --set-file)" {
  cd "$REPO_ROOT"
  joined=$(awk '
    /\\$/ { sub(/\\$/, ""); buf = buf $0; next }
          { buf = buf $0; print buf; buf = "" }
    END   { if (buf != "") print buf }
  ' Makefile)
  leaks=$(printf '%s\n' "$joined" \
    | grep -E -- '--set[[:space:]]+[^[:space:]]*\.s3Compat\.(accessKey|secretKey)=' \
    || true)
  [ -z "$leaks" ] || {
    echo "Leaky helm argv shape detected (--set used for an S3 credential field):"
    echo "$leaks"
    echo
    echo "Required shape: '--set-file deploymentConfig.storage.s3Compat.accessKey=\$\$tmp/ak'"
    echo "with the file written under chmod 600 in a 'trap rm -rf' tempdir."
    false
  }
}
