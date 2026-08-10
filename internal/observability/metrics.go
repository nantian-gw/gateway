package observability

import (
	"net/http"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	defaultMetricsHandlerMaxRequestsInFlight = 4
	defaultMetricsHandlerTimeout             = 10 * time.Second
)

type Metrics struct {
	registry                              *prometheus.Registry
	BuildInfo                             *prometheus.GaugeVec
	BuildsTotal                           prometheus.Counter
	BuildFailures                         prometheus.Counter
	PublishedTotal                        prometheus.Counter
	LastBuildSuccess                      prometheus.Gauge
	SnapshotBuildDurationSeconds          prometheus.Histogram
	SnapshotResourceCount                 *prometheus.HistogramVec
	SnapshotListenerAttachedRoutes        prometheus.Histogram
	AdminAPIRequestsTotal                 *prometheus.CounterVec
	AdminAPIRequestDurationSeconds        *prometheus.HistogramVec
	XDSActiveStreams                      prometheus.Gauge
	XDSSnapshotFanoutCoalescedTotal       prometheus.Counter
	XDSStreamTerminationsTotal            *prometheus.CounterVec
	XDSStatusReportRejectionsTotal        *prometheus.CounterVec
	XDSSnapshotSendDurationSeconds        prometheus.Histogram
	XDSSnapshotSendTimeoutsTotal          prometheus.Counter
	XDSSnapshotAckTimeoutsTotal           prometheus.Counter
	XDSPublishAckLagSeconds               prometheus.Histogram
	XDSPublishNackLagSeconds              prometheus.Histogram
	NodeStatusPersistQueueDepth           prometheus.Gauge
	NodeStatusPersistPendingNodes         prometheus.Gauge
	NodeStatusPersistEnqueuedTotal        prometheus.Counter
	NodeStatusPersistDroppedTotal         prometheus.Counter
	NodeStatusPersistImmediateTotal       prometheus.Counter
	NodeStatusPersistDebouncedTotal       prometheus.Counter
	NodeStatusPersistFlushDurationSeconds prometheus.Histogram
	ReconcilerRunnerRunsTotal             prometheus.Counter
	ReconcilerRunnerFailuresTotal         prometheus.Counter
	ReconcilerRunnerLastRunSuccess        prometheus.Gauge
	ReconcilerRunnerRunDurationSeconds    *prometheus.HistogramVec
	ReconcilerRunnerQueueDepth            prometheus.Gauge
	ReconcilerRunnerTriggerEnqueuedTotal  prometheus.Counter
	ReconcilerRunnerTriggerDedupedTotal   prometheus.Counter
	ReconcilerRunnerTriggerSettledTotal   prometheus.Counter
	ReconcilerRunnerSettlePending         prometheus.Gauge
	ReconcilerRunnerRetriesScheduledTotal prometheus.Counter
	ReconcilerRunnerRetryPending          prometheus.Gauge
	MemAllocBytes                         prometheus.GaugeFunc
	MemHeapInuseBytes                     prometheus.GaugeFunc
	MemStackInuseBytes                    prometheus.GaugeFunc
	MemGCCPUFraction                      prometheus.GaugeFunc
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		registry: registry,
		BuildInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "nantian_gateway_build_info",
				Help: "Controlplane build information. Always 1; labels carry the module version, VCS revision, and Go version.",
			},
			[]string{"version", "revision", "go_version"},
		),
		BuildsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_snapshot_builds_total",
			Help: "Total number of snapshot rebuild attempts.",
		}),
		BuildFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_snapshot_build_failures_total",
			Help: "Total number of failed snapshot rebuilds.",
		}),
		PublishedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_snapshot_published_total",
			Help: "Total number of published snapshots.",
		}),
		LastBuildSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nantian_gateway_snapshot_last_build_success",
			Help: "1 if last build succeeded, 0 otherwise.",
		}),
		SnapshotBuildDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "nantian_gateway_snapshot_build_duration_seconds",
			Help:    "Duration of controlplane snapshot build attempts, including failed builds.",
			Buckets: prometheus.DefBuckets,
		}),
		SnapshotResourceCount: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "nantian_gateway_snapshot_resource_count",
				Help:    "Observed resource counts in successfully built snapshots partitioned by resource type.",
				Buckets: prometheus.ExponentialBuckets(1, 2, 12),
			},
			[]string{"resource"},
		),
		SnapshotListenerAttachedRoutes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "nantian_gateway_snapshot_listener_attached_routes",
			Help:    "Observed attached route fanout per listener across successfully built snapshots.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		}),
		AdminAPIRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "nantian_gateway_controlplane_admin_requests_total",
				Help: "Total number of controlplane admin API requests partitioned by method, normalized route, and status class.",
			},
			[]string{"method", "route", "status_class"},
		),
		AdminAPIRequestDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "nantian_gateway_controlplane_admin_request_duration_seconds",
				Help:    "Duration of controlplane admin API requests partitioned by method, normalized route, and status class.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "route", "status_class"},
		),
		XDSActiveStreams: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nantian_gateway_controlplane_xds_active_streams",
			Help: "Current number of connected dataplane xDS streams.",
		}),
		XDSSnapshotFanoutCoalescedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_xds_snapshot_fanout_coalesced_total",
			Help: "Total number of per-subscriber pending snapshots replaced by newer published snapshots because a dataplane xDS stream was not keeping up with publish fanout.",
		}),
		XDSStreamTerminationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "nantian_gateway_controlplane_xds_stream_terminations_total",
				Help: "Total number of controlplane xDS stream terminations partitioned by the reason the stream exited.",
			},
			[]string{"reason"},
		),
		XDSStatusReportRejectionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "nantian_gateway_controlplane_xds_status_report_rejections_total",
				Help: "Total number of controlplane dataplane status reports rejected before mutating node state, partitioned by the rejection reason.",
			},
			[]string{"reason"},
		),
		XDSSnapshotSendDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "nantian_gateway_controlplane_xds_snapshot_send_duration_seconds",
			Help:    "Duration of sending a snapshot to a single dataplane xDS stream, including timeout cases.",
			Buckets: prometheus.DefBuckets,
		}),
		XDSSnapshotSendTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_xds_snapshot_send_timeouts_total",
			Help: "Total number of dataplane xDS streams disconnected because snapshot sending timed out.",
		}),
		XDSSnapshotAckTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_xds_snapshot_ack_timeouts_total",
			Help: "Total number of dataplane xDS streams disconnected because no matching ACK or NACK arrived for the latest published snapshot in time.",
		}),
		XDSPublishAckLagSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "nantian_gateway_controlplane_xds_publish_ack_lag_seconds",
			Help:    "Latency between publishing a snapshot to a dataplane node and receiving the matching ACK.",
			Buckets: prometheus.DefBuckets,
		}),
		XDSPublishNackLagSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "nantian_gateway_controlplane_xds_publish_nack_lag_seconds",
			Help:    "Latency between publishing a snapshot to a dataplane node and receiving the matching NACK.",
			Buckets: prometheus.DefBuckets,
		}),
		NodeStatusPersistQueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nantian_gateway_controlplane_node_status_persist_queue_depth",
			Help: "Current number of distinct node status updates waiting to enter the persistence worker.",
		}),
		NodeStatusPersistPendingNodes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nantian_gateway_controlplane_node_status_persist_pending_nodes",
			Help: "Current number of distinct node status updates waiting in the debounce window before persistence.",
		}),
		NodeStatusPersistEnqueuedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_node_status_persist_enqueued_total",
			Help: "Total number of node status persistence updates accepted into the bounded backlog.",
		}),
		NodeStatusPersistDroppedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_node_status_persist_dropped_total",
			Help: "Total number of node status persistence updates dropped because the bounded backlog was full.",
		}),
		NodeStatusPersistImmediateTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_node_status_persist_immediate_total",
			Help: "Total number of immediate node status persistence updates accepted into the bounded backlog.",
		}),
		NodeStatusPersistDebouncedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_node_status_persist_debounced_total",
			Help: "Total number of debounced node status persistence updates accepted into the bounded backlog.",
		}),
		NodeStatusPersistFlushDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "nantian_gateway_controlplane_node_status_persist_flush_duration_seconds",
			Help:    "Duration of flushing debounced node status persistence batches.",
			Buckets: prometheus.DefBuckets,
		}),
		ReconcilerRunnerRunsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_runs_total",
			Help: "Total number of custom controlplane reconcile runner executions.",
		}),
		ReconcilerRunnerFailuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_failures_total",
			Help: "Total number of failed custom controlplane reconcile runner executions.",
		}),
		ReconcilerRunnerLastRunSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_last_run_success",
			Help: "1 if the last custom controlplane reconcile runner execution succeeded, 0 otherwise.",
		}),
		ReconcilerRunnerRunDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "nantian_gateway_controlplane_reconciler_runner_duration_seconds",
				Help:    "Duration of custom controlplane reconcile runner executions partitioned by reconcile scope.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"scope"},
		),
		ReconcilerRunnerQueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_queue_depth",
			Help: "Current queue depth for custom controlplane reconcile runner triggers.",
		}),
		ReconcilerRunnerTriggerEnqueuedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_triggers_enqueued_total",
			Help: "Total number of custom controlplane reconcile runner triggers accepted into the queue.",
		}),
		ReconcilerRunnerTriggerDedupedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_triggers_deduplicated_total",
			Help: "Total number of custom controlplane reconcile runner triggers dropped because the queue was already full.",
		}),
		ReconcilerRunnerTriggerSettledTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_triggers_settled_total",
			Help: "Total number of custom controlplane reconcile runner triggers routed through the settle window.",
		}),
		ReconcilerRunnerSettlePending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_settle_pending",
			Help: "1 if a delayed settle trigger is pending for the custom controlplane reconcile runner, 0 otherwise.",
		}),
		ReconcilerRunnerRetriesScheduledTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_retries_scheduled_total",
			Help: "Total number of failure-triggered retry runs scheduled for the custom controlplane reconcile runner.",
		}),
		ReconcilerRunnerRetryPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nantian_gateway_controlplane_reconciler_runner_retry_pending",
			Help: "1 if a failure-triggered retry is pending for the custom controlplane reconcile runner, 0 otherwise.",
		}),
	// Memory metrics with project-specific prefix
	MemAllocBytes: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "nantian_gw_controlplane_mem_alloc_bytes",
		Help: "Current heap allocation in bytes (runtime.MemStats.Alloc).",
	}, func() float64 {
		var s runtime.MemStats
		runtime.ReadMemStats(&s)
		return float64(s.Alloc)
	}),
	MemHeapInuseBytes: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "nantian_gw_controlplane_mem_heap_inuse_bytes",
		Help: "Heap memory in use in bytes (runtime.MemStats.HeapInuse).",
	}, func() float64 {
		var s runtime.MemStats
		runtime.ReadMemStats(&s)
		return float64(s.HeapInuse)
	}),
	MemStackInuseBytes: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "nantian_gw_controlplane_mem_stack_inuse_bytes",
		Help: "Stack memory in use in bytes (runtime.MemStats.StackInuse).",
	}, func() float64 {
		var s runtime.MemStats
		runtime.ReadMemStats(&s)
		return float64(s.StackInuse)
	}),
	MemGCCPUFraction: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "nantian_gw_controlplane_mem_gc_cpu_fraction",
		Help: "GC CPU fraction since latest GC (runtime.MemStats.GCCPUFraction).",
	}, func() float64 {
		var s runtime.MemStats
		runtime.ReadMemStats(&s)
		return float64(s.GCCPUFraction)
	}),
	}

	registry.MustRegister(
		m.BuildInfo,
		m.XDSActiveStreams,
		m.BuildsTotal,
		m.BuildFailures,
		m.PublishedTotal,
		m.LastBuildSuccess,
		m.SnapshotBuildDurationSeconds,
		m.SnapshotResourceCount,
		m.SnapshotListenerAttachedRoutes,
		m.AdminAPIRequestsTotal,
		m.AdminAPIRequestDurationSeconds,
		m.XDSSnapshotFanoutCoalescedTotal,
		m.XDSStreamTerminationsTotal,
		m.XDSStatusReportRejectionsTotal,
		m.XDSSnapshotSendDurationSeconds,
		m.XDSSnapshotSendTimeoutsTotal,
		m.XDSSnapshotAckTimeoutsTotal,
		m.XDSPublishAckLagSeconds,
		m.XDSPublishNackLagSeconds,
		m.NodeStatusPersistQueueDepth,
		m.NodeStatusPersistPendingNodes,
		m.NodeStatusPersistEnqueuedTotal,
		m.NodeStatusPersistDroppedTotal,
		m.NodeStatusPersistImmediateTotal,
		m.NodeStatusPersistDebouncedTotal,
		m.NodeStatusPersistFlushDurationSeconds,
		m.ReconcilerRunnerRunsTotal,
		m.ReconcilerRunnerFailuresTotal,
		m.ReconcilerRunnerLastRunSuccess,
		m.ReconcilerRunnerRunDurationSeconds,
		m.ReconcilerRunnerQueueDepth,
		m.ReconcilerRunnerTriggerEnqueuedTotal,
		m.ReconcilerRunnerTriggerDedupedTotal,
		m.ReconcilerRunnerTriggerSettledTotal,
		m.ReconcilerRunnerSettlePending,
		m.ReconcilerRunnerRetriesScheduledTotal,
		m.ReconcilerRunnerRetryPending,
		m.MemAllocBytes,
		m.MemHeapInuseBytes,
		m.MemStackInuseBytes,
		m.MemGCCPUFraction,
	)

	version, revision := readBuildIdentity()
	m.BuildInfo.WithLabelValues(version, revision, runtime.Version()).Set(1)

	return m
}

// readBuildIdentity returns the module version and VCS revision recorded in the
// binary. Go embeds these automatically when building from a module and a VCS
// checkout; missing values fall back to "unknown" so the metric always reports.
func readBuildIdentity() (version, revision string) {
	version, revision = "unknown", "unknown"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version, revision
	}
	if info.Main.Version != "" {
		version = info.Main.Version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			revision = setting.Value
		}
	}
	return version, revision
}

func Handler(metrics *Metrics) http.Handler {
	return metricsHandlerForGatherer(metricsGatherer(metrics))
}

func metricsGatherer(metrics *Metrics) prometheus.Gatherer {
	gatherers := []prometheus.Gatherer{}
	if metrics != nil && metrics.registry != nil {
		gatherers = append(gatherers, metrics.registry)
	}
	gatherers = append(
		gatherers,
		ctrlmetrics.Registry,
		defaultRuntimeCollectorsRegistry(),
	)
	return deduplicatingGatherer{gatherers: gatherers}
}

func metricsHandlerForGatherer(gatherer prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(gatherer, metricsHandlerOptions())
}

func metricsHandlerOptions() promhttp.HandlerOpts {
	return promhttp.HandlerOpts{
		MaxRequestsInFlight: defaultMetricsHandlerMaxRequestsInFlight,
		Timeout:             defaultMetricsHandlerTimeout,
	}
}

type deduplicatingGatherer struct {
	gatherers []prometheus.Gatherer
}

func (g deduplicatingGatherer) Gather() ([]*dto.MetricFamily, error) {
	seen := make(map[string]struct{})
	families := make([]*dto.MetricFamily, 0)
	var errs prometheus.MultiError

	for _, gatherer := range g.gatherers {
		if gatherer == nil {
			continue
		}
		metricFamilies, err := gatherer.Gather()
		if err != nil {
			errs.Append(err)
		}
		for _, family := range metricFamilies {
			if family == nil {
				continue
			}
			name := family.GetName()
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			families = append(families, family)
		}
	}

	return families, errs.MaybeUnwrap()
}

func defaultRuntimeCollectorsRegistry() *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return registry
}
