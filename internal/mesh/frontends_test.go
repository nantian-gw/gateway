package mesh

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestListenerProtocolForServicePortUsesAppProtocolHints(t *testing.T) {
	appH2C := "kubernetes.io/h2c"
	appWS := "kubernetes.io/ws"
	appWSS := "kubernetes.io/wss"
	appGRPC := "grpc"

	tests := []struct {
		name string
		port corev1.ServicePort
		want string
	}{
		{
			name: "h2c maps to http listener",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, AppProtocol: &appH2C},
			want: "HTTP",
		},
		{
			name: "ws maps to http listener",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, AppProtocol: &appWS},
			want: "HTTP",
		},
		{
			name: "wss maps to tls passthrough listener",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, AppProtocol: &appWSS},
			want: "TLS_PASSTHROUGH",
		},
		{
			name: "grpc maps to grpc listener",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, AppProtocol: &appGRPC},
			want: "GRPC",
		},
		{
			name: "udp stays udp",
			port: corev1.ServicePort{Protocol: corev1.ProtocolUDP, AppProtocol: &appH2C},
			want: "UDP",
		},
		{
			name: "plain tcp without hints stays tcp",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, Name: "db", Port: 5432},
			want: "TCP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ListenerProtocolForServicePort(tt.port); got != tt.want {
				t.Fatalf("ListenerProtocolForServicePort() = %q, want %q", got, tt.want)
			}
		})
	}
}
