package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapi"
	backendlbv1alpha2 "github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/managedresources"
)

func (s *Syncer) snapshotReconcileRequests(ctx context.Context, object client.Object) []reconcile.Request {
	if object == nil {
		return nil
	}

	switch item := object.(type) {
	case *gatewayv1.GatewayClass:
		return s.gatewayClassReconcileRequests(ctx, item)
	case *gatewayv1.Gateway:
		return s.gatewayReconcileRequests(ctx, item)
	case *gatewayv1.HTTPRoute:
		return []reconcile.Request{snapshotHTTPRoutesReconcileRequestForKey(client.ObjectKeyFromObject(item))}
	case *gatewayv1.GRPCRoute:
		return []reconcile.Request{snapshotGRPCRoutesReconcileRequestForKey(client.ObjectKeyFromObject(item))}
	case *gatewayv1alpha2.TCPRoute:
		return []reconcile.Request{snapshotTCPRoutesReconcileRequestForKey(client.ObjectKeyFromObject(item))}
	case *gatewayv1alpha2.UDPRoute:
		return []reconcile.Request{snapshotUDPRoutesReconcileRequestForKey(client.ObjectKeyFromObject(item))}
	case *gatewayv1alpha2.TLSRoute:
		return []reconcile.Request{snapshotTLSRoutesReconcileRequestForKey(client.ObjectKeyFromObject(item))}
	case *corev1.Service:
		if !managedresources.ShouldAffectSnapshot(item) {
			return nil
		}
		return []reconcile.Request{snapshotServiceDependenciesReconcileRequestForService(client.ObjectKeyFromObject(item))}
	case *corev1.Pod:
		if !managedresources.ShouldAffectSnapshot(item) {
			return nil
		}
		return s.podReconcileRequests(item)
	case *discoveryv1.EndpointSlice:
		if !managedresources.ShouldAffectSnapshot(item) {
			return nil
		}
		return endpointSliceBackendReconcileRequests(item)
	case *corev1.Secret:
		return s.secretReconcileRequests(ctx, item)
	case *corev1.ConfigMap:
		return s.configMapReconcileRequests(ctx, item)
	case *corev1.Namespace:
		return s.namespaceReconcileRequests(ctx, item)
	case *gatewayv1beta1.ReferenceGrant:
		return s.referenceGrantReconcileRequests(ctx, item)
	case *mcsv1alpha1.ServiceImport:
		return []reconcile.Request{snapshotBackendDependenciesReconcileRequestForServiceImport(client.ObjectKeyFromObject(item))}
	case *backendlbv1alpha2.BackendLBPolicy:
		return backendLBPolicyReconcileRequests(item)
	case *gatewayv1.ListenerSet:
		return listenerSetReconcileRequests(item)
	case *unstructured.Unstructured:
		if item.GroupVersionKind() == gatewayapi.BackendTLSPolicyV1GVK {
			policy, err := gatewayapi.DecodeBackendTLSPolicyV1(item)
			if err != nil {
				if item.GetNamespace() == "" {
					return []reconcile.Request{snapshotBackendsReconcileRequest}
				}
				return []reconcile.Request{snapshotBackendsReconcileRequestForNamespace(item.GetNamespace())}
			}
			return backendTLSPolicyReconcileRequests(policy)
		}
	}

	return []reconcile.Request{snapshotReconcileRequest}
}
