# Developer Documentation

This documentation entry is intended for repository developers, contributors, and maintainers who need to modify runtime behavior.
If your goal is deployment, evaluation, or troubleshooting, please read the [User Guide](../user/README.md) first.

## Suggested Reading Order

1. [Architecture Document](../architecture.md): Understand the boundaries between the control plane, data plane, and IR.
2. [Design Document](../design.md): Understand component decomposition, admin interfaces, and protocol design.
3. [Gateway API Support Matrix](../gateway-api-support.md): Confirm the current level of support and which capabilities remain gaps.
4. [Gateway API Status Matrix](../status-matrix.md): Confirm the current semantics, boundaries, and test anchors for `Accepted / Programmed / ResolvedRefs / PartiallyInvalid / Conflicted`.
5. [Roadmap](../roadmap.md): Confirm the current phase goals, exit criteria, and subsequent extension boundaries.
6. [Backlog Navigation](../backlog/README.md): Select the next work item by control plane, data plane, release, and security.
7. [Test Plan](../test/plan.md): Select the minimal yet sufficient test path based on the repository's verification ladder.
8. [Third-Party Dependencies](third-party.md): Understand the current upstream-only dependency status, historical vendored Rust proxy background, and repository guardrails.
9. [Rust Proxy Upstream Migration](pingora-upstream-migration.md): Review which capabilities were retained and which behaviors were converged after exiting the local Rust proxy fork.
10. [Script Contract](scripts.md): Confirm the arguments, artifacts, and exit code semantics of `make` and `scripts/*`.
11. [Development Workflow](../development.md): Follow the recommended process for making changes, verification, and integration testing.
12. [Source Layout and Large File Splitting](source-file-layout.md): Review the splitting results, responsibility boundaries, and remaining exceptions for source files exceeding 800 lines.

## Shortest Path for Development Work

- When modifying `proto/` or structures shared between the control plane and data plane, first update the protocol and documentation, then run `make proto`.
- When modifying control plane logic, first run `cd controlplane && go test ./...`, then decide whether Kind is needed.
- When modifying data plane logic, first run `cargo test --manifest-path dataplane/Cargo.toml --workspace`, then decide whether Kind is needed.
- When modifying the dashboard, first run `cd dashboard && npm run check`; the dashboard is not implicitly covered by core release validation.
- When release gating is involved, explicitly run `ALL_FEATURES=true ./tests/conformance/run.sh` — do not treat the quick profile as the final conclusion.
- When only modifying deployment scripts or manifests, prefer reusing existing Kind clusters and images; do not rebuild by default.

## Development Principles

- Validate using the cheapest verification method first, then move to more expensive verification levels.
- The control plane and data plane establish contracts through `proto/` and the IR — do not modify only one side.
- Keep each independent functional unit as a separate commit; avoid mixing documentation, code, tests, and refactoring together.
- Maintain a multi-crate organization for the data plane; do not collapse HTTP, stream, xDS, and logging logic back into a single file.
- The data plane has switched back to the upstream Rust proxy; prioritize implementing new capabilities within first-party code or upstream capabilities, and do not reintroduce a long-term vendored patch surface.
