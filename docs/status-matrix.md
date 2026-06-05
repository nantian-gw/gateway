# Gateway API Status Matrix

This page only describes the Kubernetes Gateway API status semantics that are already implemented in the current repository and have automated test anchors.

Boundary notes:

- Only covers objects and conditions that the current repository declares as supported, without mixing in fields that future Gateway API versions may add into "implemented" declarations.
- `GatewayConditionReady` is still a reserved field in upstream types; the current repository will not fabricate it just to "complete the matrix."
- `Policy` currently only covers `BackendTLSPolicy` and `BackendLBPolicy` implemented in the repository.
- The "test anchors" below all fall within the lightweight `go test ./...` coverage scope and regress together with the control plane CI.

## Matrix

| Resource | Status Scope | Condition | Current Main Reason | Notes | Test Anchors |
| --- | --- | --- | --- | --- | --- |
| `GatewayClass` | `status.conditions` | `Accepted` | `Accepted` | Only takes over `GatewayClass` matching the current controller. | `controlplane/internal/status/reconciler_core_test.go` |
| `GatewayClass` | `status.conditions` | `SupportedVersion` | `SupportedVersion`, `UnsupportedVersion` | Computed from the `gateway.networking.k8s.io/bundle-version` of installed Gateway API CRDs. | `controlplane/internal/status/reconciler_core_test.go` |
| `GatewayClass` | `status.supportedFeatures` | Non-condition field | N/A | Exported from the feature set in code, sorted by name. | `controlplane/internal/gatewayapi/supported_features_test.go` |
| `Gateway` | `status.conditions` | `Accepted` | `Accepted`, `ListenersNotValid`, `InvalidParameters` | Reflects whether the overall listener configuration is valid; also promotes invalid `Gateway.spec.infrastructure.parametersRef` / `GatewayClass.spec.parametersRef` configurations to readable status. | `controlplane/internal/status/reconciler_core_test.go`, `controlplane/internal/status/reconciler_acceptance_cross_namespace_test.go`, `controlplane/internal/status/reconciler_route_misc_test.go`, `controlplane/internal/status/reconciler_infrastructure_parameters_test.go` |
| `Gateway` | `status.conditions` | `Programmed` | `Programmed`, `ListenersNotValid`, `AddressNotAssigned`, `Invalid` | Reflects whether address assignment, derived Service, and listener orchestration are complete; invalid parameter configuration also blocks Programmed. | `controlplane/internal/status/gateway_addresses_test.go`, `controlplane/internal/status/reconciler_route_misc_test.go`, `controlplane/internal/status/reconciler_infrastructure_parameters_test.go` |
| `Gateway` | `status.conditions` | `InsecureFrontendValidationMode` | `ConfigurationChanged` | Written back when any `HTTPS` listener uses `AllowInsecureFallback` via `Gateway.spec.tls.frontend.default/perPort.validation.mode`, explicitly identifying that this is an intentional relaxation of frontend client certificate validation rather than a missing CA. | `controlplane/internal/status/frontend_validation_test.go` |
| `Gateway` | `status.listeners[*].conditions` | `Accepted` | `Accepted`, `Invalid`, `HostnameConflict`, `ProtocolConflict`, `NoValidCACertificate` | Indicates whether individual listener semantics are valid; when all CAs for frontend mTLS are invalid or unauthorized, the listener is also directly rejected with `NoValidCACertificate`. | `controlplane/internal/status/reconciler_acceptance_cross_namespace_test.go`, `controlplane/internal/status/reconciler_route_misc_test.go`, `controlplane/internal/status/frontend_validation_test.go` |
| `Gateway` | `status.listeners[*].conditions` | `ResolvedRefs` | `ResolvedRefs`, `RefNotPermitted`, `InvalidRouteKinds`, `InvalidCertificateRef`, `InvalidCACertificateRef`, `InvalidCACertificateKind` | Indicates whether the listener's referenced certificates, frontend mTLS CAs, and route kinds are resolvable. | `controlplane/internal/status/backend_tls_test.go`, `controlplane/internal/status/frontend_validation_test.go`, `controlplane/internal/status/reconciler_acceptance_cross_namespace_test.go`, `controlplane/internal/status/reconciler_route_misc_test.go` |
| `Gateway` | `status.listeners[*].conditions` | `Programmed` | `Programmed`, `Invalid`, `HostnameConflict`, `ProtocolConflict` | Indicates whether the listener can actually be deployed and running. For scenarios like `certificateRefs` or frontend `caCertificateRefs` where "partially bad references still retain at least one valid reference," the listener now stays `Programmed=True` while exposing the bad references via `ResolvedRefs=False`. | `controlplane/internal/status/reconciler_core_test.go`, `controlplane/internal/status/reconciler_route_misc_test.go`, `controlplane/internal/status/frontend_validation_test.go` |
| `Gateway` | `status.listeners[*].conditions` | `Conflicted` | `HostnameConflict`, `ProtocolConflict` | Only explicitly written back in listener conflict scenarios. | `controlplane/internal/status/reconciler_route_misc_test.go` |
| `Route` | `status.parents[*].conditions` | `Accepted` | `Accepted`, `NotAllowedByListeners` | Indicates whether the Route successfully attaches to the parent object. | `controlplane/internal/status/reconciler_core_test.go`, `controlplane/internal/status/reconciler_acceptance_cross_namespace_test.go`, `controlplane/internal/status/reconciler_service_parent_test.go` |
| `Route` | `status.parents[*].conditions` | `ResolvedRefs` | `ResolvedRefs`, `RefNotPermitted`, `InvalidKind`, `UnsupportedValue`, `BackendNotFound` | Indicates whether backend, extension, ReferenceGrant, and other references are resolvable. | `controlplane/internal/status/reconciler_route_misc_test.go`, `controlplane/internal/status/reconciler_acceptance_cross_namespace_test.go`, `controlplane/internal/status/extension_filters_test.go`, `controlplane/internal/status/native_filters_test.go` |
| `Route` | `status.parents[*].conditions` | `PartiallyInvalid` | `UnsupportedValue` | Only written back when "some rules are discarded but the remaining rules can still take effect." | `controlplane/internal/status/native_filters_test.go`, `controlplane/internal/status/reconciler_route_misc_test.go`, `controlplane/internal/translator/translator_test.go` |
| `BackendLBPolicy` | `status.ancestors[*].conditions` | `Accepted` | `Accepted`, `Conflicted`, `TargetNotFound`, `Invalid` | Reflects whether target selection, conflicts, and semantics are valid. | `controlplane/internal/status/backend_lb_policy_test.go` |
| `BackendLBPolicy` | `status.ancestors[*].conditions` | `ResolvedRefs` | `ResolvedRefs`, `TargetNotFound`, `InvalidKind` | Reflects whether the target resolves to an actual backend. | `controlplane/internal/status/backend_lb_policy_test.go` |
| `BackendTLSPolicy` | `status.ancestors[*].conditions` | `Accepted` | `Accepted`, `Conflicted`, `TargetNotFound`, `Invalid`, `NoValidCACertificate` | Reflects target, validation parameters, and conflict status. | `controlplane/internal/status/reconciler_backend_tls_precedence_test.go`, `controlplane/internal/status/reconciler_backend_tls_validation_test.go` |
| `BackendTLSPolicy` | `status.ancestors[*].conditions` | `ResolvedRefs` | `ResolvedRefs`, `TargetNotFound`, `InvalidCACertificateRef`, `InvalidKind` | Reflects CA, target, and backend resolution results. | `controlplane/internal/status/reconciler_backend_tls_precedence_test.go`, `controlplane/internal/status/reconciler_backend_tls_validation_test.go` |

## Admin-Side Consistency

Status is not only written back to Kubernetes objects — it is also exposed externally via the control plane snapshot and admin API:

- The translator carries `ListenerStatus`, `RouteParentStatus` conditions, `ObservedGeneration`, and summary fields into the IR snapshot.
- `/v1/snapshot` returns these status summaries externally.
- `/v1/summary` continues to provide a global aggregated view and does not carry the raw conditions of each object.

Current test anchors:

- `controlplane/internal/translator/translator_test.go`
- `controlplane/internal/admin/server_test.go`

## What Is Currently Not in the "Completed Matrix" Declaration

- New condition / status fields from Gateway API versions newer than the current default `v1.5.1`, or new fields not yet audited for this repository's status surface.
- Status surface for unimplemented resources.
- Additional Policy types that may be introduced in the future.
- Any internal errors that are only visible in logs but have no Kubernetes status or admin API external semantics; `parametersRef` input errors currently declared as supported are no longer in this category.