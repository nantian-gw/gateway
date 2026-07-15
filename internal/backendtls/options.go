package backendtls

import (
	"fmt"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	OptionMinVersion = "gateway.nantian.dev/backend-tls-min-version"
	OptionMaxVersion = "gateway.nantian.dev/backend-tls-max-version"

	VersionTLS12 = "TLS1_2"
	VersionTLS13 = "TLS1_3"
)

type Options struct {
	MinVersion string
	MaxVersion string
}

func ParseOptions(raw map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue) (Options, error) {
	if len(raw) == 0 {
		return Options{}, nil
	}

	if _, ok := raw[gatewayv1.AnnotationKey(OptionMinVersion)]; ok {
		return Options{}, fmt.Errorf(
			"BackendTLSPolicy option %q is not supported by the upstream runtime",
			OptionMinVersion,
		)
	}
	if _, ok := raw[gatewayv1.AnnotationKey(OptionMaxVersion)]; ok {
		return Options{}, fmt.Errorf(
			"BackendTLSPolicy option %q is not supported by the upstream runtime",
			OptionMaxVersion,
		)
	}

	for key := range raw {
		return Options{}, fmt.Errorf("unsupported BackendTLSPolicy option %q", key)
	}

	return Options{}, nil
}
