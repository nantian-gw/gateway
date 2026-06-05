package infrastructure

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("add gateway scheme: %v", err)
	}
	if err := gatewayv1alpha2.Install(scheme); err != nil {
		t.Fatalf("add gateway alpha2 scheme: %v", err)
	}
	if err := discoveryv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add discovery scheme: %v", err)
	}
	if err := networkingv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add networking scheme: %v", err)
	}
	return scheme
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type validatingClient struct {
	client.Client
	listValidators map[reflect.Type]func(client.ListOptions) error
}

type countingGetClient struct {
	client.Client
	onGet func(client.ObjectKey, client.Object)
}

func (c validatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if validator, ok := c.listValidators[reflect.TypeOf(list)]; ok {
		var listOptions client.ListOptions
		for _, opt := range opts {
			opt.ApplyToList(&listOptions)
		}
		if err := validator(listOptions); err != nil {
			return fmt.Errorf("unexpected List for %T: %w", list, err)
		}
	}

	return c.Client.List(ctx, list, opts...)
}

func (c countingGetClient) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption,
) error {
	if c.onGet != nil {
		c.onGet(key, obj)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func assertServicePort(
	t *testing.T,
	ports []corev1.ServicePort,
	port int32,
	protocol corev1.Protocol,
	nodePort int32,
) {
	t.Helper()

	for _, item := range ports {
		if item.Port != port || item.Protocol != protocol {
			continue
		}
		if item.NodePort != nodePort {
			t.Fatalf("service port %d/%s nodePort = %d, want %d", port, protocol, item.NodePort, nodePort)
		}
		if item.TargetPort != intstr.FromInt(int(port)) && port != defaultAdminPort {
			t.Fatalf("service port %d/%s targetPort = %#v", port, protocol, item.TargetPort)
		}
		return
	}

	t.Fatalf("service port %d/%s not found in %#v", port, protocol, ports)
}

func assertMissingServicePort(
	t *testing.T,
	ports []corev1.ServicePort,
	port int32,
	protocol corev1.Protocol,
) {
	t.Helper()

	for _, item := range ports {
		if item.Port == port && item.Protocol == protocol {
			t.Fatalf("service port %d/%s should be absent, got %#v", port, protocol, item)
		}
	}
}

func assertNetworkPolicyPort(
	t *testing.T,
	ingress []networkingv1.NetworkPolicyIngressRule,
	port int32,
	protocol corev1.Protocol,
) {
	t.Helper()

	for _, rule := range ingress {
		for _, item := range rule.Ports {
			if item.Port == nil || item.Protocol == nil {
				continue
			}
			if item.Port.IntValue() == int(port) && *item.Protocol == protocol {
				return
			}
		}
	}

	t.Fatalf("network policy port %d/%s not found in %#v", port, protocol, ingress)
}

func assertMissingNetworkPolicyPort(
	t *testing.T,
	ingress []networkingv1.NetworkPolicyIngressRule,
	port int32,
	protocol corev1.Protocol,
) {
	t.Helper()

	for _, rule := range ingress {
		for _, item := range rule.Ports {
			if item.Port == nil || item.Protocol == nil {
				continue
			}
			if item.Port.IntValue() == int(port) && *item.Protocol == protocol {
				t.Fatalf("network policy port %d/%s should be absent, got %#v", port, protocol, item)
			}
		}
	}
}

func assertAdminNetworkPolicyRule(
	t *testing.T,
	ingress []networkingv1.NetworkPolicyIngressRule,
	namespace string,
) {
	t.Helper()

	for _, rule := range ingress {
		if len(rule.From) != 1 || len(rule.Ports) != 1 {
			continue
		}
		peer := rule.From[0]
		port := rule.Ports[0]
		if peer.NamespaceSelector == nil || port.Port == nil || port.Protocol == nil {
			continue
		}
		if peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != namespace {
			continue
		}
		if port.Port.IntValue() != defaultAdminPort || *port.Protocol != corev1.ProtocolTCP {
			continue
		}
		return
	}

	t.Fatalf("admin ingress rule for namespace %q not found in %#v", namespace, ingress)
}
