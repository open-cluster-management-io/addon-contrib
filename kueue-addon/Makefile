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
