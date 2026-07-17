package gatewayapi

import (
	"reflect"
	"sort"
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfeatures "sigs.k8s.io/gateway-api/pkg/features"
)

func TestSupportedFeatureNamesAreSortedAndComplete(t *testing.T) {
	got := SupportedFeatureNames()
	if !sort.SliceIsSorted(got, func(i, j int) bool {
		return got[i] < got[j]
	}) {
		t.Fatalf("supported feature names are not sorted: %#v", got)
	}

	want := sortedFeatureNames([]gatewayfeatures.FeatureName{
		SupportedBackendLBSessionPersistence,
		gatewayfeatures.SupportGRPCRoute,
		gatewayfeatures.SupportGRPCRouteNamedRouteRule,
		gatewayfeatures.SupportGateway,
		gatewayfeatures.SupportGatewayAddressEmpty,
		gatewayfeatures.SupportGatewayBackendClientCertificate,
		gatewayfeatures.SupportGatewayHTTPSListenerDetectMisdirectedRequests,
		gatewayfeatures.SupportGatewayInfrastructurePropagation,
		gatewayfeatures.SupportGatewayPort8080,
		gatewayfeatures.SupportGatewayStaticAddresses,
		gatewayfeatures.SupportHTTPRoute,
		gatewayfeatures.SupportHTTPRoute303RedirectStatusCode,
		gatewayfeatures.SupportHTTPRoute307RedirectStatusCode,
		gatewayfeatures.SupportHTTPRoute308RedirectStatusCode,
		gatewayfeatures.SupportHTTPRouteBackendProtocolH2C,
		gatewayfeatures.SupportHTTPRouteBackendProtocolWebSocket,
		gatewayfeatures.SupportHTTPRouteBackendRequestHeaderModification,
		gatewayfeatures.SupportHTTPRouteBackendTimeout,
		gatewayfeatures.SupportHTTPRouteCORS,
		gatewayfeatures.SupportHTTPRouteDestinationPortMatching,
		gatewayfeatures.SupportHTTPRouteHostRewrite,
		gatewayfeatures.SupportHTTPRouteMethodMatching,
		gatewayfeatures.SupportHTTPRouteNamedRouteRule,
		gatewayfeatures.SupportHTTPRouteParentRefPort,
		gatewayfeatures.SupportHTTPRoutePathRedirect,
		gatewayfeatures.SupportHTTPRoutePathRewrite,
		gatewayfeatures.SupportHTTPRouteQueryParamMatching,
		gatewayfeatures.SupportHTTPRouteRequestMirror,
		gatewayfeatures.SupportHTTPRouteRequestMultipleMirrors,
		gatewayfeatures.SupportHTTPRouteRequestPercentageMirror,
		gatewayfeatures.SupportHTTPRouteRequestTimeout,
		gatewayfeatures.SupportHTTPRouteResponseHeaderModification,
		gatewayfeatures.SupportMesh,
		gatewayfeatures.SupportMeshClusterIPMatching,
		gatewayfeatures.SupportMeshConsumerRoute,
		gatewayfeatures.SupportMeshHTTPRouteBackendRequestHeaderModification,
		gatewayfeatures.SupportMeshHTTPRouteNamedRouteRule,
		gatewayfeatures.SupportMeshHTTPRouteQueryParamMatching,
		gatewayfeatures.SupportMeshHTTPRouteRedirectPath,
		gatewayfeatures.SupportMeshHTTPRouteRedirectPort,
		gatewayfeatures.SupportMeshHTTPRouteRewritePath,
		gatewayfeatures.SupportMeshHTTPRouteSchemeRedirect,
		gatewayfeatures.SupportReferenceGrant,
		gatewayfeatures.SupportTLSRoute,
		gatewayfeatures.SupportTLSRouteModeMixed,
		gatewayfeatures.SupportTLSRouteModeTerminate,
		SupportedTCPRoute,
		gatewayfeatures.SupportUDPRoute,
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("supported feature names = %#v, want %#v", got, want)
	}
}

func TestSupportedFeatureNamesForOptionsExcludesExperimentalGatewayFeaturesWhenDisabled(t *testing.T) {
	got := SupportedFeatureNamesForOptions(FeatureOptions{EnableExperimentalGateway: false})
	names := featureNameSet(got)

	for _, name := range []gatewayfeatures.FeatureName{
		gatewayfeatures.SupportListenerSet,
		SupportedTCPRoute,
		gatewayfeatures.SupportUDPRoute,
		gatewayfeatures.SupportTLSRoute,
		gatewayfeatures.SupportTLSRouteModeTerminate,
		gatewayfeatures.SupportTLSRouteModeMixed,
	} {
		if names[name] {
			t.Fatalf("feature %s should not be advertised when experimental Gateway support is disabled: %#v", name, got)
		}
	}

	for _, name := range []gatewayfeatures.FeatureName{
		gatewayfeatures.SupportGateway,
		gatewayfeatures.SupportHTTPRoute,
		gatewayfeatures.SupportGRPCRoute,
		gatewayfeatures.SupportReferenceGrant,
	} {
		if !names[name] {
			t.Fatalf("feature %s should remain advertised when experimental Gateway support is disabled: %#v", name, got)
		}
	}
}

func TestSupportedFeatureNamesForOptionsExcludesUnsupportedFrontendClientCertificateValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options FeatureOptions
	}{
		{name: "default runtime", options: FeatureOptions{EnableExperimentalGateway: false}},
		{name: "experimental runtime", options: FeatureOptions{EnableExperimentalGateway: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SupportedFeatureNamesForOptions(tc.options)
			names := featureNameSet(got)

			for _, name := range []gatewayfeatures.FeatureName{
				gatewayfeatures.SupportGatewayFrontendClientCertificateValidation,
				gatewayfeatures.SupportGatewayFrontendClientCertificateValidationInsecureFallback,
			} {
				if names[name] {
					t.Fatalf("feature %s should not be advertised until strict frontend client certificate validation is supported: %#v", name, got)
				}
			}
		})
	}
}

func TestSupportedFeatureNamesForOptionsIncludesExperimentalGatewayFeaturesWhenEnabled(t *testing.T) {
	got := SupportedFeatureNamesForOptions(FeatureOptions{EnableExperimentalGateway: true})
	names := featureNameSet(got)

	for _, name := range []gatewayfeatures.FeatureName{
		SupportedTCPRoute,
		gatewayfeatures.SupportUDPRoute,
		gatewayfeatures.SupportTLSRoute,
		gatewayfeatures.SupportTLSRouteModeTerminate,
		gatewayfeatures.SupportTLSRouteModeMixed,
	} {
		if !names[name] {
			t.Fatalf("feature %s should be advertised when experimental Gateway support is enabled: %#v", name, got)
		}
	}

	want := sortedFeatureNames([]gatewayfeatures.FeatureName{
		SupportedBackendLBSessionPersistence,
		gatewayfeatures.SupportGRPCRoute,
		gatewayfeatures.SupportGRPCRouteNamedRouteRule,
		gatewayfeatures.SupportGateway,
		gatewayfeatures.SupportGatewayAddressEmpty,
		gatewayfeatures.SupportGatewayBackendClientCertificate,
		gatewayfeatures.SupportGatewayHTTPSListenerDetectMisdirectedRequests,
		gatewayfeatures.SupportGatewayInfrastructurePropagation,
		gatewayfeatures.SupportGatewayPort8080,
		gatewayfeatures.SupportGatewayStaticAddresses,
		gatewayfeatures.SupportHTTPRoute,
		gatewayfeatures.SupportHTTPRoute303RedirectStatusCode,
		gatewayfeatures.SupportHTTPRoute307RedirectStatusCode,
		gatewayfeatures.SupportHTTPRoute308RedirectStatusCode,
		gatewayfeatures.SupportHTTPRouteBackendProtocolH2C,
		gatewayfeatures.SupportHTTPRouteBackendProtocolWebSocket,
		gatewayfeatures.SupportHTTPRouteBackendRequestHeaderModification,
		gatewayfeatures.SupportHTTPRouteBackendTimeout,
		gatewayfeatures.SupportHTTPRouteCORS,
		gatewayfeatures.SupportHTTPRouteDestinationPortMatching,
		gatewayfeatures.SupportHTTPRouteHostRewrite,
		gatewayfeatures.SupportHTTPRouteMethodMatching,
		gatewayfeatures.SupportHTTPRouteNamedRouteRule,
		gatewayfeatures.SupportHTTPRouteParentRefPort,
		gatewayfeatures.SupportHTTPRoutePathRedirect,
		gatewayfeatures.SupportHTTPRoutePathRewrite,
		gatewayfeatures.SupportHTTPRouteQueryParamMatching,
		gatewayfeatures.SupportHTTPRouteRequestMirror,
		gatewayfeatures.SupportHTTPRouteRequestMultipleMirrors,
		gatewayfeatures.SupportHTTPRouteRequestPercentageMirror,
		gatewayfeatures.SupportHTTPRouteRequestTimeout,
		gatewayfeatures.SupportHTTPRouteResponseHeaderModification,
		gatewayfeatures.SupportMesh,
		gatewayfeatures.SupportMeshClusterIPMatching,
		gatewayfeatures.SupportMeshConsumerRoute,
		gatewayfeatures.SupportMeshHTTPRouteBackendRequestHeaderModification,
		gatewayfeatures.SupportMeshHTTPRouteNamedRouteRule,
		gatewayfeatures.SupportMeshHTTPRouteQueryParamMatching,
		gatewayfeatures.SupportMeshHTTPRouteRedirectPath,
		gatewayfeatures.SupportMeshHTTPRouteRedirectPort,
		gatewayfeatures.SupportMeshHTTPRouteRewritePath,
		gatewayfeatures.SupportMeshHTTPRouteSchemeRedirect,
		gatewayfeatures.SupportReferenceGrant,
		gatewayfeatures.SupportTLSRoute,
		gatewayfeatures.SupportTLSRouteModeMixed,
		gatewayfeatures.SupportTLSRouteModeTerminate,
		SupportedTCPRoute,
		gatewayfeatures.SupportUDPRoute,
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("enabled runtime feature names = %#v, want runtime-supported set %#v", got, want)
	}
}

func TestSupportedFeaturesExposeSortedGatewayClassStatusShape(t *testing.T) {
	got := SupportedFeatures()
	names := make([]gatewayfeatures.FeatureName, 0, len(got))
	for _, feature := range got {
		names = append(names, gatewayfeatures.FeatureName(feature.Name))
	}

	if len(names) == 0 {
		t.Fatal("expected supported features to be non-empty")
	}
	if !sort.SliceIsSorted(names, func(i, j int) bool {
		return names[i] < names[j]
	}) {
		t.Fatalf("gatewayclass supported features are not sorted: %#v", got)
	}

	want := SupportedFeatureNames()
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("gatewayclass supported features = %#v, want %#v", names, want)
	}

	for _, feature := range got {
		if feature.Name == gatewayv1.FeatureName("") {
			t.Fatalf("unexpected empty supported feature entry in %#v", got)
		}
	}
}

func sortedFeatureNames(items []gatewayfeatures.FeatureName) []gatewayfeatures.FeatureName {
	out := append([]gatewayfeatures.FeatureName(nil), items...)
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func featureNameSet(items []gatewayfeatures.FeatureName) map[gatewayfeatures.FeatureName]bool {
	out := make(map[gatewayfeatures.FeatureName]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}
