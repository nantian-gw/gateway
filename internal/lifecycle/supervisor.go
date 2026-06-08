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

type Supervisor struct {
	logger         *slog.Logger
	startupTimeout time.Duration
	startupGate    *StartupGate
	components     []Component
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

	return &Supervisor{
		logger:         logger,
		startupTimeout: startupTimeout,
		startupGate:    startupGate,
		components:     slices.Clone(components),
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

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	resultCh := make(chan componentResult, len(s.components))
	startedCh := make(chan string, len(s.components))

	for _, component := range s.components {
		component := component
		go func() {
			var started sync.Once
			err := component.Run(ctx, func() {
				started.Do(func() {
					startedCh <- component.Name
				})
			})
			resultCh <- componentResult{name: component.Name, err: err}
		}()
	}

	started := make(map[string]struct{}, len(s.components))
	consumedResults := 0
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
			consumedResults++
			if result.err == nil {
				return s.collectAfterCancel(
					cancel,
					resultCh,
					len(s.components)-consumedResults,
					fmt.Errorf(`component %q exited before startup completed`, result.name),
				)
			}
			return s.collectAfterCancel(
				cancel,
				resultCh,
				len(s.components)-consumedResults,
				fmt.Errorf(`component %q failed during startup: %w`, result.name, result.err),
			)
		case <-timer.C:
			return s.collectAfterCancel(
				cancel,
				resultCh,
				len(s.components)-consumedResults,
				fmt.Errorf(
					"timed out waiting for components to start: %s",
					strings.Join(missingComponents(s.components, started), ", "),
				),
			)
		case <-ctx.Done():
			return s.collectAfterCancel(cancel, resultCh, len(s.components)-consumedResults, nil)
		}
	}

	if s.startupGate != nil {
		s.startupGate.MarkReady()
	}
	s.logger.Info("all lifecycle components started")

	for {
		select {
		case result := <-resultCh:
			consumedResults++
			if result.err == nil {
				return s.collectAfterCancel(
					cancel,
					resultCh,
					len(s.components)-consumedResults,
					fmt.Errorf(`component %q stopped unexpectedly`, result.name),
				)
			}
			return s.collectAfterCancel(
				cancel,
				resultCh,
				len(s.components)-consumedResults,
				fmt.Errorf(`component %q failed: %w`, result.name, result.err),
			)
		case <-ctx.Done():
			return s.collectAfterCancel(cancel, resultCh, len(s.components)-consumedResults, nil)
		}
	}
}

func (s *Supervisor) validate() error {
	seen := make(map[string]struct{}, len(s.components))
	for _, component := range s.components {
		name := strings.TrimSpace(component.Name)
		if name == "" {
			return errors.New("lifecycle component name is required")
		}
		if component.Run == nil {
			return fmt.Errorf(`lifecycle component %q has no Run function`, name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf(`duplicate lifecycle component name %q`, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func (s *Supervisor) collectAfterCancel(
	cancel context.CancelFunc,
	resultCh <-chan componentResult,
	remaining int,
	primary error,
) error {
	cancel()

	errs := make([]error, 0, remaining+1)
	if primary != nil {
		errs = append(errs, primary)
	}

	for i := 0; i < remaining; i++ {
		result := <-resultCh
		if result.err != nil {
			errs = append(errs, fmt.Errorf(`component %q shutdown error: %w`, result.name, result.err))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func missingComponents(components []Component, started map[string]struct{}) []string {
	out := make([]string, 0, len(components))
	for _, component := range components {
		if _, ok := started[component.Name]; ok {
			continue
		}
		out = append(out, component.Name)
	}
	slices.Sort(out)
	return out
}
