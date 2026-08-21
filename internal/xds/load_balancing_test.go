package xds

import (
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
)

func TestToProtoHealthCheck(t *testing.T) {
	in := &ir.HealthCheckConfig{Type: "HTTP", Path: "/healthz", ExpectedStatus: 200, HealthyThreshold: 2}
	out := toProtoHealthCheck(in)
	if out.GetType() != "HTTP" || out.GetPath() != "/healthz" || out.GetExpectedStatus() != 200 || out.GetHealthyThreshold() != 2 {
		t.Fatalf("unexpected proto: %+v", out)
	}
}

func TestToProtoOutlierDetection(t *testing.T) {
	out := toProtoOutlierDetection(&ir.OutlierDetectionConfig{Consecutive5xx: 5, MaxEjectionPercent: 50})
	if out.GetConsecutive_5Xx() != 5 || out.GetMaxEjectionPercent() != 50 {
		t.Fatalf("unexpected proto: %+v", out)
	}
}

func TestToProtoSlowStart(t *testing.T) {
	w := 30 * time.Second
	out := toProtoSlowStart(&ir.SlowStartConfig{Window: &w})
	if out.GetWindow().AsDuration() != 30*time.Second {
		t.Fatalf("unexpected proto: %+v", out)
	}
}
