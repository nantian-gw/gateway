package status

import (
	"context"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/mesh"
)

const (
	statusHTTPRouteBackendRefIndex = "nantian.dev/status.httproute.backend-refs"
	statusGRPCRouteBackendRefIndex = "nantian.dev/status.grpcroute.backend-refs"
	statusTCPRouteBackendRefIndex  = "nantian.dev/status.tcproute.backend-refs"
	statusUDPRouteBackendRefIndex  = "nantian.dev/status.udproute.backend-refs"
	statusTLSRouteBackendRefIndex  = "nantian.dev/status.tlsroute.backend-refs"
)

func statusHTTPRouteBackendRefIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.HTTPRoute)
	if !ok {
		return nil
	}
	return backendRefStatusIndexKeys(httpRouteInput(*route))
}

func statusGRPCRouteBackendRefIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.GRPCRoute)
	if !ok {
		return nil
	}
	return backendRefStatusIndexKeys(grpcRouteInput(*route))
}

func statusTCPRouteBackendRefIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TCPRoute)
	if !ok {
		return nil
	}
	return backendRefStatusIndexKeys(tcpRouteInput(*route))
}

func statusUDPRouteBackendRefIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.UDPRoute)
	if !ok {
		return nil
	}
	return backendRefStatusIndexKeys(udpRouteInput(*route))
}

func statusTLSRouteBackendRefIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TLSRoute)
	if !ok {
		return nil
	}
	return backendRefStatusIndexKeys(tlsRouteInput(*route))
}

func backendRefStatusIndexKeys(route routeInput) []string {
	if len(route.backends) == 0 {
		return nil
	}

	values := make(map[string]struct{}, len(route.backends))
	for _, backend := range route.backends {
		kind, ok := backendKindForStatus(backend.Group, backend.Kind)
		if !ok {
			continue
		}

		namespace := backend.Namespace
		if namespace == "" {
			namespace = route.namespace
		}
		if namespace == "" || backend.Name == "" {
			continue
		}

		values[backendRefStatusIndexValue(kind, namespace, backend.Name)] = struct{}{}
	}
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func backendRefStatusIndexValue(kind, namespace, name string) string {
	return kind + "/" + namespace + "/" + name
}

func listHTTPRoutesForServiceBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.HTTPRoute, bool, error) {
	return listHTTPRoutesForBackendRef(ctx, reader, "Service", key)
}

func listHTTPRoutesForServiceImportBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.HTTPRoute, bool, error) {
	return listHTTPRoutesForBackendRef(ctx, reader, "ServiceImport", key)
}

func listHTTPRoutesForServiceParent(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.HTTPRoute, bool, error) {
	var routes gatewayv1.HTTPRouteList
	scoped, err := listRoutesForServiceParent(ctx, reader, &routes, statusHTTPRouteServiceParentIndex, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1.HTTPRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesServiceParent(httpRouteInput(route), key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listGRPCRoutesForServiceBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.GRPCRoute, bool, error) {
	return listGRPCRoutesForBackendRef(ctx, reader, "Service", key)
}

func listGRPCRoutesForServiceImportBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.GRPCRoute, bool, error) {
	return listGRPCRoutesForBackendRef(ctx, reader, "ServiceImport", key)
}

func listGRPCRoutesForServiceParent(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.GRPCRoute, bool, error) {
	var routes gatewayv1.GRPCRouteList
	scoped, err := listRoutesForServiceParent(ctx, reader, &routes, statusGRPCRouteServiceParentIndex, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1.GRPCRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesServiceParent(grpcRouteInput(route), key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listTCPRoutesForServiceBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TCPRoute, bool, error) {
	return listTCPRoutesForBackendRef(ctx, reader, "Service", key)
}

func listTCPRoutesForServiceImportBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TCPRoute, bool, error) {
	return listTCPRoutesForBackendRef(ctx, reader, "ServiceImport", key)
}

func listTCPRoutesForServiceParent(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TCPRoute, bool, error) {
	var routes gatewayv1alpha2.TCPRouteList
	scoped, err := listRoutesForServiceParent(ctx, reader, &routes, statusTCPRouteServiceParentIndex, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1alpha2.TCPRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesServiceParent(tcpRouteInput(route), key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listUDPRoutesForServiceBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.UDPRoute, bool, error) {
	return listUDPRoutesForBackendRef(ctx, reader, "Service", key)
}

func listUDPRoutesForServiceImportBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.UDPRoute, bool, error) {
	return listUDPRoutesForBackendRef(ctx, reader, "ServiceImport", key)
}

func listUDPRoutesForServiceParent(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.UDPRoute, bool, error) {
	var routes gatewayv1alpha2.UDPRouteList
	scoped, err := listRoutesForServiceParent(ctx, reader, &routes, statusUDPRouteServiceParentIndex, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1alpha2.UDPRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesServiceParent(udpRouteInput(route), key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listTLSRoutesForServiceBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TLSRoute, bool, error) {
	return listTLSRoutesForBackendRef(ctx, reader, "Service", key)
}

func listTLSRoutesForServiceImportBackend(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TLSRoute, bool, error) {
	return listTLSRoutesForBackendRef(ctx, reader, "ServiceImport", key)
}

func listTLSRoutesForServiceParent(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TLSRoute, bool, error) {
	var routes gatewayv1alpha2.TLSRouteList
	scoped, err := listRoutesForServiceParent(ctx, reader, &routes, statusTLSRouteServiceParentIndex, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1alpha2.TLSRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesServiceParent(tlsRouteInput(route), key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listHTTPRoutesForBackendRef(
	ctx context.Context,
	reader client.Reader,
	backendKind string,
	key client.ObjectKey,
) ([]gatewayv1.HTTPRoute, bool, error) {
	var routes gatewayv1.HTTPRouteList
	scoped, err := listRoutesForBackendRef(ctx, reader, &routes, statusHTTPRouteBackendRefIndex, backendKind, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1.HTTPRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesBackendRef(httpRouteInput(route), backendKind, key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listGRPCRoutesForBackendRef(
	ctx context.Context,
	reader client.Reader,
	backendKind string,
	key client.ObjectKey,
) ([]gatewayv1.GRPCRoute, bool, error) {
	var routes gatewayv1.GRPCRouteList
	scoped, err := listRoutesForBackendRef(ctx, reader, &routes, statusGRPCRouteBackendRefIndex, backendKind, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1.GRPCRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesBackendRef(grpcRouteInput(route), backendKind, key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listTCPRoutesForBackendRef(
	ctx context.Context,
	reader client.Reader,
	backendKind string,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TCPRoute, bool, error) {
	var routes gatewayv1alpha2.TCPRouteList
	scoped, err := listRoutesForBackendRef(ctx, reader, &routes, statusTCPRouteBackendRefIndex, backendKind, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1alpha2.TCPRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesBackendRef(tcpRouteInput(route), backendKind, key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listUDPRoutesForBackendRef(
	ctx context.Context,
	reader client.Reader,
	backendKind string,
	key client.ObjectKey,
) ([]gatewayv1alpha2.UDPRoute, bool, error) {
	var routes gatewayv1alpha2.UDPRouteList
	scoped, err := listRoutesForBackendRef(ctx, reader, &routes, statusUDPRouteBackendRefIndex, backendKind, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1alpha2.UDPRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesBackendRef(udpRouteInput(route), backendKind, key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listTLSRoutesForBackendRef(
	ctx context.Context,
	reader client.Reader,
	backendKind string,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TLSRoute, bool, error) {
	var routes gatewayv1alpha2.TLSRouteList
	scoped, err := listRoutesForBackendRef(ctx, reader, &routes, statusTLSRouteBackendRefIndex, backendKind, key)
	if err != nil {
		return nil, scoped, err
	}
	if scoped {
		return routes.Items, true, nil
	}
	out := make([]gatewayv1alpha2.TLSRoute, 0, len(routes.Items))
	for _, route := range routes.Items {
		if routeUsesBackendRef(tlsRouteInput(route), backendKind, key) {
			out = append(out, route)
		}
	}
	return out, false, nil
}

func listRoutesForServiceParent(
	ctx context.Context,
	reader client.Reader,
	list client.ObjectList,
	field string,
	key client.ObjectKey,
) (bool, error) {
	indexValue := gatewayParentStatusIndexValue(key.Namespace, key.Name)
	err := reader.List(ctx, list, client.MatchingFields{field: indexValue})
	if err == nil || !isMissingFieldIndexError(err) {
		return true, err
	}
	return false, reader.List(ctx, list)
}

func listRoutesForBackendRef(
	ctx context.Context,
	reader client.Reader,
	list client.ObjectList,
	field string,
	backendKind string,
	key client.ObjectKey,
) (bool, error) {
	indexValue := backendRefStatusIndexValue(backendKind, key.Namespace, key.Name)
	err := reader.List(ctx, list, client.MatchingFields{field: indexValue})
	if err == nil || !isMissingFieldIndexError(err) {
		return true, err
	}
	return false, reader.List(ctx, list)
}

func routeUsesServiceParent(route routeInput, key client.ObjectKey) bool {
	for _, parentRef := range route.parentRefs {
		parentKey, ok := mesh.ParentServiceRef(parentRef, route.namespace)
		if !ok {
			continue
		}
		if parentKey.Namespace == key.Namespace && parentKey.Name == key.Name {
			return true
		}
	}
	return false
}

func routeUsesBackendRef(route routeInput, backendKind string, key client.ObjectKey) bool {
	for _, backend := range route.backends {
		kind, ok := backendKindForStatus(backend.Group, backend.Kind)
		if !ok || kind != backendKind {
			continue
		}

		namespace := backend.Namespace
		if namespace == "" {
			namespace = route.namespace
		}
		if namespace == key.Namespace && backend.Name == key.Name {
			return true
		}
	}
	return false
}
