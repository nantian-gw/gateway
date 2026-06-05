## Summary

Briefly explain what this change does and why.

## Scope

- [ ] controlplane
- [ ] dataplane
- [ ] proto
- [ ] release / CI
- [ ] deploy / kind
- [ ] tests
- [ ] docs

## Ownership

- [ ] I have confirmed the default reviewer path according to `CODEOWNERS`
- [ ] I have read the relevant ownership notes in `MAINTAINERS.md`

## Validation

Check only the validation you actually ran.
Only check items you actually executed; do not check items you did not run.

- [ ] `make proto`
- [ ] `cd controlplane && go test ./...`
- [ ] `cargo test --manifest-path dataplane/Cargo.toml --workspace`
- [ ] `bash -n scripts/render-release-manifest.sh`
- [ ] `bash -n scripts/prepare-release-assets.sh`
- [ ] `bash -n tests/e2e/run-kind.sh`
- [ ] `./tests/e2e/run-kind.sh`
- [ ] `./tests/conformance/run.sh`

## Why This Validation Level

Explain why this validation level is enough.
Default to the cheapest validation path first, then move to more expensive layers only when needed. Do not default to running Kind.

## Contract / API Changes

- [ ] No external behavior changes
- [ ] Modified `proto/`
- [ ] Modified configuration examples
- [ ] Modified admin interface
- [ ] Modified deployment manifests

If any of the items above are checked, explain compatibility impact and migration steps.

## Release Communication

- [ ] No release note or compatibility note needed
- [ ] Updated `CHANGELOG.md`
- [ ] Updated `docs/user/compatibility-notes.md`

If you updated release communication, explain what upgrade or compatibility impact it covers.

## Docs

- [ ] No documentation update needed
- [ ] Updated relevant documentation or comments

## Risks

Describe risks, rollback approach, and what reviewers should focus on.
