package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
)

func (s *Syncer) shouldBypassSettleDelay(ctx context.Context, request ctrl.Request) bool {
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotHTTPRoutesReconcileRequestName); ok {
		return s.liveHTTPRouteUsesOnlyServiceParents(ctx, key) ||
			currentHTTPRouteUsesOnlyServiceParents(s.store.Current(), key)
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotGRPCRoutesReconcileRequestName); ok {
		return s.liveGRPCRouteUsesOnlyServiceParents(ctx, key) ||
			currentGRPCRouteUsesOnlyServiceParents(s.store.Current(), key)
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotTCPRoutesReconcileRequestName); ok {
		return s.liveTCPRouteUsesOnlyServiceParents(ctx, key) ||
			currentStreamRouteUsesOnlyServiceParents(s.store.Current(), key, "TCP")
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotUDPRoutesReconcileRequestName); ok {
		return s.liveUDPRouteUsesOnlyServiceParents(ctx, key) ||
			currentStreamRouteUsesOnlyServiceParents(s.store.Current(), key, "UDP")
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotTLSRoutesReconcileRequestName); ok {
		return s.liveTLSRouteUsesOnlyServiceParents(ctx, key) ||
			currentStreamRouteUsesOnlyServiceParents(s.store.Current(), key, "TLS")
	}

	return false
}

func (s *Syncer) liveHTTPRouteUsesOnlyServiceParents(ctx context.Context, key client.ObjectKey) bool {
	route := &gatewayv1.HTTPRoute{}
	if err := s.client.Get(ctx, key, route); err != nil {
		return false
	}
	return mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace)
}

func (s *Syncer) liveGRPCRouteUsesOnlyServiceParents(ctx context.Context, key client.ObjectKey) bool {
	route := &gatewayv1.GRPCRoute{}
	if err := s.client.Get(ctx, key, route); err != nil {
		return false
	}
	return mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace)
}

func (s *Syncer) liveTCPRouteUsesOnlyServiceParents(ctx context.Context, key client.ObjectKey) bool {
	route := &gatewayv1alpha2.TCPRoute{}
	if err := s.client.Get(ctx, key, route); err != nil {
		return false
	}
	return mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace)
}

func (s *Syncer) liveUDPRouteUsesOnlyServiceParents(ctx context.Context, key client.ObjectKey) bool {
	route := &gatewayv1alpha2.UDPRoute{}
	if err := s.client.Get(ctx, key, route); err != nil {
		return false
	}
	return mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace)
}

func (s *Syncer) liveTLSRouteUsesOnlyServiceParents(ctx context.Context, key client.ObjectKey) bool {
	route := &gatewayv1alpha2.TLSRoute{}
	if err := s.client.Get(ctx, key, route); err != nil {
		return false
	}
	return mesh.RouteUsesOnlyServiceParents(route.Spec.ParentRefs, route.Namespace)
}

func currentHTTPRouteUsesOnlyServiceParents(snapshot *ir.Snapshot, key client.ObjectKey) bool {
	if snapshot == nil {
		return false
	}
	for _, route := range snapshot.HTTPRoutes {
		if route.Namespace == key.Namespace && route.Name == key.Name {
			return currentRouteUsesOnlyServiceParents(route.ParentRefs)
		}
	}
	return false
}

func currentGRPCRouteUsesOnlyServiceParents(snapshot *ir.Snapshot, key client.ObjectKey) bool {
	if snapshot == nil {
		return false
	}
	for _, route := range snapshot.GRPCRoutes {
		if route.Namespace == key.Namespace && route.Name == key.Name {
			return currentRouteUsesOnlyServiceParents(route.ParentRefs)
		}
	}
	return false
}

func currentStreamRouteUsesOnlyServiceParents(snapshot *ir.Snapshot, key client.ObjectKey, kind string) bool {
	if snapshot == nil {
		return false
	}
	for _, route := range snapshot.StreamRoutes {
		if route.Kind == kind && route.Namespace == key.Namespace && route.Name == key.Name {
			return currentRouteUsesOnlyServiceParents(route.ParentRefs)
		}
	}
	return false
}

func currentRouteUsesOnlyServiceParents(parentRefs []ir.ParentRef) bool {
	if len(parentRefs) == 0 {
		return false
	}
	for _, parentRef := range parentRefs {
		if parentRef.Group != "" || parentRef.Kind != mesh.FrontendKindService {
			return false
		}
	}
	return true
}
