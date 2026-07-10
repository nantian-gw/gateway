# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### ⚠️ Breaking changes

- **Admin API `authMode: kubernetes` now fails closed for writes.** Previously any
  identity that passed TokenReview authentication was allowed to perform writes.
  Write requests (any method other than `GET`/`HEAD`) now additionally require the
  authenticated user or one of its groups to appear in the new
  `adminAuth.allowedUsers` / `adminAuth.allowedGroups` allowlists. **With no
  allowlist configured, all writes are denied (403).** Reads still require only
  successful authentication. Static (`bearerToken`) auth mode is unaffected.

  **Upgrade action** (only if you run `adminAuth.authMode: kubernetes`): add an
  allowlist so authorized identities retain write access:

  ```yaml
  adminAuth:
    authMode: kubernetes
    allowedGroups:
      - "platform:admins"      # any group returned by TokenReview
    # allowedUsers:
    #   - "system:serviceaccount:nantian-gw:admin"
  ```

### Security

- Admin `authMode: kubernetes` TokenReview now restricts audiences via
  `adminAuth.tokenReviewAudiences`. When set, only tokens actually issued for one
  of the configured audiences are accepted, closing a token-confusion gap where
  any valid ServiceAccount token in the cluster was accepted.
- Admin rate limiter now honors `X-Forwarded-For` only when the direct peer is a
  configured trusted proxy (`adminAuth.trustedProxies`, list of CIDRs/IPs).
  Otherwise the direct `RemoteAddr` is used, preventing limit-bypass and unbounded
  per-key map growth via spoofed headers.
- TokenReview verification now reuses a single Kubernetes clientset, honors the
  request context for cancellation/deadline, and caches results for 30s (keyed by a
  SHA-256 of the token) to reduce load on the API server.

### Added

- `adminAuth` config keys: `tokenReviewAudiences`, `allowedUsers`, `allowedGroups`,
  `trustedProxies`.
- `grpcRuntime` config keys: `gracefulStopTimeout` (default `3s`), and opt-in
  flow-control/message-size knobs `initialWindowSize`, `initialConnWindowSize`,
  `maxConcurrentStreams`, `maxRecvMsgSize`. The window knobs are opt-in: a zero
  value leaves gRPC's automatic BDP window tuning in place — only override after
  measuring, as an explicit window disables autotuning and primarily affects the
  inbound direction.
- Observability: `nantian_gateway_build_info` metric and
  `nantian_gateway_controlplane_xds_active_streams` gauge.

### Changed

- xDS proto snapshot builds are de-duplicated via `singleflight` and served from a
  read-locked cache fast path, reducing redundant marshaling under concurrent
  stream fan-out.
