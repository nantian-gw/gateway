package xds

import (
	"context"
	"fmt"
	"time"
)

const (
	streamDisconnectMessageShutdown         = "xds stream drained for controlplane shutdown"
	streamDisconnectMessageClientDisconnect = "xds stream closed by dataplane"
	streamDisconnectMessageSendTimeout      = "timed out sending snapshot to dataplane"
	streamDisconnectMessageAckTimeout       = "timed out waiting for dataplane snapshot ack"
	streamDisconnectMessageInvalidRequest   = "invalid xds discovery request"
)

func (s *Server) registerStream(nodeID string) *streamRegistration {
	if nodeID == "" {
		return nil
	}

	s.streamsMu.Lock()
	s.nextStreamID++
	registration := &streamRegistration{
		nodeID:     nodeID,
		id:         s.nextStreamID,
		superseded: make(chan struct{}),
	}
	previous := s.activeStreams[nodeID]
	s.activeStreams[nodeID] = registration
	s.streamsMu.Unlock()

	if previous != nil {
		previous.supersede()
	}

	return registration
}

func (s *Server) unregisterStream(registration *streamRegistration) {
	if registration == nil {
		return
	}

	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	current := s.activeStreams[registration.nodeID]
	if current != nil && current.id == registration.id {
		delete(s.activeStreams, registration.nodeID)
	}
}

func (s *Server) isActiveStream(registration *streamRegistration) bool {
	if registration == nil {
		return true
	}

	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	current := s.activeStreams[registration.nodeID]
	return current != nil && current.id == registration.id
}

func (s *Server) disconnectStreamIfActive(
	ctx context.Context,
	registration *streamRegistration,
	now time.Time,
	reason string,
	message string,
) {
	if registration == nil || !s.isActiveStream(registration) {
		return
	}
	s.nodes.DisconnectWithReason(ctx, registration.nodeID, reason, message, now)
}

func streamDisconnectMessageForStreamError(err error) string {
	if err == nil {
		return "xds stream failed"
	}
	return fmt.Sprintf("xds stream failed: %s", err)
}

func registrationSuperseded(registration *streamRegistration) <-chan struct{} {
	if registration == nil {
		return nil
	}
	return registration.superseded
}

func (r *streamRegistration) supersede() {
	if r == nil {
		return
	}
	r.supersedeMu.Do(func() {
		close(r.superseded)
	})
}
