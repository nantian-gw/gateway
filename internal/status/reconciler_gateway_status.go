package status

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/gwapi"
)

const (
	gatewayInfrastructureParametersInvalidEventReason  = "InvalidInfrastructureParameters"
	gatewayInfrastructureParametersResolvedEventReason = "InfrastructureParametersResolved"
)

func (r *Reconciler) reconcileGatewayClassStatus(ctx context.Context, name string) error {
	return r.reconcileGatewayClassStatusWithSupportResolver(
		ctx,
		name,
		newGatewayClassStatusSupportResolver(r),
	)
}

func (r *Reconciler) reconcileGatewayClassStatusWithSupportResolver(
	ctx context.Context,
	name string,
	resolver *gatewayClassStatusSupportResolver,
) error {
	return r.retryStatusUpdate(ctx, statusUpdateResourceGatewayClass, func() error {
		var current gatewayv1.GatewayClass
		if err := r.reader.Get(ctx, client.ObjectKey{Name: name}, &current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		supportedVersionCondition, supportedFeatures, err := resolver.resolve(ctx, current.Generation)
		if err != nil {
			return err
		}

		desired := current.DeepCopy()
		setCondition(
			&desired.Status.Conditions,
			conditionSpec{
				Type:               string(gatewayv1.GatewayClassConditionStatusAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.GatewayClassReasonAccepted),
				Message:            "GatewayClass is accepted by nantian-gw",
				ObservedGeneration: desired.Generation,
			},
		)
		setCondition(&desired.Status.Conditions, supportedVersionCondition)
		desired.Status.SupportedFeatures = supportedFeatures

		if apiequality.Semantic.DeepEqual(current.Status, desired.Status) {
			return nil
		}
		return r.client.Status().Update(ctx, desired)
	})
}

func (r *Reconciler) reconcileGatewayStatus(
	ctx context.Context,
	key client.ObjectKey,
	eval gatewayEvaluation,
) error {
	return r.reconcileGatewayStatusWithSeed(ctx, key, nil, eval)
}

func (r *Reconciler) reconcileGatewayStatusWithSeed(
	ctx context.Context,
	key client.ObjectKey,
	seed *gatewayv1.Gateway,
	eval gatewayEvaluation,
) error {
	return r.retryStatusUpdate(ctx, statusUpdateResourceGateway, func() error {
		var current gatewayv1.Gateway
		if seed != nil {
			current = *seed.DeepCopy()
			seed = nil
		} else {
			if err := r.reader.Get(ctx, key, &current); err != nil {
				if apierrors.IsNotFound(err) {
					deleteGatewayConvergenceStageMetric(key)
					return nil
				}
				return err
			}
		}

	eval = gatewayEvaluationWithObservedGeneration(eval, current.Generation)
		observeGatewayConvergenceMetrics(eval.convergence)

		desired := current.DeepCopy()
		desired.Status.Addresses = eval.addresses
		desired.Status.Listeners = mergeListenerStatuses(current.Status.Listeners, eval.listeners)
		desired.Status.AttachedListenerSets = &eval.attachedListenerSets
		setCondition(&desired.Status.Conditions, eval.acceptedCondition)
		setCondition(&desired.Status.Conditions, eval.programmedCondition)
		removeCondition(
			&desired.Status.Conditions,
			string(gatewayv1.GatewayConditionInsecureFrontendValidationMode),
		)
		removeCondition(
			&desired.Status.Conditions,
			string(gatewayv1.GatewayConditionResolvedRefs),
		)
		removeCondition(
			&desired.Status.Conditions,
			gwapi.GatewayConditionDefaultGateway,
		)
		for _, extra := range eval.extraConditions {
			setCondition(&desired.Status.Conditions, extra)
		}

		currentInfraMessage := gatewayInfrastructureParametersMessage(current.Status.Conditions)
		desiredInfraMessage := eval.infraValidation.Error()
		if apiequality.Semantic.DeepEqual(current.Status, desired.Status) {
			updateGatewayConvergenceStageMetric(key, eval)
			return nil
		}
		if err := r.client.Status().Update(ctx, desired); err != nil {
			if apierrors.IsNotFound(err) {
				deleteGatewayConvergenceStageMetric(key)
				return nil
			}
			return err
		}

		r.emitGatewayInfrastructureParameterEvent(&current, currentInfraMessage, desiredInfraMessage)
		updateGatewayConvergenceStageMetric(key, eval)
		return nil
	})
}

func gatewayInfrastructureParametersMessage(conditions []metav1.Condition) string {
	accepted := meta.FindStatusCondition(conditions, string(gatewayv1.GatewayConditionAccepted))
	if accepted == nil || accepted.Reason != string(gatewayv1.GatewayReasonInvalidParameters) {
		return ""
	}

	message := strings.TrimSpace(accepted.Message)
	if strings.Contains(message, "Gateway.spec.infrastructure.parametersRef") ||
		strings.Contains(message, "GatewayClass.spec.parametersRef") {
		return message
	}

	return ""
}

func (r *Reconciler) emitGatewayInfrastructureParameterEvent(
	gateway *gatewayv1.Gateway,
	currentMessage string,
	desiredMessage string,
) {
	if gateway == nil || currentMessage == desiredMessage {
		return
	}

	switch {
	case desiredMessage != "":
		r.recorder.Eventf(
			gateway,
			corev1.EventTypeWarning,
			gatewayInfrastructureParametersInvalidEventReason,
			"%s",
			desiredMessage,
		)
	case currentMessage != "":
		r.recorder.Eventf(
			gateway,
			corev1.EventTypeNormal,
			gatewayInfrastructureParametersResolvedEventReason,
			"Gateway infrastructure parameters are resolved",
		)
	}
}
