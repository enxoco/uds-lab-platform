# Lab server image — runs inside the cluster so it can reach session pod DNS.
# Used for local dev with the pod provider (PROVIDER_TYPE=pod).
#
# Build:
#   docker build -t ghcr.io/enxoco/uds-labserver:dev -f docker/labserver/Dockerfile .
FROM golang:1.26 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o labserver ./cmd/labserver/

FROM ubuntu:24.04
RUN apt-get update -q && apt-get install -y -q ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/labserver /usr/local/bin/labserver
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/labserver"]
