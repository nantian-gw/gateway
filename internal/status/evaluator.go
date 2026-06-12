package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/infrastructure"
)

func evaluateRoutes(state *clusterState) routeState {
	out := routeState{
		http:        make(map[client.ObjectKey][]routeParentEvaluation),
		grpc:        make(map[client.ObjectKey][]routeParentEvaluation),
		tcp:         make(map[client.ObjectKey][]routeParentEvaluation),
		udp:         make(map[client.ObjectKey][]routeParentEvaluation),
		tls:         make(map[client.ObjectKey][]routeParentEvaluation),
		attachments: make(map[listenerKey]map[string]struct{}),
	}

	for _, route := range state.httpRoutes {
		key := client.ObjectKeyFromObject(&route)
		evals := evaluateRoute(state, httpRouteInput(route))
		out.http[key] = evals
		recordAttachments(out.attachments, key, evals)
	}

	for _, route := range state.grpcRoutes {
		key := client.ObjectKeyFromObject(&route)
		evals := evaluateRoute(state, grpcRouteInput(route))
		out.grpc[key] = evals
		recordAttachments(out.attachments, key, evals)
	}

	for _, route := range state.tcpRoutes {
		key := client.ObjectKeyFromObject(&route)
		evals := evaluateRoute(state, tcpRouteInput(route))
		out.tcp[key] = evals
		recordAttachments(out.attachments, key, evals)
	}

	for _, route := range state.udpRoutes {
		key := client.ObjectKeyFromObject(&route)
		evals := evaluateRoute(state, udpRouteInput(route))
		out.udp[key] = evals
		recordAttachments(out.attachments, key, evals)
	}

	for _, route := range state.tlsRoutes {
		key := client.ObjectKeyFromObject(&route)
		evals := evaluateRoute(state, tlsRouteInput(route))
		out.tls[key] = evals
		recordAttachments(out.attachments, key, evals)
	}

	return out
}

func evaluateGateways(state *clusterState, attachments map[listenerKey]map[string]struct{}) map[client.ObjectKey]gatewayEvaluation {
	out := make(map[client.ObjectKey]gatewayEvaluation, len(state.managedGateways))

	for _, gateway := range state.managedGateways {
		key := client.ObjectKeyFromObject(&gateway)
		listenerEvals := make([]listenerEvaluation, 0, len(gateway.Spec.Listeners))
		listenersProgrammed := true
		acceptedListeners := 0
		invalidListeners := 0

		for _, listener := range gateway.Spec.Listeners {
			key := listenerKey{
				gatewayNamespace: gateway.Namespace,
				gatewayName:      gateway.Name,
				listenerName:     listener.Name,
			}
			eval := evaluateGatewayListener(
				state,
				gateway,
				gateway.Spec.Listeners,
				listener,
				int32(len(attachments[key])),
			)
			if eval.acceptedCondition.Status == metav1.ConditionTrue {
				acceptedListeners++
			} else {
				invalidListeners++
			}
			if eval.programmedCondition.Status != metav1.ConditionTrue {
				listenersProgrammed = false
			}
			listenerEvals = append(listenerEvals, eval)
		}

		for _, eval := range evaluateGatewayListenerSetListeners(state, gateway, state.listenerSets, attachments) {
			if eval.acceptedCondition.Status == metav1.ConditionTrue {
				acceptedListeners++
			} else {
				invalidListeners++
			}
			if eval.programmedCondition.Status != metav1.ConditionTrue {
				listenersProgrammed = false
			}
			listenerEvals = append(listenerEvals, eval)
		}

		addressEvaluation := evaluateGatewayAddresses(
			gateway.Spec.Addresses,
			gatewayPublishedAddresses(state, gateway),
			gatewayAdvertisedAddresses(state, gateway),
			gateway.Generation,
		)
		infraValidation := infrastructure.ValidateGatewayInfrastructureParameters(
			gateway,
			state.managedGatewayClasses,
			state.configMapByKey,
		)
		translationReady := listenersProgrammed &&
			addressEvaluation.programmedCondition.Status == metav1.ConditionTrue &&
			!infraValidation.HasIssues()
		serviceReady := false
		serviceMessage := ""
		if translationReady {
			serviceReady, serviceMessage = gatewayInfrastructureServiceStatus(state, gateway)
		}
		gatewayEval := gatewayEvaluation{
			sourceGeneration:     gateway.Generation,
			addresses:            addressEvaluation.addresses,
			acceptedCondition:    addressEvaluation.acceptedCondition,
			programmedCondition:  addressEvaluation.programmedCondition,
			extraConditions:      gatewayExtraConditions(state, gateway),
			listeners:            listenerEvals,
			infraValidation:      infraValidation,
			convergence:          gatewayConvergenceObservationForCurrentState(state, gateway),
			translationReady:     translationReady,
			infraConverged:       translationReady && serviceReady,
			attachedListenerSets: countAttachedListenerSets(state, gateway),
		}
		if gatewayEval.acceptedCondition.Status == metav1.ConditionTrue && invalidListeners > 0 {
			gatewayEval.acceptedCondition.Reason = string(gatewayv1.GatewayReasonListenersNotValid)
			if acceptedListeners > 0 {
				gatewayEval.acceptedCondition.Message = "One or more listeners are not accepted"
			} else {
				gatewayEval.acceptedCondition.Status = metav1.ConditionFalse
				gatewayEval.acceptedCondition.Message = "No listeners are accepted"
			}
		}
		if !listenersProgrammed && gatewayEval.programmedCondition.Status == metav1.ConditionTrue {
			gatewayEval.programmedCondition.Status = metav1.ConditionFalse
			gatewayEval.programmedCondition.Reason = string(gatewayv1.GatewayReasonListenersNotValid)
			gatewayEval.programmedCondition.Message = "One or more listeners are not programmed"
		}
		if gatewayEval.programmedCondition.Status == metav1.ConditionTrue {
			if !serviceReady {
				gatewayEval.programmedCondition.Status = metav1.ConditionFalse
				gatewayEval.programmedCondition.Reason = string(gatewayv1.GatewayReasonPending)
				gatewayEval.programmedCondition.Message = serviceMessage
			}
		}
		if infraValidation.HasIssues() {
			gatewayEval.acceptedCondition.Status = metav1.ConditionFalse
			gatewayEval.acceptedCondition.Reason = string(gatewayv1.GatewayReasonInvalidParameters)
			gatewayEval.acceptedCondition.Message = infraValidation.Error()
			gatewayEval.programmedCondition.Status = metav1.ConditionFalse
			gatewayEval.programmedCondition.Reason = string(gatewayv1.GatewayReasonInvalid)
			gatewayEval.programmedCondition.Message = infraValidation.Error()
		}

		out[key] = gatewayEval
	}

	return out
}
