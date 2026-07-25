package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/nantian-gw/gateway/internal/lifecycle"
	"github.com/nantian-gw/gateway/internal/xds"
)

func newManagerComponent(mgr ctrl.Manager, leaderElectionEnabled bool) lifecycle.Component {
	return lifecycle.Component{
		Name: "controller-manager",
		Run: func(ctx context.Context, markStarted func()) error {
			errCh := make(chan error, 1)
			go func() {
				errCh <- mgr.Start(ctx)
			}()

			if leaderElectionEnabled {
				markStarted()
				return waitManagerExit(ctx, errCh, false)
			}

			syncedCh := make(chan bool, 1)
			go func() {
				syncedCh <- mgr.GetCache().WaitForCacheSync(ctx)
			}()

			for {
				select {
				case synced := <-syncedCh:
					if !synced {
						if ctx.Err() != nil {
							return waitManagerExit(ctx, errCh, true)
						}
						select {
						case err := <-errCh:
							if err != nil {
								return err
							}
						default:
						}
						return errors.New("controller manager cache sync did not complete")
					}

					markStarted()
					return waitManagerExit(ctx, errCh, false)
				case err := <-errCh:
					if ctx.Err() != nil && err == nil {
						return nil
					}
					if err != nil {
						return err
					}
					return errors.New("controller manager stopped before cache sync completed")
				case <-ctx.Done():
					return waitManagerExit(ctx, errCh, true)
				}
			}
		},
	}
}

func waitManagerExit(ctx context.Context, errCh <-chan error, shuttingDown bool) error {
	err := <-errCh
	if ctx.Err() != nil {
		//nolint:nilerr // intentional: context cancellation is graceful shutdown, not an error
		return nil
	}
	if err != nil {
		return err
	}
	if shuttingDown {
		return nil
	}
	return errors.New("controller manager stopped unexpectedly")
}

func newHTTPComponent(
	name string,
	addr string,
	serve func(net.Listener) error,
	shutdown func(context.Context) error,
	closeServer func() error,
	shutdownTimeout time.Duration,
	logger *slog.Logger,
) lifecycle.Component {
	return lifecycle.Component{
		Name: name,
		Run: func(ctx context.Context, markStarted func()) error {
			listener, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", addr, err)
			}
			defer func() { _ = listener.Close() }()

			go func() { //nolint:gosec
				<-ctx.Done()

				shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
				defer cancel()

				if err := shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Warn("http component shutdown returned error", "component", name, "error", err)
					if closeErr := closeServer(); closeErr != nil {
						logger.Warn("http component force-close returned error", "component", name, "error", closeErr)
					}
				}
				if shutdownCtx.Err() == context.DeadlineExceeded {
					logger.Warn("http component shutdown timed out, forcing close", "component", name)
					if closeErr := closeServer(); closeErr != nil {
						logger.Warn("http component force-close returned error", "component", name, "error", closeErr)
					}
				}
			}()

			markStarted()
			err = serve(listener)
			switch {
			case ctx.Err() != nil:
				return nil
			case err == nil:
				return fmt.Errorf("%s server stopped unexpectedly", name)
			default:
				return err
			}
		},
	}
}

func newGRPCComponent(name, addr string, server *xds.Server) lifecycle.Component {
	return lifecycle.Component{
		Name: name,
		Run: func(ctx context.Context, markStarted func()) error {
			listener, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", addr, err)
			}
			defer func() { _ = listener.Close() }()
			return server.Serve(ctx, listener, markStarted)
		},
	}
}

func ptr[T any](value T) *T {
	return &value
}
