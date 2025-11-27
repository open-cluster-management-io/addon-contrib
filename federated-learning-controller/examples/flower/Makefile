# Variables
IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMAGE_TAG ?= latest
DIST_DIR = dist
APP_DIR = app-torch
APP_NAME ?= flower-mnist
PLATFORMS ?= linux/amd64,linux/arm64
IMAGE_NAME ?= $(IMAGE_REGISTRY)/federated-learning-application:$(APP_NAME)-$(IMAGE_TAG)

CONTAINER_TOOL ?= docker

# Legacy variable for backwards compatibility
REGISTRY ?= $(IMAGE_REGISTRY)
IMAGE_FULL_NAME = $(IMAGE_NAME)

# Single-arch build (local)
.PHONY: build-app-image
build-app-image:
	cd $(APP_DIR) && $(CONTAINER_TOOL) build -t $(IMAGE_NAME) . -f Dockerfile

# Single-arch push
.PHONY: push-app-image
push-app-image:
	$(CONTAINER_TOOL) push $(IMAGE_NAME)

# Multi-arch build without push (export to local registry/engine not supported directly)
.PHONY: build-platform-image
build-platform-image:
	cd $(APP_DIR) && \
	$(CONTAINER_TOOL) buildx build \
		--platform $(PLATFORMS) \
		--tag $(IMAGE_NAME) \
		--output=type=docker \
		. -f Dockerfile

# Multi-arch push (requires --push during buildx)
.PHONY: push-platform-image
push-platform-image:
	cd $(APP_DIR) && \
	$(CONTAINER_TOOL) buildx build \
		--platform $(PLATFORMS) \
		--tag $(IMAGE_NAME) \
		--push \
		. -f Dockerfile

# CI/CD compatible targets
.PHONY: image
image: ## Build flower app docker image (CI/CD compatible)
	cd $(APP_DIR) && $(CONTAINER_TOOL) build $(IMAGE_BUILD_EXTRA_FLAGS) \
		-t $(IMAGE_NAME) .

.PHONY: image-push
image-push: ## Push flower app docker image
	$(CONTAINER_TOOL) push $(IMAGE_NAME)

.PHONY: image-manifest
image-manifest: ## Create and push multi-arch manifest for flower app
	$(CONTAINER_TOOL) manifest create $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/federated-learning-application:$(APP_NAME)-$(IMAGE_TAG)-amd64 \
		$(IMAGE_REGISTRY)/federated-learning-application:$(APP_NAME)-$(IMAGE_TAG)-arm64
	$(CONTAINER_TOOL) manifest annotate $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/federated-learning-application:$(APP_NAME)-$(IMAGE_TAG)-amd64 --arch amd64
	$(CONTAINER_TOOL) manifest annotate $(IMAGE_NAME) \
		$(IMAGE_REGISTRY)/federated-learning-application:$(APP_NAME)-$(IMAGE_TAG)-arm64 --arch arm64
	$(CONTAINER_TOOL) manifest push $(IMAGE_NAME)

# Clean
.PHONY: clean
clean:
	rm -rf $(DIST_DIR) __pycache__ *.spec
