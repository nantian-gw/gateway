package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/nantian-gw/gateway/internal/config"
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/nodestatus"
	"github.com/nantian-gw/gateway/internal/observability"
)

const gracefulStopTimeout = 3 * time.Second

type Server struct {
	controlv1.UnimplementedConfigurationDiscoveryServiceServer
	addr          string
	store         *ir.SnapshotStore
	nodes         *nodestatus.Registry
	logger        *slog.Logger
	metrics       *observability.Metrics
	runtime       grpcRuntimeSettings
	protoCache    *snapshotProtoCache
	serverOptions []grpc.ServerOption
	shutdownCh    chan struct{}
	shutdownOnce  sync.Once
	streamsMu     sync.Mutex
	nextStreamID  uint64
	activeStreams map[string]*streamRegistration
}

const (
	streamTerminationShutdown         = "shutdown"
	streamTerminationClientDisconnect = "client_disconnect"
	streamTerminationStreamError      = "stream_error"
	streamTerminationSendTimeout      = "send_timeout"
	streamTerminationAckTimeout       = "ack_timeout"
	streamTerminationSuperseded       = "superseded"
	streamTerminationInvalidRequest   = "invalid_request"
	streamTerminationOther            = "other"

	statusReportRejectionShutdown       = "shutdown"
	statusReportRejectionInvalidRequest = "invalid_request"
	statusReportRejectionUnknownNode    = "unknown_node"
	statusReportRejectionOther          = "other"
)

type streamRegistration struct {
	nodeID      string
	id          uint64
	superseded  chan struct{}
	supersedeMu sync.Once
}

func New(
	addr string,
	tlsConfig config.GRPCTLSConfig,
	runtimeConfig config.GRPCRuntimeConfig,
	store *ir.SnapshotStore,
	nodes *nodestatus.Registry,
	logger *slog.Logger,
	metrics *observability.Metrics,
) (*Server, error) {
	serverOptions, runtimeSettings, err := serverOptionsFromConfig(tlsConfig, runtimeConfig)
	if err != nil {
		return nil, err
	}

	return &Server{
		addr:          addr,
		store:         store,
		nodes:         nodes,
		logger:        logger,
		metrics:       metrics,
		runtime:       runtimeSettings,
		protoCache:    newSnapshotProtoCache(nil),
		serverOptions: serverOptions,
		shutdownCh:    make(chan struct{}),
		activeStreams: make(map[string]*streamRegistration),
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}
	return s.Serve(ctx, lis, nil)
}

func (s *Server) Serve(ctx context.Context, lis net.Listener, markStarted func()) error {
	grpcServer := grpc.NewServer(s.serverOptions...)
	controlv1.RegisterConfigurationDiscoveryServiceServer(grpcServer, s)

	go func() {
		<-ctx.Done()
		s.signalShutdown()
		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-time.After(gracefulStopTimeout):
			grpcServer.Stop()
		}
	}()

	s.logger.Info(
		"starting grpc server",
		"addr",
		lis.Addr().String(),
		"keepalive_time",
		s.runtime.keepaliveParams.Time,
		"keepalive_timeout",
		s.runtime.keepaliveParams.Timeout,
		"min_ping_interval",
		s.runtime.keepalivePolicy.MinTime,
		"max_connection_idle",
		s.runtime.keepaliveParams.MaxConnectionIdle,
		"max_connection_age",
		s.runtime.keepaliveParams.MaxConnectionAge,
		"max_connection_age_grace",
		s.runtime.keepaliveParams.MaxConnectionAgeGrace,
		"snapshot_send_timeout",
		s.runtime.snapshotSendTimeout,
		"snapshot_ack_timeout",
		s.runtime.snapshotAckTimeout,
		"stream_idle_heartbeat_interval",
		s.runtime.streamIdleHeartbeat,
		"permit_without_stream",
		s.runtime.keepalivePolicy.PermitWithoutStream,
	)
	if markStarted != nil {
		markStarted()
	}
	if err := grpcServer.Serve(lis); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if ctx.Err() != nil {
		return nil
	}
	return errors.New("grpc server stopped unexpectedly")
}

func (s *Server) signalShutdown() {
	s.shutdownOnce.Do(func() {
		close(s.shutdownCh)
	})
}
