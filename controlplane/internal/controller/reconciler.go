package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	k8sptr "k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/controlplane/internal/gatewayapi"
	aiservicev1alpha1 "github.com/nantian-gw/gateway/controlplane/internal/gatewayapiexperimental/aiservicev1alpha1"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
	tokenpolicyv1alpha1 "github.com/nantian-gw/gateway/controlplane/internal/gatewayapiexperimental/tokenpolicyv1alpha1"
	wasmpluginv1alpha1 "github.com/nantian-gw/gateway/controlplane/internal/gatewayapiexperimental/wasmpluginv1alpha1"
	"github.com/nantian-gw/gateway/controlplane/internal/managedresources"
)

func (s *Syncer) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	scope, attachmentNamespace, backendNamespace, gatewayKeys, serviceKeys, serviceImportKeys, routeKeys := s.buildScopeForRequest(request)
	if s.settleDelay <= 0 || s.shouldBypassSettleDelay(ctx, request) {
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
	if err := s.setupReferenceIndexes(mgr); err != nil {
		return err
	}

	snapshotMutationPredicate := builder.WithPredicates(snapshotInputMutationPredicate())
	snapshotInputPredicate := builder.WithPredicates(
		predicate.NewPredicateFuncs(managedresources.ShouldAffectSnapshot),
	)
	snapshotRequests := EnqueueRequestsFromX(s.snapshotReconcileRequests)

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		Named("snapshot-syncer").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			// Each controlplane replica still rebuilds local snapshots, but reconcile
			// events are debounced through settleRun so bursty resource updates
			// converge to a stable snapshot before xDS clients apply it.
			NeedLeaderElection: k8sptr.To(false),
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

	if resourceSupported(mgr, &gatewayv1alpha2.TCPRoute{}) {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1alpha2.TCPRoute{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &gatewayv1alpha2.UDPRoute{}) {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1alpha2.UDPRoute{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &gatewayv1alpha2.TLSRoute{}) {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1alpha2.TLSRoute{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &gatewayv1.ListenerSet{}) {
		controllerBuilder = controllerBuilder.Watches(
			&gatewayv1.ListenerSet{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}

	if backendTLSPolicyV1Supported(mgr) {
		controllerBuilder = controllerBuilder.Watches(
			gatewayapi.NewBackendTLSPolicyV1Object(),
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &backendlbv1alpha2.BackendLBPolicy{}) {
		controllerBuilder = controllerBuilder.Watches(
			&backendlbv1alpha2.BackendLBPolicy{},
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
	if resourceSupported(mgr, &aiservicev1alpha1.AIService{}) {
		controllerBuilder = controllerBuilder.Watches(
			&aiservicev1alpha1.AIService{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &tokenpolicyv1alpha1.TokenPolicy{}) {
		controllerBuilder = controllerBuilder.Watches(
			&tokenpolicyv1alpha1.TokenPolicy{},
			snapshotRequests,
			snapshotMutationPredicate,
		)
	}
	if resourceSupported(mgr, &wasmpluginv1alpha1.WasmPlugin{}) {
		controllerBuilder = controllerBuilder.Watches(
			&wasmpluginv1alpha1.WasmPlugin{},
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
