package translator

import (
	discoveryv1 "k8s.io/api/discovery/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func hostnames[T ~string](items []T) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item))
	}
	return out
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}

	return string(*value)
}

func namespaceOrDefault[T ~string](value *T, defaultNamespace string) string {
	if value == nil || string(*value) == "" {
		return defaultNamespace
	}

	return string(*value)
}

func portValue[T ~int32](value *T) uint32 {
	if value == nil {
		return 0
	}
	return uint32(*value)
}

func weightValue[T ~int32](value *T) int32 {
	if value == nil {
		return 1
	}
	return int32(*value)
}

func gatewayParents(refs []gatewayv1.ParentReference, defaultNamespace string) []ir.ParentRef {
	out := make([]ir.ParentRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ir.ParentRef{
			Group:       stringValue(ref.Group),
			Kind:        stringValue(ref.Kind),
			Namespace:   namespaceOrDefault(ref.Namespace, defaultNamespace),
			Name:        string(ref.Name),
			SectionName: stringValue(ref.SectionName),
			Port:        portValue(ref.Port),
		})
	}
	return out
}

func selectSlicePort(ports []discoveryv1.EndpointPort, servicePortName string, servicePort int32) *discoveryv1.EndpointPort {
	for _, port := range ports {
		if port.Name != nil && *port.Name == servicePortName {
			return &port
		}
	}
	for _, port := range ports {
		if port.Port != nil && *port.Port == servicePort {
			return &port
		}
	}
	if len(ports) > 0 {
		return &ports[0]
	}
	return nil
}
