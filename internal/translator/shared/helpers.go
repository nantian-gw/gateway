package shared

import (
	discoveryv1 "k8s.io/api/discovery/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/ir"
)

func BackendObjectKey(namespace string, name string) string {
	return namespace + "/" + name
}

func Hostnames[T ~string](items []T) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item))
	}
	return out
}

func StringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}

	return string(*value)
}

func NamespaceOrDefault[T ~string](value *T, defaultNamespace string) string {
	if value == nil || string(*value) == "" {
		return defaultNamespace
	}

	return string(*value)
}

func PortValue[T ~int32](value *T) uint32 {
	if value == nil {
		return 0
	}
	return uint32(*value)
}

func WeightValue[T ~int32](value *T) int32 {
	if value == nil {
		return 1
	}
	return int32(*value)
}

func GatewayParents(refs []gatewayv1.ParentReference, defaultNamespace string) []ir.ParentRef {
	out := make([]ir.ParentRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ir.ParentRef{
			Group:       StringValue(ref.Group),
			Kind:        StringValue(ref.Kind),
			Namespace:   NamespaceOrDefault(ref.Namespace, defaultNamespace),
			Name:        string(ref.Name),
			SectionName: StringValue(ref.SectionName),
			Port:        PortValue(ref.Port),
		})
	}
	return out
}

func SelectSlicePort(ports []discoveryv1.EndpointPort, servicePortName string, servicePort int32) *discoveryv1.EndpointPort {
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
		selected := 0
		for idx := 1; idx < len(ports); idx++ {
			if endpointPortLess(ports[idx], ports[selected]) {
				selected = idx
			}
		}
		return &ports[selected]
	}
	return nil
}

func endpointPortLess(left, right discoveryv1.EndpointPort) bool {
	leftName, rightName := endpointPortName(left), endpointPortName(right)
	if leftName != rightName {
		return leftName < rightName
	}
	leftPort, leftHasPort := endpointPortNumber(left)
	rightPort, rightHasPort := endpointPortNumber(right)
	if leftHasPort != rightHasPort {
		return !leftHasPort
	}
	if leftPort != rightPort {
		return leftPort < rightPort
	}
	return endpointPortProtocol(left) < endpointPortProtocol(right)
}

func endpointPortName(port discoveryv1.EndpointPort) string {
	if port.Name == nil {
		return ""
	}
	return *port.Name
}

func endpointPortNumber(port discoveryv1.EndpointPort) (int32, bool) {
	if port.Port == nil {
		return 0, false
	}
	return *port.Port, true
}

func endpointPortProtocol(port discoveryv1.EndpointPort) string {
	if port.Protocol == nil {
		return ""
	}
	return string(*port.Protocol)
}
