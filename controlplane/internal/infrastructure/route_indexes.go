package infrastructure

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/controlplane/internal/mesh"
)

const (
	gatewayClassControllerNameIndex = "nantian.dev/infrastructure.gatewayclass.controller-name"
	gatewayGatewayClassNameIndex    = "nantian.dev/infrastructure.gateway.gatewayclass-name"
	httpRouteServiceParentIndex     = "nantian.dev/infrastructure.httproute.service-parents"
	grpcRouteServiceParentIndex     = "nantian.dev/infrastructure.grpcroute.service-parents"
	tcpRouteServiceParentIndex      = "nantian.dev/infrastructure.tcproute.service-parents"
	udpRouteServiceParentIndex      = "nantian.dev/infrastructure.udproute.service-parents"
	tlsRouteServiceParentIndex      = "nantian.dev/infrastructure.tlsroute.service-parents"
	serviceParentIndexMarker        = "__service_parent__"
)

func SetupIndexes(ctx context.Context, indexer client.FieldIndexer) error {
	if err := indexer.IndexField(ctx, &gatewayv1.GatewayClass{}, gatewayClassControllerNameIndex, gatewayClassControllerNameIndexKeys); err != nil {
		return fmt.Errorf("index GatewayClass controller name: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1.Gateway{}, gatewayGatewayClassNameIndex, gatewayGatewayClassNameIndexKeys); err != nil {
		return fmt.Errorf("index Gateway gatewayClassName: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1.HTTPRoute{}, httpRouteServiceParentIndex, httpRouteServiceParentIndexKeys); err != nil {
		return fmt.Errorf("index HTTPRoute service parents: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1.GRPCRoute{}, grpcRouteServiceParentIndex, grpcRouteServiceParentIndexKeys); err != nil {
		return fmt.Errorf("index GRPCRoute service parents: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1alpha2.TCPRoute{}, tcpRouteServiceParentIndex, tcpRouteServiceParentIndexKeys); err != nil {
		return fmt.Errorf("index TCPRoute service parents: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1alpha2.UDPRoute{}, udpRouteServiceParentIndex, udpRouteServiceParentIndexKeys); err != nil {
		return fmt.Errorf("index UDPRoute service parents: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1alpha2.TLSRoute{}, tlsRouteServiceParentIndex, tlsRouteServiceParentIndexKeys); err != nil {
		return fmt.Errorf("index TLSRoute service parents: %w", err)
	}

	return nil
}

func gatewayClassControllerNameIndexKeys(object client.Object) []string {
	gatewayClass, ok := object.(*gatewayv1.GatewayClass)
	if !ok {
		return nil
	}
	controllerName := string(gatewayClass.Spec.ControllerName)
	if controllerName == "" {
		return nil
	}
	return []string{controllerName}
}

func gatewayGatewayClassNameIndexKeys(object client.Object) []string {
	gateway, ok := object.(*gatewayv1.Gateway)
	if !ok {
		return nil
	}
	className := string(gateway.Spec.GatewayClassName)
	if className == "" {
		return nil
	}
	return []string{className}
}

func httpRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.HTTPRoute)
	if !ok {
		return nil
	}
	return serviceParentIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func grpcRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.GRPCRoute)
	if !ok {
		return nil
	}
	return serviceParentIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func tcpRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TCPRoute)
	if !ok {
		return nil
	}
	return serviceParentIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func udpRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.UDPRoute)
	if !ok {
		return nil
	}
	return serviceParentIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func tlsRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TLSRoute)
	if !ok {
		return nil
	}
	return serviceParentIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func serviceParentIndexKeys(
	parentRefs []gatewayv1.ParentReference,
	defaultNamespace string,
) []string {
	if len(parentRefs) == 0 {
		return nil
	}

	values := make(map[string]struct{}, len(parentRefs)+1)
	for _, parentRef := range parentRefs {
		key, ok := mesh.ParentServiceRef(parentRef, defaultNamespace)
		if !ok {
			continue
		}
		values[serviceParentIndexMarker] = struct{}{}
		values[serviceKey(key.Namespace, key.Name)] = struct{}{}
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

func listHTTPRoutesWithServiceParents(ctx context.Context, cl client.Client) ([]gatewayv1.HTTPRoute, error) {
	var routes gatewayv1.HTTPRouteList
	if err := listWithServiceParentIndex(ctx, cl, &routes, httpRouteServiceParentIndex); err != nil {
		return nil, err
	}
	return routes.Items, nil
}

func listGRPCRoutesWithServiceParents(ctx context.Context, cl client.Client) ([]gatewayv1.GRPCRoute, error) {
	var routes gatewayv1.GRPCRouteList
	if err := listWithServiceParentIndex(ctx, cl, &routes, grpcRouteServiceParentIndex); err != nil {
		return nil, err
	}
	return routes.Items, nil
}

func listTCPRoutesWithServiceParents(ctx context.Context, cl client.Client) ([]gatewayv1alpha2.TCPRoute, error) {
	var routes gatewayv1alpha2.TCPRouteList
	if err := listWithServiceParentIndex(ctx, cl, &routes, tcpRouteServiceParentIndex); err != nil {
		return nil, err
	}
	return routes.Items, nil
}

func listUDPRoutesWithServiceParents(ctx context.Context, cl client.Client) ([]gatewayv1alpha2.UDPRoute, error) {
	var routes gatewayv1alpha2.UDPRouteList
	if err := listWithServiceParentIndex(ctx, cl, &routes, udpRouteServiceParentIndex); err != nil {
		return nil, err
	}
	return routes.Items, nil
}

func listTLSRoutesWithServiceParents(ctx context.Context, cl client.Client) ([]gatewayv1alpha2.TLSRoute, error) {
	var routes gatewayv1alpha2.TLSRouteList
	if err := listWithServiceParentIndex(ctx, cl, &routes, tlsRouteServiceParentIndex); err != nil {
		return nil, err
	}
	return routes.Items, nil
}

func listWithServiceParentIndex(
	ctx context.Context,
	cl client.Client,
	list client.ObjectList,
	field string,
) error {
	err := cl.List(ctx, list, client.MatchingFields{field: serviceParentIndexMarker})
	if err == nil {
		return nil
	}
	if isMissingFieldIndexError(err) {
		return requiredFieldIndexError("Route", field, err)
	}
	return err
}

func isMissingFieldIndexError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "no index with name") ||
		(strings.Contains(message, "specifies selector on field") && strings.Contains(message, "has been registered")) ||
		strings.Contains(message, "field label not supported")
}

func requiredFieldIndexError(kind, field string, err error) error {
	return fmt.Errorf("%s query requires field index %q: %w", kind, field, err)
}
