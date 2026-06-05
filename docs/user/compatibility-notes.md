# Compatibility Notes

This document serves as the stable compatibility notes entry point for Aether Gateway.
It has a different responsibility from `CHANGELOG.md`:

- `CHANGELOG.md` explains "what changed in this release"
- This document explains "how these changes affect upgrades, rollbacks, skew, admin APIs, or deployment defaults"

## How To Use This File

If a release involves any of the following, a corresponding version section should be added:

- Expected skew changes for `controlplane` / `dataplane` / `proto`
- Admin API field, query semantics, or authentication changes
- Deployment manifest, default port, default security boundary, or default image behavior changes
- Upgrade, rollback, or canary workflows that require additional manual steps

## Current Baseline

- The default officially supported baseline remains the controlplane and dataplane combination from the same release tag.
- Adjacent release skew support scope is defined in [`docs/skew-compatibility.md`](../skew-compatibility.md).
- If a release changes behavior noted in `CHANGELOG.md` but does not affect upgrade or rollback contracts, additional notes in this document are not required.

## Unreleased / Current Mainline

### API / Config / Runtime Impact

- dataplane has switched back to the upstream `pingora 0.8.0` OpenSSL runtime, no longer depending on the local Rust proxy fork under `dataplane/third_party/`.
- `BackendTLSPolicy` continues to support system CA, custom `ConfigMap/ca.crt` CA bundle, `validation.hostname`, and one or more `Hostname` / `URI` `subjectAltNames`; when `subjectAltNames` is explicitly configured, `validation.hostname` continues to be used only as upstream SNI, and certificate identity is validated post-handshake against the configured SAN set. The two repo-specific options `gateway.nantian.dev/backend-tls-min-version` / `gateway.nantian.dev/backend-tls-max-version` remain unsupported, and the control plane will explicitly reject them.
- HTTP/1.1 chunked request trailers on the upstream-only Rust proxy path are no longer forwarded to the upstream backend. The relevant `TE: trailers` / `Trailer` headers are still forwarded with the request headers, but the trailer fields themselves are discarded after downstream parsing. This is a known compatibility convergence after removing the vendored Rust proxy patch.

## Future Release Entries

Suggested per-version format:

```markdown
## vX.Y.Z

### Upgrade Notes

- ...

### Rollback Notes

- ...

### API / Config / Manifest Impact

- ...
```