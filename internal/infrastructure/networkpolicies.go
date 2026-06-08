package infrastructure

import (
	"context"
	"reflect"
	"sort"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/mesh"
)

const defaultDataplaneNetworkPolicyName = "nantian-dataplane"

type networkPolicyPortKey struct {
	port     int32
	protocol corev1.Protocol
}

func desiredDataplaneNetworkPolicy(
	current *networkingv1.NetworkPolicy,
	gateways []gatewayv1.Gateway,
	meshPorts []networkPolicyPortKey,
	options Options,
) *networkingv1.NetworkPolicy {
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultDataplaneNetworkPolicyName,
			Namespace: options.DataplaneNamespace,
			Labels: map[string]string{
				managedByLabel: managedByValue,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: cloneStringMap(options.DataplaneSelector),
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		},
	}

	if current.Name != "" {
		desired.ResourceVersion = current.ResourceVersion
	}

	publicPorts := dataplaneListenerNetworkPolicyPorts(gateways, meshPorts)
	if len(publicPorts) > 0 {
		desired.Spec.Ingress = append(desired.Spec.Ingress, networkingv1.NetworkPolicyIngressRule{
			Ports: publicPorts,
		})
	}

	adminProtocol := corev1.ProtocolTCP
	desired.Spec.Ingress = append(desired.Spec.Ingress, networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": options.DataplaneNamespace,
					},
				},
			},
		},
		Ports: []networkingv1.NetworkPolicyPort{{
			Protocol: &adminProtocol,
			Port:     intstrPointer(defaultAdminPort),
		}},
	})

	return desired
}

func dataplaneListenerNetworkPolicyPorts(
	gateways []gatewayv1.Gateway,
	meshPorts []networkPolicyPortKey,
) []networkingv1.NetworkPolicyPort {
	index := make(map[networkPolicyPortKey]struct{})
	ordered := make([]networkPolicyPortKey, 0)

	for _, gateway := range gateways {
		for _, listener := range gatewayapi.InfrastructureListeners(gateway) {
			protocol, ok := serviceProtocol(listener.Protocol)
			if !ok {
				continue
			}

			key := networkPolicyPortKey{
				port:     int32(listener.Port),
				protocol: protocol,
			}
			if _, exists := index[key]; exists {
				continue
			}
			index[key] = struct{}{}
			ordered = append(ordered, key)
		}
	}
	for _, key := range meshPorts {
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = struct{}{}
		ordered = append(ordered, key)
	}

	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].port != ordered[j].port {
			return ordered[i].port < ordered[j].port
		}
		return ordered[i].protocol < ordered[j].protocol
	})

	out := make([]networkingv1.NetworkPolicyPort, 0, len(ordered))
	for _, item := range ordered {
		protocol := item.protocol
		out = append(out, networkingv1.NetworkPolicyPort{
			Protocol: &protocol,
			Port:     intstrPointer(item.port),
		})
	}
	return out
}

func reconcileDataplaneNetworkPolicy(
	ctx context.Context,
	cl client.Client,
	gateways []gatewayv1.Gateway,
	options Options,
) error {
	current := &networkingv1.NetworkPolicy{}
	key := client.ObjectKey{
		Namespace: options.DataplaneNamespace,
		Name:      defaultDataplaneNetworkPolicyName,
	}
	if err := cl.Get(ctx, key, current); client.IgnoreNotFound(err) != nil {
		return err
	}

	meshPorts, err := loadMeshFrontendNetworkPolicyPorts(ctx, cl)
	if err != nil {
		return err
	}

	desired := desiredDataplaneNetworkPolicy(current, gateways, meshPorts, options)
	return applyNetworkPolicy(ctx, cl, current, desired)
}

func loadMeshFrontendNetworkPolicyPorts(
	ctx context.Context,
	cl client.Client,
) ([]networkPolicyPortKey, error) {
	var services corev1.ServiceList
	if err := cl.List(
		ctx,
		&services,
		client.MatchingLabels{
			managedByLabel:   managedByValue,
			serviceRoleLabel: serviceRoleMeshFrontend,
		},
	); err != nil {
		return nil, err
	}

	index := make(map[networkPolicyPortKey]struct{})
	ordered := make([]networkPolicyPortKey, 0)
	for _, service := range services.Items {
		if service.Annotations[mesh.ManagedServiceAnnotation] != "true" {
			continue
		}

		for _, port := range service.Spec.Ports {
			if port.TargetPort.Type != intstr.Int || port.TargetPort.IntValue() <= 0 {
				continue
			}

			key := networkPolicyPortKey{
				port:     int32(port.TargetPort.IntValue()),
				protocol: port.Protocol,
			}
			if _, exists := index[key]; exists {
				continue
			}
			index[key] = struct{}{}
			ordered = append(ordered, key)
		}
	}

	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].port != ordered[j].port {
			return ordered[i].port < ordered[j].port
		}
		return ordered[i].protocol < ordered[j].protocol
	})

	return ordered, nil
}

func applyNetworkPolicy(
	ctx context.Context,
	cl client.Client,
	current, desired *networkingv1.NetworkPolicy,
) error {
	if current.Name == "" {
		return cl.Create(ctx, desired)
	}
	if networkPolicyEqual(current, desired) {
		return nil
	}
	return cl.Update(ctx, desired)
}

func networkPolicyEqual(
	current, desired *networkingv1.NetworkPolicy,
) bool {
	if current.Name != desired.Name ||
		current.Namespace != desired.Namespace ||
		!stringMapEqual(current.Labels, desired.Labels) ||
		!stringMapEqual(current.Annotations, desired.Annotations) {
		return false
	}

	return reflect.DeepEqual(current.Spec, desired.Spec)
}

func intstrPointer(value int32) *intstr.IntOrString {
	out := intstr.FromInt(int(value))
	return &out
}
