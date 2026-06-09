// Package translator converts Kubernetes Gateway API resources and Nantian
// extension resources into the internal routing model used by the control
// plane.
//
// The controller layer calls this package after Kubernetes object changes.
// A full rebuild loads Gateways, routes, Services, ServiceImports,
// EndpointSlices, Secrets, ConfigMaps, ReferenceGrants, policies, workloads,
// and extension resources. The translator then builds shared lookup indexes,
// resolves listener attachment, route rules, backend references, policy
// precedence, filters, workload metadata, and extension behavior, and emits an
// IR snapshot plus status summaries for the controller and gRPC publication
// layers.
//
// The main file groups are:
//   - translator*.go: full snapshot translation and shared route/listener
//     helpers.
//   - partial*.go: incremental rebuild helpers used when only part of the
//     snapshot must be refreshed.
//   - backend_*.go, backend_tls*.go, and backend_lb_policy.go: backend,
//     BackendTLSPolicy, BackendLBPolicy, and session-persistence translation.
//   - attachments*.go, gateway_scoped_queries.go, and
//     policy_target_ref_indexes.go: route attachment, parent scoping, and
//     policy target indexing.
//   - ai_service.go, token_policy.go, and wasm_plugin.go: Nantian extension
//     resources.
//   - indexes.go, limits.go, and status_summary.go: support utilities used by
//     the translation paths.
//
// When changing this package, keep Gateway API status behavior, ReferenceGrant
// scoping, backend policy precedence, partial rebuild behavior, and IR shape
// covered by focused tests. Prefer the shared indexes and support-object
// loaders over ad hoc object scans so full and partial rebuild paths stay
// consistent.
package translator
