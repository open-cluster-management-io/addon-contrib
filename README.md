# Addon-Contrib

This repository hosts a collection of Open Cluster Management (OCM) addons for staging and testing Proof of Concept (PoC) purposes.

## Overview

OCM is a project that provides a unified way to manage multiple Kubernetes clusters.
This repository is intended for the development, staging, and testing of various OCM addons.

Addons in this repository are designed to extend the capabilities of the OCM deployment,
allowing for enhanced AI integration, IoT layer, cluster proxy, telemetry, resource usage collection and more.

## Onboarding a New Project

To onboard a new addon-contrib project:

1. If not already discussed with maintainers, open an issue to propose your idea.
1. Once acknowledged, create a folder named after your project and add your code/docs.
1. Add an `OWNERS` file listing the new project's maintainers.
1. Create a PR with a brief project overview and confirm the `OWNERS` file is present.
1. An OCM maintainer will review and merge the PR.

### GitHub Actions

All projects must follow certain conventions to ensure compatibility with the addon-contrib repository's Github Actions workflows.

Refer to the [Test](./.github/workflows/test.yml) and [E2E](./.github/workflows/e2e.yml) workflows for exact details.

#### `make` targets

All projects must define the following `make` targets:

- `verify`: Import statement formatting verification using `gci` and static code analysis and linting using `golangci-lint`
- `build`: Compile the Go application into a statically linked binary with debug information stripped for optimal container deployment
- `test-unit`: Invoke unit tests and return an exit code accordingly.
- `test-chart`: Invoke scripts to verify your chart can be installed succefully.
- `test-e2e`: Invoke end-to-end tests and return an exit code accordingly.
- `image`: Build all container image.
- `image-push`: Push all container images.
- `image-manifest`: Create annotate and push multi-architecture manifests for all images.

#### Dockerfiles

All Dockerfiles for the project must reside under `<project_name>/` and the default Dockerfile must be named `Dockerfile`.

#### Helm Charts

Any projects that require a Helm chart must be structured as follows:

```bash
<project_name>
└── charts
    └── <project_name> # chart name must match project directory name
        ├── Chart.yaml
        ├── templates
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

- addon-contrib is a sub-project of the OCM main project, complying with the rules of OCM main projects.
- addon-contrib does not have independent leadership, adopting the same leadership strategy as the OCM main project.

## Repository Structure

The repository is organized into directories, each containing the source code and configuration files for a specific addon.

