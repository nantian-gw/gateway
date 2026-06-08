package infrastructure

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDesiredMeshEndpointSlicesFromDataplaneEndpointsMatchesDirectPlanning(t *testing.T) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nantian-dataplane-0",
				Namespace: defaultDataplaneNamespace,
			},
			Status: corev1.PodStatus{
				PodIP: "10.0.0.50",
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nantian-dataplane-1",
				Namespace: defaultDataplaneNamespace,
			},
			Status: corev1.PodStatus{
				PodIP: "fd00::50",
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nantian-dataplane-2",
				Namespace: defaultDataplaneNamespace,
			},
			Status: corev1.PodStatus{
				PodIP: "10.0.0.51",
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
	}

	want := desiredMeshEndpointSlices(service, pods)
	got := desiredMeshEndpointSlicesFromDataplaneEndpoints(
		service,
		meshDataplaneEndpoints(pods),
	)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("desiredMeshEndpointSlicesFromDataplaneEndpoints() mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}
