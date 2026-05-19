#!/usr/bin/env bats
# Hub-vs-.env precedence rule, exercised end-to-end against the real
# entrypoint script that the chart embeds in the AddOnTemplate.
#
# Why bats and not helm-unittest: helm-unittest can prove the script's
# *source* mentions ocm_wins, but only an executor can prove the rule
# behaves correctly when both sides set the same variable. Each test
# case here mirrors a deployment scenario described in
# docs/configuration-and-overrides.md (#effective-authority-rules).

load test_helper

@test "hub TASK_ADDRESS overrides .env when both are set" {
  TASK_ADDRESS=0xHUB_TASK \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--override blockchain.task_address=0xHUB_TASK"* ]]
  [[ "$output" != *"--override blockchain.task_address=0xENV_TASK"* ]]
}

@test ".env TASK_ADDRESS wins when the hub leaves it empty" {
  TASK_ADDRESS="" \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--override blockchain.task_address=0xENV_TASK"* ]]
}

@test "hub BLOCKCHAIN_RPC overrides .env RPC when both are set" {
  BLOCKCHAIN_RPC=http://hub-rpc:8545 \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--override blockchain.rpc=http://hub-rpc:8545"* ]]
  [[ "$output" != *"--override blockchain.rpc=http://node-env-rpc:8545"* ]]
}

@test ".env BLOCKCHAIN_RPC wins when the hub leaves it empty (testnet flow)" {
  BLOCKCHAIN_RPC="" \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--override blockchain.rpc=http://node-env-rpc:8545"* ]]
}

@test "hub TOKEN_ADDRESS overrides .env TOKEN when both are set" {
  TOKEN_ADDRESS=0xHUB_TOKEN \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--override blockchain.token_address=0xHUB_TOKEN"* ]]
  [[ "$output" != *"--override blockchain.token_address=0xENV_TOKEN"* ]]
}

@test "no .env file present and hub silent yields no token/rpc/task overrides" {
  FLOCK_ALLIANCE_ENV_FILE="" \
  TASK_ADDRESS="" TOKEN_ADDRESS="" BLOCKCHAIN_RPC="" \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" != *"--override blockchain.task_address="* ]]
  [[ "$output" != *"--override blockchain.token_address="* ]]
  [[ "$output" != *"--override blockchain.rpc="* ]]
  [[ "$output" == *"WARN: TASK_ADDRESS is empty"* ]]
}

@test "missing .env path is logged but does not abort the entrypoint" {
  FLOCK_ALLIANCE_ENV_FILE=/nonexistent/path/.env \
  TASK_ADDRESS=0xHUB_TASK \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"env file not found:"* ]]
  [[ "$output" == *"--override blockchain.task_address=0xHUB_TASK"* ]]
}

@test "STORAGE_BACKEND from the hub wins over a stale value in .env" {
  STORAGE_BACKEND=nami \
  run run_entrypoint
  [ "$status" -eq 0 ]
  [[ "$output" == *"--override storage.backend=nami"* ]]
  [[ "$output" == *"effective: STORAGE_BACKEND=nami"* ]]
}
