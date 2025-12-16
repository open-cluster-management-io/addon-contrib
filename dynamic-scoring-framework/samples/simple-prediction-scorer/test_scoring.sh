#!/bin/bash

curl -X POST "http://localhost:8444/scoring" \
  -H "Content-Type: application/json" \
  -d @sample_cpu_load.json