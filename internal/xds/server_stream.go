package xds

import (
	"context"
	"errors"
	"io"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

func (s *Server) StreamConfiguration(stream controlv1.ConfigurationDiscoveryService_StreamConfigurationServer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic recovered in StreamConfiguration handler", "panic", r)
			err = status.Error(codes.Internal, "internal server error")
		}
	}()

	req, err := s.recvInitialRequest(stream)
	if err != nil {
		if status.Code(err) == codes.Unavailable {
			s.recordStreamTermination(streamTerminationShutdown)
		}
		return err
	}
	if err := validateInitialDiscoveryRequest(req); err != nil {
		s.logger.Warn(
			"rejecting dataplane stream with invalid initial discovery request",
			"node_id",
			req.GetNodeId(),
			"error",
			err,
		)
		s.recordStreamTermination(streamTerminationInvalidRequest)
		return err
	}

	nodeID := req.GetNodeId()
	advertisedFeatures := canonicalizeSupportedFeatures(req.GetSupportedFeatures())
	registration := s.registerStream(nodeID)
	defer s.unregisterStream(registration)
	s.logger.Info("dataplane connected", "node_id", nodeID, "version", req.GetVersion())
	if s.isActiveStream(registration) {
		s.nodes.ConnectWithFeatures(
			stream.Context(),
			nodeID,
			req.GetCluster(),
			req.GetSubscriptions(),
			advertisedFeatures,
			time.Now().UTC(),
		)
	}

	sub, unsubscribe := s.store.Subscribe()
	defer unsubscribe()
	recvCh := make(chan *controlv1.DiscoveryRequest, 8)
	errCh := make(chan error, 1)
	idleHeartbeat := time.NewTicker(s.runtime.streamIdleHeartbeat)
	defer idleHeartbeat.Stop()
	supersededCh := registrationSuperseded(registration)
	sender := newDiscoveryResponseSender(stream)
	defer sender.close()
	var ackTimer *time.Timer
	var ackTimerCh <-chan time.Time
	pendingAckVersion := ""

	stopAckTimer := func() {
		if ackTimer == nil {
			ackTimerCh = nil
			return
		}
		if !ackTimer.Stop() {
			select {
			case <-ackTimer.C:
			default:
			}
		}
		ackTimerCh = nil
	}
	defer stopAckTimer()

	armAckTimer := func(version string) {
		pendingAckVersion = version
		if ackTimer == nil {
			ackTimer = time.NewTimer(s.runtime.snapshotAckTimeout)
		} else {
			if !ackTimer.Stop() {
				select {
				case <-ackTimer.C:
				default:
				}
			}
			ackTimer.Reset(s.runtime.snapshotAckTimeout)
		}
		ackTimerCh = ackTimer.C
	}

	clearAckTimer := func(version string) {
		if pendingAckVersion == "" || version == "" || pendingAckVersion != version {
			return
		}
		pendingAckVersion = ""
		stopAckTimer()
	}
	terminate := func(reason string, err error) error {
		s.recordStreamTermination(reason)
		return err
	}

	go func() {
		for {
			next, err := stream.Recv()
			if err != nil {
				errCh <- err
				close(recvCh)
				return
			}

			recvCh <- next
		}
	}()

	for {
		select {
		case <-supersededCh:
			s.logger.Info("draining superseded dataplane stream", "node_id", nodeID)
			return terminate(streamTerminationSuperseded, supersededStreamError())
		case <-s.shutdownCh:
			s.disconnectStreamIfActive(
				stream.Context(),
				registration,
				time.Now().UTC(),
				streamTerminationShutdown,
				streamDisconnectMessageShutdown,
			)
			s.logger.Info("draining dataplane stream for shutdown", "node_id", nodeID)
			return terminate(streamTerminationShutdown, shutdownStreamError())
		case <-stream.Context().Done():
			s.disconnectStreamIfActive(
				stream.Context(),
				registration,
				time.Now().UTC(),
				streamTerminationClientDisconnect,
				streamDisconnectMessageClientDisconnect,
			)
			s.logger.Info("dataplane disconnected", "node_id", nodeID)
			return terminate(streamTerminationClientDisconnect, nil)
		case err := <-errCh:
			if err == nil || errors.Is(err, io.EOF) {
				s.disconnectStreamIfActive(
					stream.Context(),
					registration,
					time.Now().UTC(),
					streamTerminationClientDisconnect,
					streamDisconnectMessageClientDisconnect,
				)
				return terminate(streamTerminationClientDisconnect, nil)
			}
			s.disconnectStreamIfActive(
				stream.Context(),
				registration,
				time.Now().UTC(),
				streamTerminationStreamError,
				streamDisconnectMessageForStreamError(err),
			)
			return terminate(streamTerminationStreamError, err)
		case <-ackTimerCh:
			s.recordSnapshotAckTimeout()
			s.disconnectStreamIfActive(
				stream.Context(),
				registration,
				time.Now().UTC(),
				streamTerminationAckTimeout,
				streamDisconnectMessageAckTimeout,
			)
			s.logger.Warn(
				"timed out waiting for dataplane snapshot ack; disconnecting stale stream",
				"node_id",
				nodeID,
				"version",
				pendingAckVersion,
				"timeout",
				s.runtime.snapshotAckTimeout,
			)
			return terminate(
				streamTerminationAckTimeout,
				snapshotAckTimeoutError(pendingAckVersion, s.runtime.snapshotAckTimeout),
			)
		case next, ok := <-recvCh:
			if !ok {
				continue
			}
			if !s.isActiveStream(registration) {
				return terminate(streamTerminationSuperseded, supersededStreamError())
			}
			now := time.Now().UTC()
			if err := validateDiscoveryRequestNodeID(nodeID, next); err != nil {
				s.disconnectStreamIfActive(
					stream.Context(),
					registration,
					now,
					streamTerminationInvalidRequest,
					streamDisconnectMessageInvalidRequest,
				)
				s.logger.Warn(
					"rejecting dataplane stream with mismatched discovery request node_id",
					"node_id",
					nodeID,
					"received_node_id",
					next.GetNodeId(),
					"error",
					err,
				)
				return terminate(streamTerminationInvalidRequest, err)
			}
			if len(next.GetSupportedFeatures()) > 0 {
				advertisedFeatures = canonicalizeSupportedFeatures(next.GetSupportedFeatures())
			}
			switch next.GetResultStatus() {
			case controlv1.DiscoveryResultStatus_DISCOVERY_RESULT_STATUS_ACK:
				s.nodes.ObserveAckWithFeatures(
					stream.Context(),
					nodeID,
					next.GetCluster(),
					next.GetVersion(),
					next.GetNonce(),
					next.GetSubscriptions(),
					advertisedFeatures,
					now,
				)
				clearAckTimer(next.GetVersion())
			case controlv1.DiscoveryResultStatus_DISCOVERY_RESULT_STATUS_NACK:
				s.nodes.ObserveNackWithFeatures(
					stream.Context(),
					nodeID,
					next.GetCluster(),
					next.GetVersion(),
					next.GetNonce(),
					next.GetErrorDetail(),
					next.GetSubscriptions(),
					advertisedFeatures,
					now,
				)
				clearAckTimer(next.GetVersion())
			default:
				s.nodes.ConnectWithFeatures(
					stream.Context(),
					nodeID,
					next.GetCluster(),
					next.GetSubscriptions(),
					advertisedFeatures,
					now,
				)
			}
		case <-idleHeartbeat.C:
			if !s.isActiveStream(registration) {
				return terminate(streamTerminationSuperseded, supersededStreamError())
			}
			if err := s.sendDiscoveryResponse(sender, stream, nodeID, &controlv1.DiscoveryResponse{}, supersededCh); err != nil {
				if isSupersededStreamError(err) {
					return terminate(streamTerminationSuperseded, err)
				}
				disconnectMessage := streamDisconnectMessageForStreamError(err)
				disconnectReason := streamTerminationStreamError
				if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
					disconnectReason = streamTerminationClientDisconnect
					disconnectMessage = streamDisconnectMessageClientDisconnect
				} else {
					switch status.Code(err) {
					case codes.Unavailable:
						disconnectReason = streamTerminationShutdown
						disconnectMessage = streamDisconnectMessageShutdown
					case codes.DeadlineExceeded:
						disconnectReason = streamTerminationSendTimeout
						disconnectMessage = streamDisconnectMessageSendTimeout
					}
				}
				s.disconnectStreamIfActive(stream.Context(), registration, time.Now().UTC(), disconnectReason, disconnectMessage)
				if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
					s.logger.Info("dataplane disconnected", "node_id", nodeID)
					return terminate(streamTerminationClientDisconnect, nil)
				}
				switch status.Code(err) {
				case codes.Unavailable:
					return terminate(streamTerminationShutdown, err)
				case codes.DeadlineExceeded:
					return terminate(streamTerminationSendTimeout, err)
				default:
					return terminate(streamTerminationStreamError, err)
				}
			}
		case snapshot, ok := <-sub:
			if !ok {
				return nil
			}
			if !s.isActiveStream(registration) {
				return terminate(streamTerminationSuperseded, supersededStreamError())
			}
			profile := effectiveProjectionProfile(advertisedFeatures)

			tracer := otel.Tracer("github.com/nantian-gw/gateway/internal/xds")
			_, pushSpan := tracer.Start(stream.Context(), "xds.push_snapshot")
			pushSpan.SetAttributes(
				attribute.String("xds.node_id", nodeID),
				attribute.String("xds.snapshot_id", snapshot.ID),
			)

			variant := s.protoCache.get(stream.Context(), snapshot, profile, s.logger)
			pushSpan.SetAttributes(
				attribute.String("xds.compatibility_profile", variant.GetCompatibilityProfile()),
			)

			response := &controlv1.DiscoveryResponse{
				Version:  snapshot.ID,
				Nonce:    snapshot.ID,
				Snapshot: variant,
			}

			if err := s.sendDiscoveryResponse(sender, stream, nodeID, response, supersededCh); err != nil {
				pushSpan.RecordError(err)
				pushSpan.SetStatus(otelcodes.Error, err.Error())
				pushSpan.End()
				if isSupersededStreamError(err) {
					return terminate(streamTerminationSuperseded, err)
				}
				disconnectMessage := streamDisconnectMessageForStreamError(err)
				disconnectReason := streamTerminationStreamError
				if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
					disconnectReason = streamTerminationClientDisconnect
					disconnectMessage = streamDisconnectMessageClientDisconnect
				} else {
					switch status.Code(err) {
					case codes.Unavailable:
						disconnectReason = streamTerminationShutdown
						disconnectMessage = streamDisconnectMessageShutdown
					case codes.DeadlineExceeded:
						disconnectReason = streamTerminationSendTimeout
						disconnectMessage = streamDisconnectMessageSendTimeout
					}
				}
				s.disconnectStreamIfActive(stream.Context(), registration, time.Now().UTC(), disconnectReason, disconnectMessage)
				if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
					s.logger.Info("dataplane disconnected", "node_id", nodeID)
					return terminate(streamTerminationClientDisconnect, nil)
				}
				switch status.Code(err) {
				case codes.Unavailable:
					return terminate(streamTerminationShutdown, err)
				case codes.DeadlineExceeded:
					return terminate(streamTerminationSendTimeout, err)
				default:
					return terminate(streamTerminationStreamError, err)
				}
			}
			pushSpan.End()
			s.logger.Info(
				"publishing projected snapshot",
				"node_id",
				nodeID,
				"version",
				snapshot.ID,
				"compatibility_profile",
				variant.GetCompatibilityProfile(),
				"required_features",
				variant.GetRequiredFeatures(),
			)
			logListenerSetHTTPRoutingSnapshotShape(s.logger, nodeID, snapshot.ID, variant)
			if !s.isActiveStream(registration) {
				return terminate(streamTerminationSuperseded, supersededStreamError())
			}
			publishedAt := time.Now().UTC()
			s.nodes.ObservePublished(stream.Context(), nodeID, snapshot.ID, publishedAt)
			armAckTimer(snapshot.ID)
		}
	}
}

type initialDiscoveryRequestResult struct {
	req *controlv1.DiscoveryRequest
	err error
}

func (s *Server) recvInitialRequest(
	stream controlv1.ConfigurationDiscoveryService_StreamConfigurationServer,
) (*controlv1.DiscoveryRequest, error) {
	select {
	case <-s.shutdownCh:
		s.logger.Info("draining dataplane stream before initial request for shutdown")
		return nil, shutdownStreamError()
	default:
	}

	resultCh := make(chan initialDiscoveryRequestResult, 1)
	go func() {
		// stream.Recv() blocks until the initial request arrives or the
		// stream context is cancelled. When shutdownCh fires first, the
		// goroutine remains blocked until StreamConfiguration returns and
		// the gRPC framework cancels the stream context, which unblocks
		// Recv() and allows the goroutine to exit.
		req, err := stream.Recv()
		resultCh <- initialDiscoveryRequestResult{req: req, err: err}
	}()

	select {
	case result := <-resultCh:
		return result.req, result.err
	case <-s.shutdownCh:
		s.logger.Info("draining dataplane stream before initial request for shutdown")
		return nil, shutdownStreamError()
	}
}
