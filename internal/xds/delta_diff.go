package xds

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/nantian-gw/gateway/internal/ir"
)

// ResourceVersion maps a fully-qualified resource name to its content version.
func ResourceVersion(r interface{}) string {
	raw, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:16])
}

// SnapshotVersions extracts per-resource type version maps from a snapshot.
// Keys are the gRPC type URLs used in xDS (matching proto message names).
func SnapshotVersions(snap *ir.Snapshot) map[string]map[string]string {
	versions := make(map[string]map[string]string)

	versions[typeURLListener] = resourceNameVersionMap(snap.Listeners, func(l ir.Listener) string {
		return fmt.Sprintf("%s/%d", l.Name, l.Port)
	})

	versions[typeURLHTTPRoute] = resourceNameVersionMap(snap.HTTPRoutes, func(r ir.HTTPRoute) string {
		return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	})

	versions[typeURLGRPCRoute] = resourceNameVersionMap(snap.GRPCRoutes, func(r ir.GRPCRoute) string {
		return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	})

	versions[typeURLStreamRoute] = resourceNameVersionMap(snap.StreamRoutes, func(r ir.StreamRoute) string {
		return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	})

	versions[typeURLBackend] = resourceNameVersionMap(snap.Backends, func(b ir.BackendCluster) string {
		return b.Name
	})

	versions[typeURLSecret] = resourceNameVersionMap(snap.Secrets, func(s ir.SecretMaterial) string {
		return s.Name
	})

	return versions
}

func resourceNameVersionMap[T any](items []T, nameFn func(T) string) map[string]string {
	m := make(map[string]string, len(items))
	for _, item := range items {
		m[nameFn(item)] = ResourceVersion(item)
	}
	return m
}

// xDS type URLs matching proto message types for delta subscriptions.
const (
	typeURLListener   = "type.googleapis.com/gateway.control.v1.Listener"
	typeURLHTTPRoute  = "type.googleapis.com/gateway.control.v1.HttpRoute"
	typeURLGRPCRoute  = "type.googleapis.com/gateway.control.v1.GrpcRoute"
	typeURLStreamRoute = "type.googleapis.com/gateway.control.v1.StreamRoute"
	typeURLBackend    = "type.googleapis.com/gateway.control.v1.BackendCluster"
	typeURLSecret     = "type.googleapis.com/gateway.control.v1.SecretMaterial"
)

// DeltaDiff computes the delta between two IR snapshots for a given resource
// type. Returns lists of resource names for added/changed and removed resources.
//
// addedChanged contains names of resources that are new or whose content hash
// differs from the previous version. removed contains names present in old but
// absent in new.
func DeltaDiff[T any](
	oldItems []T,
	newItems []T,
	nameFn func(T) string,
	oldVersionFn func(T) string,
) (addedChanged []string, removed []string) {
	oldMap := make(map[string]bool, len(oldItems))
	newMap := make(map[string]bool, len(newItems))
	newVersion := make(map[string]string, len(newItems))

	for _, item := range newItems {
		name := nameFn(item)
		newMap[name] = true
		newVersion[name] = oldVersionFn(item)
	}

	// Find removed: in old but not in new
	for _, item := range oldItems {
		name := nameFn(item)
		oldMap[name] = true
		if !newMap[name] {
			removed = append(removed, name)
		}
	}

	// Find added/changed: in new, either not in old or version differs
	for _, item := range newItems {
		name := nameFn(item)
		if !oldMap[name] {
			addedChanged = append(addedChanged, name)
		} else {
			// In both maps: check version
			oldVersion := ResourceVersion(findByName(oldItems, nameFn, name))
			if oldVersion != newVersion[name] {
				addedChanged = append(addedChanged, name)
			}
		}
	}

	sort.Strings(addedChanged)
	sort.Strings(removed)
	return
}

func findByName[T any](items []T, nameFn func(T) string, target string) (result T) {
	for _, item := range items {
		if nameFn(item) == target {
			return item
		}
	}
	return
}

// SnapshotDelta computes a complete delta across all resource types between
// old and new snapshots. If old is nil, all resources in new are "added".
func SnapshotDelta(old, new *ir.Snapshot) DeltaResult {
	if old == nil {
		return fullSnapshotDelta(new)
	}

	return DeltaResult{
		Listeners:    typeDelta(old.Listeners, new.Listeners, listenerName),
		HTTPRoutes:   typeDelta(old.HTTPRoutes, new.HTTPRoutes, httpRouteName),
		GRPCRoutes:   typeDelta(old.GRPCRoutes, new.GRPCRoutes, grpcRouteName),
		StreamRoutes: typeDelta(old.StreamRoutes, new.StreamRoutes, streamRouteName),
		Backends:     typeDelta(old.Backends, new.Backends, backendName),
		Secrets:      typeDelta(old.Secrets, new.Secrets, secretName),
	}
}

// DeltaResult holds per-resource-type delta information.
type DeltaResult struct {
	Listeners    ResourceDelta
	HTTPRoutes   ResourceDelta
	GRPCRoutes   ResourceDelta
	StreamRoutes ResourceDelta
	Backends     ResourceDelta
	Secrets      ResourceDelta
}

// ResourceDelta tracks which resources were added/changed or removed.
type ResourceDelta struct {
	AddedChanged []string
	Removed      []string
}

// IsEmpty reports whether this delta has no changes.
func (d ResourceDelta) IsEmpty() bool {
	return len(d.AddedChanged) == 0 && len(d.Removed) == 0
}

// TotalChanges reports the total number of changed resources.
func (d ResourceDelta) TotalChanges() int {
	return len(d.AddedChanged) + len(d.Removed)
}

func (d ResourceDelta) HasNonIncremental(oldCount int) bool {
	if oldCount == 0 {
		return false
	}
	return float64(d.TotalChanges())/float64(oldCount) > 0.5
}

func fullSnapshotDelta(snap *ir.Snapshot) DeltaResult {
	return DeltaResult{
		Listeners:    fullTypeDelta(snap.Listeners, listenerName),
		HTTPRoutes:   fullTypeDelta(snap.HTTPRoutes, httpRouteName),
		GRPCRoutes:   fullTypeDelta(snap.GRPCRoutes, grpcRouteName),
		StreamRoutes: fullTypeDelta(snap.StreamRoutes, streamRouteName),
		Backends:     fullTypeDelta(snap.Backends, backendName),
		Secrets:      fullTypeDelta(snap.Secrets, secretName),
	}
}

func fullTypeDelta[T any](items []T, nameFn func(T) string) ResourceDelta {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = nameFn(item)
	}
	return ResourceDelta{AddedChanged: names}
}

func typeDelta[T any](old, new []T, nameFn func(T) string) ResourceDelta {
	added, removed := DeltaDiff(old, new, nameFn, func(item T) string {
		return ResourceVersion(item)
	})
	return ResourceDelta{AddedChanged: added, Removed: removed}
}

func listenerName(l ir.Listener) string      { return fmt.Sprintf("%s/%d", l.Name, l.Port) }
func httpRouteName(r ir.HTTPRoute) string    { return fmt.Sprintf("%s/%s", r.Namespace, r.Name) }
func grpcRouteName(r ir.GRPCRoute) string    { return fmt.Sprintf("%s/%s", r.Namespace, r.Name) }
func streamRouteName(r ir.StreamRoute) string { return fmt.Sprintf("%s/%s", r.Namespace, r.Name) }
func backendName(b ir.BackendCluster) string  { return b.Name }
func secretName(s ir.SecretMaterial) string   { return s.Name }

func newNonce() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return "", err
	}
	return n.String(), nil
}

func (dr *DeltaResult) deltaForType(typeURL string) *ResourceDelta {
	switch typeURL {
	case typeURLListener:
		return &dr.Listeners
	case typeURLHTTPRoute:
		return &dr.HTTPRoutes
	case typeURLGRPCRoute:
		return &dr.GRPCRoutes
	case typeURLStreamRoute:
		return &dr.StreamRoutes
	case typeURLBackend:
		return &dr.Backends
	case typeURLSecret:
		return &dr.Secrets
	default:
		return nil
	}
}

func (dr *DeltaResult) oldCountForType(typeURL string, snapshot *ir.Snapshot) int {
	return typeResourceCount(typeURL, snapshot)
}

func typeResourceCount(typeURL string, snapshot *ir.Snapshot) int {
	if snapshot == nil {
		return 0
	}
	switch typeURL {
	case typeURLListener:
		return len(snapshot.Listeners)
	case typeURLHTTPRoute:
		return len(snapshot.HTTPRoutes)
	case typeURLGRPCRoute:
		return len(snapshot.GRPCRoutes)
	case typeURLStreamRoute:
		return len(snapshot.StreamRoutes)
	case typeURLBackend:
		return len(snapshot.Backends)
	case typeURLSecret:
		return len(snapshot.Secrets)
	default:
		return 0
	}
}
