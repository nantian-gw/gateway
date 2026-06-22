package xds

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

type discoveryResponseSender struct {
	stream    controlv1.ConfigurationDiscoveryService_StreamConfigurationServer
	requests  chan discoveryResponseSendRequest
	done      chan struct{}
	closeOnce sync.Once
}

type discoveryResponseSendRequest struct {
	response *controlv1.DiscoveryResponse
	resultCh chan error
}

func newDiscoveryResponseSender(
	stream controlv1.ConfigurationDiscoveryService_StreamConfigurationServer,
) *discoveryResponseSender {
	sender := &discoveryResponseSender{
		stream:   stream,
		requests: make(chan discoveryResponseSendRequest, 1),
		done:     make(chan struct{}),
	}

	go sender.run()
	return sender
}

func (s *discoveryResponseSender) send(response *controlv1.DiscoveryResponse) (<-chan error, bool) {
	req := discoveryResponseSendRequest{
		response: response,
		resultCh: make(chan error, 1),
	}

	select {
	case s.requests <- req:
		return req.resultCh, true
	case <-s.done:
		return nil, false
	case <-s.stream.Context().Done():
		return nil, false
	}
}

func (s *discoveryResponseSender) close() {
	s.closeOnce.Do(func() {
		close(s.requests)
	})
}

func (s *discoveryResponseSender) run() {
	defer close(s.done)

	for req := range s.requests {
		req.resultCh <- s.stream.Send(req.response)
		close(req.resultCh)
	}
}

func (s *Server) sendDiscoveryResponse(
	sender *discoveryResponseSender,
	stream controlv1.ConfigurationDiscoveryService_StreamConfigurationServer,
	nodeID string,
	response *controlv1.DiscoveryResponse,
	supersededCh <-chan struct{},
) error {
	started := time.Now()
	snapshotResponse := isSnapshotDiscoveryResponse(response)
	defer func() {
		if snapshotResponse {
			s.observeSnapshotSendDuration(time.Since(started))
		}
	}()

	sendResult, ok := sender.send(response)
	if !ok {
		select {
		case <-stream.Context().Done():
			return context.Canceled
		default:
			return status.Error(codes.Canceled, "xds response sender closed")
		}
	}

	timer := time.NewTimer(s.runtime.snapshotSendTimeout)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case err := <-sendResult:
		return err
	case <-timer.C:
		if snapshotResponse {
			s.recordSnapshotSendTimeout()
		}
		s.logger.Warn(
			"timed out sending snapshot to dataplane; disconnecting slow consumer",
			"node_id",
			nodeID,
			"version",
			response.GetVersion(),
			"timeout",
			s.runtime.snapshotSendTimeout,
		)
		return snapshotSendTimeoutError(s.runtime.snapshotSendTimeout)
	case <-s.shutdownCh:
		s.logger.Info(
			"interrupting dataplane snapshot send for shutdown",
			"node_id",
			nodeID,
			"version",
			response.GetVersion(),
		)
		return shutdownStreamError()
	case <-supersededCh:
		s.logger.Info(
			"interrupting dataplane snapshot send for superseded stream",
			"node_id",
			nodeID,
			"version",
			response.GetVersion(),
		)
		return supersededStreamError()
	case <-stream.Context().Done():
		return context.Canceled
	}
}

func isSnapshotDiscoveryResponse(response *controlv1.DiscoveryResponse) bool {
	return response != nil && (response.GetSnapshot() != nil || response.GetVersion() != "")
}

func shutdownStreamError() error {
	return status.Error(codes.Unavailable, "controlplane shutting down")
}

func supersededStreamError() error {
	return status.Error(codes.Unavailable, "xds stream superseded by newer connection")
}

func isSupersededStreamError(err error) bool {
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.Unavailable && st.Message() == "xds stream superseded by newer connection"
}

func snapshotSendTimeoutError(timeout time.Duration) error {
	return status.Errorf(codes.DeadlineExceeded, "snapshot send timed out after %s", timeout)
}

func snapshotAckTimeoutError(version string, timeout time.Duration) error {
	if version == "" {
		return status.Errorf(codes.DeadlineExceeded, "snapshot ack timed out after %s", timeout)
	}
	return status.Errorf(codes.DeadlineExceeded, "snapshot %s ack timed out after %s", version, timeout)
}
