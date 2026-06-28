package translator

import "github.com/prometheus/client_golang/prometheus"

var (
	metricTranslationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nantian_controlplane_translator_errors_total",
			Help: "Total translation errors by resource type.",
		},
		[]string{"resource"},
	)
	metricTranslationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "nantian_controlplane_translator_duration_seconds",
			Help:    "Translation duration by resource type.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"resource"},
	)
)

func init() {
	prometheus.MustRegister(metricTranslationErrors)
	prometheus.MustRegister(metricTranslationDuration)
}

func recordTranslationError(resource string) {
	metricTranslationErrors.WithLabelValues(resource).Inc()
}

func observeTranslationDuration(resource string, fn func() error) error {
	timer := prometheus.NewTimer(metricTranslationDuration.WithLabelValues(resource))
	defer timer.ObserveDuration()
	err := fn()
	if err != nil {
		metricTranslationErrors.WithLabelValues(resource).Inc()
	}
	return err
}
