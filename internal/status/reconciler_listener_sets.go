package status

import (
	"context"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
)

func (r *Reconciler) reconcileListenerSetStatuses(ctx context.Context, lses []gatewayv1.ListenerSet, evals map[string]listenerSetEvaluation) error {
	for i := range lses {
		key := client.ObjectKeyFromObject(&lses[i])
		eval, ok := evals[lses[i].Namespace+"/"+lses[i].Name]
		if !ok {
			continue
		}
		if err := r.reconcileListenerSetStatus(ctx, key, eval); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcileListenerSetStatus(
	ctx context.Context,
	key client.ObjectKey,
	eval listenerSetEvaluation,
) error {
	return r.retryStatusUpdate("listenerset", func() error {
		var current gatewayv1.ListenerSet
		if err := r.reader.Get(ctx, key, &current); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			if err := r.client.Get(ctx, key, &current); err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}
		}
		currentRaw := current.DeepCopy()

		generation := current.Generation
		if evalGeneration := listenerSetEvaluationObservedGeneration(eval); evalGeneration > generation {
			generation = evalGeneration
		}
		eval = listenerSetEvaluationWithObservedGeneration(eval, generation)
		desired := current.Status.DeepCopy()
		setCondition(&desired.Conditions, eval.accepted)
		setCondition(&desired.Conditions, eval.programmed)
		desired.Listeners = eval.listeners

		if apiequality.Semantic.DeepEqual(current.Status, *desired) {
			return nil
		}
		if err := gatewayapi.UpdateListenerSetV1Status(ctx, r.client, currentRaw, *desired); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return nil
	})
}
