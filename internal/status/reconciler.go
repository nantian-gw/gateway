package status

import (
	"context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"log/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Reconciler struct {
	client                client.Client
	listReader            client.Reader
	reader                client.Reader
	controllerName        string
	statusAddresses       []string
	logger                *slog.Logger
	recorder              record.EventRecorder
	triggerInfrastructure func()
	options               Options
}

type noopEventRecorder struct{}

func (noopEventRecorder) Event(_ runtime.Object, _, _, _ string) {}

func (noopEventRecorder) Eventf(_ runtime.Object, _, _, _ string, _ ...interface{}) {}

func (noopEventRecorder) AnnotatedEventf(_ runtime.Object, _ map[string]string, _, _, _ string, _ ...interface{}) {
}

func New(
	client client.Client,
	controllerName string,
	statusAddress string,
	logger *slog.Logger,
) *Reconciler {
	return NewWithAddressesAndReader(client, client, controllerName, []string{statusAddress}, logger)
}

func NewWithAddresses(
	client client.Client,
	controllerName string,
	statusAddresses []string,
	logger *slog.Logger,
) *Reconciler {
	return NewWithAddressesAndReader(client, client, controllerName, statusAddresses, logger)
}

func NewWithAddressesAndReader(
	client client.Client,
	reader client.Reader,
	controllerName string,
	statusAddresses []string,
	logger *slog.Logger,
) *Reconciler {
	return NewWithAddressesAndReaderOptions(client, reader, controllerName, statusAddresses, logger, defaultOptions())
}

func NewWithAddressesAndReaderOptions(
	client client.Client,
	reader client.Reader,
	controllerName string,
	statusAddresses []string,
	logger *slog.Logger,
	options Options,
) *Reconciler {
	if reader == nil {
		reader = client
	}
	return &Reconciler{
		client:          client,
		listReader:      client,
		reader:          reader,
		controllerName:  controllerName,
		statusAddresses: append([]string(nil), statusAddresses...),
		logger:          logger,
		recorder:        noopEventRecorder{},
		options:         options,
	}
}

func (r *Reconciler) experimentalGatewayEnabled() bool {
	return r != nil && r.options.EnableExperimentalGateway
}

func (r *Reconciler) SetEventRecorder(recorder record.EventRecorder) {
	if recorder == nil {
		r.recorder = noopEventRecorder{}
		return
	}
	r.recorder = recorder
}

func (r *Reconciler) SetTriggerInfrastructure(fn func()) {
	r.triggerInfrastructure = fn
}

func (r *Reconciler) Reconcile(ctx context.Context) error {
	state, err := r.loadState(ctx)
	if err != nil {
		return err
	}

	routeState := evaluateRoutes(state)
	gatewayState := evaluateGateways(state, routeState.attachments)

	if err := r.reconcileGatewayClasses(ctx, state.gatewayClasses); err != nil {
		return err
	}
	if err := r.reconcileGateways(ctx, state.managedGateways, gatewayState); err != nil {
		return err
	}
	syncGatewayConvergenceStageMetrics(gatewayState)
	if err := r.reconcileHTTPRoutes(ctx, state.httpRoutes, routeState.http); err != nil {
		return err
	}
	if err := r.reconcileGRPCRoutes(ctx, state.grpcRoutes, routeState.grpc); err != nil {
		return err
	}
	if err := r.reconcileTCPRoutes(ctx, state.tcpRoutes, routeState.tcp); err != nil {
		return err
	}
	if err := r.reconcileUDPRoutes(ctx, state.udpRoutes, routeState.udp); err != nil {
		return err
	}
	if err := r.reconcileTLSRoutes(ctx, state.tlsRoutes, routeState.tls); err != nil {
		return err
	}
	if err := r.reconcileBackendLBPolicies(
		ctx,
		state.backendLBPolicies,
		evaluateBackendLBPolicies(state, routeState),
	); err != nil {
		return err
	}
	if err := r.reconcileBackendTLSPolicies(
		ctx,
		state.backendTLSPolicies,
		evaluateBackendTLSPolicies(state, routeState),
	); err != nil {
		return err
	}
	if err := r.reconcileListenerSetStatuses(
		ctx,
		state.listenerSets,
		evaluateListenerSets(state, state.listenerSets, state.managedGatewayByKey, routeState.attachments),
	); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) ReconcileGatewayStatuses(ctx context.Context) error {
	state, err := r.loadState(ctx)
	if err != nil {
		return err
	}

	routeState := evaluateRoutes(state)
	gatewayState := evaluateGateways(state, routeState.attachments)

	if err := r.reconcileGatewayClasses(ctx, state.gatewayClasses); err != nil {
		return err
	}
	if err := r.reconcileGateways(ctx, state.managedGateways, gatewayState); err != nil {
		return err
	}
	syncGatewayConvergenceStageMetrics(gatewayState)
	return nil
}

func (r *Reconciler) ReconcileRouteStatuses(ctx context.Context) error {
	state, err := r.loadState(ctx)
	if err != nil {
		return err
	}

	routeState := evaluateRoutes(state)
	if err := r.reconcileHTTPRoutes(ctx, state.httpRoutes, routeState.http); err != nil {
		return err
	}
	if err := r.reconcileGRPCRoutes(ctx, state.grpcRoutes, routeState.grpc); err != nil {
		return err
	}
	if err := r.reconcileTCPRoutes(ctx, state.tcpRoutes, routeState.tcp); err != nil {
		return err
	}
	if err := r.reconcileUDPRoutes(ctx, state.udpRoutes, routeState.udp); err != nil {
		return err
	}
	return r.reconcileTLSRoutes(ctx, state.tlsRoutes, routeState.tls)
}

func (r *Reconciler) ReconcilePolicyStatuses(ctx context.Context) error {
	state, err := r.loadState(ctx)
	if err != nil {
		return err
	}

	routeState := evaluateRoutes(state)
	if err := r.reconcileBackendLBPolicies(
		ctx,
		state.backendLBPolicies,
		evaluateBackendLBPolicies(state, routeState),
	); err != nil {
		return err
	}
	return r.reconcileBackendTLSPolicies(
		ctx,
		state.backendTLSPolicies,
		evaluateBackendTLSPolicies(state, routeState),
	)
}

func (r *Reconciler) ReconcileGatewayClassObject(ctx context.Context, name string) error {
	var current gatewayv1.GatewayClass
	if err := r.reader.Get(ctx, client.ObjectKey{Name: name}, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if string(current.Spec.ControllerName) != r.controllerName {
		return nil
	}
	return r.reconcileGatewayClassStatus(ctx, name)
}

func (r *Reconciler) ReconcileListenerSetObject(ctx context.Context, key client.ObjectKey) error {
	var ls gatewayv1.ListenerSet
	if err := r.reader.Get(ctx, key, &ls); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	ref := ls.Spec.ParentRef
	gwNs := ls.Namespace
	if ref.Namespace != nil && string(*ref.Namespace) != "" {
		gwNs = string(*ref.Namespace)
	}
	if string(ref.Name) == "" {
		return nil
	}
	gwKey := client.ObjectKey{Namespace: gwNs, Name: string(ref.Name)}

	var gateway gatewayv1.Gateway
	if err := r.reader.Get(ctx, gwKey, &gateway); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	state, err := r.loadGatewayObjectStateWithListenerSets(ctx, gateway, []gatewayv1.ListenerSet{ls})
	if err != nil {
		return err
	}

	eval, ok := evaluateListenerSets(
		state,
		state.listenerSets,
		state.managedGatewayByKey,
		evaluateRouteAttachments(state),
	)[namespacedName(ls.Namespace, ls.Name)]
	if !ok {
		return nil
	}
	return r.reconcileListenerSetStatus(ctx, key, eval)
}
