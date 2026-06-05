package admin

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDiscoverDataplaneAdminEndpointsFromEndpointSlices(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := discoveryv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	ready := true
	notReady := false
	portName := "admin"
	port := int32(19080)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp-admin-a",
			Namespace: "aether-gateway",
			Labels: map[string]string{
				"kubernetes.io/service-name": "aether-gateway-dataplane-admin",
			},
		},
		Ports: []discoveryv1.EndpointPort{{Name: &portName, Port: &port}},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses:  []string{"10.244.0.10"},
				Conditions: discoveryv1.EndpointConditions{Ready: &ready},
				TargetRef:  &corev1.ObjectReference{Name: "dp-1"},
			},
			{
				Addresses:  []string{"10.244.0.11"},
				Conditions: discoveryv1.EndpointConditions{Ready: &notReady},
				TargetRef:  &corev1.ObjectReference{Name: "dp-2"},
			},
		},
	}).Build()

	discovery := NewDataplaneAdminDiscovery(client, DataplaneAdminDiscoveryConfig{
		Namespace:   "aether-gateway",
		ServiceName: "aether-gateway-dataplane-admin",
		PortName:    "admin",
	})

	endpoints, err := discovery.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints len = %d, want 1", len(endpoints))
	}
	if endpoints[0].NodeID != "dp-1" || endpoints[0].URL != "http://10.244.0.10:19080" {
		t.Fatalf("unexpected endpoint: %+v", endpoints[0])
	}
}