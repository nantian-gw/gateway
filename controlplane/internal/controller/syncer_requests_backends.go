package controller

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapi"
	backendlbv1alpha2 "github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
)

func endpointSliceBackendReconcileRequests(slice *discoveryv1.EndpointSlice) []reconcile.Request {
	if slice == nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, 2)
	if serviceName := slice.Labels[discoveryv1.LabelServiceName]; serviceName != "" {
		requests = append(requests, snapshotBackendsReconcileRequestForService(client.ObjectKey{
			Namespace: slice.Namespace,
			Name:      serviceName,
		}))
	}
	if serviceImportName := slice.Labels[mcsv1alpha1.LabelServiceName]; serviceImportName != "" {
		requests = append(requests, snapshotBackendsReconcileRequestForServiceImport(client.ObjectKey{
			Namespace: slice.Namespace,
			Name:      serviceImportName,
		}))
	}
	if len(requests) != 0 {
		return requests
	}
	return []reconcile.Request{snapshotBackendsReconcileRequest}
}

func (s *Syncer) secretReconcileRequests(ctx context.Context, secret *corev1.Secret) []reconcile.Request {
	if secret == nil || secret.Name == "" {
		return nil
	}

	gateways, err := s.gatewaysForFieldIndex(
		ctx,
		gatewaySecretReferenceIndex,
		namespacedIndexValue(secret.Namespace, secret.Name),
	)
	if err != nil {
		s.logDependencyLookupError("Secret", secret.Namespace, secret.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}
	if len(gateways) == 0 {
		return nil
	}

	requests := make(map[reconcile.Request]struct{}, len(gateways))
	if err := s.addRelevantGatewayListenerRequests(ctx, requests, gateways); err != nil {
		s.logDependencyLookupError("Secret", secret.Namespace, secret.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}
	return sortedReconcileRequestsMap(requests)
}

func (s *Syncer) configMapReconcileRequests(ctx context.Context, configMap *corev1.ConfigMap) []reconcile.Request {
	if configMap == nil || configMap.Name == "" {
		return nil
	}

	indexValue := namespacedIndexValue(configMap.Namespace, configMap.Name)
	requests := make(map[reconcile.Request]struct{})

	gateways, err := s.gatewaysForFieldIndex(ctx, gatewayConfigMapReferenceIndex, indexValue)
	if err != nil {
		s.logDependencyLookupError("ConfigMap", configMap.Namespace, configMap.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}
	if err := s.addRelevantGatewayListenerRequests(ctx, requests, gateways); err != nil {
		s.logDependencyLookupError("ConfigMap", configMap.Namespace, configMap.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}

	httpRoutes, err := s.httpRoutesForConfigMapIndex(ctx, indexValue)
	if err != nil {
		s.logDependencyLookupError("ConfigMap", configMap.Namespace, configMap.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}
	for _, route := range httpRoutes {
		requests[snapshotHTTPRoutesReconcileRequestForKey(client.ObjectKeyFromObject(&route))] = struct{}{}
	}

	grpcRoutes, err := s.grpcRoutesForConfigMapIndex(ctx, indexValue)
	if err != nil {
		s.logDependencyLookupError("ConfigMap", configMap.Namespace, configMap.Name, err)
		return []reconcile.Request{snapshotReconcileRequest}
	}
	for _, route := range grpcRoutes {
		requests[snapshotGRPCRoutesReconcileRequestForKey(client.ObjectKeyFromObject(&route))] = struct{}{}
	}

	if s.backendTLSPolicyConfigMapIndexAvailable() {
		policies, usedIndex, err := s.backendTLSPoliciesForConfigMapIndex(ctx, indexValue)
		if err != nil {
			s.logDependencyLookupError("ConfigMap", configMap.Namespace, configMap.Name, err)
			return []reconcile.Request{snapshotReconcileRequest}
		}
		if usedIndex {
			if len(policies) != 0 {
				for _, request := range backendTLSPoliciesReconcileRequests(configMap.Namespace, policies) {
					requests[request] = struct{}{}
				}
			}
			return sortedReconcileRequestsMap(requests)
		}
		for _, policy := range policies {
			if backendTLSPolicyReferencesConfigMap(policy, configMap.Name) {
				for _, request := range backendTLSPoliciesReconcileRequests(configMap.Namespace, matchingBackendTLSPoliciesForConfigMap(policies, configMap.Name)) {
					requests[request] = struct{}{}
				}
				return sortedReconcileRequestsMap(requests)
			}
		}
	} else {
		policies, err := s.listBackendTLSPoliciesInNamespace(ctx, configMap.Namespace)
		if err != nil {
			s.logDependencyLookupError("ConfigMap", configMap.Namespace, configMap.Name, err)
			return []reconcile.Request{snapshotReconcileRequest}
		}
		for _, policy := range policies {
			if backendTLSPolicyReferencesConfigMap(policy, configMap.Name) {
				for _, request := range backendTLSPoliciesReconcileRequests(configMap.Namespace, matchingBackendTLSPoliciesForConfigMap(policies, configMap.Name)) {
					requests[request] = struct{}{}
				}
				return sortedReconcileRequestsMap(requests)
			}
		}
	}

	return sortedReconcileRequestsMap(requests)
}

func (s *Syncer) addRelevantGatewayListenerRequests(
	ctx context.Context,
	requests map[reconcile.Request]struct{},
	gateways []gatewayv1.Gateway,
) error {
	for _, gateway := range gateways {
		relevant, err := s.gatewayRelevantToCurrentSnapshot(ctx, &gateway)
		if err != nil {
			return err
		}
		if !relevant {
			continue
		}
		requests[snapshotGatewayListenersReconcileRequestForKey(client.ObjectKeyFromObject(&gateway))] = struct{}{}
	}
	return nil
}

func (s *Syncer) backendTLSPoliciesForConfigMapIndex(
	ctx context.Context,
	indexValue string,
) ([]gatewayv1alpha3.BackendTLSPolicy, bool, error) {
	list := gatewayapi.NewBackendTLSPolicyV1List()

	decode := func(_ client.ObjectList) ([]gatewayv1alpha3.BackendTLSPolicy, error) {
		policies := make([]gatewayv1alpha3.BackendTLSPolicy, 0, len(list.Items))
		for i := range list.Items {
			policy, err := gatewayapi.DecodeBackendTLSPolicyV1(&list.Items[i])
			if err != nil {
				return nil, err
			}
			policies = append(policies, policy)
		}
		return policies, nil
	}

	fallback := func(ctx context.Context) ([]gatewayv1alpha3.BackendTLSPolicy, error) {
		namespace, _ := splitNamespacedIndexValue(indexValue)
		return s.listBackendTLSPoliciesInNamespace(ctx, namespace)
	}

	return ListByIndexOrFallback(
		ctx,
		s.client,
		list,
		backendTLSPolicyConfigMapRefIndex,
		indexValue,
		decode,
		fallback,
		indexFallbackSemantics{
			Owner:          "snapshot-syncer",
			Kind:           "BackendTLSPolicy",
			Index:          backendTLSPolicyConfigMapRefIndex,
			IndexValue:     indexValue,
			RequestMapping: "backendTLSPoliciesForConfigMapIndex",
			FallbackScope:  "namespace",
		},
		func(semantics indexFallbackSemantics, err error) {
			s.logMissingFieldIndexFallbackOnce(semantics, err)
			s.setBackendTLSPolicyConfigMapIndexAvailable(false)
		},
	)
}

func (s *Syncer) listBackendTLSPoliciesInNamespace(
	ctx context.Context,
	namespace string,
) ([]gatewayv1alpha3.BackendTLSPolicy, error) {
	opts := make([]client.ListOption, 0, 1)
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}

	return gatewayapi.ListBackendTLSPoliciesV1WithOptions(ctx, s.client, opts...)
}

func splitNamespacedIndexValue(value string) (string, string) {
	for i := range value {
		if value[i] == '/' {
			return value[:i], value[i+1:]
		}
	}
	return "", value
}

func backendLBPolicyReconcileRequests(policy *backendlbv1alpha2.BackendLBPolicy) []reconcile.Request {
	if policy == nil {
		return nil
	}

	requests := make(map[reconcile.Request]struct{}, len(policy.Spec.TargetRefs))
	for _, targetRef := range policy.Spec.TargetRefs {
		addBackendPolicyTargetRequest(
			requests,
			policy.Namespace,
			string(targetRef.Group),
			string(targetRef.Kind),
			string(targetRef.Name),
		)
	}
	return finalizedBackendPolicyRequests(policy.Namespace, requests)
}

func backendTLSPolicyReconcileRequests(policy gatewayv1alpha3.BackendTLSPolicy) []reconcile.Request {
	requests := make(map[reconcile.Request]struct{}, len(policy.Spec.TargetRefs))
	for _, targetRef := range policy.Spec.TargetRefs {
		addBackendPolicyTargetRequest(
			requests,
			policy.Namespace,
			string(targetRef.Group),
			string(targetRef.Kind),
			string(targetRef.Name),
		)
	}
	return finalizedBackendPolicyRequests(policy.Namespace, requests)
}

func backendTLSPoliciesReconcileRequests(
	namespace string,
	policies []gatewayv1alpha3.BackendTLSPolicy,
) []reconcile.Request {
	requests := make(map[reconcile.Request]struct{}, len(policies))
	for _, policy := range policies {
		for _, targetRef := range policy.Spec.TargetRefs {
			addBackendPolicyTargetRequest(
				requests,
				policy.Namespace,
				string(targetRef.Group),
				string(targetRef.Kind),
				string(targetRef.Name),
			)
		}
	}
	return finalizedBackendPolicyRequests(namespace, requests)
}

func addBackendPolicyTargetRequest(
	requests map[reconcile.Request]struct{},
	namespace string,
	group string,
	kind string,
	name string,
) {
	if namespace == "" || name == "" {
		return
	}

	switch {
	case group == "" && kind == "Service":
		requests[snapshotBackendsReconcileRequestForService(client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		})] = struct{}{}
	case group == mcsv1alpha1.GroupName && kind == "ServiceImport":
		requests[snapshotBackendsReconcileRequestForServiceImport(client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		})] = struct{}{}
	}
}

func finalizedBackendPolicyRequests(
	fallbackNamespace string,
	requests map[reconcile.Request]struct{},
) []reconcile.Request {
	if len(requests) == 0 {
		if fallbackNamespace == "" {
			return []reconcile.Request{snapshotBackendsReconcileRequest}
		}
		return []reconcile.Request{snapshotBackendsReconcileRequestForNamespace(fallbackNamespace)}
	}

	out := make([]reconcile.Request, 0, len(requests))
	for request := range requests {
		out = append(out, request)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Namespace + "/" + out[i].Name
		right := out[j].Namespace + "/" + out[j].Name
		return left < right
	})
	return out
}

func matchingBackendTLSPoliciesForConfigMap(
	policies []gatewayv1alpha3.BackendTLSPolicy,
	configMapName string,
) []gatewayv1alpha3.BackendTLSPolicy {
	matching := make([]gatewayv1alpha3.BackendTLSPolicy, 0, len(policies))
	for _, policy := range policies {
		if backendTLSPolicyReferencesConfigMap(policy, configMapName) {
			matching = append(matching, policy)
		}
	}
	return matching
}
