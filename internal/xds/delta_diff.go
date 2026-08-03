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

func ResourceVersion(r interface{}) string {
	raw, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:16])
}

func SnapshotVersions(snap *ir.Snapshot) map[string]map[string]string {
	versions := make(map[string]map[string]string)

	versions[typeURLListener] = resourceNameVersionMap(snap.Listeners, func(l *ir.Listener) string {
		return fmt.Sprintf("%s/%d", l.Name, l.Port)
	})

	versions[typeURLHTTPRoute] = resourceNameVersionMap(snap.HTTPRoutes, func(r *ir.HTTPRoute) string {
		return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	})

	versions[typeURLGRPCRoute] = resourceNameVersionMap(snap.GRPCRoutes, func(r *ir.GRPCRoute) string {
		return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	})

	versions[typeURLStreamRoute] = resourceNameVersionMap(snap.StreamRoutes, func(r *ir.StreamRoute) string {
		return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	})

	versions[typeURLBackend] = resourceNameVersionMap(snap.Backends, func(b *ir.BackendCluster) string {
		return b.Name
	})

	versions[typeURLSecret] = resourceNameVersionMap(snap.Secrets, func(s *ir.SecretMaterial) string {
		return s.Name
	})

	return versions
}

func resourceNameVersionMap[T any](items []T, nameFn func(*T) string) map[string]string {
	m := make(map[string]string, len(items))
	for i := range items {
		m[nameFn(&items[i])] = ResourceVersion(&items[i])
	}
	return m
}

const (
	typeURLListener    = "type.googleapis.com/gateway.control.v1.Listener"
	typeURLHTTPRoute   = "type.googleapis.com/gateway.control.v1.HttpRoute"
	typeURLGRPCRoute   = "type.googleapis.com/gateway.control.v1.GrpcRoute"
	typeURLStreamRoute = "type.googleapis.com/gateway.control.v1.StreamRoute"
	typeURLBackend     = "type.googleapis.com/gateway.control.v1.BackendCluster"
	typeURLSecret      = "type.googleapis.com/gateway.control.v1.SecretMaterial"
)

func DeltaDiff[T any](
	oldItems, newItems []T,
	nameFn func(*T) string,
	oldVersionFn func(*T) string,
) (addedChanged, removed []string) {
	oldMap := make(map[string]bool, len(oldItems))
	newMap := make(map[string]bool, len(newItems))
	newVersion := make(map[string]string, len(newItems))

	for i := range newItems {
		name := nameFn(&newItems[i])
		newMap[name] = true
		newVersion[name] = oldVersionFn(&newItems[i])
	}

	for i := range oldItems {
		name := nameFn(&oldItems[i])
		oldMap[name] = true
		if !newMap[name] {
			removed = append(removed, name)
		}
	}

	for i := range newItems {
		name := nameFn(&newItems[i])
		if !oldMap[name] {
			addedChanged = append(addedChanged, name)
		} else {
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

func findByName[T any](items []T, nameFn func(*T) string, target string) (result T) {
	for i := range items {
		if nameFn(&items[i]) == target {
			return items[i]
		}
	}
	return
}

func SnapshotDelta(prev, curr *ir.Snapshot) DeltaResult {
	if prev == nil {
		return fullSnapshotDelta(curr)
	}

	return DeltaResult{
		Listeners:    typeDelta(prev.Listeners, curr.Listeners, listenerNameFn),
		HTTPRoutes:   typeDelta(prev.HTTPRoutes, curr.HTTPRoutes, httpRouteNameFn),
		GRPCRoutes:   typeDelta(prev.GRPCRoutes, curr.GRPCRoutes, grpcRouteNameFn),
		StreamRoutes: typeDelta(prev.StreamRoutes, curr.StreamRoutes, streamRouteNameFn),
		Backends:     typeDelta(prev.Backends, curr.Backends, backendNameFn),
		Secrets:      typeDelta(prev.Secrets, curr.Secrets, secretNameFn),
	}
}

type DeltaResult struct {
	Listeners    ResourceDelta
	HTTPRoutes   ResourceDelta
	GRPCRoutes   ResourceDelta
	StreamRoutes ResourceDelta
	Backends     ResourceDelta
	Secrets      ResourceDelta
}

type ResourceDelta struct {
	AddedChanged []string
	Removed      []string
}

func (d ResourceDelta) IsEmpty() bool {
	return len(d.AddedChanged) == 0 && len(d.Removed) == 0
}

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
		Listeners:    fullTypeDelta(snap.Listeners, listenerNameFn),
		HTTPRoutes:   fullTypeDelta(snap.HTTPRoutes, httpRouteNameFn),
		GRPCRoutes:   fullTypeDelta(snap.GRPCRoutes, grpcRouteNameFn),
		StreamRoutes: fullTypeDelta(snap.StreamRoutes, streamRouteNameFn),
		Backends:     fullTypeDelta(snap.Backends, backendNameFn),
		Secrets:      fullTypeDelta(snap.Secrets, secretNameFn),
	}
}

func fullTypeDelta[T any](items []T, nameFn func(*T) string) ResourceDelta {
	names := make([]string, len(items))
	for i := range items {
		names[i] = nameFn(&items[i])
	}
	return ResourceDelta{AddedChanged: names}
}

func typeDelta[T any](prev, curr []T, nameFn func(*T) string) ResourceDelta {
	added, removed := DeltaDiff(prev, curr, nameFn, func(item *T) string {
		return ResourceVersion(item)
	})
	return ResourceDelta{AddedChanged: added, Removed: removed}
}

func listenerNameFn(l *ir.Listener) string       { return fmt.Sprintf("%s/%d", l.Name, l.Port) }
func httpRouteNameFn(r *ir.HTTPRoute) string     { return fmt.Sprintf("%s/%s", r.Namespace, r.Name) }
func grpcRouteNameFn(r *ir.GRPCRoute) string     { return fmt.Sprintf("%s/%s", r.Namespace, r.Name) }
func streamRouteNameFn(r *ir.StreamRoute) string { return fmt.Sprintf("%s/%s", r.Namespace, r.Name) }
func backendNameFn(b *ir.BackendCluster) string  { return b.Name }
func secretNameFn(s *ir.SecretMaterial) string   { return s.Name }

func newNonce() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return n.String(), nil
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
