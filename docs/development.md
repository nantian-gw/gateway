# Development Documentation

This document is intended for repository developers and contributors.
If you want to deploy, try out, or troubleshoot, please read the [User Documentation](user/getting-started.md) first.

## 1. Documentation Entry Points

- [Architecture Documentation](architecture.md): Understand the boundaries between the control plane, data plane, and IR.
- [Design Documentation](design.md): Understand components, interfaces, protocols, and runtime design.
- [Test Plan](test/plan.md): Choose the execution level for unit tests, local integration, Kind smoke, and conformance.
- [User Documentation](user/getting-started.md): View deployment, usage, and management interface instructions.
- [Deployment Directory README](../deploy/README.md): Understand the purpose of `deploy/kubernetes/base`, `overlays/kind`, `overlays/production`, and fixed Services.

## 2. Environment Requirements

- Go 1.26+
- Rust 1.88.0+ (MSRV pinned via `dataplane/rust-toolchain.toml`)
- `protoc`
- Docker
- Kind

Recommended tools:

- `cargo fmt`
- `gofmt`
- `kubectl`
- `jq`

## 3. Repository Structure

```text
controlplane/   Go control plane
dataplane/      Rust data plane workspace
proto/          gRPC protocol definitions
configs/        YAML configuration examples
deploy/         Kubernetes/Kind scaffolding
tests/          Kind smoke / e2e / conformance workflows
scripts/        General-purpose scripts
docs/           Developer and user documentation
```

## 4. Recommended Development Order

Unless the user specifies otherwise, follow this order:

1. First, build basic capabilities and unit tests.
2. Then, build local integration debugging capabilities.
3. Finally, do e2e and conformance.

Do not default to starting Kind after every change.
If the current issue can be verified through unit tests, partial integration, management interfaces, or local process debugging, do not start a cluster first.

## 5. Pre-Change Checklist

Before starting to code, at minimum confirm the following:

- Whether the current functional block is sufficiently independent to be committed separately.
- Whether there is already a lighter-weight verification method available, to avoid directly running Kind.
- Whether the files to be changed are already too large and should be split into modules first.
- Whether the data structures between the control plane and data plane are consistent.
- Whether documentation, configuration examples, and management interfaces need to be updated in sync.

## 6. Common Commands

### 6.1 Generate Proto

```bash
make proto
```

### 6.2 Build

```bash
make build
```

### 6.3 Unit Tests

```bash
make test-unit
cd controlplane && go test ./...
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

### 6.4 Kind e2e

```bash
make test-e2e
```

### 6.5 Conformance

```bash
make test-conformance
```

### 6.6 Automatic Lowest-Cost Validation Based on Changes

```bash
make test-targeted
PLAN_ONLY=true ./scripts/run-targeted-validation.sh
INCLUDE_KIND=true SKIP_BUILD=true ./scripts/run-targeted-validation.sh
```

By default, it automatically selects the cheapest validation commands based on the current workspace diff, for example:

- `proto/`: `make proto` + Go/Rust unit tests
- `controlplane/`: `go test ./...`
- `dataplane/`: `cargo test --workspace`
- `*.sh`: `bash -n`

The script output also provides the selection reason for each command, making it easy to quickly determine "why these validations need to run" in both local and CI environments.

If the changes hit `deploy/`, `tests/e2e/`, `tests/conformance/`, or metrics/admin surface areas that are more cluster-behavior-oriented, the script by default places Kind smoke or targeted E2E into the `skipped validations` section, with the skip reason and enablement method clearly stated; Kind cluster startup is avoided for every small change by only including `./tests/e2e/run-kind.sh` and corresponding targeted scripts in the plan when `INCLUDE_KIND=true` is explicitly passed.

## 7. Local Development Workflow

Recommended workflow:

1. When modifying `proto/` or IR, update documentation first.
2. Run `make proto`.
3. Execute Go/Rust unit tests separately.
4. Do local process integration debugging when necessary.
5. Only run Kind when deployment, NodePort, Gateway behavior, or cluster interaction needs to be verified.

### 7.1 Running the Control Plane Locally

The control plane depends on the current kubeconfig and Kubernetes API. Before starting, ensure the local context can access the target cluster and the cluster has Gateway API CRDs installed.

```bash
cd controlplane
go run ./cmd/manager -config ../configs/controlplane/config.yaml
```

Default configuration:

- gRPC: `:18080`
- admin: `:18081`
- metrics: `:18082`
- health probe: `:18083`
- `snapshot-syncer` settle delay: `syncSettleDelay=200ms`
- controlplane aggregate convergence runner: `reconcilerRunner.settleDelay=300ms`, `reconcilerRunner.retryBackoff=1s`

These listen addresses use the `:port` form, which binds to all local addresses on the host machine.
When debugging on the same machine, they are typically still accessed via `127.0.0.1:18080`, `127.0.0.1:18081`, etc.

### 7.2 Running the Data Plane Locally

```bash
cargo run --manifest-path dataplane/Cargo.toml -p aeg-app -- \
  --config ../configs/dataplane/config.yaml
```

Default configuration:

- Traffic entry: `0.0.0.0:10080`
- admin: `127.0.0.1:19080`

When debugging on the same machine, although the traffic entry binds to `0.0.0.0:10080`, it is typically still accessed via `http://127.0.0.1:10080`.
The configuration option `runtime.enableHttp3` currently only appears in configuration, summary, and metrics; the current build will still report `http3Available=false`. Do not base local debugging on HTTP/3.

If you need to verify production configuration locally:

- The control plane can enable management interface Bearer authentication via `adminAuth.bearerToken` or `adminAuth.bearerTokenFile`.
- The control plane gRPC can enable TLS or mTLS via `grpcTLS`.
- The data plane can enable TLS or mTLS to the control plane via `xdsTls`.

### 7.2.1 Local Log Debugging

Common logging configuration:

- Control plane: `log.level`, `log.format`, `log.addSource`
- Data plane: `log.level`, `log.format`, `log.addSource`, `log.includeTarget`, `log.includeThreadIds`, `log.includeThreadNames`

Common access log configuration:

- `accessLog.enabled`
- `accessLog.path`
- `accessLog.format`
- `accessLog.mode`
- `accessLog.sampleRate`
- `accessLog.routeAnnotationPrefix`

Default recommendations:

- Both data plane system logs and access logs should use JSON line format.
- `accessLog.path` should prefer `stdout`, `stderr`, or a separate file; do not mix text access logs with JSON system logs.

The data plane outputs the following business events by default:

- `http_request`
- `tcp_session`
- `tls_session`
- `udp_datagram`

To increase log volume or switch log files for a specific Route only, prefer using annotations rather than changing global configuration. The default prefix is `gateway.nantian.dev/access-log-`, for example:

```yaml
metadata:
  annotations:
    gateway.nantian.dev/access-log-enabled: "true"
    gateway.nantian.dev/access-log-mode: "json"
    gateway.nantian.dev/access-log-path: "/var/log/aether/orders-access.jsonl"
    gateway.nantian.dev/access-log-sample-rate: "1.0"
```

Currently supported template variables include:

- `%TIMESTAMP%`
- `%EVENT%`
- `%LISTENER%`
- `%PROTOCOL%`
- `%REQUEST_ID%`
- `%ROUTE_NAMESPACE%`
- `%ROUTE_KIND%`
- `%BYTES_RECEIVED%`
- `%BYTES_SENT%`
- `%RESPONSE_FLAGS%`

### 7.3 Frequently Used Endpoints for Local Debugging

Control plane:

- `GET /livez`
- `GET /readyz`
- `GET /v1/summary`
- `GET /v1/snapshot-sync`
- `GET /v1/snapshot`
- `GET /v1/listeners`
- `GET /v1/routes`
- `GET /v1/backends`
- `GET /v1/nodes`

Data plane:

- `GET /livez`
- `GET /readyz`
- `GET /metrics`
- `GET /v1/summary`
- `GET /v1/node`
- `GET /v1/snapshot`
- `GET /v1/listeners`
- `GET /v1/routes`
- `GET /v1/backends`

## 8. Kind Debugging Principles

`tests/e2e/run-kind.sh` by default adopts a "reusable cluster + local registry + local cache" model:

1. Ensure the `kind-registry` local image registry exists, with data persisted to `tmp/kind/registry-data/`.
2. Create the Kind cluster if it does not exist; if it does, reuse it by default instead of deleting and recreating every time.
3. Cache Gateway API CRDs to `tmp/kind/gateway-api-crds/` for subsequent reuse of local files.
4. Build control plane and data plane images and push them to the local registry.
5. Render Kind-specific deployment manifests to avoid pulling host machine temporary tags inside the cluster.
6. Deploy smoke test cases as needed and perform basic connectivity checks.

Common environment variables:

- `RECREATE_CLUSTER=true`: Force delete and recreate the Kind cluster.
- `SKIP_BUILD=true`: Skip image building, reuse the previously recorded image tag.
- `SKIP_SMOKE=true`: Only deploy basic components, do not run smoke business traffic checks.
- `IMAGE_TAG=<tag>`: Explicitly specify the image tag to deploy.
- `LOCAL_REGISTRY_PORT=<port>`: Change the local registry exposed port, default `5001`.

Recommended usage:

```bash
./tests/e2e/run-kind.sh
SKIP_BUILD=true ./tests/e2e/run-kind.sh
SKIP_SMOKE=true ./tests/e2e/run-kind.sh
RECREATE_CLUSTER=true ./tests/e2e/run-kind.sh
```

Usage principles:

- When only changing control plane management logic or Rust matching logic, run unit tests first, do not start Kind directly.
- When only changing deployment manifests or cluster behavior, use `SKIP_BUILD=true` to reuse existing images.
- Only use `RECREATE_CLUSTER=true` when Kind configuration, containerd registry patches, or network state is inconsistent.

## 9. Code and Commit Process Constraints

- Each code file should not exceed 800 lines where possible.
- Prefer modular splitting; do not write "monolithic" files.
- HTTP/GRPC logic should rely on native Rust proxy capabilities where possible.
- Stream protocol should be placed in a separate crate, not mixed with HTTP crate.
- One commit should do only one thing; do not mix documentation, runtime, tests, refactoring, and unrelated fixes together.
- Do not amend existing commits unless explicitly requested by the user.

Recommended commit granularity:

- One independent feature.
- One independent infrastructure change.
- One independent test framework integration.
- One independent documentation addition.

## 10. Minimum Verification After Changes

If the control plane is changed:

```bash
cd controlplane && go test ./...
```

If the data plane is changed:

```bash
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

If shared protocols or interaction logic between both sides is changed:

```bash
make proto
cd controlplane && go test ./...
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

If only documentation or scripts are changed, at minimum perform the corresponding static validation, for example:

```bash
bash -n tests/e2e/run-kind.sh
```

## 11. Mainland China Environment Recommendations

### 11.1 GitHub Proxy

When pulling raw files or repositories, you can prepend the following to the address:

```text
https://gh-proxy.com/
```

For example:

```bash
git clone https://gh-proxy.com/https://github.com/kubernetes-sigs/gateway-api.git
```

### 11.2 Docker Hub Proxy

The following image prefixes can be used:

- `docker.1ms.run`
- `m.daocloud.io/docker.io/`

For example:

```bash
docker pull m.daocloud.io/docker.io/library/golang:1.26
docker pull m.daocloud.io/docker.io/kindest/node:v1.30.6
```

## 12. One-Sentence Principle

First verify using the cheapest method, then escalate to more expensive validation levels.
After completing each independent functional block, make a clean commit.
