package lifecycle

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSupervisorMarksStartupGateReadyAfterAllComponentsStart(t *testing.T) {
	t.Parallel()

	gate := NewStartupGate("controlplane startup incomplete")
	supervisor := NewSupervisor(
		testLogger(),
		100*time.Millisecond,
		gate,
		Component{
			Name: "admin",
			Run: func(ctx context.Context, markStarted func()) error {
				markStarted()
				<-ctx.Done()
				return nil
			},
		},
		Component{
			Name: "grpc",
			Run: func(ctx context.Context, markStarted func()) error {
				markStarted()
				<-ctx.Done()
				return nil
			},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- supervisor.Run(ctx)
	}()

	waitFor(t, time.Second, func() bool {
		return gate.Check(nil) == nil
	}, "expected startup gate to become ready")

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor did not exit after cancellation")
	}

	if err := gate.Check(nil); err == nil {
		t.Fatal("expected startup gate to become not ready after shutdown")
	}
}

func TestSupervisorReturnsComponentStartupFailure(t *testing.T) {
	t.Parallel()

	gate := NewStartupGate("controlplane startup incomplete")
	supervisor := NewSupervisor(
		testLogger(),
		100*time.Millisecond,
		gate,
		Component{
			Name: "grpc",
			Run: func(context.Context, func()) error {
				return errors.New("listen tcp :18080: bind failed")
			},
		},
	)

	err := supervisor.Run(context.Background())
	if err == nil {
		t.Fatal("expected startup failure")
	}
	if !strings.Contains(err.Error(), `component "grpc" failed during startup`) {
		t.Fatalf("expected component failure context, got %v", err)
	}
	if gate.Check(nil) == nil {
		t.Fatal("expected startup gate to remain not ready")
	}
}

func TestSupervisorCancelsPeersWhenComponentFailsAfterStartup(t *testing.T) {
	t.Parallel()

	canceled := make(chan struct{}, 1)
	gate := NewStartupGate("controlplane startup incomplete")
	supervisor := NewSupervisor(
		testLogger(),
		100*time.Millisecond,
		gate,
		Component{
			Name: "manager",
			Run: func(ctx context.Context, markStarted func()) error {
				markStarted()
				<-ctx.Done()
				select {
				case canceled <- struct{}{}:
				default:
				}
				return nil
			},
		},
		Component{
			Name: "metrics",
			Run: func(_ context.Context, markStarted func()) error {
				markStarted()
				return errors.New("serve failed")
			},
		},
	)

	err := supervisor.Run(context.Background())
	if err == nil {
		t.Fatal("expected runtime failure")
	}
	if !strings.Contains(err.Error(), `component "metrics" stopped unexpectedly`) &&
		!strings.Contains(err.Error(), `component "metrics" failed`) {
		t.Fatalf("expected metrics failure in error, got %v", err)
	}

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("expected peer component to observe cancellation")
	}
}

func TestSupervisorTimesOutWaitingForStartup(t *testing.T) {
	t.Parallel()

	gate := NewStartupGate("controlplane startup incomplete")
	supervisor := NewSupervisor(
		testLogger(),
		20*time.Millisecond,
		gate,
		Component{
			Name: "manager",
			Run: func(ctx context.Context, _ func()) error {
				<-ctx.Done()
				return nil
			},
		},
	)

	err := supervisor.Run(context.Background())
	if err == nil {
		t.Fatal("expected startup timeout")
	}
	if !strings.Contains(err.Error(), "timed out waiting for components to start") {
		t.Fatalf("expected startup timeout, got %v", err)
	}
	if gate.Check(nil) == nil {
		t.Fatal("expected startup gate to remain not ready")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitFor(t *testing.T, timeout time.Duration, check func() bool, message string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal(message)
}
