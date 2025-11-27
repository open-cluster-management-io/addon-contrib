# Sidecar image build configuration
IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMAGE_TAG ?= latest
IMAGE_NAME ?= $(IMAGE_REGISTRY)/federated-learning-sidecar:$(IMAGE_TAG)

CONTAINER_TOOL ?= docker

.PHONY: build
build: ## Build sidecar binary
	CGO_ENABLED=0 go build -o fl_sidecar main.go

.PHONY: image
image: ## Build sidecar docker image
	$(CONTAINER_TOOL) build $(IMAGE_BUILD_EXTRA_FLAGS) -t $(IMAGE_NAME) .

.PHONY: image-push
image-push: ## Push sidecar docker image
	$(CONTAINER_TOOL) push $(IMAGE_NAME)

.PHONY: image-manifest
image-manifest: ## Create and push multi-arch manifest for sidecar
	$(CONTAINER_TOOL) manifest create $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/federated-learning-sidecar:$(IMAGE_TAG)-amd64 \
		$(IMAGE_REGISTRY)/federated-learning-sidecar:$(IMAGE_TAG)-arm64
	$(CONTAINER_TOOL) manifest annotate $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/federated-learning-sidecar:$(IMAGE_TAG)-amd64 --arch amd64
	$(CONTAINER_TOOL) manifest annotate $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/federated-learning-sidecar:$(IMAGE_TAG)-arm64 --arch arm64
	$(CONTAINER_TOOL) manifest push $(IMAGE_NAME)
