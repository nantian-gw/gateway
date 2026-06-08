package status

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	must(t, corev1.AddToScheme(scheme))
	must(t, discoveryv1.AddToScheme(scheme))
	must(t, apiextensionsv1.AddToScheme(scheme))
	must(t, gatewayv1.Install(scheme))
	must(t, gatewayv1alpha2.Install(scheme))
	must(t, backendlbv1alpha2.Install(scheme))
	must(t, gatewayv1alpha3.Install(scheme))
	must(t, gatewayv1beta1.Install(scheme))
	must(t, mcsv1alpha1.AddToScheme(scheme))
	return scheme
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func assertCondition(
	t *testing.T,
	conditions []metav1.Condition,
	condType string,
	status metav1.ConditionStatus,
	reason string,
	observedGeneration int64,
) {
	t.Helper()

	for _, condition := range conditions {
		if condition.Type != condType {
			continue
		}
		if condition.Status != status {
			t.Fatalf("condition %s status = %s, want %s", condType, condition.Status, status)
		}
		if condition.Reason != reason {
			t.Fatalf("condition %s reason = %s, want %s", condType, condition.Reason, reason)
		}
		if condition.ObservedGeneration != observedGeneration {
			t.Fatalf("condition %s observedGeneration = %d, want %d", condType, condition.ObservedGeneration, observedGeneration)
		}
		return
	}

	t.Fatalf("condition %s not found in %#v", condType, conditions)
}

func conditionMessage(t *testing.T, conditions []metav1.Condition, condType string) string {
	t.Helper()

	for _, condition := range conditions {
		if condition.Type == condType {
			return condition.Message
		}
	}

	t.Fatalf("condition %s not found in %#v", condType, conditions)
	return ""
}

func assertConditionMessage(t *testing.T, conditions []metav1.Condition, condType, want string) {
	t.Helper()

	if got := conditionMessage(t, conditions, condType); got != want {
		t.Fatalf("condition %s message = %q, want %q", condType, got, want)
	}
}

func assertConditionAbsent(t *testing.T, conditions []metav1.Condition, condType string) {
	t.Helper()

	for _, condition := range conditions {
		if condition.Type == condType {
			t.Fatalf("condition %s unexpectedly present in %#v", condType, conditions)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func portPtr(port int32) *gatewayv1.PortNumber {
	value := gatewayv1.PortNumber(port)
	return &value
}

func namespacePtr(namespace string) *gatewayv1.Namespace {
	value := gatewayv1.Namespace(namespace)
	return &value
}

func namespaceFromPtr(value gatewayv1.FromNamespaces) *gatewayv1.FromNamespaces {
	return &value
}

func gatewayAPICRD(name, version string) *apiextensionsv1.CustomResourceDefinition {
	annotations := map[string]string{}
	if version != "" {
		annotations[gatewayAPIBundleVersionAnnotation] = version
	}

	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: gatewayv1.GroupName,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   name,
				Singular: name,
				Kind:     "GatewayAPITest",
			},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    "v1",
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
					},
				},
			}},
		},
	}
}

func assertSupportedKinds(t *testing.T, supportedKinds []gatewayv1.RouteGroupKind, wantKinds ...gatewayv1.Kind) {
	t.Helper()

	if len(supportedKinds) != len(wantKinds) {
		t.Fatalf("supportedKinds length = %d, want %d (%#v)", len(supportedKinds), len(wantKinds), supportedKinds)
	}

	for index, wantKind := range wantKinds {
		if supportedKinds[index].Group == nil || *supportedKinds[index].Group != gatewayGroup {
			t.Fatalf("supportedKinds[%d] group = %#v, want %q", index, supportedKinds[index].Group, gatewayGroup)
		}
		if supportedKinds[index].Kind != wantKind {
			t.Fatalf("supportedKinds[%d] kind = %q, want %q", index, supportedKinds[index].Kind, wantKind)
		}
	}
}

func ptr[T any](value T) *T {
	return &value
}

type restrictedReader struct {
	client.Reader
	blockedListTypes map[reflect.Type]string
}

func (r restrictedReader) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if reason, blocked := r.blockedListTypes[reflect.TypeOf(list)]; blocked {
		return fmt.Errorf("unexpected List for %T: %s", list, reason)
	}

	return r.Reader.List(ctx, list, opts...)
}

type validatingListReader struct {
	client.Reader
	listValidators map[reflect.Type]func(client.ListOptions) error
}

func (r validatingListReader) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if validator, ok := r.listValidators[reflect.TypeOf(list)]; ok {
		var listOptions client.ListOptions
		for _, opt := range opts {
			opt.ApplyToList(&listOptions)
		}
		if err := validator(listOptions); err != nil {
			return fmt.Errorf("unexpected List for %T: %w", list, err)
		}
	}

	return r.Reader.List(ctx, list, opts...)
}

type countingGetReader struct {
	client.Reader
	gatewayGets          int
	gatewayClassGets     int
	serviceGets          int
	partialMetadataLists int
	serviceLists         int
	endpointSliceLists   int
}

func (r *countingGetReader) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption,
) error {
	if _, ok := obj.(*gatewayv1.Gateway); ok {
		r.gatewayGets++
	}
	if _, ok := obj.(*gatewayv1.GatewayClass); ok {
		r.gatewayClassGets++
	}
	if _, ok := obj.(*corev1.Service); ok {
		r.serviceGets++
	}

	return r.Reader.Get(ctx, key, obj, opts...)
}

func (r *countingGetReader) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	switch list.(type) {
	case *metav1.PartialObjectMetadataList:
		r.partialMetadataLists++
	case *corev1.ServiceList:
		r.serviceLists++
	case *discoveryv1.EndpointSliceList:
		r.endpointSliceLists++
	}

	return r.Reader.List(ctx, list, opts...)
}

type rawValidatingReader struct {
	client.Reader
	listValidators map[reflect.Type]func([]client.ListOption) error
}

func (r rawValidatingReader) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if validator, ok := r.listValidators[reflect.TypeOf(list)]; ok {
		if err := validator(opts); err != nil {
			return fmt.Errorf("unexpected List for %T: %w", list, err)
		}
	}

	return r.Reader.List(ctx, list, opts...)
}

type gatewayParentFilteringReader struct {
	client.Reader
}

func (r gatewayParentFilteringReader) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	switch typed := list.(type) {
	case *gatewayv1.HTTPRouteList:
		if value, ok := matchingFieldValue(opts, statusHTTPRouteGatewayParentIndex); ok {
			var full gatewayv1.HTTPRouteList
			if err := r.Reader.List(ctx, &full); err != nil {
				return err
			}
			typed.Items = filterHTTPRoutesByGatewayParent(full.Items, value)
			return nil
		}
	case *gatewayv1.GRPCRouteList:
		if value, ok := matchingFieldValue(opts, statusGRPCRouteGatewayParentIndex); ok {
			var full gatewayv1.GRPCRouteList
			if err := r.Reader.List(ctx, &full); err != nil {
				return err
			}
			typed.Items = filterGRPCRoutesByGatewayParent(full.Items, value)
			return nil
		}
	case *gatewayv1alpha2.TCPRouteList:
		if value, ok := matchingFieldValue(opts, statusTCPRouteGatewayParentIndex); ok {
			var full gatewayv1alpha2.TCPRouteList
			if err := r.Reader.List(ctx, &full); err != nil {
				return err
			}
			typed.Items = filterTCPRoutesByGatewayParent(full.Items, value)
			return nil
		}
	case *gatewayv1alpha2.UDPRouteList:
		if value, ok := matchingFieldValue(opts, statusUDPRouteGatewayParentIndex); ok {
			var full gatewayv1alpha2.UDPRouteList
			if err := r.Reader.List(ctx, &full); err != nil {
				return err
			}
			typed.Items = filterUDPRoutesByGatewayParent(full.Items, value)
			return nil
		}
	case *gatewayv1alpha2.TLSRouteList:
		if value, ok := matchingFieldValue(opts, statusTLSRouteGatewayParentIndex); ok {
			var full gatewayv1alpha2.TLSRouteList
			if err := r.Reader.List(ctx, &full); err != nil {
				return err
			}
			typed.Items = filterTLSRoutesByGatewayParent(full.Items, value)
			return nil
		}
	}

	return r.Reader.List(ctx, list, opts...)
}

func matchingFieldValue(opts []client.ListOption, field string) (string, bool) {
	for _, opt := range opts {
		matching, ok := opt.(client.MatchingFields)
		if !ok {
			continue
		}
		value, ok := matching[field]
		if ok {
			return value, true
		}
	}
	return "", false
}

func filterHTTPRoutesByGatewayParent(items []gatewayv1.HTTPRoute, value string) []gatewayv1.HTTPRoute {
	out := make([]gatewayv1.HTTPRoute, 0, len(items))
	for _, item := range items {
		if hasGatewayParentIndexValue(item.Spec.ParentRefs, item.Namespace, value) {
			out = append(out, item)
		}
	}
	return out
}

func filterGRPCRoutesByGatewayParent(items []gatewayv1.GRPCRoute, value string) []gatewayv1.GRPCRoute {
	out := make([]gatewayv1.GRPCRoute, 0, len(items))
	for _, item := range items {
		if hasGatewayParentIndexValue(item.Spec.ParentRefs, item.Namespace, value) {
			out = append(out, item)
		}
	}
	return out
}

func filterTCPRoutesByGatewayParent(items []gatewayv1alpha2.TCPRoute, value string) []gatewayv1alpha2.TCPRoute {
	out := make([]gatewayv1alpha2.TCPRoute, 0, len(items))
	for _, item := range items {
		if hasGatewayParentIndexValue(item.Spec.ParentRefs, item.Namespace, value) {
			out = append(out, item)
		}
	}
	return out
}

func filterUDPRoutesByGatewayParent(items []gatewayv1alpha2.UDPRoute, value string) []gatewayv1alpha2.UDPRoute {
	out := make([]gatewayv1alpha2.UDPRoute, 0, len(items))
	for _, item := range items {
		if hasGatewayParentIndexValue(item.Spec.ParentRefs, item.Namespace, value) {
			out = append(out, item)
		}
	}
	return out
}

func filterTLSRoutesByGatewayParent(items []gatewayv1alpha2.TLSRoute, value string) []gatewayv1alpha2.TLSRoute {
	out := make([]gatewayv1alpha2.TLSRoute, 0, len(items))
	for _, item := range items {
		if hasGatewayParentIndexValue(item.Spec.ParentRefs, item.Namespace, value) {
			out = append(out, item)
		}
	}
	return out
}

func hasGatewayParentIndexValue(parentRefs []gatewayv1.ParentReference, namespace string, value string) bool {
	for _, candidate := range gatewayParentStatusIndexKeys(parentRefs, namespace) {
		if candidate == value {
			return true
		}
	}
	return false
}

func requireMatchingFieldOption(opts []client.ListOption, field string, value string) error {
	for _, opt := range opts {
		matching, ok := opt.(client.MatchingFields)
		if !ok {
			continue
		}
		if matching[field] == value {
			return nil
		}
	}
	return fmt.Errorf("list must include matching field %s=%s", field, value)
}

func requireNamespaceOption(opts []client.ListOption, namespace string) error {
	for _, opt := range opts {
		inNamespace, ok := opt.(client.InNamespace)
		if !ok {
			continue
		}
		if string(inNamespace) == namespace {
			return nil
		}
	}
	return fmt.Errorf("list must include namespace %s", namespace)
}

func readStatusTLSAsset(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join("..", "..", "tests", "testdata", "tls", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(raw)
}
