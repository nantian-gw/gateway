package xds

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

func (s *Server) ReportStatus(ctx context.Context, report *controlv1.StatusReport) (*controlv1.StatusAck, error) {
	select {
	case <-s.shutdownCh:
		s.logger.Info("rejecting dataplane status report during shutdown")
		s.recordStatusReportRejection(statusReportRejectionShutdown)
		return nil, shutdownStreamError()
	default:
	}

	if err := validateStatusReport(report); err != nil {
		nodeID := ""
		if report != nil {
			nodeID = report.GetNodeId()
		}
		s.logger.Warn(
			"rejecting invalid dataplane status report",
			"node_id",
			nodeID,
			"error",
			err,
		)
		s.recordStatusReportRejection(statusReportRejectionInvalidRequest)
		return nil, err
	}

	nodeID := report.GetNodeId()
	if _, ok := s.nodes.Get(ctx, nodeID); !ok {
		err := status.Error(codes.FailedPrecondition, "status report requires prior xds stream identity")
		s.logger.Warn(
			"rejecting dataplane status report without established xds identity",
			"node_id",
			nodeID,
			"error",
			err,
		)
		s.recordStatusReportRejection(statusReportRejectionUnknownNode)
		return nil, err
	}

	now := time.Now().UTC()
	observedAt := normalizeStatusObservedAt(report, now)
	if report.GetObservedAt() != nil && report.GetObservedAt().AsTime().UTC().After(now) {
		s.logger.Warn(
			"clamping future dataplane status report timestamp",
			"node_id",
			report.GetNodeId(),
			"observed_at",
			report.GetObservedAt().AsTime().UTC(),
			"clamped_to",
			observedAt,
		)
	}

	s.nodes.ObserveReport(
		ctx,
		nodeID,
		report.GetVersion(),
		report.GetReady(),
		report.GetMessage(),
		observedAt,
	)
	return &controlv1.StatusAck{Accepted: true}, nil
}

var _ interface {
	ReportStatus(context.Context, *controlv1.StatusReport) (*controlv1.StatusAck, error)
} = (*Server)(nil)
