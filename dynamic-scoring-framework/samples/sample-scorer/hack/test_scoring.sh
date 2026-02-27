#!/bin/bash

SCORING_HOST=$1
DATA_FILE=$2

# Fetch the scoring path from the /config endpoint
SCORING_PATH=$(curl -sS "$SCORING_HOST/config" -H "Content-Type: application/json" | jq  -r '.scoring.path')

# Send a POST request to the scoring endpoint with the provided data file
curl -sS -X POST "$SCORING_HOST$SCORING_PATH" \
  -H "Content-Type: application/json" \
  -d @$DATA_FILE| jq .