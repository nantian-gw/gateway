package status

import (
	"context"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backendlb "github.com/nantian-gw/gateway/internal/gwexp/backendlb"
	"github.com/nantian-gw/gateway/internal/infrastructure"
)

type fullReconcileSupportRefs struct {
	services                 map[string]client.ObjectKey
	endpointSliceServices    map[string]client.ObjectKey
	serviceImports           map[string]client.ObjectKey
	referenceGrantNamespaces map[string]struct{}
	namespaces               map[string]struct{}
	secrets                  map[string]client.ObjectKey
	configMaps               map[string]client.ObjectKey
}

func newFullReconcileSupportRefs() fullReconcileSupportRefs {
	return fullReconcileSupportRefs{
		services:                 make(map[string]client.ObjectKey),
		endpointSliceServices:    make(map[string]client.ObjectKey),
		serviceImports:           make(map[string]client.ObjectKey),
		referenceGrantNamespaces: make(map[string]struct{}),
		namespaces:               make(map[string]struct{}),
		secrets:                  make(map[string]client.ObjectKey),
		configMaps:               make(map[string]client.ObjectKey),
	}
}

func (r *Reconciler) loadReferencedSupportResources(ctx context.Context, state *clusterState) error {
	refs := collectFullReconcileSupportRefs(state)

	if err := r.loadReferencedServices(ctx, state, refs.services); err != nil {
		return err
	}
	if err := r.loadReferencedEndpointSlices(ctx, state, refs.endpointSliceServices); err != nil {
		return err
	}
	if err := r.loadReferencedServiceImports(ctx, state, refs.serviceImports); err != nil {
		return err
	}
	if err := r.loadReferencedReferenceGrants(ctx, state, refs.referenceGrantNamespaces); err != nil {
		return err
	}
	if err := r.loadReferencedNamespaces(ctx, state, refs.namespaces); err != nil {
		return err
	}
	if err := r.loadReferencedSecrets(ctx, state, refs.secrets); err != nil {
		return err
	}
	if err := r.loadReferencedConfigMaps(ctx, state, refs.configMaps); err != nil {
		return err
	}

	return nil
}

func collectFullReconcileSupportRefs(state *clusterState) fullReconcileSupportRefs {
	refs := newFullReconcileSupportRefs()

	collectGatewayServiceRefs(refs.services, refs.endpointSliceServices, state.managedGateways)
	collectAllRouteTrafficRefs(refs.services, refs.serviceImports, state)
	collectBackendLBPolicyTrafficRefs(refs.services, refs.serviceImports, state.backendLBPolicies)
	collectBackendTLSPolicyTrafficRefs(refs.services, refs.serviceImports, state.backendTLSPolicies)
	collectReferenceGrantNamespaces(refs.referenceGrantNamespaces, state)
	collectRouteNamespaceRefs(refs.namespaces, state)
	collectListenerSetNamespaceRefs(refs.namespaces, state)
	collectGatewaySecretRefs(refs.secrets, state.managedGateways)
	collectGatewayConfigMapRefs(refs.configMaps, state.managedGateways, state.managedGatewayClasses)
	collectRouteExtensionConfigMapRefs(refs.configMaps, state.httpRoutes, state.grpcRoutes)
	collectBackendTLSPolicyConfigMapRefs(refs.configMaps, state.backendTLSPolicies)

	return refs
}

func collectGatewayServiceRefs(
	services map[string]client.ObjectKey,
	endpointSliceServices map[string]client.ObjectKey,
	gateways []gatewayv1.Gateway,
) {
	for _, gateway := range gateways {
		namespace := gateway.Namespace
		name := infrastructure.GatewayServiceName(gateway.Name)
		addObjectKeyRef(services, namespace, name)
		addObjectKeyRef(endpointSliceServices, namespace, name)
	}
}

func collectReferenceGrantNamespaces(out map[string]struct{}, state *clusterState) {
	for _, route := range state.httpRoutes {
		addReferenceGrantTargetNamespacesForRoute(out, httpRouteInput(route), state.managedGateways)
	}
	for _, route := range state.grpcRoutes {
		addReferenceGrantTargetNamespacesForRoute(out, grpcRouteInput(route), state.managedGateways)
	}
	for _, route := range state.tcpRoutes {
		addReferenceGrantTargetNamespacesForRoute(out, tcpRouteInput(route), state.managedGateways)
	}
	for _, route := range state.udpRoutes {
		addReferenceGrantTargetNamespacesForRoute(out, udpRouteInput(route), state.managedGateways)
	}
	for _, route := range state.tlsRoutes {
		addReferenceGrantTargetNamespacesForRoute(out, tlsRouteInput(route), state.managedGateways)
	}
	for _, gateway := range state.managedGateways {
		for _, namespace := range referenceGrantTargetNamespacesForGateway(gateway, state) {
			out[namespace] = struct{}{}
		}
	}
}

func collectAllRouteTrafficRefs(
	services map[string]client.ObjectKey,
	serviceImports map[string]client.ObjectKey,
	state *clusterState,
) {
	for _, route := range state.httpRoutes {
		collectRouteTrafficRefs(services, serviceImports, httpRouteInput(route))
	}
	for _, route := range state.grpcRoutes {
		collectRouteTrafficRefs(services, serviceImports, grpcRouteInput(route))
	}
	for _, route := range state.tcpRoutes {
		collectRouteTrafficRefs(services, serviceImports, tcpRouteInput(route))
	}
	for _, route := range state.udpRoutes {
		collectRouteTrafficRefs(services, serviceImports, udpRouteInput(route))
	}
	for _, route := range state.tlsRoutes {
		collectRouteTrafficRefs(services, serviceImports, tlsRouteInput(route))
	}
}

func collectRouteTrafficRefs(
	services map[string]client.ObjectKey,
	serviceImports map[string]client.ObjectKey,
	route routeInput,
) {
	for _, parentRef := range route.parentRefs {
		if !isServiceParentRef(parentRef) {
			continue
		}
		addObjectKeyRef(
			services,
			namespaceOrDefault(parentRef.Namespace, route.namespace),
			string(parentRef.Name),
		)
	}

	for _, backend := range route.backends {
		targetKind, ok := backendKindForStatus(backend.Group, backend.Kind)
		if !ok {
			continue
		}

		targetNamespace := strings.TrimSpace(backend.Namespace)
		if targetNamespace == "" {
			targetNamespace = route.namespace
		}

		switch targetKind {
		case "Service":
			addObjectKeyRef(services, targetNamespace, backend.Name)
		case "ServiceImport":
			addObjectKeyRef(serviceImports, targetNamespace, backend.Name)
		}
	}
}

func collectBackendLBPolicyTrafficRefs(
	services map[string]client.ObjectKey,
	serviceImports map[string]client.ObjectKey,
	policies []backendlb.BackendLBPolicy,
) {
	for _, policy := range policies {
		for _, targetRef := range policy.Spec.TargetRefs {
			switch {
			case targetRef.Group == "" && targetRef.Kind == "Service":
				addObjectKeyRef(services, policy.Namespace, string(targetRef.Name))
			case targetRef.Group == mcsv1alpha1.GroupName && targetRef.Kind == "ServiceImport":
				addObjectKeyRef(serviceImports, policy.Namespace, string(targetRef.Name))
			}
		}
	}
}

func collectBackendTLSPolicyTrafficRefs(
	services map[string]client.ObjectKey,
	serviceImports map[string]client.ObjectKey,
	policies []gatewayv1alpha3.BackendTLSPolicy,
) {
	for _, policy := range policies {
		for _, targetRef := range policy.Spec.TargetRefs {
			switch {
			case targetRef.Group == "" && targetRef.Kind == "Service":
				addObjectKeyRef(services, policy.Namespace, string(targetRef.Name))
			case targetRef.Group == mcsv1alpha1.GroupName && targetRef.Kind == "ServiceImport":
				addObjectKeyRef(serviceImports, policy.Namespace, string(targetRef.Name))
			}
		}
	}
}

func collectRouteNamespaceRefs(out map[string]struct{}, state *clusterState) {
	for _, route := range state.httpRoutes {
		addNamespaceRef(out, route.Namespace)
	}
	for _, route := range state.grpcRoutes {
		addNamespaceRef(out, route.Namespace)
	}
	for _, route := range state.tcpRoutes {
		addNamespaceRef(out, route.Namespace)
	}
	for _, route := range state.udpRoutes {
		addNamespaceRef(out, route.Namespace)
	}
	for _, route := range state.tlsRoutes {
		addNamespaceRef(out, route.Namespace)
	}
}

func collectListenerSetNamespaceRefs(out map[string]struct{}, state *clusterState) {
	if !gatewaysUseListenerSetNamespaceSelectors(state.managedGateways) {
		return
	}
	for _, ls := range state.listenerSets {
		addNamespaceRef(out, ls.Namespace)
	}
}

func gatewaysUseListenerSetNamespaceSelectors(gateways []gatewayv1.Gateway) bool {
	for _, gateway := range gateways {
		if gateway.Spec.AllowedListeners == nil ||
			gateway.Spec.AllowedListeners.Namespaces == nil ||
			gateway.Spec.AllowedListeners.Namespaces.From == nil {
			continue
		}
		if *gateway.Spec.AllowedListeners.Namespaces.From == gatewayv1.NamespacesFromSelector {
			return true
		}
	}
	return false
}

func collectGatewaySecretRefs(out map[string]client.ObjectKey, gateways []gatewayv1.Gateway) {
	for _, gateway := range gateways {
		for _, listener := range gateway.Spec.Listeners {
			if listener.TLS == nil {
				continue
			}
			for _, certificateRef := range listener.TLS.CertificateRefs {
				if group := strings.TrimSpace(stringOrEmpty(certificateRef.Group)); group != "" {
					continue
				}

				kind := strings.TrimSpace(stringOrEmpty(certificateRef.Kind))
				if kind != "" && kind != "Secret" {
					continue
				}

				addObjectKeyRef(
					out,
					namespaceOrDefault(certificateRef.Namespace, gateway.Namespace),
					string(certificateRef.Name),
				)
			}
		}

		if backendTLS := gatewayapi.GatewayBackendTLS(gateway); backendTLS != nil && backendTLS.ClientCertificateRef != nil {
			clientCertificateRef := backendTLS.ClientCertificateRef
			if group := strings.TrimSpace(stringOrEmpty(clientCertificateRef.Group)); group == "" {
				kind := strings.TrimSpace(stringOrEmpty(clientCertificateRef.Kind))
				if kind == "" || kind == "Secret" {
					addObjectKeyRef(
						out,
						namespaceOrDefault(clientCertificateRef.Namespace, gateway.Namespace),
						string(clientCertificateRef.Name),
					)
				}
			}
		}
	}
}

func collectGatewayConfigMapRefs(
	out map[string]client.ObjectKey,
	gateways []gatewayv1.Gateway,
	gatewayClasses map[string]gatewayv1.GatewayClass,
) {
	for _, gateway := range gateways {
		if gateway.Spec.Infrastructure != nil && gateway.Spec.Infrastructure.ParametersRef != nil {
			ref := gateway.Spec.Infrastructure.ParametersRef
			if group := strings.TrimSpace(string(ref.Group)); group == "" &&
				strings.EqualFold(strings.TrimSpace(string(ref.Kind)), "ConfigMap") {
				addObjectKeyRef(out, gateway.Namespace, string(ref.Name))
			}
		}

		if gatewayClass, ok := gatewayClasses[string(gateway.Spec.GatewayClassName)]; ok && gatewayClass.Spec.ParametersRef != nil {
			ref := gatewayClass.Spec.ParametersRef
			namespace := strings.TrimSpace(stringOrEmpty(ref.Namespace))
			if namespace != "" &&
				strings.TrimSpace(string(ref.Group)) == "" &&
				strings.EqualFold(strings.TrimSpace(string(ref.Kind)), "ConfigMap") {
				addObjectKeyRef(out, namespace, string(ref.Name))
			}
		}

		for _, listener := range gateway.Spec.Listeners {
			validation := gatewayapi.FrontendValidationForListener(gateway, listener)
			if validation == nil {
				continue
			}
			for _, caRef := range validation.CACertificateRefs {
				if group := strings.TrimSpace(string(caRef.Group)); group != "" {
					continue
				}

				kind := strings.TrimSpace(string(caRef.Kind))
				if kind != "" && kind != "ConfigMap" {
					continue
				}

				addObjectKeyRef(
					out,
					namespaceOrDefault(caRef.Namespace, gateway.Namespace),
					string(caRef.Name),
				)
			}
		}
	}
}

func collectRouteExtensionConfigMapRefs(
	out map[string]client.ObjectKey,
	httpRoutes []gatewayv1.HTTPRoute,
	grpcRoutes []gatewayv1.GRPCRoute,
) {
	for _, route := range httpRoutes {
		collectExtensionConfigMapRefs(out, httpRouteExtensionRefs(route))
	}
	for _, route := range grpcRoutes {
		collectExtensionConfigMapRefs(out, grpcRouteExtensionRefs(route))
	}
}

func collectExtensionConfigMapRefs(out map[string]client.ObjectKey, refs []extfilter.Ref) {
	for _, ref := range refs {
		if strings.TrimSpace(ref.Group) != "" ||
			strings.TrimSpace(ref.Kind) != extfilter.ConfigMapKind ||
			strings.TrimSpace(ref.Name) == "" {
			continue
		}

		addObjectKeyRef(out, ref.Namespace, ref.Name)
	}
}

func collectBackendTLSPolicyConfigMapRefs(
	out map[string]client.ObjectKey,
	policies []gatewayv1alpha3.BackendTLSPolicy,
) {
	for _, policy := range policies {
		for _, ref := range policy.Spec.Validation.CACertificateRefs {
			if strings.TrimSpace(string(ref.Group)) != "" {
				continue
			}

			kind := strings.TrimSpace(string(ref.Kind))
			if kind != "" && kind != "ConfigMap" {
				continue
			}

			addObjectKeyRef(out, policy.Namespace, string(ref.Name))
		}
	}
}

func (r *Reconciler) loadReferencedServices(
	ctx context.Context,
	state *clusterState,
	keys map[string]client.ObjectKey,
) error {
	for _, key := range sortedObjectKeys(keys) {
		var service corev1.Service
		found, err := r.getOptionalFromListReader(ctx, key, &service)
		if err != nil {
			return err
		}
		if found {
			state.services = append(state.services, service)
		}
	}
	return nil
}

func (r *Reconciler) loadReferencedEndpointSlices(
	ctx context.Context,
	state *clusterState,
	serviceKeys map[string]client.ObjectKey,
) error {
	for _, key := range sortedObjectKeys(serviceKeys) {
		var endpointSlices discoveryv1.EndpointSliceList
		if err := r.listReader.List(
			ctx,
			&endpointSlices,
			client.InNamespace(key.Namespace),
			client.MatchingLabels{discoveryv1.LabelServiceName: key.Name},
		); err != nil {
			return err
		}
		state.endpointSlices = append(state.endpointSlices, endpointSlices.Items...)
	}
	return nil
}

func (r *Reconciler) loadReferencedServiceImports(
	ctx context.Context,
	state *clusterState,
	keys map[string]client.ObjectKey,
) error {
	for _, key := range sortedObjectKeys(keys) {
		var serviceImport mcsv1alpha1.ServiceImport
		found, err := r.getOptionalFromListReader(ctx, key, &serviceImport)
		if err != nil {
			return err
		}
		if found {
			state.serviceImports = append(state.serviceImports, serviceImport)
		}
	}
	return nil
}

func (r *Reconciler) loadReferencedReferenceGrants(
	ctx context.Context,
	state *clusterState,
	namespaces map[string]struct{},
) error {
	state.referenceGrants = state.referenceGrants[:0]
	for _, namespace := range sortedStringKeys(namespaces) {
		var grants gatewayv1beta1.ReferenceGrantList
		if err := r.listReader.List(ctx, &grants, client.InNamespace(namespace)); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		state.referenceGrants = append(state.referenceGrants, grants.Items...)
	}
	return nil
}

func (r *Reconciler) loadReferencedNamespaces(
	ctx context.Context,
	state *clusterState,
	names map[string]struct{},
) error {
	for _, name := range sortedStringKeys(names) {
		var namespace corev1.Namespace
		found, err := r.getOptionalFromListReader(ctx, client.ObjectKey{Name: name}, &namespace)
		if err != nil {
			return err
		}
		if found {
			state.namespaces = append(state.namespaces, namespace)
		}
	}
	return nil
}

func (r *Reconciler) loadReferencedSecrets(
	ctx context.Context,
	state *clusterState,
	keys map[string]client.ObjectKey,
) error {
	for _, key := range sortedObjectKeys(keys) {
		var secret corev1.Secret
		found, err := r.getOptionalFromListReader(ctx, key, &secret)
		if err != nil {
			return err
		}
		if found {
			state.secrets = append(state.secrets, secret)
		}
	}
	return nil
}

func (r *Reconciler) loadReferencedConfigMaps(
	ctx context.Context,
	state *clusterState,
	keys map[string]client.ObjectKey,
) error {
	for _, key := range sortedObjectKeys(keys) {
		var configMap corev1.ConfigMap
		found, err := r.getOptionalFromListReader(ctx, key, &configMap)
		if err != nil {
			return err
		}
		if found {
			state.configMaps = append(state.configMaps, configMap)
		}
	}
	return nil
}

func addNamespaceRef(out map[string]struct{}, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	out[name] = struct{}{}
}

func addObjectKeyRef(out map[string]client.ObjectKey, namespace, name string) {
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	if namespace == "" || name == "" {
		return
	}

	key := client.ObjectKey{Namespace: namespace, Name: name}
	out[namespacedName(namespace, name)] = key
}

func sortedStringKeys(items map[string]struct{}) []string {
	out := make([]string, 0, len(items))
	for item := range items {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func sortedObjectKeys(items map[string]client.ObjectKey) []client.ObjectKey {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]client.ObjectKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, items[key])
	}
	return out
}
