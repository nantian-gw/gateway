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
// Sub-packages:
//   - shared: indexes, helpers, limits, metrics, status summaries
//   - backends: backend cluster, TLS, LB policy, session translation
//   - routes: route and filter translation, timeout parsing
//   - listeners: frontend validation, mesh service translation
//   - policies: route attachment, gateway queries, Wasm/Token translation
//   - routepolicy: RoutePolicy translation
//   - aiservice: AIService translation
//
// Root package retains the Translator struct, full Build() orchestration,
// partial rebuild methods, support object loaders, and workload translation.
//
// When changing this package, keep Gateway API status behavior, ReferenceGrant
// scoping, backend policy precedence, partial rebuild behavior, and IR shape
// covered by focused tests. Prefer the shared indexes and support-object
// loaders over ad hoc object scans so full and partial rebuild paths stay
// consistent.
package translator
