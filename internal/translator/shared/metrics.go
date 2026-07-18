package shared

import "github.com/prometheus/client_golang/prometheus"

var (
	MetricTranslationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nantian_controlplane_translator_errors_total",
			Help: "Total translation errors by resource type.",
		},
		[]string{"resource"},
	)
	MetricTranslationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "nantian_controlplane_translator_duration_seconds",
			Help:    "Translation duration by resource type.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"resource"},
	)
)

func init() {
	prometheus.MustRegister(MetricTranslationErrors)
	prometheus.MustRegister(MetricTranslationDuration)
}

func RecordTranslationError(resource string) {
	MetricTranslationErrors.WithLabelValues(resource).Inc()
}

func ObserveTranslationDuration(resource string, fn func() error) error {
	timer := prometheus.NewTimer(MetricTranslationDuration.WithLabelValues(resource))
	defer timer.ObserveDuration()
	err := fn()
	if err != nil {
		MetricTranslationErrors.WithLabelValues(resource).Inc()
	}
	return err
}
