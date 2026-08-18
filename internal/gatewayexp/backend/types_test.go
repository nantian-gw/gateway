package backend

import (
	"testing"
	"time"

	gatewaycorev1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBackendLBPolicyDeepCopyNewFields(t *testing.T) {
	slowStart := 30 * time.Second
	interval := 5 * time.Second
	policy := &BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: BackendLBPolicySpec{
			TargetRefs:         []LocalPolicyTargetReference{},
			SessionPersistence: &gatewaycorev1alpha2.SessionPersistence{SessionName: ptr("sess")},
			LoadBalancing: &LoadBalancingPolicy{
				Type:      ptr(LoadBalancingStrategyTypeLeastRequest),
				SlowStart: &SlowStartConfig{Window: &slowStart},
			},
			HealthCheck: &HealthCheckConfig{
				Type:               ptr("HTTP"),
				Path:               ptr("/healthz"),
				ExpectedStatus:     ptr[int32](200),
				Interval:           &interval,
				HealthyThreshold:   ptr(uint32(2)),
				UnhealthyThreshold: ptr(uint32(2)),
			},
			OutlierDetection: &OutlierDetectionConfig{
				Consecutive5xx:     ptr(uint32(3)),
				Interval:           &interval,
				BaseEjectionTime:   &interval,
				MaxEjectionPercent: ptr(uint32(50)),
			},
		},
	}

	cp := policy.DeepCopy()
	if cp.Spec.LoadBalancing.SlowStart == nil || cp.Spec.LoadBalancing.SlowStart.Window == nil || *cp.Spec.LoadBalancing.SlowStart.Window != slowStart {
		t.Fatal("SlowStart not deep-copied")
	}
	if cp.Spec.HealthCheck == nil || cp.Spec.HealthCheck.Path == nil || *cp.Spec.HealthCheck.Path != "/healthz" {
		t.Fatal("HealthCheck not deep-copied")
	}
	if cp.Spec.OutlierDetection == nil || cp.Spec.OutlierDetection.MaxEjectionPercent == nil || *cp.Spec.OutlierDetection.MaxEjectionPercent != 50 {
		t.Fatal("OutlierDetection not deep-copied")
	}
	// 修改副本不影响原对象
	*cp.Spec.HealthCheck.Path = "/changed"
	if *policy.Spec.HealthCheck.Path != "/healthz" {
		t.Fatal("DeepCopy shares pointer")
	}
}

func ptr[T any](v T) *T { return &v }
