package listeners

import (
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
)

func CollectMeshServiceFrontends(
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

func TranslateMeshServiceListeners(frontends []mesh.ServiceFrontendPort) []ir.Listener {
	out := make([]ir.Listener, 0, len(frontends))
	for _, frontend := range frontends {
		out = append(out, ir.Listener{
			Name:     frontend.ListenerName(),
			Address:  "0.0.0.0",
			Port:     uint32(frontend.ListenPort), //nolint:gosec
			Protocol: frontend.Protocol,
			Metadata: frontend.Metadata(),
		})
	}
	return out
}
