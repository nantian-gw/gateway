package admin

import (
	"sync"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

type snapshotDetailIndexCache struct {
	mu      sync.RWMutex
	current *snapshotDetailIndex
}

type snapshotDetailIndex struct {
	snapshot  *ir.Snapshot
	listeners map[string]ir.Listener
	backends  map[detailBackendKey]ir.BackendCluster
	routes    map[detailRouteKey]any
}

type detailBackendKey struct {
	namespace string
	name      string
}

type detailRouteKey struct {
	kind      string
	namespace string
	name      string
}

func newSnapshotDetailIndexCache() *snapshotDetailIndexCache {
	return &snapshotDetailIndexCache{}
}

func (c *snapshotDetailIndexCache) get(snapshot *ir.Snapshot) *snapshotDetailIndex {
	if snapshot == nil {
		return nil
	}

	c.mu.RLock()
	current := c.current
	c.mu.RUnlock()
	if current != nil && current.snapshot == snapshot {
		return current
	}

	built := buildSnapshotDetailIndex(snapshot)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil && c.current.snapshot == snapshot {
		return c.current
	}
	c.current = built
	return built
}

func buildSnapshotDetailIndex(snapshot *ir.Snapshot) *snapshotDetailIndex {
	if snapshot == nil {
		return nil
	}

	index := &snapshotDetailIndex{
		snapshot:  snapshot,
		listeners: make(map[string]ir.Listener, len(snapshot.Listeners)),
		backends:  make(map[detailBackendKey]ir.BackendCluster),
		routes:    make(map[detailRouteKey]any, len(snapshot.HTTPRoutes)+len(snapshot.GRPCRoutes)+len(snapshot.StreamRoutes)),
	}

	for _, listener := range displayListeners(snapshot.Listeners) {
		if listener.Name == "" {
			continue
		}
		index.listeners[listener.Name] = listener
	}

	for _, backend := range visibleBackends(snapshot, false) {
		if backend.Namespace == "" || backend.Name == "" {
			continue
		}
		index.backends[detailBackendKey{
			namespace: backend.Namespace,
			name:      backend.Name,
		}] = backend
	}

	for _, route := range snapshot.HTTPRoutes {
		if route.Namespace == "" || route.Name == "" {
			continue
		}
		index.routes[detailRouteKey{
			kind:      "HTTP",
			namespace: route.Namespace,
			name:      route.Name,
		}] = route
	}

	for _, route := range snapshot.GRPCRoutes {
		if route.Namespace == "" || route.Name == "" {
			continue
		}
		index.routes[detailRouteKey{
			kind:      "GRPC",
			namespace: route.Namespace,
			name:      route.Name,
		}] = route
	}

	for _, route := range snapshot.StreamRoutes {
		if route.Namespace == "" || route.Name == "" {
			continue
		}
		kind := canonicalRouteKind(route.Kind)
		if kind == "" {
			continue
		}
		index.routes[detailRouteKey{
			kind:      kind,
			namespace: route.Namespace,
			name:      route.Name,
		}] = route
	}

	return index
}

func (i *snapshotDetailIndex) listener(name string) (ir.Listener, bool) {
	if i == nil {
		return ir.Listener{}, false
	}

	item, ok := i.listeners[name]
	return item, ok
}

func (i *snapshotDetailIndex) backend(namespace, name string) (ir.BackendCluster, bool) {
	if i == nil {
		return ir.BackendCluster{}, false
	}

	item, ok := i.backends[detailBackendKey{
		namespace: namespace,
		name:      name,
	}]
	return item, ok
}

func (i *snapshotDetailIndex) route(kind, namespace, name string) (any, bool, error) {
	if i == nil {
		return nil, false, nil
	}

	canonicalKind, err := parseRequiredRouteKind(kind)
	if err != nil {
		return nil, false, err
	}

	item, ok := i.routes[detailRouteKey{
		kind:      canonicalKind,
		namespace: namespace,
		name:      name,
	}]
	return item, ok, nil
}

func (s *Server) snapshotDetailIndex(snapshot *ir.Snapshot) *snapshotDetailIndex {
	if s == nil {
		return nil
	}
	return s.detailIndex.get(snapshot)
}
