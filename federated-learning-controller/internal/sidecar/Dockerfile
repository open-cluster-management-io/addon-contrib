FROM golang:1.24 AS builder
ARG OS=linux
ARG ARCH=amd64

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} go build -o fl_sidecar main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
ENV USER_UID=10001

COPY --from=builder /app/fl_sidecar /fl_sidecar

USER ${USER_UID}

ENTRYPOINT ["/fl_sidecar"]
