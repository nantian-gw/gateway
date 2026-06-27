package translator

import (
	"testing"

	"github.com/nantian-gw/gateway/internal/ir"
)

func TestApplyPartialSnapshotWithSecrets_NilCurrent(t *testing.T) {
	backends := []ir.BackendCluster{{Name: "svc1", Namespace: "ns1"}}
	listeners := []ir.Listener{{Name: "l1"}}
	secrets := []ir.SecretMaterial{{Name: "s1", Namespace: "ns1"}}

	result := ApplyPartialSnapshotWithSecrets(nil, backends, listeners, secrets)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Backends) != 1 || result.Backends[0].Name != "svc1" {
		t.Error("backends not set correctly")
	}
	if len(result.Listeners) != 1 || result.Listeners[0].Name != "l1" {
		t.Error("listeners not set correctly")
	}
	if len(result.Secrets) != 1 || result.Secrets[0].Name != "s1" {
		t.Error("secrets not set correctly")
	}
}

func TestApplyPartialSnapshotWithSecrets_ReplaceBackends(t *testing.T) {
	current := &ir.Snapshot{
		ID:         "v1",
		Backends:   []ir.BackendCluster{{Name: "old-svc", Namespace: "ns1"}},
		Listeners:  []ir.Listener{{Name: "l1", Port: 80}},
		HTTPRoutes: []ir.HTTPRoute{{Name: "r1", Namespace: "ns1"}},
	}
	newBackends := []ir.BackendCluster{{Name: "new-svc", Namespace: "ns1"}}

	result := ApplyPartialSnapshotWithSecrets(current, newBackends, nil, nil)
	if result.ID != "v1" {
		t.Errorf("ID = %s, want v1", result.ID)
	}
	if len(result.Backends) != 1 || result.Backends[0].Name != "new-svc" {
		t.Error("backends not replaced")
	}
	if len(result.Listeners) != 1 || result.Listeners[0].Port != 80 {
		t.Error("listeners not preserved")
	}
	if len(result.HTTPRoutes) != 1 || result.HTTPRoutes[0].Name != "r1" {
		t.Error("routes not preserved")
	}
}
