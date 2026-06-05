# Aether Gateway Security Regression Execution Template

This document turns the `SEC-*` items from `docs/test/plan.md` into a set of repeatable execution templates.
The goal is not to replace a full security audit, but to ensure at least one round of structured security regression before every major release.

## 1. Scope

Primarily intended for the following types of changes:

- admin authentication, probe bypass, management interface changes
- `ReferenceGrant`, cross-namespace binding, or certificate references
- `BackendTLSPolicy`, frontend `frontendValidation`, client certificates
- request parsing, header handling, connection reuse, timeout and connection management
- centralized security regression before release

## 2. Prerequisites

### 2.1 Basic Environment

- `go`, `cargo`, `curl`, `kubectl`, `kind`, `jq`, `openssl` already available
- Also recommended:
  - `osv-scanner`
  - `trivy` or `grype`
  - `socat`

### 2.2 Recommended Execution Tiers

Execute from lowest to highest cost:

1. Pure package tests and runtime tests
2. Kind specialized E2E
3. Raw protocol and resource exhaustion tests
4. Supply chain and image scanning

## 3. Evidence Directory

It is recommended to fix an evidence directory before each execution:

```bash
export SECURITY_EVIDENCE_DIR="tmp/test-evidence/security-$(date +%Y%m%d%H%M%S)"
mkdir -p "${SECURITY_EVIDENCE_DIR}"
```

Collect admin snapshots both before and after execution:

```bash
OUTPUT_DIR="${SECURITY_EVIDENCE_DIR}/admin-before" \
  ./scripts/collect-admin-snapshots.sh

# Execute security regression steps

OUTPUT_DIR="${SECURITY_EVIDENCE_DIR}/admin-after" \
  ./scripts/collect-admin-snapshots.sh
```

If this is a Kind environment and the local machine has not set up port forwarding for the admin Service, you can let the script set up a temporary forward:

```bash
ENABLE_KIND_PORT_FORWARD=true \
OUTPUT_DIR="${SECURITY_EVIDENCE_DIR}/admin-before" \
  ./scripts/collect-admin-snapshots.sh
```

## 4. Minimum Security Regression Set

### 4.1 Package Tests and Runtime Tests

Control plane:

```bash
cd controlplane
go test ./internal/backendtls ./internal/grpcserver ./internal/status
```

Data plane:

```bash
cargo test --manifest-path dataplane/Cargo.toml -p aeg-http --lib
```

This step should at least cover the following security-related paths:

- backend CA bundle missing or invalid must not be bypassed
- `Hostname`/`URI` SAN and multi-SAN combinations take effect per explicit configuration; empty values, relative URIs, and unsupported SAN types are rejected
- listener `certificateRefs`, frontend `caCertificateRefs`, backend `clientCertificateRef` kind/group support boundaries must fail closed
- client certificate missing or invalid
- HTTPS listener frontend validation bundle handling

Representative test names in the current repository directly related to this regression set include:

- `TestParseSubjectAltNamesAcceptsURIAndHostnameEntries`
- `TestParseSubjectAltNamesAcceptsMultipleHostnames`
- `TestParseSubjectAltNamesRejectsRelativeURI`
- `TestReconcileRejectsUnsupportedFrontendValidationCARefKind`
- `proxy::tests::backend_tls_validation::peer_tls::build_upstream_peer_uses_backend_tls_validation_hostname`
- `proxy::tests::backend_tls_validation::subject_alt_names::build_upstream_peer_uses_post_handshake_subject_alt_name_validation`
- `proxy::tests::backend_tls_validation::subject_alt_names::build_upstream_peer_accepts_uri_subject_alt_name_validation`
- `proxy::tests::backend_tls_validation::subject_alt_names::backend_certificate_matches_any_configured_subject_alt_name`
- `TestBuildSnapshotSkipsBackendClientCertificateRefWithUnsupportedKind`
- `proxy::tests::backend_client_cert::build_upstream_peer_uses_client_certificate_for_tls_backends`
- `runtime::tests::builds_https_listener_with_frontend_validation_bundle`

### 4.2 Cross-Namespace Authorization Boundaries

Execute:

```bash
./tests/e2e/validate-reference-grants.sh
./tests/e2e/validate-grpc-reference-grants.sh
./tests/e2e/validate-gateway-cross-namespace-certs.sh
```

Pass criteria:

- Traffic must fail before authorization
- Behavior must recover after creating `ReferenceGrant`
- Behavior must fail again after deleting `ReferenceGrant`
- Control plane state and actual traffic must be consistent

### 4.3 Admin Authentication Boundaries

If the target environment has `adminAuth` enabled, at least cover the following two types of requests:

```bash
./tests/e2e/validate-admin-token-rotation.sh

curl -si http://127.0.0.1:18081/livez
curl -si http://127.0.0.1:19080/readyz

curl -si http://127.0.0.1:18081/v1/summary
curl -si http://127.0.0.1:19080/v1/summary

curl -si -H "Authorization: Bearer ${PGW_ADMIN_TOKEN}" \
  http://127.0.0.1:18081/v1/summary
curl -si -H "Authorization: Bearer ${PGW_ADMIN_TOKEN}" \
  http://127.0.0.1:19080/v1/summary
```

Pass criteria:

- `/livez`, `/readyz` remain anonymously accessible
- `/v1/*` and `/metrics` return `401` or equivalent rejection when no token is provided
- Normal access is restored with the correct token

If the current environment does not have `adminAuth` enabled, record this item as `N/A`, but do not record it as “verified passed”.

### 4.4 TLS / SAN / mTLS

Backend TLS and SAN:

```bash
./tests/e2e/validate-gateway-cross-namespace-certs.sh
```

Frontend mTLS and client certificate scenarios are recommended to be executed in an environment with `frontendValidation` enabled.
Client certificates directly reusable in the repository are located at:

- `tests/testdata/tls/client.crt`
- `tests/testdata/tls/client.key`

Example command template:

```bash
openssl s_client \
  -connect 127.0.0.1:18443 \
  -servername mtls.example.com \
  -cert tests/testdata/tls/client.crt \
  -key tests/testdata/tls/client.key \
  -quiet
```

At minimum, simultaneously verify:

- valid client cert passes
- missing client cert is rejected
- wrong CA or SAN mismatch is rejected
- `ReferenceGrant` still takes effect for cross-namespace `frontendValidation` CA references

### 4.5 Raw Protocol Security Regression

The current repository already has a standard script for this regression set:

```bash
./tests/e2e/validate-http-security.sh
```

The script deploys a dedicated inspect backend in the Kind environment and actually covers:

- `CL/TE`
- `TE/CL`
- malformed chunked
- duplicate `Host`
- conflicting `Content-Length`
- oversized headers
- `Host`/`X-Forwarded-*` spoofing
- lightweight slow-header probing

Optional modes:

```bash
KEEP_RESOURCES=true ./tests/e2e/validate-http-security.sh
OVERSIZED_HEADER_BYTES=524288 SLOW_CONNECTIONS=16 ./tests/e2e/validate-http-security.sh
```

Minimum coverage scenarios:

- `CL/TE`
- `TE/CL`
- malformed chunked
- duplicate header
- oversized headers
- `Host`/`X-Forwarded-*` spoofing

Execution recommendations:

- If the Kind environment is not yet ready, the script will first perform necessary refresh via `./tests/e2e/run-kind.sh`.
- After sending each type of anomalous request, the script immediately sends a clean request and checks the inspect backend counters to confirm that anomalous traffic did not hit the backend and subsequent requests were not contaminated.
- If the current change involves finer-grained header parser, obs-fold, CRLF injection, or cache hit paths, it is recommended to supplement custom raw requests beyond this script.

Clean request baseline:

```bash
curl -fsS -H 'Host: security.example.com' http://127.0.0.1:18080/inspect/baseline
```

Pass criteria:

- Anomalous requests are rejected, truncated, or safely handled
- inspect backend `requests_total` does not increase due to anomalous requests
- subsequent clean requests succeed stably, without stream contamination such as “the next normal request hits the anomalous backend”
- access log, metrics, and admin API show no unexplainable contamination

### 4.6 Slowloris / Connection Flood

The repository script `./tests/e2e/validate-http-security.sh` already covers “lightweight slow-header probing”.
It is suitable for daily regression but cannot replace resource exhaustion testing in a staging environment.

The staging environment should additionally cover at minimum:

- slow header
- slow body
- long idle connections
- short-term high-concurrency connection establishment

Pass criteria:

- normal traffic remains serviceable
- FD, memory, active connections do not spiral out of control
- timeout and cleanup behavior can be explained from metrics or logs

## 5. Supply Chain and Image Scanning

Recommended minimum execution:

```bash
./scripts/run-security-scans.sh
```

This script will uniformly execute:

- `cargo-audit --file dataplane/Cargo.lock`
- `scripts/check-dependabot-alert-triage.sh`, checks whether open GitHub Dependabot alerts have been locally fixed and are awaiting platform refresh, or are bound to repository risk acceptance items
- `osv-scanner scan source -r .`
- `grype dir:.`
- `kubescape scan`, targeting the rendered `deploy/kubernetes/overlays/production` overlay, not the local debug-oriented `deploy/kubernetes/base/`

Output defaults to:

```bash
tmp/security-scans/latest/
```

CI and release workflows now also reuse this entry point, so local and automated runs will no longer use different commands.

If the release window permits, additionally supplement:

- image scanning
- running cluster scanning

Pass criteria:

- no high-severity blockers left unaddressed
- risk acceptance items must have clear records

The current repository has clearly documented and consolidated a set of Kubescape exceptions:

- [tests/security/kubescape-exceptions.json](../../tests/security/kubescape-exceptions.json)

Currently only precise `alertOnly` exceptions are allowed for the `aether-gateway-controlplane` ServiceAccount, covering:

- `C-0015`: The control plane needs cross-namespace `list/watch` Secret to handle Gateway certificate references, `BackendTLSPolicy`, and `ReferenceGrant`
- `C-0007`: The control plane needs to delete its own managed derived resources such as Service, EndpointSlice, NetworkPolicy, ServiceImport

Do not expand this exception into a wildcard scope; if RBAC is later broadened, you must first re-audit before modifying the exception file.

## 6. Required Evidence

At minimum, retain the following for each security regression:

- execution command list
- exit codes
- `admin-before` and `admin-after` snapshot directories
- relevant script output
- if anomalies exist, retain control plane logs, data plane logs, and post-mortem conclusions

## 7. Current Boundaries

This document has mapped existing repository entry points into executable templates, but the following items are not yet fully scripted:

- header injection / CRLF / more systematic parser fuzzing
- slow body, long idle, and large-scale flood automation
- unified scanning entry point for images and manifests

These items still need to be supplemented as standard scripts during the maintenance period.
