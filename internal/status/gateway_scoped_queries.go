package status

import (
	"context"
	"sort"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	statusGatewayClassControllerNameIndex = "nantian.dev/infrastructure.gatewayclass.controller-name"
	statusGatewayGatewayClassNameIndex    = "nantian.dev/infrastructure.gateway.gatewayclass-name"
)

func listGatewayClassesForController(
	ctx context.Context,
	reader client.Reader,
	controllerName string,
) ([]gatewayv1.GatewayClass, error) {
	var gatewayClasses gatewayv1.GatewayClassList
	if err := reader.List(
		ctx,
		&gatewayClasses,
		client.MatchingFields{statusGatewayClassControllerNameIndex: controllerName},
	); err != nil {
		if !isMissingFieldIndexError(err) {
			return nil, err
		}
		gatewayClasses = gatewayv1.GatewayClassList{}
		if err := reader.List(ctx, &gatewayClasses); err != nil {
			return nil, err
		}
	}

	out := make([]gatewayv1.GatewayClass, 0, len(gatewayClasses.Items))
	for _, gatewayClass := range gatewayClasses.Items {
		if string(gatewayClass.Spec.ControllerName) != controllerName {
			continue
		}
		out = append(out, gatewayClass)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out, nil
}

func listGatewaysForGatewayClass(
	ctx context.Context,
	reader client.Reader,
	gatewayClassName string,
) ([]gatewayv1.Gateway, error) {
	var gateways gatewayv1.GatewayList
	if err := reader.List(
		ctx,
		&gateways,
		client.MatchingFields{statusGatewayGatewayClassNameIndex: gatewayClassName},
	); err != nil {
		if !isMissingFieldIndexError(err) {
			return nil, err
		}
		gateways = gatewayv1.GatewayList{}
		if err := reader.List(ctx, &gateways); err != nil {
			return nil, err
		}
	}

	out := make([]gatewayv1.Gateway, 0, len(gateways.Items))
	for _, gateway := range gateways.Items {
		if string(gateway.Spec.GatewayClassName) != gatewayClassName {
			continue
		}
		out = append(out, gateway)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})

	return out, nil
}

func isMissingFieldIndexError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "no index with name") ||
		(strings.Contains(message, "specifies selector on field") && strings.Contains(message, "has been registered")) ||
		strings.Contains(message, "field label not supported")
}
