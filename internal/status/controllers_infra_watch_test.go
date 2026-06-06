package status

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/nantian-gw/gateway/internal/managedresources"
)

func TestGatewayInfrastructureStatusRequests(t *testing.T) {
	t.Parallel()

	want := []reconcile.Request{{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "public"},
	}}

	testCases := []struct {
		name   string
		object client.Object
		want   []reconcile.Request
	}{
		{
			name: "gateway service enqueues owning gateway",
			object: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					managedresources.ManagedByLabel:          "nantian-gw",
					managedresources.ServiceRoleKey:          "gateway-metadata",
					"gateway.networking.k8s.io/gateway-name": "public",
					"nantian.dev/gateway-namespace":               "default",
				}},
			},
			want: want,
		},
		{
			name: "shared service is ignored",
			object: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					managedresources.ManagedByLabel:          "nantian-gw",
					managedresources.ServiceRoleKey:          "shared-dataplane",
					"gateway.networking.k8s.io/gateway-name": "public",
					"nantian.dev/gateway-namespace":               "default",
				}},
			},
		},
		{
			name: "gateway frontend endpoint slice enqueues owning gateway",
			object: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					discoveryv1.LabelManagedBy:               "nantian-gw",
					managedresources.ServiceRoleKey:          "gateway-frontend-endpoints",
					"gateway.networking.k8s.io/gateway-name": "public",
					"nantian.dev/gateway-namespace":               "default",
				}},
			},
			want: want,
		},
		{
			name: "shared frontend endpoint slice is ignored",
			object: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					discoveryv1.LabelManagedBy:               "nantian-gw",
					managedresources.ServiceRoleKey:          "shared-frontend-endpoints",
					"gateway.networking.k8s.io/gateway-name": "public",
					"nantian.dev/gateway-namespace":               "default",
				}},
			},
		},
		{
			name: "missing gateway labels are ignored",
			object: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					discoveryv1.LabelManagedBy:      "nantian-gw",
					managedresources.ServiceRoleKey: "gateway-frontend-endpoints",
				}},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := gatewayInfrastructureStatusRequests(context.Background(), tt.object); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("gatewayInfrastructureStatusRequests() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestGatewayFrontendEndpointSliceMetadataChanged(t *testing.T) {
	t.Parallel()

	base := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-ipv4",
			Namespace: "default",
			Labels: map[string]string{
				discoveryv1.LabelManagedBy:               "nantian-gw",
				managedresources.ServiceRoleKey:          managedresources.EndpointSliceRoleGatewayFrontend,
				"gateway.networking.k8s.io/gateway-name": "public",
				"nantian.dev/gateway-namespace":               "default",
			},
			Annotations: map[string]string{
				"nantian.dev/owner-generation": "1",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "public-gateway",
				UID:        types.UID("svc-1"),
			}},
		},
		Endpoints: []discoveryv1.Endpoint{{
			Addresses: []string{"10.0.0.1"},
		}},
	}

	testCases := []struct {
		name   string
		mutate func(*discoveryv1.EndpointSlice)
		want   bool
	}{
		{
			name: "endpoint-only change is ignored",
			mutate: func(slice *discoveryv1.EndpointSlice) {
				slice.Endpoints = []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.2"},
				}}
			},
			want: false,
		},
		{
			name: "label change triggers reconcile",
			mutate: func(slice *discoveryv1.EndpointSlice) {
				slice.Labels["nantian.dev/gateway-namespace"] = "infra"
			},
			want: true,
		},
		{
			name: "annotation change triggers reconcile",
			mutate: func(slice *discoveryv1.EndpointSlice) {
				slice.Annotations["nantian.dev/owner-generation"] = "2"
			},
			want: true,
		},
		{
			name: "owner reference change triggers reconcile",
			mutate: func(slice *discoveryv1.EndpointSlice) {
				slice.OwnerReferences[0].UID = types.UID("svc-2")
			},
			want: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := base.DeepCopy()
			tt.mutate(next)

			got := gatewayFrontendEndpointSliceMetadataChanged(event.UpdateEvent{
				ObjectOld: base,
				ObjectNew: next,
			})
			if got != tt.want {
				t.Fatalf("gatewayFrontendEndpointSliceMetadataChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGatewayInfrastructureServiceChanged(t *testing.T) {
	t.Parallel()

	base := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public-gateway",
			Namespace: "default",
			Labels: map[string]string{
				managedresources.ManagedByLabel:          "nantian-gw",
				managedresources.ServiceRoleKey:          managedresources.ServiceRoleGateway,
				"gateway.networking.k8s.io/gateway-name": "public",
				"nantian.dev/gateway-namespace":               "default",
			},
			Annotations: map[string]string{
				"nantian.dev/owner-generation": "1",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "gateway.networking.k8s.io/v1",
				Kind:       "Gateway",
				Name:       "public",
				UID:        types.UID("gw-1"),
			}},
			ResourceVersion: "1",
		},
		Spec: corev1.ServiceSpec{
			ExternalIPs:    []string{"192.0.2.10"},
			LoadBalancerIP: "192.0.2.20",
			Ports: []corev1.ServicePort{{
				Name: "http",
				Port: 80,
			}},
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{
					IP: "198.51.100.10",
				}},
			},
		},
	}

	testCases := []struct {
		name   string
		mutate func(*corev1.Service)
		want   bool
	}{
		{
			name: "resource-version-only change is ignored",
			mutate: func(service *corev1.Service) {
				service.ResourceVersion = "2"
			},
			want: false,
		},
		{
			name: "port-only change is ignored",
			mutate: func(service *corev1.Service) {
				service.Spec.Ports[0].Port = 8080
			},
			want: false,
		},
		{
			name: "label change triggers reconcile",
			mutate: func(service *corev1.Service) {
				service.Labels["nantian.dev/gateway-namespace"] = "infra"
			},
			want: true,
		},
		{
			name: "annotation change triggers reconcile",
			mutate: func(service *corev1.Service) {
				service.Annotations["nantian.dev/owner-generation"] = "2"
			},
			want: true,
		},
		{
			name: "owner reference change triggers reconcile",
			mutate: func(service *corev1.Service) {
				service.OwnerReferences[0].UID = types.UID("gw-2")
			},
			want: true,
		},
		{
			name: "external ips change triggers reconcile",
			mutate: func(service *corev1.Service) {
				service.Spec.ExternalIPs = []string{"192.0.2.11"}
			},
			want: true,
		},
		{
			name: "load balancer ip change triggers reconcile",
			mutate: func(service *corev1.Service) {
				service.Spec.LoadBalancerIP = "192.0.2.21"
			},
			want: true,
		},
		{
			name: "load balancer ingress change triggers reconcile",
			mutate: func(service *corev1.Service) {
				service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{
					Hostname: "gw.example.com",
				}}
			},
			want: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := base.DeepCopy()
			tt.mutate(next)

			got := gatewayInfrastructureServiceChanged(event.UpdateEvent{
				ObjectOld: base,
				ObjectNew: next,
			})
			if got != tt.want {
				t.Fatalf("gatewayInfrastructureServiceChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGatewayInfrastructureServicePredicateAllowsCreateAndDelete(t *testing.T) {
	t.Parallel()

	predicate := gatewayInfrastructureServicePredicate()
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			managedresources.ManagedByLabel:          managedresources.ManagedByValue,
			managedresources.ServiceRoleKey:          managedresources.ServiceRoleGateway,
			"gateway.networking.k8s.io/gateway-name": "public",
			"nantian.dev/gateway-namespace":               "default",
		}},
	}

	if !predicate.Create(event.CreateEvent{Object: service}) {
		t.Fatal("expected gateway Service create event to trigger status reconcile")
	}
	if !predicate.Delete(event.DeleteEvent{Object: service}) {
		t.Fatal("expected gateway Service delete event to trigger status reconcile")
	}
}

func TestGatewayFrontendEndpointSlicePredicateAllowsCreateAndDelete(t *testing.T) {
	t.Parallel()

	predicate := gatewayFrontendEndpointSlicePredicate()
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			discoveryv1.LabelManagedBy:               managedresources.ManagedByValue,
			managedresources.ServiceRoleKey:          managedresources.EndpointSliceRoleGatewayFrontend,
			"gateway.networking.k8s.io/gateway-name": "public",
			"nantian.dev/gateway-namespace":               "default",
		}},
	}

	if !predicate.Create(event.CreateEvent{Object: slice}) {
		t.Fatal("expected gateway frontend EndpointSlice create event to trigger status reconcile")
	}
	if !predicate.Delete(event.DeleteEvent{Object: slice}) {
		t.Fatal("expected gateway frontend EndpointSlice delete event to trigger status reconcile")
	}
}
