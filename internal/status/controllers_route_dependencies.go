package status

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	ctrl "sigs.k8s.io/controller-runtime"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func routeBackendServiceDependencyPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			_, ok := event.Object.(*corev1.Service)
			return ok
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			oldService, okOld := event.ObjectOld.(*corev1.Service)
			newService, okNew := event.ObjectNew.(*corev1.Service)
			if !okOld || !okNew || oldService == nil || newService == nil {
				return false
			}
			return !reflect.DeepEqual(oldService.Spec.Ports, newService.Spec.Ports)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			_, ok := event.Object.(*corev1.Service)
			return ok
		},
	}
}

func routeBackendServiceImportDependencyPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			_, ok := event.Object.(*mcsv1alpha1.ServiceImport)
			return ok
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			oldImport, okOld := event.ObjectOld.(*mcsv1alpha1.ServiceImport)
			newImport, okNew := event.ObjectNew.(*mcsv1alpha1.ServiceImport)
			if !okOld || !okNew || oldImport == nil || newImport == nil {
				return false
			}
			return !reflect.DeepEqual(oldImport.Spec, newImport.Spec)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			_, ok := event.Object.(*mcsv1alpha1.ServiceImport)
			return ok
		},
	}
}

func httpRouteStatusRequestsForService(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForService(
		ctx,
		reader,
		object,
		listHTTPRoutesForServiceBackend,
		listHTTPRoutesForServiceParent,
		func(route gatewayv1.HTTPRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func httpRouteStatusRequestsForServiceImport(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForServiceImport(
		ctx,
		reader,
		object,
		listHTTPRoutesForServiceImportBackend,
		func(route gatewayv1.HTTPRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func grpcRouteStatusRequestsForService(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForService(
		ctx,
		reader,
		object,
		listGRPCRoutesForServiceBackend,
		listGRPCRoutesForServiceParent,
		func(route gatewayv1.GRPCRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func grpcRouteStatusRequestsForServiceImport(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForServiceImport(
		ctx,
		reader,
		object,
		listGRPCRoutesForServiceImportBackend,
		func(route gatewayv1.GRPCRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func tcpRouteStatusRequestsForService(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForService(
		ctx,
		reader,
		object,
		listTCPRoutesForServiceBackend,
		listTCPRoutesForServiceParent,
		func(route gatewayv1alpha2.TCPRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func tcpRouteStatusRequestsForServiceImport(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForServiceImport(
		ctx,
		reader,
		object,
		listTCPRoutesForServiceImportBackend,
		func(route gatewayv1alpha2.TCPRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func udpRouteStatusRequestsForService(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForService(
		ctx,
		reader,
		object,
		listUDPRoutesForServiceBackend,
		listUDPRoutesForServiceParent,
		func(route gatewayv1alpha2.UDPRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func udpRouteStatusRequestsForServiceImport(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForServiceImport(
		ctx,
		reader,
		object,
		listUDPRoutesForServiceImportBackend,
		func(route gatewayv1alpha2.UDPRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func tlsRouteStatusRequestsForService(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForService(
		ctx,
		reader,
		object,
		listTLSRoutesForServiceBackend,
		listTLSRoutesForServiceParent,
		func(route gatewayv1alpha2.TLSRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func tlsRouteStatusRequestsForServiceImport(
	ctx context.Context,
	reader client.Reader,
	object client.Object,
) []reconcile.Request {
	return routeStatusRequestsForServiceImport(
		ctx,
		reader,
		object,
		listTLSRoutesForServiceImportBackend,
		func(route gatewayv1alpha2.TLSRoute) client.ObjectKey {
			return client.ObjectKeyFromObject(&route)
		},
	)
}

func routeStatusRequestsForService[T any](
	ctx context.Context,
	reader client.Reader,
	object client.Object,
	listBackend func(context.Context, client.Reader, client.ObjectKey) ([]T, bool, error),
	listParents func(context.Context, client.Reader, client.ObjectKey) ([]T, bool, error),
	routeKey func(T) client.ObjectKey,
) []reconcile.Request {
	service, ok := object.(*corev1.Service)
	if !ok || service == nil {
		return nil
	}

	key := client.ObjectKeyFromObject(service)
	requests := make(map[client.ObjectKey]struct{})

	backendRoutes, _, err := listBackend(ctx, reader, key)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "list route status backend dependencies", "service", key)
		return nil
	}
	for _, route := range backendRoutes {
		requests[routeKey(route)] = struct{}{}
	}

	parentRoutes, _, err := listParents(ctx, reader, key)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "list route status service-parent dependencies", "service", key)
		return nil
	}
	for _, route := range parentRoutes {
		requests[routeKey(route)] = struct{}{}
	}

	return reconcileRequestsForKeys(requests)
}

func routeStatusRequestsForServiceImport[T any](
	ctx context.Context,
	reader client.Reader,
	object client.Object,
	listBackend func(context.Context, client.Reader, client.ObjectKey) ([]T, bool, error),
	routeKey func(T) client.ObjectKey,
) []reconcile.Request {
	serviceImport, ok := object.(*mcsv1alpha1.ServiceImport)
	if !ok || serviceImport == nil {
		return nil
	}

	key := client.ObjectKeyFromObject(serviceImport)
	routes, _, err := listBackend(ctx, reader, key)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "list route status serviceimport backend dependencies", "serviceImport", key)
		return nil
	}

	requests := make(map[client.ObjectKey]struct{}, len(routes))
	for _, route := range routes {
		requests[routeKey(route)] = struct{}{}
	}
	return reconcileRequestsForKeys(requests)
}

func reconcileRequestsForKeys(keys map[client.ObjectKey]struct{}) []reconcile.Request {
	if len(keys) == 0 {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(keys))
	for key := range keys {
		requests = append(requests, reconcile.Request{NamespacedName: key})
	}
	return requests
}
