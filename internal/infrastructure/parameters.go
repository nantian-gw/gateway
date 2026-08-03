package infrastructure

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	infrastructureParametersConfigMapKind = "ConfigMap"
	serviceParametersYAMLKey              = "service.yaml"
	serviceParametersYMLKey               = "service.yml"
	serviceParametersKey                  = "service"
)

var supportedServiceParameterKeys = []string{
	serviceParametersYAMLKey,
	serviceParametersYMLKey,
	serviceParametersKey,
}

type gatewayServiceParameters struct {
	Type                          corev1.ServiceType                   `yaml:"type"`
	ExternalTrafficPolicy         *corev1.ServiceExternalTrafficPolicy `yaml:"externalTrafficPolicy"`
	InternalTrafficPolicy         *corev1.ServiceInternalTrafficPolicy `yaml:"internalTrafficPolicy"`
	IPFamilyPolicy                *corev1.IPFamilyPolicy               `yaml:"ipFamilyPolicy"`
	IPFamilies                    []corev1.IPFamily                    `yaml:"ipFamilies"`
	SessionAffinity               *corev1.ServiceAffinity              `yaml:"sessionAffinity"`
	SessionAffinityConfig         *sessionAffinityConfigParameters     `yaml:"sessionAffinityConfig"`
	ExternalIPs                   []string                             `yaml:"externalIPs"`
	LoadBalancerIP                *string                              `yaml:"loadBalancerIP"`
	HealthCheckNodePort           *int32                               `yaml:"healthCheckNodePort"`
	LoadBalancerClass             *string                              `yaml:"loadBalancerClass"`
	LoadBalancerSourceRanges      []string                             `yaml:"loadBalancerSourceRanges"`
	AllocateLoadBalancerNodePorts *bool                                `yaml:"allocateLoadBalancerNodePorts"`
	PublishNotReadyAddresses      *bool                                `yaml:"publishNotReadyAddresses"`
}

type sessionAffinityConfigParameters struct {
	ClientIP *clientIPConfigParameters `yaml:"clientIP"`
}

type clientIPConfigParameters struct {
	TimeoutSeconds *int32 `yaml:"timeoutSeconds"`
}

type gatewayClassServiceParametersResult struct {
	params       gatewayServiceParameters
	ok           bool
	reference    string
	hasReference bool
}

type gatewayServiceParameterResolver struct {
	reconciler *Reconciler
	classCache map[string]gatewayClassServiceParametersResult
}

func newGatewayServiceParameterResolver(reconciler *Reconciler) *gatewayServiceParameterResolver {
	return &gatewayServiceParameterResolver{
		reconciler: reconciler,
		classCache: make(map[string]gatewayClassServiceParametersResult),
	}
}

func (r *gatewayServiceParameterResolver) resolve(
	ctx context.Context,
	gateway gatewayv1.Gateway,
) gatewayServiceParameters {
	if r == nil || r.reconciler == nil {
		return gatewayServiceParameters{}
	}

	params := gatewayServiceParameters{}

	if classParams := r.loadGatewayClassServiceParameters(ctx, gateway); classParams.ok {
		params = classParams.params
	}

	if gateway.Spec.Infrastructure == nil || gateway.Spec.Infrastructure.ParametersRef == nil {
		return params
	}

	override, err := loadGatewayServiceParameters(
		ctx,
		r.reconciler.client,
		gateway.Namespace,
		gateway.Spec.Infrastructure.ParametersRef,
	)
	if err != nil {
		r.reconciler.logger.Warn(
			"failed to resolve gateway infrastructure parameters",
			"gateway",
			gateway.Namespace+"/"+gateway.Name,
			"reference",
			string(gateway.Spec.Infrastructure.ParametersRef.Kind)+"/"+gateway.Spec.Infrastructure.ParametersRef.Name,
			"error",
			err,
		)
		return params
	}

	merged := mergeGatewayServiceParameters(params, override)
	if err := merged.validate(); err != nil {
		r.reconciler.logger.Warn(
			"failed to merge gateway infrastructure parameters",
			"gateway",
			gateway.Namespace+"/"+gateway.Name,
			"error",
			err,
		)
		return params
	}

	return merged
}

func (r *gatewayServiceParameterResolver) loadGatewayClassServiceParameters(
	ctx context.Context,
	gateway gatewayv1.Gateway,
) gatewayClassServiceParametersResult {
	if r == nil || r.reconciler == nil {
		return gatewayClassServiceParametersResult{}
	}

	className := string(gateway.Spec.GatewayClassName)
	if cached, ok := r.classCache[className]; ok {
		return cached
	}

	result := r.reconciler.loadGatewayClassServiceParametersUncached(ctx, gateway)
	r.classCache[className] = result
	return result
}

func (r *gatewayServiceParameterResolver) gatewayClassParametersReference(
	ctx context.Context,
	gateway gatewayv1.Gateway,
) string {
	result := r.loadGatewayClassServiceParameters(ctx, gateway)
	if !result.hasReference {
		return ""
	}
	return result.reference
}

func (r *Reconciler) loadGatewayClassServiceParametersUncached(
	ctx context.Context,
	gateway gatewayv1.Gateway,
) gatewayClassServiceParametersResult {
	gatewayClass := &gatewayv1.GatewayClass{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: string(gateway.Spec.GatewayClassName)}, gatewayClass); err != nil {
		r.logger.Warn(
			"failed to load gateway class for infrastructure parameters",
			"gateway",
			gateway.Namespace+"/"+gateway.Name,
			"gatewayClass",
			gateway.Spec.GatewayClassName,
			"error",
			err,
		)
		return gatewayClassServiceParametersResult{}
	}

	result := gatewayClassServiceParametersResult{
		reference:    GatewayClassParametersReference(gatewayClass),
		hasReference: gatewayClass.Spec.ParametersRef != nil,
	}
	if gatewayClass.Spec.ParametersRef == nil {
		return result
	}

	params, err := loadGatewayClassServiceParameters(ctx, r.client, gatewayClass)
	if err != nil {
		r.logger.Warn(
			"failed to resolve gateway class infrastructure parameters",
			"gateway",
			gateway.Namespace+"/"+gateway.Name,
			"gatewayClass",
			gatewayClass.Name,
			"reference",
			string(gatewayClass.Spec.ParametersRef.Kind)+"/"+gatewayClass.Spec.ParametersRef.Name,
			"error",
			err,
		)
		return result
	}
	result.params = params
	result.ok = true
	return result
}
