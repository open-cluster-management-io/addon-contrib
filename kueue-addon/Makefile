SHELL :=/bin/bash

all: build
.PHONY: all

LOCALBIN ?= $(shell pwd)/bin
KUBECTL?=kubectl

IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMAGE_TAG ?= latest
IMAGE_NAME ?= $(IMAGE_REGISTRY)/kueue-addon:$(IMAGE_TAG)

# verify
verify-gocilint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.6
	golangci-lint run --timeout=5m --modules-download-mode vendor ./...

install-golang-gci:
	go install github.com/daixiang0/gci@v0.13.6

fmt-imports: install-golang-gci
	gci write --skip-generated -s standard -s default -s "prefix(open-cluster-management.io)" -s "prefix(open-cluster-management.io/ocm)" cmd pkg

verify-fmt-imports: install-golang-gci
	@output=$$(gci diff --skip-generated -s standard -s default -s "prefix(open-cluster-management.io)" -s "prefix(open-cluster-management.io/ocm)" cmd pkg); \
	if [ -n "$$output" ]; then \
	    echo "Diff output is not empty: $$output"; \
	    echo "Please run 'make fmt-imports' to format the golang files imports automatically."; \
	    exit 1; \
	else \
	    echo "Diff output is empty"; \
	fi

verify: vendor verify-fmt-imports verify-gocilint

# build
vendor: 
	go mod tidy
	go mod vendor
.PHONY: vendor

build: vendor
	CGO_ENABLED=0 go build -ldflags="-s -w" -o kueue-addon-controller cmd/kueue/main.go
.PHONY: build

image:
	docker build -f Dockerfile -t $(IMAGE_NAME) .
.PHONY: image

image-push:
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
	$(info Downloading kube-apiserver into '$(KUBEBUILDER_ASSETS)')
	mkdir -p '$(KUBEBUILDER_ASSETS)'
	curl -s -f -L https://storage.googleapis.com/kubebuilder-tools/$(KB_TOOLS_ARCHIVE_NAME) -o '$(KB_TOOLS_ARCHIVE_PATH)'
	tar -C '$(KUBEBUILDER_ASSETS)' --strip-components=2 -zvxf '$(KB_TOOLS_ARCHIVE_PATH)'
else
	$(info Using existing kube-apiserver from "$(KUBEBUILDER_ASSETS)")
endif
.PHONY: ensure-kubebuilder-tools

clean-integration-test:
	$(RM) '$(KB_TOOLS_ARCHIVE_PATH)'
	rm -rf $(TEST_TMP)/kubebuilder
	$(RM) ./*integration.test
.PHONY: clean-integration-test

clean: clean-integration-test

test-unit:
	go test $(GO_TEST_FLAGS) $(GO_TEST_PACKAGES)
.PHONY: test-unit

test-integration: ensure-kubebuilder-tools
	go test -c ./test/integration -o ./kueue-integration.test -tags=integration
	./kueue-integration.test -ginkgo.slow-spec-threshold=15s -ginkgo.v -ginkgo.fail-fast ${ARGS}
.PHONY: test-integration

test-e2e:
	echo "TODO: Implement e2e tests"
.PHONY: test-e2e

test-chart:
	echo "TODO: Implement test-chart"
	echo "image.repository=$(IMAGE_NAME) image.tag=$(IMAGE_TAG)"
.PHONY: test-chart