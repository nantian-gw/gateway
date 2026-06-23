package mesh

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	FrontendKindMetadataKey      = "nantian.dev/frontend-kind"
	FrontendNamespaceMetadataKey = "nantian.dev/frontend-namespace"
	FrontendNameMetadataKey      = "nantian.dev/frontend-name"
	FrontendPortMetadataKey      = "nantian.dev/frontend-port"

	ManagedServiceAnnotation      = "nantian.dev/mesh-frontend"
	ShadowServiceAnnotation       = "nantian.dev/mesh-shadow-service"
	ShadowServiceRoleLabel        = "nantian.dev/service-role"
	ShadowServiceRoleValue        = "mesh-backend-shadow"
	OriginalServiceNameLabel      = "nantian.dev/original-service-name"
	OriginalServiceNamespaceLabel = "nantian.dev/original-service-namespace"

	FrontendKindService = "Service"

	FrontendPortBase  int32 = 20000
	FrontendPortCount int32 = 10000
)

type ServiceParentKey struct {
	Namespace string
	Name      string
}

type ServiceFrontendPort struct {
	Namespace    string
	Name         string
	ServicePort  int32
	ListenPort   int32
	Protocol     string
	KubeProtocol corev1.Protocol
}

func ParentServiceRef(ref gatewayv1.ParentReference, defaultNamespace string) (ServiceParentKey, bool) {
	group := stringValue(ref.Group)
	if group != "" {
		return ServiceParentKey{}, false
	}

	kind := stringValue(ref.Kind)
	if kind != FrontendKindService {
		return ServiceParentKey{}, false
	}

	return ServiceParentKey{
		Namespace: namespaceOrDefault(ref.Namespace, defaultNamespace),
		Name:      string(ref.Name),
	}, true
}

func ExpandServiceFrontends(
	services []corev1.Service,
	parentKeys []ServiceParentKey,
) []ServiceFrontendPort {
	requested := make(map[string]struct{}, len(parentKeys))
	for _, key := range parentKeys {
		requested[serviceKey(key.Namespace, key.Name)] = struct{}{}
	}

	out := make([]ServiceFrontendPort, 0)
	portKeys := make([]string, 0)
	for _, service := range services {
		if _, ok := requested[serviceKey(service.Namespace, service.Name)]; !ok {
			continue
		}

		for _, port := range service.Spec.Ports {
			protocol := ListenerProtocolForServicePort(port)
			item := ServiceFrontendPort{
				Namespace:    service.Namespace,
				Name:         service.Name,
				ServicePort:  port.Port,
				Protocol:     protocol,
				KubeProtocol: port.Protocol,
			}
			out = append(out, item)
			portKeys = append(portKeys, item.Key())
		}
	}

	assignments := assignFrontendPorts(portKeys)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	for idx := range out {
		out[idx].ListenPort = assignments[out[idx].Key()]
	}

	return out
}

func ListenerProtocolForServicePort(port corev1.ServicePort) string {
	if port.Protocol == corev1.ProtocolUDP {
		return "UDP"
	}

	appProtocol := strings.ToLower(stringValue(port.AppProtocol))
	switch appProtocol {
	case "grpc", "grpcs":
		return "GRPC"
	case "http", "h2c", "kubernetes.io/h2c", "http2", "http/2", "ws", "kubernetes.io/ws":
		return "HTTP"
	case "https", "wss", "kubernetes.io/wss":
		return "TLS_PASSTHROUGH"
	}

	name := strings.ToLower(port.Name)
	switch {
	case strings.Contains(name, "grpc"):
		return "GRPC"
	case strings.Contains(name, "http"):
		return "HTTP"
	case strings.Contains(name, "https"), strings.Contains(name, "tls"):
		return "TLS_PASSTHROUGH"
	case port.Port == 443:
		return "TLS_PASSTHROUGH"
	default:
		return "TCP"
	}
}

func ShadowServiceName(namespace string, name string) string {
	const prefix = "nantian-gw-shadow-"
	const maxLen = 63

	base := prefix + name
	if len(base) <= maxLen {
		return base
	}

	sum := fnv.New32a()
	_, _ = sum.Write([]byte(namespace + "/" + name))
	suffix := fmt.Sprintf("%08x", sum.Sum32())
	trimmed := name[:maxLen-len(prefix)-len(suffix)-1]
	return prefix + trimmed + "-" + suffix
}

func (item ServiceFrontendPort) Key() string {
	return servicePortKey(item.Namespace, item.Name, item.ServicePort)
}

func (item ServiceFrontendPort) ListenerName() string {
	return fmt.Sprintf("mesh/%s/%s/%d", item.Namespace, item.Name, item.ListenPort)
}

func (item ServiceFrontendPort) Metadata() map[string]string {
	return map[string]string{
		FrontendKindMetadataKey:      FrontendKindService,
		FrontendNamespaceMetadataKey: item.Namespace,
		FrontendNameMetadataKey:      item.Name,
		FrontendPortMetadataKey:      strconv.Itoa(int(item.ServicePort)),
	}
}

func assignFrontendPorts(keys []string) map[string]int32 {
	seen := make(map[int32]string, len(keys))
	out := make(map[string]int32, len(keys))

	sort.Strings(keys)
	for _, key := range keys {
		port := FrontendPortBase + int32(hashString(key)%uint32(FrontendPortCount))
		for {
			if _, ok := seen[port]; !ok {
				seen[port] = key
				out[key] = port
				break
			}

			offset := (port - FrontendPortBase + 1) % FrontendPortCount
			port = FrontendPortBase + offset
		}
	}

	return out
}

func hashString(value string) uint32 {
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(value))
	return sum.Sum32()
}

func serviceKey(namespace string, name string) string {
	return namespace + "/" + name
}

func servicePortKey(namespace string, name string, port int32) string {
	return namespace + "/" + name + "/" + strconv.Itoa(int(port))
}

func namespaceOrDefault(namespace *gatewayv1.Namespace, defaultNamespace string) string {
	if namespace == nil || *namespace == "" {
		return defaultNamespace
	}
	return string(*namespace)
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
