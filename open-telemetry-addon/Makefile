.PHONY: help install-all generate-certs install-prometheus install-addon install-addon-no-tls clean

# Default target
help:
	@echo "Available targets:"
	@echo "  install-all           - Install everything (certs, prometheus, addon)"
	@echo "  generate-certs        - Generate TLS certificates"
	@echo "  install-prometheus    - Install kube-prometheus-stack"
	@echo "  install-addon         - Install opentelemetry-addon chart"
	@echo "  install-addon-no-tls  - Install addon without TLS (for existing Prometheus)"
	@echo "  clean                 - Clean up installations"

# Variables
NAMESPACE ?= open-cluster-management-addon
HUB_NAMESPACE ?= open-cluster-management-hub
PROMETHEUS_NAMESPACE ?= monitoring
CHART_DIR = charts/opentelemetry-addon
CERTS_DIR = certs
CONTEXT ?= kind-hub

# Install everything in order
install-all: generate-certs install-prometheus install-addon

# Generate TLS certificates
generate-certs:
	@echo "Generating TLS certificates..."
	@mkdir -p $(CERTS_DIR)
	cd $(CERTS_DIR) && \
	echo "Generating Root CA..." && \
	openssl genrsa -out root-ca.key 4096 && \
	openssl req -x509 -new -nodes -key root-ca.key -sha256 -days 3650 \
		-subj "/CN=My Root CA" \
		-out root-ca.crt && \
	echo "Generating Client CA..." && \
	openssl genrsa -out client-ca.key 4096 && \
	openssl req -x509 -new -nodes -key client-ca.key -sha256 -days 3650 \
		-subj "/CN=My Client CA" \
		-out client-ca.crt && \
	echo "Creating SAN config..." && \
	echo "[ req ]" > san.cnf && \
	echo "distinguished_name = req_distinguished_name" >> san.cnf && \
	echo "req_extensions = v3_req" >> san.cnf && \
	echo "prompt = no" >> san.cnf && \
	echo "" >> san.cnf && \
	echo "[ req_distinguished_name ]" >> san.cnf && \
	echo "CN = hub-control-plane" >> san.cnf && \
	echo "" >> san.cnf && \
	echo "[ v3_req ]" >> san.cnf && \
	echo "subjectAltName = @alt_names" >> san.cnf && \
	echo "" >> san.cnf && \
	echo "[ alt_names ]" >> san.cnf && \
	echo "DNS.1 = hub-control-plane" >> san.cnf && \
	echo "Generating Server Key + CSR + Signed Cert..." && \
	openssl genrsa -out server.key 2048 && \
	openssl req -new -key server.key -out server.csr -config san.cnf && \
	openssl x509 -req -in server.csr -CA root-ca.crt -CAkey root-ca.key \
		-CAcreateserial -out server.crt -days 825 -sha256 -extensions v3_req -extfile san.cnf
	@echo "Certificates generated in $(CERTS_DIR)/"

# Install kube-prometheus-stack
install-prometheus: generate-certs
	@echo "Installing kube-prometheus-stack..."
	@# Add prometheus-community helm repo
	helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
	helm repo update
	@# Create namespace if it doesn't exist
	kubectl --context $(CONTEXT) create namespace $(PROMETHEUS_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@# Create prometheus-tls secret in monitoring namespace
	kubectl --context $(CONTEXT) create secret generic prometheus-tls -n $(PROMETHEUS_NAMESPACE) \
		--from-file=server.crt=$(CERTS_DIR)/server.crt \
		--from-file=server.key=$(CERTS_DIR)/server.key \
		--from-file=client-ca.crt=$(CERTS_DIR)/client-ca.crt \
		--dry-run=client -o yaml | kubectl apply -f -
	@# Install prometheus stack with TLS configuration
	helm upgrade --install prometheus-stack prometheus-community/kube-prometheus-stack \
		--namespace $(PROMETHEUS_NAMESPACE) \
		--set prometheus.prometheusSpec.enableRemoteWriteReceiver=true \
		--set prometheus.prometheusSpec.web.tlsConfig.keySecret.key=server.key \
		--set prometheus.prometheusSpec.web.tlsConfig.keySecret.name=prometheus-tls \
		--set prometheus.prometheusSpec.web.tlsConfig.cert.secret.key=server.crt \
		--set prometheus.prometheusSpec.web.tlsConfig.cert.secret.name=prometheus-tls \
		--set prometheus.prometheusSpec.web.tlsConfig.client_ca.secret.key=client-ca.crt \
		--set prometheus.prometheusSpec.web.tlsConfig.client_ca.secret.name=prometheus-tls \
		--set prometheus.prometheusSpec.web.tlsConfig.clientAuthType=RequestClientCert \
		--set prometheus.service.type=NodePort \
		--set prometheusOperator.enabled=true
	@echo "kube-prometheus-stack installed successfully"

# Install opentelemetry-addon chart
install-addon:
	@echo "Installing opentelemetry-addon..."
	@# Create hub namespace if it doesn't exist
	kubectl --context $(CONTEXT) create namespace $(HUB_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@# Create addon namespace if it doesn't exist
	kubectl --context $(CONTEXT) create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@# Create otel-signer secret in hub namespace
	kubectl --context $(CONTEXT) create secret tls otel-signer -n $(HUB_NAMESPACE) \
		--cert=$(CERTS_DIR)/client-ca.crt \
		--key=$(CERTS_DIR)/client-ca.key \
		--dry-run=client -o yaml | kubectl apply -f -
	@# Install the opentelemetry-addon chart
	helm upgrade --install opentelemetry-addon $(CHART_DIR) \
		--namespace $(NAMESPACE) \
		--set opentelemetryCollector.tls.enabled=true \
		--set promeRemoteWriteEndpoint="https://hub-control-plane:30090/api/v1/write" \
		--set-file promWebRootCACert=$(CERTS_DIR)/root-ca.crt
	@echo "opentelemetry-addon installed successfully"

# Install opentelemetry-addon chart without TLS (for existing Prometheus)
install-addon-no-tls:
	@echo "Installing opentelemetry-addon without TLS..."
	@# Create addon namespace if it doesn't exist
	kubectl --context $(CONTEXT) create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@# Install the opentelemetry-addon chart without TLS
	helm upgrade --install opentelemetry-addon $(CHART_DIR) \
		--namespace $(NAMESPACE) \
		--set promeRemoteWriteEndpoint="http://hub-control-plane:30090/api/v1/write" \
		--set opentelemetryCollector.tls.enabled=false
	@echo "opentelemetry-addon installed successfully (no TLS)"

# Clean up installations
clean:
	@echo "Cleaning up installations..."
	@helm uninstall opentelemetry-addon -n $(NAMESPACE) || true
	@helm uninstall prometheus-stack -n $(PROMETHEUS_NAMESPACE) || true
	@kubectl delete namespace $(NAMESPACE) || true
	@kubectl delete namespace $(PROMETHEUS_NAMESPACE) || true
	@rm -rf $(CERTS_DIR)
	@echo "Cleanup completed"

test-e2e:
	@echo "Skipping e2e tests (not implemented)"
	@true
.PHONY: test-e2e

verify:
	@echo "Skipping verify (not implemented)"
	@true
.PHONY: verify

build:
	@echo "Skipping build (not implemented)"
	@true
.PHONY: build

test-unit:
	@echo "Skipping unit tests (not implemented)"
	@true
.PHONY: test-unit

test-integration:
	@echo "Skipping integration tests (not implemented)"
	@true
.PHONY: test-integration

.PHONY: test-chart
test-chart: ## Run chart tests
	@echo "Skipping chart tests (not implemented)"
	@true
