# Gateway API Version and Status Plane Audit

This document addresses the requirement in the release backlog that "upgrading to a higher version of Gateway API requires a re-run of the status plane audit."
The purpose is not to re-run conformance, but to systematically verify when the Gateway API version changes:

- `GatewayClass.status.conditions` still conform to the current CRD semantics
- `GatewayClass.status.supportedFeatures` have no omissions or stale entries
- `docs/gateway-api-support.md`, `docs/status-matrix.md` are consistent with the actual code declarations
- Release baseline does not carry forward the default stance of the old version

## 1. When Must It Be Executed

A full audit must be performed when any of the following occurs:

- Upgrading the `gateway-api` Go module
- Updating Gateway API CRDs installed by Kind / release scripts
- Changing the conformance harness version
- Adjusting `GatewayClass.status.supportedFeatures`
- Changing the feature list generation logic managed by `scripts/update-gateway-api-support.sh`
- Changing status output logic for `Accepted`, `SupportedVersion`, `ResolvedRefs`, `PartiallyInvalid`, etc.

Seeing that conformance still passes is not sufficient to skip this step.

## 2. Audit Entry Point

First check the CRD bundle version actually installed in the cluster:

```bash
./scripts/audit-gateway-api-bundle.sh
./scripts/update-gateway-api-support.sh --check
```

If the current version is not the default `v1.5.1`, you can explicitly pass the expected version:

```bash
EXPECTED_BUNDLE_VERSION=v1.5.1 ./scripts/audit-gateway-api-bundle.sh
```

If you need to verify the status output of a specific `GatewayClass`:

```bash
GATEWAY_CLASS_NAME=aether ./scripts/audit-gateway-api-bundle.sh
```

This script performs three tasks:

- Checks the core CRD’s `gateway.networking.k8s.io/bundle-version`
- Fails directly on version mismatch, preventing use of the old stance
- Prints the target `GatewayClass`’s `conditions` and `supportedFeatures`

## 3. Mandatory Checks

### 3.1 Status Fields

At minimum, verify that the following outputs still conform to the current CRD semantics:

- `GatewayClass.status.conditions`
  - `Accepted`
  - `SupportedVersion`
- `GatewayClass.status.supportedFeatures`
- `Gateway.status.conditions`
  - `Accepted`
  - `Programmed`
- `Gateway.status.listeners[*].conditions`
  - `Accepted`
  - `Programmed`
  - `ResolvedRefs`
- Route `status.parents[*].conditions`
  - `Accepted`
  - `ResolvedRefs`
  - `PartiallyInvalid`

Reference sources:

- [status-matrix.md](./status-matrix.md)
- `controlplane/internal/status/`

### 3.2 Supported Features

Confirm that the following three are not out of sync:

- `controlplane/internal/gatewayapi/supported_features.go`
- [gateway-api-support.md](./gateway-api-support.md)
- Feature/profile scope used by conformance

If any one changes the support scope, the other two must be updated in sync.

It is recommended to use:

```bash
./scripts/update-gateway-api-support.sh
```

### 3.3 Automation Entry Points

Confirm that release and validation entry points still use the correct version scope:

- `tests/conformance/run.sh`
- `scripts/run-release-validation.sh`
- `docs/test/latest-baseline.md`
- `reports/conformance/latest/metadata.yaml`

## 4. Minimum Validation

After completing the static audit, at minimum re-run the following checks:

```bash
cd controlplane && go test ./...
./scripts/run-release-validation.sh
```

If the upgrade affects the support matrix or status plane semantics, also update in sync:

- [gateway-api-support.md](./gateway-api-support.md)
- [status-matrix.md](./status-matrix.md)
- [Release backlog](backlog/release.md)

## 5. Conclusion Record Format

After each audit, at minimum record:

- Audit date
- Target Gateway API version
- Related commit
- Whether `SupportedVersion` / `supportedFeatures` were adjusted
- Documents and scripts requiring synchronized updates
- Whether automated validation passed

It is recommended to record the results in:

- [docs/test/latest-baseline.md](./test/latest-baseline.md)
- Or new release / conformance archive notes
