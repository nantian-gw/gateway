package infrastructure

import (
	"context"
	"log/slog"
	"sort"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/nodeinfo"
)

const (
	defaultDataplaneNamespace = "nantian-gw"
	defaultSharedServiceName  = "nantian-gw-dataplane"
	defaultAdminPortName      = "admin"
	defaultAdminPort          = 19080

	gatewayNameLabel        = "gateway.networking.k8s.io/gateway-name"
	gatewayNamespaceLabel   = "nantian.dev/gateway-namespace"
	managedByLabel          = "app.kubernetes.io/managed-by"
	managedByValue          = "nantian-gw"
	serviceRoleLabel        = "nantian.dev/service-role"
	serviceRoleShared       = "shared-dataplane"
	serviceRoleGateway      = "gateway-metadata"
	serviceRoleMeshFrontend = "mesh-frontend"
)

var defaultDataplaneSelector = map[string]string{
	"app": "nantian-gw-dataplane",
}

type Options struct {
	DataplaneNamespace        string
	SharedServiceName         string
	DataplaneSelector         map[string]string
	SnapshotStore             *ir.SnapshotStore
	NodeStatus                *nodeinfo.Registry
	EnableExperimentalGateway bool
}

type Reconciler struct {
	client         client.Client
	apiReader      client.Reader
	controllerName string
	options        Options
	store          *ir.SnapshotStore
	nodes          *nodeinfo.Registry
	logger         *slog.Logger
}

func New(client client.Client, controllerName string, logger *slog.Logger) *Reconciler {
	return NewWithOptions(client, nil, controllerName, DefaultOptions(), logger)
}

func NewWithOptions(
	client client.Client,
	apiReader client.Reader,
	controllerName string,
	options Options,
	logger *slog.Logger,
) *Reconciler {
	if options.DataplaneNamespace == "" {
		options.DataplaneNamespace = defaultDataplaneNamespace
	}
	if options.SharedServiceName == "" {
		options.SharedServiceName = defaultSharedServiceName
	}
	if len(options.DataplaneSelector) == 0 {
		options.DataplaneSelector = cloneStringMap(defaultDataplaneSelector)
	}

	return &Reconciler{
		client:         client,
		apiReader:      apiReader,
		controllerName: controllerName,
		options:        options,
		store:          options.SnapshotStore,
		nodes:          options.NodeStatus,
		logger:         logger,
	}
}

func DefaultOptions() Options {
	return Options{
		DataplaneNamespace:        defaultDataplaneNamespace,
		SharedServiceName:         defaultSharedServiceName,
		DataplaneSelector:         cloneStringMap(defaultDataplaneSelector),
		EnableExperimentalGateway: true,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context) error {
	tracer := otel.Tracer("github.com/nantian-gw/gateway/internal/infrastructure")
	ctx, span := tracer.Start(ctx, "controlplane.infrastructure.reconcile")
	defer span.End()

	r.logger.InfoContext(ctx, "infrastructure reconciler starting")
	managedGateways, err := r.loadManagedGateways(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	span.SetAttributes(attribute.Int("infrastructure.managed_gateways", len(managedGateways)))
	r.logger.InfoContext(ctx, "infrastructure reconciler loaded gateways", "count", len(managedGateways))
	sharedPods, gatewayPods, meshPods, err := r.loadFrontendEligibleDataplanePods(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	span.SetAttributes(
		attribute.Int("infrastructure.shared_pods", len(sharedPods)),
		attribute.Int("infrastructure.gateway_pods", len(gatewayPods)),
		attribute.Int("infrastructure.mesh_pods", len(meshPods)),
	)

	if err := r.reconcileSharedService(ctx, managedGateways, sharedPods); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if err := r.reconcileMeshServicesWithPods(ctx, meshPods); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if err := reconcileDataplaneNetworkPolicy(ctx, r.client, managedGateways, r.options); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	gwErr := r.reconcileGatewayServices(ctx, managedGateways, gatewayPods)
	if gwErr != nil {
		span.RecordError(gwErr)
		span.SetStatus(codes.Error, gwErr.Error())
	}
	span.SetAttributes(attribute.Bool("infrastructure.gateway_services_failed", gwErr != nil))
	r.logger.InfoContext(ctx, "infrastructure reconciler completed", "error", gwErr)
	return gwErr
}

func (r *Reconciler) loadManagedGateways(ctx context.Context) ([]gatewayv1.Gateway, error) {
	gatewayClasses, err := listGatewayClassesForController(ctx, r.client, r.controllerName)
	if err != nil {
		return nil, err
	}
	r.logger.InfoContext(ctx, "loadManagedGateways: field index GatewayClasses", "count", len(gatewayClasses))

	if len(gatewayClasses) == 0 {
		gatewayClasses, err = listAllGatewayClassesForController(ctx, r.client, r.controllerName)
		if err != nil {
			return nil, err
		}
		r.logger.InfoContext(ctx, "loadManagedGateways: fallback GatewayClasses", "count", len(gatewayClasses))
	}

	if len(gatewayClasses) == 0 {
		return nil, nil
	}

	out := make([]gatewayv1.Gateway, 0)
	for _, gatewayClass := range gatewayClasses {
		gateways, err := listGatewaysForGatewayClass(ctx, r.client, gatewayClass.Name)
		if err != nil {
			return nil, err
		}
		r.logger.InfoContext(ctx, "loadManagedGateways: field index Gateways", "class", gatewayClass.Name, "count", len(gateways))
		out = append(out, gateways...)
	}

	if len(out) == 0 {
		var allGateways gatewayv1.GatewayList
		var listErr error
		if r.apiReader != nil {
			listErr = r.apiReader.List(ctx, &allGateways)
		} else {
			listErr = r.client.List(ctx, &allGateways)
		}
		if listErr != nil {
			return nil, listErr
		}
		r.logger.InfoContext(ctx, "loadManagedGateways: fallback all Gateways", "total", len(allGateways.Items))
		classNames := make(map[string]bool, len(gatewayClasses))
		for _, gc := range gatewayClasses {
			classNames[gc.Name] = true
		}
		for _, gw := range allGateways.Items {
			if classNames[string(gw.Spec.GatewayClassName)] {
				out = append(out, gw)
			}
		}
		r.logger.InfoContext(ctx, "loadManagedGateways: fallback filtered Gateways", "count", len(out))
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})

	return out, nil
}

func listAllGatewayClassesForController(
	ctx context.Context,
	cl client.Client,
	controllerName string,
) ([]gatewayv1.GatewayClass, error) {
	var all gatewayv1.GatewayClassList
	if err := cl.List(ctx, &all); err != nil {
		return nil, err
	}
	out := make([]gatewayv1.GatewayClass, 0, len(all.Items))
	for _, gc := range all.Items {
		if string(gc.Spec.ControllerName) == controllerName {
			out = append(out, gc)
		}
	}
	return out, nil
}

func listGatewayClassesForController(
	ctx context.Context,
	cl client.Client,
	controllerName string,
) ([]gatewayv1.GatewayClass, error) {
	var gatewayClasses gatewayv1.GatewayClassList
	if err := cl.List(
		ctx,
		&gatewayClasses,
		client.MatchingFields{gatewayClassControllerNameIndex: controllerName},
	); err != nil {
		if isMissingFieldIndexError(err) {
			return nil, requiredFieldIndexError("GatewayClass", gatewayClassControllerNameIndex, err)
		}
		return nil, err
	}

	out := make([]gatewayv1.GatewayClass, 0, len(gatewayClasses.Items))
	for _, gatewayClass := range gatewayClasses.Items {
		if string(gatewayClass.Spec.ControllerName) != controllerName {
			continue
		}
		out = append(out, gatewayClass)
	}
	return out, nil
}

func listGatewaysForGatewayClass(
	ctx context.Context,
	cl client.Client,
	gatewayClassName string,
) ([]gatewayv1.Gateway, error) {
	var gateways gatewayv1.GatewayList
	if err := cl.List(
		ctx,
		&gateways,
		client.MatchingFields{gatewayGatewayClassNameIndex: gatewayClassName},
	); err != nil {
		if isMissingFieldIndexError(err) {
			return nil, requiredFieldIndexError("Gateway", gatewayGatewayClassNameIndex, err)
		}
		return nil, err
	}

	out := make([]gatewayv1.Gateway, 0, len(gateways.Items))
	for _, gateway := range gateways.Items {
		if string(gateway.Spec.GatewayClassName) != gatewayClassName {
			continue
		}
		out = append(out, gateway)
	}

	return out, nil
}

func (r *Reconciler) loadFrontendEligibleDataplanePods(
	ctx context.Context,
) ([]corev1.Pod, []corev1.Pod, []corev1.Pod, error) {
	var dataplanePods corev1.PodList
	if err := r.client.List(
		ctx,
		&dataplanePods,
		client.InNamespace(r.options.DataplaneNamespace),
		client.MatchingLabels(r.options.DataplaneSelector),
	); err != nil {
		return nil, nil, nil, err
	}

	sharedPods, gatewayPods, meshPods := r.frontendEligibleDataplanePodSets(ctx, dataplanePods.Items)
	return sharedPods, gatewayPods, meshPods, nil
}

func (r *Reconciler) reconcileSharedService(
	ctx context.Context,
	gateways []gatewayv1.Gateway,
	eligibleDataplanePods []corev1.Pod,
) error {
	current := &corev1.Service{}
	key := client.ObjectKey{
		Namespace: r.options.DataplaneNamespace,
		Name:      r.options.SharedServiceName,
	}
	if err := r.client.Get(ctx, key, current); client.IgnoreNotFound(err) != nil {
		return err
	}

	serviceID := serviceKey(key.Namespace, key.Name)
	state, err := loadServiceEndpointState(
		ctx,
		r.client,
		map[string]struct{}{serviceID: {}},
		sharedEndpointSliceRoleValue,
	)
	if err != nil {
		return err
	}

	desired := desiredSharedService(current, gateways, r.options)
	if desired == nil {
		if current.Name == "" {
			return cleanupServiceEndpointResources(ctx, r.client, serviceID, state)
		}
		if err := client.IgnoreNotFound(r.client.Delete(ctx, current)); err != nil {
			return err
		}
		return cleanupServiceEndpointResources(ctx, r.client, serviceID, state)
	}

	if err := applyService(ctx, r.client, current, desired); err != nil {
		return err
	}
	desired, err = serviceAfterApply(ctx, r.client, desired)
	if err != nil {
		return err
	}

	if err := deleteServiceEndpoints(ctx, r.client, state.endpoints[serviceID]); err != nil {
		return err
	}
	if err := reconcileFrontendEndpointSlices(
		ctx,
		r.client,
		*desired,
		eligibleDataplanePods,
		state.managedSlices[serviceID],
		sharedEndpointSliceRoleValue,
		sharedEndpointSliceNamePrefix,
	); err != nil {
		return err
	}
	// Foreign EndpointSlices (e.g. created by the Kubernetes EndpointSlice controller
	// when the shared Service has a selector) are kept as a fallback; they ensure the
	// Service has endpoints even when the dataplane snapshot version has not converged.
	return nil
}

func (r *Reconciler) reconcileGatewayServices(
	ctx context.Context,
	gateways []gatewayv1.Gateway,
	eligibleDataplanePods []corev1.Pod,
) error {
	desiredKeys := make(map[string]struct{}, len(gateways))
	serviceKeys := make(map[string]struct{}, len(gateways))
	desiredServices := make(map[string]*corev1.Service, len(gateways))
	parameterResolver := newGatewayServiceParameterResolver(r)

	for _, gateway := range gateways {
		key := gatewayServiceObjectKey(gateway)
		serviceID := serviceKey(key.Namespace, key.Name)
		serviceKeys[serviceID] = struct{}{}

		current := &corev1.Service{}
		if err := r.client.Get(ctx, key, current); client.IgnoreNotFound(err) != nil {
			return err
		}

		params := parameterResolver.resolve(ctx, gateway)
		desired := desiredGatewayService(
			current,
			gateway,
			params,
			parameterResolver.gatewayClassParametersReference(ctx, gateway),
		)
		if desired == nil {
			continue
		}
		if err := applyService(ctx, r.client, current, desired); err != nil {
			return err
		}
		updatedService, err := serviceAfterApply(ctx, r.client, desired)
		if err != nil {
			return err
		}
		desiredKeys[serviceID] = struct{}{}
		desiredServices[serviceID] = updatedService
	}

	var services corev1.ServiceList
	if err := r.client.List(
		ctx,
		&services,
		client.MatchingLabels{
			managedByLabel:   managedByValue,
			serviceRoleLabel: serviceRoleGateway,
		},
	); err != nil {
		return err
	}

	for _, service := range services.Items {
		serviceKeys[serviceKey(service.Namespace, service.Name)] = struct{}{}
	}
	state, err := loadServiceEndpointState(ctx, r.client, serviceKeys, gatewayEndpointSliceRoleValue)
	if err != nil {
		return err
	}

	for _, gateway := range gateways {
		key := gatewayServiceObjectKey(gateway)
		serviceID := serviceKey(key.Namespace, key.Name)
		desired := desiredServices[serviceID]
		if desired == nil {
			if err := cleanupServiceEndpointResources(ctx, r.client, serviceID, state); err != nil {
				return err
			}
			continue
		}
		if err := deleteServiceEndpoints(ctx, r.client, state.endpoints[serviceID]); err != nil {
			return err
		}
		if err := reconcileFrontendEndpointSlices(
			ctx,
			r.client,
			*desired,
			eligibleDataplanePods,
			state.managedSlices[serviceID],
			gatewayEndpointSliceRoleValue,
			gatewayEndpointSliceNamePrefix,
		); err != nil {
			return err
		}
		// Foreign EndpointSlices (e.g. created by Kubernetes when the Service
		// has a selector) are kept as a fallback for snapshot convergence.
	}

	for _, service := range services.Items {
		serviceID := serviceKey(service.Namespace, service.Name)
		if _, ok := desiredKeys[serviceID]; ok {
			continue
		}
		if err := r.client.Delete(ctx, &service); client.IgnoreNotFound(err) != nil {
			return err
		}
		if err := cleanupServiceEndpointResources(ctx, r.client, serviceID, state); err != nil {
			return err
		}
	}

	return nil
}
