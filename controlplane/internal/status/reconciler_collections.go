package status

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	backendlbv1alpha2 "github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/infrastructure"
)

func (r *Reconciler) reconcileGatewayClasses(ctx context.Context, gatewayClasses []gatewayv1.GatewayClass) error {
	resolver := newGatewayClassStatusSupportResolver(r)
	for _, listed := range gatewayClasses {
		if string(listed.Spec.ControllerName) != r.controllerName {
			continue
		}
		if err := r.reconcileGatewayClassStatusWithSupportResolver(ctx, listed.Name, resolver); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) reconcileGateways(
	ctx context.Context,
	gateways []gatewayv1.Gateway,
	evals map[types.NamespacedName]gatewayEvaluation,
) error {
	refreshCandidates := make([]gatewayv1.Gateway, 0, len(gateways))
	refreshCandidateKeys := make(map[types.NamespacedName]struct{}, len(gateways))
	for _, listed := range gateways {
		key := client.ObjectKeyFromObject(&listed)
		eval, ok := evals[key]
		if !ok {
			continue
		}
		if gatewayNeedsGenerationProbe(listed, eval) {
			refreshCandidates = append(refreshCandidates, listed)
			refreshCandidateKeys[key] = struct{}{}
		}
	}

	currentGenerations := map[types.NamespacedName]int64{}
	if len(refreshCandidates) > 0 {
		var err error
		currentGenerations, err = r.currentGatewayGenerations(ctx, refreshCandidates)
		if err != nil {
			return err
		}
	}
	if err := r.refreshGatewayInfrastructureEvaluations(ctx, gateways, evals, currentGenerations); err != nil {
		return err
	}

	for _, listed := range gateways {
		key := client.ObjectKeyFromObject(&listed)
		eval, ok := evals[key]
		if !ok {
			continue
		}
		if _, needsProbe := refreshCandidateKeys[key]; !needsProbe {
			if err := r.reconcileGatewayStatusWithSeed(ctx, key, &listed, eval); err != nil {
				return err
			}
			continue
		}
		if generation, ok := currentGenerations[key]; ok && generation == eval.sourceGeneration {
			if err := r.reconcileGatewayStatusWithSeed(ctx, key, &listed, eval); err != nil {
				return err
			}
			continue
		}

		var current gatewayv1.Gateway
		if err := r.reader.Get(ctx, key, &current); err != nil {
			if apierrors.IsNotFound(err) {
				deleteGatewayConvergenceStageMetric(key)
				delete(evals, key)
				continue
			}
			return err
		}

		refreshedEval, managed, err := r.refreshGatewayEvaluationWithCurrent(ctx, key, current, eval)
		if err != nil {
			return err
		}
		if !managed {
			deleteGatewayConvergenceStageMetric(key)
			delete(evals, key)
			continue
		}
		evals[key] = refreshedEval

		if err := r.reconcileGatewayStatusWithSeed(ctx, key, &current, refreshedEval); err != nil {
			return err
		}
	}

	return nil
}

func gatewayNeedsGenerationProbe(
	gateway gatewayv1.Gateway,
	eval gatewayEvaluation,
) bool {
	if gatewayRequiresInfrastructureRefresh(eval) {
		return true
	}
	if !conditionObservedGenerationCurrent(
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		gateway.Generation,
	) {
		return true
	}
	if !conditionObservedGenerationCurrent(
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		gateway.Generation,
	) {
		return true
	}
	if len(gateway.Status.Listeners) != len(gateway.Spec.Listeners) {
		return true
	}
	attachedListenerSets := int32(0)
		if gateway.Status.AttachedListenerSets != nil {
			attachedListenerSets = *gateway.Status.AttachedListenerSets
		}
		if attachedListenerSets != int32(eval.attachedListenerSets) {
		return true
	}

	statusByName := make(map[gatewayv1.SectionName]gatewayv1.ListenerStatus, len(gateway.Status.Listeners))
	for _, listener := range gateway.Status.Listeners {
		statusByName[listener.Name] = listener
	}
	for _, listener := range gateway.Spec.Listeners {
		status, ok := statusByName[listener.Name]
		if !ok {
			return true
		}
		if !conditionObservedGenerationCurrent(
			status.Conditions,
			string(gatewayv1.ListenerConditionAccepted),
			gateway.Generation,
		) {
			return true
		}
		if !conditionObservedGenerationCurrent(
			status.Conditions,
			string(gatewayv1.ListenerConditionResolvedRefs),
			gateway.Generation,
		) {
			return true
		}
		if !conditionObservedGenerationCurrent(
			status.Conditions,
			string(gatewayv1.ListenerConditionProgrammed),
			gateway.Generation,
		) {
			return true
		}
	}

	return false
}

func conditionObservedGenerationCurrent(
	conditions []metav1.Condition,
	conditionType string,
	generation int64,
) bool {
	condition := meta.FindStatusCondition(conditions, conditionType)
	return condition != nil && condition.ObservedGeneration == generation
}

func (r *Reconciler) currentGatewayGenerations(
	ctx context.Context,
	gateways []gatewayv1.Gateway,
) (map[types.NamespacedName]int64, error) {
	namespaces := make(map[string]struct{}, len(gateways))
	for _, gateway := range gateways {
		if gateway.Namespace == "" {
			continue
		}
		namespaces[gateway.Namespace] = struct{}{}
	}

	out := make(map[types.NamespacedName]int64, len(gateways))
	for namespace := range namespaces {
		var listed metav1.PartialObjectMetadataList
		listed.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gatewayv1.GroupVersion.Group,
			Version: gatewayv1.GroupVersion.Version,
			Kind:    "GatewayList",
		})
		if err := r.reader.List(ctx, &listed, client.InNamespace(namespace)); err != nil {
			return nil, err
		}
		for _, gateway := range listed.Items {
			out[types.NamespacedName{
				Namespace: gateway.Namespace,
				Name:      gateway.Name,
			}] = gateway.Generation
		}
	}

	return out, nil
}

func (r *Reconciler) refreshGatewayEvaluationWithCurrent(
	ctx context.Context,
	key client.ObjectKey,
	current gatewayv1.Gateway,
	eval gatewayEvaluation,
) (gatewayEvaluation, bool, error) {
	if current.Generation == eval.sourceGeneration {
		return eval, true, nil
	}

	return r.refreshGatewayEvaluation(ctx, key, current)
}

func (r *Reconciler) refreshGatewayEvaluation(
	ctx context.Context,
	key client.ObjectKey,
	gateway gatewayv1.Gateway,
) (gatewayEvaluation, bool, error) {
	state, err := r.loadGatewayObjectState(ctx, gateway)
	if err != nil {
		return gatewayEvaluation{}, false, err
	}

	refreshedEval, ok := evaluateGateways(state, evaluateRouteAttachments(state))[key]
	return refreshedEval, ok, nil
}

func gatewayRequiresInfrastructureRefresh(eval gatewayEvaluation) bool {
	return eval.convergence.serviceMetadataGenerationLag > 0 ||
		eval.convergence.frontendEndpointSliceRefreshRequired
}

func (r *Reconciler) refreshGatewayInfrastructureEvaluations(
	ctx context.Context,
	gateways []gatewayv1.Gateway,
	evals map[types.NamespacedName]gatewayEvaluation,
	currentGenerations map[types.NamespacedName]int64,
) error {
	buckets := make(map[string][]gatewayv1.Gateway)
	for _, gateway := range gateways {
		key := client.ObjectKeyFromObject(&gateway)
		eval, ok := evals[key]
		if !ok || !gatewayRequiresInfrastructureRefresh(eval) {
			continue
		}
		if generation, ok := currentGenerations[key]; !ok || generation != eval.sourceGeneration {
			continue
		}
		buckets[gateway.Namespace] = append(buckets[gateway.Namespace], gateway)
	}

	for namespace, items := range buckets {
		state := r.newClusterState()
		state.gateways = append(state.gateways, items...)
		if err := r.loadGatewayClassesForLoadedGateways(ctx, state); err != nil {
			return err
		}

		managedGateways := make(map[types.NamespacedName]gatewayv1.Gateway, len(state.gateways))
		serviceNames := make(map[string]struct{}, len(state.gateways))
		for _, gateway := range state.gateways {
			key := client.ObjectKeyFromObject(&gateway)
			managedGateways[key] = gateway
			serviceNames[infrastructure.GatewayServiceName(gateway.Name)] = struct{}{}
		}
		if len(serviceNames) == 0 {
			for _, gateway := range items {
				key := client.ObjectKeyFromObject(&gateway)
				deleteGatewayConvergenceStageMetric(key)
				delete(evals, key)
			}
			continue
		}

		if err := r.loadGatewayServicesForNamespace(ctx, state, namespace, serviceNames); err != nil {
			return err
		}
		if err := r.loadGatewayFrontendEndpointSlicesForNamespace(ctx, state, namespace, serviceNames); err != nil {
			return err
		}
		state.index()

		for _, gateway := range items {
			key := client.ObjectKeyFromObject(&gateway)
			managedGateway, ok := managedGateways[key]
			if !ok {
				deleteGatewayConvergenceStageMetric(key)
				delete(evals, key)
				continue
			}
			evals[key] = refreshGatewayInfrastructureEvaluationFromState(
				managedGateway,
				evals[key],
				state,
			)
		}
	}

	return nil
}

func (r *Reconciler) refreshGatewayInfrastructureEvaluation(
	ctx context.Context,
	key client.ObjectKey,
	gateway gatewayv1.Gateway,
	eval gatewayEvaluation,
) (gatewayEvaluation, bool, error) {
	state := r.newClusterState()
	state.gateways = append(state.gateways, gateway)

	if err := r.loadGatewayClassesForLoadedGateways(ctx, state); err != nil {
		return gatewayEvaluation{}, false, err
	}
	if len(state.gateways) == 0 {
		return gatewayEvaluation{}, false, nil
	}
	if err := r.loadGatewayService(ctx, state, gateway); err != nil {
		return gatewayEvaluation{}, false, err
	}
	if err := r.loadGatewayFrontendEndpointSlices(ctx, state, gateway); err != nil {
		return gatewayEvaluation{}, false, err
	}
	state.index()

	return refreshGatewayInfrastructureEvaluationFromState(gateway, eval, state), true, nil
}

func refreshGatewayInfrastructureEvaluationFromState(
	gateway gatewayv1.Gateway,
	eval gatewayEvaluation,
	state *clusterState,
) gatewayEvaluation {
	addressEvaluation := evaluateGatewayAddresses(
		gateway.Spec.Addresses,
		gatewayPublishedAddresses(state, gateway),
		gatewayAdvertisedAddresses(state, gateway),
		gateway.Generation,
	)
	listenersProgrammed := gatewayListenersProgrammed(eval.listeners)
	refreshedEval := eval
	refreshedEval.addresses = addressEvaluation.addresses
	refreshedEval.convergence = gatewayConvergenceObservationForCurrentState(state, gateway)
	refreshedEval.translationReady = listenersProgrammed &&
		addressEvaluation.programmedCondition.Status == metav1.ConditionTrue &&
		!refreshedEval.infraValidation.HasIssues()
	refreshedEval.infraConverged = false
	refreshedEval.programmedCondition = addressEvaluation.programmedCondition

	if !listenersProgrammed && refreshedEval.programmedCondition.Status == metav1.ConditionTrue {
		refreshedEval.programmedCondition.Status = metav1.ConditionFalse
		refreshedEval.programmedCondition.Reason = string(gatewayv1.GatewayReasonListenersNotValid)
		refreshedEval.programmedCondition.Message = "One or more listeners are not programmed"
	}
	if refreshedEval.programmedCondition.Status == metav1.ConditionTrue {
		serviceReady, serviceMessage := gatewayInfrastructureServiceStatus(state, gateway)
		refreshedEval.infraConverged = refreshedEval.translationReady && serviceReady
		if !serviceReady {
			refreshedEval.programmedCondition.Status = metav1.ConditionFalse
			refreshedEval.programmedCondition.Reason = string(gatewayv1.GatewayReasonPending)
			refreshedEval.programmedCondition.Message = serviceMessage
		}
	}
	if refreshedEval.infraValidation.HasIssues() {
		refreshedEval.programmedCondition.Status = metav1.ConditionFalse
		refreshedEval.programmedCondition.Reason = string(gatewayv1.GatewayReasonInvalid)
		refreshedEval.programmedCondition.Message = refreshedEval.infraValidation.Error()
	}

	return refreshedEval
}

func gatewayListenersProgrammed(listeners []listenerEvaluation) bool {
	for _, listener := range listeners {
		if listener.programmedCondition.Status != metav1.ConditionTrue {
			return false
		}
	}
	return true
}

func (r *Reconciler) reconcileHTTPRoutes(
	ctx context.Context,
	_ []gatewayv1.HTTPRoute,
	evals map[types.NamespacedName][]routeParentEvaluation,
) error {
	for key, desiredParents := range evals {
		if err := r.reconcileHTTPRouteStatus(ctx, key, desiredParents); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) reconcileGRPCRoutes(
	ctx context.Context,
	_ []gatewayv1.GRPCRoute,
	evals map[types.NamespacedName][]routeParentEvaluation,
) error {
	for key, desiredParents := range evals {
		if err := r.reconcileGRPCRouteStatus(ctx, key, desiredParents); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) reconcileTCPRoutes(
	ctx context.Context,
	_ []gatewayv1alpha2.TCPRoute,
	evals map[types.NamespacedName][]routeParentEvaluation,
) error {
	for key, desiredParents := range evals {
		if err := r.reconcileTCPRouteStatus(ctx, key, desiredParents); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) reconcileUDPRoutes(
	ctx context.Context,
	_ []gatewayv1alpha2.UDPRoute,
	evals map[types.NamespacedName][]routeParentEvaluation,
) error {
	for key, desiredParents := range evals {
		if err := r.reconcileUDPRouteStatus(ctx, key, desiredParents); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) reconcileTLSRoutes(
	ctx context.Context,
	_ []gatewayv1alpha2.TLSRoute,
	evals map[types.NamespacedName][]routeParentEvaluation,
) error {
	for key, desiredParents := range evals {
		if err := r.reconcileTLSRouteStatus(ctx, key, desiredParents); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) reconcileBackendTLSPolicies(
	ctx context.Context,
	policies []gatewayv1alpha3.BackendTLSPolicy,
	evals map[types.NamespacedName]backendTLSPolicyEvaluation,
) error {
	for _, listed := range policies {
		key := client.ObjectKeyFromObject(&listed)
		if err := r.reconcileBackendTLSPolicyStatus(ctx, key, evals[key]); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) reconcileBackendLBPolicies(
	ctx context.Context,
	policies []backendlbv1alpha2.BackendLBPolicy,
	evals map[types.NamespacedName]backendLBPolicyEvaluation,
) error {
	for _, listed := range policies {
		key := client.ObjectKeyFromObject(&listed)
		if err := r.reconcileBackendLBPolicyStatus(ctx, key, evals[key]); err != nil {
			return err
		}
	}

	return nil
}
