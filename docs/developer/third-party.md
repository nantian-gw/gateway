# Third-Party Dependencies and Rust Proxy Status

This document describes the current repository's dependency on the Rust proxy and the boundaries that must be observed when making future modifications.

## Current Status

- The dataplane no longer vendors `aether-core` / `aether-proxy` source code.
- `dataplane/Cargo.toml` currently depends directly on upstream `aether 0.8.0` with the `openssl` feature enabled.
- `dataplane/third_party/` should now remain absent; if this directory reappears, it should be treated as a repository constraint violation.

In other words, the repository has exited the "locally maintained Rust proxy fork" model and returned to the normal upstream crate dependency management path.

## Historical Background

The repository previously vendored two Rust proxy crates:

- `dataplane/third_party/aether-core`
- `dataplane/third_party/aether-proxy`

These copies were used to carry the following local patch surfaces:

- Backend TLS custom CA bundle
- Backend TLS `subjectAltNames` extension validation
- Backend TLS minimum / maximum version options
- HTTP/1.1 request trailers and other protocol compatibility patches

These patches have been removed from the current mainline, replaced by upstream-only implementations and explicit support boundaries.

## Currently Retained Capabilities

Without continuing to patch the Rust proxy, the repository still retains the following capabilities:

- Backend mTLS client certificates
- `BackendTLSPolicy.validation.hostname`
- `BackendTLSPolicy` system CA validation
- `BackendTLSPolicy` same-namespace `ConfigMap/ca.crt` custom CA bundle
- `BackendTLSPolicy` `Hostname` / `URI` `subjectAltNames`
- Multiple `subjectAltNames` combinations with post-handshake validation matching any one
- HTTPS listener frontend mTLS validation

## Currently Explicitly Removed Behaviors

The following capabilities or behaviors are no longer retained in upstream-only mode:

- `gateway.nantian.dev/backend-tls-min-version`
- `gateway.nantian.dev/backend-tls-max-version`
- HTTP/1.1 chunked request trailers forwarding to upstream backend

The handling principle is:

- Configurations that can fail closed are explicitly rejected by the control plane.
- Runtime behaviors that cannot be restored on the first-party side and for which upstream currently has no public interface must be documented in compatibility documentation, and no longer rely on hidden patches.

## Audit Entry Points

The following script can be used to confirm the repository remains in upstream-only state:

```bash
./scripts/audit-vendored-upstream.sh
```

This script currently checks:

- `dataplane/Cargo.toml` does not patch `aether-core` / `aether-proxy` to local paths
- `dataplane/third_party/` does not exist
- Rust proxy versions in workspace and lockfile are consistent

## Modification Constraints

If future changes to Rust proxy-related behavior are needed, at minimum follow these guidelines:

1. First determine whether the change can be made in a first-party crate, rather than restoring a vendored patch.
2. If upstream currently does not support a capability, prefer to explicitly reject the configuration or reduce the support surface.
3. Only discuss temporary patches when the change cannot be made in first-party code and the capability is business-critical; such patches must be separately tracked, separately documented, and carry a clear owner and removal target.
4. After any changes involving TLS, protocol boundaries, connection reuse, or request forwarding, at minimum run:

```bash
cargo test --manifest-path dataplane/Cargo.toml --workspace
```

## One-Sentence Principle

The current repository depends on the upstream Rust proxy, rather than maintaining a local Rust proxy fork.
