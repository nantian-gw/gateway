# Security Policy

This document describes how to report security issues, the current scope of concern, and response expectations.

## Reporting

Please do not disclose details of unpatched vulnerabilities in public issues.

Recommended reporting order:

1. Prefer GitHub's private security reporting feature (if enabled for the repository).
2. If private reporting is unavailable, contact the current repository maintainers before deciding to disclose publicly.
3. Only use a regular issue when the matter does not involve sensitive exploitation details.

When reporting, please provide as much of the following as possible:

- Affected version or commit
- Reproduction steps
- Scope of impact
- Logs, error messages, or a minimal PoC
- Whether a temporary mitigation already exists

Do not include:

- Private keys, tokens, or complete certificate material
- Sensitive production configuration
- Unsanitized user data

## Scope

The following types of issues are currently prioritized:

- Authentication bypass in the control plane or data plane
- Admin interface exposure or permission boundary errors
- gRPC control channel TLS / mTLS configuration defects
- Backend TLS verification and certificate validation errors
- Request misrouting to wrong backends, cross-namespace reference bypasses, or policy failures
- High-risk default configurations in deployment manifests

## Response Expectations

This project is still in an early phase, so enterprise-grade SLAs are not guaranteed. Maintainers will aim to respond in the following order:

- First, confirm whether the issue is reproducible
- Then, assess the impact level and temporary mitigation options
- Finally, schedule a fix, regression verification, and public disclosure

If residual risks are accepted in the future, the risk register must be updated accordingly rather than noted only in issue threads or chat records.

## Supported Branches

By default, only the `main` branch is guaranteed to be under continuous maintenance. If stable release branches are established in the future, support windows and backport policies should be documented here.

---

Summary: Please do not disclose details of unpatched security vulnerabilities in public issues; use private reporting or contact maintainers first. The project prioritizes authentication bypass, admin interface exposure, and TLS/mTLS configuration defects. Currently, only the `main` branch is guaranteed to be under continuous maintenance.
