# Versioning And Release Policy

This document describes the current repository's version naming, release thresholds, compatibility expectations, and support scope.

## 1. Current Stage

The project is still in a rapid convergence phase.
Before a stable user baseline forms, the versioning strategy prioritizes "clear, verifiable, rollback-capable" over complex long-term support commitments.

## 2. Version Format

Semantic versioning tags are recommended:

- `vMAJOR.MINOR.PATCH`
- Example: `v0.3.1`

Current suggested meanings:

- `MAJOR`
  For releases with explicit breaking changes, significantly increased migration cost, or runtime contract resets.
- `MINOR`
  For new capabilities, expanded Gateway API support scope, or new operational/governance capabilities.
- `PATCH`
  For defect fixes, stability convergence, documentation corrections, and small changes that do not alter external contracts.

Before `v1.0.0`, a degree of compatibility adjustment may still occur, so strict long-term API stability should not be promised.

## 3. Release Triggers

The current repository's release workflow supports two trigger modes:

- Pushing a `v*` tag
- Manual trigger with `release_tag` provided

The release process runs:

- controlplane Go unit tests
- dataplane Rust workspace tests
- Kind smoke
- `ALL_FEATURES=true` full-suite Gateway API conformance

Only after these baselines pass is a release suitable for external reference as a formal release.

## 4. Release Artifacts

The current release process generates or publishes:

- GitHub Release
- GHCR images
  - `nantian-controlplane`
  - `nantian-dataplane`
- conformance artifact
- Reports archived to `reports/conformance/releases/<tag>/`
- Latest reports published to the `conformance-reports` branch

The in-repo dashboard currently has source code and Kubernetes manifests, but the dashboard image is not in the set of images published by the current core release workflow. Enabling dashboard production installation requires separately pinning the dashboard image in an environment overlay or external release pipeline; the core release gate still uses controlplane / dataplane / conformance / install manifest as the standard.

Each formal release should also include:

- A corresponding version change summary in [`CHANGELOG.md`](CHANGELOG.md)
- Corresponding version compatibility notes in [`docs/user/compatibility-notes.md`](docs/user/compatibility-notes.md)

## 5. Support Scope

Currently, only the following are committed:

- `main` is the continuously evolving branch
- The latest release tag is the recommended usage baseline

The following are not yet established:

- Parallel maintenance of multiple stable branches
- Long-term support version windows
- Formal backport strategy

If stable branches are established in the future, at minimum the following are recommended:

- List of supported releases
- End-of-support dates for each release
- Which fixes are eligible for backport
- Which features only go into the next minor version

## 6. Compatibility Expectations

Current compatibility commitments are based on the principle of "avoid undocumented breaking changes wherever possible":

- `proto/` changes must clearly describe the coupling impact on control plane and data plane.
- controlplane and dataplane are by default officially supported as the combination from the same release tag; only the short-duration rolling upgrade / rollback window of adjacent releases is within the declared skew support scope.
- Admin API changes must describe compatibility impact and migration approach.
- Deployment manifest changes must describe whether rolling upgrade or additional manual steps are required.
- Documentation, support matrices, and known limitations must stay consistent with release behavior.

The following should be treated as compatibility changes requiring more careful handling:

- Gateway API behavior semantic changes
- `gateway.control.v1` wire protocol changes
- Default listener port or Service exposure method changes
- Admin API field deletion or renaming
- Config file field renaming or meaning changes
- Security boundary, authentication method, or certificate handling logic changes

For current skew conventions and automation entry points, see [docs/skew-compatibility.md](docs/skew-compatibility.md).

## 7. Conformance Expectations

If a release is to serve as a "more formally referenceable Gateway API implementation version," it is recommended to at least satisfy:

- Full-suite conformance report archived
- Report corresponds one-to-one with the release tag
- Documentation clearly states the Gateway API version the report is based on
- Support matrix describes the boundaries of declared and undeclared support

## 8. Upgrade Guidance

Before upgrading, it is recommended to at least confirm:

- The target version's [`CHANGELOG.md`](CHANGELOG.md)
- The target version's [`docs/user/compatibility-notes.md`](docs/user/compatibility-notes.md)
- The target version's release notes
- [controlplane / dataplane / proto version skew contract](docs/skew-compatibility.md)
- Support boundary changes in `docs/gateway-api-support.md`
- Verification results for the corresponding version in `reports/conformance/`
- Whether admin API, deployment manifests, or config examples have contract changes

If changes involve `proto/gateway/control/v1`, the control plane / data plane rolling upgrade path, or continued skew support declarations for adjacent releases, at minimum run before release:

```bash
./scripts/run-skew-validation.sh
```

The current repository's stable release documentation entry points are [`CHANGELOG.md`](CHANGELOG.md) and [`docs/user/compatibility-notes.md`](docs/user/compatibility-notes.md).
Before a more formal release cadence is established, at minimum ensure both records are complete for every formal release.