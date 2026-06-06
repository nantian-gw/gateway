ARG GO_IMAGE=docker.io/library/golang:1.26-bookworm
ARG RUNTIME_IMAGE=docker.io/library/debian:bookworm-slim
FROM ${GO_IMAGE} AS builder

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /src

COPY controlplane/go.mod controlplane/go.sum ./controlplane/
COPY controlplane/vendor/ ./controlplane/vendor/
WORKDIR /src/controlplane

WORKDIR /src
COPY controlplane/ ./controlplane/
RUN cd ./controlplane \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -trimpath -o /out/nantian-controlplane ./cmd/manager

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
COPY --from=builder /out/nantian-controlplane /usr/local/bin/nantian-controlplane

ENTRYPOINT ["/usr/local/bin/nantian-controlplane"]