# Proposals

This directory is used to archive design proposals, decision records, and review evidence that need long-term preservation.

Content suitable for this directory includes:

- Changes that affect external contracts, compatibility, or upgrade paths
- Changes involving cross-module coordination across `controlplane`, `dataplane`, and `proto`
- Design decisions that require explicit trade-off documentation from reviewers or maintainers
- Roadmap items that need a public explanation of "why this approach was chosen"

Content not suitable for this directory includes:

- Pure copy/text corrections
- Local, low-risk, small fixes that don't require additional design trade-offs
- Drafts produced only during temporary troubleshooting with no long-term reuse value

Each proposal should cover at minimum:

1. Background and problem
2. Approach and boundaries
3. Compatibility and operational impact
4. Validation plan
5. Rollback plan or alternatives

See [template.md](template.md) for the template.

## Current Proposals

| Proposal | Status | Purpose |
| --- | --- | --- |
| [gateway-api-next-version-upgrade.md](gateway-api-next-version-upgrade.md) | proposed | Defines version alignment, support boundaries, conformance, status audit, and rollback requirements when upgrading from Gateway API `v1.5.1` to the next stable version. |
