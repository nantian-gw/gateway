package admin

import "github.com/nantian-gw/gateway/internal/ir"

func visibleNodes(snapshot *ir.Snapshot, nodes []ir.NodeStatus) []ir.NodeStatus {
	if len(nodes) == 0 {
		return nil
	}
	if snapshot == nil || len(snapshot.Workloads) == 0 {
		return nodes
	}

	workloads := make(map[string]struct{}, len(snapshot.Workloads))
	for _, workload := range snapshot.Workloads {
		if workload.Name == "" {
			continue
		}
		workloads[workload.Name] = struct{}{}
	}
	if len(workloads) == 0 {
		return nodes
	}

	out := make([]ir.NodeStatus, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := workloads[node.NodeID]; !ok {
			continue
		}
		out = append(out, node)
	}
	return out
}
