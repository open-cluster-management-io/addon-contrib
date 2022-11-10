FROM golang:1.19 AS builder
WORKDIR /go/src/open-cluster-management.io/addon-contrib/hellospoke-addon
COPY . .
ENV GO_PACKAGE open-cluster-management.io/addon-contrib/hellospoke-addon

# Build
RUN make build --warn-undefined-variables

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

# Add the binaries
COPY --from=builder go/src/open-cluster-management.io/addon-contrib/hellospoke-addon/bin/hellospoke-addon .

ENTRYPOINT ["/hellospoke-addon"]
