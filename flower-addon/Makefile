# Kubernetes configuration
KUBECTL ?= kubectl
HELM ?= helm
NAMESPACE ?= open-cluster-management
RELEASE_NAME ?= flower-addon
CHART_PATH ?= charts/flower-addon

# Image configuration (default: official Flower images)
SUPERLINK_IMAGE ?= flwr/superlink:1.26.1-py3.13-ubuntu24.04
SUPERNODE_IMAGE ?= flwr/supernode:1.26.1-py3.13-ubuntu24.04
APP_IMAGE ?= flower-app:1.0.0

.PHONY: all
all: help

##@ Image Build

.PHONY: build-app
build-app: ## Build application image for process mode (single image for serverapp and clientapp)
	@echo "Building application image for process mode..."
	docker build -t $(APP_IMAGE) -f cifar10/Containerfile .
	@echo ""
	@echo "Image built successfully: $(APP_IMAGE)"

.PHONY: push-app
push-app: ## Push application image to registry
	docker push $(APP_IMAGE)

.PHONY: load-app-kind
load-app-kind: ## Load app image into kind clusters
	kind load docker-image $(APP_IMAGE) --name hub
	kind load docker-image $(APP_IMAGE) --name cluster1
	kind load docker-image $(APP_IMAGE) --name cluster2

##@ Helm Deployment

.PHONY: deploy
deploy: ## Deploy Flower addon using Helm (SuperLink + OCM addon)
	@HUB_IP=$$($(KUBECTL) get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'); \
	echo "Deploying Flower addon with SuperLink address: $$HUB_IP"; \
	$(HELM) upgrade --install $(RELEASE_NAME) $(CHART_PATH) \
		--set deploymentConfig.superlinkAddress=$$HUB_IP \
		--set supernode.image.repository=$$(echo $(SUPERNODE_IMAGE) | cut -d: -f1) \
		--set supernode.image.tag=$$(echo $(SUPERNODE_IMAGE) | cut -d: -f2)
	@echo ""
	@echo "Deployment complete! Check status with: make status"

.PHONY: deploy-superlink
deploy-superlink: ## Deploy only SuperLink on hub cluster
	$(HELM) upgrade --install $(RELEASE_NAME) $(CHART_PATH) \
		--set deploymentConfig.enabled=false
	@echo ""
	@echo "SuperLink deployment complete!"
	$(KUBECTL) wait --for=condition=available --timeout=120s deployment/superlink -n flower-system || true

.PHONY: undeploy
undeploy: ## Remove Flower addon using Helm
	$(HELM) uninstall $(RELEASE_NAME) || true
	$(KUBECTL) delete namespace flower-system --ignore-not-found

.PHONY: upgrade
upgrade: ## Upgrade Flower addon with new values
	@HUB_IP=$$($(KUBECTL) get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'); \
	$(HELM) upgrade $(RELEASE_NAME) $(CHART_PATH) \
		--set deploymentConfig.superlinkAddress=$$HUB_IP

##@ Placement-based Auto-Install

.PHONY: deploy-auto-gpu
deploy-auto-gpu: ## Deploy with auto-install for GPU clusters (clusters with gpu=true label)
	@HUB_IP=$$($(KUBECTL) get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'); \
	$(HELM) upgrade --install $(RELEASE_NAME) $(CHART_PATH) \
		--set deploymentConfig.superlinkAddress=$$HUB_IP \
		--set addon.installStrategy=Placements \
		--set placement.gpu.enabled=true
	@echo ""
	@echo "GPU auto-install deployed!"
	@echo "Label clusters with: kubectl label managedcluster <cluster-name> gpu=true"

.PHONY: deploy-auto-all
deploy-auto-all: ## Deploy with auto-install for all clusters (global cluster set)
	@HUB_IP=$$($(KUBECTL) get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'); \
	$(HELM) upgrade --install $(RELEASE_NAME) $(CHART_PATH) \
		--set deploymentConfig.superlinkAddress=$$HUB_IP \
		--set addon.installStrategy=Placements \
		--set placement.all.enabled=true
	@echo ""
	@echo "All-clusters auto-install deployed!"

##@ Manual Cluster Configuration

.PHONY: enable-addon
enable-addon: ## Enable addon on a cluster (usage: make enable-addon CLUSTER=cluster1 [CONFIG=flower-addon-config])
ifndef CLUSTER
	$(error CLUSTER is not set. Usage: make enable-addon CLUSTER=cluster1)
endif
ifdef CONFIG
	@echo "Creating ManagedClusterAddOn for $(CLUSTER) with config $(CONFIG)..."
	@echo "apiVersion: addon.open-cluster-management.io/v1alpha1" | \
	{ cat; echo "kind: ManagedClusterAddOn"; } | \
	{ cat; echo "metadata:"; } | \
	{ cat; echo "  name: flower-addon"; } | \
	{ cat; echo "  namespace: $(CLUSTER)"; } | \
	{ cat; echo "spec:"; } | \
	{ cat; echo "  configs:"; } | \
	{ cat; echo "    - group: addon.open-cluster-management.io"; } | \
	{ cat; echo "      resource: addondeploymentconfigs"; } | \
	{ cat; echo "      name: $(CONFIG)"; } | \
	{ cat; echo "      namespace: $(NAMESPACE)"; } | \
	$(KUBECTL) apply -f -
else
	@echo "Creating ManagedClusterAddOn for $(CLUSTER) (using defaultConfig from ClusterManagementAddOn)..."
	@echo "apiVersion: addon.open-cluster-management.io/v1alpha1" | \
	{ cat; echo "kind: ManagedClusterAddOn"; } | \
	{ cat; echo "metadata:"; } | \
	{ cat; echo "  name: flower-addon"; } | \
	{ cat; echo "  namespace: $(CLUSTER)"; } | \
	{ cat; echo "spec:"; } | \
	{ cat; echo "  installNamespace: flower-addon"; } | \
	$(KUBECTL) apply -f -
endif

.PHONY: disable-addon
disable-addon: ## Disable addon on a cluster (usage: make disable-addon CLUSTER=cluster1)
ifndef CLUSTER
	$(error CLUSTER is not set. Usage: make disable-addon CLUSTER=cluster1)
endif
	$(KUBECTL) delete managedclusteraddon flower-addon -n $(CLUSTER) --ignore-not-found

.PHONY: deploy-cluster-config
deploy-cluster-config: ## Deploy per-cluster config (usage: make deploy-cluster-config CLUSTER=cluster1 PARTITION_ID=0 NUM_PARTITIONS=2)
ifndef CLUSTER
	$(error CLUSTER is not set. Usage: make deploy-cluster-config CLUSTER=cluster1 PARTITION_ID=0 NUM_PARTITIONS=2)
endif
ifndef PARTITION_ID
	$(error PARTITION_ID is not set. Usage: make deploy-cluster-config CLUSTER=cluster1 PARTITION_ID=0 NUM_PARTITIONS=2)
endif
ifndef NUM_PARTITIONS
	$(error NUM_PARTITIONS is not set. Usage: make deploy-cluster-config CLUSTER=cluster1 PARTITION_ID=0 NUM_PARTITIONS=2)
endif
	@HUB_IP=$$($(KUBECTL) get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'); \
	echo "Creating AddOnDeploymentConfig for $(CLUSTER) with partition $(PARTITION_ID)/$(NUM_PARTITIONS)..."; \
	echo "apiVersion: addon.open-cluster-management.io/v1alpha1" | \
	{ cat; echo "kind: AddOnDeploymentConfig"; } | \
	{ cat; echo "metadata:"; } | \
	{ cat; echo "  name: flower-addon-config"; } | \
	{ cat; echo "  namespace: $(CLUSTER)"; } | \
	{ cat; echo "spec:"; } | \
	{ cat; echo "  customizedVariables:"; } | \
	{ cat; echo "    - name: SUPERLINK_ADDRESS"; } | \
	{ cat; echo "      value: \"$$HUB_IP\""; } | \
	{ cat; echo "    - name: SUPERLINK_PORT"; } | \
	{ cat; echo "      value: \"30092\""; } | \
	{ cat; echo "    - name: IMAGE"; } | \
	{ cat; echo "      value: \"$(SUPERNODE_IMAGE)\""; } | \
	{ cat; echo "    - name: PARTITION_ID"; } | \
	{ cat; echo "      value: \"$(PARTITION_ID)\""; } | \
	{ cat; echo "    - name: NUM_PARTITIONS"; } | \
	{ cat; echo "      value: \"$(NUM_PARTITIONS)\""; } | \
	$(KUBECTL) apply -f -

##@ Quick Setup

.PHONY: setup-clusters
setup-clusters: deploy ## Deploy hub and configure example clusters (cluster1, cluster2)
	@echo "Setting up cluster1 (partition 0)..."
	$(MAKE) deploy-cluster-config CLUSTER=cluster1 PARTITION_ID=0 NUM_PARTITIONS=2
	$(MAKE) enable-addon CLUSTER=cluster1
	@echo "Setting up cluster2 (partition 1)..."
	$(MAKE) deploy-cluster-config CLUSTER=cluster2 PARTITION_ID=1 NUM_PARTITIONS=2
	$(MAKE) enable-addon CLUSTER=cluster2
	@echo ""
	@echo "Setup complete! Check addon status with: make status"

##@ Application Deployment (Process Isolation Mode)

.PHONY: deploy-app
deploy-app: ## Deploy SuperExec app (serverapp on hub, clientapp via ManifestWorkReplicaSet)
	@echo "Deploying SuperExec app..."
	$(KUBECTL) apply -k cifar10/deploy/
	@echo ""
	@echo "App deployment complete!"
	@echo "  - SuperExec-ServerApp: deployed to flower-system namespace"
	@echo "  - SuperExec-ClientApp: distributed via ManifestWorkReplicaSet to clusters with flower-addon"

.PHONY: undeploy-app
undeploy-app: ## Remove SuperExec app deployments
	$(KUBECTL) delete -k cifar10/deploy/ --ignore-not-found
	@echo "App removal complete!"

##@ Federated Learning

.PHONY: run-app
run-app: ## Run federated learning app on OCM federation
	@HUB_IP=$$($(KUBECTL) get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'); \
	echo "Submitting FL app to SuperLink at $$HUB_IP:30093..."; \
	flwr run . ocm-deployment --federation-config 'address="'"$$HUB_IP"':30093"' --stream

.PHONY: app-logs
app-logs: ## Show FL app logs (SuperExec components)
	@echo "=== SuperLink Logs ==="
	@$(KUBECTL) logs -n flower-system -l app.kubernetes.io/component=superlink --tail=20
	@echo ""
	@echo "=== SuperExec-ServerApp Logs ==="
	@$(KUBECTL) logs -n flower-system -l app.kubernetes.io/component=superexec-serverapp --tail=20

##@ Status

.PHONY: status
status: ## Show addon status
	@echo "=== Helm Release ==="
	$(HELM) list -a | grep $(RELEASE_NAME) || echo "No Helm release found"
	@echo ""
	@echo "=== SuperLink Status ==="
	$(KUBECTL) get pods -n flower-system
	@echo ""
	@echo "=== AddOnTemplate ==="
	$(KUBECTL) get addontemplates
	@echo ""
	@echo "=== ClusterManagementAddOn ==="
	$(KUBECTL) get clustermanagementaddons
	@echo ""
	@echo "=== AddOnDeploymentConfigs ==="
	$(KUBECTL) get addondeploymentconfigs -A
	@echo ""
	@echo "=== Placements ==="
	$(KUBECTL) get placements -n $(NAMESPACE)
	@echo ""
	@echo "=== PlacementDecisions ==="
	$(KUBECTL) get placementdecisions -n $(NAMESPACE)
	@echo ""
	@echo "=== ManagedClusterAddOns ==="
	$(KUBECTL) get managedclusteraddons -A

##@ CI Targets (required by addon-contrib CI)

.PHONY: verify
verify: lint ## Verify the project (lint helm chart)
	@echo "Verification complete!"

.PHONY: build
build: ## Build target (no-op for Helm-only addon)
	@echo "No build required for Helm-only addon"

.PHONY: test-unit
test-unit: ## Run unit tests (no-op for Helm-only addon)
	@echo "No unit tests for Helm-only addon"

.PHONY: test-integration
test-integration: ## Run integration tests (no-op for Helm-only addon)
	@echo "No integration tests for Helm-only addon"

.PHONY: test-chart
test-chart: ## Run Helm chart tests
	@echo "Running chart installation test..."
	$(HELM) lint $(CHART_PATH)
	@echo "Chart tests passed!"

.PHONY: test-e2e
test-e2e: ## Run e2e tests (no-op for Helm-only addon)
	@echo "No e2e tests for Helm-only addon"

##@ Development

.PHONY: lint
lint: ## Lint the Helm chart
	$(HELM) lint $(CHART_PATH)

.PHONY: template
template: ## Render Helm templates locally
	@HUB_IP=$$($(KUBECTL) get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || echo "192.168.1.1"); \
	$(HELM) template $(RELEASE_NAME) $(CHART_PATH) --set deploymentConfig.superlinkAddress=$$HUB_IP

##@ Help

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
