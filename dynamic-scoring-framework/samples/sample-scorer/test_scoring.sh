#!/bin/bash

curl -X POST "http://localhost:8000/scoring" \
  -H "Content-Type: application/json" \
  -d @data.json