
# Image URL to use all building/pushing image targets
IMG ?= quay.io/skeeey/device-addon:latest

CONTROLLER_TOOLS_VERSION ?= v0.12.0
CODE_GENERATOR_VERSION ?= v0.27.3

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## build
.PHONY: build
build:
	go build -o bin/device-addon cmd/main.go

.PHONY: image
image:
	docker build -t ${IMG} .

## deploy and run demo
.PHONY: deploy
deploy:
	contrib/demo/deploy.sh

.PHONY: demo
demo:
	contrib/demo/demo.sh

## generate code and crds
.PHONY: controller-gen
controller-gen: $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: code-generator
code-generator: $(LOCALBIN)
	test -s $(LOCALBIN)/client-gen || GOBIN=$(LOCALBIN) go install k8s.io/code-generator/cmd/client-gen@$(CODE_GENERATOR_VERSION)
	test -s $(LOCALBIN)/informer-gen || GOBIN=$(LOCALBIN) go install k8s.io/code-generator/cmd/informer-gen@$(CODE_GENERATOR_VERSION)
	test -s $(LOCALBIN)/lister-gen || GOBIN=$(LOCALBIN) go install k8s.io/code-generator/cmd/lister-gen@$(CODE_GENERATOR_VERSION)
	test -s $(LOCALBIN)/deepcopy-gen || GOBIN=$(LOCALBIN) go install k8s.io/code-generator/cmd/deepcopy-gen@$(CODE_GENERATOR_VERSION)
	test -s $(LOCALBIN)/openapi-gen || GOBIN=$(LOCALBIN) go install k8s.io/code-generator/cmd/openapi-gen@$(CODE_GENERATOR_VERSION)

.PHONY: crds-gen
crds-gen: controller-gen
	hack/crds_gen.sh

.PHONY: code-gen
code-gen: code-generator
	hack/code_gen.sh

.PHONY: update
update: code-gen crds-gen
