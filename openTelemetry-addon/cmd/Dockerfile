FROM golang:1.18 as builder

WORKDIR /workspace

ARG ADDON_AGENT_IMAGE_NAME

# ARG ADDON_AGENT_IMAGE_NAME

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

# Build addons
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o agent cmd/addon-agent/main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a \
        -ldflags="-X 'otel-add-on/pkg/config.AgentImageName=${ADDON_AGENT_IMAGE_NAME}'" \
        -o manager cmd/addon-manager/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine:3.13

WORKDIR /
RUN apk add libc6-compat
COPY --from=builder /workspace/agent /workspace/manager ./
USER 65532:65532
