# Addon-Contrib

This repository hosts a collection of Open Cluster Management (OCM) addons for staging and testing purposes.

## Overview

OCM is a project that provides a unified way to manage multiple Kubernetes clusters.
This repository is intended for the development, staging, and testing of various OCM addons.

Addons in this repository are designed to extend the capabilities of OCM deployments,
providing specialized functionality for workload scheduling, application deployment, observability, 
device management, data orchestration, and federated learning across multiple clusters.

## Available Addons

This repository contains the following OCM addons:

- **argocd-agent-addon**: Integrates Argo CD Agent for highly scalable application deployment across managed clusters
- **clusternet-addon**: Provides Clusternet integration for enhanced cluster networking capabilities
- **device-addon**: Enables device management functionality within the OCM ecosystem
- **dynamic-scoring-framework**: A framework for distributed evaluation across multiclusters and centralized aggregation
- **federated-learning-controller**: Implements federated learning capabilities across distributed clusters
- **fluid-addon**: Integrates Fluid for data orchestration and management in multicluster environments
- **hellospoke-addon**: A simple example addon demonstrating basic OCM addon development patterns
- **kueue-addon**: Integrates Kueue for advanced multicluster batch job scheduling and queue management
- **open-telemetry-addon**: Deploys OpenTelemetry collectors for comprehensive observability and metrics collection
- **resource-usage-collect-addon**: Collects and aggregates resource usage metrics across managed clusters

Each addon directory contains its own README with specific installation and usage instructions.

## Onboarding a New Project

To onboard a new addon-contrib project:

1. If not already discussed with maintainers, open an issue to propose your idea.
1. Once acknowledged, create a folder named after your project and add your code/docs.
1. Add an `OWNERS` file listing the new project's maintainers.
1. **Register your project** in `.github/repositories.json` (see [Repository Registration](#repository-registration) below).
1. **Ensure CI/CD compliance** by implementing required structure and targets (see [GitHub Actions Requirements](#github-actions-requirements) below).
1. Create a PR with a brief project overview and confirm all requirements are met.
1. An OCM maintainer will review and merge the PR.

### GitHub Actions Requirements

All projects must follow certain conventions to ensure compatibility with the addon-contrib repository's Github Actions workflows.

Refer to the [Test](./.github/workflows/test.yml) and [E2E](./.github/workflows/e2e.yml) workflows for exact details.

#### Required `make` Targets

All projects **must** define the following `make` targets in their `Makefile`:

| Target | Description | Can be Stub? |
|--------|-------------|--------------|
| `verify` | Code verification (linting, formatting) | No - should run actual checks |
| `vendor` | Update Go module dependencies | No - if Go project |
| `build` | Build the application binary | No - if Go project |
| `test-unit` | Run unit tests | No - should run actual tests |
| `test-integration` | Run integration tests | **Yes** - can return true if not implemented |
| `test-e2e` | Run end-to-end tests | **Yes** - can return true if not implemented |
| `test-chart` | Test Helm chart installation | **Yes** - can return true if no chart |
| `image` | Build container image | **Yes** - only if Dockerfile exists |
| `image-push` | Push container image to registry | **Yes** - only if Dockerfile exists |
| `image-manifest` | Create multi-arch image manifest | **Yes** - only if Dockerfile exists |

#### Dockerfiles (Optional)

Container images are **optional**. If your addon requires a container image:

- Dockerfile must reside under `<project_name>/Dockerfile`
- The workflow will automatically detect Dockerfile presence
- If no Dockerfile exists, image build steps will be skipped

#### Helm Charts

**If** your project requires a Helm chart, it must be structured as follows:

```bash
<project_name>
└── charts
    └── <project_name>  # Chart name must match project directory name
        ├── Chart.yaml
        ├── templates
        │   └── *.yaml
        └── values.yaml
```

Furthermore, `values.yaml` must contain the following top-level values:

```yaml
image:
  repository: <project_image_repository>
  tag: <project_image_tag>
```

This is required so that a locally built image can be used consistently for Helm chart testing.

See the `test-chart` job in the [Test](./.github/workflows/test.yml) workflow for further details.

## Release a Sub Project

Release tags must follow the format `<sub-project-name>/v*.*.*`. For example, to release kueue-addon v0.1.0, use tag `kueue-addon/v0.1.0`.

See [Release](./.github/workflows/release.yml), [ReleaseImage](./.github/workflows/releaseimage.yml) and [ChartUpload](./.github/workflows/chart-upload.yml) workflows for details.

## Governance

addon-contrib operates under the governance structure of the Open Cluster Management (OCM) project:

- **Project Status**: addon-contrib is a sub-project of the OCM main project and complies with all OCM project rules and guidelines
- **Leadership**: addon-contrib does not have independent leadership and follows the same leadership strategy as the OCM main project
- **Maintainership**: Each addon has its own maintainers listed in the respective OWNERS files, while overall repository governance follows OCM standards
- **Community Guidelines**: All contributions must adhere to the OCM Code of Conduct and contribution guidelines
- **Decision Making**: Technical decisions follow the same consensus-based approach used across the OCM ecosystem
