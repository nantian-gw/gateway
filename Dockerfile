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
