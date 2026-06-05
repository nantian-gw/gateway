# 0005: Make the Production Overlay Secure by Default

Status: Accepted  
Date: 2026-04-30

## Context

The base Kubernetes manifests are useful for local development and lightweight
smoke testing. Production-like environments require stricter defaults:

- controlplane admin APIs should not be anonymously reachable
- dataplane admin APIs and metrics should not be openly exposed
- controlplane-to-dataplane configuration streams should support TLS/mTLS
- dataplane session persistence needs a stable secret

Relying only on documentation warnings creates drift between recommended
operations and the manifests users actually apply.

## Decision

Keep a production overlay that is secure by default.

The production overlay requires:

- controlplane admin Bearer Token
- dataplane admin Bearer Token
- controlplane gRPC TLS/mTLS
- dataplane xDS TLS/mTLS
- stable dataplane session persistence secret
- Secret mounts that fail closed when required production Secrets are absent

The base overlay can remain development-friendly, but production install paths
must make unsafe defaults explicit and visible.

## Alternatives Considered

- Put strict production settings directly in base: safer by default, but makes
  local development, kind smoke and conformance loops unnecessarily heavy.
- Keep only documentation and no production overlay: lower maintenance cost, but
  too easy for users to deploy insecure defaults.
- Provide only Helm values later: useful in the future, but does not solve the
  current Kustomize release path.

## Consequences

- Production installation requires users to create real Secrets before applying
  the overlay.
- Release validation and security scans should include the production overlay.
- Docs must explain which values are safe defaults and which are placeholders.
- NetworkPolicy, Service exposure and admin auth boundaries are part of the
  release contract, not optional afterthoughts.

## Revisit / Rollback Conditions

Revisit this decision if the project adds a formal installer, Helm chart or
Operator that can generate profiles with equivalent or stronger security
defaults. Do not weaken production defaults without replacing them with an
auditable installation profile.
