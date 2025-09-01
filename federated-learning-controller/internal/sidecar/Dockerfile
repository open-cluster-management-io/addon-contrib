FROM golang:1.24 AS builder

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o fl_sidecar main.go

FROM alpine:latest

COPY --from=builder /app/fl_sidecar /fl_sidecar

ENTRYPOINT ["/fl_sidecar"]
