# External Review Evidence Ledger

This ledger defines what evidence is required before Aether Gateway can claim
that its governance, maintainership, roadmap, adopter, and case-study material
is strong enough for a more formal external review.

It does not create that evidence by itself. Entries only count when they point
to public, reviewable records or repository artifacts.

## Current Status

Status as of 2026-05-13: not satisfied.

The repository has baseline governance documents and a public review packet, but
it does not yet have enough external participation or adopter evidence to close
the community-readiness gate.

| Evidence area | Current status | Why it is not closed |
| --- | --- | --- |
| Governance documents | present | `GOVERNANCE.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `SUPPORT.md`, and `VERSIONING.md` exist. |
| Maintainer list | present but single-maintainer | `MAINTAINERS.md` does not yet show multiple long-term active human maintainers. |
| Reviewer pool | not satisfied | No stable public reviewer group separate from the current maintainer role. |
| External contributor record | not satisfied | Git history shows more than one human author, but this is not enough to prove sustained external contribution and review. |
| Adopter evidence | not satisfied | `docs/adopters-and-compatibility.md` has the public entrypoint, but no named adopter. |
| Case studies | not satisfied | No public case study or deployment retrospective is recorded. |
| Public design review | partial | `docs/proposals/` and ADRs exist, but there is not yet a sustained external review record. |
| Release history | partial | Release and evidence gates exist, but a stable public release cadence still needs time and tags. |

## Minimum Gate To Close The External Evidence TODO

The external evidence TODO item can only be checked off after all required
evidence below is present and linked from this document,
`docs/community-readiness.md`, and
`docs/implementation-review-packet.md`.

| Requirement | Minimum acceptable evidence |
| --- | --- |
| Multiple maintainers | At least two long-term active human maintainers listed in `MAINTAINERS.md`, each with visible review, triage, or release activity in the last 90 days. |
| Reviewer trail | Public PR reviews or issue triage showing that maintainers are not self-reviewing all meaningful changes. |
| External contributor activity | At least two non-maintainer human contributions with public review records, or one sustained external contributor with multiple accepted PRs. |
| Adopter evidence | At least one named adopter with permission to publish organization/project name, use case, version, and validated scope. |
| Case study or deployment note | At least one public case study, deployment retrospective, blog, talk, or equivalent externally reviewable record. |
| Roadmap and proposal review | Public roadmap plus at least one proposal or ADR that includes reviewer feedback or maintainer decision notes. |
| Release history | At least one public release tag with release notes, compatibility notes, and linked conformance/security/performance evidence. |

## Evidence Entry Format

Use this format when adding a new evidence item:

```md
### <YYYY-MM-DD> <short title>

- Evidence type: maintainer / reviewer / contributor / adopter / case-study / roadmap / release
- Public link: <URL or repository path>
- Related version or commit: `<tag-or-sha>`
- Summary: <one or two sentences>
- Permission status: public / anonymous / internal-only
- Counts toward gate: yes / no
```

Only `public` evidence can close the external-review gate. Anonymous or
internal-only evidence may be useful context, but it cannot satisfy named
adopter or public case-study requirements.

## Current Evidence Items

### 2026-05-12 Review Packet Created

- Evidence type: roadmap
- Public link: `docs/implementation-review-packet.md`
- Related version or commit: repository state as of 2026-05-12
- Summary: The repository has a central external review packet linking support,
  conformance, release, security, governance, and adopter boundaries.
- Permission status: public
- Counts toward gate: partial

### 2026-05-12 Adopter Entry Point Created

- Evidence type: adopter
- Public link: `docs/adopters-and-compatibility.md`
- Related version or commit: repository state as of 2026-05-12
- Summary: The repository has a page where adopters and case studies can be
  added, but it does not list a named adopter yet.
- Permission status: public
- Counts toward gate: no

## Maintenance Rules

- Do not convert planned outreach into evidence.
- Do not count bot commits as maintainer or contributor diversity.
- Do not count an adopter unless the name and use case are approved for public
  publication.
- Keep `docs/community-readiness.md`, `docs/adopters-and-compatibility.md`, and
  `docs/implementation-review-packet.md` aligned whenever this ledger changes.
- Keep the external evidence TODO item open until the minimum gate is
  satisfied.
