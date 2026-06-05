# Getting Started

This document is intended for "end users." The goal is to get Aether Gateway running and complete basic validation with as little prerequisite knowledge as possible.
If you need to modify code or extend capabilities, please read [Development Documentation](../development.md).

## 1. Kind Quick Start

This is the most direct way to try out the current repository. The script automatically performs the following actions:

- Prepares a local registry `kind-registry`.
- Reuses an existing Kind cluster, rebuilding only when necessary.
- Caches Gateway API CRDs to `tmp/kind/gateway-api-crds/`.
- Builds and pushes control plane and data plane images to the local registry.
- Deploys base manifests and smoke test cases.

### 1.1 Prerequisites

- Docker
- Kind
- `kubectl`
- `curl`
- `jq`

### 1.2 Start the Cluster

Run at the repository root:

```bash
./tests/e2e/run-kind.sh
```

If you already have reusable images, skip the build:

```bash
SKIP_BUILD=true ./tests/e2e/run-kind.sh
```

If you only want to deploy base components without running smoke tests:

```bash
SKIP_SMOKE=true ./tests/e2e/run-kind.sh
```

Only rebuild the cluster when Kind configuration, registry patches, or port mappings are abnormal:

```bash
RECREATE_CLUSTER=true ./tests/e2e/run-kind.sh
```

### 1.3 Verify the Data Path

The default smoke test cases create:

- `GatewayClass`: `aether`
- `Gateway`: `aether-gateway/edge`
- `HTTPRoute`: `aether-gateway/echo`
- Backend `Service`: `aether-gateway/echo`

Kind maps the shared dataplane Service's NodePorts to the host by default:

| Gateway listener | NodePort | Host port |
| --- | ---: | ---: |
| HTTP `80/TCP` | `30080` | `18080` |
| HTTPS/TLS `443/TCP` | `30443` | `18443` |
| UDP `5300/UDP` | `31300` | `5300` |
| UDP failure smoke `5301/UDP` | `31301` | `5301` |
| TCPRoute `9000/TCP` | `32000` | `19000` |
| TCPRoute failure smoke `9001/TCP` | `32001` | `19001` |

So you can directly verify the HTTP data path:

```bash
curl -fsS -H 'Host: example.com' http://127.0.0.1:18080/
```

Expected response:

```text
aether-gateway-ok
```

TCPRoute smoke can also be verified directly from the host:

```bash
printf 'GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n' \
  | nc -w 5 127.0.0.1 19000
```

### 1.4 View Running Resources

```bash
kubectl -n aether-gateway get pods
kubectl -n aether-gateway get svc
kubectl -n aether-gateway get gateway
kubectl -n aether-gateway get httproute
```

## 2. Accessing Admin APIs

### 2.1 Control Plane Admin API

The control plane admin API is a cluster-internal `Service` by default. Access is recommended via `port-forward`:

```bash
kubectl -n aether-gateway port-forward svc/aether-gateway-controlplane-admin 18081:18081
```

Then in another terminal:

```bash
curl -fsS http://127.0.0.1:18081/livez
curl -fsS http://127.0.0.1:18081/readyz
curl -fsS http://127.0.0.1:18081/v1/summary | jq
curl -fsS http://127.0.0.1:18081/v1/snapshot-sync | jq
curl -fsS http://127.0.0.1:18081/v1/nodes | jq
```

### 2.2 Data Plane Admin API

The data plane admin API is only exposed within the cluster via `ClusterIP` by default. Use `port-forward`:

```bash
kubectl -n aether-gateway port-forward svc/aether-gateway-dataplane-admin 19080:19080
```

Then access from another terminal:

```bash
curl -fsS http://127.0.0.1:19080/livez
curl -fsS http://127.0.0.1:19080/readyz
curl -fsS http://127.0.0.1:19080/v1/summary | jq
curl -fsS http://127.0.0.1:19080/v1/listener-statuses | jq
```

For API details see [Admin API Documentation](admin-api.md).

### 2.3 Optional Admin Auth

If you have configured `adminAuth.bearerToken` or `adminAuth.bearerTokenFile` for the control plane or data plane, all admin endpoints except `/livez` and `/readyz` require a Bearer Token:

```bash
curl -fsS -H "Authorization: Bearer ${PGW_ADMIN_TOKEN}" http://127.0.0.1:18081/v1/summary | jq
curl -fsS -H "Authorization: Bearer ${PGW_ADMIN_TOKEN}" http://127.0.0.1:19080/v1/summary | jq
```

For more complete deployment methods, see [Production Operations Documentation](operations.md).

## 3. Local Process Mode

If you already have an accessible Kubernetes cluster and Gateway API CRDs, you can also run the control plane and data plane processes directly on the host.
This approach is more suitable for local debugging and is not the recommended path for first-time trials.

### 3.1 Prerequisites

- Current `kubectl` / kubeconfig can connect to the target cluster.
- The cluster has Gateway API CRDs installed.
- Go, Rust, and `protoc` are installed.

### 3.2 Start the Control Plane

```bash
make proto
cd controlplane
go run ./cmd/manager -config ../configs/controlplane/config.yaml
```

Under default configuration:

- gRPC listener: `:18080`
- admin listener: `:18081`
- metrics listener: `:18082`
- health probe listener: `:18083`

These ports bind to all local addresses on the host by default; during same-host debugging, access them via `127.0.0.1:18080`, `127.0.0.1:18081`, `127.0.0.1:18082`.
The local process sample config `configs/controlplane/config.yaml` still explicitly sets `statusAddress: 127.0.0.1`, suitable only for same-host debugging; Kind, multi-node, or production environments should not publish loopback addresses. Without a truly reachable `statusAddress` / `statusAddresses` or Service `LoadBalancer ingress` / `externalIPs`, `Gateway.status.addresses` will remain empty.

### 3.3 Start the Data Plane

```bash
cargo run --manifest-path dataplane/Cargo.toml -p aeg-app -- \
  --config ../configs/dataplane/config.yaml
```

Under default configuration:

- External HTTP listener: `0.0.0.0:10080`
- admin: `127.0.0.1:19080`

During same-host debugging, you can still access `http://127.0.0.1:10080`.
The config option `runtime.enableHttp3` is currently only a capability flag; the current build still reports `http3Available=false`, so local quick-start validation should focus on HTTP/1.1, HTTP/2, or HTTPS.

### 3.4 Local Validation

```bash
curl -fsS http://127.0.0.1:18081/v1/summary | jq
curl -fsS http://127.0.0.1:19080/v1/summary | jq
curl -fsS http://127.0.0.1:19080/livez
```

## 4. Common Issues

### 4.1 Kind Startup Is Slow

First confirm whether you really need to rebuild the cluster. By default, reuse the existing Kind cluster rather than deleting and recreating.

### 4.2 Image Pull Failure

First check whether the local registry and existing tags can be reused:

```bash
SKIP_BUILD=true ./tests/e2e/run-kind.sh
```

If a rebuild is necessary, confirm that `kind-registry` is still running and `tmp/kind/last-image-tag` exists.

### 4.3 Control Plane Compilation Complains About Missing Generated Code

Run first:

```bash
make proto
```

This type of issue is usually caused by missing generated code, not an error in the `go.mod` dependency declarations.