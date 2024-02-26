FROM golang:1.21 AS builder
WORKDIR /go/src/open-cluster-management.io/addon-contrib/resource-usage-collect
COPY . .
ENV GO_PACKAGE open-cluster-management.io/addon-contrib/resource-usage-collect

RUN make build --warn-undefined-variables

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
COPY --from=builder /go/src/open-cluster-management.io/addon-contrib/resource-usage-collect/addon /
