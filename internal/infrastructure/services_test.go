package infrastructure

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestSharedNodePortForPrivilegedTCP(t *testing.T) {
	opts := Options{
		NodePortBasePrivileged: 30000,
		NodePortBaseUDP:        31000,
		NodePortBaseDefault:    32000,
		NodePortRangeMax:       32767,
	}

	tests := []struct {
		name     string
		port     int32
		protocol corev1.Protocol
		want     int32
	}{
		{
			name:     "privileged port 80",
			port:     80,
			protocol: corev1.ProtocolTCP,
			want:     30080,
		},
		{
			name:     "privileged port 443",
			port:     443,
			protocol: corev1.ProtocolTCP,
			want:     30443,
		},
		{
			name:     "port 1023 (last privileged)",
			port:     1023,
			protocol: corev1.ProtocolTCP,
			want:     31023,
		},
		{
			name:     "port 1024 falls through to default range",
			port:     1024,
			protocol: corev1.ProtocolTCP,
			want:     32024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sharedNodePortFor(tt.port, tt.protocol, opts)
			if got != tt.want {
				t.Errorf("sharedNodePortFor(%d, %s) = %d, want %d", tt.port, tt.protocol, got, tt.want)
			}
		})
	}
}

func TestSharedNodePortForDefaultRange(t *testing.T) {
	opts := Options{
		NodePortBasePrivileged: 30000,
		NodePortBaseUDP:        31000,
		NodePortBaseDefault:    32000,
		NodePortRangeMax:       32767,
	}

	tests := []struct {
		name string
		port int32
		want int32
	}{
		{
			name: "port 8080",
			port: 8080,
			want: 32080,
		},
		{
			name: "port 9000 (mod 1000 = 0)",
			port: 9000,
			want: 32000,
		},
		{
			name: "port 5300",
			port: 5300,
			want: 32300,
		},
		{
			name: "port 30000",
			port: 30000,
			want: 32000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sharedNodePortFor(tt.port, corev1.ProtocolTCP, opts)
			if got != tt.want {
				t.Errorf("sharedNodePortFor(%d) = %d, want %d", tt.port, got, tt.want)
			}
		})
	}
}

func TestSharedNodePortForRangeMaxClamping(t *testing.T) {
	opts := Options{
		NodePortBasePrivileged: 30000,
		NodePortBaseUDP:        31000,
		NodePortBaseDefault:    32000,
		NodePortRangeMax:       32767,
	}

	// port 10768: 10768 % 1000 = 768, 32000 + 768 = 32768 > 32767 → 31768
	got := sharedNodePortFor(10768, corev1.ProtocolTCP, opts)
	want := int32(31768)
	if got != want {
		t.Errorf("sharedNodePortFor(10768) = %d, want %d (clamped)", got, want)
	}

	// port 700: 32000 + 700 = 32700 ≤ 32767, no clamping needed
	got = sharedNodePortFor(10700, corev1.ProtocolTCP, opts)
	want = int32(32700)
	if got != want {
		t.Errorf("sharedNodePortFor(10700) = %d, want %d (not clamped)", got, want)
	}
}

func TestSharedNodePortForUDP(t *testing.T) {
	opts := Options{
		NodePortBasePrivileged: 30000,
		NodePortBaseUDP:        31000,
		NodePortBaseDefault:    32000,
		NodePortRangeMax:       32767,
	}

	tests := []struct {
		name string
		port int32
		want int32
	}{
		{
			name: "UDP port 5353",
			port: 5353,
			want: 31353, // 31000 + (5353 % 1000) = 31000 + 353
		},
		{
			name: "UDP port 53",
			port: 53,
			want: 31053, // 31000 + (53 % 1000) = 31000 + 53
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sharedNodePortFor(tt.port, corev1.ProtocolUDP, opts)
			if got != tt.want {
				t.Errorf("sharedNodePortFor(%d, UDP) = %d, want %d", tt.port, got, tt.want)
			}
		})
	}
}

func TestAssignSharedNodePorts(t *testing.T) {
	opts := Options{
		NodePortBasePrivileged: 30000,
		NodePortBaseUDP:        31000,
		NodePortBaseDefault:    32000,
		NodePortRangeMax:       32767,
	}

	t.Run("assigns ports to all zero-NodePort entries", func(t *testing.T) {
		ports := []corev1.ServicePort{
			{Port: 80, Protocol: corev1.ProtocolTCP, NodePort: 0},
			{Port: 8080, Protocol: corev1.ProtocolTCP, NodePort: 0},
			{Port: 5353, Protocol: corev1.ProtocolUDP, NodePort: 0},
		}

		result := assignSharedNodePorts(ports, opts)

		if result[0].NodePort != 30080 {
			t.Errorf("port 80/TCP NodePort = %d, want 30080", result[0].NodePort)
		}
		if result[1].NodePort != 32080 {
			t.Errorf("port 8080/TCP NodePort = %d, want 32080", result[1].NodePort)
		}
		if result[2].NodePort != 31353 {
			t.Errorf("port 5353/UDP NodePort = %d, want 31353", result[2].NodePort)
		}
	})

	t.Run("preserves already-assigned NodePorts", func(t *testing.T) {
		ports := []corev1.ServicePort{
			{Port: 80, Protocol: corev1.ProtocolTCP, NodePort: 30123},
			{Port: 443, Protocol: corev1.ProtocolTCP, NodePort: 0},
		}

		result := assignSharedNodePorts(ports, opts)

		// Preserved
		if result[0].NodePort != 30123 {
			t.Errorf("preserved port 80/TCP NodePort = %d, want 30123", result[0].NodePort)
		}
		// Assigned
		if result[1].NodePort != 30443 {
			t.Errorf("port 443/TCP NodePort = %d, want 30443", result[1].NodePort)
		}
	})

	t.Run("returns empty slice unchanged", func(t *testing.T) {
		result := assignSharedNodePorts(nil, opts)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})
}
