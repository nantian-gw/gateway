package admin

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
)

const (
	readinessModeSnapshot           = "snapshot"
	readinessModeCurrentSnapshotAny = "current-snapshot-any"
	readinessModeCurrentSnapshotAll = "current-snapshot-all"
)

type nodeSyncSummary struct {
	connectedNodeCount       int
	currentVersionNodeCount  int
	currentVersionReadyCount int
	driftedNodeCount         int
	warnings                 []string
}

func normalizeReadinessMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case readinessModeCurrentSnapshotAny:
		return readinessModeCurrentSnapshotAny
	case readinessModeCurrentSnapshotAll:
		return readinessModeCurrentSnapshotAll
	default:
		return readinessModeSnapshot
	}
}

func requiresCurrentSnapshotReadiness(mode string) bool {
	return normalizeReadinessMode(mode) != readinessModeSnapshot
}

func summarizeNodeSync(snapshot *ir.Snapshot, nodes []ir.NodeStatus, driftWarningThreshold time.Duration, now time.Time) nodeSyncSummary {
	if snapshot == nil || snapshot.ID == "" || len(nodes) == 0 {
		return nodeSyncSummary{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	summary := nodeSyncSummary{}
	driftedNodeIDs := make([]string, 0)
	for _, node := range nodes {
		if node.LastAckVersion == snapshot.ID && !node.RejectsVersion(snapshot.ID) {
			summary.currentVersionNodeCount++
		}
		if !node.Connected {
			continue
		}
		summary.connectedNodeCount++
		if node.LastAckVersion != snapshot.ID || node.RejectsVersion(snapshot.ID) {
			summary.driftedNodeCount++
			driftedNodeIDs = append(driftedNodeIDs, node.NodeID)
			continue
		}
		if node.Ready {
			summary.currentVersionReadyCount++
		}
	}

	if summary.driftedNodeCount > 0 && driftWarningTriggered(snapshot, driftWarningThreshold, now) {
		sort.Strings(driftedNodeIDs)
		summary.warnings = append(summary.warnings, fmt.Sprintf(
			"snapshot %s has been published for %s and %d connected dataplane node(s) are still on older versions: %s",
			snapshot.ID,
			formatSnapshotAge(snapshot.GeneratedAt, now),
			summary.driftedNodeCount,
			summarizeNodeIDs(driftedNodeIDs),
		))
	}
	if summary.connectedNodeCount > 0 && summary.currentVersionReadyCount == 0 {
		summary.warnings = append(summary.warnings, "no connected dataplane node is ready on the current snapshot")
	}

	return summary
}

func driftWarningTriggered(snapshot *ir.Snapshot, threshold time.Duration, now time.Time) bool {
	if snapshot == nil {
		return false
	}
	if threshold <= 0 {
		return true
	}
	if snapshot.GeneratedAt.IsZero() {
		return false
	}
	return now.Sub(snapshot.GeneratedAt.UTC()) >= threshold
}

func formatSnapshotAge(generatedAt, now time.Time) string {
	if generatedAt.IsZero() {
		return "0s"
	}

	age := now.Sub(generatedAt.UTC())
	if age < 0 {
		age = 0
	}
	if age < time.Second {
		return age.Round(time.Millisecond).String()
	}
	return age.Round(time.Second).String()
}

func summarizeNodeIDs(nodeIDs []string) string {
	const sampleSize = 3

	if len(nodeIDs) == 0 {
		return "none"
	}
	if len(nodeIDs) <= sampleSize {
		return strings.Join(nodeIDs, ", ")
	}
	return fmt.Sprintf("%s (+%d more)", strings.Join(nodeIDs[:sampleSize], ", "), len(nodeIDs)-sampleSize)
}
