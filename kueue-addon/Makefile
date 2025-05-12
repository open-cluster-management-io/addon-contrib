SHELL :=/bin/bash

all: build
.PHONY: all

LOCALBIN ?= $(shell pwd)/bin
KUBECTL?=kubectl

IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMAGE_TAG ?= latest
IMAGE_NAME ?= $(IMAGE_REGISTRY)/kueue-addon:$(IMAGE_TAG)

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
