package status

import "time"

const defaultMaxConcurrentReconciles = 5

type Options struct {
	EnableExperimentalGateway bool
	EnableAiGateway           bool
	MaxConcurrentReconciles   int
	RateLimiterBaseDelay      time.Duration
	RateLimiterMaxDelay       time.Duration
	RateLimiterQPS            int
	RateLimiterBucketSize     int
}

func defaultOptions() Options {
	return Options{
		EnableExperimentalGateway: true,
		MaxConcurrentReconciles:   defaultMaxConcurrentReconciles,
		RateLimiterBaseDelay:      200 * time.Millisecond,
		RateLimiterMaxDelay:       30 * time.Second,
		RateLimiterQPS:            10,
		RateLimiterBucketSize:     100,
	}
}

func normalizeOptions(options []Options) Options {
	if len(options) == 0 {
		return defaultOptions()
	}
	return options[0]
}

func (r *Reconciler) statusBatchWorkerLimit() int {
	if r == nil || r.options.MaxConcurrentReconciles <= 0 {
		return defaultMaxConcurrentReconciles
	}
	return r.options.MaxConcurrentReconciles
}
