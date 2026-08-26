package shared

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/ir"
)

func TestValidateLimit(t *testing.T) {
	tests := []struct {
		name    string
		actual  int
		limit   int
		wantErr bool
	}{
		{"within limit", 5, 10, false},
		{"at limit", 10, 10, false},
		{"exceeds limit", 15, 10, true},
		{"unlimited", 100, 0, false},
		{"zero limit", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLimit("test", tt.actual, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLimit() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestSnapshotObjectCount(t *testing.T) {
	t.Run("nil snapshot", func(t *testing.T) {
		if got := SnapshotObjectCount(nil); got != 0 {
			t.Errorf("SnapshotObjectCount(nil) = %d, want 0", got)
		}
	})
	t.Run("populated snapshot", func(t *testing.T) {
		snap := &ir.Snapshot{
			Listeners:    make([]ir.Listener, 2),
			HTTPRoutes:   make([]ir.HTTPRoute, 3),
			GRPCRoutes:   make([]ir.GRPCRoute, 1),
			StreamRoutes: make([]ir.StreamRoute, 4),
			Backends:     make([]ir.BackendCluster, 5),
			Secrets:      make([]ir.SecretMaterial, 2),
			Workloads:    make([]ir.Workload, 1),
		}
		want := 2 + 3 + 1 + 4 + 5 + 2 + 1
		if got := SnapshotObjectCount(snap); got != want {
			t.Errorf("SnapshotObjectCount() = %d, want %d", got, want)
		}
	})
}

func TestSnapshotEndpointCount(t *testing.T) {
	t.Run("nil snapshot", func(t *testing.T) {
		if got := SnapshotEndpointCount(nil); got != 0 {
			t.Errorf("SnapshotEndpointCount(nil) = %d, want 0", got)
		}
	})
	t.Run("with endpoints", func(t *testing.T) {
		snap := &ir.Snapshot{
			Backends: []ir.BackendCluster{
				{Endpoints: make([]ir.BackendEndpoint, 3)},
				{Endpoints: make([]ir.BackendEndpoint, 0)},
				{Endpoints: make([]ir.BackendEndpoint, 5)},
			},
		}
		if got := SnapshotEndpointCount(snap); got != 8 {
			t.Errorf("SnapshotEndpointCount() = %d, want 8", got)
		}
	})
}

func TestPositiveIntOrZero(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{5, 5},
		{1, 1},
		{0, 0},
		{-1, 0},
		{-100, 0},
	}
	for _, tt := range tests {
		if got := PositiveIntOrZero(tt.input); got != tt.want {
			t.Errorf("PositiveIntOrZero(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestDefaultDurationIfZero(t *testing.T) {
	def := 5 * time.Second
	tests := []struct {
		name  string
		value time.Duration
		want  time.Duration
	}{
		{"uses default", 0, def},
		{"uses value", 10 * time.Second, 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultDurationIfZero(tt.value, def); got != tt.want {
				t.Errorf("DefaultDurationIfZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeLimits(t *testing.T) {
	limits := Limits{
		MaxInputObjects:       -5,
		MaxSnapshotObjects:    100,
		MaxSnapshotEndpoints:  0,
		DefaultConnectTimeout: 0,
	}
	normalized := NormalizeLimits(limits)
	if normalized.MaxInputObjects != 0 {
		t.Errorf("MaxInputObjects = %d, want 0 (negative → zero)", normalized.MaxInputObjects)
	}
	if normalized.MaxSnapshotObjects != 100 {
		t.Errorf("MaxSnapshotObjects = %d, want 100", normalized.MaxSnapshotObjects)
	}
	if normalized.DefaultConnectTimeout != DefaultConnectTimeout {
		t.Errorf("DefaultConnectTimeout = %v, want %v", normalized.DefaultConnectTimeout, DefaultConnectTimeout)
	}
}

func TestParseGatewayDuration(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := ParseGatewayDuration(nil); got != nil {
			t.Errorf("ParseGatewayDuration(nil) = %v, want nil", got)
		}
	})
	t.Run("valid", func(t *testing.T) {
		d := gatewayv1.Duration("30s")
		got := ParseGatewayDuration(&d)
		if got == nil {
			t.Fatal("ParseGatewayDuration returned nil")
		}
		if *got != 30*time.Second {
			t.Errorf("ParseGatewayDuration() = %v, want 30s", *got)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		d := gatewayv1.Duration("not-a-duration")
		if got := ParseGatewayDuration(&d); got != nil {
			t.Errorf("ParseGatewayDuration(invalid) = %v, want nil", got)
		}
	})
}

func TestSelectSlicePortUsesStableFallback(t *testing.T) {
	httpName := "http"
	grpcName := "grpc"
	httpPort := int32(8080)
	grpcPort := int32(9000)
	ports := []discoveryv1.EndpointPort{
		{Name: &grpcName, Port: &grpcPort},
		{Name: &httpName, Port: &httpPort},
	}

	got := SelectSlicePort(ports, "missing", 1234)
	if got == nil || got.Port == nil || *got.Port != grpcPort {
		t.Fatalf("SelectSlicePort fallback port = %#v, want grpc/9000", got)
	}

	ports[0], ports[1] = ports[1], ports[0]
	got = SelectSlicePort(ports, "missing", 1234)
	if got == nil || got.Port == nil || *got.Port != grpcPort {
		t.Fatalf("SelectSlicePort reordered fallback port = %#v, want grpc/9000", got)
	}
}

func TestNewTranslatorIndexes(t *testing.T) {
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "svc"},
	}
	slices := []discoveryv1.EndpointSlice{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es1", Labels: map[string]string{discoveryv1.LabelServiceName: "svc"}},
		},
	}
	grants := []gatewayv1beta1.ReferenceGrant{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "rg1"}},
	}

	idx := NewTranslatorIndexes(
		[]corev1.Service{svc},
		[]mcsv1alpha1.ServiceImport{},
		slices,
		[]corev1.Secret{},
		[]corev1.ConfigMap{},
		grants,
	)

	t.Run("Service lookup", func(t *testing.T) {
		result, ok := idx.Service("ns", "svc")
		if !ok {
			t.Fatal("Service not found")
		}
		if result.Name != "svc" {
			t.Errorf("Service.Name = %s, want svc", result.Name)
		}
	})

	t.Run("Service not found", func(t *testing.T) {
		_, ok := idx.Service("ns", "missing")
		if ok {
			t.Error("Service should not be found")
		}
	})

	t.Run("EndpointSlices lookup", func(t *testing.T) {
		result := idx.ServiceEndpointSlices("ns", "svc")
		if len(result) != 1 {
			t.Errorf("ServiceEndpointSlices len = %d, want 1", len(result))
		}
	})

	t.Run("ReferenceGrants lookup", func(t *testing.T) {
		result := idx.ReferenceGrantsByNamespace["ns"]
		if len(result) != 1 {
			t.Errorf("ReferenceGrantsByNamespace len = %d, want 1", len(result))
		}
	})
}

func TestTLSSecret(t *testing.T) {
	validSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "tls-cert"},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("cert-data"),
			"tls.key": []byte("key-data"),
		},
	}
	idx := NewTranslatorIndexes(nil, nil, nil, []corev1.Secret{validSecret}, nil, nil)

	t.Run("valid TLS secret", func(t *testing.T) {
		s, ok := idx.TLSSecret("ns", "tls-cert")
		if !ok {
			t.Fatal("valid TLS secret not found")
		}
		if s.Name != "tls-cert" {
			t.Errorf("Name = %s, want tls-cert", s.Name)
		}
	})

	t.Run("missing tls.key", func(t *testing.T) {
		s := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "missing-key"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{"tls.crt": []byte("cert")},
		}
		idx2 := NewTranslatorIndexes(nil, nil, nil, []corev1.Secret{s}, nil, nil)
		_, ok := idx2.TLSSecret("ns", "missing-key")
		if ok {
			t.Error("secret with missing tls.key should not be valid")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		s := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "opaque"},
			Type:       corev1.SecretTypeOpaque,
		}
		idx2 := NewTranslatorIndexes(nil, nil, nil, []corev1.Secret{s}, nil, nil)
		_, ok := idx2.TLSSecret("ns", "opaque")
		if ok {
			t.Error("non-TLS secret should not be valid")
		}
	})
}
