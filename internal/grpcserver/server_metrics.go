package grpcserver

import "time"

func (s *Server) observeSnapshotSendDuration(duration time.Duration) {
	if s == nil || s.metrics == nil || s.metrics.XDSSnapshotSendDurationSeconds == nil {
		return
	}
	s.metrics.XDSSnapshotSendDurationSeconds.Observe(duration.Seconds())
}

func (s *Server) recordSnapshotSendTimeout() {
	if s == nil || s.metrics == nil || s.metrics.XDSSnapshotSendTimeoutsTotal == nil {
		return
	}
	s.metrics.XDSSnapshotSendTimeoutsTotal.Inc()
}

func (s *Server) recordSnapshotAckTimeout() {
	if s == nil || s.metrics == nil || s.metrics.XDSSnapshotAckTimeoutsTotal == nil {
		return
	}
	s.metrics.XDSSnapshotAckTimeoutsTotal.Inc()
}

func (s *Server) recordStreamTermination(reason string) {
	if s == nil || s.metrics == nil || s.metrics.XDSStreamTerminationsTotal == nil || reason == "" {
		return
	}
	reason = normalizeStreamTerminationReason(reason)
	s.metrics.XDSStreamTerminationsTotal.WithLabelValues(reason).Inc()
}

func (s *Server) recordStatusReportRejection(reason string) {
	if s == nil || s.metrics == nil || s.metrics.XDSStatusReportRejectionsTotal == nil || reason == "" {
		return
	}
	reason = normalizeStatusReportRejectionReason(reason)
	s.metrics.XDSStatusReportRejectionsTotal.WithLabelValues(reason).Inc()
}

func normalizeStreamTerminationReason(reason string) string {
	switch reason {
	case streamTerminationShutdown,
		streamTerminationClientDisconnect,
		streamTerminationStreamError,
		streamTerminationSendTimeout,
		streamTerminationAckTimeout,
		streamTerminationSuperseded,
		streamTerminationInvalidRequest:
		return reason
	default:
		return streamTerminationOther
	}
}

func normalizeStatusReportRejectionReason(reason string) string {
	switch reason {
	case statusReportRejectionShutdown,
		statusReportRejectionInvalidRequest,
		statusReportRejectionUnknownNode:
		return reason
	default:
		return statusReportRejectionOther
	}
}
