package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func preserveLoggingGlobals(t *testing.T) {
	t.Helper()

	klogState := klog.CaptureState()
	defaultLogger := slog.Default()
	t.Cleanup(func() {
		klogState.Restore()
		slog.SetDefault(defaultLogger)
	})
}

func TestControllerRuntimeLoggerUsesSlogHandler(t *testing.T) {
	preserveLoggingGlobals(t)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	controllerRuntimeLogger(logger).Info("starting controller", "controller", "snapshot-syncer")

	output := buffer.String()
	if !strings.Contains(output, `"msg":"starting controller"`) {
		t.Fatalf("expected slog JSON message, got %q", output)
	}
	if !strings.Contains(output, `"controller":"snapshot-syncer"`) {
		t.Fatalf("expected slog JSON fields, got %q", output)
	}
}

func TestConfigureKubernetesLoggingRoutesKlogThroughSlog(t *testing.T) {
	preserveLoggingGlobals(t)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	configureKubernetesLogging(logger)
	ctrl.Log.WithName("snapshot-syncer").Info("starting workers", "worker_count", 1)
	klog.Background().Info("attempting to acquire leader lease", "lease", "nantian-gw/leader")

	output := buffer.String()
	if !strings.Contains(output, `"msg":"starting workers"`) {
		t.Fatalf("expected controller-runtime log through slog, got %q", output)
	}
	if !strings.Contains(output, `"msg":"attempting to acquire leader lease"`) {
		t.Fatalf("expected klog log through slog, got %q", output)
	}
	if !strings.Contains(output, `"lease":"nantian-gw/leader"`) {
		t.Fatalf("expected structured klog fields, got %q", output)
	}
	if !strings.Contains(output, `"log_origin":"klog"`) {
		t.Fatalf("expected structured klog origin marker, got %q", output)
	}
}

func TestConfigureKubernetesLoggingRoutesLegacyKlogOutputThroughSlog(t *testing.T) {
	preserveLoggingGlobals(t)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	configureKubernetesLogging(logger)
	klog.Infof("attempting to acquire leader lease %s", "nantian-gw/leader")

	output := buffer.String()
	if !strings.Contains(output, `"msg":"attempting to acquire leader lease nantian-gw/leader"`) {
		t.Fatalf("expected klog formatted output through slog, got %q", output)
	}
	if !strings.Contains(output, `"log_origin":"klog"`) {
		t.Fatalf("expected klog origin marker, got %q", output)
	}
	if !strings.Contains(output, `"klog_source":"logging_test.go:`) {
		t.Fatalf("expected structured klog caller field, got %q", output)
	}
	if strings.Contains(output, "I0") {
		t.Fatalf("expected legacy klog prefix to be stripped, got %q", output)
	}
	if strings.Contains(output, `"msg":"logging_test.go:`) {
		t.Fatalf("expected legacy caller prefix to be stripped from message field, got %q", output)
	}
}

func TestConfigureKubernetesLoggingPreservesLegacyKlogSeverity(t *testing.T) {
	preserveLoggingGlobals(t)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	configureKubernetesLogging(logger)
	klog.Warningf("watch jitter detected for %s", "gateway/default")

	output := buffer.String()
	if !strings.Contains(output, `"level":"WARN"`) {
		t.Fatalf("expected warning severity through slog, got %q", output)
	}
	if !strings.Contains(output, `"msg":"watch jitter detected for gateway/default"`) {
		t.Fatalf("expected warning message through slog, got %q", output)
	}
	if !strings.Contains(output, `"log_origin":"klog"`) {
		t.Fatalf("expected klog origin marker, got %q", output)
	}
}
