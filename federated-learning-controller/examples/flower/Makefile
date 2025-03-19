
# Variables
REGISTRY ?= quay.io/open-cluster-management
IMAGE_TAG ?= latest
DIST_DIR = dist
APP = app-torch

build-app-image:
	cd ${APP} && docker build -t ${REGISTRY}/flower-${APP}:${IMAGE_TAG} . -f Dockerfile && cd ..

push-app-image:
	docker push ${REGISTRY}/flower-${APP}:${IMAGE_TAG}

clean:
	rm -rf $(DIST_DIR) __pycache__ *.spec