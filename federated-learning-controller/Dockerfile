# Build the unified binary (controller + sidecar)
FROM golang:1.23 AS builder
ARG OS=linux
ARG ARCH=amd64

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build unified binary
RUN CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} go build -a -o manager cmd/main.go

# Use UBI minimal as base image for consistency with other addons
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
ENV USER_UID=10001

WORKDIR /
COPY --from=builder /workspace/manager .
USER ${USER_UID}

# Default to controller mode
# Override with: docker run <image> --mode=sidecar [sidecar-flags]
ENTRYPOINT ["/manager"]
CMD ["--mode=controller"]
