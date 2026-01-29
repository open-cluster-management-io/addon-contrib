# Image URL to use all building/pushing image targets
CONTROLLER_IMAGE_NAME ?= dynamic-scoring-controller
ifdef CONTROLLER_IMAGE_TAG
CONTROLLER_IMAGE_TAG := $(CONTROLLER_IMAGE_TAG)
else ifdef IMAGE_TAG
CONTROLLER_IMAGE_TAG := $(IMAGE_TAG)
else
CONTROLLER_IMAGE_TAG := latest
endif
CONTROLLER_IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMG_CONTROLLER = $(CONTROLLER_IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_NAME):$(CONTROLLER_IMAGE_TAG)

ADDON_IMAGE_NAME ?= dynamic-scoring-addon
ifdef ADDON_IMAGE_TAG
ADDON_IMAGE_TAG := $(ADDON_IMAGE_TAG)
else ifdef IMAGE_TAG
ADDON_IMAGE_TAG := $(IMAGE_TAG)
else
ADDON_IMAGE_TAG := latest
endif
ADDON_IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMG_ADDON = $(ADDON_IMAGE_REGISTRY)/$(ADDON_IMAGE_NAME):$(ADDON_IMAGE_TAG)


# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= podman

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: update-helm-crds
update-helm-crds: manifests ## Update Helm CRDs
	@echo "Updating Helm CRDs..."
	@mkdir -p charts/dynamic-scoring-framework/crds
	@rm -rf charts/dynamic-scoring-framework/crds/*
	@cp -r config/crd/bases/* charts/dynamic-scoring-framework/crds/

.PHONY: update-helm-controller-manifests
update-helm-controller-manifests:
	@echo "Generating controller default manifests..."
	@mkdir -p build/dynamic-scoring
	@$(KUSTOMIZE) build config/default > build/dynamic-scoring/dynamic-scoring-controller.yaml

.PHONY: update-helm-addon-manifests
update-helm-addon-manifests:
	@echo "Generating addon default manifests..."
	@mkdir -p build/dynamic-scoring
	@$(KUSTOMIZE) build deploy/addon/dynamic-scoring > build/dynamic-scoring/addon-controller.yaml


.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= dynamic-scoring-framework-test-e2e
E2E_CONTROLLER_IMAGE ?= quay.io/open-cluster-management/dynamic-scoring-controller:e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@if $(KIND) get clusters | grep -qx "$(KIND_CLUSTER)"; then \
		echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping create."; \
	else \
		$(KIND) create cluster --name $(KIND_CLUSTER); \
	fi
	$(CONTAINER_TOOL) build -t $(E2E_CONTROLLER_IMAGE) -f Dockerfile .
	$(KIND) load docker-image $(E2E_CONTROLLER_IMAGE) --name $(KIND_CLUSTER)

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND_CLUSTER=$(KIND_CLUSTER) go test ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

## TODO Implement verify, test-unit, test-integration targets
verify:
	@echo "Skipping verify (not implemented)"
	@true
.PHONY: verify

# test-unit:
# 	@echo "Skipping unit tests (not implemented)"
# 	@true
# .PHONY: test-unit
test-unit-controller:
	@echo "Running unit tests for controller..."
	go test ./internal/controller/...
.PHONY: test-unit-controller

test-unit-addon:
	@echo "Running unit tests for addon..."
	go test ./pkg/dynamic_scoring/... ./pkg/dynamic_scoring_agent/...
.PHONY: test-unit-addon

test-unit: test-unit-controller test-unit-addon
.PHONY: test-unit


test-integration:
	@echo "Skipping integration tests (not implemented)"
	@true
.PHONY: test-integration

.PHONY: test-chart
test-chart: ## Run chart tests
	@echo "Skipping chart tests (not implemented)"
	@true

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build-controller
docker-build-controller: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG_CONTROLLER} .

.PHONY: docker-build-addon
docker-build-addon: ## Build docker image with the addon.
	$(CONTAINER_TOOL) build -t ${IMG_ADDON} . -f Dockerfile.addon --no-cache

.PHONY: docker-push-controller
docker-push-controller: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG_CONTROLLER}

.PHONY: docker-push-addon
docker-push-addon: ## Push docker image with the addon.
	$(CONTAINER_TOOL) push ${IMG_ADDON}


image-manifest-controller:
	$(CONTAINER_TOOL) manifest create $(IMG_CONTROLLER) \
		$(IMG_CONTROLLER)-amd64 \
		$(IMG_CONTROLLER)-arm64
	$(CONTAINER_TOOL) manifest annotate $(IMG_CONTROLLER) \
		$(IMG_CONTROLLER)-amd64 --arch amd64
	$(CONTAINER_TOOL) manifest annotate $(IMG_CONTROLLER) \
		$(IMG_CONTROLLER)-arm64 --arch arm64
	$(CONTAINER_TOOL) manifest push $(IMG_CONTROLLER)
.PHONY: image-manifest-controller

image-manifest-addon:
	$(CONTAINER_TOOL) manifest create $(IMG_ADDON) \
		$(IMG_ADDON)-amd64 \
		$(IMG_ADDON)-arm64
	$(CONTAINER_TOOL) manifest annotate $(IMG_ADDON) \
		$(IMG_ADDON)-amd64 --arch amd64
	$(CONTAINER_TOOL) manifest annotate $(IMG_ADDON) \
		$(IMG_ADDON)-arm64 --arch arm64
	$(CONTAINER_TOOL) manifest push $(IMG_ADDON)
.PHONY: image-manifest-addon

image: docker-build-controller docker-build-addon
.PHONY: image

image-push: docker-push-controller docker-push-addon
.PHONY: image-push

image-manifest: image-manifest-controller image-manifest-addon
.PHONY: image-manifest


# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name dynamic-scoring-framework-builder
	$(CONTAINER_TOOL) buildx use dynamic-scoring-framework-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG_CONTROLLER} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm dynamic-scoring-framework-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG_CONTROLLER}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG_CONTROLLER}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.18.0
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.1.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

.PHONY: deploy-addon
deploy-addon:
	cd deploy/addon/dynamic-scoring && $(KUSTOMIZE) edit set image quay.io/open-cluster-management/dynamic-scoring-addon=$(IMG_ADDON)
	$(KUSTOMIZE) build deploy/addon/dynamic-scoring | $(KUBECTL) apply -f -

.PHONY: undeploy-addon
undeploy-addon:
	$(KUBECTL) delete -f deploy/addon/dynamic-scoring/resources/clustermanagementaddon.yaml --ignore-not-found