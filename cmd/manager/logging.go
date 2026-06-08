package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	klogOriginKey = "log_origin"
	klogOrigin    = "klog"
	klogSourceKey = "klog_source"
)

func controllerRuntimeLogger(logger *slog.Logger) logr.Logger {
	return logr.FromSlogHandler(logger.Handler())
}

func configureKubernetesLogging(logger *slog.Logger) {
	slog.SetDefault(logger)

	controllerLogger := controllerRuntimeLogger(logger)
	ctrl.SetLogger(controllerLogger)

	klogLogger := logger.With(klogOriginKey, klogOrigin)
	klog.SetLoggerWithOptions(
		controllerRuntimeLogger(klogLogger),
		klog.ContextualLogger(true),
		klog.WriteKlogBuffer(newLegacyKlogWriter(klogLogger).WriteKlogBuffer),
	)
	klog.LogToStderr(false)
}

type legacyKlogWriter struct {
	logger *slog.Logger
}

func newLegacyKlogWriter(logger *slog.Logger) *legacyKlogWriter {
	return &legacyKlogWriter{logger: logger}
}

func (w *legacyKlogWriter) WriteKlogBuffer(data []byte) {
	level, source, msg := parseLegacyKlogRecord(data)
	if msg == "" {
		return
	}

	attrs := make([]slog.Attr, 0, 1)
	if source != "" {
		attrs = append(attrs, slog.String(klogSourceKey, source))
	}
	w.logger.LogAttrs(context.Background(), level, msg, attrs...)
}

func parseLegacyKlogRecord(data []byte) (slog.Level, string, string) {
	record := strings.TrimSpace(string(data))
	if record == "" {
		return slog.LevelInfo, "", ""
	}

	level := klogSeverity(record)
	separator := strings.Index(record, "] ")
	if separator == -1 {
		return level, "", record
	}

	header := record[:separator]
	msg := strings.TrimSpace(record[separator+2:])
	fields := strings.Fields(strings.TrimSpace(header[1:]))
	if len(fields) == 0 {
		return level, "", msg
	}

	source := fields[len(fields)-1]
	if !strings.Contains(source, ":") {
		source = ""
	}
	return level, source, msg
}

func klogSeverity(record string) slog.Level {
	if record == "" {
		return slog.LevelInfo
	}

	switch record[0] {
	case 'W':
		return slog.LevelWarn
	case 'E', 'F':
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
