# Contributing

This repository is split between a Go control plane and a Rust data plane:

- `controlplane/`: Go control plane
- `dataplane/`: Rust data plane workspace
- `proto/`: shared gRPC contract

Before opening a PR, read: [README.md](README.md), [docs/development.md](docs/development.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), [CODEOWNERS](CODEOWNERS), [MAINTAINERS.md](MAINTAINERS.md), [GOVERNANCE.md](GOVERNANCE.md), [SECURITY.md](SECURITY.md), [VERSIONING.md](VERSIONING.md).

## Development Flow

Prefer the cheapest validation first. Don't default to Kind clusters or heavy e2e flows for every change. Recommended order: unit tests → local process integration → admin API checks → Kind smoke → conformance.

## Local Commands

```bash
make proto
make build
make test-unit
```

Targeted:

```bash
cd controlplane && go test ./...
cargo test --manifest-path dataplane/Cargo.toml --workspace
bash -n tests/e2e/run-kind.sh
```

## Pull Request Expectations

- One PR, one logical change. Don't mix docs, runtime, tests, and refactoring.
- Consult [`CODEOWNERS`](CODEOWNERS) and [`MAINTAINERS.md`](MAINTAINERS.md) to find the right review path.
- Changes to `proto/` must be verified against both planes.
- Config, manifest, or admin API changes need a compatibility summary.
- Release or upgrade behavior changes require an updated changelog.
- Docs must reflect behavior, contract, or workflow changes.

## Reviewer and Maintainer Path

The reviewer-to-maintainer path is open under lightweight governance. Reviewers demonstrate consistent quality changes in an ownership area and give concrete, verifiable review feedback. Maintainers add sustained review, triage, and cross-module convergence work. Governance and role expectations live in [`GOVERNANCE.md`](GOVERNANCE.md) and [`MAINTAINERS.md`](MAINTAINERS.md).

## Large Validation

Use Kind only when cluster behavior, deployment manifests, NodePort behavior, or registry wiring are part of the change.

```bash
./tests/e2e/run-kind.sh
SKIP_BUILD=true ./tests/e2e/run-kind.sh
SKIP_SMOKE=true ./tests/e2e/run-kind.sh
RECREATE_CLUSTER=true ./tests/e2e/run-kind.sh
```

## Questions and Support

Use GitHub issue forms for bugs, features, and questions. See [SUPPORT.md](SUPPORT.md) for usage and troubleshooting. See [MAINTAINERS.md](MAINTAINERS.md), [VERSIONING.md](VERSIONING.md), and [ROADMAP.md](ROADMAP.md) for maintainer scope and project growth expectations.

## Chinese

The repository uses a control plane (Go) and data plane (Rust) separated architecture. Read the documents listed above before submitting a PR.

**Development flow:** Use the cheapest validation first. Recommended order: unit tests → local process integration → admin API checks → Kind smoke → conformance. Don't default to rebuilding the Kind cluster for every change.

**Common commands:** See above for `make proto`, `make build`, `make test-unit`. Fine-grained validation: `cd controlplane && go test ./...`, `cargo test --manifest-path dataplane/Cargo.toml --workspace`, `bash -n tests/e2e/run-kind.sh`.

**PR expectations:** One PR should do one kind of thing. Before requesting review, consult `CODEOWNERS` and `MAINTAINERS.md` to confirm the reviewer path. Changes to `proto/` must verify both control plane and data plane. Changes to config, deployment manifests, or admin APIs need a compatibility impact summary. Changes to release or upgrade behavior require a changelog update. Keep docs in sync with behavior, contract, or workflow changes.

**Reviewer and Maintainer path:** Under lightweight governance, the reviewer → maintainer path is public. Becoming a reviewer requires: consistently submitting high-quality changes in an ownership area, providing concrete, verifiable review feedback, and familiarity with validation tiers, compatibility constraints, and rollback requirements. After sustained review, triage, and cross-module convergence work, upgrading to maintainer can be discussed. Refer to `GOVERNANCE.md` and `MAINTAINERS.md`.

**Heavy validation:** Only use Kind when changes involve cluster behavior, deployment manifests, NodePort, or registry wiring.

**Questions and support:** Use GitHub issue forms. See `SUPPORT.md` for usage and troubleshooting. See `MAINTAINERS.md`, `VERSIONING.md`, and `ROADMAP.md` for maintainer responsibilities, versioning policy, and evolution targets.