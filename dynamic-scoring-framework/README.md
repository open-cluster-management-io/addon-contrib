# ocm-dynamic-scoring

DynamicScoring Framework is a framework for automating resource scoring in multi-cluster environments using Prometheus metrics.  
It calculates dynamic scores for each cluster and provides foundational information for resource optimization and automated control.

---

## Features

- **Multi-cluster support**: Automates scoring across multiple clusters using OCM (Open Cluster Management)
- **Flexible Scoring API integration**: Register and use any scoring API
- **Prometheus integration**: Collects metrics from each cluster's Prometheus for scoring
- **Extensible**: CRD-based configuration and management, supports external APIs and authentication

---

## Architecture Overview

1. **DynamicScorer**  
   Registers scoring API information as a CRD

2. **DynamicScoringConfig**  
   Aggregates registered DynamicScorers and distributes them as ConfigMaps to each cluster

3. **DynamicScoringAgent**  
   Watches ConfigMaps in each cluster, fetches metrics from Prometheus, calls the scoring API, and exports results as metrics

For more details, refer to the [design document](docs/concept_and_design.md).

---

## Development Environment Setup

Please refer to the [development guide](docs/development.md) for instructions on setting up your local development environment.

## Directory Structure

- `api/`: API definitions for CRDs
- `internal/controller/`: Controllers for managing CRDs (DynamicScorer, DynamicScoringConfig)
- `config/`: Configuration files for deploying the operator
- `docs/`: Documentation files
- `pkg/`: Package source code. It contains the common and addon agents implementation.
  - `pkg/common/`: Common utilities and types used across the project.
  - `pkg/dynamic_scoring/`: Implementation of the DynamicScoring addon controller in hub cluster.
  - `pkg/dynamic_scoring_agent/`: Implementation of the DynamicScoringAgent that runs in managed clusters.
- `samples/`: Sample CRDs and Scoring APIs for testing and demonstration
