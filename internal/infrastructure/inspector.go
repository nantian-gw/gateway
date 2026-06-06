package infrastructure

import "context"

const (
	InfrastructureStateReady          = "ready"
	InfrastructureStateDrifted        = "drifted"
	InfrastructureStateMissing        = "missing"
	InfrastructureStateOrphan         = "orphan"
	InfrastructureKindService         = "Service"
	InfrastructureKindSlice           = "EndpointSlice"
	InfrastructureRoleSharedSvc       = "shared-service"
	InfrastructureRoleGatewaySvc      = "gateway-service"
	InfrastructureRoleMeshFrontendSvc = "mesh-frontend-service"
	InfrastructureRoleMeshShadowSvc   = "mesh-shadow-service"
	InfrastructureRoleSharedSlice     = "shared-endpointslice"
	InfrastructureRoleGatewaySlice    = "gateway-endpointslice"
	InfrastructureRoleMeshSlice       = "mesh-endpointslice"
)

type InfrastructureReport struct {
	Summary   InfrastructureSummary    `json:"summary"`
	Resources []InfrastructureResource `json:"resources"`
	Warnings  []string                 `json:"warnings,omitempty"`
}

type InfrastructureSummary struct {
	ResourceCount      int                       `json:"resourceCount"`
	ServiceCount       int                       `json:"serviceCount"`
	EndpointSliceCount int                       `json:"endpointSliceCount"`
	ReadyCount         int                       `json:"readyCount"`
	DriftedCount       int                       `json:"driftedCount"`
	MissingCount       int                       `json:"missingCount"`
	OrphanCount        int                       `json:"orphanCount"`
	GatewayConvergence GatewayConvergenceSummary `json:"gatewayConvergence"`
}

type GatewayConvergenceSummary struct {
	GatewayCount                             int   `json:"gatewayCount"`
	ReadyCount                               int   `json:"readyCount"`
	PendingServiceMetadataCount              int   `json:"pendingServiceMetadataCount"`
	PendingFrontendEndpointSliceCount        int   `json:"pendingFrontendEndpointSliceCount"`
	PendingProgrammedObservedGenerationCount int   `json:"pendingProgrammedObservedGenerationCount"`
	MaxServiceMetadataGenerationLag          int64 `json:"maxServiceMetadataGenerationLag"`
	MaxFrontendEndpointSliceGenerationLag    int64 `json:"maxFrontendEndpointSliceGenerationLag"`
	MaxProgrammedObservedGenerationLag       int64 `json:"maxProgrammedObservedGenerationLag"`
}

type InfrastructureResource struct {
	Kind           string   `json:"kind"`
	Namespace      string   `json:"namespace"`
	Name           string   `json:"name"`
	Role           string   `json:"role"`
	OwnerKind      string   `json:"ownerKind,omitempty"`
	OwnerNamespace string   `json:"ownerNamespace,omitempty"`
	OwnerName      string   `json:"ownerName,omitempty"`
	State          string   `json:"state"`
	Reasons        []string `json:"reasons,omitempty"`
}

func (r *Reconciler) Inspect(ctx context.Context) (InfrastructureReport, error) {
	report := InfrastructureReport{
		Resources: make([]InfrastructureResource, 0),
	}

	gateways, err := r.loadManagedGateways(ctx)
	if err != nil {
		return report, err
	}

	meshParentKeys, err := r.loadMeshServiceParents(ctx)
	if err != nil {
		return report, err
	}

	services, serviceIndex, err := r.loadObservedServices(ctx, gateways, meshParentKeys)
	if err != nil {
		return report, err
	}

	expectedServices, expectedSlices, err := r.expectedInfrastructure(
		ctx,
		gateways,
		meshParentKeys,
		services,
		serviceIndex,
	)
	if err != nil {
		return report, err
	}

	endpointSlices, sliceIndex, err := r.loadObservedEndpointSlices(ctx, expectedSlices)
	if err != nil {
		return report, err
	}

	report.Summary.GatewayConvergence = summarizeGatewayConvergence(
		gateways,
		expectedServices,
		serviceIndex,
		expectedSlices,
		sliceIndex,
	)

	for _, key := range sortedExpectationKeys(expectedServices) {
		expected := expectedServices[key]
		current, ok := serviceIndex[key]
		switch {
		case !ok:
			item := expected.resource
			item.State = InfrastructureStateMissing
			item.Reasons = []string{"resource not found"}
			report.Resources = append(report.Resources, item)
		case !serviceEqual(&current, expected.service):
			item := expected.resource
			item.State = InfrastructureStateDrifted
			item.Reasons = serviceDiffReasons(&current, expected.service)
			report.Resources = append(report.Resources, item)
		default:
			item := expected.resource
			item.State = InfrastructureStateReady
			report.Resources = append(report.Resources, item)
		}
	}

	for _, key := range sortedExpectationKeys(expectedSlices) {
		expected := expectedSlices[key]
		current, ok := sliceIndex[key]
		switch {
		case !ok:
			item := expected.resource
			item.State = InfrastructureStateMissing
			item.Reasons = []string{"resource not found"}
			report.Resources = append(report.Resources, item)
		case !endpointSliceEqual(&current, expected.endpointSlice):
			item := expected.resource
			item.State = InfrastructureStateDrifted
			item.Reasons = endpointSliceDiffReasons(&current, expected.endpointSlice)
			report.Resources = append(report.Resources, item)
		default:
			item := expected.resource
			item.State = InfrastructureStateReady
			report.Resources = append(report.Resources, item)
		}
	}

	for _, service := range services {
		item, ok := classifyObservedService(service)
		if !ok {
			continue
		}
		if _, exists := expectedServices[serviceKey(service.Namespace, service.Name)]; exists {
			continue
		}
		item.State = InfrastructureStateOrphan
		item.Reasons = []string{"managed resource is no longer desired"}
		report.Resources = append(report.Resources, item)
	}

	for _, endpointSlice := range endpointSlices {
		item, ok := classifyObservedEndpointSlice(endpointSlice)
		if !ok {
			continue
		}
		if _, exists := expectedSlices[serviceKey(endpointSlice.Namespace, endpointSlice.Name)]; exists {
			continue
		}
		item.State = InfrastructureStateOrphan
		item.Reasons = []string{"managed resource is no longer desired"}
		report.Resources = append(report.Resources, item)
	}

	finalizeInfrastructureReport(&report)
	return report, nil
}
