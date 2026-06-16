package ir

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type NodeStatus struct {
	NodeID            string    `json:"nodeId"`
	Cluster           string    `json:"cluster"`
	Connected         bool      `json:"connected"`
	ConnectedAt       time.Time `json:"connectedAt,omitempty"`
	DisconnectedAt    time.Time `json:"disconnectedAt,omitempty"`
	DisconnectReason  string    `json:"disconnectReason,omitempty"`
	LastSeenAt        time.Time `json:"lastSeenAt,omitempty"`
	LastSentVersion   string    `json:"lastSentVersion,omitempty"`
	LastAckVersion    string    `json:"lastAckVersion,omitempty"`
	LastNonce         string    `json:"lastNonce,omitempty"`
	LastConfigStatus  string    `json:"lastConfigStatus,omitempty"`
	LastNackVersion   string    `json:"lastNackVersion,omitempty"`
	LastNackNonce     string    `json:"lastNackNonce,omitempty"`
	LastNackMessage   string    `json:"lastNackMessage,omitempty"`
	Ready             bool      `json:"ready"`
	Message           string    `json:"message,omitempty"`
	Subscriptions     []string  `json:"subscriptions,omitempty"`
	SupportedFeatures []string  `json:"supportedFeatures,omitempty"`
}

func (n NodeStatus) RejectsVersion(version string) bool {
	version = strings.TrimSpace(version)
	return version != "" &&
		n.LastConfigStatus == "NACK" &&
		strings.TrimSpace(n.LastNackVersion) == version
}

type NodeStatusStore struct {
	mu    sync.RWMutex
	nodes map[string]NodeStatus
}

func NewNodeStatusStore() *NodeStatusStore {
	return &NodeStatusStore{
		nodes: make(map[string]NodeStatus),
	}
}

func (s *NodeStatusStore) Connect(nodeID, cluster string, subscriptions []string, now time.Time) NodeStatus {
	return s.ConnectWithFeatures(nodeID, cluster, subscriptions, nil, now)
}

func (s *NodeStatusStore) ConnectWithFeatures(
	nodeID, cluster string,
	subscriptions []string,
	supportedFeatures []string,
	now time.Time,
) NodeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	node := s.nodes[nodeID]
	wasConnected := node.Connected
	node.NodeID = nodeID
	node.Cluster = cluster
	node.Connected = true
	if !wasConnected || node.ConnectedAt.IsZero() {
		node.ConnectedAt = now
	}
	if !wasConnected {
		node.DisconnectedAt = time.Time{}
		node.DisconnectReason = ""
		node.Message = ""
	}
	node.LastSeenAt = now
	node.Subscriptions = cloneStrings(subscriptions)
	node.SupportedFeatures = cloneStrings(supportedFeatures)
	s.nodes[nodeID] = node

	return cloneNodeStatus(node)
}

func (s *NodeStatusStore) Disconnect(nodeID string, now time.Time) (NodeStatus, bool) {
	return s.DisconnectWithReason(nodeID, now, "", "")
}

func (s *NodeStatusStore) DisconnectWithMessage(nodeID string, now time.Time, message string) (NodeStatus, bool) {
	return s.DisconnectWithReason(nodeID, now, "", message)
}

func (s *NodeStatusStore) DisconnectWithReason(nodeID string, now time.Time, reason, message string) (NodeStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return NodeStatus{}, false
	}

	node.Connected = false
	node.Ready = false
	node.LastSeenAt = now
	node.DisconnectedAt = now
	node.DisconnectReason = strings.TrimSpace(reason)
	node.Message = ""
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		node.Message = trimmed
	}
	s.nodes[nodeID] = node

	return cloneNodeStatus(node), true
}

func (s *NodeStatusStore) ObserveAck(nodeID, cluster, version, nonce string, subscriptions []string, now time.Time) NodeStatus {
	return s.ObserveAckWithFeatures(nodeID, cluster, version, nonce, subscriptions, nil, now)
}

func (s *NodeStatusStore) ObserveAckWithFeatures(
	nodeID, cluster, version, nonce string,
	subscriptions []string,
	supportedFeatures []string,
	now time.Time,
) NodeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	node := s.nodes[nodeID]
	node.NodeID = nodeID
	node.Cluster = cluster
	node.LastAckVersion = version
	node.LastNonce = nonce
	node.LastConfigStatus = "ACK"
	node.LastNackVersion = ""
	node.LastNackNonce = ""
	node.LastNackMessage = ""
	node.LastSeenAt = now
	if len(subscriptions) > 0 {
		node.Subscriptions = cloneStrings(subscriptions)
	}
	node.SupportedFeatures = cloneStrings(supportedFeatures)
	s.nodes[nodeID] = node

	return cloneNodeStatus(node)
}

func (s *NodeStatusStore) ObserveNack(
	nodeID, cluster, version, nonce, message string,
	subscriptions []string,
	now time.Time,
) NodeStatus {
	return s.ObserveNackWithFeatures(nodeID, cluster, version, nonce, message, subscriptions, nil, now)
}

func (s *NodeStatusStore) ObserveNackWithFeatures(
	nodeID, cluster, version, nonce, message string,
	subscriptions []string,
	supportedFeatures []string,
	now time.Time,
) NodeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	node := s.nodes[nodeID]
	node.NodeID = nodeID
	node.Cluster = cluster
	node.LastConfigStatus = "NACK"
	node.LastNackVersion = version
	node.LastNackNonce = nonce
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		node.LastNackMessage = trimmed
		node.Message = trimmed
	}
	node.LastSeenAt = now
	if len(subscriptions) > 0 {
		node.Subscriptions = cloneStrings(subscriptions)
	}
	node.SupportedFeatures = cloneStrings(supportedFeatures)
	s.nodes[nodeID] = node

	return cloneNodeStatus(node)
}

func (s *NodeStatusStore) ObservePublished(nodeID, version string, now time.Time) (NodeStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return NodeStatus{}, false
	}

	node.LastSentVersion = version
	node.LastSeenAt = now
	s.nodes[nodeID] = node

	return cloneNodeStatus(node), true
}

func (s *NodeStatusStore) ObserveReport(nodeID, version string, ready bool, message string, observedAt time.Time) NodeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	node := s.nodes[nodeID]
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	if !node.LastSeenAt.IsZero() && observedAt.Before(node.LastSeenAt) {
		return cloneNodeStatus(node)
	}
	node.NodeID = nodeID
	disconnected := !node.Connected && (!node.DisconnectedAt.IsZero() || node.DisconnectReason != "")
	if disconnected {
		node.Ready = false
	} else {
		node.Ready = ready
		if trimmed := strings.TrimSpace(message); trimmed != "" {
			node.Message = trimmed
		}
		if version != "" {
			node.LastAckVersion = version
		}
	}
	node.LastSeenAt = observedAt
	s.nodes[nodeID] = node

	return cloneNodeStatus(node)
}

func (s *NodeStatusStore) Upsert(node NodeStatus) NodeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	node = cloneNodeStatus(node)
	if !node.ConnectedAt.IsZero() {
		node.ConnectedAt = node.ConnectedAt.UTC()
	}
	if !node.DisconnectedAt.IsZero() {
		node.DisconnectedAt = node.DisconnectedAt.UTC()
	}
	if !node.LastSeenAt.IsZero() {
		node.LastSeenAt = node.LastSeenAt.UTC()
	}
	s.nodes[node.NodeID] = node

	return cloneNodeStatus(node)
}

func (s *NodeStatusStore) Get(nodeID string) (NodeStatus, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return NodeStatus{}, false
	}

	return cloneNodeStatus(node), true
}

func (s *NodeStatusStore) List() []NodeStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]NodeStatus, 0, len(s.nodes))
	for _, node := range s.nodes {
		out = append(out, cloneNodeStatus(node))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NodeID < out[j].NodeID
	})

	return out
}

func cloneNodeStatus(node NodeStatus) NodeStatus {
	node.Subscriptions = cloneStrings(node.Subscriptions)
	node.SupportedFeatures = cloneStrings(node.SupportedFeatures)
	return node
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, len(values))
	copy(out, values)
	return out
}
