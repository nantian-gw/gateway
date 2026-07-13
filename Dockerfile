ARG GO_IMAGE=docker.io/library/golang:1.26-bookworm
ARG RUNTIME_IMAGE=gcr.io/distroless/static:nonroot
FROM ${GO_IMAGE} AS builder

ARG GOPROXY
ENV GOPROXY=${GOPROXY}

WORKDIR /src

COPY go.mod go.sum ./
COPY gen/ ./gen/
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -ldflags="-s -w" -trimpath -o /out/nantian-controlplane ./cmd/manager

FROM ${RUNTIME_IMAGE}

COPY --from=builder /out/nantian-controlplane /usr/local/bin/nantian-controlplane

USER 65532:65532

ENTRYPOINT ["/usr/local/bin/nantian-controlplane"]

# ──────────────────────────────────────────────
# Debug image (for troubleshooting, not for production)
# Build with: docker build --target=debug -t nantian-controlplane:debug .
# ──────────────────────────────────────────────
FROM debian:bookworm-slim AS debug
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl dnsutils iproute2 netcat-openbsd procps tcpdump \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/nantian-controlplane /usr/local/bin/nantian-controlplane
ENTRYPOINT ["/usr/local/bin/nantian-controlplane"]
