package infrastructure

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type rawValidatingClient struct {
	client.Client
	listValidators map[reflect.Type]func([]client.ListOption) error
}

func (c rawValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if validator, ok := c.listValidators[reflect.TypeOf(list)]; ok {
		if err := validator(opts); err != nil {
			return fmt.Errorf("unexpected List for %T: %w", list, err)
		}
	}

	return c.Client.List(ctx, list, opts...)
}

func withInfrastructureRouteParentIndexes(builder *fake.ClientBuilder) *fake.ClientBuilder {
	return builder.
		WithIndex(&gatewayv1.HTTPRoute{}, httpRouteServiceParentIndex, httpRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, grpcRouteServiceParentIndex, grpcRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, tcpRouteServiceParentIndex, tcpRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, udpRouteServiceParentIndex, udpRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, tlsRouteServiceParentIndex, tlsRouteServiceParentIndexKeys)
}

func withInfrastructureGatewayIndexes(builder *fake.ClientBuilder) *fake.ClientBuilder {
	return builder.
		WithIndex(&gatewayv1.GatewayClass{}, gatewayClassControllerNameIndex, gatewayClassControllerNameIndexKeys).
		WithIndex(&gatewayv1.Gateway{}, gatewayGatewayClassNameIndex, gatewayGatewayClassNameIndexKeys)
}

func newInfrastructureClientBuilder(scheme *runtime.Scheme) *fake.ClientBuilder {
	return withInfrastructureGatewayIndexes(
		withInfrastructureRouteParentIndexes(
			fake.NewClientBuilder().WithScheme(scheme),
		),
	)
}

func requireMatchingField(field string, value string) func([]client.ListOption) error {
	return func(opts []client.ListOption) error {
		for _, opt := range opts {
			matching, ok := opt.(client.MatchingFields)
			if !ok {
				continue
			}
			if matching[field] == value {
				return nil
			}
		}
		return fmt.Errorf("list must include matching field %s=%s", field, value)
	}
}
