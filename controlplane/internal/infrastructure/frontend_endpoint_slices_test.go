package infrastructure

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestLoadServiceEndpointStateScopesPerServiceQueries(t *testing.T) {
	scheme := newScheme(t)

	baseClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "edge",
					Namespace: "default",
				},
			},
			&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "edge-canary",
					Namespace: "default",
				},
			},
			&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared",
					Namespace: defaultDataplaneNamespace,
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "edge-v4",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "edge",
						discoveryv1.LabelManagedBy:   managedByValue,
						serviceRoleLabel:             gatewayEndpointSliceRoleValue,
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "edge-canary-v4",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "edge-canary",
						discoveryv1.LabelManagedBy:   managedByValue,
						serviceRoleLabel:             gatewayEndpointSliceRoleValue,
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-v4",
					Namespace: defaultDataplaneNamespace,
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "shared",
						discoveryv1.LabelManagedBy:   managedByValue,
						serviceRoleLabel:             gatewayEndpointSliceRoleValue,
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
			},
		).
		Build()

	seenSliceLookups := make(map[string]int)
	state, err := loadServiceEndpointState(
		context.Background(),
		validatingClient{
			Client: baseClient,
			listValidators: map[reflect.Type]func(client.ListOptions) error{
				reflect.TypeOf(&corev1.EndpointsList{}): func(client.ListOptions) error {
					return fmt.Errorf("endpoint state loader must use Get for Endpoints")
				},
				reflect.TypeOf(&discoveryv1.EndpointSliceList{}): requireBatchedServiceEndpointSliceList(
					map[string][]string{
						"default":                 {"edge", "edge-canary"},
						defaultDataplaneNamespace: {"shared"},
					},
					seenSliceLookups,
				),
			},
		},
		map[string]struct{}{
			serviceKey("default", "edge"):                   {},
			serviceKey("default", "edge-canary"):            {},
			serviceKey(defaultDataplaneNamespace, "shared"): {},
		},
		gatewayEndpointSliceRoleValue,
	)
	if err != nil {
		t.Fatalf("loadServiceEndpointState returned error: %v", err)
	}

	if len(state.endpoints) != 3 {
		t.Fatalf("expected 2 scoped Endpoints gets, got %#v", state.endpoints)
	}
	if len(state.managedSlices) != 3 {
		t.Fatalf("expected managed EndpointSlices for all services, got %#v", state.managedSlices)
	}
	if seenSliceLookups[serviceKey("default", "edge")] != 1 {
		t.Fatalf("default/edge EndpointSlice lookup count = %d, want 1", seenSliceLookups[serviceKey("default", "edge")])
	}
	if seenSliceLookups[serviceKey("default", "edge-canary")] != 1 {
		t.Fatalf(
			"default/edge-canary EndpointSlice lookup count = %d, want 1",
			seenSliceLookups[serviceKey("default", "edge-canary")],
		)
	}
	if seenSliceLookups[serviceKey(defaultDataplaneNamespace, "shared")] != 1 {
		t.Fatalf(
			"%s/shared EndpointSlice lookup count = %d, want 1",
			defaultDataplaneNamespace,
			seenSliceLookups[serviceKey(defaultDataplaneNamespace, "shared")],
		)
	}
}

func requireBatchedServiceEndpointSliceList(
	expectedByNamespace map[string][]string,
	seen map[string]int,
) func(client.ListOptions) error {
	return func(opts client.ListOptions) error {
		serviceNames, ok := expectedByNamespace[opts.Namespace]
		if !ok {
			return fmt.Errorf("unexpected namespace %q for EndpointSlice lookup", opts.Namespace)
		}
		if opts.LabelSelector == nil || opts.LabelSelector.Empty() {
			return fmt.Errorf("EndpointSlice lookup for namespace %q must include service-name selector", opts.Namespace)
		}

		for _, serviceName := range serviceNames {
			if !opts.LabelSelector.Matches(labels.Set{discoveryv1.LabelServiceName: serviceName}) {
				return fmt.Errorf("selector %q does not match service-name=%s", opts.LabelSelector.String(), serviceName)
			}
			if opts.LabelSelector.Matches(labels.Set{discoveryv1.LabelServiceName: serviceName + "-other"}) {
				return fmt.Errorf("selector %q is broader than service-name=%s", opts.LabelSelector.String(), serviceName)
			}
			seen[serviceKey(opts.Namespace, serviceName)]++
		}
		return nil
	}
}
