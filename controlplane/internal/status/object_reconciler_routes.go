package status

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/extensionfilter"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapi"
)

func (r *Reconciler) ReconcileHTTPRouteObject(ctx context.Context, key client.ObjectKey) error {
	var current gatewayv1.HTTPRoute
	if err := r.reader.Get(ctx, key, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.reconcileRouteObject(ctx, key, httpRouteInput(current), r.reconcileHTTPRouteStatus)
}

func (r *Reconciler) ReconcileGRPCRouteObject(ctx context.Context, key client.ObjectKey) error {
	var current gatewayv1.GRPCRoute
	if err := r.reader.Get(ctx, key, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.reconcileRouteObject(ctx, key, grpcRouteInput(current), r.reconcileGRPCRouteStatus)
}

func (r *Reconciler) ReconcileTCPRouteObject(ctx context.Context, key client.ObjectKey) error {
	var current gatewayv1alpha2.TCPRoute
	if err := r.reader.Get(ctx, key, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.reconcileRouteObject(ctx, key, tcpRouteInput(current), r.reconcileTCPRouteStatus)
}

func (r *Reconciler) ReconcileUDPRouteObject(ctx context.Context, key client.ObjectKey) error {
	var current gatewayv1alpha2.UDPRoute
	if err := r.reader.Get(ctx, key, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.reconcileRouteObject(ctx, key, udpRouteInput(current), r.reconcileUDPRouteStatus)
}

func (r *Reconciler) ReconcileTLSRouteObject(ctx context.Context, key client.ObjectKey) error {
	var current gatewayv1alpha2.TLSRoute
	if err := r.reader.Get(ctx, key, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.reconcileRouteObject(ctx, key, tlsRouteInput(current), r.reconcileTLSRouteStatus)
}

func (r *Reconciler) reconcileRouteObject(
	ctx context.Context,
	key client.ObjectKey,
	route routeInput,
	updateStatus routeStatusUpdater,
) error {
	state, err := r.loadRouteObjectState(ctx, route)
	if err != nil {
		return err
	}

	return updateStatus(ctx, key, evaluateRoute(state, route))
}

func (r *Reconciler) loadRouteObjectState(ctx context.Context, route routeInput) (*clusterState, error) {
	state := r.newClusterState()

	if err := r.loadRouteParentGateways(ctx, state, route); err != nil {
		return nil, err
	}
	if err := r.loadGatewayClassesForLoadedGateways(ctx, state); err != nil {
		return nil, err
	}
	if err := r.loadRouteServices(ctx, state, route); err != nil {
		return nil, err
	}
	if err := r.loadRouteServiceImports(ctx, state, route); err != nil {
		return nil, err
	}
	if err := r.loadNamespace(ctx, state, route.namespace); err != nil {
		return nil, err
	}
	if err := r.loadRouteExtensionConfigMaps(ctx, state, route); err != nil {
		return nil, err
	}
	if err := r.loadRouteReferenceGrants(ctx, state, route); err != nil {
		return nil, err
	}

	state.index()
	return state, nil
}

func (r *Reconciler) listAllRoutes(ctx context.Context, state *clusterState) error {
	var httpRoutes gatewayv1.HTTPRouteList
	if err := r.reader.List(ctx, &httpRoutes); err != nil {
		return err
	}
	state.httpRoutes = httpRoutes.Items

	var grpcRoutes gatewayv1.GRPCRouteList
	if err := r.reader.List(ctx, &grpcRoutes); err != nil {
		return err
	}
	state.grpcRoutes = grpcRoutes.Items

	var tcpRoutes gatewayv1alpha2.TCPRouteList
	if err := r.reader.List(ctx, &tcpRoutes); err != nil {
		return err
	}
	state.tcpRoutes = tcpRoutes.Items

	var udpRoutes gatewayv1alpha2.UDPRouteList
	if err := r.reader.List(ctx, &udpRoutes); err != nil {
		return err
	}
	state.udpRoutes = udpRoutes.Items

	var tlsRoutes gatewayv1alpha2.TLSRouteList
	if err := r.reader.List(ctx, &tlsRoutes); err != nil {
		return err
	}
	state.tlsRoutes = tlsRoutes.Items

	return nil
}

func (r *Reconciler) loadRouteParentGateways(
	ctx context.Context,
	state *clusterState,
	route routeInput,
) error {
	keys := make(map[string]client.ObjectKey)
	for _, parentRef := range route.parentRefs {
		if isServiceParentRef(parentRef) {
			continue
		}

		key := client.ObjectKey{
			Namespace: namespaceOrDefault(parentRef.Namespace, route.namespace),
			Name:      string(parentRef.Name),
		}
		keys[namespacedName(key.Namespace, key.Name)] = key
	}

	for _, key := range keys {
		var gateway gatewayv1.Gateway
		found, err := r.getOptional(ctx, key, &gateway)
		if err != nil {
			return err
		}
		if found {
			state.gateways = append(state.gateways, gateway)
		}
	}

	if gatewayapi.UsesDefaultGateways(route.defaultGatewayScope) {
		if err := r.loadRouteDefaultGateways(ctx, state, route.defaultGatewayScope); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) loadRouteDefaultGateways(
	ctx context.Context,
	state *clusterState,
	scope gatewayv1.GatewayDefaultScope,
) error {
	var classes gatewayv1.GatewayClassList
	if err := r.reader.List(ctx, &classes); err != nil {
		return err
	}

	managedClasses := make(map[string]struct{}, len(classes.Items))
	for _, gatewayClass := range classes.Items {
		if string(gatewayClass.Spec.ControllerName) != r.controllerName {
			continue
		}
		state.gatewayClasses = append(state.gatewayClasses, gatewayClass)
		managedClasses[gatewayClass.Name] = struct{}{}
	}
	if len(managedClasses) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(state.gateways))
	for _, gateway := range state.gateways {
		seen[namespacedName(gateway.Namespace, gateway.Name)] = struct{}{}
	}

	var gateways gatewayv1.GatewayList
	if err := r.reader.List(ctx, &gateways); err != nil {
		return err
	}
	for _, gateway := range gateways.Items {
		if _, ok := managedClasses[string(gateway.Spec.GatewayClassName)]; !ok {
			continue
		}
		if !gatewayapi.GatewayMatchesDefaultScope(gateway, scope) {
			continue
		}
		key := namespacedName(gateway.Namespace, gateway.Name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		state.gateways = append(state.gateways, gateway)
	}

	return nil
}

func (r *Reconciler) loadRouteServices(
	ctx context.Context,
	state *clusterState,
	route routeInput,
) error {
	keys := make(map[string]client.ObjectKey)

	for _, parentRef := range route.parentRefs {
		if !isServiceParentRef(parentRef) {
			continue
		}

		key := client.ObjectKey{
			Namespace: namespaceOrDefault(parentRef.Namespace, route.namespace),
			Name:      string(parentRef.Name),
		}
		keys[namespacedName(key.Namespace, key.Name)] = key
	}

	for _, backend := range route.backends {
		targetKind, ok := backendKindForStatus(backend.Group, backend.Kind)
		if !ok || targetKind != "Service" {
			continue
		}

		key := client.ObjectKey{Namespace: backend.Namespace, Name: backend.Name}
		keys[namespacedName(key.Namespace, key.Name)] = key
	}

	for _, key := range keys {
		var service corev1.Service
		found, err := r.getOptional(ctx, key, &service)
		if err != nil {
			return err
		}
		if found {
			state.services = append(state.services, service)
		}
	}

	return nil
}

func (r *Reconciler) loadRouteServiceImports(
	ctx context.Context,
	state *clusterState,
	route routeInput,
) error {
	keys := make(map[string]client.ObjectKey)
	for _, backend := range route.backends {
		targetKind, ok := backendKindForStatus(backend.Group, backend.Kind)
		if !ok || targetKind != "ServiceImport" {
			continue
		}

		key := client.ObjectKey{Namespace: backend.Namespace, Name: backend.Name}
		keys[namespacedName(key.Namespace, key.Name)] = key
	}

	for _, key := range keys {
		var serviceImport mcsv1alpha1.ServiceImport
		found, err := r.getOptional(ctx, key, &serviceImport)
		if err != nil {
			return err
		}
		if found {
			state.serviceImports = append(state.serviceImports, serviceImport)
		}
	}

	return nil
}

func (r *Reconciler) loadRouteNamespaces(ctx context.Context, state *clusterState) error {
	namespaces := make(map[string]struct{})
	for _, route := range state.httpRoutes {
		namespaces[route.Namespace] = struct{}{}
	}
	for _, route := range state.grpcRoutes {
		namespaces[route.Namespace] = struct{}{}
	}
	for _, route := range state.tcpRoutes {
		namespaces[route.Namespace] = struct{}{}
	}
	for _, route := range state.udpRoutes {
		namespaces[route.Namespace] = struct{}{}
	}
	for _, route := range state.tlsRoutes {
		namespaces[route.Namespace] = struct{}{}
	}

	for name := range namespaces {
		if err := r.loadNamespace(ctx, state, name); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) loadRouteExtensionConfigMaps(
	ctx context.Context,
	state *clusterState,
	route routeInput,
) error {
	keys := make(map[string]client.ObjectKey)
	for _, ref := range route.extensionRefs {
		if ref.Group != "" || ref.Kind != extensionfilter.ConfigMapKind || strings.TrimSpace(ref.Name) == "" {
			continue
		}

		key := client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}
		keys[namespacedName(key.Namespace, key.Name)] = key
	}

	for _, key := range keys {
		var configMap corev1.ConfigMap
		found, err := r.getOptional(ctx, key, &configMap)
		if err != nil {
			return err
		}
		if found {
			state.configMaps = append(state.configMaps, configMap)
		}
	}

	return nil
}