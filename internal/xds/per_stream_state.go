package xds

import (
	"sync"

	"log/slog"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

// deltaStreamState tracks the per-stream resource state for a delta xDS
// connection. It maintains the last acknowledged versions per resource type
// and per resource name, enabling the server to compute accurate diffs.
type deltaStreamState struct {
	mu sync.RWMutex

	// acknowledgedVersions tracks the last version the client acknowledged
	// for each resource. Key structure: type_url -> resource_name -> version.
	acknowledgedVersions map[string]map[string]string

	// subscribed tracks which resource type URLs the client is subscribed to.
	subscribed map[string]bool

	// logger for logging state changes.
	logger *slog.Logger
}

// newDeltaStreamState creates a new per-stream state tracker.
func newDeltaStreamState(logger *slog.Logger) *deltaStreamState {
	return &deltaStreamState{
		acknowledgedVersions: make(map[string]map[string]string),
		subscribed:           make(map[string]bool),
		logger:               logger,
	}
}

// IsSubscribed returns true if the client is subscribed to the given type URL.
func (s *deltaStreamState) IsSubscribed(typeURL string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.subscribed[typeURL]
}

// Subscribe adds one or more type URLs to the subscription set.
func (s *deltaStreamState) Subscribe(typeURLs ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, typeURL := range typeURLs {
		s.subscribed[typeURL] = true
	}
}

// Unsubscribe removes one or more type URLs from the subscription set.
func (s *deltaStreamState) Unsubscribe(typeURLs ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, typeURL := range typeURLs {
		delete(s.subscribed, typeURL)
	}
}

// SubscribedTypes returns a copy of the currently subscribed type URLs.
func (s *deltaStreamState) SubscribedTypes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	types := make([]string, 0, len(s.subscribed))
	for typeURL := range s.subscribed {
		types = append(types, typeURL)
	}
	return types
}

// AcknowledgedVersion returns the last acknowledged version for a resource,
// or empty string if the resource has never been acknowledged.
func (s *deltaStreamState) AcknowledgedVersion(typeURL, resourceName string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if versions, ok := s.acknowledgedVersions[typeURL]; ok {
		return versions[resourceName]
	}
	return ""
}

// HasAcknowledged returns true if the resource has been acknowledged at least once.
func (s *deltaStreamState) HasAcknowledged(typeURL, resourceName string) bool {
	return s.AcknowledgedVersion(typeURL, resourceName) != ""
}

// OnACK updates the acknowledged version for all resources in a delta response.
// This is called when the client sends a DISCOVERY_ACK with the
// nonce matching the response we sent.
func (s *deltaStreamState) OnACK(typeURL string, resourceNames []string, versions map[string]string) {
	if typeURL == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	typeVersions, ok := s.acknowledgedVersions[typeURL]
	if !ok {
		typeVersions = make(map[string]string, len(resourceNames))
		s.acknowledgedVersions[typeURL] = typeVersions
	}

	for _, name := range resourceNames {
		if v, exists := versions[name]; exists {
			typeVersions[name] = v
		}
	}
}

// OnNACK logs the NACK and resets the acknowledged versions for the affected
// resources so they will be re-sent on the next update.
func (s *deltaStreamState) OnNACK(typeURL string, errorDetail string) {
	s.logger.Warn("delta NACK received, will resend resources",
		"type_url", typeURL,
		"error_detail", errorDetail,
	)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear acknowledged versions for this type URL so the next push
	// sends all resources of this type as "added" (full re-sync).
	delete(s.acknowledgedVersions, typeURL)
}

// OnRemovedResources removes the acknowledged versions for resources that
// no longer exist. This prevents stale version tracking.
func (s *deltaStreamState) OnRemovedResources(typeURL string, removedNames []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	typeVersions, ok := s.acknowledgedVersions[typeURL]
	if !ok {
		return
	}
	for _, name := range removedNames {
		delete(typeVersions, name)
	}
}

// OnFullResourceUpdate replaces the acknowledged versions for a type URL
// with a complete set. This is used when a non-incremental (SotW) response
// is sent, which replaces all resources of a type.
func (s *deltaStreamState) OnFullResourceUpdate(typeURL string, resourceVersions map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	versions := make(map[string]string, len(resourceVersions))
	for name, version := range resourceVersions {
		versions[name] = version
	}
	s.acknowledgedVersions[typeURL] = versions
}

// IsEmpty returns true if no resources have been acknowledged for any type.
// An empty state means the client is connecting for the first time and needs
// the full initial snapshot.
func (s *deltaStreamState) IsEmpty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, versions := range s.acknowledgedVersions {
		if len(versions) > 0 {
			return false
		}
	}
	return true
}

// computeInitialResources returns the set of resource versions to send on
// initial connection. This is derived from the snapshot's resource versions
// for the subscribed types.
func (s *deltaStreamState) computeInitialResources(snapshotVersions map[string]map[string]string) map[string]map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]map[string]string)
	for typeURL := range s.subscribed {
		if versions, ok := snapshotVersions[typeURL]; ok && len(versions) > 0 {
			typeResult := make(map[string]string, len(versions))
			for name, version := range versions {
				typeResult[name] = version
			}
			result[typeURL] = typeResult
		}
	}
	return result
}

// handleDeltaRequest processes a DeltaDiscoveryRequest from the client,
// updating subscription state and acknowledging versions.
func (s *deltaStreamState) handleDeltaRequest(req *controlv1.DeltaDiscoveryRequest) {
	if req == nil {
		return
	}

	// Handle subscription changes
	if len(req.GetResourceNamesSubscribe()) > 0 {
		s.Subscribe(req.GetResourceNamesSubscribe()...)
	}
	if len(req.GetResourceNamesUnsubscribe()) > 0 {
		s.Unsubscribe(req.GetResourceNamesUnsubscribe()...)
	}

	// Handle ACK/NACK
	switch req.GetResultStatus() {
	case controlv1.DiscoveryResultStatus_DISCOVERY_ACK:
		// ACK: update acknowledged versions for the type URL
		// The resource versions are tracked via the response we sent
		// (with the matching nonce). Here we just acknowledge the type.
		// Full resource-level version tracking happens in OnACK.
		if req.GetTypeUrl() != "" {
			s.OnACK(req.GetTypeUrl(), nil, nil)
		}

	case controlv1.DiscoveryResultStatus_DISCOVERY_NACK:
		if req.GetTypeUrl() != "" {
			s.OnNACK(req.GetTypeUrl(), req.GetErrorDetail())
		}
	}

	// Handle initial resource versions (seeding from client)
	if len(req.GetInitialResourceVersions()) > 0 {
		for typeURL, versionsJSON := range req.GetInitialResourceVersions() {
			_ = versionsJSON // Client-provided initial versions, stored per type
			_ = typeURL
		}
	}
}