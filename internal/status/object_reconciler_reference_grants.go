package status

import (
	"context"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/mesh"
)

func (r *Reconciler) listReferenceGrants(ctx context.Context, state *clusterState) error {
	var grants gatewayv1beta1.ReferenceGrantList
	if err := r.listReader.List(ctx, &grants); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	state.referenceGrants = grants.Items
	return nil
}

func (r *Reconciler) loadRouteReferenceGrants(ctx context.Context, state *clusterState, route routeInput) error {
	namespaces := referenceGrantTargetNamespacesForRoute(route, state.gateways)
	return r.loadReferenceGrantsForNamespaces(ctx, state, namespaces)
}

func (r *Reconciler) loadGatewayReferenceGrants(
	ctx context.Context,
	state *clusterState,
	gateway gatewayv1.Gateway,
) error {
	namespaces := referenceGrantTargetNamespacesForGateway(gateway, state)
	return r.loadReferenceGrantsForNamespaces(ctx, state, namespaces)
}

func (r *Reconciler) loadReferenceGrantsForNamespaces(
	ctx context.Context,
	state *clusterState,
	namespaces []string,
) error {
	if len(namespaces) == 0 {
		state.referenceGrants = state.referenceGrants[:0]
		return nil
	}

	state.referenceGrants = state.referenceGrants[:0]
	for _, namespace := range namespaces {
		var grants gatewayv1beta1.ReferenceGrantList
		if err := r.listReader.List(ctx, &grants, client.InNamespace(namespace)); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		state.referenceGrants = append(state.referenceGrants, grants.Items...)
	}

	return nil
}

func referenceGrantTargetNamespacesForRoute(route routeInput, gateways []gatewayv1.Gateway) []string {
	parentRefs := gatewayapi.DefaultGatewayParentRefs(
		route.parentRefs,
		route.namespace,
		route.defaultGatewayScope,
		gateways,
	)
	if mesh.RouteUsesOnlyServiceParents(parentRefs, route.namespace) {
		return nil
	}

	namespaces := make(map[string]struct{})
	for _, backend := range route.backends {
		targetKind, ok := backendKindForStatus(backend.Group, backend.Kind)
		if !ok {
			continue
		}
		if targetKind != "Service" && targetKind != "ServiceImport" {
			continue
		}

		targetNamespace := strings.TrimSpace(backend.Namespace)
		if targetNamespace == "" || targetNamespace == route.namespace {
			continue
		}
		namespaces[targetNamespace] = struct{}{}
	}
	if len(namespaces) == 0 {
		return nil
	}

	out := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}

func referenceGrantTargetNamespacesForGateway(
	gateway gatewayv1.Gateway,
	state *clusterState,
) []string {
	namespaces := make(map[string]struct{})

	for _, route := range state.httpRoutes {
		addReferenceGrantTargetNamespacesForRoute(namespaces, httpRouteInput(route), state.gateways)
	}
	for _, route := range state.grpcRoutes {
		addReferenceGrantTargetNamespacesForRoute(namespaces, grpcRouteInput(route), state.gateways)
	}
	for _, route := range state.tcpRoutes {
		addReferenceGrantTargetNamespacesForRoute(namespaces, tcpRouteInput(route), state.gateways)
	}
	for _, route := range state.udpRoutes {
		addReferenceGrantTargetNamespacesForRoute(namespaces, udpRouteInput(route), state.gateways)
	}
	for _, route := range state.tlsRoutes {
		addReferenceGrantTargetNamespacesForRoute(namespaces, tlsRouteInput(route), state.gateways)
	}

	for _, listener := range gateway.Spec.Listeners {
		if listener.TLS != nil {
			for _, certificateRef := range listener.TLS.CertificateRefs {
				if group := strings.TrimSpace(stringOrEmpty(certificateRef.Group)); group != "" {
					continue
				}

				kind := strings.TrimSpace(stringOrEmpty(certificateRef.Kind))
				if kind != "" && kind != "Secret" {
					continue
				}

				targetNamespace := namespaceOrDefault(certificateRef.Namespace, gateway.Namespace)
				if targetNamespace != gateway.Namespace {
					namespaces[targetNamespace] = struct{}{}
				}
			}
		}

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

			targetNamespace := namespaceOrDefault(caRef.Namespace, gateway.Namespace)
			if targetNamespace != gateway.Namespace {
				namespaces[targetNamespace] = struct{}{}
			}
		}
	}

	gatewayKey := namespacedName(gateway.Namespace, gateway.Name)
	for _, listenerSet := range state.listenerSets {
		if listenerSetParentGatewayKey(listenerSet) != gatewayKey {
			continue
		}
		for _, listener := range listenerSet.Spec.Listeners {
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

				targetNamespace := namespaceOrDefault(certificateRef.Namespace, listenerSet.Namespace)
				if targetNamespace != listenerSet.Namespace {
					namespaces[targetNamespace] = struct{}{}
				}
			}
		}
	}

	if backendTLS := gatewayapi.GatewayBackendTLS(gateway); backendTLS != nil && backendTLS.ClientCertificateRef != nil {
		clientCertificateRef := backendTLS.ClientCertificateRef
		if group := strings.TrimSpace(stringOrEmpty(clientCertificateRef.Group)); group == "" {
			kind := strings.TrimSpace(stringOrEmpty(clientCertificateRef.Kind))
			if kind == "" || kind == "Secret" {
				targetNamespace := namespaceOrDefault(clientCertificateRef.Namespace, gateway.Namespace)
				if targetNamespace != gateway.Namespace {
					namespaces[targetNamespace] = struct{}{}
				}
			}
		}
	}

	if len(namespaces) == 0 {
		return nil
	}

	out := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}

func addReferenceGrantTargetNamespacesForRoute(namespaces map[string]struct{}, route routeInput, gateways []gatewayv1.Gateway) {
	for _, namespace := range referenceGrantTargetNamespacesForRoute(route, gateways) {
		namespaces[namespace] = struct{}{}
	}
}
