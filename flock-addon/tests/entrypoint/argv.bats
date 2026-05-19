#!/usr/bin/env bats
# Argv shape of the python invocation. FLockAlliance reads its config from
# config/conf.yaml first and then layers `--override key=value` flags on
# top, so the precise argv is the contract between the entrypoint and the
# downstream YAML. A regression here typically silently fails to apply a
# runtime knob (storage backend, GPU on/off, num_participants) without any
# log line that points at the cause.

load test_helper

@test "always emits the static --override block (mode/gpu/stake/backend/incentive/data)" {
  STORAGE_BACKEND=s3 USE_GPU=false STAKE=0 NO_INCENTIVE=false \
  TASK_ADDRESS=0xT \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--config config/conf.yaml"* ]]
  [[ "$output" == *"--override runtime.mode=local"* ]]
  [[ "$output" == *"--override runtime.gpu=false"* ]]
  [[ "$output" == *"--override blockchain.stake=0"* ]]
  [[ "$output" == *"--override storage.backend=s3"* ]]
  [[ "$output" == *"--override training.no_incentive=false"* ]]
  [[ "$output" == *"--override data.inputs=/data"* ]]
}

@test "appends storage.local.shared_dir only when LOCAL_STORAGE_DIR is set" {
  STORAGE_BACKEND=local LOCAL_STORAGE_DIR=/data/shared \
  TASK_ADDRESS=0xT \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--override storage.local.shared_dir=/data/shared"* ]]
}

@test "omits storage.local.shared_dir when LOCAL_STORAGE_DIR is empty" {
  STORAGE_BACKEND=s3 LOCAL_STORAGE_DIR="" \
  TASK_ADDRESS=0xT \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" != *"--override storage.local.shared_dir"* ]]
}

@test "appends training.num_participants only when NUM_PARTICIPANTS is set" {
  NUM_PARTICIPANTS=4 TASK_ADDRESS=0xT \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--override training.num_participants=4"* ]]
}

@test "GPU variant runtime.gpu=true is forwarded into the argv" {
  USE_GPU=true TASK_ADDRESS=0xT \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--override runtime.gpu=true"* ]]
}

@test "S3-compatible nami fields do not leak into the python argv" {
  # nami fields are consumed by FLockAlliance's storage layer via env vars,
  # not via --override; emitting them as overrides would bypass the YAML
  # validation that the storage layer performs at startup.
  #
  # The negative assertions below target the dotted YAML key shapes the
  # entrypoint could plausibly emit if a future change accidentally piped
  # the S3_COMPAT_* env vars through the same `--override` builder used
  # for blockchain.* and runtime.*. Asserting the env-var-style shape
  # `S3_COMPAT_*` here would be tautological because the entrypoint never
  # uses env-var names as override keys.
  STORAGE_BACKEND=nami \
  S3_COMPAT_ENDPOINT_URL=http://minio:9000 \
  S3_COMPAT_BUCKET=flock-dev \
  S3_COMPAT_ACCESS_KEY=AK \
  S3_COMPAT_SECRET_KEY=SK \
  TASK_ADDRESS=0xT \
  run run_entrypoint
  [ "$status" -eq 0 ]
  # Trailing dot pins the match to nested keys (storage.s3.endpoint_url etc.)
  # so this does not falsely match the legitimate `storage.s3` literal value
  # that storage.backend might carry on the s3 backend.
  [[ "$output" != *"--override storage.s3."* ]]
  [[ "$output" != *"--override storage.s3_compat"* ]]
  [[ "$output" != *"--override storage.nami."* ]]
  [[ "$output" != *"--override storage.endpoint_url"* ]]
  [[ "$output" != *"--override storage.bucket"* ]]
  [[ "$output" != *"--override storage.access_key"* ]]
  [[ "$output" != *"--override storage.secret_key"* ]]
}

@test "emits effective summary lines that downstream debugging relies on" {
  # The "effective:" line redacts sensitive identifiers to <set>/<empty>
  # presence markers via the entrypoint's presence_marker() helper. We
  # assert on the marker shape (TASK_ADDRESS=<set>, BLOCKCHAIN_RPC=<empty>)
  # rather than on the raw values, both because the value is intentionally
  # not emitted and because asserting on the value would re-introduce the
  # cleartext-leak that the helper was added to prevent.
  STORAGE_BACKEND=s3 USE_GPU=false TASK_ADDRESS=0xT \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"effective: STORAGE_BACKEND=s3 USE_GPU=false"* ]]
  [[ "$output" == *"BLOCKCHAIN_RPC=<empty>"* ]]
  [[ "$output" == *"TASK_ADDRESS=<set>"* ]]
  # Negative: the raw value of TASK_ADDRESS must not appear adjacent to
  # the marker (regression guard for the `${VAR:+<set>}${VAR:-<empty>}`
  # bug that concatenated marker + value when VAR was non-empty).
  [[ "$output" != *"TASK_ADDRESS=<set>0xT"* ]]
}

@test "nami backend emits its S3 summary line for troubleshooting" {
  STORAGE_BACKEND=nami \
  S3_COMPAT_ENDPOINT_URL=http://minio:9000 \
  S3_COMPAT_BUCKET=flock-dev \
  S3_COMPAT_REGION=us-east-1 \
  TASK_ADDRESS=0xT \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"effective: S3_COMPAT_ENDPOINT_URL=http://minio:9000 S3_COMPAT_BUCKET=flock-dev S3_COMPAT_REGION=us-east-1"* ]]
}

@test "no .env file path skips the env load step entirely" {
  FLOCK_ALLIANCE_ENV_FILE="" \
  TASK_ADDRESS=0xT \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"FLOCK_ALLIANCE_ENV_FILE is empty; skipping node env load"* ]]
}
