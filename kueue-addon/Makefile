SHELL :=/bin/bash

all: build
.PHONY: all

LOCALBIN ?= $(shell pwd)/bin
KUBECTL?=kubectl

IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMAGE_TAG ?= latest
IMAGE_NAME ?= $(IMAGE_REGISTRY)/kueue-addon:$(IMAGE_TAG)
export KUBECONFIG?= $(HOME)/.kube/config
# Parse Kueue version from go.mod
KUEUE_VERSION := $(shell grep 'sigs.k8s.io/kueue' go.mod | awk '{print $$2}')

# update
update: update-manifests
.PHONY: update

# update cluster-permission.yaml with latest rules from Kueue repository
update-manifests:
	@echo "Updating cluster-permission.yaml with latest rules from Kueue repository..."
	@python3 scripts/update-cluster-permission.py
.PHONY: update-manifests

# verify
verify-gocilint:
	@echo "Running golangci-lint..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.6
	golangci-lint run --timeout=5m --modules-download-mode vendor ./...

install-golang-gci:
	@echo "Installing golang-gci..."
	go install github.com/daixiang0/gci@v0.13.6

fmt-imports: install-golang-gci
	@echo "Formatting imports..."
	gci write --skip-generated -s standard -s default -s "prefix(open-cluster-management.io)" -s "prefix(open-cluster-management.io/ocm)" cmd pkg

verify-fmt-imports: install-golang-gci
	@echo "Verifying import formatting..."
	@output=$$(gci diff --skip-generated -s standard -s default -s "prefix(open-cluster-management.io)" -s "prefix(open-cluster-management.io/ocm)" cmd pkg); \
	if [ -n "$$output" ]; then \
	    echo "Diff output is not empty: $$output"; \
	    echo "Please run 'make fmt-imports' to format the golang files imports automatically."; \
	    exit 1; \
	else \
	    echo "Diff output is empty"; \
	fi

verify-manifests: update
	@echo "Verifying cluster-permission.yaml is up to date..."
	@if git status --porcelain manifests/cluster-permission/cluster-permission.yaml | grep -q manifests/cluster-permission/cluster-permission.yaml; then \
		echo "✗ cluster-permission.yaml has uncommitted changes. Please commit or stash them."; \
		git status manifests/cluster-permission/cluster-permission.yaml; \
		exit 1; \
	else \
		echo "✓ cluster-permission.yaml is up to date"; \
	fi
.PHONY: verify-manifests

verify-chart:
	@echo "Verifying chart appVersion matches Kueue version in go.mod..."
	@CHART_VERSION=$$(grep 'appVersion:' charts/kueue-addon/Chart.yaml | awk '{print $$2}' | tr -d '"'); \
	if [ v"$$CHART_VERSION" = "$(KUEUE_VERSION)" ]; then \
		echo "✓ Chart appVersion ($$CHART_VERSION) matches Kueue version in go.mod ($(KUEUE_VERSION))"; \
	else \
		echo "✗ Chart appVersion ($$CHART_VERSION) does not match Kueue version in go.mod ($(KUEUE_VERSION))"; \
		echo "Please update the appVersion in charts/kueue-addon/Chart.yaml to match the Kueue version in go.mod"; \
		exit 1; \
	fi
.PHONY: verify-chart

verify: vendor verify-fmt-imports verify-gocilint verify-manifests verify-chart
.PHONY: verify

# build
vendor: 
	@echo "Updating vendor dependencies..."
	go mod tidy
	go mod vendor
.PHONY: vendor

build: vendor
	@echo "Building kueue-addon-controller..."
	CGO_ENABLED=0 go build -ldflags="-s -w" -o kueue-addon-controller cmd/kueue/main.go
.PHONY: build

image:
	@echo "Building Docker image..."
	docker build -f Dockerfile -t $(IMAGE_NAME) .
.PHONY: image

image-push:
	@echo "Pushing Docker image..."
	docker push $(IMAGE_NAME)
.PHONY: image-push

image-manifest:
	docker manifest create $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/kueue-addon:$(IMAGE_TAG)-amd64 \
		$(IMAGE_REGISTRY)/kueue-addon:$(IMAGE_TAG)-arm64
	docker manifest annotate $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/kueue-addon:$(IMAGE_TAG)-amd64 --arch amd64
	docker manifest annotate $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/kueue-addon:$(IMAGE_TAG)-arm64 --arch arm64
	docker manifest push $(IMAGE_NAME)

# deploy
deploy: kustomize
	@echo "Deploying kueue-addon..."
	cp deploy/kustomization.yaml deploy/kustomization.yaml.tmp
	cd deploy && $(KUSTOMIZE) edit set image kueue-addon-image=$(IMAGE_NAME)
	$(KUSTOMIZE) build deploy | $(KUBECTL) apply -f -
	mv deploy/kustomization.yaml.tmp deploy/kustomization.yaml

# install kustomize
KUSTOMIZE ?= $(LOCALBIN)/kustomize
KUSTOMIZE_VERSION ?= v3.8.7
KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"

kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE):
	@echo "Installing kustomize..."
	mkdir -p $(LOCALBIN)
	curl $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

# test
TEST_TMP :=/tmp
export KUBEBUILDER_ASSETS ?=$(TEST_TMP)/kubebuilder/bin

K8S_VERSION ?=1.30.0
GOHOSTOS ?=$(shell go env GOHOSTOS)
GOHOSTARCH ?=$(shell go env GOHOSTARCH)
KB_TOOLS_ARCHIVE_NAME :=kubebuilder-tools-$(K8S_VERSION)-$(GOHOSTOS)-$(GOHOSTARCH).tar.gz
KB_TOOLS_ARCHIVE_PATH := $(TEST_TMP)/$(KB_TOOLS_ARCHIVE_NAME)
GO_TEST_PACKAGES :=./pkg/...
GO_TEST_FLAGS := -race -coverprofile=coverage.out

# download the kubebuilder-tools to get kube-apiserver binaries from it
ensure-kubebuilder-tools:
ifeq "" "$(wildcard $(KUBEBUILDER_ASSETS))"
	@echo "Downloading kube-apiserver into '$(KUBEBUILDER_ASSETS)'"
	mkdir -p '$(KUBEBUILDER_ASSETS)'
	curl -s -f -L https://storage.googleapis.com/kubebuilder-tools/$(KB_TOOLS_ARCHIVE_NAME) -o '$(KB_TOOLS_ARCHIVE_PATH)'
	tar -C '$(KUBEBUILDER_ASSETS)' --strip-components=2 -zvxf '$(KB_TOOLS_ARCHIVE_PATH)'
else
	@echo "Using existing kube-apiserver from '$(KUBEBUILDER_ASSETS)'"
endif
.PHONY: ensure-kubebuilder-tools

clean-integration-test:
	@echo "Cleaning integration test artifacts..."
	$(RM) '$(KB_TOOLS_ARCHIVE_PATH)'
	rm -rf $(TEST_TMP)/kubebuilder
	$(RM) ./*integration.test
.PHONY: clean-integration-test

clean: clean-integration-test

test-unit:
	@echo "Running unit tests..."
	go test $(GO_TEST_FLAGS) $(GO_TEST_PACKAGES)
.PHONY: test-unit

test-integration: ensure-kubebuilder-tools
	@echo "Running integration tests..."
	go test -c -ldflags="-s -w" ./test/integration -o ./kueue-integration.test -tags=integration
	./kueue-integration.test -ginkgo.slow-spec-threshold=15s -ginkgo.v -ginkgo.fail-fast ${ARGS}
.PHONY: test-integration

test-e2e-setup:
	@echo "Setting up e2e test environment..."
	IMAGE_TAG=e2e make image
	@echo "Cleaning Docker build cache..."
	@docker builder prune -af || true
	@build/setup-env.sh --e2e --clean --kueue-version $(KUEUE_VERSION)
.PHONY: test-e2e-setup

test-e2e: test-e2e-setup
	@echo "Running e2e tests..."
	go test -c -ldflags="-s -w" ./test/e2e -o kueue-addon-e2e.test
	./kueue-addon-e2e.test -test.v -ginkgo.v --ginkgo.timeout=2h
.PHONY: test-e2e

test-chart:
	@echo "TODO: Implement test-chart"
	@echo "image.repository=$(IMAGE_NAME) image.tag=$(IMAGE_TAG)"
.PHONY: test-chart
