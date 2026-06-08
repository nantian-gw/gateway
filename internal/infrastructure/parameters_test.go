package infrastructure

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestLoadGatewayClassServiceParameters(t *testing.T) {
	namespace := gatewayv1.Namespace("infra-system")

	tests := []struct {
		name                       string
		objects                    []client.Object
		gatewayClass               *gatewayv1.GatewayClass
		wantType                   corev1.ServiceType
		wantPublish                bool
		wantIPFamilyPolicy         *corev1.IPFamilyPolicyType
		wantIPFamilies             []corev1.IPFamily
		wantSessionAffinityTimeout *int32
		wantExternalIPs            []string
		wantLoadBalancerIP         string
		wantHealthCheckNodePort    *int32
		wantErr                    string
		wantNotFound               bool
	}{
		{
			name: "exists",
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "gatewayclass-defaults", Namespace: "infra-system"},
					Data: map[string]string{
						serviceParametersYAMLKey: "type: LoadBalancer\nexternalTrafficPolicy: Local\nsessionAffinity: ClientIP\nsessionAffinityConfig:\n  clientIP:\n    timeoutSeconds: 7200\npublishNotReadyAddresses: true\nipFamilyPolicy: PreferDualStack\nipFamilies:\n  - IPv6\n  - IPv4\nexternalIPs:\n  - 203.0.113.10\n  - 203.0.113.20\nloadBalancerIP: 203.0.113.10\nhealthCheckNodePort: 32080\n",
					},
				},
			},
			gatewayClass: &gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ParametersRef: &gatewayv1.ParametersReference{
						Kind:      gatewayv1.Kind("ConfigMap"),
						Name:      "gatewayclass-defaults",
						Namespace: &namespace,
					},
				},
			},
			wantType:                   corev1.ServiceTypeLoadBalancer,
			wantPublish:                true,
			wantIPFamilyPolicy:         ptrIPFamilyPolicy(corev1.IPFamilyPolicyPreferDualStack),
			wantIPFamilies:             []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			wantSessionAffinityTimeout: ptrInt32(7200),
			wantExternalIPs:            []string{"203.0.113.10", "203.0.113.20"},
			wantLoadBalancerIP:         "203.0.113.10",
			wantHealthCheckNodePort:    ptrInt32(32080),
		},
		{
			name:    "missing configmap",
			objects: []client.Object{},
			gatewayClass: &gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ParametersRef: &gatewayv1.ParametersReference{
						Kind:      gatewayv1.Kind("ConfigMap"),
						Name:      "gatewayclass-defaults",
						Namespace: &namespace,
					},
				},
			},
			wantNotFound: true,
		},
		{
			name:    "unsupported kind",
			objects: []client.Object{},
			gatewayClass: &gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ParametersRef: &gatewayv1.ParametersReference{
						Kind:      gatewayv1.Kind("Secret"),
						Name:      "gatewayclass-defaults",
						Namespace: &namespace,
					},
				},
			},
			wantErr: `unsupported gatewayClass parametersRef kind "Secret"`,
		},
		{
			name: "invalid content",
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "gatewayclass-defaults", Namespace: "infra-system"},
					Data: map[string]string{
						serviceParametersYAMLKey: "type: ExternalName\n",
					},
				},
			},
			gatewayClass: &gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ParametersRef: &gatewayv1.ParametersReference{
						Kind:      gatewayv1.Kind("ConfigMap"),
						Name:      "gatewayclass-defaults",
						Namespace: &namespace,
					},
				},
			},
			wantErr: `unsupported service type "ExternalName"`,
		},
		{
			name: "unknown field",
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "gatewayclass-defaults", Namespace: "infra-system"},
					Data: map[string]string{
						serviceParametersYAMLKey: "type: LoadBalancer\nunsupportedField: true\n",
					},
				},
			},
			gatewayClass: &gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ParametersRef: &gatewayv1.ParametersReference{
						Kind:      gatewayv1.Kind("ConfigMap"),
						Name:      "gatewayclass-defaults",
						Namespace: &namespace,
					},
				},
			},
			wantErr: `field unsupportedField not found`,
		},
		{
			name: "multiple parameter documents are rejected",
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "gatewayclass-defaults", Namespace: "infra-system"},
					Data: map[string]string{
						serviceParametersYAMLKey: "type: LoadBalancer\n",
						serviceParametersKey:     "type: ClusterIP\n",
					},
				},
			},
			gatewayClass: &gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ParametersRef: &gatewayv1.ParametersReference{
						Kind:      gatewayv1.Kind("ConfigMap"),
						Name:      "gatewayclass-defaults",
						Namespace: &namespace,
					},
				},
			},
			wantErr: `contains multiple supported service parameter keys`,
		},
		{
			name:    "missing namespace",
			objects: []client.Object{},
			gatewayClass: &gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ParametersRef: &gatewayv1.ParametersReference{
						Kind: gatewayv1.Kind("ConfigMap"),
						Name: "gatewayclass-defaults",
					},
				},
			},
			wantErr: "gatewayClass ConfigMap parametersRef namespace is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newScheme(t)
			builder := newInfrastructureClientBuilder(scheme)

			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...)
			}

			k8sClient := builder.Build()

			params, err := loadGatewayClassServiceParameters(context.Background(), k8sClient, tt.gatewayClass)
			if tt.wantNotFound {
				if !apierrors.IsNotFound(err) {
					t.Fatalf("expected NotFound error, got %v", err)
				}
				return
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadGatewayClassServiceParameters returned error: %v", err)
			}

			if params.Type != tt.wantType {
				t.Fatalf("type = %q, want %q", params.Type, tt.wantType)
			}
			if params.PublishNotReadyAddresses == nil || *params.PublishNotReadyAddresses != tt.wantPublish {
				t.Fatalf("publishNotReadyAddresses = %#v, want %v", params.PublishNotReadyAddresses, tt.wantPublish)
			}
			if tt.wantIPFamilyPolicy == nil {
				if params.IPFamilyPolicy != nil {
					t.Fatalf("ipFamilyPolicy = %#v, want nil", params.IPFamilyPolicy)
				}
			} else if params.IPFamilyPolicy == nil || *params.IPFamilyPolicy != *tt.wantIPFamilyPolicy {
				t.Fatalf("ipFamilyPolicy = %#v, want %q", params.IPFamilyPolicy, *tt.wantIPFamilyPolicy)
			}
			if strings.Join(ipFamiliesToStrings(params.IPFamilies), ",") != strings.Join(ipFamiliesToStrings(tt.wantIPFamilies), ",") {
				t.Fatalf("ipFamilies = %#v, want %#v", params.IPFamilies, tt.wantIPFamilies)
			}
			if tt.wantSessionAffinityTimeout == nil {
				if params.SessionAffinityConfig != nil {
					t.Fatalf("sessionAffinityConfig = %#v, want nil", params.SessionAffinityConfig)
				}
			} else if params.SessionAffinityConfig == nil || params.SessionAffinityConfig.ClientIP == nil ||
				params.SessionAffinityConfig.ClientIP.TimeoutSeconds == nil ||
				*params.SessionAffinityConfig.ClientIP.TimeoutSeconds != *tt.wantSessionAffinityTimeout {
				t.Fatalf("sessionAffinityConfig = %#v, want timeout %d", params.SessionAffinityConfig, *tt.wantSessionAffinityTimeout)
			}
			if !strings.EqualFold(strings.Join(params.ExternalIPs, ","), strings.Join(tt.wantExternalIPs, ",")) {
				t.Fatalf("externalIPs = %#v, want %#v", params.ExternalIPs, tt.wantExternalIPs)
			}
			if tt.wantLoadBalancerIP == "" {
				if params.LoadBalancerIP != nil {
					t.Fatalf("loadBalancerIP = %#v, want nil", params.LoadBalancerIP)
				}
			} else if params.LoadBalancerIP == nil || *params.LoadBalancerIP != tt.wantLoadBalancerIP {
				t.Fatalf("loadBalancerIP = %#v, want %q", params.LoadBalancerIP, tt.wantLoadBalancerIP)
			}
			if tt.wantHealthCheckNodePort == nil {
				if params.HealthCheckNodePort != nil {
					t.Fatalf("healthCheckNodePort = %#v, want nil", params.HealthCheckNodePort)
				}
			} else if params.HealthCheckNodePort == nil || *params.HealthCheckNodePort != *tt.wantHealthCheckNodePort {
				t.Fatalf("healthCheckNodePort = %#v, want %d", params.HealthCheckNodePort, *tt.wantHealthCheckNodePort)
			}
		})
	}
}

func ptrIPFamilyPolicy(value corev1.IPFamilyPolicyType) *corev1.IPFamilyPolicyType {
	return &value
}

func ptrInt32(value int32) *int32 {
	return &value
}

func ipFamiliesToStrings(items []corev1.IPFamily) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item))
	}
	return out
}
