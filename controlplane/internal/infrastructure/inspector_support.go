package infrastructure

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/mesh"
)

type infrastructureExpectation struct {
	resource      InfrastructureResource
	service       *corev1.Service
	endpointSlice *discoveryv1.EndpointSlice
}

func (r *Reconciler) expectedInfrastructure(
	ctx context.Context,
	gateways []gatewayv1.Gateway,
	meshParentKeys []mesh.ServiceParentKey,
	services []corev1.Service,
	serviceIndex map[string]corev1.Service,
) (map[string]infrastructureExpectation, map[string]infrastructureExpectation, error) {
	expectedServices := make(map[string]infrastructureExpectation)
	expectedSlices := make(map[string]infrastructureExpectation)
	parameterResolver := newGatewayServiceParameterResolver(r)

	var dataplanePods corev1.PodList
	if err := r.client.List(
		ctx,
		&dataplanePods,
		client.InNamespace(r.options.DataplaneNamespace),
		client.MatchingLabels(r.options.DataplaneSelector),
	); err != nil {
		return nil, nil, err
	}

	sharedPods, gatewayPods, meshPods := r.frontendEligibleDataplanePodSets(ctx, dataplanePods.Items)

	currentShared := serviceIndex[serviceKey(r.options.DataplaneNamespace, r.options.SharedServiceName)]
	if desiredShared := desiredSharedService(serviceOrEmpty(currentShared), gateways, r.options); desiredShared != nil {
		addServiceExpectation(
			expectedServices,
			desiredShared,
			InfrastructureRoleSharedSvc,
			"",
			"",
			"",
		)
		for _, endpointSlice := range desiredFrontendEndpointSlices(
			*desiredShared,
			sharedPods,
			sharedEndpointSliceRoleValue,
			sharedEndpointSliceNamePrefix,
		) {
			addEndpointSliceExpectation(
				expectedSlices,
				endpointSlice,
				InfrastructureRoleSharedSlice,
				InfrastructureKindService,
				desiredShared.Namespace,
				desiredShared.Name,
			)
		}
	}

	for _, gateway := range gateways {
		current := serviceIndex[serviceKey(gateway.Namespace, gatewayServiceName(gateway.Name))]
		params := parameterResolver.resolve(ctx, gateway)
		desired := desiredGatewayService(
			serviceOrEmpty(current),
			gateway,
			params,
			parameterResolver.gatewayClassParametersReference(ctx, gateway),
		)
		if desired == nil {
			continue
		}
		addServiceExpectation(
			expectedServices,
			desired,
			InfrastructureRoleGatewaySvc,
			"Gateway",
			gateway.Namespace,
			gateway.Name,
		)
		for _, endpointSlice := range desiredFrontendEndpointSlices(
			*desired,
			gatewayPods,
			gatewayEndpointSliceRoleValue,
			gatewayEndpointSliceNamePrefix,
		) {
			addEndpointSliceExpectation(
				expectedSlices,
				endpointSlice,
				InfrastructureRoleGatewaySlice,
				"Gateway",
				gateway.Namespace,
				gateway.Name,
			)
		}
	}

	servicesByKey := make(map[string]corev1.Service)
	shadowByOriginal := make(map[string]corev1.Service)
	for _, service := range services {
		if service.Labels[mesh.ShadowServiceRoleLabel] == mesh.ShadowServiceRoleValue {
			key := service.Labels[mesh.OriginalServiceNamespaceLabel] + "/" + service.Labels[mesh.OriginalServiceNameLabel]
			if key != "/" {
				shadowByOriginal[key] = service
			}
			continue
		}
		servicesByKey[serviceKey(service.Namespace, service.Name)] = service
	}

	frontendsByService := make(map[string][]mesh.ServiceFrontendPort)
	for _, frontend := range mesh.ExpandServiceFrontends(
		services,
		meshParentKeys,
	) {
		key := serviceKey(frontend.Namespace, frontend.Name)
		frontendsByService[key] = append(frontendsByService[key], frontend)
	}

	for key, frontends := range frontendsByService {
		current, ok := servicesByKey[key]
		if !ok {
			continue
		}

		shadow := shadowByOriginal[key]
		source := sourceService(current, shadow)
		shadowName := mesh.ShadowServiceName(current.Namespace, current.Name)
		desiredShadow := desiredShadowService(
			shadow,
			current.Namespace,
			current.Name,
			shadowName,
			source,
		)
		desiredFrontend := desiredMeshFrontendService(current, shadowName, source, frontends)
		addServiceExpectation(
			expectedServices,
			desiredFrontend,
			InfrastructureRoleMeshFrontendSvc,
			InfrastructureKindService,
			current.Namespace,
			current.Name,
		)
		addServiceExpectation(
			expectedServices,
			desiredShadow,
			InfrastructureRoleMeshShadowSvc,
			InfrastructureKindService,
			current.Namespace,
			current.Name,
		)
		for _, endpointSlice := range desiredMeshEndpointSlices(*desiredFrontend, meshPods) {
			addEndpointSliceExpectation(
				expectedSlices,
				endpointSlice,
				InfrastructureRoleMeshSlice,
				InfrastructureKindService,
				current.Namespace,
				current.Name,
			)
		}
	}

	return expectedServices, expectedSlices, nil
}

func (r *Reconciler) loadMeshServiceParents(ctx context.Context) ([]mesh.ServiceParentKey, error) {
	if r.store != nil {
		if snapshot := r.store.Current(); snapshotHasMeshParentSource(snapshot) {
			return meshServiceParentsFromSnapshot(snapshot), nil
		}
	}

	httpRoutes, err := listHTTPRoutesWithServiceParents(ctx, r.client)
	if err != nil {
		return nil, err
	}
	grpcRoutes, err := listGRPCRoutesWithServiceParents(ctx, r.client)
	if err != nil {
		return nil, err
	}
	tcpRoutes, err := listTCPRoutesWithServiceParents(ctx, r.client)
	if err != nil {
		return nil, err
	}
	udpRoutes, err := listUDPRoutesWithServiceParents(ctx, r.client)
	if err != nil {
		return nil, err
	}
	tlsRoutes, err := listTLSRoutesWithServiceParents(ctx, r.client)
	if err != nil {
		return nil, err
	}

	return collectMeshServiceParents(
		httpRoutes,
		grpcRoutes,
		tcpRoutes,
		udpRoutes,
		tlsRoutes,
	), nil
}

func snapshotHasMeshParentSource(snapshot *ir.Snapshot) bool {
	if snapshot == nil {
		return false
	}

	return len(snapshot.Listeners) > 0 ||
		len(snapshot.HTTPRoutes) > 0 ||
		len(snapshot.GRPCRoutes) > 0 ||
		len(snapshot.StreamRoutes) > 0 ||
		len(snapshot.Backends) > 0 ||
		len(snapshot.Secrets) > 0 ||
		len(snapshot.Workloads) > 0
}

func meshServiceParentsFromSnapshot(snapshot *ir.Snapshot) []mesh.ServiceParentKey {
	if snapshot == nil {
		return nil
	}

	keys := make(map[string]mesh.ServiceParentKey)
	addParentRefs := func(parentRefs []ir.ParentRef, defaultNamespace string) {
		for _, parentRef := range parentRefs {
			kind := parentRef.Kind
			if kind == "" {
				kind = mesh.FrontendKindService
			}
			if parentRef.Group != "" || kind != mesh.FrontendKindService {
				continue
			}

			namespace := parentRef.Namespace
			if namespace == "" {
				namespace = defaultNamespace
			}
			if namespace == "" || parentRef.Name == "" {
				continue
			}

			key := serviceKey(namespace, parentRef.Name)
			keys[key] = mesh.ServiceParentKey{
				Namespace: namespace,
				Name:      parentRef.Name,
			}
		}
	}

	for _, route := range snapshot.HTTPRoutes {
		addParentRefs(route.ParentRefs, route.Namespace)
	}
	for _, route := range snapshot.GRPCRoutes {
		addParentRefs(route.ParentRefs, route.Namespace)
	}
	for _, route := range snapshot.StreamRoutes {
		addParentRefs(route.ParentRefs, route.Namespace)
	}

	out := make([]mesh.ServiceParentKey, 0, len(keys))
	for _, key := range sortedServiceKeys(serviceKeySetFromMeshParents(keys)) {
		out = append(out, keys[key])
	}
	return out
}

func serviceKeySetFromMeshParents(keys map[string]mesh.ServiceParentKey) map[string]struct{} {
	out := make(map[string]struct{}, len(keys))
	for key := range keys {
		out[key] = struct{}{}
	}
	return out
}

func (r *Reconciler) loadObservedServices(
	ctx context.Context,
	gateways []gatewayv1.Gateway,
	meshParentKeys []mesh.ServiceParentKey,
) ([]corev1.Service, map[string]corev1.Service, error) {
	var managedServices corev1.ServiceList
	serviceSelector, err := infrastructureServiceSelector()
	if err != nil {
		return nil, nil, err
	}
	if err := r.client.List(
		ctx,
		&managedServices,
		client.MatchingLabelsSelector{Selector: serviceSelector},
	); err != nil {
		return nil, nil, err
	}

	observed := make([]corev1.Service, 0, len(managedServices.Items))
	index := make(map[string]corev1.Service, len(managedServices.Items))
	add := func(service corev1.Service) {
		key := serviceKey(service.Namespace, service.Name)
		if _, exists := index[key]; exists {
			return
		}
		index[key] = service
		observed = append(observed, service)
	}

	for _, service := range managedServices.Items {
		add(service)
	}

	requested := make(map[string]struct{})
	load := func(key client.ObjectKey) error {
		id := serviceKey(key.Namespace, key.Name)
		if _, exists := index[id]; exists {
			return nil
		}
		if _, exists := requested[id]; exists {
			return nil
		}
		requested[id] = struct{}{}

		current := &corev1.Service{}
		if err := r.client.Get(ctx, key, current); client.IgnoreNotFound(err) != nil {
			return err
		}
		if current.Name != "" {
			add(*current)
		}
		return nil
	}

	if err := load(client.ObjectKey{
		Namespace: r.options.DataplaneNamespace,
		Name:      r.options.SharedServiceName,
	}); err != nil {
		return nil, nil, err
	}
	for _, gateway := range gateways {
		if err := load(gatewayServiceObjectKey(gateway)); err != nil {
			return nil, nil, err
		}
	}
	for _, parentKey := range meshParentKeys {
		if err := load(client.ObjectKey{
			Namespace: parentKey.Namespace,
			Name:      parentKey.Name,
		}); err != nil {
			return nil, nil, err
		}
		if err := load(client.ObjectKey{
			Namespace: parentKey.Namespace,
			Name:      mesh.ShadowServiceName(parentKey.Namespace, parentKey.Name),
		}); err != nil {
			return nil, nil, err
		}
	}

	return observed, index, nil
}

func (r *Reconciler) loadObservedEndpointSlices(
	ctx context.Context,
	expected map[string]infrastructureExpectation,
) ([]discoveryv1.EndpointSlice, map[string]discoveryv1.EndpointSlice, error) {
	var managedSlices discoveryv1.EndpointSliceList
	sliceSelector, err := infrastructureEndpointSliceSelector()
	if err != nil {
		return nil, nil, err
	}
	if err := r.client.List(
		ctx,
		&managedSlices,
		client.MatchingLabelsSelector{Selector: sliceSelector},
	); err != nil {
		return nil, nil, err
	}

	observed := make([]discoveryv1.EndpointSlice, 0, len(managedSlices.Items)+len(expected))
	index := make(map[string]discoveryv1.EndpointSlice, len(managedSlices.Items)+len(expected))
	add := func(endpointSlice discoveryv1.EndpointSlice) {
		key := serviceKey(endpointSlice.Namespace, endpointSlice.Name)
		if _, exists := index[key]; exists {
			return
		}
		index[key] = endpointSlice
		observed = append(observed, endpointSlice)
	}

	for _, endpointSlice := range managedSlices.Items {
		add(endpointSlice)
	}

	for _, key := range sortedExpectationKeys(expected) {
		if _, exists := index[key]; exists {
			continue
		}
		current := &discoveryv1.EndpointSlice{}
		if err := r.client.Get(
			ctx,
			client.ObjectKey{
				Namespace: expected[key].resource.Namespace,
				Name:      expected[key].resource.Name,
			},
			current,
		); client.IgnoreNotFound(err) != nil {
			return nil, nil, err
		}
		if current.Name != "" {
			add(*current)
		}
	}

	return observed, index, nil
}

func infrastructureServiceSelector() (labels.Selector, error) {
	managedByReq, err := labels.NewRequirement(managedByLabel, selection.Equals, []string{managedByValue})
	if err != nil {
		return nil, err
	}
	roleReq, err := labels.NewRequirement(
		serviceRoleLabel,
		selection.In,
		[]string{
			serviceRoleShared,
			serviceRoleGateway,
			serviceRoleMeshFrontend,
			mesh.ShadowServiceRoleValue,
		},
	)
	if err != nil {
		return nil, err
	}
	return labels.NewSelector().Add(*managedByReq, *roleReq), nil
}

func infrastructureEndpointSliceSelector() (labels.Selector, error) {
	managedByReq, err := labels.NewRequirement(discoveryv1.LabelManagedBy, selection.Equals, []string{managedByValue})
	if err != nil {
		return nil, err
	}
	roleReq, err := labels.NewRequirement(
		serviceRoleLabel,
		selection.In,
		[]string{
			sharedEndpointSliceRoleValue,
			gatewayEndpointSliceRoleValue,
			meshEndpointSliceRoleValue,
		},
	)
	if err != nil {
		return nil, err
	}
	return labels.NewSelector().Add(*managedByReq, *roleReq), nil
}

func addServiceExpectation(
	target map[string]infrastructureExpectation,
	service *corev1.Service,
	role string,
	ownerKind string,
	ownerNamespace string,
	ownerName string,
) {
	if service == nil {
		return
	}
	target[serviceKey(service.Namespace, service.Name)] = infrastructureExpectation{
		resource: InfrastructureResource{
			Kind:           InfrastructureKindService,
			Namespace:      service.Namespace,
			Name:           service.Name,
			Role:           role,
			OwnerKind:      ownerKind,
			OwnerNamespace: ownerNamespace,
			OwnerName:      ownerName,
		},
		service: service.DeepCopy(),
	}
}

func addEndpointSliceExpectation(
	target map[string]infrastructureExpectation,
	endpointSlice *discoveryv1.EndpointSlice,
	role string,
	ownerKind string,
	ownerNamespace string,
	ownerName string,
) {
	if endpointSlice == nil {
		return
	}
	target[serviceKey(endpointSlice.Namespace, endpointSlice.Name)] = infrastructureExpectation{
		resource: InfrastructureResource{
			Kind:           InfrastructureKindSlice,
			Namespace:      endpointSlice.Namespace,
			Name:           endpointSlice.Name,
			Role:           role,
			OwnerKind:      ownerKind,
			OwnerNamespace: ownerNamespace,
			OwnerName:      ownerName,
		},
		endpointSlice: endpointSlice.DeepCopy(),
	}
}

func classifyObservedService(service corev1.Service) (InfrastructureResource, bool) {
	switch {
	case service.Labels[managedByLabel] == managedByValue &&
		service.Labels[mesh.ShadowServiceRoleLabel] == mesh.ShadowServiceRoleValue:
		return InfrastructureResource{
			Kind:           InfrastructureKindService,
			Namespace:      service.Namespace,
			Name:           service.Name,
			Role:           InfrastructureRoleMeshShadowSvc,
			OwnerKind:      InfrastructureKindService,
			OwnerNamespace: service.Labels[mesh.OriginalServiceNamespaceLabel],
			OwnerName:      service.Labels[mesh.OriginalServiceNameLabel],
		}, true
	case service.Labels[managedByLabel] == managedByValue &&
		service.Labels[serviceRoleLabel] == serviceRoleShared:
		return InfrastructureResource{
			Kind:      InfrastructureKindService,
			Namespace: service.Namespace,
			Name:      service.Name,
			Role:      InfrastructureRoleSharedSvc,
		}, true
	case service.Labels[managedByLabel] == managedByValue &&
		service.Labels[serviceRoleLabel] == serviceRoleGateway:
		return InfrastructureResource{
			Kind:           InfrastructureKindService,
			Namespace:      service.Namespace,
			Name:           service.Name,
			Role:           InfrastructureRoleGatewaySvc,
			OwnerKind:      "Gateway",
			OwnerNamespace: service.Labels[gatewayNamespaceLabel],
			OwnerName:      service.Labels[gatewayNameLabel],
		}, true
	case service.Labels[managedByLabel] == managedByValue &&
		service.Labels[serviceRoleLabel] == serviceRoleMeshFrontend:
		return InfrastructureResource{
			Kind:           InfrastructureKindService,
			Namespace:      service.Namespace,
			Name:           service.Name,
			Role:           InfrastructureRoleMeshFrontendSvc,
			OwnerKind:      InfrastructureKindService,
			OwnerNamespace: service.Namespace,
			OwnerName:      service.Name,
		}, true
	case service.Annotations[mesh.ManagedServiceAnnotation] == "true":
		return InfrastructureResource{
			Kind:           InfrastructureKindService,
			Namespace:      service.Namespace,
			Name:           service.Name,
			Role:           InfrastructureRoleMeshFrontendSvc,
			OwnerKind:      InfrastructureKindService,
			OwnerNamespace: service.Namespace,
			OwnerName:      service.Name,
		}, true
	default:
		return InfrastructureResource{}, false
	}
}

func classifyObservedEndpointSlice(endpointSlice discoveryv1.EndpointSlice) (InfrastructureResource, bool) {
	if endpointSlice.Labels[discoveryv1.LabelManagedBy] != managedByValue {
		return InfrastructureResource{}, false
	}

	serviceName := endpointSlice.Labels[discoveryv1.LabelServiceName]
	switch endpointSlice.Labels[serviceRoleLabel] {
	case sharedEndpointSliceRoleValue:
		return InfrastructureResource{
			Kind:           InfrastructureKindSlice,
			Namespace:      endpointSlice.Namespace,
			Name:           endpointSlice.Name,
			Role:           InfrastructureRoleSharedSlice,
			OwnerKind:      InfrastructureKindService,
			OwnerNamespace: endpointSlice.Namespace,
			OwnerName:      serviceName,
		}, true
	case gatewayEndpointSliceRoleValue:
		return InfrastructureResource{
			Kind:           InfrastructureKindSlice,
			Namespace:      endpointSlice.Namespace,
			Name:           endpointSlice.Name,
			Role:           InfrastructureRoleGatewaySlice,
			OwnerKind:      "Gateway",
			OwnerNamespace: endpointSlice.Labels[gatewayNamespaceLabel],
			OwnerName:      endpointSlice.Labels[gatewayNameLabel],
		}, true
	case meshEndpointSliceRoleValue:
		return InfrastructureResource{
			Kind:           InfrastructureKindSlice,
			Namespace:      endpointSlice.Namespace,
			Name:           endpointSlice.Name,
			Role:           InfrastructureRoleMeshSlice,
			OwnerKind:      InfrastructureKindService,
			OwnerNamespace: endpointSlice.Namespace,
			OwnerName:      serviceName,
		}, true
	default:
		return InfrastructureResource{}, false
	}
}

func serviceDiffReasons(current, desired *corev1.Service) []string {
	reasons := make([]string, 0, 8)
	if !stringMapEqual(current.Labels, desired.Labels) {
		reasons = append(reasons, "labels differ")
	}
	if !stringMapEqual(current.Annotations, desired.Annotations) {
		reasons = append(reasons, "annotations differ")
	}
	if current.Spec.Type != desired.Spec.Type {
		reasons = append(reasons, fmt.Sprintf("service type is %s, want %s", current.Spec.Type, desired.Spec.Type))
	}
	if !stringMapEqual(current.Spec.Selector, desired.Spec.Selector) {
		reasons = append(reasons, "selector differs")
	}
	if !stringSliceEqual(current.Spec.ExternalIPs, desired.Spec.ExternalIPs) {
		reasons = append(reasons, "externalIPs differ")
	}
	if current.Spec.LoadBalancerIP != desired.Spec.LoadBalancerIP {
		reasons = append(reasons, "loadBalancerIP differs")
	}
	if !ipFamilyPolicyEqual(current.Spec.IPFamilyPolicy, desired.Spec.IPFamilyPolicy) {
		reasons = append(reasons, "ipFamilyPolicy differs")
	}
	if !internalTrafficPolicyEqual(current.Spec.InternalTrafficPolicy, desired.Spec.InternalTrafficPolicy) {
		reasons = append(reasons, "internalTrafficPolicy differs")
	}
	if current.Spec.ExternalTrafficPolicy != desired.Spec.ExternalTrafficPolicy {
		reasons = append(reasons, "externalTrafficPolicy differs")
	}
	if current.Spec.SessionAffinity != desired.Spec.SessionAffinity {
		reasons = append(reasons, "sessionAffinity differs")
	}
	if current.Spec.PublishNotReadyAddresses != desired.Spec.PublishNotReadyAddresses {
		reasons = append(reasons, "publishNotReadyAddresses differs")
	}
	if !stringSliceEqual(current.Spec.LoadBalancerSourceRanges, desired.Spec.LoadBalancerSourceRanges) {
		reasons = append(reasons, "loadBalancerSourceRanges differ")
	}
	if !stringPointerEqual(current.Spec.LoadBalancerClass, desired.Spec.LoadBalancerClass) {
		reasons = append(reasons, "loadBalancerClass differs")
	}
	if !boolPointerEqual(current.Spec.AllocateLoadBalancerNodePorts, desired.Spec.AllocateLoadBalancerNodePorts) {
		reasons = append(reasons, "allocateLoadBalancerNodePorts differs")
	}
	if !servicePortsEqual(current.Spec.Ports, desired.Spec.Ports) {
		reasons = append(reasons, "ports differ")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "service spec differs")
	}
	return reasons
}

func endpointSliceDiffReasons(current, desired *discoveryv1.EndpointSlice) []string {
	reasons := make([]string, 0, 5)
	if !stringMapEqual(current.Labels, desired.Labels) {
		reasons = append(reasons, "labels differ")
	}
	if !stringMapEqual(current.Annotations, desired.Annotations) {
		reasons = append(reasons, "annotations differ")
	}
	if current.AddressType != desired.AddressType {
		reasons = append(reasons, fmt.Sprintf("addressType is %s, want %s", current.AddressType, desired.AddressType))
	}
	if len(current.Ports) != len(desired.Ports) {
		reasons = append(reasons, "ports differ")
	} else {
		for idx := range current.Ports {
			if !endpointPortEqual(current.Ports[idx], desired.Ports[idx]) {
				reasons = append(reasons, "ports differ")
				break
			}
		}
	}
	if len(current.Endpoints) != len(desired.Endpoints) {
		reasons = append(reasons, "endpoints differ")
	} else {
		for idx := range current.Endpoints {
			if !endpointEqual(current.Endpoints[idx], desired.Endpoints[idx]) {
				reasons = append(reasons, "endpoints differ")
				break
			}
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "EndpointSlice differs")
	}
	return reasons
}

func finalizeInfrastructureReport(report *InfrastructureReport) {
	sort.Slice(report.Resources, func(i, j int) bool {
		left := report.Resources[i]
		right := report.Resources[j]
		if infrastructureStateRank(left.State) != infrastructureStateRank(right.State) {
			return infrastructureStateRank(left.State) < infrastructureStateRank(right.State)
		}
		if left.Role != right.Role {
			return left.Role < right.Role
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Namespace != right.Namespace {
			return left.Namespace < right.Namespace
		}
		return left.Name < right.Name
	})

	gatewayConvergence := report.Summary.GatewayConvergence
	report.Summary = InfrastructureSummary{
		GatewayConvergence: gatewayConvergence,
	}
	for _, item := range report.Resources {
		report.Summary.ResourceCount++
		switch item.Kind {
		case InfrastructureKindService:
			report.Summary.ServiceCount++
		case InfrastructureKindSlice:
			report.Summary.EndpointSliceCount++
		}
		switch item.State {
		case InfrastructureStateReady:
			report.Summary.ReadyCount++
		case InfrastructureStateDrifted:
			report.Summary.DriftedCount++
		case InfrastructureStateMissing:
			report.Summary.MissingCount++
		case InfrastructureStateOrphan:
			report.Summary.OrphanCount++
		}
	}

	report.Warnings = report.Warnings[:0]
	if report.Summary.MissingCount > 0 {
		report.Warnings = append(
			report.Warnings,
			fmt.Sprintf("%d derived infrastructure resources are missing", report.Summary.MissingCount),
		)
	}
	if report.Summary.DriftedCount > 0 {
		report.Warnings = append(
			report.Warnings,
			fmt.Sprintf("%d derived infrastructure resources have drifted from desired state", report.Summary.DriftedCount),
		)
	}
	if report.Summary.OrphanCount > 0 {
		report.Warnings = append(
			report.Warnings,
			fmt.Sprintf("%d managed infrastructure resources are orphaned", report.Summary.OrphanCount),
		)
	}
	if report.Summary.GatewayConvergence.PendingServiceMetadataCount > 0 {
		report.Warnings = append(
			report.Warnings,
			fmt.Sprintf(
				"%d gateways are waiting for derived Service metadata convergence",
				report.Summary.GatewayConvergence.PendingServiceMetadataCount,
			),
		)
	}
	if report.Summary.GatewayConvergence.PendingFrontendEndpointSliceCount > 0 {
		report.Warnings = append(
			report.Warnings,
			fmt.Sprintf(
				"%d gateways are waiting for derived frontend EndpointSlice convergence",
				report.Summary.GatewayConvergence.PendingFrontendEndpointSliceCount,
			),
		)
	}
	if report.Summary.GatewayConvergence.PendingProgrammedObservedGenerationCount > 0 {
		report.Warnings = append(
			report.Warnings,
			fmt.Sprintf(
				"%d gateways are waiting for Programmed observedGeneration convergence",
				report.Summary.GatewayConvergence.PendingProgrammedObservedGenerationCount,
			),
		)
	}
}

func sortedExpectationKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func infrastructureStateRank(state string) int {
	switch state {
	case InfrastructureStateMissing:
		return 0
	case InfrastructureStateDrifted:
		return 1
	case InfrastructureStateOrphan:
		return 2
	default:
		return 3
	}
}
