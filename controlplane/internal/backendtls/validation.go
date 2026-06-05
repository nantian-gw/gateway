package backendtls

import (
	"fmt"
	"net/url"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type SubjectAltName struct {
	Type  string
	Value string
}

func ParseSubjectAltNames(
	items []gatewayv1.SubjectAltName,
) ([]SubjectAltName, error) {
	out := make([]SubjectAltName, 0, len(items))
	for index, item := range items {
		switch item.Type {
		case gatewayv1.HostnameSubjectAltNameType:
			if item.Hostname == "" {
				return nil, fmt.Errorf(
					"BackendTLSPolicy subjectAltNames[%d] must contain hostname when type is Hostname",
					index,
				)
			}
			if item.URI != "" {
				return nil, fmt.Errorf(
					"BackendTLSPolicy subjectAltNames[%d] must not set uri when type is Hostname",
					index,
				)
			}
			out = append(out, SubjectAltName{
				Type:  "Hostname",
				Value: string(item.Hostname),
			})
		case gatewayv1.URISubjectAltNameType:
			if item.URI == "" {
				return nil, fmt.Errorf(
					"BackendTLSPolicy subjectAltNames[%d] must contain uri when type is URI",
					index,
				)
			}
			if item.Hostname != "" {
				return nil, fmt.Errorf(
					"BackendTLSPolicy subjectAltNames[%d] must not set hostname when type is URI",
					index,
				)
			}
			if !isAbsoluteURI(string(item.URI)) {
				return nil, fmt.Errorf(
					"BackendTLSPolicy subjectAltNames[%d] must contain an absolute URI when type is URI",
					index,
				)
			}
			out = append(out, SubjectAltName{
				Type:  "URI",
				Value: string(item.URI),
			})
		default:
			return nil, fmt.Errorf(
				"BackendTLSPolicy subjectAltNames[%d] uses unsupported type %q",
				index,
				item.Type,
			)
		}
	}

	return out, nil
}

func isAbsoluteURI(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return false
	}

	return parsed.Opaque != "" || parsed.Host != "" || parsed.Path != ""
}
