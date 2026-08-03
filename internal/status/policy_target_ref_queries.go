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

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
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
		&backend.BackendLBPolicy{},
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
	policy, ok := object.(*backend.BackendLBPolicy)
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
	targetRefs []backend.LocalPolicyTargetReference,
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
	var backendLBPolicies backend.BackendLBPolicyList
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
	targetRefs []backend.LocalPolicyTargetReference,
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
