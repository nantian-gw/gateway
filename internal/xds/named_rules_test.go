package xds

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/nantian-gw/gateway/internal/ir"
)

func TestToProtoSnapshotPreservesNamedRules(t *testing.T) {
	snapshot := &ir.Snapshot{
		HTTPRoutes: []ir.HTTPRoute{{
			Name:      "http",
			Namespace: "default",
			Rules:     []ir.HTTPRule{{Name: "http-primary"}},
		}},
		GRPCRoutes: []ir.GRPCRoute{{
			Name:      "grpc",
			Namespace: "default",
			Rules:     []ir.GRPCRule{{Name: "grpc-primary"}},
		}},
		StreamRoutes: []ir.StreamRoute{{
			Name:      "tcp",
			Namespace: "default",
			Kind:      "TCP",
			Rules:     []ir.StreamRule{{Name: "tcp-primary"}},
		}},
	}

	payload, err := protojson.Marshal(toProtoSnapshot(snapshot))
	if err != nil {
		t.Fatalf("marshal proto snapshot: %v", err)
	}

	for _, want := range []string{"http-primary", "grpc-primary", "tcp-primary"} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("proto snapshot JSON = %s, want rule name %q", payload, want)
		}
	}
}
