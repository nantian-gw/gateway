package admin

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
)

type resourceKindSpec struct {
	descriptor ResourceKindDescriptor
	aliases    []string
	newObject  func() client.Object
	newList    func() client.ObjectList
	namespaced bool
}

const clusterScopeNamespaceMarker = "_cluster"

// Common resource kind strings extracted to constants to satisfy goconst lint.
const (
	kindGateway           = "Gateway"
	kindGatewayClass      = "GatewayClass"
	kindHTTPRoute         = "HTTPRoute"
	kindGRPCRoute         = "GRPCRoute"
	kindTCPRoute          = "TCPRoute"
	kindUDPRoute          = "UDPRoute"
	kindTLSRoute          = "TLSRoute"
	kindBackendLBPolicy   = "BackendLBPolicy"
	kindBackendTLSPolicy  = "BackendTLSPolicy"
	kindReferenceGrant    = "ReferenceGrant"
	kindServiceImport     = "ServiceImport"
	kindAIService         = "AIService"
	kindTokenPolicy       = "TokenPolicy"
	kindWasmPlugin        = "WasmPlugin"
	apiVersionV1          = "gateway.networking.k8s.io/v1"
	apiVersionV1Alpha2    = "gateway.networking.k8s.io/v1alpha2"
	apiVersionV1Beta1     = "gateway.networking.k8s.io/v1beta1"
	apiVersionNantianV1   = "gateway.nantian.dev/v1alpha1"
	apiVersionMCSV1Alpha1 = "multicluster.x-k8s.io/v1alpha1"
	categoryGateway       = "gateway"
	categoryRoute         = "route"
	categoryPolicy        = "policy"
	categoryBackend       = "backend"
	categoryAI            = "ai"
	categoryWasm          = "wasm"
)
var supportedResourceKinds = []resourceKindSpec{
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindGateway,
			APIVersion:  apiVersionV1,
			Category:    "gateway",
			Description: "Kubernetes Gateway API Gateway resource",
			Namespaced:  true,
		},
		aliases:    []string{"gateway", "gateways"},
		newObject:  func() client.Object { return &gatewayv1.Gateway{} },
		newList:    func() client.ObjectList { return &gatewayv1.GatewayList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindGatewayClass,
			APIVersion:  apiVersionV1,
			Category:    "gateway",
			Description: "Kubernetes Gateway API GatewayClass resource",
			Namespaced:  false,
		},
		aliases:    []string{"gatewayclass", "gatewayclasses", "gc"},
		newObject:  func() client.Object { return &gatewayv1.GatewayClass{} },
		newList:    func() client.ObjectList { return &gatewayv1.GatewayClassList{} },
		namespaced: false,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindHTTPRoute,
			APIVersion:  apiVersionV1,
			Category:    "route",
			Description: "Kubernetes Gateway API HTTPRoute resource",
			Namespaced:  true,
		},
		aliases:    []string{"httproute", "httproutes", "http"},
		newObject:  func() client.Object { return &gatewayv1.HTTPRoute{} },
		newList:    func() client.ObjectList { return &gatewayv1.HTTPRouteList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindGRPCRoute,
			APIVersion:  apiVersionV1,
			Category:    "route",
			Description: "Kubernetes Gateway API GRPCRoute resource",
			Namespaced:  true,
		},
		aliases:    []string{"grpcroute", "grpcroutes", "grpc"},
		newObject:  func() client.Object { return &gatewayv1.GRPCRoute{} },
		newList:    func() client.ObjectList { return &gatewayv1.GRPCRouteList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindTCPRoute,
			APIVersion:  apiVersionV1Alpha2,
			Category:    "route",
			Description: "Kubernetes Gateway API TCPRoute resource",
			Namespaced:  true,
		},
		aliases:    []string{"tcproute", "tcproutes", "tcp"},
		newObject:  func() client.Object { return &gatewayv1alpha2.TCPRoute{} },
		newList:    func() client.ObjectList { return &gatewayv1alpha2.TCPRouteList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindUDPRoute,
			APIVersion:  apiVersionV1Alpha2,
			Category:    "route",
			Description: "Kubernetes Gateway API UDPRoute resource",
			Namespaced:  true,
		},
		aliases:    []string{"udproute", "udproutes", "udp"},
		newObject:  func() client.Object { return &gatewayv1alpha2.UDPRoute{} },
		newList:    func() client.ObjectList { return &gatewayv1alpha2.UDPRouteList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindTLSRoute,
			APIVersion:  apiVersionV1Alpha2,
			Category:    "route",
			Description: "Kubernetes Gateway API TLSRoute resource",
			Namespaced:  true,
		},
		aliases:    []string{"tlsroute", "tlsroutes", "tls"},
		newObject:  func() client.Object { return &gatewayv1alpha2.TLSRoute{} },
		newList:    func() client.ObjectList { return &gatewayv1alpha2.TLSRouteList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindBackendLBPolicy,
			APIVersion:  apiVersionV1Alpha2,
			Category:    "policy",
			Description: "Kubernetes Gateway API BackendLBPolicy resource",
			Namespaced:  true,
		},
		aliases:    []string{"backendlbpolicy", "backendlbpolicies", "blbpolicy"},
		newObject:  func() client.Object { return &backend.BackendLBPolicy{} },
		newList:    func() client.ObjectList { return &backend.BackendLBPolicyList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindBackendTLSPolicy,
			APIVersion:  apiVersionV1,
			Category:    "policy",
			Description: "Kubernetes Gateway API BackendTLSPolicy resource",
			Namespaced:  true,
		},
		aliases:    []string{"backendtlspolicy", "backendtlspolicies", "btlspolicy"},
		newObject:  func() client.Object { return gatewayapi.NewBackendTLSPolicyV1Object() },
		newList:    func() client.ObjectList { return gatewayapi.NewBackendTLSPolicyV1List() },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindReferenceGrant,
			APIVersion:  apiVersionV1Beta1,
			Category:    "policy",
			Description: "Kubernetes Gateway API ReferenceGrant resource",
			Namespaced:  true,
		},
		aliases:    []string{"referencegrant", "referencegrants", "refgrant"},
		newObject:  func() client.Object { return &gatewayv1beta1.ReferenceGrant{} },
		newList:    func() client.ObjectList { return &gatewayv1beta1.ReferenceGrantList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindServiceImport,
			APIVersion:  apiVersionMCSV1Alpha1,
			Category:    "backend",
			Description: "MCS API ServiceImport resource used as a backend source",
			Namespaced:  true,
		},
		aliases:    []string{"serviceimport", "serviceimports", "svcimport"},
		newObject:  func() client.Object { return &mcsv1alpha1.ServiceImport{} },
		newList:    func() client.ObjectList { return &mcsv1alpha1.ServiceImportList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindAIService,
			APIVersion:  apiVersionNantianV1,
			Category:    "ai",
			Description: "Nantian Gateway AI Service configuration",
			Namespaced:  true,
		},
		aliases:    []string{"aiservice", "aiservices", "aisvc"},
		newObject:  func() client.Object { return &aiservice.AIService{} },
		newList:    func() client.ObjectList { return &aiservice.AIServiceList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindTokenPolicy,
			APIVersion:  apiVersionNantianV1,
			Category:    "ai",
			Description: "Nantian Gateway AI Token Policy configuration",
			Namespaced:  true,
		},
		aliases:    []string{"tokenpolicy", "tokenpolicies", "tokpolicy"},
		newObject:  func() client.Object { return &tokenpolicy.TokenPolicy{} },
		newList:    func() client.ObjectList { return &tokenpolicy.TokenPolicyList{} },
		namespaced: true,
	},
	{
		descriptor: ResourceKindDescriptor{
			Kind:        kindWasmPlugin,
			APIVersion:  apiVersionNantianV1,
			Category:    "wasm",
			Description: "Nantian Gateway Wasm Plugin configuration",
			Namespaced:  true,
		},
		aliases:    []string{"wasmplugin", "wasmplugins", "wasmp"},
		newObject:  func() client.Object { return &wasmplugin.WasmPlugin{} },
		newList:    func() client.ObjectList { return &wasmplugin.WasmPluginList{} },
		namespaced: true,
	},
}

func SupportedResourceKinds() []ResourceKindDescriptor {
	items := make([]ResourceKindDescriptor, 0, len(supportedResourceKinds))
	for _, spec := range supportedResourceKinds {
		descriptor := spec.descriptor
		descriptor.Available = true
		items = append(items, descriptor)
	}

	return items
}

func resourceKindSpecFor(raw string) (resourceKindSpec, error) {
	normalized := normalizeResourceKind(raw)
	for _, spec := range supportedResourceKinds {
		if normalizeResourceKind(spec.descriptor.Kind) == normalized {
			return spec, nil
		}
		for _, alias := range spec.aliases {
			if normalizeResourceKind(alias) == normalized {
				return spec, nil
			}
		}
	}

	return resourceKindSpec{}, errInvalidRequest(fmt.Sprintf("unsupported resource kind %q", raw))
}

func (s resourceKindSpec) gvk() schema.GroupVersionKind {
	groupVersion := strings.SplitN(s.descriptor.APIVersion, "/", 2)
	if len(groupVersion) != 2 {
		return schema.GroupVersionKind{Kind: s.descriptor.Kind}
	}

	return schema.GroupVersionKind{
		Group:   groupVersion[0],
		Version: groupVersion[1],
		Kind:    s.descriptor.Kind,
	}
}

func normalizeResourceKind(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.TrimSuffix(normalized, "s")
	return normalized
}

func IsSameResourceKind(left, right string) bool {
	return normalizeResourceKind(left) == normalizeResourceKind(right)
}

func normalizedResourceNamespace(spec resourceKindSpec, raw string) string {
	namespace := strings.TrimSpace(raw)
	if spec.namespaced {
		return namespace
	}
	if namespace == clusterScopeNamespaceMarker {
		return ""
	}
	return namespace
}
