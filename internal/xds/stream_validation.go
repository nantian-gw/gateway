package xds

import (
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

func validateInitialDiscoveryRequest(req *controlv1.DiscoveryRequest) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "initial discovery request is required")
	}
	if strings.TrimSpace(req.GetNodeId()) == "" {
		return status.Error(codes.InvalidArgument, "initial discovery request must include node_id")
	}
	return nil
}

func validateDiscoveryRequestNodeID(streamNodeID string, req *controlv1.DiscoveryRequest) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "discovery request is required")
	}

	receivedNodeID := strings.TrimSpace(req.GetNodeId())
	if receivedNodeID == "" {
		return nil
	}
	if receivedNodeID != strings.TrimSpace(streamNodeID) {
		return status.Errorf(
			codes.InvalidArgument,
			"discovery request node_id %q does not match stream node_id %q",
			receivedNodeID,
			streamNodeID,
		)
	}
	return nil
}

func validateStatusReport(report *controlv1.StatusReport) error {
	if report == nil {
		return status.Error(codes.InvalidArgument, "status report is required")
	}
	if strings.TrimSpace(report.GetNodeId()) == "" {
		return status.Error(codes.InvalidArgument, "status report must include node_id")
	}
	return nil
}

func normalizeStatusObservedAt(report *controlv1.StatusReport, now time.Time) time.Time {
	if report == nil || report.GetObservedAt() == nil {
		return now.UTC()
	}

	observedAt := report.GetObservedAt().AsTime().UTC()
	if observedAt.IsZero() || observedAt.After(now.UTC()) {
		return now.UTC()
	}
	return observedAt
}
