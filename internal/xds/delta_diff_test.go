package xds

import (
	"testing"

	"github.com/nantian-gw/gateway/internal/ir"
)

// ---- helpers ----

func makeListener(name string, port uint32) ir.Listener {
	return ir.Listener{Name: name, Port: port}
}

func makeHTTPRoute(name string) ir.HTTPRoute {
	return ir.HTTPRoute{Namespace: "default", Name: name}
}

func makeGRPCRoute(name string) ir.GRPCRoute {
	return ir.GRPCRoute{Namespace: "default", Name: name}
}

func makeStreamRoute(name string) ir.StreamRoute {
	return ir.StreamRoute{Namespace: "default", Name: name}
}

func makeBackend(name string) ir.BackendCluster {
	return ir.BackendCluster{Name: name}
}

func makeSecret(name string) ir.SecretMaterial {
	return ir.SecretMaterial{Name: name}
}

// ---- ResourceVersion ----

func TestResourceVersion_sameContent(t *testing.T) {
	ln := makeListener("http", 80)
	v1 := ResourceVersion(&ln)
	v2 := ResourceVersion(&ln)
	if v1 != v2 {
		t.Fatalf("same content should produce identical versions, got %q vs %q", v1, v2)
	}
	if v1 == "" {
		t.Fatal("version should not be empty")
	}
}

func TestResourceVersion_differentContent(t *testing.T) {
	a := makeListener("http", 80)
	b := makeListener("https", 443)
	va := ResourceVersion(&a)
	vb := ResourceVersion(&b)
	if va == vb {
		t.Fatalf("different content should produce different versions, both are %q", va)
	}
}

func TestResourceVersion_differentPort(t *testing.T) {
	a := makeListener("http", 80)
	b := makeListener("http", 8080)
	va := ResourceVersion(&a)
	vb := ResourceVersion(&b)
	if va == vb {
		t.Fatalf("different port should produce different versions, both are %q", va)
	}
}

func TestResourceVersion_nilPointer(t *testing.T) {
	v := ResourceVersion(nil)
	if v == "" {
		t.Fatal("ResourceVersion(nil) should not return empty string; JSON of nil is \"null\"")
	}
}

func TestResourceVersion_hexFormat(t *testing.T) {
	ln := makeListener("http", 80)
	v := ResourceVersion(&ln)
	if len(v) != 32 {
		t.Fatalf("expected 32 hex chars (16 bytes), got %d: %q", len(v), v)
	}
	for _, c := range v {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("version contains non-hex character: %q", v)
		}
	}
}

// ---- newNonce ----

func TestNewNonce_nonEmpty(t *testing.T) {
	n, err := newNonce()
	if err != nil {
		t.Fatalf("newNonce returned error: %v", err)
	}
	if n == "" {
		t.Fatal("nonce should not be empty")
	}
}

func TestNewNonce_unique(t *testing.T) {
	const count = 20
	seen := make(map[string]bool, count)
	for range count {
		n, err := newNonce()
		if err != nil {
			t.Fatalf("newNonce returned error: %v", err)
		}
		if seen[n] {
			t.Fatalf("duplicate nonce generated: %q", n)
		}
		seen[n] = true
	}
}

// ---- ResourceDelta ----

func TestResourceDelta_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		rd   ResourceDelta
		want bool
	}{
		{"zero value", ResourceDelta{}, true},
		{"only added", ResourceDelta{AddedChanged: []string{"a"}}, false},
		{"only removed", ResourceDelta{Removed: []string{"b"}}, false},
		{"both", ResourceDelta{AddedChanged: []string{"a"}, Removed: []string{"b"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rd.IsEmpty(); got != tt.want {
				t.Fatalf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResourceDelta_TotalChanges(t *testing.T) {
	rd := ResourceDelta{AddedChanged: []string{"a", "b"}, Removed: []string{"c"}}
	if got := rd.TotalChanges(); got != 3 {
		t.Fatalf("TotalChanges() = %d, want 3", got)
	}
}

func TestResourceDelta_HasNonIncremental(t *testing.T) {
	tests := []struct {
		name     string
		delta    ResourceDelta
		oldCount int
		want     bool
	}{
		{"zero old count", ResourceDelta{AddedChanged: []string{"a", "b"}}, 0, false},
		{"single change, 1 old", ResourceDelta{Removed: []string{"a"}}, 1, true},
		{"no changes", ResourceDelta{}, 10, false},
		{"exactly half", ResourceDelta{AddedChanged: []string{"a", "b", "c"}, Removed: []string{"d", "e"}}, 10, false},
		{"just over half", ResourceDelta{AddedChanged: []string{"a", "b", "c"}, Removed: []string{"d", "e", "f"}}, 10, true},
		{"all changed", ResourceDelta{AddedChanged: []string{"a", "b", "c"}, Removed: []string{"d", "e", "f"}}, 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.delta.HasNonIncremental(tt.oldCount); got != tt.want {
				t.Fatalf("HasNonIncremental(%d) = %v, want %v", tt.oldCount, got, tt.want)
			}
		})
	}
}

// ---- DeltaDiff ----

func TestDeltaDiff_addOnly(t *testing.T) {
	old := []int{}
	new := []int{1, 2, 3}
	nameFn := func(i *int) string {
		return string(rune('0' + *i))
	}
	verFn := func(i *int) string { return ResourceVersion(i) }
	added, removed := DeltaDiff(old, new, nameFn, verFn)
	if len(removed) != 0 {
		t.Fatalf("expected no removed, got %v", removed)
	}
	if len(added) != 3 {
		t.Fatalf("expected 3 added/changed, got %v", added)
	}
}

func TestDeltaDiff_removeOnly(t *testing.T) {
	old := []int{1, 2, 3}
	new := []int{}
	nameFn := func(i *int) string { return string(rune('0' + *i)) }
	verFn := func(i *int) string { return ResourceVersion(i) }
	added, removed := DeltaDiff(old, new, nameFn, verFn)
	if len(added) != 0 {
		t.Fatalf("expected no added, got %v", added)
	}
	if len(removed) != 3 {
		t.Fatalf("expected 3 removed, got %v", removed)
	}
}

func TestDeltaDiff_noChange(t *testing.T) {
	a := makeListener("http", 80)
	b := makeListener("https", 443)
	old := []ir.Listener{a, b}
	new := []ir.Listener{a, b}
	added, removed := DeltaDiff(old, new, listenerNameFn, func(l *ir.Listener) string {
		return ResourceVersion(l)
	})
	if len(added) != 0 {
		t.Fatalf("expected no added/changed, got %v", added)
	}
	if len(removed) != 0 {
		t.Fatalf("expected no removed, got %v", removed)
	}
}

func TestDeltaDiff_modify(t *testing.T) {
	a := makeListener("http", 80)
	aMod := makeListener("http", 80)
	aMod.Hostnames = []string{"example.com"}
	b := makeListener("https", 443)
	old := []ir.Listener{a, b}
	new := []ir.Listener{aMod, b}
	added, removed := DeltaDiff(old, new, listenerNameFn, func(l *ir.Listener) string {
		return ResourceVersion(l)
	})
	if len(removed) != 0 {
		t.Fatalf("expected no removed, got %v", removed)
	}
	if len(added) != 1 {
		t.Fatalf("expected 1 added/changed (modified listener), got %v", added)
	}
	if added[0] != listenerNameFn(&a) {
		t.Fatalf("expected modified listener %q, got %q", listenerNameFn(&a), added[0])
	}
}

func TestDeltaDiff_mixed(t *testing.T) {
	a := makeHTTPRoute("a")
	b := makeHTTPRoute("b")
	cModified := makeHTTPRoute("c")
	cModified.Hostnames = []string{"added.example.com"}
	cOriginal := makeHTTPRoute("c")
	d := makeHTTPRoute("d")
	e := makeHTTPRoute("e")

	old := []ir.HTTPRoute{a, b, cOriginal, d}
	new := []ir.HTTPRoute{b, cModified, e}
	added, removed := DeltaDiff(old, new, httpRouteNameFn, func(r *ir.HTTPRoute) string {
		return ResourceVersion(r)
	})

	if len(removed) != 2 {
		t.Fatalf("expected 2 removed (a, d), got %v", removed)
	}
	if len(added) != 2 {
		t.Fatalf("expected 2 added/changed, got %v", added)
	}
	hasE := false
	hasC := false
	for _, name := range added {
		if name == "default/e" {
			hasE = true
		}
		if name == "default/c" {
			hasC = true
		}
	}
	if !hasE {
		t.Fatal("expected 'default/e' in added/changed")
	}
	if !hasC {
		t.Fatal("expected 'default/c' in added/changed")
	}
}

func TestDeltaDiff_sortedOutput(t *testing.T) {
	c := makeBackend("c-cluster")
	a := makeBackend("a-cluster")
	b := makeBackend("b-cluster")
	old := []ir.BackendCluster{}
	new := []ir.BackendCluster{c, a, b}
	added, _ := DeltaDiff(old, new, backendNameFn, func(b *ir.BackendCluster) string {
		return ResourceVersion(b)
	})
	if len(added) != 3 {
		t.Fatalf("expected 3 added, got %v", added)
	}
	if added[0] != "a-cluster" || added[1] != "b-cluster" || added[2] != "c-cluster" {
		t.Fatalf("expected sorted output [a-cluster, b-cluster, c-cluster], got %v", added)
	}
}

// ---- SnapshotDelta ----

func TestSnapshotDelta_nilPrev_AllAdded(t *testing.T) {
	curr := &ir.Snapshot{
		Listeners:    []ir.Listener{makeListener("http", 80), makeListener("https", 443)},
		HTTPRoutes:   []ir.HTTPRoute{makeHTTPRoute("route1")},
		GRPCRoutes:   []ir.GRPCRoute{makeGRPCRoute("grpc1")},
		StreamRoutes: []ir.StreamRoute{makeStreamRoute("stream1")},
		Backends:     []ir.BackendCluster{makeBackend("backend1")},
		Secrets:      []ir.SecretMaterial{makeSecret("secret1")},
	}
	result := SnapshotDelta(nil, curr)

	if len(result.Listeners.AddedChanged) != 2 {
		t.Fatalf("expected 2 listeners added, got %d", len(result.Listeners.AddedChanged))
	}
	if len(result.Listeners.Removed) != 0 {
		t.Fatalf("expected 0 listeners removed, got %d", len(result.Listeners.Removed))
	}
	if len(result.HTTPRoutes.AddedChanged) != 1 {
		t.Fatalf("expected 1 HTTP route added, got %d", len(result.HTTPRoutes.AddedChanged))
	}
	if len(result.GRPCRoutes.AddedChanged) != 1 {
		t.Fatalf("expected 1 gRPC route added, got %d", len(result.GRPCRoutes.AddedChanged))
	}
	if len(result.StreamRoutes.AddedChanged) != 1 {
		t.Fatalf("expected 1 stream route added, got %d", len(result.StreamRoutes.AddedChanged))
	}
	if len(result.Backends.AddedChanged) != 1 {
		t.Fatalf("expected 1 backend added, got %d", len(result.Backends.AddedChanged))
	}
	if len(result.Secrets.AddedChanged) != 1 {
		t.Fatalf("expected 1 secret added, got %d", len(result.Secrets.AddedChanged))
	}
}

func TestSnapshotDelta_prevHasData_currEmpty_AllRemoved(t *testing.T) {
	prev := &ir.Snapshot{
		Listeners:    []ir.Listener{makeListener("http", 80)},
		HTTPRoutes:   []ir.HTTPRoute{makeHTTPRoute("route1")},
		GRPCRoutes:   []ir.GRPCRoute{makeGRPCRoute("grpc1")},
		StreamRoutes: []ir.StreamRoute{makeStreamRoute("stream1")},
		Backends:     []ir.BackendCluster{makeBackend("backend1")},
		Secrets:      []ir.SecretMaterial{makeSecret("secret1")},
	}
	curr := &ir.Snapshot{}

	result := SnapshotDelta(prev, curr)

	if len(result.Listeners.Removed) != 1 {
		t.Fatalf("expected 1 listener removed, got %d", len(result.Listeners.Removed))
	}
	if len(result.Listeners.AddedChanged) != 0 {
		t.Fatalf("expected 0 listeners added, got %d", len(result.Listeners.AddedChanged))
	}
	if len(result.HTTPRoutes.Removed) != 1 {
		t.Fatalf("expected 1 HTTP route removed, got %d", len(result.HTTPRoutes.Removed))
	}
	if len(result.GRPCRoutes.Removed) != 1 {
		t.Fatalf("expected 1 gRPC route removed, got %d", len(result.GRPCRoutes.Removed))
	}
	if len(result.StreamRoutes.Removed) != 1 {
		t.Fatalf("expected 1 stream route removed, got %d", len(result.StreamRoutes.Removed))
	}
	if len(result.Backends.Removed) != 1 {
		t.Fatalf("expected 1 backend removed, got %d", len(result.Backends.Removed))
	}
	if len(result.Secrets.Removed) != 1 {
		t.Fatalf("expected 1 secret removed, got %d", len(result.Secrets.Removed))
	}
}

func TestSnapshotDelta_noChanges(t *testing.T) {
	listeners := []ir.Listener{makeListener("http", 80), makeListener("https", 443)}
	routes := []ir.HTTPRoute{makeHTTPRoute("route1"), makeHTTPRoute("route2")}
	backends := []ir.BackendCluster{makeBackend("backend1")}

	prev := &ir.Snapshot{Listeners: listeners, HTTPRoutes: routes, Backends: backends}
	curr := &ir.Snapshot{Listeners: listeners, HTTPRoutes: routes, Backends: backends}

	result := SnapshotDelta(prev, curr)

	if !result.Listeners.IsEmpty() {
		t.Fatalf("listeners delta should be empty, got added=%v removed=%v",
			result.Listeners.AddedChanged, result.Listeners.Removed)
	}
	if !result.HTTPRoutes.IsEmpty() {
		t.Fatalf("HTTP routes delta should be empty, got added=%v removed=%v",
			result.HTTPRoutes.AddedChanged, result.HTTPRoutes.Removed)
	}
	if !result.Backends.IsEmpty() {
		t.Fatalf("backends delta should be empty, got added=%v removed=%v",
			result.Backends.AddedChanged, result.Backends.Removed)
	}
}

func TestSnapshotDelta_singleResourceAdded(t *testing.T) {
	a := makeHTTPRoute("a")
	b := makeHTTPRoute("b")
	prev := &ir.Snapshot{HTTPRoutes: []ir.HTTPRoute{a}}
	curr := &ir.Snapshot{HTTPRoutes: []ir.HTTPRoute{a, b}}

	result := SnapshotDelta(prev, curr)

	if len(result.HTTPRoutes.AddedChanged) != 1 {
		t.Fatalf("expected 1 added, got %v", result.HTTPRoutes.AddedChanged)
	}
	if result.HTTPRoutes.AddedChanged[0] != "default/b" {
		t.Fatalf("expected 'default/b' added, got %q", result.HTTPRoutes.AddedChanged[0])
	}
	if len(result.HTTPRoutes.Removed) != 0 {
		t.Fatalf("expected 0 removed, got %v", result.HTTPRoutes.Removed)
	}
}

func TestSnapshotDelta_singleResourceRemoved(t *testing.T) {
	a := makeBackend("a")
	b := makeBackend("b")
	prev := &ir.Snapshot{Backends: []ir.BackendCluster{a, b}}
	curr := &ir.Snapshot{Backends: []ir.BackendCluster{a}}

	result := SnapshotDelta(prev, curr)

	if len(result.Backends.Removed) != 1 {
		t.Fatalf("expected 1 removed, got %v", result.Backends.Removed)
	}
	if result.Backends.Removed[0] != "b" {
		t.Fatalf("expected 'b' removed, got %q", result.Backends.Removed[0])
	}
	if len(result.Backends.AddedChanged) != 0 {
		t.Fatalf("expected 0 added/changed, got %v", result.Backends.AddedChanged)
	}
}

func TestSnapshotDelta_modifiedResource(t *testing.T) {
	original := makeBackend("svc")
	modified := makeBackend("svc")
	modified.Protocol = "HTTPS" // different content, same name

	prev := &ir.Snapshot{Backends: []ir.BackendCluster{original}}
	curr := &ir.Snapshot{Backends: []ir.BackendCluster{modified}}

	result := SnapshotDelta(prev, curr)

	if len(result.Backends.AddedChanged) != 1 {
		t.Fatalf("expected 1 modified (in AddedChanged), got %v", result.Backends.AddedChanged)
	}
	if result.Backends.AddedChanged[0] != "svc" {
		t.Fatalf("expected 'svc' in AddedChanged, got %q", result.Backends.AddedChanged[0])
	}
	if len(result.Backends.Removed) != 0 {
		t.Fatalf("expected 0 removed, got %v", result.Backends.Removed)
	}
}

func TestSnapshotDelta_nonIncremental_MoreThanHalf(t *testing.T) {
	// 3 old backends, 2 changed (AddedChanged) + 1 removed = 3 changes out of 3 > 50%
	old := []ir.BackendCluster{
		makeBackend("a"), makeBackend("b"), makeBackend("c"),
	}
	modified := makeBackend("b")
	modified.Protocol = "gRPC"
	new := []ir.BackendCluster{
		modified,         // modified
		makeBackend("d"), // new (removes a and c, adds d)
	}
	prev := &ir.Snapshot{Backends: old}
	curr := &ir.Snapshot{Backends: new}

	result := SnapshotDelta(prev, curr)

	if !result.Backends.HasNonIncremental(len(old)) {
		t.Fatalf("expected HasNonIncremental=true with %d changes / %d old > 50%% (added=%v removed=%v)",
			result.Backends.TotalChanges(), len(old),
			result.Backends.AddedChanged, result.Backends.Removed)
	}
}

func TestSnapshotDelta_nonIncremental_NotExceeded(t *testing.T) {
	// 4 old backends, 1 added = 1 change out of 4 = 25% → not non-incremental
	old := []ir.BackendCluster{
		makeBackend("a"), makeBackend("b"), makeBackend("c"), makeBackend("d"),
	}
	new := []ir.BackendCluster{
		makeBackend("a"), makeBackend("b"), makeBackend("c"), makeBackend("d"),
		makeBackend("e"),
	}
	prev := &ir.Snapshot{Backends: old}
	curr := &ir.Snapshot{Backends: new}

	result := SnapshotDelta(prev, curr)

	if result.Backends.HasNonIncremental(len(old)) {
		t.Fatalf("expected HasNonIncremental=false with %d changes / %d old = 25%% (added=%v removed=%v)",
			result.Backends.TotalChanges(), len(old),
			result.Backends.AddedChanged, result.Backends.Removed)
	}
}

func TestSnapshotDelta_multipleResourceTypes(t *testing.T) {
	prev := &ir.Snapshot{
		Listeners:  []ir.Listener{makeListener("http", 80)},
		HTTPRoutes: []ir.HTTPRoute{makeHTTPRoute("r1"), makeHTTPRoute("r2")},
		Backends:   []ir.BackendCluster{makeBackend("b1")},
	}

	currListeners := makeListener("http", 80)
	currListeners.Hostnames = []string{"example.com"} // modified
	curr := &ir.Snapshot{
		Listeners:  []ir.Listener{currListeners, makeListener("https", 443)},  // modified + added
		HTTPRoutes: []ir.HTTPRoute{makeHTTPRoute("r2")},                       // r1 removed
		Backends:   []ir.BackendCluster{makeBackend("b1"), makeBackend("b2")}, // b2 added
	}

	result := SnapshotDelta(prev, curr)

	// Listeners: http/80 modified, https/443 added
	if len(result.Listeners.AddedChanged) != 2 {
		t.Fatalf("expected 2 listener changes, got %v", result.Listeners.AddedChanged)
	}
	if len(result.Listeners.Removed) != 0 {
		t.Fatalf("expected no listeners removed, got %v", result.Listeners.Removed)
	}
	// HTTPRoutes: r1 removed
	if len(result.HTTPRoutes.Removed) != 1 || result.HTTPRoutes.Removed[0] != "default/r1" {
		t.Fatalf("expected removed=[default/r1], got %v", result.HTTPRoutes.Removed)
	}
	// Backends: b2 added
	if len(result.Backends.AddedChanged) != 1 || result.Backends.AddedChanged[0] != "b2" {
		t.Fatalf("expected added=[b2], got %v", result.Backends.AddedChanged)
	}
}

// ---- SnapshotVersions ----

func TestSnapshotVersions_returnsAllTypes(t *testing.T) {
	snap := &ir.Snapshot{
		Listeners:    []ir.Listener{makeListener("http", 80)},
		HTTPRoutes:   []ir.HTTPRoute{makeHTTPRoute("r1")},
		GRPCRoutes:   []ir.GRPCRoute{makeGRPCRoute("grpc1")},
		StreamRoutes: []ir.StreamRoute{makeStreamRoute("stream1")},
		Backends:     []ir.BackendCluster{makeBackend("b1")},
		Secrets:      []ir.SecretMaterial{makeSecret("s1")},
	}
	versions := SnapshotVersions(snap)

	for _, typeURL := range []string{
		typeURLListener, typeURLHTTPRoute, typeURLGRPCRoute,
		typeURLStreamRoute, typeURLBackend, typeURLSecret,
	} {
		if _, ok := versions[typeURL]; !ok {
			t.Fatalf("expected type URL %s in versions map", typeURL)
		}
	}
	if _, ok := versions[typeURLListener]["http/80"]; !ok {
		t.Fatalf("expected listener 'http/80' in versions, got %v", versions[typeURLListener])
	}
	if _, ok := versions[typeURLHTTPRoute]["default/r1"]; !ok {
		t.Fatalf("expected route 'default/r1' in versions, got %v", versions[typeURLHTTPRoute])
	}
	if _, ok := versions[typeURLBackend]["b1"]; !ok {
		t.Fatalf("expected backend 'b1' in versions, got %v", versions[typeURLBackend])
	}
}

func TestSnapshotVersions_emptyResources(t *testing.T) {
	snap := &ir.Snapshot{}
	versions := SnapshotVersions(snap)

	for _, typeURL := range []string{
		typeURLListener, typeURLHTTPRoute, typeURLGRPCRoute,
		typeURLStreamRoute, typeURLBackend, typeURLSecret,
	} {
		m, ok := versions[typeURL]
		if !ok {
			t.Fatalf("expected type URL %s in versions map even when empty", typeURL)
		}
		if len(m) != 0 {
			t.Fatalf("expected empty version map for %s, got %d entries", typeURL, len(m))
		}
	}
}

// ---- typeResourceCount ----

func TestTypeResourceCount_nilSnapshot(t *testing.T) {
	if got := typeResourceCount(typeURLListener, nil); got != 0 {
		t.Fatalf("expected 0 for nil snapshot, got %d", got)
	}
	if got := typeResourceCount(typeURLHTTPRoute, nil); got != 0 {
		t.Fatalf("expected 0 for nil snapshot, got %d", got)
	}
}

func TestTypeResourceCount(t *testing.T) {
	snap := &ir.Snapshot{
		Listeners:    []ir.Listener{makeListener("l1", 80), makeListener("l2", 443)},
		HTTPRoutes:   []ir.HTTPRoute{makeHTTPRoute("r1")},
		GRPCRoutes:   []ir.GRPCRoute{makeGRPCRoute("g1")},
		StreamRoutes: []ir.StreamRoute{makeStreamRoute("s1")},
		Backends:     []ir.BackendCluster{makeBackend("b1"), makeBackend("b2"), makeBackend("b3")},
		Secrets:      []ir.SecretMaterial{makeSecret("s1")},
	}
	if got := typeResourceCount(typeURLListener, snap); got != 2 {
		t.Fatalf("listener count = %d, want 2", got)
	}
	if got := typeResourceCount(typeURLHTTPRoute, snap); got != 1 {
		t.Fatalf("HTTP route count = %d, want 1", got)
	}
	if got := typeResourceCount(typeURLGRPCRoute, snap); got != 1 {
		t.Fatalf("gRPC route count = %d, want 1", got)
	}
	if got := typeResourceCount(typeURLStreamRoute, snap); got != 1 {
		t.Fatalf("stream route count = %d, want 1", got)
	}
	if got := typeResourceCount(typeURLBackend, snap); got != 3 {
		t.Fatalf("backend count = %d, want 3", got)
	}
	if got := typeResourceCount(typeURLSecret, snap); got != 1 {
		t.Fatalf("secret count = %d, want 1", got)
	}
	if got := typeResourceCount("unknown.type", snap); got != 0 {
		t.Fatalf("unknown type count = %d, want 0", got)
	}
}

// ---- DeltaDiff with non-trivial types ----

func TestDeltaDiff_secret(t *testing.T) {
	s1 := ir.SecretMaterial{Name: "s1", CertPEM: "abc", KeyPEM: "def"}
	s2 := ir.SecretMaterial{Name: "s2", CertPEM: "ghi", KeyPEM: "jkl"}
	s1Modified := ir.SecretMaterial{Name: "s1", CertPEM: "abc-updated", KeyPEM: "def"}

	old := []ir.SecretMaterial{s1}
	new := []ir.SecretMaterial{s1Modified, s2}
	added, removed := DeltaDiff(old, new, secretNameFn, func(s *ir.SecretMaterial) string {
		return ResourceVersion(s)
	})

	if len(removed) != 0 {
		t.Fatalf("expected no removed, got %v", removed)
	}
	if len(added) != 2 {
		t.Fatalf("expected 2 added/changed, got %v", added)
	}
}

func TestDeltaDiff_bothAddedAndRemoved(t *testing.T) {
	a := makeGRPCRoute("a")
	b := makeGRPCRoute("b")
	c := makeGRPCRoute("c")
	d := makeGRPCRoute("d")

	old := []ir.GRPCRoute{a, b, c}
	new := []ir.GRPCRoute{c, d}
	added, removed := DeltaDiff(old, new, grpcRouteNameFn, func(r *ir.GRPCRoute) string {
		return ResourceVersion(r)
	})

	if len(removed) != 2 {
		t.Fatalf("expected 2 removed (a,b), got %v", removed)
	}
	if len(added) != 1 {
		t.Fatalf("expected 1 added (d), got %v", added)
	}
	if added[0] != "default/d" {
		t.Fatalf("expected added[0]='default/d', got %q", added[0])
	}
}

// ---- findByName ----

func TestFindByName_found(t *testing.T) {
	items := []ir.Listener{
		makeListener("http", 80),
		makeListener("https", 443),
		makeListener("admin", 9090),
	}
	result := findByName(items, listenerNameFn, "https/443")
	if result.Name != "https" || result.Port != 443 {
		t.Fatalf("expected https/443, got %s/%d", result.Name, result.Port)
	}
}

func TestFindByName_notFound(t *testing.T) {
	items := []ir.Listener{
		makeListener("http", 80),
	}
	result := findByName(items, listenerNameFn, "nonexistent/1234")
	if result.Name != "" || result.Port != 0 {
		t.Fatalf("expected zero value, got %s/%d", result.Name, result.Port)
	}
}
