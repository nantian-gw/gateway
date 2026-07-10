package xds

import (
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/nantian-gw/gateway/internal/config"
)

const (
	defaultKeepaliveTimeout = 10 * time.Second
	defaultMinPingInterval  = 15 * time.Second
	defaultStreamHeartbeat  = 10 * time.Second
)

type grpcRuntimeSettings struct {
	keepaliveParams     keepalive.ServerParameters
	keepalivePolicy     keepalive.EnforcementPolicy
	snapshotSendTimeout time.Duration
	snapshotAckTimeout  time.Duration
	streamIdleHeartbeat time.Duration
	gracefulStopTimeout time.Duration
}

func runtimeServerOptionsFromConfig(cfg config.GRPCRuntimeConfig) ([]grpc.ServerOption, grpcRuntimeSettings) {
	snapshotSendTimeout := parseDurationOrDefault(cfg.SnapshotSendTimeout, 5*time.Second)
	if snapshotSendTimeout <= 0 {
		snapshotSendTimeout = 5 * time.Second
	}
	snapshotAckTimeout := parseDurationOrDefault(cfg.SnapshotAckTimeout, 30*time.Second)
	if snapshotAckTimeout <= 0 {
		snapshotAckTimeout = 30 * time.Second
	}
	gracefulStopTimeout := parseDurationOrDefault(cfg.GracefulStopTimeout, 3*time.Second)
	if gracefulStopTimeout <= 0 {
		gracefulStopTimeout = 3 * time.Second
	}

	settings := grpcRuntimeSettings{
		keepaliveParams: keepalive.ServerParameters{
			MaxConnectionIdle:     parseDurationOrDefault(cfg.MaxConnectionIdle, 2*time.Minute),
			MaxConnectionAge:      parseDurationOrDefault(cfg.MaxConnectionAge, 30*time.Minute),
			MaxConnectionAgeGrace: parseDurationOrDefault(cfg.MaxConnectionAgeGrace, 30*time.Second),
			Time:                  parseDurationOrDefault(cfg.KeepaliveTime, 30*time.Second),
			Timeout:               parseDurationOrDefault(cfg.KeepaliveTimeout, defaultKeepaliveTimeout),
		},
		keepalivePolicy: keepalive.EnforcementPolicy{
			MinTime:             parseDurationOrDefault(cfg.MinPingInterval, defaultMinPingInterval),
			PermitWithoutStream: cfg.PermitWithoutStream,
		},
		snapshotSendTimeout: snapshotSendTimeout,
		snapshotAckTimeout:  snapshotAckTimeout,
		gracefulStopTimeout: gracefulStopTimeout,
	}
	settings.streamIdleHeartbeat = deriveStreamIdleHeartbeatInterval(settings.keepaliveParams.Time)

	options := []grpc.ServerOption{
		grpc.KeepaliveParams(settings.keepaliveParams),
		grpc.KeepaliveEnforcementPolicy(settings.keepalivePolicy),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}

	// Flow-control and message-size knobs are opt-in: a zero value leaves the
	// gRPC defaults in place. Setting an explicit window disables gRPC's BDP
	// autotuning, so only override after measuring.
	if cfg.InitialWindowSize > 0 {
		options = append(options, grpc.InitialWindowSize(cfg.InitialWindowSize))
	}
	if cfg.InitialConnWindowSize > 0 {
		options = append(options, grpc.InitialConnWindowSize(cfg.InitialConnWindowSize))
	}
	if cfg.MaxConcurrentStreams > 0 {
		options = append(options, grpc.MaxConcurrentStreams(cfg.MaxConcurrentStreams))
	}
	if cfg.MaxRecvMsgSize > 0 {
		options = append(options, grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize))
	}

	return options, settings
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func deriveStreamIdleHeartbeatInterval(keepaliveTime time.Duration) time.Duration {
	if keepaliveTime <= 0 {
		return defaultStreamHeartbeat
	}

	interval := keepaliveTime / 3
	if interval <= 0 {
		return time.Millisecond
	}
	return interval
}
