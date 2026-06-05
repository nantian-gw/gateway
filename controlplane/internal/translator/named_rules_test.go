package translator

import (
	"encoding/json"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestTranslateRoutesPreservesNamedRulesInIR(t *testing.T) {
	httpRuleName := gatewayv1.SectionName("http-primary")
	grpcRuleName := gatewayv1.SectionName("grpc-primary")
	tcpRuleName := gatewayv1.SectionName("tcp-primary")
	udpRuleName := gatewayv1.SectionName("udp-primary")
	tlsRuleName := gatewayv1.SectionName("tls-primary")

	cases := []struct {
		name string
		rule any
		want string
	}{
		{
			name: "HTTPRoute",
			rule: translateHTTPRoute(gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "http", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{{Name: &httpRuleName}},
				},
			}).Rules[0],
			want: string(httpRuleName),
		},
		{
			name: "GRPCRoute",
			rule: translateGRPCRoute(gatewayv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "grpc", Namespace: "default"},
				Spec: gatewayv1.GRPCRouteSpec{
					Rules: []gatewayv1.GRPCRouteRule{{Name: &grpcRuleName}},
				},
			}).Rules[0],
			want: string(grpcRuleName),
		},
		{
			name: "TCPRoute",
			rule: translateTCPRoute(gatewayv1alpha2.TCPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "tcp", Namespace: "default"},
				Spec: gatewayv1alpha2.TCPRouteSpec{
					Rules: []gatewayv1alpha2.TCPRouteRule{{Name: &tcpRuleName}},
				},
			}).Rules[0],
			want: string(tcpRuleName),
		},
		{
			name: "UDPRoute",
			rule: translateUDPRoute(gatewayv1alpha2.UDPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "udp", Namespace: "default"},
				Spec: gatewayv1alpha2.UDPRouteSpec{
					Rules: []gatewayv1alpha2.UDPRouteRule{{Name: &udpRuleName}},
				},
			}).Rules[0],
			want: string(udpRuleName),
		},
		{
			name: "TLSRoute",
			rule: translateTLSRoute(gatewayv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "tls", Namespace: "default"},
				Spec: gatewayv1alpha2.TLSRouteSpec{
					Rules: []gatewayv1alpha2.TLSRouteRule{{Name: &tlsRuleName}},
				},
			}).Rules[0],
			want: string(tlsRuleName),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(tc.rule)
			if err != nil {
				t.Fatalf("marshal translated rule: %v", err)
			}
			if !strings.Contains(string(payload), `"name":"`+tc.want+`"`) {
				t.Fatalf("translated rule JSON = %s, want rule name %q", payload, tc.want)
			}
		})
	}
}
