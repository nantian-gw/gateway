package xds

import (
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/nantian-gw/gateway/internal/config"
)

func TestServerOptionsFromConfigIncludesRuntimeConstraintsWithoutTLS(t *testing.T) {
	t.Parallel()

	opts, settings, err := serverOptionsFromConfig(
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{
			KeepaliveTime:         "45s",
			KeepaliveTimeout:      "12s",
			MinPingInterval:       "20s",
			MaxConnectionIdle:     "3m",
			MaxConnectionAge:      "40m",
			MaxConnectionAgeGrace: "90s",
			SnapshotSendTimeout:   "35s",
			SnapshotAckTimeout:    "45s",
			PermitWithoutStream:   true,
		},
	)
	if err != nil {
		t.Fatalf("serverOptionsFromConfig returned error: %v", err)
	}

	snapshot := captureAppliedServerOptions(t, opts)
	if snapshot.keepaliveTime != 45*time.Second {
		t.Fatalf("unexpected keepalive time: %s", snapshot.keepaliveTime)
	}
	if snapshot.keepaliveTimeout != 12*time.Second {
		t.Fatalf("unexpected keepalive timeout: %s", snapshot.keepaliveTimeout)
	}
	if snapshot.minPingInterval != 20*time.Second {
		t.Fatalf("unexpected min ping interval: %s", snapshot.minPingInterval)
	}
	if snapshot.maxConnectionIdle != 3*time.Minute {
		t.Fatalf("unexpected max connection idle: %s", snapshot.maxConnectionIdle)
	}
	if snapshot.maxConnectionAge != 40*time.Minute {
		t.Fatalf("unexpected max connection age: %s", snapshot.maxConnectionAge)
	}
	if snapshot.maxConnectionAgeGrace != 90*time.Second {
		t.Fatalf("unexpected max connection age grace: %s", snapshot.maxConnectionAgeGrace)
	}
	if settings.snapshotSendTimeout != 35*time.Second {
		t.Fatalf("unexpected snapshot send timeout: %s", settings.snapshotSendTimeout)
	}
	if settings.snapshotAckTimeout != 45*time.Second {
		t.Fatalf("unexpected snapshot ack timeout: %s", settings.snapshotAckTimeout)
	}
	if settings.streamIdleHeartbeat != 15*time.Second {
		t.Fatalf("unexpected stream idle heartbeat interval: %s", settings.streamIdleHeartbeat)
	}
	if !snapshot.permitWithoutStream {
		t.Fatal("expected permitWithoutStream to be enabled")
	}
}

func TestRuntimeServerOptionsFromConfigFallsBackForNonPositiveSnapshotSendTimeout(t *testing.T) {
	t.Parallel()

	_, settings, err := serverOptionsFromConfig(
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{
			SnapshotSendTimeout: "0s",
			SnapshotAckTimeout:  "0s",
		},
	)
	if err != nil {
		t.Fatalf("serverOptionsFromConfig returned error: %v", err)
	}
	if settings.snapshotSendTimeout != 5*time.Second {
		t.Fatalf("unexpected snapshot send timeout fallback: %s", settings.snapshotSendTimeout)
	}
	if settings.snapshotAckTimeout != 30*time.Second {
		t.Fatalf("unexpected snapshot ack timeout fallback: %s", settings.snapshotAckTimeout)
	}
	if settings.streamIdleHeartbeat != 10*time.Second {
		t.Fatalf("unexpected stream idle heartbeat fallback: %s", settings.streamIdleHeartbeat)
	}
}

func TestRuntimeServerOptionsFromConfigIncludesTracingStatsHandler(t *testing.T) {
	t.Parallel()

	opts, _ := runtimeServerOptionsFromConfig(config.GRPCRuntimeConfig{})
	if len(opts) != 3 {
		t.Fatalf("unexpected runtime server option count: %d", len(opts))
	}
}

type appliedServerOptions struct {
	keepaliveTime         time.Duration
	keepaliveTimeout      time.Duration
	minPingInterval       time.Duration
	maxConnectionIdle     time.Duration
	maxConnectionAge      time.Duration
	maxConnectionAgeGrace time.Duration
	permitWithoutStream   bool
}

func captureAppliedServerOptions(t *testing.T, opts []grpc.ServerOption) appliedServerOptions {
	t.Helper()

	server := grpc.NewServer(opts...)
	serverValue := reflect.ValueOf(server).Elem()
	optionsValue := serverValue.FieldByName("opts")
	keepaliveValue := optionsValue.FieldByName("keepaliveParams")
	enforcementValue := optionsValue.FieldByName("keepalivePolicy")

	return appliedServerOptions{
		keepaliveTime:         time.Duration(keepaliveValue.FieldByName("Time").Int()),
		keepaliveTimeout:      time.Duration(keepaliveValue.FieldByName("Timeout").Int()),
		minPingInterval:       time.Duration(enforcementValue.FieldByName("MinTime").Int()),
		maxConnectionIdle:     time.Duration(keepaliveValue.FieldByName("MaxConnectionIdle").Int()),
		maxConnectionAge:      time.Duration(keepaliveValue.FieldByName("MaxConnectionAge").Int()),
		maxConnectionAgeGrace: time.Duration(keepaliveValue.FieldByName("MaxConnectionAgeGrace").Int()),
		permitWithoutStream:   enforcementValue.FieldByName("PermitWithoutStream").Bool(),
	}
}
