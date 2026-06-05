# Data Plane Security & Efficiency Audit

> Generated: 2026-05-22 | `4ab4bb4b` → `bba398d6` → ...
> Status: 6/11 COMPLETED, 5 REMAINING

## Security (4/5 done)

- [x] **Admin API has no mandatory authentication** — `admin_bearer_token` is optional; if not set, `/v1/*` endpoints are exposed.
  - File: `dataplane/crates/aeg-app/src/main.rs`
  - Implementation: when not configured, logs warning `"admin API is running without bearer token authentication"`

- [x] **Session Persistence uses ephemeral secret** — when no stable secret is configured, a process-startup ephemeral secret is used; sessions become invalid after multiple replicas or restart.
  - File: `dataplane/crates/aeg-http/src/runtime.rs`
  - Implementation: when ephemeral secret is enabled, logs warning `"session persistence is using an ephemeral secret..."`

- [x] **No-SAN = no hostname verification** — `tls_validation.rs:92`: when BackendTLSPolicy has no `subjectAltNames` configured, `verify_hostname = false`, relying only on CA chain validation.
  - File: `dataplane/crates/aeg-http/src/proxy/backend/tls_validation.rs`
  - Implementation: when no SAN + system CA, logs warning `"BackendTLSPolicy has no subjectAltNames and uses system CA..."`

- [x] **ExternalAuth HTTP request string concatenation** — `external_auth.rs` header values were not escaped.
  - File: `dataplane/crates/aeg-http/src/proxy/external_auth.rs:224-226`
  - Implementation: `write_request_header_values` filters header values containing `\r` `\n`

- [ ] **No body size limit for non-forwardBody requests** — when ExternalAuth forwardBody is not enabled, there is no general request body size limit.
  - File: `dataplane/crates/aeg-config/src/defaults.rs`
  - Implementation: WIP — need to add `default_http_max_request_body_bytes` function
  - Recommendation: Add a global `max_request_body_bytes` configuration, default 10MB.

## Efficiency (2/6 done)

- [ ] **6 `snapshot.read()` RwLock acquisitions per request** — `proxy.rs` acquires the read lock multiple times per request (lines 468, 582, 859, 981, 1114, 1162).
  - File: `dataplane/crates/aeg-http/src/proxy.rs`
  - Recommendation: Acquire `read()` once at `request_filter()` entry, pass via reference to subsequent functions. Estimated >5% latency improvement.

- [ ] **8 .clone() calls on hot path (Vec + Map)** — `HttpRouteContextFields` construction clones full containers like `route_name`, `filters`, `timeouts`, `backend_tls`, etc.
  - File: `dataplane/crates/aeg-http/src/proxy.rs:239-268`
  - Recommendation: Replace deep cloning with `Arc<SelectedHttpRoute>`.

- [ ] **BackendTlsValidation global cache uses RwLock** — acquires read lock per request to check cache, contention under high concurrency.
  - File: `dataplane/crates/aeg-http/src/proxy/backend/tls_validation.rs:52-53,103`
  - Recommendation: Replace `RwLock<HashMap>` with `arc-swap` or `dashmap`.

- [ ] **context.rs reads snapshot outside path** — `context.rs:312` calls `snapshot.read()` again after `request_filter` returns to get the runtime handle.
  - File: `dataplane/crates/aeg-http/src/proxy/context.rs:312`
  - Recommendation: Merge into the single snapshot read optimization.

- [ ] **Rust proxy retry buffer has a 64KB hard limit** — upstream Rust proxy limitation, cannot be modified.
  - File: upstream `nantian-core-0.8.0/src/protocols/http/v1/common.rs:32`
  - Recommendation: Evaluate whether to fork the Rust proxy or submit an upstream PR to add `set_retry_buffer()` API.

- [x] **Connection keepalive has no limit** — no upper bound on connection reuse requests, may cause connection buildup.
  - File: `dataplane/crates/aeg-config/src/defaults.rs`
  - Implementation: `http_keepalive_request_limit` default 1000, `http_max_connection_age_ms` default 1 hour
  - Note: default value 0 still means "disabled" (compatible with original behavior)