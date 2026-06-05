package grpcserver

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/nantian-gw/gateway/controlplane/internal/config"
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
	}
	settings.streamIdleHeartbeat = deriveStreamIdleHeartbeatInterval(settings.keepaliveParams.Time)

	return []grpc.ServerOption{
		grpc.KeepaliveParams(settings.keepaliveParams),
		grpc.KeepaliveEnforcementPolicy(settings.keepalivePolicy),
	}, settings
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
