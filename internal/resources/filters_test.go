package resources

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldAffectSnapshotIgnoresManagedFrontendResources(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nantian-dataplane",
			Namespace: "nantian-gw",
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
				ServiceRoleKey: ServiceRoleShared,
			},
		},
	}
	if ShouldAffectSnapshot(service) {
		t.Fatal("expected managed frontend service to be ignored")
	}

	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aeg-shared-ep-default-ipv4",
			Namespace: "nantian-gw",
			Labels: map[string]string{
				discoveryv1.LabelManagedBy: ManagedByValue,
				ServiceRoleKey:             EndpointSliceRoleSharedFrontend,
			},
		},
	}
	if ShouldAffectSnapshot(endpointSlice) {
		t.Fatal("expected managed frontend endpoint slice to be ignored")
	}
}

func TestShouldAffectSnapshotKeepsUserInputs(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo",
			Namespace: "default",
		},
	}
	if !ShouldAffectSnapshot(service) {
		t.Fatal("expected user service to affect snapshot")
	}

	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo-1",
			Namespace: "default",
			Labels: map[string]string{
				discoveryv1.LabelManagedBy: "endpointslice-controller.k8s.io",
				ServiceRoleKey:             EndpointSliceRoleSharedFrontend,
			},
		},
	}
	if !ShouldAffectSnapshot(endpointSlice) {
		t.Fatal("expected user endpoint slice to affect snapshot")
	}
}

func TestFilterServicesRemovesOnlyManagedFrontendServices(t *testing.T) {
	services := []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "nantian-gw",
				Name:      "shared",
				Labels: map[string]string{
					ManagedByLabel: ManagedByValue,
					ServiceRoleKey: ServiceRoleShared,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "nantian-gw",
				Name:      "gateway",
				Labels: map[string]string{
					ManagedByLabel: ManagedByValue,
					ServiceRoleKey: ServiceRoleGateway,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "echo",
				Labels: map[string]string{
					ManagedByLabel: ManagedByValue,
					ServiceRoleKey: "backend",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "orders"},
		},
	}

	got := FilterServices(services)
	wantNames := []string{"echo", "orders"}
	gotNames := serviceNames(got)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("FilterServices() names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestFilterEndpointSlicesRemovesOnlyManagedFrontendEndpointSlices(t *testing.T) {
	endpointSlices := []discoveryv1.EndpointSlice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "nantian-gw",
				Name:      "shared",
				Labels: map[string]string{
					discoveryv1.LabelManagedBy: ManagedByValue,
					ServiceRoleKey:             EndpointSliceRoleSharedFrontend,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "nantian-gw",
				Name:      "gateway",
				Labels: map[string]string{
					discoveryv1.LabelManagedBy: ManagedByValue,
					ServiceRoleKey:             EndpointSliceRoleGatewayFrontend,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "nantian-gw",
				Name:      "mesh",
				Labels: map[string]string{
					discoveryv1.LabelManagedBy: ManagedByValue,
					ServiceRoleKey:             EndpointSliceRoleMeshFrontend,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "echo",
				Labels: map[string]string{
					discoveryv1.LabelManagedBy: ManagedByValue,
					ServiceRoleKey:             "backend",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "orders"},
		},
	}

	got := FilterEndpointSlices(endpointSlices)
	wantNames := []string{"echo", "orders"}
	gotNames := endpointSliceNames(got)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("FilterEndpointSlices() names = %#v, want %#v", gotNames, wantNames)
	}
}

func serviceNames(services []corev1.Service) []string {
	out := make([]string, 0, len(services))
	for _, service := range services {
		out = append(out, service.Name)
	}
	return out
}

func endpointSliceNames(endpointSlices []discoveryv1.EndpointSlice) []string {
	out := make([]string, 0, len(endpointSlices))
	for _, endpointSlice := range endpointSlices {
		out = append(out, endpointSlice.Name)
	}
	return out
}
