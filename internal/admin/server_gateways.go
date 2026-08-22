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
	gatewayMap := make(map[gatewayIdentity]*gatewayRoutes)

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

	var listenerGatewayMap map[gatewayIdentity][]ir.Listener
	if include.listeners || include.summary {
		listenerGatewayMap = buildListenerGatewayMap(snapshot, gatewayMap)
	}

	var backendIndex gatewayBackendIndex
	if include.backends {
		backendIndex = newGatewayBackendIndex(snapshot.Backends)
	}

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

		var backendKeys map[string]struct{}
		if include.backends || include.summary {
			backendKeys = collectGatewayBackendKeys(routes)
		}

		if include.backends {
			detail.Backends = collectGatewayBackends(backendIndex, backendKeys)
		}

		if include.summary {
			summary := &gatewaySummary{
				RouteCount:    len(routes.http) + len(routes.grpc) + len(routes.stream),
				HTTPCount:     len(routes.http),
				GRPCCount:     len(routes.grpc),
				StreamCount:   len(routes.stream),
				ListenerCount: len(listenerGatewayMap[gw]),
				BackendCount:  len(backendKeys),
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
	routeGateways := make(map[string][]gatewayIdentity)
	for gw, routes := range gatewayMap {
		for _, r := range routes.http {
			routeGateways[r.Namespace+"/"+r.Name] = append(routeGateways[r.Namespace+"/"+r.Name], gw)
		}
		for _, r := range routes.grpc {
			routeGateways[r.Namespace+"/"+r.Name] = append(routeGateways[r.Namespace+"/"+r.Name], gw)
		}
		for _, r := range routes.stream {
			routeGateways[r.Namespace+"/"+r.Name] = append(routeGateways[r.Namespace+"/"+r.Name], gw)
		}
	}

	listenerMap := make(map[gatewayIdentity][]ir.Listener)
	for _, listener := range displayListeners(snapshot.Listeners) {
		var seen map[gatewayIdentity]struct{}
		for _, attached := range listener.AttachedRoutes {
			for _, gw := range routeGateways[attached] {
				if seen == nil {
					seen = make(map[gatewayIdentity]struct{})
				}
				if _, ok := seen[gw]; ok {
					continue
				}
				seen[gw] = struct{}{}
				listenerMap[gw] = append(listenerMap[gw], listener)
			}
		}
	}

	return listenerMap
}

type gatewayBackendIndex struct {
	byKey map[string]ir.BackendCluster
	order map[string]int
}

func newGatewayBackendIndex(backends []ir.BackendCluster) gatewayBackendIndex {
	index := gatewayBackendIndex{
		byKey: make(map[string]ir.BackendCluster, len(backends)),
		order: make(map[string]int, len(backends)),
	}
	for i, backend := range backends {
		key := backendKey(backend)
		index.byKey[key] = backend
		if _, ok := index.order[key]; !ok {
			index.order[key] = i
		}
	}
	return index
}

func collectGatewayBackends(index gatewayBackendIndex, keys map[string]struct{}) []ir.BackendCluster {
	if len(keys) == 0 {
		return nil
	}

	orderedKeys := make([]string, 0, len(keys))
	for key := range keys {
		if _, ok := index.byKey[key]; ok {
			orderedKeys = append(orderedKeys, key)
		}
	}
	sort.Slice(orderedKeys, func(i, j int) bool {
		return index.order[orderedKeys[i]] < index.order[orderedKeys[j]]
	})

	out := make([]ir.BackendCluster, 0, len(keys))
	for _, key := range orderedKeys {
		out = append(out, index.byKey[key])
	}
	return out
}

func collectGatewayBackendKeys(routes *gatewayRoutes) map[string]struct{} {
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
