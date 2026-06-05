ARG GO_IMAGE=docker.io/library/golang:1.26-bookworm
ARG RUNTIME_IMAGE=docker.io/library/debian:bookworm-slim
FROM ${GO_IMAGE} AS builder

WORKDIR /src

COPY controlplane/go.mod controlplane/go.sum ./controlplane/
WORKDIR /src/controlplane
RUN go mod download

WORKDIR /src
COPY controlplane/ ./controlplane/
RUN cd ./controlplane \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -trimpath -o /out/aether-gateway-controlplane ./cmd/manager

FROM ${RUNTIME_IMAGE}

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        dnsutils \
        iproute2 \
        netcat-openbsd \
        procps \
        tcpdump \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/aether-gateway-controlplane /usr/local/bin/aether-gateway-controlplane

ENTRYPOINT ["/usr/local/bin/aether-gateway-controlplane"]