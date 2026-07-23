package main

import (
	"fmt"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/config"
	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	routepolicy "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
)

func buildScheme(cfg *config.Config) (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	registrations := []struct {
		name string
		fn   func(*runtime.Scheme) error
	}{
		{name: "client-go", fn: clientgoscheme.AddToScheme},
		{name: "core/v1", fn: corev1.AddToScheme},
		{name: "coordination/v1", fn: coordinationv1.AddToScheme},
		{name: "discovery/v1", fn: discoveryv1.AddToScheme},
		{name: "apiextensions/v1", fn: apiextensionsv1.AddToScheme},
		{name: "gateway/v1", fn: gatewayv1.Install},
		{name: "gateway/v1alpha2", fn: gatewayv1alpha2.Install},
		{name: "gateway/v1alpha3", fn: gatewayv1alpha3.Install},
		{name: "gateway/v1beta1", fn: gatewayv1beta1.Install},
		{name: "mcs/v1alpha1", fn: mcsv1alpha1.Install},
	}

	if cfg.Features.EnableExperimentalGateway {
		registrations = append(registrations,
			[]struct {
				name string
				fn   func(*runtime.Scheme) error
			}{
				{name: "gateway.experimental/v1alpha2", fn: backend.Install},
				{name: "routepolicy/v1alpha1", fn: routepolicy.AddToScheme},
				{name: "wasmplugin/v1alpha1", fn: wasmplugin.AddToScheme},
				{name: "tokenpolicy/v1alpha1", fn: tokenpolicy.AddToScheme},
			}...,
		)
	}

	if cfg.Features.EnableAiGateway {
		registrations = append(registrations,
			struct {
				name string
				fn   func(*runtime.Scheme) error
			}{name: "aiservice/v1alpha1", fn: aiservice.AddToScheme},
		)
	}

	for _, registration := range registrations {
		if err := registration.fn(scheme); err != nil {
			return nil, fmt.Errorf("register %s scheme: %w", registration.name, err)
		}
	}

	return scheme, nil
}
