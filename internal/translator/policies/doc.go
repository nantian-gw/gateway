// Package policies provides translation helpers for Gateway API policy
// resources and Nantian extension policy resources.
//
// Responsibilities:
//   - Route attachment: resolves which routes attach to which listeners
//     based on Gateway, ListenerSet, and Service mesh frontend semantics.
//   - Gateway-scoped queries: lists GatewayClasses and Gateways using
//     controller-runtime field indexes.
//   - Policy target indexing: sets up field indexes for BackendTLSPolicy,
//     BackendLBPolicy, and RoutePolicy target references.
//   - Token policy translation: converts TokenPolicy resources into IR
//     token policy configs mapped to backend keys.
//   - Wasm plugin translation: converts WasmPlugin resources into IR
//     Wasm plugin configs, resolving inline, URL, and ConfigMap sources.
//
// The package is consumed by the parent translator package and by
// cmd/manager for field index registration.
package policies
