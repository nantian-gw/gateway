package status

import (
	"context"
	"reflect"
	"strings"
	"time"

	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	k8sptr "k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"

	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	routepolicy "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
	"github.com/nantian-gw/gateway/internal/resources"
)

type controllerSetup interface {
	SetupWithManager(ctrl.Manager) error
}

func SetupControllers(mgr ctrl.Manager, reconciler *Reconciler, options ...Options) error {
	for _, controller := range statusControllerSetups(reconciler, normalizeOptions(options)) {
		if err := controller.SetupWithManager(mgr); err != nil {
			return err
		}
	}

	return nil
}

func statusControllerSetups(reconciler *Reconciler, opts Options) []controllerSetup {
	controllers := []controllerSetup{
		&gatewayClassController{reconciler: reconciler},
		&gatewayController{
			reconciler:                reconciler,
			enableExperimentalGateway: opts.EnableExperimentalGateway,
		},
		&httpRouteController{reconciler: reconciler},
		&grpcRouteController{reconciler: reconciler},
	}

	if opts.EnableExperimentalGateway {
		controllers = append(controllers,
			&tcpRouteController{reconciler: reconciler},
			&udpRouteController{reconciler: reconciler},
			&tlsRouteController{reconciler: reconciler},
			&listenerSetController{reconciler: reconciler},
		)
	}

	controllers = append(controllers,
		&backendTLSPolicyController{reconciler: reconciler},
	)

	if opts.EnableExperimentalGateway {
		controllers = append(controllers,
			&backendLBPolicyController{reconciler: reconciler},
		)
	}
	if opts.EnableAiGateway {
		controllers = append(controllers,
			&aiserviceController{reconciler: reconciler},
		)
	}
	if opts.EnableExperimentalGateway {
		controllers = append(controllers,
			&tokenPolicyController{reconciler: reconciler},
			&wasmPluginController{reconciler: reconciler},
			&routePolicyController{reconciler: reconciler},
		)
	}

	return controllers
}

func statusControllerOptions(opts Options) controller.Options {
	maxConcurrent := opts.MaxConcurrentReconciles
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	baseDelay := opts.RateLimiterBaseDelay
	if baseDelay <= 0 {
		baseDelay = 200 * time.Millisecond
	}
	maxDelay := opts.RateLimiterMaxDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}
	qps := opts.RateLimiterQPS
	if qps <= 0 {
		qps = 10
	}
	bucketSize := opts.RateLimiterBucketSize
	if bucketSize <= 0 {
		bucketSize = 100
	}

	return controller.Options{
		MaxConcurrentReconciles: maxConcurrent,
		NeedLeaderElection:      k8sptr.To(true),
		RateLimiter: workqueue.NewTypedMaxOfRateLimiter(
			workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](baseDelay, maxDelay),
			&workqueue.TypedBucketRateLimiter[reconcile.Request]{
				Limiter: rate.NewLimiter(rate.Limit(qps), bucketSize),
			},
		),
	}
}

func resourceSupported(
	scheme *runtime.Scheme,
	restMapper meta.RESTMapper,
	object client.Object,
) bool {
	gvk, err := apiutil.GVKForObject(object, scheme)
	if err != nil {
		return false
	}
	_, err = restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	return err == nil
}

func serviceImportWatchSupported(mgr ctrl.Manager) bool {
	return resourceSupported(mgr.GetScheme(), mgr.GetRESTMapper(), &mcsv1alpha1.ServiceImport{})
}

var generationChanged = builder.WithPredicates(predicate.GenerationChangedPredicate{})

const (
	gatewayStatusGatewayNameLabel      = "gateway.networking.k8s.io/gateway-name"
	gatewayStatusGatewayNamespaceLabel = "nantian.dev/gateway-namespace"
)

type gatewayClassController struct {
	reconciler *Reconciler
}

func (c *gatewayClassController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.ReconcileGatewayClassObject(ctx, req.Name)
}

func (c *gatewayClassController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("gatewayclass-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&gatewayv1.GatewayClass{}, generationChanged).
		Complete(c)
}

type gatewayController struct {
	reconciler                *Reconciler
	enableExperimentalGateway bool
}

func (c *gatewayController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	if c.reconciler.triggerInfrastructure != nil {
		c.reconciler.triggerInfrastructure()
	}
	return ctrl.Result{}, c.reconciler.ReconcileGatewayObject(ctx, req.NamespacedName)
}

func (c *gatewayController) SetupWithManager(mgr ctrl.Manager) error {
	gatewayInfrastructureRequests := handler.EnqueueRequestsFromMapFunc(
		gatewayInfrastructureStatusRequests,
	)

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		Named("gateway-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&gatewayv1.Gateway{}, generationChanged).
		Watches(
			&corev1.Service{},
			gatewayInfrastructureRequests,
			builder.WithPredicates(gatewayInfrastructureServicePredicate()),
		).
		Watches(
			&discoveryv1.EndpointSlice{},
			gatewayInfrastructureRequests,
			builder.WithPredicates(gatewayFrontendEndpointSlicePredicate()),
		)

	if c.enableExperimentalGateway && resourceSupported(mgr.GetScheme(), mgr.GetRESTMapper(), &gatewayv1.ListenerSet{}) {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1.ListenerSet{},
			handler.EnqueueRequestsFromMapFunc(gatewayListenerSetStatusRequests),
		)
	}

	return controllerBuilder.Complete(c)
}

func gatewayInfrastructureServicePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			service, ok := event.Object.(*corev1.Service)
			return ok && gatewayInfrastructureServiceRelevant(service)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			oldService, okOld := event.ObjectOld.(*corev1.Service)
			newService, okNew := event.ObjectNew.(*corev1.Service)
			if !okOld || !okNew || oldService == nil || newService == nil {
				return false
			}
			if !gatewayInfrastructureServiceRelevant(newService) && !gatewayInfrastructureServiceRelevant(oldService) {
				return false
			}
			return gatewayInfrastructureServiceChanged(event)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			service, ok := event.Object.(*corev1.Service)
			return ok && gatewayInfrastructureServiceRelevant(service)
		},
	}
}

func gatewayFrontendEndpointSlicePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			slice, ok := event.Object.(*discoveryv1.EndpointSlice)
			return ok && gatewayFrontendEndpointSliceRelevant(slice)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			oldSlice, okOld := event.ObjectOld.(*discoveryv1.EndpointSlice)
			newSlice, okNew := event.ObjectNew.(*discoveryv1.EndpointSlice)
			if !okOld || !okNew || oldSlice == nil || newSlice == nil {
				return false
			}
			if !gatewayFrontendEndpointSliceRelevant(newSlice) && !gatewayFrontendEndpointSliceRelevant(oldSlice) {
				return false
			}
			return gatewayFrontendEndpointSliceMetadataChanged(event)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			slice, ok := event.Object.(*discoveryv1.EndpointSlice)
			return ok && gatewayFrontendEndpointSliceRelevant(slice)
		},
	}
}

func gatewayInfrastructureStatusRequests(
	_ context.Context,
	object client.Object,
) []reconcile.Request {
	switch item := object.(type) {
	case *corev1.Service:
		if !gatewayInfrastructureServiceRelevant(item) {
			return nil
		}
		return gatewayStatusRequestsForLabels(item.Labels)
	case *discoveryv1.EndpointSlice:
		if !gatewayFrontendEndpointSliceRelevant(item) {
			return nil
		}
		return gatewayStatusRequestsForLabels(item.Labels)
	default:
		return nil
	}
}

func gatewayInfrastructureServiceRelevant(service *corev1.Service) bool {
	return service != nil &&
		resources.IsManagedFrontendService(*service) &&
		service.Labels[resources.ServiceRoleKey] == resources.ServiceRoleGateway
}

func gatewayFrontendEndpointSliceRelevant(slice *discoveryv1.EndpointSlice) bool {
	return slice != nil &&
		resources.IsManagedFrontendEndpointSlice(*slice) &&
		slice.Labels[resources.ServiceRoleKey] == resources.EndpointSliceRoleGatewayFrontend
}

func gatewayStatusRequestsForLabels(labels map[string]string) []reconcile.Request {
	namespace := strings.TrimSpace(labels[gatewayStatusGatewayNamespaceLabel])
	name := strings.TrimSpace(labels[gatewayStatusGatewayNameLabel])
	if namespace == "" || name == "" {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		},
	}}
}

func gatewayFrontendEndpointSliceMetadataChanged(event event.UpdateEvent) bool {
	oldSlice, okOld := event.ObjectOld.(*discoveryv1.EndpointSlice)
	newSlice, okNew := event.ObjectNew.(*discoveryv1.EndpointSlice)
	if !okOld || !okNew || oldSlice == nil || newSlice == nil {
		return false
	}

	if !reflect.DeepEqual(oldSlice.Labels, newSlice.Labels) {
		return true
	}
	if !reflect.DeepEqual(oldSlice.Annotations, newSlice.Annotations) {
		return true
	}
	return !reflect.DeepEqual(oldSlice.OwnerReferences, newSlice.OwnerReferences)
}

func gatewayInfrastructureServiceChanged(event event.UpdateEvent) bool {
	oldService, okOld := event.ObjectOld.(*corev1.Service)
	newService, okNew := event.ObjectNew.(*corev1.Service)
	if !okOld || !okNew || oldService == nil || newService == nil {
		return false
	}

	if !reflect.DeepEqual(oldService.Labels, newService.Labels) {
		return true
	}
	if !reflect.DeepEqual(oldService.Annotations, newService.Annotations) {
		return true
	}
	if !reflect.DeepEqual(oldService.OwnerReferences, newService.OwnerReferences) {
		return true
	}
	if !reflect.DeepEqual(oldService.Spec.ExternalIPs, newService.Spec.ExternalIPs) {
		return true
	}
	if oldService.Spec.LoadBalancerIP != newService.Spec.LoadBalancerIP {
		return true
	}
	return !reflect.DeepEqual(
		oldService.Status.LoadBalancer.Ingress,
		newService.Status.LoadBalancer.Ingress,
	)
}

type httpRouteController struct {
	reconciler *Reconciler
}

func (c *httpRouteController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.ReconcileHTTPRouteObject(ctx, req.NamespacedName)
}

func (c *httpRouteController) SetupWithManager(mgr ctrl.Manager) error {
	serviceRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return httpRouteStatusRequestsForService(ctx, mgr.GetClient(), object)
	})
	serviceImportRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return httpRouteStatusRequestsForServiceImport(ctx, mgr.GetClient(), object)
	})

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		Named("httproute-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&gatewayv1.HTTPRoute{}, generationChanged).
		Watches(
			&corev1.Service{},
			serviceRequests,
			builder.WithPredicates(routeBackendServiceDependencyPredicate()),
		)

	if serviceImportWatchSupported(mgr) {
		controllerBuilder = controllerBuilder.Watches(
			&mcsv1alpha1.ServiceImport{},
			serviceImportRequests,
			builder.WithPredicates(routeBackendServiceImportDependencyPredicate()),
		)
	}

	return controllerBuilder.Complete(c)
}

type grpcRouteController struct {
	reconciler *Reconciler
}

func (c *grpcRouteController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.ReconcileGRPCRouteObject(ctx, req.NamespacedName)
}

func (c *grpcRouteController) SetupWithManager(mgr ctrl.Manager) error {
	serviceRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return grpcRouteStatusRequestsForService(ctx, mgr.GetClient(), object)
	})
	serviceImportRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return grpcRouteStatusRequestsForServiceImport(ctx, mgr.GetClient(), object)
	})

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		Named("grpcroute-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&gatewayv1.GRPCRoute{}, generationChanged).
		Watches(
			&corev1.Service{},
			serviceRequests,
			builder.WithPredicates(routeBackendServiceDependencyPredicate()),
		)

	if serviceImportWatchSupported(mgr) {
		controllerBuilder = controllerBuilder.Watches(
			&mcsv1alpha1.ServiceImport{},
			serviceImportRequests,
			builder.WithPredicates(routeBackendServiceImportDependencyPredicate()),
		)
	}

	return controllerBuilder.Complete(c)
}

type tcpRouteController struct {
	reconciler *Reconciler
}

func (c *tcpRouteController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.ReconcileTCPRouteObject(ctx, req.NamespacedName)
}

func (c *tcpRouteController) SetupWithManager(mgr ctrl.Manager) error {
	serviceRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return tcpRouteStatusRequestsForService(ctx, mgr.GetClient(), object)
	})
	serviceImportRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return tcpRouteStatusRequestsForServiceImport(ctx, mgr.GetClient(), object)
	})

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		Named("tcproute-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&gatewayv1alpha2.TCPRoute{}, generationChanged).
		Watches(
			&corev1.Service{},
			serviceRequests,
			builder.WithPredicates(routeBackendServiceDependencyPredicate()),
		)

	if serviceImportWatchSupported(mgr) {
		controllerBuilder = controllerBuilder.Watches(
			&mcsv1alpha1.ServiceImport{},
			serviceImportRequests,
			builder.WithPredicates(routeBackendServiceImportDependencyPredicate()),
		)
	}

	return controllerBuilder.Complete(c)
}

type udpRouteController struct {
	reconciler *Reconciler
}

func (c *udpRouteController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.ReconcileUDPRouteObject(ctx, req.NamespacedName)
}

func (c *udpRouteController) SetupWithManager(mgr ctrl.Manager) error {
	serviceRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return udpRouteStatusRequestsForService(ctx, mgr.GetClient(), object)
	})
	serviceImportRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return udpRouteStatusRequestsForServiceImport(ctx, mgr.GetClient(), object)
	})

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		Named("udproute-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&gatewayv1alpha2.UDPRoute{}, generationChanged).
		Watches(
			&corev1.Service{},
			serviceRequests,
			builder.WithPredicates(routeBackendServiceDependencyPredicate()),
		)

	if serviceImportWatchSupported(mgr) {
		controllerBuilder = controllerBuilder.Watches(
			&mcsv1alpha1.ServiceImport{},
			serviceImportRequests,
			builder.WithPredicates(routeBackendServiceImportDependencyPredicate()),
		)
	}

	return controllerBuilder.Complete(c)
}

type tlsRouteController struct {
	reconciler *Reconciler
}

func (c *tlsRouteController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.ReconcileTLSRouteObject(ctx, req.NamespacedName)
}

func (c *tlsRouteController) SetupWithManager(mgr ctrl.Manager) error {
	serviceRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return tlsRouteStatusRequestsForService(ctx, mgr.GetClient(), object)
	})
	serviceImportRequests := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		return tlsRouteStatusRequestsForServiceImport(ctx, mgr.GetClient(), object)
	})

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		Named("tlsroute-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&gatewayv1alpha2.TLSRoute{}, generationChanged).
		Watches(
			&corev1.Service{},
			serviceRequests,
			builder.WithPredicates(routeBackendServiceDependencyPredicate()),
		)

	if serviceImportWatchSupported(mgr) {
		controllerBuilder = controllerBuilder.Watches(
			&mcsv1alpha1.ServiceImport{},
			serviceImportRequests,
			builder.WithPredicates(routeBackendServiceImportDependencyPredicate()),
		)
	}

	return controllerBuilder.Complete(c)
}

type listenerSetController struct {
	reconciler *Reconciler
}

func (c *listenerSetController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.ReconcileListenerSetObject(ctx, req.NamespacedName)
}

func (c *listenerSetController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("listenerset-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&gatewayv1.ListenerSet{}, generationChanged).
		Complete(c)
}

func gatewayListenerSetStatusRequests(
	_ context.Context,
	object client.Object,
) []reconcile.Request {
	ls, ok := object.(*gatewayv1.ListenerSet)
	if !ok || ls == nil {
		return nil
	}
	name := string(ls.Spec.ParentRef.Name)
	if name == "" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: listenerSetParentGatewayNamespace(*ls),
			Name:      name,
		},
	}}
}

type backendLBPolicyController struct {
	reconciler *Reconciler
}

func (c *backendLBPolicyController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.reconcileBackendLBPolicyObject(ctx, req.NamespacedName)
}

func (c *backendLBPolicyController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("backendlbpolicy-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&backend.BackendLBPolicy{}, generationChanged).
		Complete(c)
}

type backendTLSPolicyController struct {
	reconciler *Reconciler
}

func (c *backendTLSPolicyController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.reconcileBackendTLSPolicyObject(ctx, req.NamespacedName)
}

func (c *backendTLSPolicyController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("backendtlspolicy-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(gatewayapi.NewBackendTLSPolicyV1Object(), generationChanged).
		Complete(c)
}

type aiserviceController struct {
	reconciler *Reconciler
}

func (c *aiserviceController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.reconcileAIServiceObject(ctx, req.NamespacedName)
}

func (c *aiserviceController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("aiservice-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&aiservice.AIService{}, generationChanged).
		Complete(c)
}

type tokenPolicyController struct {
	reconciler *Reconciler
}

func (c *tokenPolicyController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.reconcileTokenPolicyObject(ctx, req.NamespacedName)
}

func (c *tokenPolicyController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("tokenpolicy-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&tokenpolicy.TokenPolicy{}, generationChanged).
		Complete(c)
}

type wasmPluginController struct {
	reconciler *Reconciler
}

func (c *wasmPluginController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.reconcileWasmPluginObject(ctx, req.NamespacedName)
}

func (c *wasmPluginController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("wasmplugin-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&wasmplugin.WasmPlugin{}, generationChanged).
		Complete(c)
}

type routePolicyController struct {
	reconciler *Reconciler
}

func (c *routePolicyController) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	return ctrl.Result{}, c.reconciler.reconcileRoutePolicyObject(ctx, req.NamespacedName)
}

func (c *routePolicyController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("routepolicy-status").
		WithOptions(statusControllerOptions(c.reconciler.options)).
		For(&routepolicy.RoutePolicy{}, generationChanged).
		Complete(c)
}
