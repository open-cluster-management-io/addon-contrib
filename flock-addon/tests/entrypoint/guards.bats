#!/usr/bin/env bats
# Fail-fast preconditions in the entrypoint script. The chart's helm `fail`
# guards catch most misconfigurations at template time, but two classes of
# bug only become visible at Pod start:
#
#   * STORAGE_BACKEND set to a value not in {s3,local,nami} via a
#     per-cluster ManagedClusterAddOn override (which bypasses the chart).
#   * STORAGE_BACKEND missing entirely because the AddOnDeploymentConfig
#     the cluster references was deleted out from under the addon-manager.
#
# Both must surface as a clear error in the Pod log within the first
# hundred milliseconds. Exit code 2 (not 1) is part of the contract: it
# lets downstream alerting distinguish configuration faults from runtime
# crashes.

load test_helper

@test "unsupported STORAGE_BACKEND exits 2 with a clear error message" {
  STORAGE_BACKEND=garbage \
  run run_entrypoint
  [ "$status" -eq 2 ]
  [[ "$output" == *"ERROR: unsupported STORAGE_BACKEND=garbage"* ]]
}

@test "empty STORAGE_BACKEND exits 2 with a clear error message" {
  STORAGE_BACKEND="" \
  run run_entrypoint
  [ "$status" -eq 2 ]
  [[ "$output" == *"ERROR: STORAGE_BACKEND is empty"* ]]
}

@test "missing TASK_ADDRESS only warns and lets the Pod continue" {
  FLOCK_ALLIANCE_ENV_FILE="" \
  TASK_ADDRESS="" \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"WARN: TASK_ADDRESS is empty"* ]]
}

@test "supported backends do not trip the precondition" {
  for backend in s3 local nami; do
    STORAGE_BACKEND="$backend" \
    run run_entrypoint
    [ "$status" -eq 0 ] || {
      echo "backend=$backend exit=$status output=$output" >&2
      return 1
    }
  done
}
