SHELL :=/bin/bash

all: build
.PHONY: all

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

LOCALBIN ?= $(shell pwd)/bin

# Tools for deploy
KUBECONFIG ?= ./.kubeconfig
KUBECTL?=kubectl
PWD=$(shell pwd)

# Image URL to use all building/pushing image targets;
GO_BUILD_PACKAGES :=./pkg/...
IMAGE ?= resource-usage-collect-addon
IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMAGE_TAG ?= latest
IMAGE_NAME ?= $(IMAGE_REGISTRY)/$(IMAGE):$(IMAGE_TAG)

GIT_HOST ?= open-cluster-management.io
BASE_DIR := $(shell basename $(PWD))
DEST := $(GOPATH)/src/$(GIT_HOST)/$(BASE_DIR)

# Add packages to do unit test
GO_TEST_PACKAGES :=./pkg/...

##@ Development
.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

##@ Build
.PHONY: build
build: fmt vet ## Build manager binary.
	GOFLAGS="" go build -o addon ./pkg/main.go


##@ Deploy
.PHONY: deploy
deploy: kustomize
	cp deploy/kustomization.yaml deploy/kustomization.yaml.tmp
	cd deploy && $(KUSTOMIZE) edit set image example-addon-image=$(IMAGE_NAME)
	$(KUSTOMIZE) build deploy | $(KUBECTL) apply -f -
	mv deploy/kustomization.yaml.tmp deploy/kustomization.yaml

.PHONY: undeploy
undeploy: kustomize
	$(KUSTOMIZE) build deploy | $(KUBECTL) delete --ignore-not-found -f -

# install kustomize
KUSTOMIZE ?= $(LOCALBIN)/kustomize
KUSTOMIZE_VERSION ?= v3.8.7
KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE):
	mkdir -p $(LOCALBIN)
	curl $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

##@ Verification
.PHONY: vendor
vendor: ## Update vendor dependencies
	@echo "Updating vendor dependencies..."
	go mod tidy
	go mod vendor

.PHONY: verify
verify: fmt vet ## Run code verification
	@echo "✓ Verification passed"

##@ Testing
.PHONY: test-unit
test-unit: ## Run unit tests
	@echo "Running unit tests..."
	go test -race -coverprofile=coverage.out $(GO_TEST_PACKAGES)

.PHONY: test-integration
test-integration: ## Run integration tests
	@echo "Skipping integration tests (not implemented)"
	@true

.PHONY: test-e2e
test-e2e: ## Run e2e tests
	@echo "Creating placeholder e2e artifact..."
	@tar -czf $(IMAGE).tar.gz deploy/ charts/ || true
	@echo "Skipping e2e tests (not implemented)"
	@true

.PHONY: test-chart
test-chart: ## Run chart tests
	@echo "Skipping chart tests (not implemented)"
	@true

##@ Image Management
.PHONY: image
image: ## Build docker image
	docker build -t ${IMAGE_NAME} .

.PHONY: image-push
image-push: ## Push docker image
	@echo "Pushing Docker image..."
	docker push $(IMAGE_NAME)

.PHONY: image-manifest
image-manifest: ## Create and push multi-arch manifest
	docker manifest create $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/$(IMAGE):$(IMAGE_TAG)-amd64 \
		$(IMAGE_REGISTRY)/$(IMAGE):$(IMAGE_TAG)-arm64
	docker manifest annotate $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/$(IMAGE):$(IMAGE_TAG)-amd64 --arch amd64
	docker manifest annotate $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/$(IMAGE):$(IMAGE_TAG)-arm64 --arch arm64
	docker manifest push $(IMAGE_NAME)
