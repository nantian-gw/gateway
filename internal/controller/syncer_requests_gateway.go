package controller

import (
	"context"
	"strings"

	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func listenerSetReconcileRequests(listenerSet *gatewayv1.ListenerSet) []reconcile.Request {
	if listenerSet == nil || listenerSet.Name == "" || listenerSet.Namespace == "" {
		return nil
	}
	parentRef := listenerSet.Spec.ParentRef
	if parentRef.Name == "" {
		return []reconcile.Request{snapshotReconcileRequest}
	}
	namespace := listenerSet.Namespace
	if parentRef.Namespace != nil {
		namespace = string(*parentRef.Namespace)
	}
	return []reconcile.Request{
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: namespace,
			Name:      string(parentRef.Name),
		}),
	}
}

func (s *Syncer) gatewayClassReconcileRequests(
	ctx context.Context,
	gatewayClass *gatewayv1.GatewayClass,
) []reconcile.Request {
	if gatewayClass == nil || gatewayClass.Name == "" {
		return nil
	}

	gateways, err := s.gatewaysForGatewayClassName(ctx, gatewayClass.Name)
	if err != nil {
		s.logDependencyLookupError("GatewayClass", "", gatewayClass.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}
	if len(gateways) == 0 {
		return nil
	}

	requests := make(map[reconcile.Request]struct{}, len(gateways))
	for _, gateway := range gateways {
		relevant, err := s.gatewayRelevantToCurrentSnapshot(ctx, &gateway)
		if err != nil {
			s.logDependencyLookupError("GatewayClass", "", gatewayClass.Name, err)
			return []reconcile.Request{snapshotReconcileRequest}
		}
		if !relevant {
			continue
		}

		requests[snapshotGatewayListenersReconcileRequestForKey(client.ObjectKeyFromObject(&gateway))] = struct{}{}

		namespaces, err := s.routeAttachmentNamespacesForGateway(ctx, client.ObjectKeyFromObject(&gateway))
		if err != nil {
			s.logDependencyLookupError("GatewayClass", "", gatewayClass.Name, err)
			return []reconcile.Request{snapshotReconcileRequest}
		}
		for _, namespace := range namespaces {
			requests[snapshotAttachmentsReconcileRequest(namespace)] = struct{}{}
		}
	}

	return sortedReconcileRequestsMap(requests)
}

func (s *Syncer) gatewayReconcileRequests(
	ctx context.Context,
	gateway *gatewayv1.Gateway,
) []reconcile.Request {
	if gateway == nil || gateway.Name == "" || gateway.Namespace == "" {
		return nil
	}
	key := client.ObjectKeyFromObject(gateway)
	relevant, err := s.gatewayRelevantToCurrentSnapshot(ctx, gateway)
	if err != nil {
		s.logDependencyLookupError("Gateway", gateway.Namespace, gateway.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}
	if !relevant {
		return nil
	}

	namespaces, err := s.routeAttachmentNamespacesForGateway(ctx, key)
	if err != nil {
		s.logDependencyLookupError("Gateway", gateway.Namespace, gateway.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}

	requests := map[reconcile.Request]struct{}{
		snapshotGatewayListenersReconcileRequestForKey(key): {},
	}
	for _, namespace := range namespaces {
		requests[snapshotAttachmentsReconcileRequest(namespace)] = struct{}{}
	}
	return sortedReconcileRequestsMap(requests)
}

func (s *Syncer) gatewayRelevantToCurrentSnapshot(
	ctx context.Context,
	gateway *gatewayv1.Gateway,
) (bool, error) {
	if gateway == nil || gateway.Name == "" || gateway.Namespace == "" {
		return false, nil
	}
	if s.gatewayTrackedInCurrentSnapshot(client.ObjectKeyFromObject(gateway)) {
		return true, nil
	}
	return s.gatewayManagedByController(ctx, gateway)
}

func (s *Syncer) gatewayManagedByController(
	ctx context.Context,
	gateway *gatewayv1.Gateway,
) (bool, error) {
	if gateway == nil || gateway.Spec.GatewayClassName == "" {
		return false, nil
	}

	var gatewayClass gatewayv1.GatewayClass
	if err := s.client.Get(
		ctx,
		client.ObjectKey{Name: string(gateway.Spec.GatewayClassName)},
		&gatewayClass,
	); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, err
	}
	return string(gatewayClass.Spec.ControllerName) == s.translatorControllerName(), nil
}

func (s *Syncer) translatorControllerName() string {
	if s == nil || s.translator == nil {
		return ""
	}
	return s.translator.ControllerName()
}

func (s *Syncer) gatewayTrackedInCurrentSnapshot(key client.ObjectKey) bool {
	if s == nil || s.store == nil {
		return false
	}
	current := s.store.Current()
	if current == nil {
		return false
	}

	prefix := key.Namespace + "/" + key.Name + "/"
	for _, listener := range current.Listeners {
		if strings.HasPrefix(listener.Name, prefix) {
			return true
		}
	}
	return false
}

func (s *Syncer) routeAttachmentNamespacesForGateway(
	ctx context.Context,
	key client.ObjectKey,
) ([]string, error) {
	indexValue := namespacedIndexValue(key.Namespace, key.Name)
	var (
		httpRoutes []gatewayv1.HTTPRoute
		grpcRoutes []gatewayv1.GRPCRoute
		tcpRoutes  []gatewayv1alpha2.TCPRoute
		udpRoutes  []gatewayv1alpha2.UDPRoute
		tlsRoutes  []gatewayv1alpha2.TLSRoute
	)
	lookupGroup, lookupCtx := errgroup.WithContext(ctx)
	lookupGroup.Go(func() error {
		var err error
		httpRoutes, err = s.httpRoutesForGatewayParentIndex(lookupCtx, indexValue)
		return err
	})
	lookupGroup.Go(func() error {
		var err error
		grpcRoutes, err = s.grpcRoutesForGatewayParentIndex(lookupCtx, indexValue)
		return err
	})
	lookupGroup.Go(func() error {
		var err error
		tcpRoutes, err = s.tcpRoutesForGatewayParentIndex(lookupCtx, indexValue)
		return err
	})
	lookupGroup.Go(func() error {
		var err error
		udpRoutes, err = s.udpRoutesForGatewayParentIndex(lookupCtx, indexValue)
		return err
	})
	lookupGroup.Go(func() error {
		var err error
		tlsRoutes, err = s.tlsRoutesForGatewayParentIndex(lookupCtx, indexValue)
		return err
	})
	if err := lookupGroup.Wait(); err != nil {
		return nil, err
	}

	namespaces := make(map[string]struct{}, len(httpRoutes)+len(grpcRoutes)+len(tcpRoutes)+len(udpRoutes)+len(tlsRoutes))
	for _, route := range httpRoutes {
		namespaces[route.Namespace] = struct{}{}
	}
	for _, route := range grpcRoutes {
		namespaces[route.Namespace] = struct{}{}
	}
	for _, route := range tcpRoutes {
		namespaces[route.Namespace] = struct{}{}
	}
	for _, route := range udpRoutes {
		namespaces[route.Namespace] = struct{}{}
	}
	for _, route := range tlsRoutes {
		namespaces[route.Namespace] = struct{}{}
	}
	return sortedIndexValues(namespaces), nil
}
