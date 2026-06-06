package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	testGatewayClassControllerNameIndex = "nantian.dev/infrastructure.gatewayclass.controller-name"
	testGatewayGatewayClassNameIndex    = "nantian.dev/infrastructure.gateway.gatewayclass-name"
)

func newControllerClientBuilder(scheme *runtime.Scheme) *fake.ClientBuilder {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, testGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, testGatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok || gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		})
}
