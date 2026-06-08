package admin

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
)

type TopologyResponse struct {
	SnapshotVersion string            `json:"snapshotVersion,omitempty"`
	GeneratedAt     time.Time         `json:"generatedAt,omitempty"`
	Summary         TopologySummary   `json:"summary"`
	Nodes           []TopologyNode    `json:"nodes"`
	Edges           []TopologyEdge    `json:"edges"`
	Warnings        []string          `json:"warnings,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type TopologySummary struct {
	ListenerCount int `json:"listenerCount"`
	RouteCount    int `json:"routeCount"`
	BackendCount  int `json:"backendCount"`
	NodeCount     int `json:"nodeCount"`
	HealthyEdges  int `json:"healthyEdges"`
}

type TopologyNode struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Label     string            `json:"label"`
	Namespace string            `json:"namespace,omitempty"`
	Name      string            `json:"name,omitempty"`
	Status    string            `json:"status,omitempty"`
	Detail    string            `json:"detail,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type TopologyEdge struct {
	ID       string            `json:"id"`
	Source   string            `json:"source"`
	Target   string            `json:"target"`
	Type     string            `json:"type"`
	Label    string            `json:"label,omitempty"`
	Weight   int               `json:"weight,omitempty"`
	Status   string            `json:"status,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func buildTopology(snapshot *ir.Snapshot, nodes []ir.NodeStatus) TopologyResponse {
	response := TopologyResponse{
		Nodes:    make([]TopologyNode, 0),
		Edges:    make([]TopologyEdge, 0),
		Warnings: make([]string, 0),
		Metadata: map[string]string{"graph": "listener-route-backend"},
	}

	controlplaneID := "plane:controlplane"
	dataplaneID := "plane:dataplane"

	response.Nodes = append(response.Nodes,
		TopologyNode{
			ID:     controlplaneID,
			Type:   "plane",
			Label:  "Controlplane",
			Status: "active",
		},
		TopologyNode{
			ID:     dataplaneID,
			Type:   "plane",
			Label:  "Dataplane Fleet",
			Status: topologyDataplaneStatus(nodes),
			Detail: fmt.Sprintf("%d connected / %d ready", countConnectedNodes(nodes), countReadyNodes(nodes)),
		},
	)
	response.Edges = append(response.Edges, TopologyEdge{
		ID:     "edge:controlplane:dataplane",
		Source: controlplaneID,
		Target: dataplaneID,
		Type:   "control",
		Label:  "xDS",
		Weight: len(nodes),
		Status: topologyDataplaneStatus(nodes),
	})

	if snapshot == nil {
		response.Summary.NodeCount = len(nodes)
		return response
	}

	response.SnapshotVersion = snapshot.ID
	response.GeneratedAt = snapshot.GeneratedAt
	response.Summary.ListenerCount = len(snapshot.Listeners)
	response.Summary.RouteCount = len(snapshot.HTTPRoutes) + len(snapshot.GRPCRoutes) + len(snapshot.StreamRoutes)
	response.Summary.BackendCount = referencedBackendCount(snapshot)
	response.Summary.NodeCount = len(nodes)

	routeIndex := make(map[string][]TopologyNode)
	backendNodeIDs := make(map[string]string)

	for _, rawListener := range snapshot.Listeners {
		listener := displayListener(rawListener)
		nodeID := listenerNodeID(listener.Name)
		response.Nodes = append(response.Nodes, TopologyNode{
			ID:        nodeID,
			Type:      "listener",
			Label:     listener.Name,
			Status:    strings.ToLower(listener.Protocol),
			Detail:    fmt.Sprintf("%s:%d", defaultAddress(listener.Address), listener.Port),
			Metadata:  map[string]string{"protocol": listener.Protocol, "hostnames": strings.Join(listener.Hostnames, ", ")},
			Namespace: listener.Metadata["nantian.dev/frontend-namespace"],
		})
		response.Edges = append(response.Edges, TopologyEdge{
			ID:     "edge:" + dataplaneID + ":" + nodeID,
			Source: dataplaneID,
			Target: nodeID,
			Type:   "serve",
			Label:  listener.Protocol,
			Weight: len(listener.AttachedRoutes),
			Status: "active",
		})
	}

	for _, route := range snapshot.HTTPRoutes {
		node := topologyRouteNode("HTTPRoute", route.Namespace, route.Name, len(route.Rules), route.Hostnames)
		response.Nodes = append(response.Nodes, node)
		routeIndex[route.Namespace+"/"+route.Name] = append(routeIndex[route.Namespace+"/"+route.Name], node)
		response.Edges = append(response.Edges, topologyBackendEdgesFromHTTP(node.ID, route.Namespace, route.Rules, backendNodeIDs, snapshot.Backends, &response)...)
	}
	for _, route := range snapshot.GRPCRoutes {
		node := topologyRouteNode("GRPCRoute", route.Namespace, route.Name, len(route.Rules), route.Hostnames)
		response.Nodes = append(response.Nodes, node)
		routeIndex[route.Namespace+"/"+route.Name] = append(routeIndex[route.Namespace+"/"+route.Name], node)
		response.Edges = append(response.Edges, topologyBackendEdgesFromGRPC(node.ID, route.Namespace, route.Rules, backendNodeIDs, snapshot.Backends, &response)...)
	}
	for _, route := range snapshot.StreamRoutes {
		node := topologyRouteNode(route.Kind+"Route", route.Namespace, route.Name, len(route.Rules), topologyStreamHostnames(route))
		response.Nodes = append(response.Nodes, node)
		routeIndex[route.Namespace+"/"+route.Name] = append(routeIndex[route.Namespace+"/"+route.Name], node)
		response.Edges = append(response.Edges, topologyBackendEdgesFromStream(node.ID, route.Namespace, route.Rules, backendNodeIDs, snapshot.Backends, &response)...)
	}

	for _, listener := range snapshot.Listeners {
		sourceID := listenerNodeID(listener.Name)
		for _, attachedRoute := range listener.AttachedRoutes {
			routeNodes := routeIndex[attachedRoute]
			if len(routeNodes) == 0 {
				response.Warnings = append(response.Warnings, fmt.Sprintf("listener %s references unknown route %s", listener.Name, attachedRoute))
				continue
			}
			for _, routeNode := range routeNodes {
				response.Edges = append(response.Edges, TopologyEdge{
					ID:     fmt.Sprintf("edge:%s:%s", sourceID, routeNode.ID),
					Source: sourceID,
					Target: routeNode.ID,
					Type:   "attach",
					Label:  "attached",
					Weight: 1,
					Status: "active",
				})
			}
		}
	}

	sortTopology(response.Nodes, response.Edges)
	for _, edge := range response.Edges {
		if edge.Type == "forward" && edge.Status == "healthy" {
			response.Summary.HealthyEdges++
		}
	}

	return response
}

func topologyRouteNode(kind, namespace, name string, rules int, hostnames []string) TopologyNode {
	return TopologyNode{
		ID:        routeNodeID(kind, namespace, name),
		Type:      "route",
		Label:     name,
		Namespace: namespace,
		Name:      name,
		Status:    strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(kind, "Route"), "route")),
		Detail:    fmt.Sprintf("%s · %d rules", kind, rules),
		Metadata:  map[string]string{"kind": kind, "hostnames": strings.Join(hostnames, ", ")},
	}
}

func topologyBackendEdgesFromHTTP(
	sourceID string,
	defaultNamespace string,
	rules []ir.HTTPRule,
	backendNodeIDs map[string]string,
	backends []ir.BackendCluster,
	response *TopologyResponse,
) []TopologyEdge {
	refs := make([]ir.BackendRef, 0)
	for _, rule := range rules {
		refs = append(refs, rule.BackendRefs...)
	}
	return topologyBackendEdges(sourceID, defaultNamespace, refs, backendNodeIDs, backends, response)
}

func topologyBackendEdgesFromGRPC(
	sourceID string,
	defaultNamespace string,
	rules []ir.GRPCRule,
	backendNodeIDs map[string]string,
	backends []ir.BackendCluster,
	response *TopologyResponse,
) []TopologyEdge {
	refs := make([]ir.BackendRef, 0)
	for _, rule := range rules {
		refs = append(refs, rule.BackendRefs...)
	}
	return topologyBackendEdges(sourceID, defaultNamespace, refs, backendNodeIDs, backends, response)
}

func topologyBackendEdgesFromStream(
	sourceID string,
	defaultNamespace string,
	rules []ir.StreamRule,
	backendNodeIDs map[string]string,
	backends []ir.BackendCluster,
	response *TopologyResponse,
) []TopologyEdge {
	refs := make([]ir.BackendRef, 0)
	for _, rule := range rules {
		refs = append(refs, rule.BackendRefs...)
	}
	return topologyBackendEdges(sourceID, defaultNamespace, refs, backendNodeIDs, backends, response)
}

func topologyBackendEdges(
	sourceID string,
	defaultNamespace string,
	refs []ir.BackendRef,
	backendNodeIDs map[string]string,
	backends []ir.BackendCluster,
	response *TopologyResponse,
) []TopologyEdge {
	type aggregate struct {
		weight int
		ref    ir.BackendRef
	}

	aggregates := make(map[string]aggregate)
	for _, ref := range refs {
		namespace := ref.Namespace
		if namespace == "" {
			namespace = defaultNamespace
		}
		key := backendNodeKey(namespace, ref.Name, ref.Port)
		item := aggregates[key]
		item.ref = ref
		weight := int(ref.Weight)
		if weight <= 0 {
			weight = 1
		}
		item.weight += weight
		aggregates[key] = item
	}

	keys := make([]string, 0, len(aggregates))
	for key := range aggregates {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	edges := make([]TopologyEdge, 0, len(keys))
	for _, key := range keys {
		item := aggregates[key]
		namespace := item.ref.Namespace
		if namespace == "" {
			namespace = defaultNamespace
		}
		backendID := ensureBackendTopologyNode(namespace, item.ref.Name, item.ref.Port, backendNodeIDs, backends, response)
		edges = append(edges, TopologyEdge{
			ID:     fmt.Sprintf("edge:%s:%s", sourceID, backendID),
			Source: sourceID,
			Target: backendID,
			Type:   "forward",
			Label:  fmt.Sprintf("%s:%d", item.ref.Name, item.ref.Port),
			Weight: item.weight,
			Status: topologyBackendEdgeStatus(namespace, item.ref.Name, item.ref.Port, backends),
		})
	}

	return edges
}

func ensureBackendTopologyNode(
	namespace string,
	name string,
	port uint32,
	backendNodeIDs map[string]string,
	backends []ir.BackendCluster,
	response *TopologyResponse,
) string {
	key := backendNodeKey(namespace, name, port)
	if nodeID, ok := backendNodeIDs[key]; ok {
		return nodeID
	}

	nodeID := backendNodeID(namespace, name, port)
	cluster, ok := findTopologyBackend(backends, namespace, name, port)
	detail := fmt.Sprintf("%s:%d", name, port)
	status := "unknown"
	metadata := map[string]string{"service": name, "port": fmt.Sprintf("%d", port)}
	if ok {
		detail = fmt.Sprintf("%s · %d/%d healthy", cluster.Protocol, healthyEndpoints(cluster), len(cluster.Endpoints))
		status = topologyBackendStatus(cluster)
		metadata["protocol"] = cluster.Protocol
		metadata["healthyEndpoints"] = fmt.Sprintf("%d", healthyEndpoints(cluster))
		metadata["totalEndpoints"] = fmt.Sprintf("%d", len(cluster.Endpoints))
	}

	response.Nodes = append(response.Nodes, TopologyNode{
		ID:        nodeID,
		Type:      "backend",
		Label:     fmt.Sprintf("%s:%d", name, port),
		Namespace: namespace,
		Name:      name,
		Status:    status,
		Detail:    detail,
		Metadata:  metadata,
	})

	endpointNodeID := endpointSetNodeID(namespace, name, port)
	response.Nodes = append(response.Nodes, TopologyNode{
		ID:        endpointNodeID,
		Type:      "endpoint-set",
		Label:     fmt.Sprintf("%s endpoints", name),
		Namespace: namespace,
		Name:      name,
		Status:    status,
		Detail:    detail,
		Metadata:  metadata,
	})
	response.Edges = append(response.Edges, TopologyEdge{
		ID:     fmt.Sprintf("edge:%s:%s", nodeID, endpointNodeID),
		Source: nodeID,
		Target: endpointNodeID,
		Type:   "resolve",
		Label:  "endpoints",
		Weight: 1,
		Status: status,
	})

	backendNodeIDs[key] = nodeID
	return nodeID
}

func findTopologyBackend(backends []ir.BackendCluster, namespace, service string, port uint32) (ir.BackendCluster, bool) {
	exactName := fmt.Sprintf("%s:%d", service, port)
	for _, backend := range backends {
		if backend.Namespace == namespace && (backend.Name == exactName || backend.Metadata["service"] == service) {
			return backend, true
		}
	}

	return ir.BackendCluster{}, false
}

func healthyEndpoints(cluster ir.BackendCluster) int {
	total := 0
	for _, endpoint := range cluster.Endpoints {
		if endpoint.Healthy {
			total++
		}
	}
	return total
}

func topologyBackendStatus(cluster ir.BackendCluster) string {
	if len(cluster.Endpoints) == 0 {
		return "empty"
	}
	if healthyEndpoints(cluster) == 0 {
		return "degraded"
	}
	if healthyEndpoints(cluster) == len(cluster.Endpoints) {
		return "healthy"
	}
	return "mixed"
}

func topologyBackendEdgeStatus(namespace, name string, port uint32, backends []ir.BackendCluster) string {
	cluster, ok := findTopologyBackend(backends, namespace, name, port)
	if !ok {
		return "unknown"
	}
	return topologyBackendStatus(cluster)
}

func topologyDataplaneStatus(nodes []ir.NodeStatus) string {
	if len(nodes) == 0 {
		return "empty"
	}
	if countReadyNodes(nodes) == len(nodes) {
		return "healthy"
	}
	if countReadyNodes(nodes) == 0 {
		return "warming"
	}
	return "mixed"
}

func countConnectedNodes(nodes []ir.NodeStatus) int {
	count := 0
	for _, node := range nodes {
		if node.Connected {
			count++
		}
	}
	return count
}

func countReadyNodes(nodes []ir.NodeStatus) int {
	count := 0
	for _, node := range nodes {
		if node.Ready {
			count++
		}
	}
	return count
}

func topologyStreamHostnames(route ir.StreamRoute) []string {
	out := make([]string, 0)
	for _, rule := range route.Rules {
		for _, match := range rule.Matches {
			if strings.TrimSpace(match.SNIHostname) == "" {
				continue
			}
			out = append(out, match.SNIHostname)
		}
	}
	return out
}

func sortTopology(nodes []TopologyNode, edges []TopologyEdge) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type < nodes[j].Type
		}
		return nodes[i].ID < nodes[j].ID
	})
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].ID < edges[j].ID
	})
}

func listenerNodeID(name string) string {
	return "listener:" + name
}

func routeNodeID(kind, namespace, name string) string {
	return fmt.Sprintf("route:%s:%s/%s", kind, namespace, name)
}

func backendNodeID(namespace, name string, port uint32) string {
	return fmt.Sprintf("backend:%s/%s:%d", namespace, name, port)
}

func endpointSetNodeID(namespace, name string, port uint32) string {
	return fmt.Sprintf("endpoint-set:%s/%s:%d", namespace, name, port)
}

func backendNodeKey(namespace, name string, port uint32) string {
	return fmt.Sprintf("%s/%s:%d", namespace, name, port)
}

func defaultAddress(address string) string {
	if strings.TrimSpace(address) == "" {
		return "*"
	}
	return address
}
