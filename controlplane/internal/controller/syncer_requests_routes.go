package controller

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
	"github.com/nantian-gw/gateway/controlplane/internal/mesh"
)

func (s *Syncer) podReconcileRequests(pod *corev1.Pod) []reconcile.Request {
	if pod == nil || pod.Namespace == "" {
		return nil
	}
	if !snapshotHasMeshWorkloadNamespace(s.store.Current(), pod.Namespace) {
		return nil
	}
	return []reconcile.Request{snapshotWorkloadsReconcileRequest}
}

func snapshotHasMeshWorkloadNamespace(current *ir.Snapshot, namespace string) bool {
	if current == nil || namespace == "" {
		return false
	}

	hasServiceParent := func(routeNamespace string, parentRefs []ir.ParentRef) bool {
		if routeNamespace != namespace {
			return false
		}
		for _, parentRef := range parentRefs {
			if parentRef.Group == "" && parentRef.Kind == mesh.FrontendKindService {
				return true
			}
		}
		return false
	}

	for _, route := range current.HTTPRoutes {
		if hasServiceParent(route.Namespace, route.ParentRefs) {
			return true
		}
	}
	for _, route := range current.GRPCRoutes {
		if hasServiceParent(route.Namespace, route.ParentRefs) {
			return true
		}
	}
	for _, route := range current.StreamRoutes {
		if hasServiceParent(route.Namespace, route.ParentRefs) {
			return true
		}
	}
	return false
}

func (s *Syncer) referenceGrantReconcileRequests(
	ctx context.Context,
	grant *gatewayv1beta1.ReferenceGrant,
) []reconcile.Request {
	if grant == nil || grant.Namespace == "" {
		return nil
	}

	var (
		gateways   []gatewayv1.Gateway
		httpRoutes []gatewayv1.HTTPRoute
		grpcRoutes []gatewayv1.GRPCRoute
		tcpRoutes  []gatewayv1alpha2.TCPRoute
		udpRoutes  []gatewayv1alpha2.UDPRoute
		tlsRoutes  []gatewayv1alpha2.TLSRoute
	)

	lookupGroup, lookupCtx := errgroup.WithContext(ctx)
	lookupGroup.Go(func() error {
		items, err := s.gatewaysForFieldIndex(lookupCtx, gatewayReferenceGrantNamespaceIndex, grant.Namespace)
		if err != nil {
			return fmt.Errorf("list Gateways for ReferenceGrant namespace: %w", err)
		}
		gateways = items
		return nil
	})
	lookupGroup.Go(func() error {
		items, err := s.httpRoutesForReferenceGrantNamespace(lookupCtx, grant.Namespace)
		if err != nil {
			return fmt.Errorf("list HTTPRoutes for ReferenceGrant namespace: %w", err)
		}
		httpRoutes = items
		return nil
	})
	lookupGroup.Go(func() error {
		items, err := s.grpcRoutesForReferenceGrantNamespace(lookupCtx, grant.Namespace)
		if err != nil {
			return fmt.Errorf("list GRPCRoutes for ReferenceGrant namespace: %w", err)
		}
		grpcRoutes = items
		return nil
	})
	lookupGroup.Go(func() error {
		items, err := s.tcpRoutesForReferenceGrantNamespace(lookupCtx, grant.Namespace)
		if err != nil {
			return fmt.Errorf("list TCPRoutes for ReferenceGrant namespace: %w", err)
		}
		tcpRoutes = items
		return nil
	})
	lookupGroup.Go(func() error {
		items, err := s.udpRoutesForReferenceGrantNamespace(lookupCtx, grant.Namespace)
		if err != nil {
			return fmt.Errorf("list UDPRoutes for ReferenceGrant namespace: %w", err)
		}
		udpRoutes = items
		return nil
	})
	lookupGroup.Go(func() error {
		items, err := s.tlsRoutesForReferenceGrantNamespace(lookupCtx, grant.Namespace)
		if err != nil {
			return fmt.Errorf("list TLSRoutes for ReferenceGrant namespace: %w", err)
		}
		tlsRoutes = items
		return nil
	})
	if err := lookupGroup.Wait(); err != nil {
		s.logDependencyLookupError("ReferenceGrant", grant.Namespace, grant.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}

	requests := make(map[reconcile.Request]struct{})
	if err := s.addRelevantGatewayListenerRequests(ctx, requests, gateways); err != nil {
		s.logDependencyLookupError("ReferenceGrant", grant.Namespace, grant.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}

	for _, route := range httpRoutes {
		requests[snapshotHTTPRoutesReconcileRequestForKey(client.ObjectKeyFromObject(&route))] = struct{}{}
	}

	for _, route := range grpcRoutes {
		requests[snapshotGRPCRoutesReconcileRequestForKey(client.ObjectKeyFromObject(&route))] = struct{}{}
	}

	for _, route := range tcpRoutes {
		requests[snapshotTCPRoutesReconcileRequestForKey(client.ObjectKeyFromObject(&route))] = struct{}{}
	}

	for _, route := range udpRoutes {
		requests[snapshotUDPRoutesReconcileRequestForKey(client.ObjectKeyFromObject(&route))] = struct{}{}
	}

	for _, route := range tlsRoutes {
		requests[snapshotTLSRoutesReconcileRequestForKey(client.ObjectKeyFromObject(&route))] = struct{}{}
	}

	return sortedReconcileRequestsMap(requests)
}

func (s *Syncer) namespaceReconcileRequests(ctx context.Context, namespace *corev1.Namespace) []reconcile.Request {
	if namespace == nil || namespace.Name == "" {
		return nil
	}

	hasRelevantRoutes, err := s.namespaceAffectedByRelevantSelectorGateway(ctx, namespace.Name)
	if err != nil {
		s.logDependencyLookupError("Namespace", "", namespace.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}
	if !hasRelevantRoutes {
		return nil
	}

	return []reconcile.Request{snapshotAttachmentsReconcileRequest(namespace.Name)}
}

func (s *Syncer) relevantNamespaceSelectorGateways(ctx context.Context) ([]gatewayv1.Gateway, error) {
	gateways, err := s.gatewaysForFieldIndex(
		ctx,
		gatewayNamespaceSelectorIndex,
		gatewayNamespaceSelectorIndexMarker,
	)
	if err != nil {
		return nil, err
	}

	relevant := make([]gatewayv1.Gateway, 0, len(gateways))
	for _, gateway := range gateways {
		tracked, err := s.gatewayRelevantToCurrentSnapshot(ctx, &gateway)
		if err != nil {
			return nil, err
		}
		if tracked {
			relevant = append(relevant, gateway)
		}
	}
	return relevant, nil
}

func (s *Syncer) namespaceAffectedByRelevantSelectorGateway(ctx context.Context, namespace string) (bool, error) {
	gateways, err := s.relevantNamespaceSelectorGateways(ctx)
	if err != nil {
		return false, err
	}
	if len(gateways) == 0 {
		return false, nil
	}

	relevantParents := make(map[string]struct{}, len(gateways))
	for _, gateway := range gateways {
		relevantParents[namespacedIndexValue(gateway.Namespace, gateway.Name)] = struct{}{}
	}
	return s.namespaceRoutesReferenceRelevantSelectorGateways(ctx, namespace, relevantParents)
}

func (s *Syncer) namespaceRoutesReferenceRelevantSelectorGateways(
	ctx context.Context,
	namespace string,
	relevantParents map[string]struct{},
) (bool, error) {
	matchRouteParents := func(routeNamespace string, parentRefs []gatewayv1.ParentReference) bool {
		for _, candidate := range parentGatewayIndexKeys(routeNamespace, parentRefs) {
			if _, ok := relevantParents[candidate]; ok {
				return true
			}
		}
		return false
	}

	var httpRoutes gatewayv1.HTTPRouteList
	if err := s.client.List(ctx, &httpRoutes, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for _, route := range httpRoutes.Items {
		if matchRouteParents(route.Namespace, route.Spec.ParentRefs) {
			return true, nil
		}
	}

	var grpcRoutes gatewayv1.GRPCRouteList
	if err := s.client.List(ctx, &grpcRoutes, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for _, route := range grpcRoutes.Items {
		if matchRouteParents(route.Namespace, route.Spec.ParentRefs) {
			return true, nil
		}
	}

	var tcpRoutes gatewayv1alpha2.TCPRouteList
	if err := s.client.List(ctx, &tcpRoutes, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for _, route := range tcpRoutes.Items {
		if matchRouteParents(route.Namespace, route.Spec.ParentRefs) {
			return true, nil
		}
	}

	var udpRoutes gatewayv1alpha2.UDPRouteList
	if err := s.client.List(ctx, &udpRoutes, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for _, route := range udpRoutes.Items {
		if matchRouteParents(route.Namespace, route.Spec.ParentRefs) {
			return true, nil
		}
	}

	var tlsRoutes gatewayv1alpha2.TLSRouteList
	if err := s.client.List(ctx, &tlsRoutes, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	for _, route := range tlsRoutes.Items {
		if matchRouteParents(route.Namespace, route.Spec.ParentRefs) {
			return true, nil
		}
	}

	return false, nil
}
