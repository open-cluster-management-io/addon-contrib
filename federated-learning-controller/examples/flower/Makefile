# Variables
REGISTRY ?= quay.io/open-cluster-management
IMAGE_TAG ?= latest
DIST_DIR = dist
APP = app-torch
PLATFORMS ?= linux/amd64,linux/arm64
IMAGE_FULL_NAME = ${REGISTRY}/flower-${APP}:${IMAGE_TAG}

# Single-arch build (local)
build-app-image:
	cd ${APP} && docker build -t ${IMAGE_FULL_NAME} . -f Dockerfile && cd ..

# Single-arch push
push-app-image:
	docker push ${IMAGE_FULL_NAME}

# Multi-arch build without push (export to local registry/engine not supported directly)
build-platform-image:
	cd ${APP} && \
	docker buildx build \
		--platform ${PLATFORMS} \
		--tag ${IMAGE_FULL_NAME} \
		--output=type=docker \
		. -f Dockerfile && cd ..

# Multi-arch push (requires --push during buildx)
push-platform-image:
	cd ${APP} && \
	docker buildx build \
		--platform ${PLATFORMS} \
		--tag ${IMAGE_FULL_NAME} \
		--push \
		. -f Dockerfile && cd ..

# Clean
clean:
	rm -rf $(DIST_DIR) __pycache__ *.spec
