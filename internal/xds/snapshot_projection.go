package xds

import (
	"context"
	"log/slog"
	"strconv"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/constants"
)

const backendRefValidityMetadataKey = "nantian.dev/backend-ref-valid"

func buildProjectedProtoSnapshot(ctx context.Context, source *ir.Snapshot, profile projectionProfile, logger *slog.Logger) *controlv1.ConfigSnapshot {
	if source == nil {
		return &controlv1.ConfigSnapshot{
			RequiredFeatures:     []string{featureCoreV1},
			CompatibilityProfile: profile.compatibilityProfile,
		}
	}

	projectedIR := buildProjectedIRSnapshot(source, profile)
	projected := toProtoSnapshotWithLogger(projectedIR, logger)
	projected.RequiredFeatures = projectedRequiredFeatures(projectedIR)
	projected.CompatibilityProfile = profile.compatibilityProfile
	injectTraceparent(ctx, projected)
	return projected
}

func buildProjectedIRSnapshot(source *ir.Snapshot, profile projectionProfile) *ir.Snapshot {
	projected := source.Clone()
	if projected == nil {
		return nil
	}

	supported := make(map[string]struct{}, len(profile.effective))
	for _, feature := range profile.effective {
		supported[feature] = struct{}{}
	}

	survivingBackends := make(map[string]struct{}, len(projected.Backends))
	backends := make([]ir.BackendCluster, 0, len(projected.Backends))
	for _, backend := range projected.Backends {
		if backendRequiresUnsupportedHardFeature(backend, supported) {
			continue
		}
		backends = append(backends, backend)
		survivingBackends[backendProjectionKey(backend.Namespace, backend.Name)] = struct{}{}
	}
	projected.Backends = backends

	projected.HTTPRoutes = projectHTTPRoutes(projected.HTTPRoutes, supported, survivingBackends)
	projected.GRPCRoutes = projectGRPCRoutes(projected.GRPCRoutes, supported, survivingBackends)
	projected.StreamRoutes = projectStreamRoutes(projected.StreamRoutes, supported, survivingBackends)
	projected.Listeners = projectListeners(projected.Listeners, projected)

	return projected
}

func projectHTTPRoutes(
	routes []ir.HTTPRoute,
	supported map[string]struct{},
	survivingBackends map[string]struct{},
) []ir.HTTPRoute {
	out := make([]ir.HTTPRoute, 0, len(routes))
	for _, route := range routes {
		rules := make([]ir.HTTPRule, 0, len(route.Rules))
		for _, rule := range route.Rules {
			rule.BackendRefs = filterBackendRefs(route.Namespace, rule.BackendRefs, survivingBackends)
			if len(rule.BackendRefs) == 0 && !hasTerminalBackendlessHTTPBehavior(rule.Filters) {
				continue
			}
			rules = append(rules, rule)
		}
		if len(rules) == 0 {
			continue
		}
		route.Rules = rules
		if !supportsFeature(supported, featureRouteLabelsV1) {
			route.Labels = nil
		}
		out = append(out, route)
	}
	return out
}

func projectGRPCRoutes(
	routes []ir.GRPCRoute,
	supported map[string]struct{},
	survivingBackends map[string]struct{},
) []ir.GRPCRoute {
	out := make([]ir.GRPCRoute, 0, len(routes))
	for _, route := range routes {
		rules := make([]ir.GRPCRule, 0, len(route.Rules))
		for _, rule := range route.Rules {
			rule.BackendRefs = filterBackendRefs(route.Namespace, rule.BackendRefs, survivingBackends)
			if len(rule.BackendRefs) == 0 {
				continue
			}
			rules = append(rules, rule)
		}
		if len(rules) == 0 {
			continue
		}
		route.Rules = rules
		if !supportsFeature(supported, featureRouteLabelsV1) {
			route.Labels = nil
		}
		out = append(out, route)
	}
	return out
}

func projectStreamRoutes(
	routes []ir.StreamRoute,
	supported map[string]struct{},
	survivingBackends map[string]struct{},
) []ir.StreamRoute {
	out := make([]ir.StreamRoute, 0, len(routes))
	for _, route := range routes {
		rules := make([]ir.StreamRule, 0, len(route.Rules))
		for _, rule := range route.Rules {
			rule.BackendRefs = filterBackendRefs(route.Namespace, rule.BackendRefs, survivingBackends)
			if len(rule.BackendRefs) == 0 {
				continue
			}
			rules = append(rules, rule)
		}
		if len(rules) == 0 {
			continue
		}
		route.Rules = rules
		if !supportsFeature(supported, featureRouteLabelsV1) {
			route.Labels = nil
		}
		out = append(out, route)
	}
	return out
}

func projectListeners(listeners []ir.Listener, snapshot *ir.Snapshot) []ir.Listener {
	// Translator-produced listener attachments are namespace/name route keys.
	// The IR does not retain route kind in AttachedRoutes, so projection can
	// only preserve or drop those exact keys, not disambiguate same-key
	// collisions across HTTP/GRPC/stream route inventories.
	survivingRoutes := make(map[string]struct{}, len(snapshot.HTTPRoutes)+len(snapshot.GRPCRoutes)+len(snapshot.StreamRoutes))
	for _, route := range snapshot.HTTPRoutes {
		survivingRoutes[backendProjectionKey(route.Namespace, route.Name)] = struct{}{}
	}
	for _, route := range snapshot.GRPCRoutes {
		survivingRoutes[backendProjectionKey(route.Namespace, route.Name)] = struct{}{}
	}
	for _, route := range snapshot.StreamRoutes {
		survivingRoutes[backendProjectionKey(route.Namespace, route.Name)] = struct{}{}
	}

	out := make([]ir.Listener, 0, len(listeners))
	for _, listener := range listeners {
		attachedRoutes := make([]string, 0, len(listener.AttachedRoutes))
		for _, routeKey := range listener.AttachedRoutes {
			if _, ok := survivingRoutes[routeKey]; ok {
				attachedRoutes = append(attachedRoutes, routeKey)
			}
		}
		if len(attachedRoutes) == 0 && !isProjectedFrontendListener(listener) {
			continue
		}
		listener.AttachedRoutes = attachedRoutes
		out = append(out, listener)
	}
	return out
}

func isProjectedFrontendListener(listener ir.Listener) bool {
	return isGatewayProjectedListener(listener) || isMeshProjectedListener(listener)
}

func isGatewayProjectedListener(listener ir.Listener) bool {
	if listener.Metadata == nil {
		return false
	}
	return listener.Metadata["gateway"] != "" && listener.Metadata["namespace"] != ""
}

func isMeshProjectedListener(listener ir.Listener) bool {
	if listener.Metadata == nil {
		return false
	}
	return listener.Metadata[mesh.FrontendKindMetadataKey] != ""
}

func filterBackendRefs(routeNamespace string, refs []ir.BackendRef, survivingBackends map[string]struct{}) []ir.BackendRef {
	out := make([]ir.BackendRef, 0, len(refs))
	for _, ref := range refs {
		if backendRefMarkedInvalid(ref) {
			out = append(out, ref)
			continue
		}

		namespace := ref.Namespace
		if namespace == "" {
			namespace = routeNamespace
		}
		if _, ok := survivingBackends[backendProjectionKey(namespace, ref.Name)]; ok {
			out = append(out, ref)
			continue
		}
		if ref.Port != 0 {
			if _, ok := survivingBackends[backendProjectionKey(namespace, portQualifiedBackendName(ref.Name, ref.Port))]; ok {
				out = append(out, ref)
			}
		}
	}
	return out
}

func backendRefMarkedInvalid(ref ir.BackendRef) bool {
	return ref.Metadata[backendRefValidityMetadataKey] == constants.StrFalse
}

func backendRequiresUnsupportedHardFeature(backend ir.BackendCluster, supported map[string]struct{}) bool {
	if backend.AIService != nil && !supportsFeature(supported, featureBackendAIServiceV1) {
		return true
	}
	if backend.TokenPolicy != nil && !supportsFeature(supported, featureBackendTokenPolicyV1) {
		return true
	}
	if backend.WasmPlugin != nil && !supportsFeature(supported, featureBackendWasmPluginV1) {
		return true
	}
	return false
}

func hasTerminalBackendlessHTTPBehavior(filters []ir.Filter) bool {
	for _, filter := range filters {
		if filter.Type == "RequestRedirect" {
			return true
		}
		if filter.Type == "ExtensionRef" {
			if extensionType, ok := filter.Config["extensionType"].(string); ok && extensionType == "DirectResponse" {
				return true
			}
		}
	}
	return false
}

func projectedRequiredFeatures(snapshot *ir.Snapshot) []string {
	required := make([]string, 0, len(orderedProjectionFeatures))
	required = append(required, featureCoreV1)
	if snapshotRequiresRouteLabels(snapshot) {
		required = append(required, featureRouteLabelsV1)
	}
	if snapshotRequiresAIService(snapshot) {
		required = append(required, featureBackendAIServiceV1)
	}
	if snapshotRequiresTokenPolicy(snapshot) {
		required = append(required, featureBackendTokenPolicyV1)
	}
	if snapshotRequiresWasmPlugin(snapshot) {
		required = append(required, featureBackendWasmPluginV1)
	}
	return required
}

func snapshotRequiresRouteLabels(snapshot *ir.Snapshot) bool {
	for _, route := range snapshot.HTTPRoutes {
		if len(route.Labels) > 0 {
			return true
		}
	}
	for _, route := range snapshot.GRPCRoutes {
		if len(route.Labels) > 0 {
			return true
		}
	}
	for _, route := range snapshot.StreamRoutes {
		if len(route.Labels) > 0 {
			return true
		}
	}
	return false
}

func snapshotRequiresAIService(snapshot *ir.Snapshot) bool {
	for _, backend := range snapshot.Backends {
		if backend.AIService != nil {
			return true
		}
	}
	return false
}

func snapshotRequiresTokenPolicy(snapshot *ir.Snapshot) bool {
	for _, backend := range snapshot.Backends {
		if backend.TokenPolicy != nil {
			return true
		}
	}
	return false
}

func snapshotRequiresWasmPlugin(snapshot *ir.Snapshot) bool {
	for _, backend := range snapshot.Backends {
		if backend.WasmPlugin != nil {
			return true
		}
	}
	return false
}

func backendProjectionKey(namespace, name string) string {
	return namespace + "/" + name
}

func portQualifiedBackendName(name string, port uint32) string {
	return name + ":" + strconv.Itoa(int(port))
}

func supportsFeature(supported map[string]struct{}, feature string) bool {
	_, ok := supported[feature]
	return ok
}
