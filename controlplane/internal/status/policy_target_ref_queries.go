package status

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/controlplane/internal/gatewayapi"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
)

const (
	statusBackendTLSPolicyTargetRefIndex = "nantian.dev/status.backendtlspolicy.target-ref"
	statusBackendLBPolicyTargetRefIndex  = "nantian.dev/status.backendlbpolicy.target-ref"
)

func setupPolicyTargetRefIndexes(ctx context.Context, indexer client.FieldIndexer) error {
	if err := indexer.IndexField(
		ctx,
		gatewayapi.NewBackendTLSPolicyV1Object(),
		statusBackendTLSPolicyTargetRefIndex,
		statusBackendTLSPolicyTargetRefIndexKeys,
	); err != nil && !isOptionalPolicyIndexUnavailable(err) {
		return fmt.Errorf("index BackendTLSPolicy target refs: %w", err)
	}
	if err := indexer.IndexField(
		ctx,
		&backendlbv1alpha2.BackendLBPolicy{},
		statusBackendLBPolicyTargetRefIndex,
		statusBackendLBPolicyTargetRefIndexKeys,
	); err != nil && !isOptionalPolicyIndexUnavailable(err) {
		return fmt.Errorf("index BackendLBPolicy target refs: %w", err)
	}
	return nil
}

func isOptionalPolicyIndexUnavailable(err error) bool {
	return meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err)
}

func statusBackendTLSPolicyTargetRefIndexKeys(object client.Object) []string {
	switch item := object.(type) {
	case *gatewayv1alpha3.BackendTLSPolicy:
		return statusBackendTLSPolicyTargetRefValues(item.Spec.TargetRefs)
	case *unstructured.Unstructured:
		if item == nil || item.GroupVersionKind() != gatewayapi.BackendTLSPolicyV1GVK {
			return nil
		}
		policy, err := gatewayapi.DecodeBackendTLSPolicyV1(item)
		if err != nil {
			return nil
		}
		return statusBackendTLSPolicyTargetRefValues(policy.Spec.TargetRefs)
	default:
		return nil
	}
}

func statusBackendLBPolicyTargetRefIndexKeys(object client.Object) []string {
	policy, ok := object.(*backendlbv1alpha2.BackendLBPolicy)
	if !ok || policy == nil {
		return nil
	}
	return statusBackendLBPolicyTargetRefValues(policy.Spec.TargetRefs)
}

func statusBackendTLSPolicyTargetRefValues(
	targetRefs []gatewayv1.LocalPolicyTargetReferenceWithSectionName,
) []string {
	values := make(map[string]struct{}, len(targetRefs))
	for _, targetRef := range targetRefs {
		value := backendPolicyTargetRefIndexValue(
			string(targetRef.Group),
			string(targetRef.Kind),
			string(targetRef.Name),
		)
		if value == "" {
			continue
		}
		values[value] = struct{}{}
	}
	return sortedBackendPolicyTargetRefValues(values)
}

func statusBackendLBPolicyTargetRefValues(
	targetRefs []backendlbv1alpha2.LocalPolicyTargetReference,
) []string {
	values := make(map[string]struct{}, len(targetRefs))
	for _, targetRef := range targetRefs {
		value := backendPolicyTargetRefIndexValue(
			string(targetRef.Group),
			string(targetRef.Kind),
			string(targetRef.Name),
		)
		if value == "" {
			continue
		}
		values[value] = struct{}{}
	}
	return sortedBackendPolicyTargetRefValues(values)
}

func (r *Reconciler) loadRouteReferencedBackendPolicies(
	ctx context.Context,
	state *clusterState,
) error {
	return r.loadAllBackendPolicies(ctx, state)
}

func (r *Reconciler) loadAllBackendPolicies(ctx context.Context, state *clusterState) error {
	var backendLBPolicies backendlbv1alpha2.BackendLBPolicyList
	if err := r.listReader.List(ctx, &backendLBPolicies); err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
		return err
	}
	state.backendLBPolicies = backendLBPolicies.Items

	backendTLSPolicies, err := gatewayapi.ListBackendTLSPoliciesV1(ctx, r.listReader)
	if err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
		return err
	}
	state.backendTLSPolicies = backendTLSPolicies

	return nil
}

func collectRouteBackendPolicyRefs(
	state *clusterState,
) (map[string]client.ObjectKey, map[string]client.ObjectKey) {
	services := make(map[string]client.ObjectKey)
	serviceImports := make(map[string]client.ObjectKey)

	for _, route := range state.httpRoutes {
		collectRouteBackendPolicyRefsForRoute(services, serviceImports, httpRouteInput(route))
	}
	for _, route := range state.grpcRoutes {
		collectRouteBackendPolicyRefsForRoute(services, serviceImports, grpcRouteInput(route))
	}
	for _, route := range state.tcpRoutes {
		collectRouteBackendPolicyRefsForRoute(services, serviceImports, tcpRouteInput(route))
	}
	for _, route := range state.udpRoutes {
		collectRouteBackendPolicyRefsForRoute(services, serviceImports, udpRouteInput(route))
	}
	for _, route := range state.tlsRoutes {
		collectRouteBackendPolicyRefsForRoute(services, serviceImports, tlsRouteInput(route))
	}

	return services, serviceImports
}

func collectRouteBackendPolicyRefsForRoute(
	services map[string]client.ObjectKey,
	serviceImports map[string]client.ObjectKey,
	route routeInput,
) {
	for _, backend := range route.backends {
		targetKind, ok := backendKindForStatus(backend.Group, backend.Kind)
		if !ok {
			continue
		}

		targetNamespace := strings.TrimSpace(backend.Namespace)
		if targetNamespace == "" {
			targetNamespace = route.namespace
		}

		switch targetKind {
		case "Service":
			addObjectKeyRef(services, targetNamespace, backend.Name)
		case "ServiceImport":
			addObjectKeyRef(serviceImports, targetNamespace, backend.Name)
		}
	}
}

func sortedBackendPolicyNamespaces(
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) []string {
	namespaces := make(map[string]struct{}, len(serviceKeys)+len(serviceImportKeys))
	for _, key := range serviceKeys {
		if key.Namespace != "" {
			namespaces[key.Namespace] = struct{}{}
		}
	}
	for _, key := range serviceImportKeys {
		if key.Namespace != "" {
			namespaces[key.Namespace] = struct{}{}
		}
	}

	out := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}

func loadBackendTLSPoliciesForNamespaces(
	ctx context.Context,
	reader client.Reader,
	namespaces []string,
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) ([]gatewayv1alpha3.BackendTLSPolicy, error) {
	if len(namespaces) == 0 {
		return nil, nil
	}

	targetValuesByNamespace := backendPolicyTargetRefIndexValuesByNamespace(serviceKeys, serviceImportKeys)
	policies := make([]gatewayv1alpha3.BackendTLSPolicy, 0)
	seen := make(map[string]struct{})
	for _, namespace := range namespaces {
		items, err := listBackendTLSPoliciesForNamespaceTargets(
			ctx,
			reader,
			namespace,
			targetValuesByNamespace[namespace],
		)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if !backendTLSPolicyTouchesKeys(item.Namespace, item.Spec.TargetRefs, serviceKeys, serviceImportKeys) {
				continue
			}
			key := namespacedName(item.Namespace, item.Name)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			policies = append(policies, item)
		}
	}

	sort.Slice(policies, func(i, j int) bool {
		left := namespacedName(policies[i].Namespace, policies[i].Name)
		right := namespacedName(policies[j].Namespace, policies[j].Name)
		return left < right
	})
	return policies, nil
}

func loadBackendLBPoliciesForNamespaces(
	ctx context.Context,
	reader client.Reader,
	namespaces []string,
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) ([]backendlbv1alpha2.BackendLBPolicy, error) {
	if len(namespaces) == 0 {
		return nil, nil
	}

	targetValuesByNamespace := backendPolicyTargetRefIndexValuesByNamespace(serviceKeys, serviceImportKeys)
	policies := make([]backendlbv1alpha2.BackendLBPolicy, 0)
	seen := make(map[string]struct{})
	for _, namespace := range namespaces {
		items, err := listBackendLBPoliciesForNamespaceTargets(
			ctx,
			reader,
			namespace,
			targetValuesByNamespace[namespace],
		)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if !backendLBPolicyTouchesKeys(item.Namespace, item.Spec.TargetRefs, serviceKeys, serviceImportKeys) {
				continue
			}
			key := namespacedName(item.Namespace, item.Name)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			policies = append(policies, item)
		}
	}

	sort.Slice(policies, func(i, j int) bool {
		left := namespacedName(policies[i].Namespace, policies[i].Name)
		right := namespacedName(policies[j].Namespace, policies[j].Name)
		return left < right
	})
	return policies, nil
}

func listBackendTLSPoliciesForNamespaceTargets(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	targetValues []string,
) ([]gatewayv1alpha3.BackendTLSPolicy, error) {
	if len(targetValues) == 0 {
		return nil, nil
	}

	items, usedIndex, err := listBackendTLSPoliciesByTargetRefIndex(ctx, reader, namespace, targetValues)
	if err != nil {
		return nil, err
	}
	if usedIndex {
		return items, nil
	}

	return gatewayapi.ListBackendTLSPoliciesV1WithOptions(ctx, reader, client.InNamespace(namespace))
}

func listBackendTLSPoliciesByTargetRefIndex(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	targetValues []string,
) ([]gatewayv1alpha3.BackendTLSPolicy, bool, error) {
	policies := make([]gatewayv1alpha3.BackendTLSPolicy, 0)
	seen := make(map[string]struct{})

	for _, targetValue := range targetValues {
		items, err := gatewayapi.ListBackendTLSPoliciesV1WithOptions(
			ctx,
			reader,
			client.InNamespace(namespace),
			client.MatchingFields{statusBackendTLSPolicyTargetRefIndex: targetValue},
		)
		if err != nil {
			if isMissingFieldIndexError(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		for _, item := range items {
			key := namespacedName(item.Namespace, item.Name)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			policies = append(policies, item)
		}
	}

	return policies, true, nil
}

func listBackendLBPoliciesForNamespaceTargets(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	targetValues []string,
) ([]backendlbv1alpha2.BackendLBPolicy, error) {
	if len(targetValues) == 0 {
		return nil, nil
	}

	items, usedIndex, err := listBackendLBPoliciesByTargetRefIndex(ctx, reader, namespace, targetValues)
	if err != nil {
		return nil, err
	}
	if usedIndex {
		return items, nil
	}

	var list backendlbv1alpha2.BackendLBPolicyList
	if err := reader.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func listBackendLBPoliciesByTargetRefIndex(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	targetValues []string,
) ([]backendlbv1alpha2.BackendLBPolicy, bool, error) {
	policies := make([]backendlbv1alpha2.BackendLBPolicy, 0)
	seen := make(map[string]struct{})

	for _, targetValue := range targetValues {
		var list backendlbv1alpha2.BackendLBPolicyList
		if err := reader.List(
			ctx,
			&list,
			client.InNamespace(namespace),
			client.MatchingFields{statusBackendLBPolicyTargetRefIndex: targetValue},
		); err != nil {
			if isMissingFieldIndexError(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		for _, item := range list.Items {
			key := namespacedName(item.Namespace, item.Name)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			policies = append(policies, item)
		}
	}

	return policies, true, nil
}

func backendTLSPolicyTouchesKeys(
	namespace string,
	targetRefs []gatewayv1.LocalPolicyTargetReferenceWithSectionName,
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) bool {
	for _, targetRef := range targetRefs {
		key := namespacedName(namespace, string(targetRef.Name))
		switch {
		case string(targetRef.Group) == "" && string(targetRef.Kind) == "Service":
			if _, ok := serviceKeys[key]; ok {
				return true
			}
		case string(targetRef.Group) == mcsv1alpha1.GroupName &&
			string(targetRef.Kind) == "ServiceImport":
			if _, ok := serviceImportKeys[key]; ok {
				return true
			}
		}
	}
	return false
}

func backendLBPolicyTouchesKeys(
	namespace string,
	targetRefs []backendlbv1alpha2.LocalPolicyTargetReference,
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) bool {
	for _, targetRef := range targetRefs {
		key := namespacedName(namespace, string(targetRef.Name))
		switch {
		case string(targetRef.Group) == "" && string(targetRef.Kind) == "Service":
			if _, ok := serviceKeys[key]; ok {
				return true
			}
		case string(targetRef.Group) == mcsv1alpha1.GroupName &&
			string(targetRef.Kind) == "ServiceImport":
			if _, ok := serviceImportKeys[key]; ok {
				return true
			}
		}
	}
	return false
}

func backendPolicyTargetRefIndexValuesByNamespace(
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) map[string][]string {
	valuesByNamespace := make(map[string]map[string]struct{}, len(serviceKeys)+len(serviceImportKeys))
	add := func(namespace string, value string) {
		if namespace == "" || value == "" {
			return
		}
		if valuesByNamespace[namespace] == nil {
			valuesByNamespace[namespace] = make(map[string]struct{})
		}
		valuesByNamespace[namespace][value] = struct{}{}
	}

	for _, key := range serviceKeys {
		add(key.Namespace, backendPolicyTargetRefIndexValue("", "Service", key.Name))
	}
	for _, key := range serviceImportKeys {
		add(
			key.Namespace,
			backendPolicyTargetRefIndexValue(mcsv1alpha1.GroupName, "ServiceImport", key.Name),
		)
	}

	out := make(map[string][]string, len(valuesByNamespace))
	for namespace, values := range valuesByNamespace {
		out[namespace] = sortedBackendPolicyTargetRefValues(values)
	}
	return out
}

func backendPolicyTargetRefIndexValue(group string, kind string, name string) string {
	if kind == "" || name == "" {
		return ""
	}
	return group + "/" + kind + "/" + name
}

func sortedBackendPolicyTargetRefValues(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
