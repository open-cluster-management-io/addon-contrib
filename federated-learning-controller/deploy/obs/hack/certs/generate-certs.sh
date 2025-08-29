#!/bin/bash

# Generate Root CA
openssl genrsa -out root-ca.key 4096
openssl req -x509 -new -nodes -key root-ca.key -sha256 -days 3650 \
  -subj "/CN=My Root CA" \
  -out root-ca.crt

# Generate Client CA
openssl genrsa -out client-ca.key 4096
openssl req -x509 -new -nodes -key client-ca.key -sha256 -days 3650 \
  -subj "/CN=My Client CA" \
  -out client-ca.crt

# Generate Server Key + CSR + Signed Cert
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr -config san.cnf

# Sign server cert with Root CA
openssl x509 -req -in server.csr -CA root-ca.crt -CAkey root-ca.key \
  -CAcreateserial -out server.crt -days 825 -sha256 -extensions v3_req -extfile san.cnf

# Remove the secrets if they exist
kubectl --context kind-hub delete secret prometheus-tls -n monitoring --ignore-not-found
kubectl --context kind-hub delete secret otel-signer -n open-cluster-management-hub --ignore-not-found

# create prometheus-tls secret in monitoring namespace
kubectl --context kind-hub create secret generic prometheus-tls -n monitoring \
  --from-file=server.crt=server.crt\
  --from-file=server.key=server.key \
  --from-file=client-ca.crt=client-ca.crt

# create otel-signer secret in open-cluster-management-hub namespace
kubectl --context kind-hub create secret tls otel-signer -n open-cluster-management-hub \
  --cert=client-ca.crt\
  --key=client-ca.key

# replace root-ca.crt in deploy/resources/addon-template.yaml
awk '{print "              " $0}' root-ca.crt > root-ca.crt.tmp
# Cross-platform sed -i compatibility
if [[ "$OSTYPE" == "darwin"* ]]; then
    sed -i '' "/PROM_WEB_ROOT_CA/{
        r root-ca.crt.tmp
        d
    }" ../../otel-addon/resources/addon-template.yaml
else
    sed -i "/PROM_WEB_ROOT_CA/{
        r root-ca.crt.tmp
        d
    }" ../../otel-addon/resources/addon-template.yaml
fi
rm root-ca.crt.tmp
