package status

import "time"

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
		MaxConcurrentReconciles:   5,
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
