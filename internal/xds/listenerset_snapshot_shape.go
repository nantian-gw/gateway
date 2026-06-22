package xds

import (
	"log/slog"
	"strings"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

const listenerSetHTTPRoutingMarker = "listener-set-http-routing"

type listenerSetHTTPRoutingSnapshotShapeSummary struct {
	Listeners  []listenerSetHTTPRoutingListenerShape `json:"listeners"`
	HTTPRoutes []listenerSetHTTPRoutingRouteShape    `json:"http_routes"`
	Backends   []string                              `json:"backends"`
}

type listenerSetHTTPRoutingListenerShape struct {
	Name           string   `json:"name"`
	Hostnames      []string `json:"hostnames"`
	AttachedRoutes []string `json:"attached_routes"`
}

type listenerSetHTTPRoutingRouteShape struct {
	Name        string   `json:"name"`
	RuleCount   int      `json:"rule_count"`
	BackendRefs []string `json:"backend_refs"`
}

func logListenerSetHTTPRoutingSnapshotShape(
	logger *slog.Logger,
	nodeID string,
	version string,
	snapshot *controlv1.ConfigSnapshot,
) {
	if logger == nil {
		return
	}
	shape, ok := listenerSetHTTPRoutingSnapshotShape(snapshot)
	if !ok {
		return
	}
	logger.Info(
		"projected ListenerSetHTTPRouting snapshot shape",
		"node_id",
		nodeID,
		"version",
		version,
		"listener_count",
		len(snapshot.GetListeners()),
		"http_route_count",
		len(snapshot.GetHttpRoutes()),
		"backend_count",
		len(snapshot.GetBackends()),
		"listeners",
		shape.Listeners,
		"http_routes",
		shape.HTTPRoutes,
		"backends",
		shape.Backends,
	)
}

func listenerSetHTTPRoutingSnapshotShape(snapshot *controlv1.ConfigSnapshot) (listenerSetHTTPRoutingSnapshotShapeSummary, bool) {
	var shape listenerSetHTTPRoutingSnapshotShapeSummary
	if snapshot == nil {
		return shape, false
	}

	relevantRoutes := make(map[string]struct{})
	for _, listener := range snapshot.GetListeners() {
		if !listenerSetHTTPRoutingListenerRelevant(listener) {
			continue
		}
		attachedRoutes := append([]string(nil), listener.GetAttachedRoutes()...)
		shape.Listeners = append(shape.Listeners, listenerSetHTTPRoutingListenerShape{
			Name:           listener.GetName(),
			Hostnames:      append([]string(nil), listener.GetHostnames()...),
			AttachedRoutes: attachedRoutes,
		})
		for _, routeKey := range attachedRoutes {
			if routeKey != "" {
				relevantRoutes[routeKey] = struct{}{}
			}
		}
	}

	relevantBackends := make(map[string]struct{})
	for _, route := range snapshot.GetHttpRoutes() {
		key := listenerSetHTTPRoutingRouteKey(route)
		if !listenerSetHTTPRoutingRouteRelevant(key, relevantRoutes) {
			continue
		}
		routeShape := listenerSetHTTPRoutingRouteShape{
			Name:      key,
			RuleCount: len(route.GetRules()),
		}
		for _, rule := range route.GetRules() {
			for _, ref := range rule.GetBackendRefs() {
				refKey := listenerSetHTTPRoutingBackendRefKey(route.GetNamespace(), ref)
				routeShape.BackendRefs = append(routeShape.BackendRefs, refKey)
				relevantBackends[refKey] = struct{}{}
			}
		}
		shape.HTTPRoutes = append(shape.HTTPRoutes, routeShape)
	}

	for _, backend := range snapshot.GetBackends() {
		key := backendProjectionKey(backend.GetNamespace(), backend.GetName())
		if _, ok := relevantBackends[key]; ok {
			shape.Backends = append(shape.Backends, key)
		}
	}

	return shape, len(shape.Listeners) != 0 || len(shape.HTTPRoutes) != 0 || len(shape.Backends) != 0
}

func listenerSetHTTPRoutingListenerRelevant(listener *controlv1.Listener) bool {
	if listener == nil {
		return false
	}
	if strings.Contains(listener.GetName(), listenerSetHTTPRoutingMarker) {
		return true
	}
	for _, hostname := range listener.GetHostnames() {
		if strings.Contains(hostname, listenerSetHTTPRoutingMarker) {
			return true
		}
	}
	for _, route := range listener.GetAttachedRoutes() {
		if strings.Contains(route, listenerSetHTTPRoutingMarker) {
			return true
		}
	}
	return false
}

func listenerSetHTTPRoutingRouteRelevant(routeKey string, relevantRoutes map[string]struct{}) bool {
	if strings.Contains(routeKey, listenerSetHTTPRoutingMarker) {
		return true
	}
	_, ok := relevantRoutes[routeKey]
	return ok
}

func listenerSetHTTPRoutingRouteKey(route *controlv1.HttpRoute) string {
	if route == nil {
		return ""
	}
	return backendProjectionKey(route.GetNamespace(), route.GetName())
}

func listenerSetHTTPRoutingBackendRefKey(routeNamespace string, ref *controlv1.BackendRef) string {
	if ref == nil {
		return ""
	}
	namespace := ref.GetNamespace()
	if namespace == "" {
		namespace = routeNamespace
	}
	name := ref.GetName()
	if ref.GetPort() != 0 {
		name = portQualifiedBackendName(name, ref.GetPort())
	}
	return backendProjectionKey(namespace, name)
}
