package status

// Options controls optional Gateway API runtime integrations.
type Options struct {
	EnableExperimentalGateway bool
}

func defaultOptions() Options {
	return Options{EnableExperimentalGateway: true}
}

func normalizeOptions(options []Options) Options {
	if len(options) == 0 {
		return defaultOptions()
	}
	return options[0]
}
