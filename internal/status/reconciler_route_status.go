package status

import (
	"context"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func (r *Reconciler) reconcileHTTPRouteStatus(
	ctx context.Context,
	key client.ObjectKey,
	desiredParents []routeParentEvaluation,
) error {
	return r.retryStatusUpdate(statusUpdateResourceHTTPRoute, func() error {
		var current gatewayv1.HTTPRoute
		if err := r.reader.Get(ctx, key, &current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		desiredParents = routeParentEvaluationsWithObservedGeneration(desiredParents, current.Generation)

		desired := current.DeepCopy()
		desired.Status.Parents = mergeRouteParents(current.Status.Parents, desiredParents)
		if apiequality.Semantic.DeepEqual(current.Status, desired.Status) {
			return nil
		}
		return r.client.Status().Patch(ctx, desired, client.MergeFrom(&current))
	})
}

func (r *Reconciler) reconcileGRPCRouteStatus(
	ctx context.Context,
	key client.ObjectKey,
	desiredParents []routeParentEvaluation,
) error {
	return r.retryStatusUpdate(statusUpdateResourceGRPCRoute, func() error {
		var current gatewayv1.GRPCRoute
		if err := r.reader.Get(ctx, key, &current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		desiredParents = routeParentEvaluationsWithObservedGeneration(desiredParents, current.Generation)

		desired := current.DeepCopy()
		desired.Status.Parents = mergeRouteParents(current.Status.Parents, desiredParents)
		if apiequality.Semantic.DeepEqual(current.Status, desired.Status) {
			return nil
		}
		return r.client.Status().Patch(ctx, desired, client.MergeFrom(&current))
	})
}

func (r *Reconciler) reconcileTCPRouteStatus(
	ctx context.Context,
	key client.ObjectKey,
	desiredParents []routeParentEvaluation,
) error {
	return r.retryStatusUpdate(statusUpdateResourceTCPRoute, func() error {
		var current gatewayv1alpha2.TCPRoute
		if err := r.reader.Get(ctx, key, &current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		desiredParents = routeParentEvaluationsWithObservedGeneration(desiredParents, current.Generation)

		desired := current.DeepCopy()
		desired.Status.Parents = mergeRouteParents(current.Status.Parents, desiredParents)
		if apiequality.Semantic.DeepEqual(current.Status, desired.Status) {
			return nil
		}
		return r.client.Status().Patch(ctx, desired, client.MergeFrom(&current))
	})
}

func (r *Reconciler) reconcileUDPRouteStatus(
	ctx context.Context,
	key client.ObjectKey,
	desiredParents []routeParentEvaluation,
) error {
	return r.retryStatusUpdate(statusUpdateResourceUDPRoute, func() error {
		var current gatewayv1alpha2.UDPRoute
		if err := r.reader.Get(ctx, key, &current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		desiredParents = routeParentEvaluationsWithObservedGeneration(desiredParents, current.Generation)

		desired := current.DeepCopy()
		desired.Status.Parents = mergeRouteParents(current.Status.Parents, desiredParents)
		if apiequality.Semantic.DeepEqual(current.Status, desired.Status) {
			return nil
		}
		return r.client.Status().Patch(ctx, desired, client.MergeFrom(&current))
	})
}

func (r *Reconciler) reconcileTLSRouteStatus(
	ctx context.Context,
	key client.ObjectKey,
	desiredParents []routeParentEvaluation,
) error {
	return r.retryStatusUpdate(statusUpdateResourceTLSRoute, func() error {
		var current gatewayv1alpha2.TLSRoute
		if err := r.reader.Get(ctx, key, &current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		desiredParents = routeParentEvaluationsWithObservedGeneration(desiredParents, current.Generation)

		desired := current.DeepCopy()
		desired.Status.Parents = mergeRouteParents(current.Status.Parents, desiredParents)
		if apiequality.Semantic.DeepEqual(current.Status, desired.Status) {
			return nil
		}
		return r.client.Status().Patch(ctx, desired, client.MergeFrom(&current))
	})
}
