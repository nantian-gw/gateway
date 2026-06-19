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
	aiservicev1alpha1 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/aiservicev1alpha1"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
	tokenpolicyv1alpha1 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/tokenpolicyv1alpha1"
	wasmpluginv1alpha1 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/wasmpluginv1alpha1"
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
		{name: "mcs/v1alpha1", fn: mcsv1alpha1.AddToScheme},
	}

	if cfg.Features.EnableExperimentalGateway {
		registrations = append(registrations,
			[]struct {
				name string
				fn   func(*runtime.Scheme) error
			}{
				{name: "gateway.experimental/v1alpha2", fn: backendlbv1alpha2.Install},
				{name: "wasmplugin/v1alpha1", fn: wasmpluginv1alpha1.AddToScheme},
				{name: "tokenpolicy/v1alpha1", fn: tokenpolicyv1alpha1.AddToScheme},
			}...,
		)
	}

	if cfg.Features.EnableAiGateway {
		registrations = append(registrations,
			struct {
				name string
				fn   func(*runtime.Scheme) error
			}{name: "aiservice/v1alpha1", fn: aiservicev1alpha1.AddToScheme},
		)
	}

	for _, registration := range registrations {
		if err := registration.fn(scheme); err != nil {
			return nil, fmt.Errorf("register %s scheme: %w", registration.name, err)
		}
	}

	return scheme, nil
}
