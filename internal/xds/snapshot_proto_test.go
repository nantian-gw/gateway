package xds

import (
	"testing"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/ir"
)

func TestToProtoCircuitBreaker(t *testing.T) {
	// nil input returns nil
	if got := toProtoCircuitBreaker(nil); got != nil {
		t.Errorf("toProtoCircuitBreaker(nil) = %v, want nil", got)
	}

	// valid input
	input := &ir.CircuitBreakerConfig{MaxInflightRequests: 100}
	got := toProtoCircuitBreaker(input)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.MaxInflightRequests != 100 {
		t.Errorf("MaxInflightRequests = %d, want 100", got.MaxInflightRequests)
	}
	if _, ok := any(got).(*controlv1.CircuitBreakerConfig); !ok {
		t.Errorf("expected *controlv1.CircuitBreakerConfig, got %T", got)
	}

	// zero value
	input2 := &ir.CircuitBreakerConfig{MaxInflightRequests: 0}
	got2 := toProtoCircuitBreaker(input2)
	if got2 == nil {
		t.Fatal("expected non-nil result for zero value")
	}
	if got2.MaxInflightRequests != 0 {
		t.Errorf("MaxInflightRequests = %d, want 0", got2.MaxInflightRequests)
	}
}
