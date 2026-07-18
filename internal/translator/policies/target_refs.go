package policies

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	routepolicy "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
)

const (
	BackendTLSPolicyTargetRefIndex = "nantian.dev/translator.backendtlspolicy.target-ref"
	BackendLBPolicyTargetRefIndex  = "nantian.dev/translator.backendlbpolicy.target-ref"
	RoutePolicyTargetRefIndex      = "nantian.dev/translator.routepolicy.target-ref"
)

func SetupIndexes(ctx context.Context, indexer client.FieldIndexer) error {
	if err := indexer.IndexField(
		ctx,
		gatewayapi.NewBackendTLSPolicyV1Object(),
		BackendTLSPolicyTargetRefIndex,
		BackendTLSPolicyTargetRefIndexKeys,
	); err != nil && !isOptionalPolicyIndexUnavailable(err) {
		return fmt.Errorf("index BackendTLSPolicy target refs: %w", err)
	}
	if err := indexer.IndexField(
		ctx,
		&backend.BackendLBPolicy{},
		BackendLBPolicyTargetRefIndex,
		BackendLBPolicyTargetRefIndexKeys,
	); err != nil && !isOptionalPolicyIndexUnavailable(err) {
		return fmt.Errorf("index BackendLBPolicy target refs: %w", err)
	}
	if err := indexer.IndexField(
		ctx,
		&routepolicy.RoutePolicy{},
		RoutePolicyTargetRefIndex,
		RoutePolicyTargetRefIndexKeys,
	); err != nil && !isOptionalPolicyIndexUnavailable(err) {
		return fmt.Errorf("index RoutePolicy target refs: %w", err)
	}

	return nil
}

func isOptionalPolicyIndexUnavailable(err error) bool {
	return meta.IsNoMatchError(err) || k8sruntime.IsNotRegisteredError(err)
}

func BackendTLSPolicyTargetRefIndexKeys(object client.Object) []string {
	switch item := object.(type) {
	case *gatewayv1alpha3.BackendTLSPolicy:
		return backendTLSPolicyTargetRefValues(item.Spec.TargetRefs)
	case *unstructured.Unstructured:
		if item == nil || item.GroupVersionKind() != gatewayapi.BackendTLSPolicyV1GVK {
			return nil
		}
		policy, err := gatewayapi.DecodeBackendTLSPolicyV1(item)
		if err != nil {
			return nil
		}
		return backendTLSPolicyTargetRefValues(policy.Spec.TargetRefs)
	default:
		return nil
	}
}

func BackendLBPolicyTargetRefIndexKeys(object client.Object) []string {
	policy, ok := object.(*backend.BackendLBPolicy)
	if !ok || policy == nil {
		return nil
	}
	return backendLBPolicyTargetRefValues(policy.Spec.TargetRefs)
}

func RoutePolicyTargetRefIndexKeys(object client.Object) []string {
	policy, ok := object.(*routepolicy.RoutePolicy)
	if !ok || policy == nil {
		return nil
	}
	return routePolicyTargetRefValues(policy.Spec.TargetRefs)
}

func backendTLSPolicyTargetRefValues(
	targetRefs []gatewayv1.LocalPolicyTargetReferenceWithSectionName,
) []string {
	values := make(map[string]struct{}, len(targetRefs))
	for _, targetRef := range targetRefs {
		value := BackendPolicyTargetRefIndexValue(
			string(targetRef.Group),
			string(targetRef.Kind),
			string(targetRef.Name),
		)
		if value == "" {
			continue
		}
		values[value] = struct{}{}
	}
	return sortedIndexValues(values)
}

func backendLBPolicyTargetRefValues(
	targetRefs []backend.LocalPolicyTargetReference,
) []string {
	values := make(map[string]struct{}, len(targetRefs))
	for _, targetRef := range targetRefs {
		value := BackendPolicyTargetRefIndexValue(
			string(targetRef.Group),
			string(targetRef.Kind),
			string(targetRef.Name),
		)
		if value == "" {
			continue
		}
		values[value] = struct{}{}
	}
	return sortedIndexValues(values)
}

func routePolicyTargetRefValues(
	targetRefs []gatewayv1.LocalPolicyTargetReference,
) []string {
	values := make(map[string]struct{}, len(targetRefs))
	for _, targetRef := range targetRefs {
		value := BackendPolicyTargetRefIndexValue(
			string(targetRef.Group),
			string(targetRef.Kind),
			string(targetRef.Name),
		)
		if value == "" {
			continue
		}
		values[value] = struct{}{}
	}
	return sortedIndexValues(values)
}

func BackendPolicyTargetRefIndexValuesByNamespace(
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
		add(key.Namespace, BackendPolicyTargetRefIndexValue("", "Service", key.Name))
	}
	for _, key := range serviceImportKeys {
		add(
			key.Namespace,
			BackendPolicyTargetRefIndexValue(mcsv1alpha1.GroupName, "ServiceImport", key.Name),
		)
	}

	out := make(map[string][]string, len(valuesByNamespace))
	for namespace, values := range valuesByNamespace {
		out[namespace] = sortedIndexValues(values)
	}
	return out
}

func BackendPolicyTargetRefIndexValue(group string, kind string, name string) string {
	if kind == "" || name == "" {
		return ""
	}
	return group + "/" + kind + "/" + name
}

func sortedIndexValues(values map[string]struct{}) []string {
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
