package translator

import (
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
)

type backendService struct {
	namespace   string
	logicalName string
	service     corev1.Service
}

func collectMeshServiceFrontends(
	services []corev1.Service,
	httpRoutes []gatewayv1.HTTPRoute,
	grpcRoutes []gatewayv1.GRPCRoute,
	tcpRoutes []gatewayv1alpha2.TCPRoute,
	udpRoutes []gatewayv1alpha2.UDPRoute,
	tlsRoutes []gatewayv1alpha2.TLSRoute,
) []mesh.ServiceFrontendPort {
	keys := make([]mesh.ServiceParentKey, 0)

	for _, route := range httpRoutes {
		keys = append(keys, collectMeshParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}
	for _, route := range grpcRoutes {
		keys = append(keys, collectMeshParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}
	for _, route := range tcpRoutes {
		keys = append(keys, collectMeshParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}
	for _, route := range udpRoutes {
		keys = append(keys, collectMeshParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}
	for _, route := range tlsRoutes {
		keys = append(keys, collectMeshParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}

	return mesh.ExpandServiceFrontends(services, keys)
}

func collectMeshParentKeys(
	parentRefs []gatewayv1.ParentReference,
	defaultNamespace string,
) []mesh.ServiceParentKey {
	out := make([]mesh.ServiceParentKey, 0, len(parentRefs))
	for _, parentRef := range parentRefs {
		if key, ok := mesh.ParentServiceRef(parentRef, defaultNamespace); ok {
			out = append(out, key)
		}
	}
	return out
}

func translateMeshServiceListeners(frontends []mesh.ServiceFrontendPort) []ir.Listener {
	out := make([]ir.Listener, 0, len(frontends))
	for _, frontend := range frontends {
		out = append(out, ir.Listener{
			Name:     frontend.ListenerName(),
			Address:  "0.0.0.0",
			Port:     uint32(frontend.ListenPort),
			Protocol: frontend.Protocol,
			Metadata: frontend.Metadata(),
		})
	}
	return out
}

func effectiveBackendServices(services []corev1.Service) []backendService {
	shadowByName := make(map[string]corev1.Service)
	shadowByOriginal := make(map[string]corev1.Service)
	for _, service := range services {
		if service.Labels[mesh.ShadowServiceRoleLabel] != mesh.ShadowServiceRoleValue {
			continue
		}

		shadowByName[service.Namespace+"/"+service.Name] = service
		originalNamespace := service.Labels[mesh.OriginalServiceNamespaceLabel]
		originalName := service.Labels[mesh.OriginalServiceNameLabel]
		if originalNamespace != "" && originalName != "" {
			shadowByOriginal[originalNamespace+"/"+originalName] = service
		}
	}

	out := make([]backendService, 0, len(services))
	for _, service := range services {
		if service.Labels[mesh.ShadowServiceRoleLabel] == mesh.ShadowServiceRoleValue {
			continue
		}

		actual := service
		if service.Annotations[mesh.ManagedServiceAnnotation] == "true" {
			shadowName := service.Annotations[mesh.ShadowServiceAnnotation]
			if shadow, ok := shadowByName[service.Namespace+"/"+shadowName]; ok {
				actual = shadow
			} else if shadow, ok := shadowByOriginal[service.Namespace+"/"+service.Name]; ok {
				actual = shadow
			}
		}

		out = append(out, backendService{
			namespace:   service.Namespace,
			logicalName: service.Name,
			service:     actual,
		})
	}

	return out
}
