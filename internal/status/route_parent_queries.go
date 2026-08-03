package status

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/mesh"
)

const (
	statusHTTPRouteGatewayParentIndex     = "nantian.dev/status.httproute.gateway-parents"
	statusGRPCRouteGatewayParentIndex     = "nantian.dev/status.grpcroute.gateway-parents"
	statusTCPRouteGatewayParentIndex      = "nantian.dev/status.tcproute.gateway-parents"
	statusUDPRouteGatewayParentIndex      = "nantian.dev/status.udproute.gateway-parents"
	statusTLSRouteGatewayParentIndex      = "nantian.dev/status.tlsroute.gateway-parents"
	statusHTTPRouteServiceParentIndex     = "nantian.dev/status.httproute.service-parents"
	statusGRPCRouteServiceParentIndex     = "nantian.dev/status.grpcroute.service-parents"
	statusTCPRouteServiceParentIndex      = "nantian.dev/status.tcproute.service-parents"
	statusUDPRouteServiceParentIndex      = "nantian.dev/status.udproute.service-parents"
	statusTLSRouteServiceParentIndex      = "nantian.dev/status.tlsroute.service-parents"
	statusHTTPRouteListenerSetParentIndex = "nantian.dev/status.httproute.listenerset-parents"
	statusServiceParentIndexMarker        = "__service_parent__"
	statusListenerSetParentIndexMarker    = "__listenerset_parent__"
)

func SetupIndexes(ctx context.Context, indexer client.FieldIndexer, options ...Options) error {
	opts := normalizeOptions(options)

	if err := indexer.IndexField(ctx, &gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys); err != nil {
		return fmt.Errorf("index HTTPRoute gateway parents: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1.GRPCRoute{}, statusGRPCRouteGatewayParentIndex, statusGRPCRouteGatewayParentIndexKeys); err != nil {
		return fmt.Errorf("index GRPCRoute gateway parents: %w", err)
	}
	if opts.EnableExperimentalGateway {
		if err := indexer.IndexField(ctx, &gatewayv1alpha2.TCPRoute{}, statusTCPRouteGatewayParentIndex, statusTCPRouteGatewayParentIndexKeys); err != nil {
			return fmt.Errorf("index TCPRoute gateway parents: %w", err)
		}
		if err := indexer.IndexField(ctx, &gatewayv1alpha2.UDPRoute{}, statusUDPRouteGatewayParentIndex, statusUDPRouteGatewayParentIndexKeys); err != nil {
			return fmt.Errorf("index UDPRoute gateway parents: %w", err)
		}
	}
	// TLSRoute support is declared in SupportedFeatureNameSet(); the
	// translator and status evaluator load TLS routes without any
	// experimental gate, so the index must be registered in standard mode
	// too. Skip silently when the TLSRoute CRD is not installed.
	if err := indexer.IndexField(ctx, &gatewayv1alpha2.TLSRoute{}, statusTLSRouteGatewayParentIndex, statusTLSRouteGatewayParentIndexKeys); err != nil && !meta.IsNoMatchError(err) {
		return fmt.Errorf("index TLSRoute gateway parents: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1.HTTPRoute{}, statusHTTPRouteServiceParentIndex, statusHTTPRouteServiceParentIndexKeys); err != nil {
		return fmt.Errorf("index HTTPRoute service parents: %w", err)
	}
	if opts.EnableExperimentalGateway {
		if err := indexer.IndexField(ctx, &gatewayv1.HTTPRoute{}, statusHTTPRouteListenerSetParentIndex, statusHTTPRouteListenerSetParentIndexKeys); err != nil {
			return fmt.Errorf("index HTTPRoute ListenerSet parents: %w", err)
		}
	}
	if err := indexer.IndexField(ctx, &gatewayv1.HTTPRoute{}, statusHTTPRouteBackendRefIndex, statusHTTPRouteBackendRefIndexKeys); err != nil {
		return fmt.Errorf("index HTTPRoute backend refs: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1.GRPCRoute{}, statusGRPCRouteServiceParentIndex, statusGRPCRouteServiceParentIndexKeys); err != nil {
		return fmt.Errorf("index GRPCRoute service parents: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1.GRPCRoute{}, statusGRPCRouteBackendRefIndex, statusGRPCRouteBackendRefIndexKeys); err != nil {
		return fmt.Errorf("index GRPCRoute backend refs: %w", err)
	}
	if opts.EnableExperimentalGateway {
		if err := indexer.IndexField(ctx, &gatewayv1alpha2.TCPRoute{}, statusTCPRouteServiceParentIndex, statusTCPRouteServiceParentIndexKeys); err != nil {
			return fmt.Errorf("index TCPRoute service parents: %w", err)
		}
		if err := indexer.IndexField(ctx, &gatewayv1alpha2.TCPRoute{}, statusTCPRouteBackendRefIndex, statusTCPRouteBackendRefIndexKeys); err != nil {
			return fmt.Errorf("index TCPRoute backend refs: %w", err)
		}
		if err := indexer.IndexField(ctx, &gatewayv1alpha2.UDPRoute{}, statusUDPRouteServiceParentIndex, statusUDPRouteServiceParentIndexKeys); err != nil {
			return fmt.Errorf("index UDPRoute service parents: %w", err)
		}
		if err := indexer.IndexField(ctx, &gatewayv1alpha2.UDPRoute{}, statusUDPRouteBackendRefIndex, statusUDPRouteBackendRefIndexKeys); err != nil {
			return fmt.Errorf("index UDPRoute backend refs: %w", err)
		}
	}
	if err := indexer.IndexField(ctx, &gatewayv1alpha2.TLSRoute{}, statusTLSRouteServiceParentIndex, statusTLSRouteServiceParentIndexKeys); err != nil && !meta.IsNoMatchError(err) {
		return fmt.Errorf("index TLSRoute service parents: %w", err)
	}
	if err := indexer.IndexField(ctx, &gatewayv1alpha2.TLSRoute{}, statusTLSRouteBackendRefIndex, statusTLSRouteBackendRefIndexKeys); err != nil && !meta.IsNoMatchError(err) {
		return fmt.Errorf("index TLSRoute backend refs: %w", err)
	}
	if err := setupPolicyTargetRefIndexes(ctx, indexer); err != nil {
		return err
	}
	return nil
}

func statusHTTPRouteGatewayParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.HTTPRoute)
	if !ok {
		return nil
	}
	return gatewayParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func statusGRPCRouteGatewayParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.GRPCRoute)
	if !ok {
		return nil
	}
	return gatewayParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func statusTCPRouteGatewayParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TCPRoute)
	if !ok {
		return nil
	}
	return gatewayParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func statusUDPRouteGatewayParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.UDPRoute)
	if !ok {
		return nil
	}
	return gatewayParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func statusTLSRouteGatewayParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TLSRoute)
	if !ok {
		return nil
	}
	return gatewayParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func statusHTTPRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.HTTPRoute)
	if !ok {
		return nil
	}
	return serviceParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func statusHTTPRouteListenerSetParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.HTTPRoute)
	if !ok {
		return nil
	}
	return listenerSetParentStatusIndexKeys(route.Spec.ParentRefs)
}

func statusGRPCRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.GRPCRoute)
	if !ok {
		return nil
	}
	return serviceParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func statusTCPRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TCPRoute)
	if !ok {
		return nil
	}
	return serviceParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func statusUDPRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.UDPRoute)
	if !ok {
		return nil
	}
	return serviceParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func statusTLSRouteServiceParentIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TLSRoute)
	if !ok {
		return nil
	}
	return serviceParentStatusIndexKeys(route.Spec.ParentRefs, route.Namespace)
}

func gatewayParentStatusIndexKeys(parentRefs []gatewayv1.ParentReference, defaultNamespace string) []string {
	if len(parentRefs) == 0 {
		return nil
	}

	values := make(map[string]struct{}, len(parentRefs))
	for _, parentRef := range parentRefs {
		if !isGatewayParentRefForIndex(parentRef) {
			continue
		}
		namespace := namespaceOrDefault(parentRef.Namespace, defaultNamespace)
		if namespace == "" || parentRef.Name == "" {
			continue
		}
		values[gatewayParentStatusIndexValue(namespace, string(parentRef.Name))] = struct{}{}
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

func gatewayParentStatusIndexValue(namespace, name string) string {
	return namespace + "/" + name
}

func isGatewayParentRefForIndex(parentRef gatewayv1.ParentReference) bool {
	group := stringOrEmpty(parentRef.Group)
	if group != "" && group != gatewayGroup {
		return false
	}
	kind := stringOrEmpty(parentRef.Kind)
	return kind == "" || kind == "Gateway"
}

func serviceParentStatusIndexKeys(
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
		values[statusServiceParentIndexMarker] = struct{}{}
		values[gatewayParentStatusIndexValue(key.Namespace, key.Name)] = struct{}{}
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

func listenerSetParentStatusIndexKeys(
	parentRefs []gatewayv1.ParentReference,
) []string {
	if len(parentRefs) == 0 {
		return nil
	}
	for _, ref := range parentRefs {
		if ref.Group != nil && string(*ref.Group) == gatewayv1.GroupName &&
			ref.Kind != nil && string(*ref.Kind) == "ListenerSet" {
			return []string{statusListenerSetParentIndexMarker}
		}
	}
	return nil
}

func listHTTPRoutesForGateway(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.HTTPRoute, error) {
	routes, _, err := listHTTPRoutesForGatewayScoped(ctx, reader, key)
	return routes, err
}

func listHTTPRoutesForGatewayScoped(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.HTTPRoute, bool, error) {
	var routes gatewayv1.HTTPRouteList
	scoped, err := listRoutesForGateway(ctx, reader, &routes, statusHTTPRouteGatewayParentIndex, key)
	if err != nil {
		return nil, false, err
	}
	return routes.Items, scoped, nil
}

func listGRPCRoutesForGateway(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.GRPCRoute, error) {
	routes, _, err := listGRPCRoutesForGatewayScoped(ctx, reader, key)
	return routes, err
}

func listGRPCRoutesForGatewayScoped(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1.GRPCRoute, bool, error) {
	var routes gatewayv1.GRPCRouteList
	scoped, err := listRoutesForGateway(ctx, reader, &routes, statusGRPCRouteGatewayParentIndex, key)
	if err != nil {
		return nil, false, err
	}
	return routes.Items, scoped, nil
}

func listTCPRoutesForGateway(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TCPRoute, error) {
	routes, _, err := listTCPRoutesForGatewayScoped(ctx, reader, key)
	return routes, err
}

func listTCPRoutesForGatewayScoped(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TCPRoute, bool, error) {
	var routes gatewayv1alpha2.TCPRouteList
	scoped, err := listRoutesForGateway(ctx, reader, &routes, statusTCPRouteGatewayParentIndex, key)
	if err != nil {
		return nil, false, err
	}
	return routes.Items, scoped, nil
}

func listUDPRoutesForGateway(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.UDPRoute, error) {
	routes, _, err := listUDPRoutesForGatewayScoped(ctx, reader, key)
	return routes, err
}

func listUDPRoutesForGatewayScoped(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.UDPRoute, bool, error) {
	var routes gatewayv1alpha2.UDPRouteList
	scoped, err := listRoutesForGateway(ctx, reader, &routes, statusUDPRouteGatewayParentIndex, key)
	if err != nil {
		return nil, false, err
	}
	return routes.Items, scoped, nil
}

func listTLSRoutesForGateway(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TLSRoute, error) {
	routes, _, err := listTLSRoutesForGatewayScoped(ctx, reader, key)
	return routes, err
}

func listTLSRoutesForGatewayScoped(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
) ([]gatewayv1alpha2.TLSRoute, bool, error) {
	var routes gatewayv1alpha2.TLSRouteList
	scoped, err := listRoutesForGateway(ctx, reader, &routes, statusTLSRouteGatewayParentIndex, key)
	if err != nil {
		return nil, false, err
	}
	return routes.Items, scoped, nil
}

func listHTTPRoutesWithServiceParents(
	ctx context.Context,
	reader client.Reader,
) ([]gatewayv1.HTTPRoute, bool, error) {
	var routes gatewayv1.HTTPRouteList
	scoped, err := listRoutesWithServiceParents(ctx, reader, &routes, statusHTTPRouteServiceParentIndex)
	return routes.Items, scoped, err
}

func listGRPCRoutesWithServiceParents(
	ctx context.Context,
	reader client.Reader,
) ([]gatewayv1.GRPCRoute, bool, error) {
	var routes gatewayv1.GRPCRouteList
	scoped, err := listRoutesWithServiceParents(ctx, reader, &routes, statusGRPCRouteServiceParentIndex)
	return routes.Items, scoped, err
}

func listTCPRoutesWithServiceParents(
	ctx context.Context,
	reader client.Reader,
) ([]gatewayv1alpha2.TCPRoute, bool, error) {
	var routes gatewayv1alpha2.TCPRouteList
	scoped, err := listRoutesWithServiceParents(ctx, reader, &routes, statusTCPRouteServiceParentIndex)
	return routes.Items, scoped, err
}

func listUDPRoutesWithServiceParents(
	ctx context.Context,
	reader client.Reader,
) ([]gatewayv1alpha2.UDPRoute, bool, error) {
	var routes gatewayv1alpha2.UDPRouteList
	scoped, err := listRoutesWithServiceParents(ctx, reader, &routes, statusUDPRouteServiceParentIndex)
	return routes.Items, scoped, err
}

func listTLSRoutesWithServiceParents(
	ctx context.Context,
	reader client.Reader,
) ([]gatewayv1alpha2.TLSRoute, bool, error) {
	var routes gatewayv1alpha2.TLSRouteList
	scoped, err := listRoutesWithServiceParents(ctx, reader, &routes, statusTLSRouteServiceParentIndex)
	return routes.Items, scoped, err
}

func listRoutesForGateway(
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

func listRoutesWithServiceParents(
	ctx context.Context,
	reader client.Reader,
	list client.ObjectList,
	field string,
) (bool, error) {
	err := reader.List(ctx, list, client.MatchingFields{field: statusServiceParentIndexMarker})
	if err == nil || !isMissingFieldIndexError(err) {
		return true, err
	}
	return false, reader.List(ctx, list)
}
