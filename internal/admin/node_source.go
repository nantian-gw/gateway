package admin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/nodestatus"
)

const (
	nodeStatusManagedByLabelKey   = "app.kubernetes.io/managed-by"
	nodeStatusManagedByLabelValue = "nantian-gw"
	nodeStatusComponentLabelKey   = "nantian.dev/component"
	nodeStatusComponentLabelValue = "node-status"
	nodeStatusAnnotationKey       = "nantian.dev/node-status"
	nodeStatusNodeIDAnnotationKey = "nantian.dev/node-id"
)

func (s *Server) currentNodes(ctx context.Context, snapshot *ir.Snapshot) []ir.NodeStatus {
	nodes := s.nodes.List(ctx)
	if s.resources == nil {
		return visibleNodes(snapshot, nodes)
	}

	shared, err := s.resources.ListNodeStatuses(ctx)
	if err != nil {
		s.logger.Warn("failed to list shared node status for admin API", "error", err)
		return visibleNodes(snapshot, nodes)
	}

	return visibleNodes(snapshot, nodestatus.FilterStale(nodestatus.Merge(nodes, shared), time.Now().UTC()))
}

func (m *ResourceManager) ListNodeStatuses(ctx context.Context) ([]ir.NodeStatus, error) {
	if m == nil || m.client == nil {
		return nil, nil
	}

	var leases coordinationv1.LeaseList
	if err := m.client.List(
		ctx,
		&leases,
		client.MatchingLabels{
			nodeStatusManagedByLabelKey: nodeStatusManagedByLabelValue,
			nodeStatusComponentLabelKey: nodeStatusComponentLabelValue,
		},
	); err != nil {
		return nil, err
	}

	out := make([]ir.NodeStatus, 0, len(leases.Items))
	for i := range leases.Items {
		status, ok := decodeNodeStatusLease(&leases.Items[i])
		if !ok {
			continue
		}
		out = append(out, status)
	}

	return out, nil
}

func decodeNodeStatusLease(lease *coordinationv1.Lease) (ir.NodeStatus, bool) {
	if lease == nil {
		return ir.NodeStatus{}, false
	}

	raw := strings.TrimSpace(lease.Annotations[nodeStatusAnnotationKey])
	if raw == "" {
		return ir.NodeStatus{}, false
	}

	var status ir.NodeStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return ir.NodeStatus{}, false
	}

	if status.NodeID == "" {
		status.NodeID = strings.TrimSpace(lease.Annotations[nodeStatusNodeIDAnnotationKey])
	}
	if status.NodeID == "" && lease.Spec.HolderIdentity != nil {
		status.NodeID = strings.TrimSpace(*lease.Spec.HolderIdentity)
	}
	if status.NodeID == "" {
		return ir.NodeStatus{}, false
	}
	if status.LastSeenAt.IsZero() && lease.Spec.RenewTime != nil {
		status.LastSeenAt = lease.Spec.RenewTime.Time.UTC()
	}
	if status.ConnectedAt.IsZero() && lease.Spec.AcquireTime != nil {
		status.ConnectedAt = lease.Spec.AcquireTime.Time.UTC()
	}
	if !status.ConnectedAt.IsZero() {
		status.ConnectedAt = status.ConnectedAt.UTC()
	}
	if !status.DisconnectedAt.IsZero() {
		status.DisconnectedAt = status.DisconnectedAt.UTC()
	}
	if !status.LastSeenAt.IsZero() {
		status.LastSeenAt = status.LastSeenAt.UTC()
	}

	return status, true
}
