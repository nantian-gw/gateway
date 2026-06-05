# Aether Gateway Structured Security Audit (2026-03)

This document records a round of "quasi-external" structured security audit results.
The goal is not to substitute for a full third-party penetration test, but to formalize the most critical security boundaries of the current repository into traceable conclusions, evidence, and risk acceptance items.

> Historical snapshot: This audit record preserves the security baseline as of `2026-03-31` and does not represent the current default Gateway API version or latest release evidence. For current public support scope, refer to [Gateway API Support Matrix](../gateway-api-support.md), [Risk Register](risk-register.md), and the latest release / conformance evidence.

Audit date: `2026-03-31`

Audit scope:

- controlplane / dataplane admin authentication boundaries
- controlplane gRPC TLS / mTLS and dataplane xDS TLS / mTLS
- `BackendTLSPolicy` and frontend/backend certificate validation
- `ReferenceGrant` cross-namespace authorization boundaries
- HTTP request parsing boundaries and basic anomalous message regression

Baseline notes:

- Code baseline: `main` branch as of `2026-03-31`
- Release baseline (historical): Gateway API `v1.4.1`, the then-current release workflow, the then-current production overlay
- Evidence sources: repository unit tests, dedicated E2E, release-validation, security scan thresholds, and operations documentation

## 1. Conclusion Summary

- Within this audit scope, no `critical` blocker was found that would prevent the current repository from continuing production baseline convergence.
- Admin authentication, cross-namespace authorization, backend TLS validation, and basic request parsing boundaries all have clear code and regression entry points — no longer in a "documentation claims but no evidence" state.
- There are still several residual risks that need to continue being accepted or strengthened, primarily:
  - controlplane metrics still rely on network boundaries rather than Bearer Token
  - Long-term xDS TLS / mTLS rotation, expiry, and fault injection have not yet formed stronger cluster-level automation
  - Request parsing boundaries already have dedicated regression tests, but systematic fuzzing, slow body / idle / flood automation is still not closed-loop
  - upstream `aether-core 0.8.0 -> prometheus 0.13.x -> protobuf 2.x` still brings a `protobuf < 3.7.2` transitive dependency alert, but the current repository only exports Prometheus text format and does not parse external protobuf payloads

Current risk acceptance items:

- [risk-register.md](./risk-register.md)

## 2. Audit Scope and Conclusions

### 2.1 Admin Authentication Boundaries

Conclusion:

- Both `controlplane` and `dataplane` `/v1/*` admin endpoints support Bearer Token protection.
- `/livez` and `/readyz` explicitly bypass authentication, consistent with probe endpoint expectations.
- Bearer Token supports file reading, and the current implementation re-reads the file on each request, enabling restart-free rotation based on Secret volumes.
- `dataplane /metrics` shares the same auth model with admin endpoints; `controlplane metrics` still goes through a separate port and network boundary control.

Primary evidence:

- [server_test.go](../../controlplane/internal/admin/server_test.go)
- [auth.go](../../controlplane/internal/admin/auth.go)
- [tests.rs](../../dataplane/crates/aeg-app/src/admin/tests.rs)
- [operations.md](../user/operations.md)
- [admin-api.md](../user/admin-api.md)

Assessment:

- The current implementation meets the minimum security boundary of "probes are anonymous, management surface is controlled."
- controlplane metrics does not use Bearer Token, instead relying on `ClusterIP + NetworkPolicy`; this is listed as risk acceptance item `SEC-RA-001`.

### 2.2 controlplane gRPC TLS / mTLS and dataplane xDS TLS / mTLS

Conclusion:

- controlplane gRPC service supports TLS; configuring `clientCAPath` enables optional client certificate verification or enforced mTLS.
- dataplane xDS client supports custom CA, client certificate / private key, and `domainName`, and normalizes plaintext addresses to `https://...` when TLS is enabled.
- Server-side certificate materials support hot reload on new handshakes, avoiding the situation where "certificate files changed but the listener only takes effect after process restart."

Primary evidence:

- [tls.go](../../controlplane/internal/grpcserver/tls.go)
- [tls_test.go](../../controlplane/internal/grpcserver/tls_test.go)
- [tls.rs](../../dataplane/crates/aeg-xds/src/tls.rs)
- [lib.rs](../../dataplane/crates/aeg-xds/src/lib.rs)
- [production/README.md](../../deploy/kubernetes/overlays/production/README.md)

Assessment:

- Current TLS / mTLS capabilities already cover basic confidentiality and mutual authentication configuration surfaces.
- The residual gap is not "whether TLS exists" but rather "whether long-term rotation, expiry, network turbulence, and fault injection have formed strong automation evidence" — this is listed as risk acceptance item `SEC-RA-002`.

### 2.3 `BackendTLSPolicy` and Certificate Validation Boundaries

Conclusion:

- Both control plane and data plane already cover the core security semantics of `BackendTLSPolicy`:
  - Custom CA bundle
  - `validation.hostname`
  - `Hostname` / `URI` SAN validation
  - Client certificate references
  - Invalid CA, invalid SAN, conflicting policies, and partially valid reference retention
- Frontend `frontendValidation` CA `ConfigMap` and cross-namespace references are also included in the reference validation scope.

Primary evidence:

- [reconciler_backend_tls_validation_test.go](../../controlplane/internal/status/reconciler_backend_tls_validation_test.go)
- [reconciler_backend_tls_precedence_test.go](../../controlplane/internal/status/reconciler_backend_tls_precedence_test.go)
- [backend_tls_test.go](../../controlplane/internal/translator/backend_tls_test.go)
- [validation_test.go](../../controlplane/internal/backendtls/validation_test.go)
- [tests.rs](../../dataplane/crates/aeg-http/src/runtime/tests.rs)
- [security-regression.md](../test/security-regression.md)

Assessment:

- Current backend TLS and frontend mTLS are not "flag-level capabilities" but implementations with clear input validation and error state output.
- No obvious bypass paths were found within this audit scope; subsequent focus should be on longer time-scale certificate rotation and combined scenario automation.

### 2.4 `ReferenceGrant` Cross-Namespace Authorization Boundaries

Conclusion:

- Route-to-backend Service, Gateway-to-cross-namespace Secret, `frontendValidation` CA, `clientCertificateRef`, and other cross-namespace references all require `ReferenceGrant`.
- The current repository has both control plane status unit tests and three dedicated E2E tests (HTTP / gRPC / certificates), covering "failure before authorization, success after authorization, invalidation after revocation."

Primary evidence:

- [reconciler_acceptance_cross_namespace_test.go](../../controlplane/internal/status/reconciler_acceptance_cross_namespace_test.go)
- [reconciler_route_misc_test.go](../../controlplane/internal/status/reconciler_route_misc_test.go)
- [validate-reference-grants.sh](../../tests/e2e/validate-reference-grants.sh)
- [validate-grpc-reference-grants.sh](../../tests/e2e/validate-grpc-reference-grants.sh)
- [validate-gateway-cross-namespace-certs.sh](../../tests/e2e/validate-gateway-cross-namespace-certs.sh)
- [gateway-api-support.md](../gateway-api-support.md)

Assessment:

- This boundary now has relatively complete code and E2E evidence, no longer relying on manual spot-checks.
- This audit did not discover known defects such as "authorization scope overflow" or "continued effectiveness after grant deletion."

### 2.5 Request Parsing Boundaries and Anomalous Message Handling

Conclusion:

- The current repository has already captured a set of high-risk anomalous messages into standard scripts:
  - `CL/TE`
  - `TE/CL`
  - malformed chunked
  - duplicate `Host`
  - conflicting `Content-Length`
  - oversized headers
  - `Host` / `X-Forwarded-*` forgery
  - Lightweight slow-header detection
- The script not only checks "whether the request was rejected" but also verifies inspect backend counts and subsequent clean requests, avoiding verification based solely on surface-level return codes.

Primary evidence:

- [validate-http-security.sh](../../tests/e2e/validate-http-security.sh)
- [security-regression.md](../test/security-regression.md)
- [plan.md](../test/plan.md)

Assessment:

- Current basic request parsing boundaries already have dedicated regression entry points and can effectively block a batch of common smuggling / boundary pollution regressions.
- More systematic parser fuzzing, CRLF injection variants, slow body / idle / flood automation is still not closed-loop; this is listed as risk acceptance item `SEC-RA-003`.

## 3. Audit Result Triage

This round did not introduce any `critical` / `high`-level blockers.

The current conclusion is closer to the following state:

- Security boundaries with clear implementation and test evidence:
  - admin authentication
  - gRPC / xDS TLS basic configuration surface
  - `BackendTLSPolicy` key validation semantics
  - `ReferenceGrant` authorization boundaries
  - Basic anomalous HTTP message regression
- Known but temporarily accepted residual risks:
  - `SEC-RA-001`
  - `SEC-RA-002`
  - `SEC-RA-003`
  - `SEC-RA-004`

## 4. Recommended Follow-up Actions

In priority order:

1. Converge the accepted items in `risk-register.md` item by item rather than leaving them hanging long-term.
2. Add more formal certificate rotation, expiry, and handshake failure drills for xDS TLS / mTLS.
3. Continue expanding request parsing boundaries to fuzzing, slow body, idle connections, and larger-scale flooding.
4. Before the next release candidate, re-execute:
   - `./scripts/run-security-scans.sh`
   - `./tests/e2e/validate-http-security.sh`
   - `./tests/e2e/validate-reference-grants.sh`
   - `./tests/e2e/validate-grpc-reference-grants.sh`
   - `./tests/e2e/validate-gateway-cross-namespace-certs.sh`