FROM golang:1.20 AS builder
ARG OS=linux
ARG ARCH=amd64
WORKDIR /go/src/open-cluster-management.io/addon-contrib/clusternet-addon
COPY . .
RUN GOOS=${OS} \
    GOARCH=${ARCH} \
    make build

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
ENV USER_UID=10001

COPY --from=builder /go/src/open-cluster-management.io/addon-contrib/clusternet-addon/clusternet /

USER ${USER_UID}
