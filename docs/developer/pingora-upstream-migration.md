# Rust Proxy Upstream Migration

This document records the final state after the dataplane exited the local vendored Rust proxy fork, rather than continuing to describe a "future plan."

## Migration Results

The migration has completed the following actions:

- Removed the `[patch.crates-io]` entries for `aether-core` / `aether-proxy` from `dataplane/Cargo.toml`
- Switched the dataplane to the `openssl` runtime of upstream `aether 0.8.0`
- Deleted `dataplane/third_party/`
- Changed configurations that depended on vendored patches to explicit rejections or documented convergence

## Current Implementation

### Backend TLS

- Custom CA bundle: retained, mapped to the per-peer CA configuration in upstream Rust proxy/OpenSSL.
- `validation.hostname`: retained, always used as the upstream SNI; when `subjectAltNames` is not explicitly configured, it also continues to participate in hostname verification.
- `Hostname` / `URI` `subjectAltNames`: retained, with explicit SAN verification performed by the first-party dataplane after the TLS handshake.
- Multiple `subjectAltNames` combinations: retained, with the current semantics being "any configured SAN match grants passage."
- Backend TLS minimum/maximum version option: no longer supported.

### Frontend TLS

- HTTPS listener continues to use upstream Rust proxy/OpenSSL `TlsSettings`.
- `frontendValidation` continues to enable frontend mTLS via CA bundle + verify mode.
- Listener certificate and frontend mTLS CA rotation logic continues to be handled by the first-party runtime.

### HTTP/1.1 Protocol Boundaries

- Chunked request body is still supported.
- `Expect: 100-continue` is still supported.
- HTTP/1.1 chunked request trailers are no longer proxied to the upstream backend.

This last point is not the first-party actively removing functionality, but rather that upstream `aether-core/aether-proxy 0.8.0` currently does not expose HTTP/1.1 request trailers to the upper layer, and there is no upstream-only `finish_body_with_trailers` path to continue using.

## Compatibility Conclusion

| Capability | Post-Migration Status | Notes |
| --- | --- | --- |
| Backend custom CA | Retained | Uses upstream OpenSSL per-peer CA |
| Backend `validation.hostname` | Retained | Continues as upstream SNI; also serves as hostname verify when SAN is not explicitly configured |
| Backend `Hostname` / `URI` SAN | Retained | SAN verification by first-party dataplane post-handshake |
| Multiple backend SAN combinations | Retained | Any configured SAN match grants passage |
| Backend TLS min/max option | Removed | Explicitly rejected by the control plane |
| Backend mTLS client cert | Retained | Uses upstream OpenSSL `CertKey` |
| Frontend mTLS | Retained | Uses upstream `TlsSettings` |
| HTTP/1.1 request trailers passthrough | Removed | Compatibility documented, no longer retained via patches |

## Repository Constraints

After the migration is complete, the repository will adhere to the following rules by default:

1. Do not restore the long-term vendored Rust proxy directory.
2. Do not add vendored-only API assumptions for first-party functionality.
3. If upstream has gaps, prefer to fail closed or document convergence.
4. Only when first-party cannot handle the requirement and the business must retain it, discuss a temporary patch — and it must be independently audited and independently tracked.

## Minimum Verification

After changes involving this migration, at minimum execute:

```bash
cd controlplane && go test ./internal/backendtls ./internal/translator ./internal/status
cargo test --manifest-path dataplane/Cargo.toml -p aeg-http
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

If you need to confirm that the repository has not reintroduced a vendored Rust proxy, you may also run:

```bash
./scripts/audit-vendored-upstream.sh
```

## Conclusion

The current mainline has returned to an upstream-only Rust proxy dependency model. All retained capabilities are built on top of upstream's currently public capabilities; capabilities that cannot work without patches have been clearly converged and are no longer hidden in a local fork.
