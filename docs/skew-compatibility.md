# Controlplane / Dataplane / Proto Version Skew Contract

This document defines the current version skew boundaries for Nantian Gateway across `controlplane`, `dataplane`, and `proto/gateway/control/v1`.
The goal is not to encourage long-term mixed-version operation, but to clearly state "which combinations are acceptable during rolling upgrades / rollbacks, and which combinations should not be declared as supported," while providing automation entry points within the repository.

## 1. Current Support Boundaries

Currently, only one type of combination is recommended and officially supported:

- Control plane and data plane built from artifacts of the same release tag.

To support rolling upgrades and fast rollbacks, one additional type of short-duration skew is currently permitted:

- Temporary skew between adjacent releases, i.e., a `N` and `N-1` mixed operation window.

This "permitted" status has three prerequisites:

- Only for upgrade or rollback windows; should not be maintained long-term.
- `proto/gateway/control/v1` remains backward-compatible with the previous release.
- Both old and new control plane / data plane continue to follow the current ACK/NACK and last-good fallback semantics.

The following combinations are currently **not** declared as supported:

- Cross-version mixed operation spanning two or more releases.
- Long-term mixed operation of untagged development snapshots with historical releases.
- Any change requiring breaking `gateway.control.v1` wire protocol compatibility while still attempting to reuse the same proto package.

## 2. Matrix Convention

| Controlplane | Dataplane | Proto contract | Current Convention |
| --- | --- | --- | --- |
| `N` | `N` | Binding and runtime code generated from the same tag | `Supported` |
| `N` | `N-1` | `gateway.control.v1` remains backward-compatible with `N-1` | `Supported, but only within the rolling upgrade window` |
| `N-1` | `N` | `gateway.control.v1` remains backward-compatible with `N-1`, and `N` dataplane tolerates missing old fields | `Supported, but only within the rollback window` |
| `N` | `N-2` or earlier | Even if protobuf wire protocol can still decode, runtime semantics are no longer guaranteed | `Not supported` |
| Any | Any | Requires breaking the existing `gateway.control.v1` wire protocol | `Must upgrade to a new proto version; cannot continue to declare skew support` |

## 3. `gateway.control.v1` Compatibility Rules

`gateway.control.v1` is currently treated as the foundational contract for rolling upgrades between releases.
As long as it remains on this proto line, the following rules are assumed:

- Published fields are only appended, never deleted, never reuse field numbers, and never change wire types.
- Published messages, enums, services, and methods are never deleted.
- New data planes must tolerate new fields that the old control plane did not send, using safe defaults for missing fields.
- Old data planes must be able to ignore unknown fields added by the new control plane, rather than exiting due to decode failures.
- If a change cannot satisfy the above conditions, it should not continue to reuse `gateway.control.v1`; instead, explicitly introduce a new proto version and migration notes.

The in-repo automated checks currently primarily cover "previous release to current version" proto backward-compatibility constraints.
They do not automatically prove that all runtime semantics are fully compatible, so unit tests, Kind validation, and release canary tests are still needed.

## 4. ACK / NACK / Rollback Constraints

During rolling upgrades, the current repository requires both old and new versions to continue meeting the following behaviors:

- When dataplane encounters a snapshot it cannot apply, it should explicitly `NACK` rather than silently stall.
- When controlplane receives a `NACK`, it should retain the current node's error information for `/v1/nodes` and summary observation.
- When dataplane fails to apply the current snapshot, it should continue carrying last-good rather than dropping all listeners to empty.
- After rolling back control plane or data plane versions, historical last-good state should not be corrupted or rendered completely unrecoverable due to protocol compatibility issues.

This is also why the current convention only provides support for short-duration skew of adjacent releases and does not commit to larger-span mixed operation.

## 5. Automation Entry Point

The repository provides a unified entry point:

```bash
./scripts/run-skew-validation.sh
```

It performs four things by default:

1. `make proto`
2. Previous-baseline compatibility check against `proto/gateway/control/v1/control.proto`
3. `cd controlplane && go test ./...`
4. `cargo test --manifest-path dataplane/Cargo.toml --workspace`

A compatibility baseline can also be explicitly specified:

```bash
COMPAT_BASE_REF=v0.0.1 ./scripts/run-skew-validation.sh
```

If the repository already has release tags, the compatibility check defaults to comparing against the latest reachable `v*` tag from the current branch.
If the repository does not yet have usable tags, the tool falls back to `HEAD^`, at least blocking the most obvious breaking changes like "just deleted a field, changed wire type, or removed an RPC entry."

There is also a Make wrapper:

```bash
make test-skew
```

The release baseline now also invokes this check, so manual memory of "whether proto compatibility was reviewed" is no longer required before a formal release.

## 6. When Proto Version Must Be Upgraded

When any of the following occurs, `gateway.control.v1` should no longer be used:

- Need to delete a published field or message.
- Need to modify an existing field number, wire type, streaming direction, or RPC input/output types.
- Old and new versions cannot achieve smooth coexistence through defaults and unknown field ignoring mechanisms.
- Need old data planes to explicitly reject, rather than ignore, certain new configuration semantics.

When this happens, the following should be completed simultaneously:

- New proto package / version design
- controlplane / dataplane dual-side migration logic
- Skew policy update
- Release notes and upgrade documentation update

## 7. What Is Not Yet Covered

This automation is still not a complete mixed-version proof. The following are not yet fully covered:

- Real mixed-operation verification of "new controlplane + old dataplane" in Kind
- Real rollback verification of "old controlplane + new dataplane" in Kind
- Long-duration `24h/72h` soak mixed-version runtime evidence
- Dedicated drills combining node drain, apiserver/watch turbulence, and mixed-version simultaneously

Therefore, this script should currently be treated as the `P0` minimum threshold automation, not the final form.

## 8. Adjacent Version Dual-End Build Verification

`run-skew-validation.sh` now supports `MIXED_VERSION_VALIDATE=true` mode.
This mode checks out the adjacent baseline ref in a git worktree and verifies:

- Bidirectional proto compatibility (current→base and base→current)
- Control plane can successfully build at the baseline ref
- Data plane can successfully build at the baseline ref

```bash
MIXED_VERSION_VALIDATE=true ./scripts/run-skew-validation.sh
MIXED_VERSION_VALIDATE=true MIXED_VERSION_BASE_REF=v0.1.0 ./scripts/run-skew-validation.sh
```

`run-release-validation.sh` enables this mode by default during the skew verification phase.