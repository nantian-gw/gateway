# Maintainers

This document records the current named maintenance roles, ownership boundaries, and permission scope for this repository.

The project is still under lightweight governance with a small set of roles.
This document describes the current actual maintenance state of the repository, not a mature multi-organization community governance model.

## Current Maintainers

| Role | Name | GitHub | Ownership | Timezone | Merge | Release |
| --- | --- | --- | --- | --- | --- | --- |
| Maintainer | Mahmut Abi | [@mahmut-Abi](https://github.com/mahmut-Abi) | `controlplane/`, `dataplane/`, `proto/`, release, docs, GitHub workflows | `Asia/Shanghai` | yes | yes |

Maintainer definition:

- Has sustained write access to the main repository.
- Actually bears responsibility for review, merge, release, issue triage, and stability convergence.
- Responsible for consistency across `controlplane` / `dataplane` / `proto` / release contracts.

## Current Reviewers

There is currently no separately named reviewer list distinct from the maintainer role.
Before the reviewer stage is made public, the current maintainer is responsible for reviewer routing by default, with preferred review ownership indicated via [`CODEOWNERS`](CODEOWNERS).

If reviewers are added in the future, this document should at minimum include:

- Public name
- GitHub handle
- Ownership scope
- Timezone
- Whether merge permission is granted
- Whether release permission is granted

## Responsibilities

Maintainer is responsible for:

- Reviewing and merging changes.
- Maintaining release quality, test baselines, and rollback capability.
- Maintaining support matrices, known limitations, and risk notes.
- Responding to defects, security issues, and compatibility regressions.
- Maintaining consistency across `controlplane/`, `dataplane/`, `proto/`, deployment, and documentation.

Reviewers, once separately named in the future, are at minimum responsible for:

- First-round technical review within their ownership scope.
- Providing explicit feedback on compatibility, validation tiers, and documentation synchronization.
- Escalating changes requiring final maintainer judgment to the maintainer.

## Decision Areas

The maintainer makes final judgments on:

- Whether to accept breaking changes.
- Supported Gateway API versions and feature boundaries.
- Release cadence, rollback strategy, and support windows.
- Whether to accept new experimental features into the default path.

## Growth Expectations

If more formal external claims or community expansion are pursued in the future, it is recommended to at minimum establish:

- 2 or more long-term active human maintainers.
- A named reviewer list.
- Timezone coverage for review owner and release owner.
- More public reviewer -> maintainer promotion records.