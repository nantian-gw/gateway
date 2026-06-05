package managedresources

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldAffectSnapshotIgnoresManagedFrontendResources(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aether-gateway-dataplane",
			Namespace: "aether-gateway",
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
			Namespace: "aether-gateway",
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
