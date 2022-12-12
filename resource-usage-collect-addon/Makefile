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
IMAGE_REGISTRY ?= quay.io/haoqing
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
	GOFLAGS="" go build -o addon ./pkg/addon/main.go ./pkg/addon/controller.go 

.PHONY: images
images: ## Build addon binary.
	docker build -t ${IMAGE_NAME} .

##@ Deploy
.PHONY: deploy
deploy: kustomize
	cp deploy/kustomization.yaml deploy/kustomization.yaml.tmp
	cd deploy && $(KUSTOMIZE) edit set image example-addon-image=$(IMAGE_NAME) && $(KUSTOMIZE) edit add configmap image-config --from-literal=IMAGE_NAME=$(IMAGE_NAME)
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
