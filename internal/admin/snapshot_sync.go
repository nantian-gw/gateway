package admin

import (
	"sort"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
)

type SnapshotSyncResponse struct {
	ReadinessMode   string              `json:"readinessMode"`
	SnapshotVersion string              `json:"snapshotVersion,omitempty"`
	GeneratedAt     time.Time           `json:"generatedAt,omitempty"`
	PublishedFor    string              `json:"publishedFor,omitempty"`
	Summary         SnapshotSyncSummary `json:"summary"`
	Nodes           []SnapshotSyncNode  `json:"nodes"`
	Warnings        []string            `json:"warnings,omitempty"`
}

type SnapshotSyncSummary struct {
	NodeCount                int `json:"nodeCount"`
	ConnectedNodeCount       int `json:"connectedNodeCount"`
	ReadyNodeCount           int `json:"readyNodeCount"`
	CurrentVersionNodeCount  int `json:"currentVersionNodeCount"`
	CurrentVersionReadyCount int `json:"currentVersionReadyCount"`
	DriftedNodeCount         int `json:"driftedNodeCount"`
	DisconnectedNodeCount    int `json:"disconnectedNodeCount"`
	AwaitingReadyNodeCount   int `json:"awaitingReadyNodeCount"`
}

type SnapshotSyncNode struct {
	NodeID           string    `json:"nodeId"`
	Cluster          string    `json:"cluster,omitempty"`
	Connected        bool      `json:"connected"`
	Ready            bool      `json:"ready"`
	DisconnectedAt   time.Time `json:"disconnectedAt,omitempty"`
	DisconnectReason string    `json:"disconnectReason,omitempty"`
	LastConfigStatus string    `json:"lastConfigStatus,omitempty"`
	LastAckVersion   string    `json:"lastAckVersion,omitempty"`
	LastSentVersion  string    `json:"lastSentVersion,omitempty"`
	LastNackVersion  string    `json:"lastNackVersion,omitempty"`
	LastNackNonce    string    `json:"lastNackNonce,omitempty"`
	LastSeenAt       time.Time `json:"lastSeenAt,omitempty"`
	State            string    `json:"state"`
	Reason           string    `json:"reason,omitempty"`
	Message          string    `json:"message,omitempty"`
}

func buildSnapshotSync(
	snapshot *ir.Snapshot,
	nodes []ir.NodeStatus,
	readinessMode string,
	driftWarningThreshold time.Duration,
	now time.Time,
) SnapshotSyncResponse {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	response := SnapshotSyncResponse{
		ReadinessMode: normalizeReadinessMode(readinessMode),
		Summary: SnapshotSyncSummary{
			NodeCount: len(nodes),
		},
		Nodes: make([]SnapshotSyncNode, 0, len(nodes)),
	}

	for _, node := range nodes {
		if node.Connected {
			response.Summary.ConnectedNodeCount++
		}
		if node.Ready {
			response.Summary.ReadyNodeCount++
		}
	}

	if snapshot == nil {
		response.Warnings = []string{"snapshot not ready"}
		for _, node := range nodes {
			response.Nodes = append(response.Nodes, SnapshotSyncNode{
				NodeID:           node.NodeID,
				Cluster:          node.Cluster,
				Connected:        node.Connected,
				Ready:            node.Ready,
				DisconnectedAt:   node.DisconnectedAt,
				DisconnectReason: node.DisconnectReason,
				LastConfigStatus: node.LastConfigStatus,
				LastAckVersion:   node.LastAckVersion,
				LastSentVersion:  node.LastSentVersion,
				LastNackVersion:  node.LastNackVersion,
				LastNackNonce:    node.LastNackNonce,
				LastSeenAt:       node.LastSeenAt,
				State:            "snapshot-missing",
				Reason:           "controlplane has not published a snapshot yet",
				Message:          node.Message,
			})
		}
		sortSnapshotSyncNodes(response.Nodes)
		return response
	}

	response.SnapshotVersion = snapshot.ID
	response.GeneratedAt = snapshot.GeneratedAt
	response.PublishedFor = formatSnapshotAge(snapshot.GeneratedAt, now)

	sync := summarizeNodeSync(snapshot, nodes, driftWarningThreshold, now)
	response.Summary.CurrentVersionNodeCount = sync.currentVersionNodeCount
	response.Summary.CurrentVersionReadyCount = sync.currentVersionReadyCount
	response.Summary.DriftedNodeCount = sync.driftedNodeCount
	response.Warnings = sync.warnings

	for _, node := range nodes {
		item := SnapshotSyncNode{
			NodeID:           node.NodeID,
			Cluster:          node.Cluster,
			Connected:        node.Connected,
			Ready:            node.Ready,
			DisconnectedAt:   node.DisconnectedAt,
			DisconnectReason: node.DisconnectReason,
			LastConfigStatus: node.LastConfigStatus,
			LastAckVersion:   node.LastAckVersion,
			LastSentVersion:  node.LastSentVersion,
			LastNackVersion:  node.LastNackVersion,
			LastNackNonce:    node.LastNackNonce,
			LastSeenAt:       node.LastSeenAt,
			Message:          node.Message,
		}

		switch {
		case !node.Connected:
			response.Summary.DisconnectedNodeCount++
			item.State = "disconnected"
			item.Reason = "node is not currently connected to the controlplane"
		case node.RejectsVersion(snapshot.ID):
			item.State = "rejected"
			item.Reason = "node explicitly rejected the current snapshot"
			if node.LastNackMessage != "" {
				item.Message = node.LastNackMessage
			}
		case node.LastAckVersion != snapshot.ID:
			item.State = "drifted"
			if node.LastAckVersion == "" {
				item.Reason = "connected node has not acknowledged the current snapshot yet"
			} else {
				item.Reason = "connected node last acknowledged an older snapshot"
			}
		case !node.Ready:
			response.Summary.AwaitingReadyNodeCount++
			item.State = "awaiting-ready"
			item.Reason = "node has acknowledged the current snapshot but is not reporting ready yet"
		default:
			item.State = "current-ready"
			item.Reason = "node is connected, ready, and on the current snapshot"
		}

		response.Nodes = append(response.Nodes, item)
	}

	sortSnapshotSyncNodes(response.Nodes)
	return response
}

func sortSnapshotSyncNodes(nodes []SnapshotSyncNode) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeID < nodes[j].NodeID
	})
}
