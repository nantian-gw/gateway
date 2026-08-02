package controller

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/client-go/util/workqueue"
	k8sptr "k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	routepolicy "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
	"github.com/nantian-gw/gateway/internal/resources"
)

func (s *Syncer) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	tracer := otel.Tracer("github.com/nantian-gw/gateway/internal/controller")
	ctx, span := tracer.Start(ctx, "controller.reconcile")
	defer span.End()

	scope, attachmentNamespace, backendNamespace, gatewayKeys, serviceKeys, serviceImportKeys, routeKeys := s.buildScopeForRequest(request)

	bypassed := s.settleDelay <= 0 || s.shouldBypassSettleDelay(ctx, request)
	span.SetAttributes(
		attribute.String("controller.request", request.String()),
		attribute.String("controller.scope", scope.String()),
		attribute.Bool("controller.bypass_settle", bypassed),
	)

	if !bypassed {
		span.SetAttributes(attribute.String("controller.settle_delay", s.settleDelay.String()))
	}

	if bypassed {
		published, err := s.publishSnapshotWithScope(
			ctx,
			scope,
			attachmentNamespaces(attachmentNamespace),
			backendNamespaces(backendNamespace),
			gatewayKeys,
			serviceKeys,
			serviceImportKeys,
			routeKeys,
		)
		if err != nil {
			s.mergeRetryPendingBuild(
				scope,
				attachmentNamespaces(attachmentNamespace),
				backendNamespaces(backendNamespace),
				gatewayKeys,
				serviceKeys,
				serviceImportKeys,
				routeKeys,
			)
			return ctrl.Result{}, nil
		}
		if published {
			s.queueLeaderRun(scope)
		}
		return ctrl.Result{}, nil
	}
	s.queueScopedSettleRun(ctx, scope, attachmentNamespace, backendNamespace, gatewayKeys, serviceKeys, serviceImportKeys, routeKeys)
	return ctrl.Result{}, nil
}

func (s *Syncer) SetupWithManager(mgr ctrl.Manager) error {
	if err := s.setupReferenceIndexes(context.Background(), mgr); err != nil {
		return err
	}

	maxConcurrent := s.options.MaxConcurrentReconciles
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	baseDelay := s.options.RateLimiterBaseDelay
	if baseDelay <= 0 {
		baseDelay = 200 * time.Millisecond
	}
	maxDelay := s.options.RateLimiterMaxDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}
	qps := s.options.RateLimiterQPS
	if qps <= 0 {
		qps = 10
	}
	bucketSize := s.options.RateLimiterBucketSize
	if bucketSize <= 0 {
		bucketSize = 100
	}

	snapshotMutationPredicate := builder.WithPredicates(snapshotInputMutationPredicate())
	listenerSetMutationPredicate := builder.WithPredicates(snapshotListenerSetMutationPredicate())
	snapshotInputPredicate := builder.WithPredicates(
		predicate.NewPredicateFuncs(resources.ShouldAffectSnapshot),
	)
	snapshotRequests := EnqueueRequestsFromX(s.snapshotReconcileRequests)

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		Named("snapshot-syncer").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrent,
			NeedLeaderElection:      k8sptr.To(false),
			RateLimiter: workqueue.NewTypedMaxOfRateLimiter(
				workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](baseDelay, maxDelay),
				&workqueue.TypedBucketRateLimiter[reconcile.Request]{
					Limiter: rate.NewLimiter(rate.Limit(qps), bucketSize),
				},
			),
		}).
		Watches(&gatewayv1.GatewayClass{}, snapshotRequests, snapshotMutationPredicate).
		Watches(&gatewayv1.Gateway{}, snapshotRequests, snapshotMutationPredicate).
		Watches(&gatewayv1.HTTPRoute{}, snapshotRequests, snapshotMutationPredicate).
		Watches(&gatewayv1.GRPCRoute{}, snapshotRequests, snapshotMutationPredicate).
		Watches(&gatewayv1beta1.ReferenceGrant{}, snapshotRequests, snapshotMutationPredicate).
		Watches(&corev1.Service{}, snapshotRequests, snapshotInputPredicate).
		Watches(&corev1.Pod{}, snapshotRequests, snapshotInputPredicate).
		Watches(&corev1.Namespace{}, snapshotRequests).
		Watches(&corev1.Secret{}, snapshotRequests).
		Watches(&corev1.ConfigMap{}, snapshotRequests).
		Watches(&discoveryv1.EndpointSlice{}, snapshotRequests, snapshotInputPredicate)

	if s.options.EnableExperimentalGateway && resourceSupported(mgr, &gatewayv1alpha2.TCPRoute{}) {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1alpha2.TCPRoute{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if s.options.EnableExperimentalGateway && resourceSupported(mgr, &gatewayv1alpha2.UDPRoute{}) {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1alpha2.UDPRoute{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if s.options.EnableExperimentalGateway && resourceSupported(mgr, &gatewayv1alpha2.TLSRoute{}) {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1alpha2.TLSRoute{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if s.options.EnableExperimentalGateway && resourceSupported(mgr, &gatewayv1.ListenerSet{}) {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1.ListenerSet{},
			snapshotRequests,
			listenerSetMutationPredicate,
		)
	}

	if backendTLSPolicyV1Supported(mgr) {
		controllerBuilder = controllerBuilder.Watches(
			gatewayapi.NewBackendTLSPolicyV1Object(),
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &backend.BackendLBPolicy{}) {
		controllerBuilder = controllerBuilder.Watches(
			&backend.BackendLBPolicy{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &mcsv1alpha1.ServiceImport{}) {
		controllerBuilder = controllerBuilder.Watches(
			&mcsv1alpha1.ServiceImport{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &aiservice.AIService{}) {
		controllerBuilder = controllerBuilder.Watches(
			&aiservice.AIService{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &tokenpolicy.TokenPolicy{}) {
		controllerBuilder = controllerBuilder.Watches(
			&tokenpolicy.TokenPolicy{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &wasmplugin.WasmPlugin{}) {
		controllerBuilder = controllerBuilder.Watches(
			&wasmplugin.WasmPlugin{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &routepolicy.RoutePolicy{}) {
		controllerBuilder = controllerBuilder.Watches(
			&routepolicy.RoutePolicy{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}

	return controllerBuilder.Complete(s)
}

func resourceSupported(mgr ctrl.Manager, object client.Object) bool {
	gvk, err := apiutil.GVKForObject(object, mgr.GetScheme())
	if err != nil {
		return false
	}
	_, err = mgr.GetRESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	return err == nil
}

func backendTLSPolicyV1Supported(mgr ctrl.Manager) bool {
	_, err := mgr.GetRESTMapper().RESTMapping(
		gatewayapi.BackendTLSPolicyV1GVK.GroupKind(),
		gatewayapi.BackendTLSPolicyV1GVK.Version,
	)
	return err == nil
}
