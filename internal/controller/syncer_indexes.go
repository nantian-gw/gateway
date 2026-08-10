package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/mesh"
)

const (
	gatewayGatewayClassNameIndex          = "nantian.dev/infrastructure.gateway.gatewayclass-name"
	gatewaySecretReferenceIndex           = "nantian.dev/snapshot.gateway.secret-refs" //nolint:gosec // G101: field index name, not a credential
	gatewayConfigMapReferenceIndex        = "nantian.dev/snapshot.gateway.configmap-refs"
	gatewayReferenceGrantNamespaceIndex   = "nantian.dev/snapshot.gateway.referencegrant-namespaces"
	gatewayNamespaceSelectorIndex         = "nantian.dev/snapshot.gateway.namespace-selector"
	httpRouteParentGatewayIndex           = "nantian.dev/snapshot.httproute.parent-gateways"
	httpRouteConfigMapReferenceIndex      = "nantian.dev/snapshot.httproute.configmap-refs"
	httpRouteReferenceGrantNamespaceIndex = "nantian.dev/snapshot.httproute.referencegrant-namespaces"
	grpcRouteParentGatewayIndex           = "nantian.dev/snapshot.grpcroute.parent-gateways"
	grpcRouteConfigMapReferenceIndex      = "nantian.dev/snapshot.grpcroute.configmap-refs"
	grpcRouteReferenceGrantNamespaceIndex = "nantian.dev/snapshot.grpcroute.referencegrant-namespaces"
	tcpRouteParentGatewayIndex            = "nantian.dev/snapshot.tcproute.parent-gateways"
	tcpRouteReferenceGrantNamespaceIndex  = "nantian.dev/snapshot.tcproute.referencegrant-namespaces"
	udpRouteParentGatewayIndex            = "nantian.dev/snapshot.udproute.parent-gateways"
	udpRouteReferenceGrantNamespaceIndex  = "nantian.dev/snapshot.udproute.referencegrant-namespaces"
	tlsRouteParentGatewayIndex            = "nantian.dev/snapshot.tlsroute.parent-gateways"
	tlsRouteReferenceGrantNamespaceIndex  = "nantian.dev/snapshot.tlsroute.referencegrant-namespaces"
	backendTLSPolicyConfigMapRefIndex     = "nantian.dev/snapshot.backendtlspolicy.configmap-refs"
	listenerSetParentGatewayIndex         = "nantian.dev/snapshot.listenerset.parent-gateways"
	gatewayNamespaceSelectorIndexMarker   = "selector"
)

func (s *Syncer) setupReferenceIndexes(ctx context.Context, mgr ctrl.Manager) error {
	indexer := mgr.GetFieldIndexer()

	s.setBackendTLSPolicyConfigMapIndexAvailable(false)
	for _, contract := range controllerReferenceIndexContracts(backendTLSPolicyV1Supported(mgr)) {
		if !indexObjectSupported(mgr, contract.Object) {
			continue
		}
		if err := RegisterIndex(ctx, indexer, contract); err != nil {
			return err
		}
		if contract.Name == backendTLSPolicyConfigMapRefIndex {
			s.setBackendTLSPolicyConfigMapIndexAvailable(true)
		}
	}

	return nil
}

func indexObjectSupported(mgr ctrl.Manager, object client.Object) bool {
	if object == nil {
		return false
	}
	return resourceSupported(mgr, object)
}

func gatewaySecretReferenceIndexKeys(object client.Object) []string {
	gateway, ok := object.(*gatewayv1.Gateway)
	if !ok || gateway == nil {
		return nil
	}

	keys := make(map[string]struct{})
	for _, listener := range gatewayapi.EffectiveListeners(*gateway) {
		if listener.TLS == nil {
			continue
		}
		for _, ref := range listener.TLS.CertificateRefs {
			if key, ok := secretReferenceIndexValue(gateway.Namespace, ref); ok {
				keys[key] = struct{}{}
			}
		}
	}

	backendTLS := gatewayapi.GatewayBackendTLS(*gateway)
	if backendTLS != nil && backendTLS.ClientCertificateRef != nil {
		if key, ok := secretReferenceIndexValue(gateway.Namespace, *backendTLS.ClientCertificateRef); ok {
			keys[key] = struct{}{}
		}
	}

	return sortedIndexValues(keys)
}

func gatewayConfigMapReferenceIndexKeys(object client.Object) []string {
	gateway, ok := object.(*gatewayv1.Gateway)
	if !ok || gateway == nil {
		return nil
	}

	keys := make(map[string]struct{})
	for _, listener := range gatewayapi.EffectiveListeners(*gateway) {
		validation := gatewayapi.FrontendValidationForListener(*gateway, listener)
		if validation == nil {
			continue
		}
		for _, ref := range validation.CACertificateRefs {
			if key, ok := configMapReferenceIndexValue(gateway.Namespace, ref.Group, ref.Kind, ref.Namespace, ref.Name); ok {
				keys[key] = struct{}{}
			}
		}
	}

	return sortedIndexValues(keys)
}

func gatewayReferenceGrantNamespaceIndexKeys(object client.Object) []string {
	gateway, ok := object.(*gatewayv1.Gateway)
	if !ok || gateway == nil {
		return nil
	}

	keys := make(map[string]struct{})
	for _, listener := range gatewayapi.EffectiveListeners(*gateway) {
		if listener.TLS != nil {
			for _, ref := range listener.TLS.CertificateRefs {
				targetNamespace := namespaceOrDefault(ref.Namespace, gateway.Namespace)
				if targetNamespace != gateway.Namespace {
					keys[targetNamespace] = struct{}{}
				}
			}
		}

		if validation := gatewayapi.FrontendValidationForListener(*gateway, listener); validation != nil {
			for _, ref := range validation.CACertificateRefs {
				targetNamespace := namespaceOrDefault(ref.Namespace, gateway.Namespace)
				if targetNamespace != gateway.Namespace {
					keys[targetNamespace] = struct{}{}
				}
			}
		}
	}

	backendTLS := gatewayapi.GatewayBackendTLS(*gateway)
	if backendTLS != nil && backendTLS.ClientCertificateRef != nil {
		targetNamespace := namespaceOrDefault(backendTLS.ClientCertificateRef.Namespace, gateway.Namespace)
		if targetNamespace != gateway.Namespace {
			keys[targetNamespace] = struct{}{}
		}
	}

	return sortedIndexValues(keys)
}

func gatewayNamespaceSelectorIndexKeys(object client.Object) []string {
	gateway, ok := object.(*gatewayv1.Gateway)
	if !ok || gateway == nil {
		return nil
	}

	for _, listener := range gatewayapi.EffectiveListeners(*gateway) {
		if listener.AllowedRoutes == nil || listener.AllowedRoutes.Namespaces == nil || listener.AllowedRoutes.Namespaces.From == nil {
			continue
		}
		if *listener.AllowedRoutes.Namespaces.From == gatewayv1.NamespacesFromSelector {
			return []string{gatewayNamespaceSelectorIndexMarker}
		}
	}

	return nil
}

func httpRouteParentGatewayIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.HTTPRoute)
	if !ok || route == nil {
		return nil
	}
	return parentGatewayIndexKeys(route.Namespace, route.Spec.ParentRefs)
}

func httpRouteConfigMapReferenceIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.HTTPRoute)
	if !ok || route == nil {
		return nil
	}

	keys := make(map[string]struct{})
	for _, rule := range route.Spec.Rules {
		for _, filter := range rule.Filters {
			if key, ok := localConfigMapReferenceIndexValue(route.Namespace, filter.ExtensionRef); ok {
				keys[key] = struct{}{}
			}
		}
		for _, backendRef := range rule.BackendRefs {
			for _, filter := range backendRef.Filters {
				if key, ok := localConfigMapReferenceIndexValue(route.Namespace, filter.ExtensionRef); ok {
					keys[key] = struct{}{}
				}
			}
		}
	}

	return sortedIndexValues(keys)
}

func httpRouteReferenceGrantNamespaceIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.HTTPRoute)
	if !ok || route == nil {
		return nil
	}
	if mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace) {
		return nil
	}

	keys := make(map[string]struct{})
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			if key, ok := backendRefReferenceGrantNamespace(route.Namespace, backendRef.Namespace); ok {
				keys[key] = struct{}{}
			}
		}
	}
	return sortedIndexValues(keys)
}

func grpcRouteParentGatewayIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.GRPCRoute)
	if !ok || route == nil {
		return nil
	}
	return parentGatewayIndexKeys(route.Namespace, route.Spec.ParentRefs)
}

func grpcRouteConfigMapReferenceIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.GRPCRoute)
	if !ok || route == nil {
		return nil
	}

	keys := make(map[string]struct{})
	for _, rule := range route.Spec.Rules {
		for _, filter := range rule.Filters {
			if key, ok := localConfigMapReferenceIndexValue(route.Namespace, filter.ExtensionRef); ok {
				keys[key] = struct{}{}
			}
		}
		for _, backendRef := range rule.BackendRefs {
			for _, filter := range backendRef.Filters {
				if key, ok := localConfigMapReferenceIndexValue(route.Namespace, filter.ExtensionRef); ok {
					keys[key] = struct{}{}
				}
			}
		}
	}

	return sortedIndexValues(keys)
}

func grpcRouteReferenceGrantNamespaceIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1.GRPCRoute)
	if !ok || route == nil {
		return nil
	}
	if mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace) {
		return nil
	}

	keys := make(map[string]struct{})
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			if key, ok := backendRefReferenceGrantNamespace(route.Namespace, backendRef.Namespace); ok {
				keys[key] = struct{}{}
			}
		}
	}
	return sortedIndexValues(keys)
}

func tcpRouteParentGatewayIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TCPRoute)
	if !ok || route == nil {
		return nil
	}
	return parentGatewayIndexKeys(route.Namespace, route.Spec.ParentRefs)
}

func tcpRouteReferenceGrantNamespaceIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TCPRoute)
	if !ok || route == nil {
		return nil
	}
	if mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace) {
		return nil
	}

	keys := make(map[string]struct{})
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			if key, ok := backendRefReferenceGrantNamespace(route.Namespace, backendRef.Namespace); ok {
				keys[key] = struct{}{}
			}
		}
	}
	return sortedIndexValues(keys)
}

func udpRouteParentGatewayIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.UDPRoute)
	if !ok || route == nil {
		return nil
	}
	return parentGatewayIndexKeys(route.Namespace, route.Spec.ParentRefs)
}

func udpRouteReferenceGrantNamespaceIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.UDPRoute)
	if !ok || route == nil {
		return nil
	}
	if mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace) {
		return nil
	}

	keys := make(map[string]struct{})
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			if key, ok := backendRefReferenceGrantNamespace(route.Namespace, backendRef.Namespace); ok {
				keys[key] = struct{}{}
			}
		}
	}
	return sortedIndexValues(keys)
}

func tlsRouteParentGatewayIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TLSRoute)
	if !ok || route == nil {
		return nil
	}
	return parentGatewayIndexKeys(route.Namespace, route.Spec.ParentRefs)
}

func tlsRouteReferenceGrantNamespaceIndexKeys(object client.Object) []string {
	route, ok := object.(*gatewayv1alpha2.TLSRoute)
	if !ok || route == nil {
		return nil
	}
	if mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace) {
		return nil
	}

	keys := make(map[string]struct{})
	for _, rule := range route.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			if key, ok := backendRefReferenceGrantNamespace(route.Namespace, backendRef.Namespace); ok {
				keys[key] = struct{}{}
			}
		}
	}
	return sortedIndexValues(keys)
}

func listenerSetParentGatewayIndexKeys(object client.Object) []string {
	ls, ok := object.(*gatewayv1.ListenerSet)
	if !ok || ls == nil {
		return nil
	}
	if ls.Spec.ParentRef.Name == "" {
		return nil
	}
	return []string{namespacedIndexValue(
		namespaceOrDefault(ls.Spec.ParentRef.Namespace, ls.Namespace),
		string(ls.Spec.ParentRef.Name),
	)}
}

func parentGatewayIndexKeys(
	defaultNamespace string,
	parentRefs []gatewayv1.ParentReference,
) []string {
	keys := make(map[string]struct{})
	for _, parentRef := range parentRefs {
		if key, ok := gatewayParentIndexValue(defaultNamespace, parentRef); ok {
			keys[key] = struct{}{}
		}
	}
	return sortedIndexValues(keys)
}

func gatewayParentIndexValue(
	defaultNamespace string,
	parentRef gatewayv1.ParentReference,
) (string, bool) {
	if parentRef.Name == "" {
		return "", false
	}

	group := ""
	if parentRef.Group != nil {
		group = string(*parentRef.Group)
	}
	if group != "" && group != gatewayv1.GroupVersion.Group {
		return "", false
	}

	kind := ""
	if parentRef.Kind != nil {
		kind = string(*parentRef.Kind)
	}
	if kind == "" {
		kind = "Gateway"
	}
	if kind != "Gateway" {
		return "", false
	}

	return namespacedIndexValue(
		namespaceOrDefault(parentRef.Namespace, defaultNamespace),
		string(parentRef.Name),
	), true
}

func backendRefReferenceGrantNamespace(
	defaultNamespace string,
	targetNamespace *gatewayv1.Namespace,
) (string, bool) {
	namespace := namespaceOrDefault(targetNamespace, defaultNamespace)
	if namespace == defaultNamespace || namespace == "" {
		return "", false
	}
	return namespace, true
}

func backendTLSPolicyConfigMapReferenceIndexKeys(object client.Object) []string {
	item, ok := object.(*unstructured.Unstructured)
	if !ok || item == nil {
		return nil
	}

	policy, err := gatewayapi.DecodeBackendTLSPolicyV1(item)
	if err != nil {
		return nil
	}

	keys := make(map[string]struct{})
	for _, ref := range policy.Spec.Validation.CACertificateRefs {
		if key, ok := backendTLSPolicyConfigMapIndexValue(policy.Namespace, ref); ok {
			keys[key] = struct{}{}
		}
	}

	return sortedIndexValues(keys)
}

func backendTLSPolicyReferencesConfigMap(
	policy gatewayv1alpha3.BackendTLSPolicy,
	configMapName string,
) bool {
	if configMapName == "" || policy.Namespace == "" {
		return false
	}

	wantIndexValue := namespacedIndexValue(policy.Namespace, configMapName)
	for _, ref := range policy.Spec.Validation.CACertificateRefs {
		if key, ok := backendTLSPolicyConfigMapIndexValue(policy.Namespace, ref); ok && key == wantIndexValue {
			return true
		}
	}
	return false
}

func backendTLSPolicyConfigMapIndexValue(
	defaultNamespace string,
	ref gatewayv1.LocalObjectReference,
) (string, bool) {
	if ref.Name == "" || string(ref.Group) != "" {
		return "", false
	}
	kind := string(ref.Kind)
	if kind == "" {
		kind = extfilter.ConfigMapKind
	}
	if kind != extfilter.ConfigMapKind {
		return "", false
	}
	return namespacedIndexValue(defaultNamespace, string(ref.Name)), true
}

func secretReferenceIndexValue(defaultNamespace string, ref gatewayv1.SecretObjectReference) (string, bool) {
	if ref.Name == "" {
		return "", false
	}
	if ref.Group != nil && string(*ref.Group) != "" {
		return "", false
	}
	kind := "Secret"
	if ref.Kind != nil && string(*ref.Kind) != "" {
		kind = string(*ref.Kind)
	}
	if kind != "Secret" {
		return "", false
	}

	namespace := defaultNamespace
	if ref.Namespace != nil && *ref.Namespace != "" {
		namespace = string(*ref.Namespace)
	}
	return namespacedIndexValue(namespace, string(ref.Name)), true
}

func configMapReferenceIndexValue(
	defaultNamespace string,
	group gatewayv1.Group,
	kind gatewayv1.Kind,
	namespace *gatewayv1.Namespace,
	name gatewayv1.ObjectName,
) (string, bool) {
	if name == "" || string(group) != "" {
		return "", false
	}

	targetKind := string(kind)
	if targetKind == "" {
		targetKind = "ConfigMap"
	}
	if targetKind != "ConfigMap" {
		return "", false
	}

	targetNamespace := defaultNamespace
	if namespace != nil && *namespace != "" {
		targetNamespace = string(*namespace)
	}
	return namespacedIndexValue(targetNamespace, string(name)), true
}

func localConfigMapReferenceIndexValue(defaultNamespace string, ref *gatewayv1.LocalObjectReference) (string, bool) {
	if ref == nil || ref.Name == "" {
		return "", false
	}
	if string(ref.Group) != "" {
		return "", false
	}
	if string(ref.Kind) != extfilter.ConfigMapKind {
		return "", false
	}
	return namespacedIndexValue(defaultNamespace, string(ref.Name)), true
}

func namespacedIndexValue(namespace, name string) string {
	return namespace + "/" + name
}

func namespaceOrDefault(namespace *gatewayv1.Namespace, defaultNamespace string) string {
	if namespace != nil && *namespace != "" {
		return string(*namespace)
	}
	return defaultNamespace
}
