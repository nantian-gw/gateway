package admin

import (
	"net/http"
	"sort"
	"strings"

	"github.com/nantian-gw/gateway/internal/ir"
)

type gatewayIdentity struct {
	namespace string
	name      string
}

type gatewayDetail struct {
	Name      string              `json:"name"`
	Namespace string              `json:"namespace"`
	Routes    *routeListResponse  `json:"routes,omitempty"`
	Listeners []ir.Listener       `json:"listeners,omitempty"`
	Backends  []ir.BackendCluster `json:"backends,omitempty"`
	Summary   *gatewaySummary     `json:"summary,omitempty"`
}

type gatewaySummary struct {
	RouteCount    int `json:"routeCount"`
	HTTPCount     int `json:"httpCount"`
	GRPCCount     int `json:"grpcCount"`
	StreamCount   int `json:"streamCount"`
	ListenerCount int `json:"listenerCount"`
	BackendCount  int `json:"backendCount"`
}

type gatewayRoutes struct {
	http   []ir.HTTPRoute
	grpc   []ir.GRPCRoute
	stream []ir.StreamRoute
}

type includeFlags struct {
	routes    bool
	listeners bool
	backends  bool
	summary   bool
}

func parseIncludeFlags(raw string) includeFlags {
	var flags includeFlags
	for _, token := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(token)) {
		case "":
			continue
		case "all":
			return includeFlags{
				routes:    true,
				listeners: true,
				backends:  true,
				summary:   true,
			}
		case "routes":
			flags.routes = true
		case "listeners":
			flags.listeners = true
		case "backends":
			flags.backends = true
		case "summary":
			flags.summary = true
		}
	}
	return flags
}

func (s *Server) handleGateways(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Current()
	if snapshot == nil {
		s.respondJSON(w, map[string]any{"gateways": []any{}, "total": 0})
		return
	}

	include := parseIncludeFlags(r.URL.Query().Get("include"))
	gateways := buildGatewayDetails(snapshot, include)

	s.respondJSON(w, map[string]any{
		"gateways": gateways,
		"total":    len(gateways),
	})
}

func buildGatewayDetails(snapshot *ir.Snapshot, include includeFlags) []gatewayDetail {
	// Build reverse index: gateway identity -> routes

	gatewayMap := make(map[gatewayIdentity]*gatewayRoutes)

	// Collect all gateway identities from route parentRefs
	collectRoute := func(gw gatewayIdentity) *gatewayRoutes {
		if _, ok := gatewayMap[gw]; !ok {
			gatewayMap[gw] = &gatewayRoutes{}
		}
		return gatewayMap[gw]
	}

	for _, route := range snapshot.HTTPRoutes {
		for _, parent := range route.ParentRefs {
			ns := parent.Namespace
			if ns == "" {
				ns = route.Namespace
			}
			gr := collectRoute(gatewayIdentity{namespace: ns, name: parent.Name})
			gr.http = append(gr.http, route)
		}
	}

	for _, route := range snapshot.GRPCRoutes {
		for _, parent := range route.ParentRefs {
			ns := parent.Namespace
			if ns == "" {
				ns = route.Namespace
			}
			gr := collectRoute(gatewayIdentity{namespace: ns, name: parent.Name})
			gr.grpc = append(gr.grpc, route)
		}
	}

	for _, route := range snapshot.StreamRoutes {
		for _, parent := range route.ParentRefs {
			ns := parent.Namespace
			if ns == "" {
				ns = route.Namespace
			}
			gr := collectRoute(gatewayIdentity{namespace: ns, name: parent.Name})
			gr.stream = append(gr.stream, route)
		}
	}

	// Build listener-to-gateway mapping: a listener belongs to a gateway if its
	// AttachedRoutes reference routes that belong to that gateway.
	listenerGatewayMap := buildListenerGatewayMap(snapshot, gatewayMap)

	// Build result
	out := make([]gatewayDetail, 0, len(gatewayMap))
	for gw, routes := range gatewayMap {
		detail := gatewayDetail{
			Name:      gw.name,
			Namespace: gw.namespace,
		}

		if include.listeners {
			detail.Listeners = listenerGatewayMap[gw]
		}

		if include.routes {
			routesResp := newRouteListResponse()
			routesResp.HTTP = append(routesResp.HTTP, routes.http...)
			routesResp.GRPC = append(routesResp.GRPC, routes.grpc...)
			routesResp.Stream = append(routesResp.Stream, routes.stream...)
			detail.Routes = &routesResp
		}

		if include.backends {
			detail.Backends = collectGatewayBackends(snapshot, routes)
		}

		if include.summary {
			summary := &gatewaySummary{
				RouteCount:    len(routes.http) + len(routes.grpc) + len(routes.stream),
				HTTPCount:     len(routes.http),
				GRPCCount:     len(routes.grpc),
				StreamCount:   len(routes.stream),
				ListenerCount: len(listenerGatewayMap[gw]),
				BackendCount:  len(collectGatewayBackendKeys(snapshot, routes)),
			}
			detail.Summary = summary
		}

		out = append(out, detail)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})

	return out
}

func buildListenerGatewayMap(snapshot *ir.Snapshot, gatewayMap map[gatewayIdentity]*gatewayRoutes) map[gatewayIdentity][]ir.Listener {
	// Build a set of route keys (namespace/name) per gateway
	gwRouteKeys := make(map[gatewayIdentity]map[string]struct{})
	for gw, routes := range gatewayMap {
		keys := make(map[string]struct{})
		for _, r := range routes.http {
			keys[r.Namespace+"/"+r.Name] = struct{}{}
		}
		for _, r := range routes.grpc {
			keys[r.Namespace+"/"+r.Name] = struct{}{}
		}
		for _, r := range routes.stream {
			keys[r.Namespace+"/"+r.Name] = struct{}{}
		}
		gwRouteKeys[gw] = keys
	}

	// Map listeners to gateways by checking if their attached routes belong to that gateway
	listenerMap := make(map[gatewayIdentity][]ir.Listener)
	for _, listener := range displayListeners(snapshot.Listeners) {
		// Find which gateway this listener belongs to
		for _, attached := range listener.AttachedRoutes {
			for gw, routeKeys := range gwRouteKeys {
				if _, ok := routeKeys[attached]; ok {
					listenerMap[gw] = append(listenerMap[gw], listener)
					break
				}
			}
		}
	}

	return listenerMap
}

func collectGatewayBackends(snapshot *ir.Snapshot, routes *gatewayRoutes) []ir.BackendCluster {
	keys := collectGatewayBackendKeys(snapshot, routes)
	if len(keys) == 0 {
		return nil
	}

	out := make([]ir.BackendCluster, 0, len(keys))
	for _, backend := range snapshot.Backends {
		if _, ok := keys[backendKey(backend)]; ok {
			out = append(out, backend)
		}
	}
	return out
}

func collectGatewayBackendKeys(snapshot *ir.Snapshot, routes *gatewayRoutes) map[string]struct{} {
	keys := make(map[string]struct{})

	for _, route := range routes.http {
		collectHTTPRuleBackendKeys(keys, route.Namespace, route.Rules)
	}
	for _, route := range routes.grpc {
		collectGRPCRuleBackendKeys(keys, route.Namespace, route.Rules)
	}
	for _, route := range routes.stream {
		collectStreamRuleBackendKeys(keys, route.Namespace, route.Rules)
	}

	return keys
}
