ARG GO_IMAGE=docker.io/library/golang:1.26-bookworm
ARG RUNTIME_IMAGE=docker.io/library/debian:bookworm-slim
FROM ${GO_IMAGE} AS builder

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /src

COPY go.mod go.sum ./
COPY gen/ ./gen/
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -ldflags="-s -w" -trimpath -o /out/nantian-controlplane ./cmd/manager

FROM ${RUNTIME_IMAGE}

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/nantian-controlplane /usr/local/bin/nantian-controlplane

USER 65532

ENTRYPOINT ["/usr/local/bin/nantian-controlplane"]