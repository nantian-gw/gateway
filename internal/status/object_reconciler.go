package status

import (
	"context"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/infrastructure"
)

type routeStatusUpdater func(context.Context, client.ObjectKey, []routeParentEvaluation) error

func (r *Reconciler) ReconcileGatewayObject(ctx context.Context, key client.ObjectKey) error {
	var current gatewayv1.Gateway
	if err := r.reader.Get(ctx, key, &current); err != nil {
		if apierrors.IsNotFound(err) {
			deleteGatewayConvergenceStageMetric(key)
			return nil
		}
		return err
	}

	state, err := r.loadGatewayObjectState(ctx, current)
	if err != nil {
		return err
	}

	eval, ok := evaluateGateways(state, evaluateRouteAttachments(state))[key]
	if !ok {
		r.logger.InfoContext(ctx, "ReconcileGatewayObject: eval not found, skipping status update", "key", key)
		deleteGatewayConvergenceStageMetric(key)
		return nil
	}
	if err := r.reconcileGatewayStatusWithSeed(ctx, key, &current, eval); err != nil {
		return err
	}
	listenerSetEvals := evaluateListenerSets(state, state.listenerSets, state.managedGatewayByKey)
	return r.reconcileListenerSetStatuses(ctx, state.listenerSets, listenerSetEvals)
}

func (r *Reconciler) loadGatewayObjectState(
	ctx context.Context,
	gateway gatewayv1.Gateway,
) (*clusterState, error) {
	state := r.newClusterState()
	state.gateways = append(state.gateways, gateway)

	if err := r.loadGatewayClassesForLoadedGateways(ctx, state); err != nil {
		return nil, err
	}
	if len(state.gateways) == 0 {
		state.index()
		return state, nil
	}
	if err := r.loadGatewayRoutes(ctx, state, gateway); err != nil {
		return nil, err
	}
	if err := r.loadRouteNamespaces(ctx, state); err != nil {
		return nil, err
	}
	if err := r.loadGatewayService(ctx, state, gateway); err != nil {
		return nil, err
	}
	if err := r.loadGatewayFrontendEndpointSlices(ctx, state, gateway); err != nil {
		return nil, err
	}
	if err := r.loadGatewayTLSSecrets(ctx, state, gateway); err != nil {
		return nil, err
	}
	if err := r.loadGatewayTLSConfigMaps(ctx, state, gateway); err != nil {
		return nil, err
	}
	if err := r.loadGatewayReferenceGrants(ctx, state, gateway); err != nil {
		return nil, err
	}
	if err := r.loadGatewayListenerSets(ctx, state); err != nil {
		return nil, err
	}
	if err := r.loadListenerSetNamespaces(ctx, state); err != nil {
		return nil, err
	}

	state.index()
	return state, nil
}

func (r *Reconciler) newClusterState() *clusterState {
	return &clusterState{
		controllerName:  r.controllerName,
		statusAddresses: append([]string(nil), r.statusAddresses...),
	}
}

func (r *Reconciler) loadGatewayListenerSets(ctx context.Context, state *clusterState) error {
	listenerSets, err := loadListenerSetsForState(ctx, r.reader, state.gateways)
	if err != nil {
		return err
	}
	state.listenerSets = listenerSets
	return nil
}

func (r *Reconciler) loadListenerSetNamespaces(ctx context.Context, state *clusterState) error {
	namespaces := make(map[string]struct{})
	for _, ls := range state.listenerSets {
		namespaces[ls.Namespace] = struct{}{}
	}
	for name := range namespaces {
		if err := r.loadNamespace(ctx, state, name); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) loadGatewayClassesForLoadedGateways(ctx context.Context, state *clusterState) error {
	classNames := gatewayClassNamesForGateways(state.gateways)
	state.gatewayClasses = state.gatewayClasses[:0]
	if len(classNames) == 0 {
		state.gateways = state.gateways[:0]
		return nil
	}

	managedGatewayClasses := make(map[string]struct{}, len(classNames))
	for _, name := range classNames {
		var gatewayClass gatewayv1.GatewayClass
		found, err := r.getOptional(ctx, client.ObjectKey{Name: name}, &gatewayClass)
		if err != nil {
			return err
		}
		if !found || string(gatewayClass.Spec.ControllerName) != r.controllerName {
			continue
		}

		state.gatewayClasses = append(state.gatewayClasses, gatewayClass)
		managedGatewayClasses[name] = struct{}{}
	}

	state.gateways = filterGatewaysByManagedGatewayClasses(state.gateways, managedGatewayClasses)
	return nil
}

func gatewayClassNamesForGateways(gateways []gatewayv1.Gateway) []string {
	set := make(map[string]struct{})
	for _, gateway := range gateways {
		name := strings.TrimSpace(string(gateway.Spec.GatewayClassName))
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}

	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func filterGatewaysByManagedGatewayClasses(
	gateways []gatewayv1.Gateway,
	managedGatewayClasses map[string]struct{},
) []gatewayv1.Gateway {
	filtered := gateways[:0]
	for _, gateway := range gateways {
		if _, ok := managedGatewayClasses[string(gateway.Spec.GatewayClassName)]; !ok {
			continue
		}
		filtered = append(filtered, gateway)
	}
	return filtered
}

func (r *Reconciler) loadGatewayRoutes(ctx context.Context, state *clusterState, gateway gatewayv1.Gateway) error {
	key := client.ObjectKeyFromObject(&gateway)
	httpRoutes, err := listHTTPRoutesForGateway(ctx, r.reader, key)
	if err != nil {
		return err
	}
	state.httpRoutes = httpRoutes

	grpcRoutes, err := listGRPCRoutesForGateway(ctx, r.reader, key)
	if err != nil {
		return err
	}
	state.grpcRoutes = grpcRoutes

	tcpRoutes, err := listTCPRoutesForGateway(ctx, r.reader, key)
	if err != nil {
		return err
	}
	state.tcpRoutes = tcpRoutes

	udpRoutes, err := listUDPRoutesForGateway(ctx, r.reader, key)
	if err != nil {
		return err
	}
	state.udpRoutes = udpRoutes

	tlsRoutes, err := listTLSRoutesForGateway(ctx, r.reader, key)
	if err != nil {
		return err
	}
	state.tlsRoutes = tlsRoutes

	if gatewayapi.GatewayActsAsDefault(gateway) {
		if err := r.loadDefaultGatewayRoutes(ctx, state, gateway); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) loadDefaultGatewayRoutes(
	ctx context.Context,
	state *clusterState,
	gateway gatewayv1.Gateway,
) error {
	var httpRoutes gatewayv1.HTTPRouteList
	if err := r.reader.List(ctx, &httpRoutes); err != nil {
		return err
	}
	state.httpRoutes = mergeHTTPRoutesByKey(state.httpRoutes, filterHTTPRoutesByDefaultScope(httpRoutes.Items, gateway.Spec.DefaultScope))

	var grpcRoutes gatewayv1.GRPCRouteList
	if err := r.reader.List(ctx, &grpcRoutes); err != nil {
		return err
	}
	state.grpcRoutes = mergeGRPCRoutesByKey(state.grpcRoutes, filterGRPCRoutesByDefaultScope(grpcRoutes.Items, gateway.Spec.DefaultScope))

	var tcpRoutes gatewayv1alpha2.TCPRouteList
	if err := r.reader.List(ctx, &tcpRoutes); err != nil {
		return err
	}
	state.tcpRoutes = mergeTCPRoutesByKey(state.tcpRoutes, filterTCPRoutesByDefaultScope(tcpRoutes.Items, gateway.Spec.DefaultScope))

	var udpRoutes gatewayv1alpha2.UDPRouteList
	if err := r.reader.List(ctx, &udpRoutes); err != nil {
		return err
	}
	state.udpRoutes = mergeUDPRoutesByKey(state.udpRoutes, filterUDPRoutesByDefaultScope(udpRoutes.Items, gateway.Spec.DefaultScope))

	var tlsRoutes gatewayv1alpha2.TLSRouteList
	if err := r.reader.List(ctx, &tlsRoutes); err != nil {
		return err
	}
	state.tlsRoutes = mergeTLSRoutesByKey(state.tlsRoutes, filterTLSRoutesByDefaultScope(tlsRoutes.Items, gateway.Spec.DefaultScope))

	return nil
}

func (r *Reconciler) loadNamespace(
	ctx context.Context,
	state *clusterState,
	name string,
) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}

	var namespace corev1.Namespace
	found, err := r.getOptional(ctx, client.ObjectKey{Name: name}, &namespace)
	if err != nil {
		return err
	}
	if found {
		state.namespaces = append(state.namespaces, namespace)
	}

	return nil
}

func (r *Reconciler) loadGatewayService(
	ctx context.Context,
	state *clusterState,
	gateway gatewayv1.Gateway,
) error {
	key := client.ObjectKey{
		Namespace: gateway.Namespace,
		Name:      infrastructure.GatewayServiceName(gateway.Name),
	}

	var service corev1.Service
	found, err := r.getOptional(ctx, key, &service)
	if err != nil {
		return err
	}
	if found {
		state.services = append(state.services, service)
	}

	return nil
}

func (r *Reconciler) loadGatewayServicesForNamespace(
	ctx context.Context,
	state *clusterState,
	namespace string,
	serviceNames map[string]struct{},
) error {
	if len(serviceNames) == 0 {
		return nil
	}

	var services corev1.ServiceList
	if err := r.reader.List(ctx, &services, client.InNamespace(namespace)); err != nil {
		return err
	}
	for _, service := range services.Items {
		if _, ok := serviceNames[service.Name]; !ok {
			continue
		}
		state.services = append(state.services, service)
	}

	return nil
}

func (r *Reconciler) loadGatewayFrontendEndpointSlices(
	ctx context.Context,
	state *clusterState,
	gateway gatewayv1.Gateway,
) error {
	var endpointSlices discoveryv1.EndpointSliceList
	if err := r.reader.List(
		ctx,
		&endpointSlices,
		client.InNamespace(gateway.Namespace),
		client.MatchingLabels{
			discoveryv1.LabelServiceName: infrastructure.GatewayServiceName(gateway.Name),
		},
	); err != nil {
		return err
	}

	state.endpointSlices = append(state.endpointSlices, endpointSlices.Items...)
	return nil
}

func (r *Reconciler) loadGatewayFrontendEndpointSlicesForNamespace(
	ctx context.Context,
	state *clusterState,
	namespace string,
	serviceNames map[string]struct{},
) error {
	if len(serviceNames) == 0 {
		return nil
	}

	var endpointSlices discoveryv1.EndpointSliceList
	if err := r.reader.List(ctx, &endpointSlices, client.InNamespace(namespace)); err != nil {
		return err
	}
	for _, endpointSlice := range endpointSlices.Items {
		serviceName := strings.TrimSpace(endpointSlice.Labels[discoveryv1.LabelServiceName])
		if _, ok := serviceNames[serviceName]; !ok {
			continue
		}
		state.endpointSlices = append(state.endpointSlices, endpointSlice)
	}

	return nil
}

func (r *Reconciler) loadGatewayTLSSecrets(
	ctx context.Context,
	state *clusterState,
	gateway gatewayv1.Gateway,
) error {
	keys := make(map[string]client.ObjectKey)

	for _, listener := range gateway.Spec.Listeners {
		if listener.TLS == nil {
			continue
		}
		for _, certificateRef := range listener.TLS.CertificateRefs {
			if group := stringOrEmpty(certificateRef.Group); group != "" {
				continue
			}

			kind := stringOrEmpty(certificateRef.Kind)
			if kind != "" && kind != "Secret" {
				continue
			}

			key := client.ObjectKey{
				Namespace: namespaceOrDefault(certificateRef.Namespace, gateway.Namespace),
				Name:      string(certificateRef.Name),
			}
			keys[namespacedName(key.Namespace, key.Name)] = key
		}
	}

	if backendTLS := gatewayapi.GatewayBackendTLS(gateway); backendTLS != nil && backendTLS.ClientCertificateRef != nil {
		clientCertificateRef := backendTLS.ClientCertificateRef
		if group := stringOrEmpty(clientCertificateRef.Group); group == "" {
			kind := stringOrEmpty(clientCertificateRef.Kind)
			if kind == "" || kind == "Secret" {
				key := client.ObjectKey{
					Namespace: namespaceOrDefault(clientCertificateRef.Namespace, gateway.Namespace),
					Name:      string(clientCertificateRef.Name),
				}
				keys[namespacedName(key.Namespace, key.Name)] = key
			}
		}
	}

	for _, key := range keys {
		var secret corev1.Secret
		found, err := r.getOptional(ctx, key, &secret)
		if err != nil {
			return err
		}
		if found {
			state.secrets = append(state.secrets, secret)
		}
	}

	return nil
}

func (r *Reconciler) loadGatewayTLSConfigMaps(
	ctx context.Context,
	state *clusterState,
	gateway gatewayv1.Gateway,
) error {
	keys := make(map[string]client.ObjectKey)

	for _, listener := range gateway.Spec.Listeners {
		validation := gatewayapi.FrontendValidationForListener(gateway, listener)
		if validation == nil {
			continue
		}
		for _, caRef := range validation.CACertificateRefs {
			if group := string(caRef.Group); group != "" {
				continue
			}

			kind := string(caRef.Kind)
			if kind != "" && kind != "ConfigMap" {
				continue
			}

			key := client.ObjectKey{
				Namespace: namespaceOrDefault(caRef.Namespace, gateway.Namespace),
				Name:      string(caRef.Name),
			}
			keys[namespacedName(key.Namespace, key.Name)] = key
		}
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

func (r *Reconciler) getOptional(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
) (bool, error) {
	return getOptionalWithReader(ctx, r.reader, key, obj)
}

func (r *Reconciler) getOptionalFromListReader(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
) (bool, error) {
	return getOptionalWithReader(ctx, r.listReader, key, obj)
}

func getOptionalWithReader(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
	obj client.Object,
) (bool, error) {
	if err := reader.Get(ctx, key, obj); err != nil {
		if isOptionalReadError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isOptionalReadError(err error) bool {
	return apierrors.IsNotFound(err) || meta.IsNoMatchError(err) || k8sruntime.IsNotRegisteredError(err)
}