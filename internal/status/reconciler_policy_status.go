package status

import (
	"context"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
)

func (r *Reconciler) reconcileBackendTLSPolicyStatus(
	ctx context.Context,
	key client.ObjectKey,
	eval backendTLSPolicyEvaluation,
) error {
	return r.retryStatusUpdate(ctx, statusUpdateResourceBackendTLSPolicy, func() error {
		currentRaw, current, err := gatewayapi.GetBackendTLSPolicyV1(ctx, r.client, key)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		desiredStatus := current.Status.DeepCopy()
		desiredStatus.Ancestors = mergePolicyAncestors(
			current.Status.Ancestors,
			policyAncestorsWithObservedGeneration(eval.ancestors, current.Generation),
			r.controllerName,
		)

		if apiequality.Semantic.DeepEqual(current.Status, *desiredStatus) {
			return nil
		}
		return gatewayapi.UpdateBackendTLSPolicyV1Status(ctx, r.client, currentRaw, *desiredStatus)
	})
}

func (r *Reconciler) reconcileBackendLBPolicyStatus(
	ctx context.Context,
	key client.ObjectKey,
	eval backendLBPolicyEvaluation,
) error {
	return r.retryStatusUpdate(ctx, statusUpdateResourceBackendLBPolicy, func() error {
		var current backend.BackendLBPolicy
		if err := r.reader.Get(ctx, key, &current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		desired := current.DeepCopy()
		desired.Status.Ancestors = mergePolicyAncestors(
			current.Status.Ancestors,
			policyAncestorsWithObservedGeneration(eval.ancestors, current.Generation),
			r.controllerName,
		)

		if apiequality.Semantic.DeepEqual(current.Status, desired.Status) {
			return nil
		}
		return r.client.Status().Patch(ctx, desired, client.MergeFrom(&current))
	})
}
