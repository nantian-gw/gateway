package admin

import (
	"net/url"
	"strings"
)

type topologyQueryFilter struct {
	Type           string
	RouteKind      string
	Namespace      string
	Name           string
	Status         string
	IncludeRelated bool
}

func parseTopologyFilter(query url.Values) (topologyQueryFilter, error) {
	nodeType, err := parseTopologyNodeType(query.Get("type"))
	if err != nil {
		return topologyQueryFilter{}, err
	}
	routeKind, err := parseRouteKindFilter(query.Get("kind"))
	if err != nil {
		return topologyQueryFilter{}, err
	}
	includeRelated, err := parseIncludeRelatedTopology(query.Get("includeRelated"))
	if err != nil {
		return topologyQueryFilter{}, err
	}

	return topologyQueryFilter{
		Type:           nodeType,
		RouteKind:      routeKind,
		Namespace:      strings.TrimSpace(query.Get("namespace")),
		Name:           strings.TrimSpace(query.Get("name")),
		Status:         normalizeTopologyStatus(query.Get("status")),
		IncludeRelated: includeRelated,
	}, nil
}

func parseTopologyNodeType(raw string) (string, error) {
	switch normalizeSortField(raw) {
	case "":
		return "", nil
	case "plane":
		return "plane", nil
	case "listener":
		return "listener", nil
	case "route":
		return "route", nil
	case "backend":
		return "backend", nil
	case "endpointset":
		return "endpoint-set", nil
	default:
		return "", errInvalidQuery("invalid type")
	}
}

func parseIncludeRelatedTopology(raw string) (bool, error) {
	value, err := parseOptionalBool(raw)
	if err != nil {
		return false, errInvalidQuery("invalid includeRelated")
	}
	if value == nil {
		return false, nil
	}
	return *value, nil
}

func normalizeTopologyStatus(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func filterTopology(response TopologyResponse, filter topologyQueryFilter) TopologyResponse {
	if !filter.hasSelectors() {
		return response
	}

	index := newTopologyIndex(response)
	retainedNodes := make(map[string]TopologyNode)
	retainedEdges := make(map[string]TopologyEdge)
	anchors := make([]string, 0)

	for _, node := range response.Nodes {
		if !DoesTopologyNodeMatch(node, filter) {
			continue
		}
		retainedNodes[node.ID] = node
		anchors = append(anchors, node.ID)
	}

	if filter.IncludeRelated {
		for _, nodeID := range anchors {
			index.addIncidentEdges(nodeID, retainedNodes, retainedEdges)
		}

		retainedSnapshot := topologyNodeValues(retainedNodes)
		for _, node := range retainedSnapshot {
			switch node.Type {
			case "route":
				index.addIncidentEdgesByType(node.ID, retainedNodes, retainedEdges, "attach")
			case "listener":
				index.addIncidentEdgesByType(node.ID, retainedNodes, retainedEdges, "serve")
			case "backend":
				index.addIncidentEdgesByType(node.ID, retainedNodes, retainedEdges, "resolve")
			}
		}

		if _, ok := retainedNodes["plane:dataplane"]; ok {
			index.addIncidentEdgesByType("plane:dataplane", retainedNodes, retainedEdges, "control")
		}
	} else {
		for _, edge := range response.Edges {
			if _, ok := retainedNodes[edge.Source]; !ok {
				continue
			}
			if _, ok := retainedNodes[edge.Target]; !ok {
				continue
			}
			retainedEdges[edge.ID] = edge
		}
	}

	out := response
	out.Nodes = topologyNodeValues(retainedNodes)
	out.Edges = topologyEdgeValues(retainedEdges)
	sortTopology(out.Nodes, out.Edges)
	return out
}

func (f topologyQueryFilter) hasSelectors() bool {
	return f.Type != "" || f.RouteKind != "" || f.Namespace != "" || f.Name != "" || f.Status != ""
}

func DoesTopologyNodeMatch(node TopologyNode, filter topologyQueryFilter) bool {
	if filter.Type != "" && node.Type != filter.Type {
		return false
	}
	if filter.RouteKind != "" {
		if node.Type != "route" {
			return false
		}
		if topologyRouteKind(node) != topologyRouteKindLabel(filter.RouteKind) {
			return false
		}
	}
	if filter.Namespace != "" && node.Namespace != filter.Namespace {
		return false
	}
	if filter.Name != "" && node.Name != filter.Name {
		return false
	}
	if filter.Status != "" && normalizeTopologyStatus(node.Status) != filter.Status {
		return false
	}
	return true
}

func topologyRouteKind(node TopologyNode) string {
	if node.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(node.Metadata["kind"])
}

func topologyRouteKindLabel(kind string) string {
	switch kind {
	case kindHTTPRoute:
		return "HTTPRoute"
	case kindGRPCRoute:
		return "GRPCRoute"
	case kindTCPRoute:
		return "TCPRoute"
	case kindUDPRoute:
		return "UDPRoute"
	case kindTLSRoute:
		return "TLSRoute"
	default:
		return ""
	}
}

type topologyIndex struct {
	nodes map[string]TopologyNode
	edges map[string][]TopologyEdge
}

func newTopologyIndex(response TopologyResponse) topologyIndex {
	index := topologyIndex{
		nodes: make(map[string]TopologyNode, len(response.Nodes)),
		edges: make(map[string][]TopologyEdge, len(response.Nodes)),
	}

	for _, node := range response.Nodes {
		index.nodes[node.ID] = node
	}
	for _, edge := range response.Edges {
		index.edges[edge.Source] = append(index.edges[edge.Source], edge)
		index.edges[edge.Target] = append(index.edges[edge.Target], edge)
	}

	return index
}

func (i topologyIndex) addIncidentEdges(
	nodeID string,
	retainedNodes map[string]TopologyNode,
	retainedEdges map[string]TopologyEdge,
) {
	for _, edge := range i.edges[nodeID] {
		i.addEdge(edge, retainedNodes, retainedEdges)
	}
}

func (i topologyIndex) addIncidentEdgesByType(
	nodeID string,
	retainedNodes map[string]TopologyNode,
	retainedEdges map[string]TopologyEdge,
	edgeType string,
) {
	for _, edge := range i.edges[nodeID] {
		if edge.Type != edgeType {
			continue
		}
		i.addEdge(edge, retainedNodes, retainedEdges)
	}
}

func (i topologyIndex) addEdge(
	edge TopologyEdge,
	retainedNodes map[string]TopologyNode,
	retainedEdges map[string]TopologyEdge,
) {
	retainedEdges[edge.ID] = edge

	if node, ok := i.nodes[edge.Source]; ok {
		retainedNodes[node.ID] = node
	}
	if node, ok := i.nodes[edge.Target]; ok {
		retainedNodes[node.ID] = node
	}
}

func topologyNodeValues(nodes map[string]TopologyNode) []TopologyNode {
	out := make([]TopologyNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node)
	}
	return out
}

func topologyEdgeValues(edges map[string]TopologyEdge) []TopologyEdge {
	out := make([]TopologyEdge, 0, len(edges))
	for _, edge := range edges {
		out = append(out, edge)
	}
	return out
}
