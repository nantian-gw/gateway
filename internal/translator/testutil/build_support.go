package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
)

// Field-index constants mirrored from translator non-test sources.
const (
	gatewayClassControllerNameIndex = "nantian.dev/infrastructure.gatewayclass.controller-name"
	gatewayGatewayClassNameIndex    = "nantian.dev/infrastructure.gateway.gatewayclass-name"
	listenerSetParentGatewayField   = "nantian.dev/snapshot.listenerset.parent-gateways"
	backendTLSPolicyTargetRefField  = "nantian.dev/translator.backendtlspolicy.target-ref"
	backendLBPolicyTargetRefField   = "nantian.dev/translator.backendlbpolicy.target-ref"
)

// Ptr is a generic helper that returns a pointer to the given value.
func Ptr[T any](value T) *T { return &value }

// Must calls t.Fatalf when err is non-nil.
func Must(err error, t *testing.T) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// BuildSupportScheme returns a runtime.Scheme pre-registered with the
// Gateway API, Service API, core, and discovery types needed for
// translator tests.
func BuildSupportScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	Must(gatewayv1.Install(scheme), t)
	Must(gatewayv1alpha2.Install(scheme), t)
	Must(gatewayv1alpha3.Install(scheme), t)
	Must(gatewayv1beta1.Install(scheme), t)
	Must(backend.Install(scheme), t)
	Must(mcsv1alpha1.Install(scheme), t)
	Must(corev1.AddToScheme(scheme), t)
	Must(discoveryv1.AddToScheme(scheme), t)
	return scheme
}

// NewTranslatorClientBuilder returns a fake controller-runtime
// ClientBuilder pre-configured with the Gateway API scheme and field
// indexes for GatewayClasses, Gateways, and ListenerSets.
func NewTranslatorClientBuilder(scheme *runtime.Scheme) *fake.ClientBuilder {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, gatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, gatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok || gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithIndex(&gatewayv1.ListenerSet{}, listenerSetParentGatewayField, listenerSetParentGatewayIndexKeys)
}

// listenerSetParentGatewayIndexKeys mirrored from translator.
func listenerSetParentGatewayIndexKeys(object client.Object) []string {
	ls, ok := object.(*gatewayv1.ListenerSet)
	if !ok || ls == nil || ls.Spec.ParentRef.Name == "" {
		return nil
	}
	namespace := ls.Namespace
	if ls.Spec.ParentRef.Namespace != nil {
		namespace = string(*ls.Spec.ParentRef.Namespace)
	}
	return []string{namespace + "/" + string(ls.Spec.ParentRef.Name)}
}

// backendTLSPolicyTargetRefIndexKeys mirrored from translator.
func NewFakeValidatingTranslatorClient(c client.Client, forbiddenLists map[reflect.Type]string) client.Client {
	return &fakeValidatingTranslatorClient{Client: c, ForbiddenLists: forbiddenLists}
}

type fakeValidatingTranslatorClient struct {
	client.Client
	ForbiddenLists map[reflect.Type]string
}

func (c *fakeValidatingTranslatorClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if message, ok := c.ForbiddenLists[reflect.TypeOf(list)]; ok {
		return fmt.Errorf("unexpected List for %T: %s", list, message)
	}
	return c.Client.List(ctx, list, opts...)
}

// NewFakeScopedBuildDependencyValidatingClient returns a client that
// validates namespace-scoped lists during build.
func NewFakeScopedBuildDependencyValidatingClient(c client.Client, expectedPodNamespaces map[string]struct{}) client.Client {
	return &fakeScopedBuildDependencyValidatingClient{Client: c, ExpectedPodNamespaces: expectedPodNamespaces}
}

type fakeScopedBuildDependencyValidatingClient struct {
	client.Client
	ExpectedPodNamespaces map[string]struct{}
}

func (c *fakeScopedBuildDependencyValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}

	switch typed := list.(type) {
	case *corev1.PodList:
		if len(c.ExpectedPodNamespaces) == 0 {
			return fmt.Errorf("build should not list Pods when no mesh route namespaces are referenced")
		}
		if listOptions.Namespace == "" {
			return fmt.Errorf("pod list must be namespace-scoped")
		}
		if _, ok := c.ExpectedPodNamespaces[listOptions.Namespace]; !ok {
			return fmt.Errorf("unexpected Pod list namespace %q", listOptions.Namespace)
		}
	case *gatewayv1beta1.ReferenceGrantList:
		if listOptions.Namespace == "" {
			return fmt.Errorf("ReferenceGrant list must be namespace-scoped")
		}
	case *backend.BackendLBPolicyList:
		if listOptions.Namespace == "" {
			return fmt.Errorf("BackendLBPolicy list must be namespace-scoped")
		}
	case *gatewayv1alpha3.BackendTLSPolicyList:
		if listOptions.Namespace == "" {
			return fmt.Errorf("BackendTLSPolicy typed list must be namespace-scoped")
		}
	case *unstructured.UnstructuredList:
		if typed.GroupVersionKind() == gatewayapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") &&
			listOptions.Namespace == "" {
			return fmt.Errorf("BackendTLSPolicy list must be namespace-scoped")
		}
	}

	return c.Client.List(ctx, list, opts...)
}

// NewFakeScopedPolicyListValidatingClient returns a client that
// validates BackendLBPolicy and BackendTLSPolicy lists are
// namespace-scoped.
func NewFakeScopedPolicyListValidatingClient(c client.Client) client.Client {
	return &fakeScopedPolicyListValidatingClient{Client: c}
}

type fakeScopedPolicyListValidatingClient struct {
	client.Client
}

func (c *fakeScopedPolicyListValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	namespace := ListNamespace(opts)
	switch typed := list.(type) {
	case *backend.BackendLBPolicyList:
		if namespace == "" {
			return fmt.Errorf("BackendLBPolicy list must be namespace-scoped")
		}
	case *gatewayv1alpha3.BackendTLSPolicyList:
		if namespace == "" {
			return fmt.Errorf("BackendTLSPolicy typed list must be namespace-scoped")
		}
	case *unstructured.UnstructuredList:
		if typed.GroupVersionKind() == gatewayapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") &&
			namespace == "" {
			return fmt.Errorf("BackendTLSPolicy list must be namespace-scoped")
		}
	}
	return c.Client.List(ctx, list, opts...)
}

// ListNamespace returns the Namespace field from a set of client.ListOptions.
func ListNamespace(opts []client.ListOption) string {
	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}
	return listOptions.Namespace
}

// NewFakeScopedReferenceGrantValidatingClient returns a client that
// validates ReferenceGrant lists are namespace-scoped.
func NewFakeScopedReferenceGrantValidatingClient(c client.Client) client.Client {
	return &fakeScopedReferenceGrantValidatingClient{Client: c}
}

type fakeScopedReferenceGrantValidatingClient struct {
	client.Client
}

func (c *fakeScopedReferenceGrantValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if _, ok := list.(*gatewayv1beta1.ReferenceGrantList); ok && ListNamespace(opts) == "" {
		return fmt.Errorf("ReferenceGrant list must be namespace-scoped")
	}
	return c.Client.List(ctx, list, opts...)
}

// NewFakeIndexedPolicyListValidatingClient returns a client that
// validates BackendLBPolicy and BackendTLSPolicy lists use target-ref
// field-selector indexes.
func NewFakeIndexedPolicyListValidatingClient(
	c client.Client,
	expectedTLSTargets map[string]struct{},
	expectedLBTargets map[string]struct{},
) client.Client {
	return &fakeIndexedPolicyListValidatingClient{
		Client:                    c,
		ExpectedBackendTLSTargets: expectedTLSTargets,
		ExpectedBackendLBTargets:  expectedLBTargets,
	}
}

type fakeIndexedPolicyListValidatingClient struct {
	client.Client
	ExpectedBackendTLSTargets map[string]struct{}
	ExpectedBackendLBTargets  map[string]struct{}
}

func (c *fakeIndexedPolicyListValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	switch typed := list.(type) {
	case *backend.BackendLBPolicyList:
		if err := RequireMatchingAnyField(opts, backendLBPolicyTargetRefField, c.ExpectedBackendLBTargets); err != nil {
			return err
		}
	case *gatewayv1alpha3.BackendTLSPolicyList:
		if err := RequireMatchingAnyField(opts, backendTLSPolicyTargetRefField, c.ExpectedBackendTLSTargets); err != nil {
			return err
		}
	case *unstructured.UnstructuredList:
		if typed.GroupVersionKind() == gatewayapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") {
			if err := RequireMatchingAnyField(opts, backendTLSPolicyTargetRefField, c.ExpectedBackendTLSTargets); err != nil {
				return err
			}
		}
	}
	return c.Client.List(ctx, list, opts...)
}

// RequireMatchingAnyField asserts that the field selector in opts
// matches at least one key in allowed.
func RequireMatchingAnyField(
	opts []client.ListOption,
	field string,
	allowed map[string]struct{},
) error {
	if len(allowed) == 0 {
		return nil
	}

	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}
	if listOptions.FieldSelector == nil || listOptions.FieldSelector.Empty() {
		return fmt.Errorf("list must include %s field selector", field)
	}
	for value := range allowed {
		if listOptions.FieldSelector.Matches(fields.Set{field: value}) {
			return nil
		}
	}
	return fmt.Errorf("field selector %q does not match any expected %s value", listOptions.FieldSelector.String(), field)
}

// backendPolicyTargetRefValue mirrors the function in policy_target_ref_indexes.go.
func backendPolicyTargetRefValue(group string, kind string, name string) string {
	if name == "" {
		return ""
	}
	return group + "/" + kind + "/" + name
}

// BackendLBPolicyTargetRefIndexKeys returns field-index values for the
// target refs of a BackendLBPolicy.
func BackendLBPolicyTargetRefIndexKeys(policy *backend.BackendLBPolicy) []string {
	if policy == nil {
		return nil
	}
	out := make([]string, 0, len(policy.Spec.TargetRefs))
	seen := make(map[string]struct{}, len(policy.Spec.TargetRefs))
	for _, targetRef := range policy.Spec.TargetRefs {
		value := backendPolicyTargetRefValue(
			string(targetRef.Group),
			string(targetRef.Kind),
			string(targetRef.Name),
		)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// NewFakeFieldSelectorRejectingClient returns a client that rejects
// field-selector queries for BackendTLSPolicy.
func NewFakeFieldSelectorRejectingClient(c client.Client) client.Client {
	return &fakeFieldSelectorRejectingClient{Client: c}
}

type fakeFieldSelectorRejectingClient struct {
	client.Client
}

func (c *fakeFieldSelectorRejectingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	typed, ok := list.(*unstructured.UnstructuredList)
	if !ok {
		return c.Client.List(ctx, list, opts...)
	}
	if typed.GroupVersionKind() != gatewayapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") {
		return c.Client.List(ctx, list, opts...)
	}

	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}
	if listOptions.FieldSelector != nil && !listOptions.FieldSelector.Empty() {
		return fmt.Errorf("field label not supported: %s", backendTLSPolicyTargetRefField)
	}

	return c.Client.List(ctx, list, opts...)
}

// ReadTestTLSAsset reads a TLS test asset from the repository testdata directory.
func ReadTestTLSAsset(t *testing.T, name string) []byte {
	t.Helper()

	candidates := []string{
		filepath.Join("..", "..", "test", "testdata", "tls", name),
		filepath.Join("..", "..", "..", "test", "testdata", "tls", name),
	}

	var lastErr error
	for _, path := range candidates {
		raw, err := os.ReadFile(path) //nolint:gosec // G304: test helper — path is controlled by test
		if err == nil {
			return raw
		}
		lastErr = err
	}
	t.Fatalf("read tls asset %s: %v", name, lastErr)
	return nil
}

// SectionNamePtr returns a pointer to a gateway SectionName.
func SectionNamePtr(value string) *gatewayv1.SectionName {
	item := gatewayv1.SectionName(value)
	return &item
}
