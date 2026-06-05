# Controlplane RBAC Audit

This document records the intended least-privilege model for the controlplane
`ClusterRole` in `deploy/kubernetes/base/rbac.yaml`.

The machine-readable source of truth is
`docs/security/controlplane-rbac-required.json`. The audit command compares that
baseline with the deployed RBAC YAML and with controller watch declarations:

```bash
scripts/audit-controlplane-rbac.sh --check
```

If a future change adds a new resource or verb, it must update both the RBAC
manifest and `controlplane-rbac-required.json` with a concrete purpose and source
file. Undocumented extra permissions fail the audit.

## Permission Purposes

Core Kubernetes resources:

- `services`: watched as route backends, read for status/snapshot construction, listed by the admin service catalog, and created/updated/deleted only for managed dataplane frontend Services.
- `secrets`: read and watched for Gateway TLS certificates and BackendTLSPolicy client certificate material. The controlplane does not write user Secrets.
- `endpoints`: read/watched through the controller-runtime cache and deleted only to clean up legacy core Endpoints for managed frontend Services while EndpointSlice is the active model.
- `pods`: listed and watched to discover dataplane Pods and mesh workload Pods for managed frontend EndpointSlice generation.
- `namespaces`: watched for `allowedRoutes` selector changes and read during route attachment/status evaluation.
- `configmaps`: read and watched for Gateway frontend validation CA refs, BackendTLSPolicy CA refs and operator-supplied config.
- `events`: created/patched/updated through the Kubernetes event recorder for operator-visible status changes.

Coordination and discovery:

- `leases`: used by controller-runtime leader election and shared dataplane node status storage.
- `endpointslices`: watched/read for backend endpoint discovery and created/updated/deleted for managed dataplane frontend EndpointSlices.

Gateway API and MCS resources:

- Gateway API main resources are watched/read for snapshot translation and status evaluation. `create/update/delete` is present because the admin resource API can mutate the supported Gateway API resource set when resource mutations are enabled.
- Gateway API status subresources only require `get/update`; the code uses `Status().Update` and does not use status patch.
- `serviceimports` are watched/read as MCS backends and can be mutated through the admin resource API when resource mutations are enabled.

Infrastructure:

- `customresourcedefinitions` are read to detect optional Gateway API and MCS resources through the REST mapper and support-matrix checks.
- `networkpolicies` are created/updated/deleted by the infrastructure reconciler for managed dataplane listener/admin ingress policy.

## Audit Scope

The audit currently enforces three rules:

- Every RBAC resource and verb in `deploy/kubernetes/base/rbac.yaml` must be documented in `controlplane-rbac-required.json`.
- Every documented permission must be present in the RBAC YAML.
- Every resource discovered from controller `For(...)` or `Watches(...)` declarations must have `get/list/watch` documented and granted.

This is intentionally conservative. It prevents silent permission growth and
keeps the permission rationale close to the code path that requires it.
