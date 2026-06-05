package admin

import (
	"net/url"
	"strings"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
)

type routeListResponse struct {
	HTTP   []ir.HTTPRoute   `json:"http"`
	GRPC   []ir.GRPCRoute   `json:"grpc"`
	Stream []ir.StreamRoute `json:"stream"`
}

func newRouteListResponse() routeListResponse {
	return routeListResponse{
		HTTP:   make([]ir.HTTPRoute, 0),
		GRPC:   make([]ir.GRPCRoute, 0),
		Stream: make([]ir.StreamRoute, 0),
	}
}

func filterListeners(listeners []ir.Listener, query url.Values) ([]ir.Listener, error) {
	out := make([]ir.Listener, 0)

	sortField, err := parseListenerSortField(query.Get("sort"))
	if err != nil {
		return nil, err
	}
	order, err := parseSortOrder(query.Get("order"))
	if err != nil {
		return nil, err
	}
	pagination, err := parseListPagination(query)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(query.Get("name"))
	hostname := strings.TrimSpace(query.Get("hostname"))
	attachedRoute := strings.TrimSpace(query.Get("attachedRoute"))
	protocol, err := parseProtocolFilter(query.Get("protocol"))
	if err != nil {
		return nil, err
	}

	for _, listener := range listeners {
		if name != "" && listener.Name != name {
			continue
		}
		if protocol != "" && canonicalProtocol(listener.Protocol) != protocol {
			continue
		}
		if hostname != "" && !stringSliceContains(listener.Hostnames, hostname) {
			continue
		}
		if attachedRoute != "" && !stringSliceContains(listener.AttachedRoutes, attachedRoute) {
			continue
		}
		out = append(out, listener)
	}

	sortListeners(out, sortField, order)
	return paginateSlice(out, pagination), nil
}

func filterRoutes(snapshot *ir.Snapshot, query url.Values) (routeListResponse, error) {
	response := newRouteListResponse()
	if snapshot == nil {
		return response, nil
	}

	kind, err := parseRouteKindFilter(query.Get("kind"))
	if err != nil {
		return response, err
	}
	sortField, err := parseRouteSortField(query.Get("sort"))
	if err != nil {
		return response, err
	}
	order, err := parseSortOrder(query.Get("order"))
	if err != nil {
		return response, err
	}
	pagination, err := parseRoutePagination(query, kind)
	if err != nil {
		return response, err
	}

	namespace := strings.TrimSpace(query.Get("namespace"))
	name := strings.TrimSpace(query.Get("name"))
	hostname := strings.TrimSpace(query.Get("hostname"))

	if kind == "" || kind == "HTTP" {
		for _, route := range snapshot.HTTPRoutes {
			if !httpRouteMatches(route, namespace, name, hostname) {
				continue
			}
			response.HTTP = append(response.HTTP, route)
		}
	}
	if kind == "" || kind == "GRPC" {
		for _, route := range snapshot.GRPCRoutes {
			if !grpcRouteMatches(route, namespace, name, hostname) {
				continue
			}
			response.GRPC = append(response.GRPC, route)
		}
	}
	if kind == "" || kind == "TCP" || kind == "UDP" || kind == "TLS" {
		for _, route := range snapshot.StreamRoutes {
			if kind != "" && canonicalRouteKind(route.Kind) != kind {
				continue
			}
			if !streamRouteMatches(route, namespace, name, hostname) {
				continue
			}
			response.Stream = append(response.Stream, route)
		}
	}

	sortHTTPRoutes(response.HTTP, sortField, order)
	sortGRPCRoutes(response.GRPC, sortField, order)
	sortStreamRoutes(response.Stream, sortField, order)

	if kind != "" {
		switch kind {
		case "HTTP":
			response.HTTP = paginateSlice(response.HTTP, pagination)
		case "GRPC":
			response.GRPC = paginateSlice(response.GRPC, pagination)
		default:
			response.Stream = paginateSlice(response.Stream, pagination)
		}
	}

	return response, nil
}

func filterBackends(snapshot *ir.Snapshot, query url.Values) ([]ir.BackendCluster, error) {
	out := make([]ir.BackendCluster, 0)

	sortField, err := parseBackendSortField(query.Get("sort"))
	if err != nil {
		return nil, err
	}
	order, err := parseSortOrder(query.Get("order"))
	if err != nil {
		return nil, err
	}
	pagination, err := parseListPagination(query)
	if err != nil {
		return nil, err
	}

	includeAll, err := parseIncludeAllBackends(query.Get("all"))
	if err != nil {
		return nil, err
	}

	namespace := strings.TrimSpace(query.Get("namespace"))
	name := strings.TrimSpace(query.Get("name"))
	protocol, err := parseBackendProtocolFilter(query.Get("protocol"))
	if err != nil {
		return nil, err
	}
	service := strings.TrimSpace(query.Get("service"))

	for _, backend := range visibleBackends(snapshot, includeAll) {
		if namespace != "" && backend.Namespace != namespace {
			continue
		}
		if name != "" && backend.Name != name {
			continue
		}
		if protocol != "" && canonicalBackendProtocol(backend.Protocol) != protocol {
			continue
		}
		if service != "" && backend.Metadata["service"] != service {
			continue
		}
		out = append(out, backend)
	}

	sortBackends(out, sortField, order)
	return paginateSlice(out, pagination), nil
}

func filterNodes(nodes []ir.NodeStatus, query url.Values) ([]ir.NodeStatus, error) {
	out := make([]ir.NodeStatus, 0)

	sortField, err := parseNodeSortField(query.Get("sort"))
	if err != nil {
		return nil, err
	}
	order, err := parseSortOrder(query.Get("order"))
	if err != nil {
		return nil, err
	}
	pagination, err := parseListPagination(query)
	if err != nil {
		return nil, err
	}

	nodeID := strings.TrimSpace(query.Get("nodeId"))
	cluster := strings.TrimSpace(query.Get("cluster"))
	connected, err := parseOptionalBool(query.Get("connected"))
	if err != nil {
		return nil, err
	}
	ready, err := parseOptionalBool(query.Get("ready"))
	if err != nil {
		return nil, err
	}
	version := strings.TrimSpace(query.Get("version"))

	for _, node := range nodes {
		if nodeID != "" && node.NodeID != nodeID {
			continue
		}
		if cluster != "" && node.Cluster != cluster {
			continue
		}
		if connected != nil && node.Connected != *connected {
			continue
		}
		if ready != nil && node.Ready != *ready {
			continue
		}
		if version != "" && node.LastAckVersion != version && node.LastSentVersion != version && node.LastNackVersion != version {
			continue
		}
		out = append(out, node)
	}

	sortNodes(out, sortField, order)
	return paginateSlice(out, pagination), nil
}

func findNode(nodes []ir.NodeStatus, nodeID string) (ir.NodeStatus, bool) {
	for _, node := range nodes {
		if node.NodeID == nodeID {
			return node, true
		}
	}

	return ir.NodeStatus{}, false
}

func httpRouteMatches(route ir.HTTPRoute, namespace, name, hostname string) bool {
	if namespace != "" && route.Namespace != namespace {
		return false
	}
	if name != "" && route.Name != name {
		return false
	}
	if hostname != "" && !stringSliceContains(route.Hostnames, hostname) {
		return false
	}
	return true
}

func grpcRouteMatches(route ir.GRPCRoute, namespace, name, hostname string) bool {
	if namespace != "" && route.Namespace != namespace {
		return false
	}
	if name != "" && route.Name != name {
		return false
	}
	if hostname != "" && !stringSliceContains(route.Hostnames, hostname) {
		return false
	}
	return true
}

func streamRouteMatches(route ir.StreamRoute, namespace, name, hostname string) bool {
	if namespace != "" && route.Namespace != namespace {
		return false
	}
	if name != "" && route.Name != name {
		return false
	}
	if hostname == "" {
		return true
	}

	for _, rule := range route.Rules {
		for _, match := range rule.Matches {
			if match.SNIHostname == hostname {
				return true
			}
		}
	}

	return false
}
