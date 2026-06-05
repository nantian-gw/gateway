package status

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapi"
)

func (r *Reconciler) reconcileListenerSetStatuses(ctx context.Context, lses []gatewayv1.ListenerSet, evals map[string]listenerSetEvaluation) error {
	for i := range lses {
		key := client.ObjectKeyFromObject(&lses[i])
		eval, ok := evals[fmt.Sprintf("%s/%s", lses[i].Namespace, lses[i].Name)]
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
	return r.retryStatusUpdate(ctx, "listenerset", func() error {
		currentRaw, current, err := gatewayapi.GetListenerSetV1(ctx, r.client, key)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		if currentRaw == nil {
			return nil
		}

		eval = listenerSetEvaluationWithObservedGeneration(eval, current.Generation)
		desired := current.Status.DeepCopy()
		setCondition(&desired.Conditions, eval.accepted)
		setCondition(&desired.Conditions, eval.programmed)
		desired.Listeners = eval.listeners

		if apiequality.Semantic.DeepEqual(current.Status, *desired) {
			return nil
		}
		return gatewayapi.UpdateListenerSetV1Status(ctx, r.client, currentRaw, *desired)
	})
}