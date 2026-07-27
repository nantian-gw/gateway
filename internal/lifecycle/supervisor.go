package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

type Component struct {
	Name string
	Run  func(context.Context, func()) error
}

type componentResult struct {
	name string
	err  error
}

type StartupGate struct {
	message string
	mu      sync.RWMutex
	ready   bool
}

func NewStartupGate(message string) *StartupGate {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "startup in progress"
	}
	return &StartupGate{message: message}
}

func (g *StartupGate) MarkReady() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ready = true
}

func (g *StartupGate) MarkNotReady() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ready = false
}

func (g *StartupGate) Check(_ *http.Request) error {
	if g == nil {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.ready {
		return nil
	}
	return errors.New(g.message)
}

type managedComponent struct {
	Component
	ctx    context.Context
	cancel context.CancelFunc
	done   chan error
}

type Supervisor struct {
	logger          *slog.Logger
	startupTimeout  time.Duration
	shutdownTimeout time.Duration
	startupGate     *StartupGate
	components      []managedComponent
}

func NewSupervisor(
	logger *slog.Logger,
	startupTimeout time.Duration,
	startupGate *StartupGate,
	components ...Component,
) *Supervisor {
	if logger == nil {
		logger = slog.Default()
	}
	if startupTimeout <= 0 {
		startupTimeout = 30 * time.Second
	}

	managed := make([]managedComponent, len(components))
	for i, c := range components {
		managed[i] = managedComponent{
			Component: c,
			done:      make(chan error, 1),
		}
	}

	return &Supervisor{
		logger:          logger,
		startupTimeout:  startupTimeout,
		shutdownTimeout: 30 * time.Second,
		startupGate:     startupGate,
		components:      managed,
	}
}

func (s *Supervisor) Run(parent context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}

	if s.startupGate != nil {
		s.startupGate.MarkNotReady()
		defer s.startupGate.MarkNotReady()
	}

	if len(s.components) == 0 {
		<-parent.Done()
		return nil
	}

	ctx, cancelAll := context.WithCancel(parent)

	startedCh := make(chan string, len(s.components))
	resultCh := make(chan componentResult, len(s.components))

	for i := range s.components {
		mc := &s.components[i]
		mc.ctx, mc.cancel = context.WithCancel(context.Background())
	}

	for i := range s.components {
		mc := &s.components[i]
		go func() {
			var started sync.Once
			err := mc.Run(mc.ctx, func() {
				started.Do(func() {
					startedCh <- mc.Name
				})
			})
			mc.done <- err
			resultCh <- componentResult{name: mc.Name, err: err}
		}()
	}

	started := make(map[string]struct{}, len(s.components))
	var consumed int
	timer := time.NewTimer(s.startupTimeout)
	defer timer.Stop()

	for len(started) < len(s.components) {
		select {
		case name := <-startedCh:
			if _, exists := started[name]; exists {
				continue
			}
			started[name] = struct{}{}
			s.logger.Info("component started", "component", name)
		case result := <-resultCh:
			consumed++
			if result.err == nil {
				return s.stopInReverseAndDrain(
					cancelAll, resultCh, len(s.components)-consumed,
					fmt.Errorf(`component %q exited before startup completed`, result.name),
				)
			}
			return s.stopInReverseAndDrain(
				cancelAll, resultCh, len(s.components)-consumed,
				fmt.Errorf(`component %q failed during startup: %w`, result.name, result.err),
			)
		case <-timer.C:
			return s.stopInReverseAndDrain(
				cancelAll, resultCh, len(s.components)-consumed,
				fmt.Errorf(
					"timed out waiting for components to start: %s",
					strings.Join(missingManagedComponents(s.components, started), ", "),
				),
			)
		case <-ctx.Done():
			return s.stopInReverseAndDrain(cancelAll, resultCh, len(s.components)-consumed, nil)
		}
	}

	if s.startupGate != nil {
		s.startupGate.MarkReady()
	}
	s.logger.Info("all lifecycle components started")

	select {
	case result := <-resultCh:
		consumed++
		if result.err == nil {
			return s.stopInReverseAndDrain(
				cancelAll, resultCh, len(s.components)-consumed,
				fmt.Errorf(`component %q stopped unexpectedly`, result.name),
			)
		}
		return s.stopInReverseAndDrain(
			cancelAll, resultCh, len(s.components)-consumed,
			fmt.Errorf(`component %q failed: %w`, result.name, result.err),
		)
	case <-ctx.Done():
		return s.stopInReverseAndDrain(cancelAll, resultCh, len(s.components)-consumed, nil)
	}
}

func (s *Supervisor) stopInReverseAndDrain(
	cancelAll context.CancelFunc,
	resultCh <-chan componentResult,
	remaining int,
	primary error,
) error {
	defer cancelAll()

	// Drain shared resultCh without blocking; individual done channels
	// provide ordered shutdown signals. Use a non-blocking drain to avoid
	// hanging on components that may never exit (e.g. stuck goroutines).
	for i := 0; i < remaining; i++ {
		select {
		case <-resultCh:
		default:
		}
	}

	// Stop in reverse registration order using individual cancel+done.
	var errs []error
	if primary != nil {
		errs = append(errs, primary)
	}

	for i := len(s.components) - 1; i >= 0; i-- {
		mc := &s.components[i]
		s.logger.Info("stopping component", "component", mc.Name)
		mc.cancel()

		select {
		case err := <-mc.done:
			if err != nil {
				errs = append(errs, fmt.Errorf("component %q shutdown error: %w", mc.Name, err))
			}
		case <-time.After(s.shutdownTimeout):
			errs = append(errs, fmt.Errorf("component %q did not stop within %v", mc.Name, s.shutdownTimeout))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func (s *Supervisor) validate() error {
	seen := make(map[string]struct{}, len(s.components))
	for _, mc := range s.components {
		name := strings.TrimSpace(mc.Name)
		if name == "" {
			return errors.New("lifecycle component name is required")
		}
		if mc.Run == nil {
			return fmt.Errorf(`lifecycle component %q has no Run function`, name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf(`duplicate lifecycle component name %q`, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func missingManagedComponents(components []managedComponent, started map[string]struct{}) []string {
	out := make([]string, 0, len(components))
	for _, mc := range components {
		if _, ok := started[mc.Name]; ok {
			continue
		}
		out = append(out, mc.Name)
	}
	slices.Sort(out)
	return out
}
