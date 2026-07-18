package policies

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	gatewayClassControllerNameIndex = "nantian.dev/infrastructure.gatewayclass.controller-name"
	gatewayGatewayClassNameIndex    = "nantian.dev/infrastructure.gateway.gatewayclass-name"
)

func ListGatewayClassesForController(
	ctx context.Context,
	cl client.Client,
	controllerName string,
) ([]gatewayv1.GatewayClass, error) {
	var gatewayClasses gatewayv1.GatewayClassList
	if err := cl.List(
		ctx,
		&gatewayClasses,
		client.MatchingFields{gatewayClassControllerNameIndex: controllerName},
	); err != nil {
		if IsMissingFieldIndexError(err) {
			return nil, requiredFieldIndexError("GatewayClass", gatewayClassControllerNameIndex, err)
		}
		return nil, err
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

func ListGatewaysForGatewayClass(
	ctx context.Context,
	cl client.Client,
	gatewayClassName string,
) ([]gatewayv1.Gateway, error) {
	var gateways gatewayv1.GatewayList
	if err := cl.List(
		ctx,
		&gateways,
		client.MatchingFields{gatewayGatewayClassNameIndex: gatewayClassName},
	); err != nil {
		if IsMissingFieldIndexError(err) {
			return nil, requiredFieldIndexError("Gateway", gatewayGatewayClassNameIndex, err)
		}
		return nil, err
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

func IsMissingFieldIndexError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "no index with name") ||
		(strings.Contains(message, "specifies selector on field") && strings.Contains(message, "has been registered")) ||
		strings.Contains(message, "field label not supported")
}

func requiredFieldIndexError(kind, field string, err error) error {
	return fmt.Errorf("%s query requires field index %q: %w", kind, field, err)
}
