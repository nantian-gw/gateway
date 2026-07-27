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

func TestSupervisorStopsComponentsInReverseOrder(t *testing.T) {
	t.Parallel()

	stopped := make([]string, 0, 3)
	gate := NewStartupGate("controlplane startup incomplete")
	supervisor := NewSupervisor(
		testLogger(),
		100*time.Millisecond,
		gate,
		Component{
			Name: "first",
			Run: func(ctx context.Context, markStarted func()) error {
				markStarted()
				<-ctx.Done()
				stopped = append(stopped, "first")
				return nil
			},
		},
		Component{
			Name: "second",
			Run: func(ctx context.Context, markStarted func()) error {
				markStarted()
				<-ctx.Done()
				stopped = append(stopped, "second")
				return nil
			},
		},
		Component{
			Name: "third",
			Run: func(ctx context.Context, markStarted func()) error {
				markStarted()
				<-ctx.Done()
				stopped = append(stopped, "third")
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

	if len(stopped) != 3 {
		t.Fatalf("expected 3 components to stop, got %d: %v", len(stopped), stopped)
	}
	if stopped[0] != "third" || stopped[1] != "second" || stopped[2] != "first" {
		t.Fatalf("expected reverse stop order [third second first], got %v", stopped)
	}
}

func TestSupervisorShutdownTimeout(t *testing.T) {
	t.Parallel()

	stuckCh := make(chan struct{})

	gate := NewStartupGate("controlplane startup incomplete")
	supervisor := NewSupervisor(
		testLogger(),
		100*time.Millisecond,
		gate,
		Component{
			Name: "hanging",
			Run: func(ctx context.Context, markStarted func()) error {
				markStarted()
				<-ctx.Done()
				return nil
			},
		},
		Component{
			Name: "stuck",
			Run: func(ctx context.Context, markStarted func()) error {
				markStarted()
				<-stuckCh
				return nil
			},
		},
	)
	supervisor.shutdownTimeout = 50 * time.Millisecond

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
		if err == nil {
			t.Fatal("expected timeout error from stuck component")
		}
		if !strings.Contains(err.Error(), `did not stop within`) {
			t.Fatalf("expected shutdown timeout error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not exit after shutdown timeout")
	}

	close(stuckCh)
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
