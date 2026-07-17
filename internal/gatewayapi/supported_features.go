package gatewayapi

import (
	"sort"

	"k8s.io/apimachinery/pkg/util/sets"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfeatures "sigs.k8s.io/gateway-api/pkg/features"
)

// SupportedTCPRoute indicates that the implementation supports TCPRoute resources.
const SupportedTCPRoute gatewayfeatures.FeatureName = "TCPRoute"

// SupportedBackendLBSessionPersistence indicates that the implementation
// supports session persistence configuration in BackendLBPolicy.
const SupportedBackendLBSessionPersistence gatewayfeatures.FeatureName = "BackendLBSessionPersistence"

// FeatureOptions controls which supported features are advertised for a
// particular control-plane runtime configuration.
type FeatureOptions struct {
	EnableExperimentalGateway bool
}

var experimentalGatewayFeatures = []gatewayfeatures.FeatureName{
	SupportedTCPRoute,
	gatewayfeatures.SupportUDPRoute,
	gatewayfeatures.SupportTLSRoute,
	gatewayfeatures.SupportTLSRouteModeTerminate,
	gatewayfeatures.SupportTLSRouteModeMixed,
}

// SupportedFeatureNameSet returns the feature-name set this repository
// advertises through the in-repo conformance profile.
func SupportedFeatureNameSet() sets.Set[gatewayfeatures.FeatureName] {
	return sets.New(
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
	)
}

// SupportedFeatureNameSetForOptions returns the feature-name set advertised by
// a control-plane instance with the provided runtime feature options.
func SupportedFeatureNameSetForOptions(options FeatureOptions) sets.Set[gatewayfeatures.FeatureName] {
	out := SupportedFeatureNameSet()
	if !options.EnableExperimentalGateway {
		for _, name := range experimentalGatewayFeatures {
			out.Delete(name)
		}
	}
	return out
}

// SupportedFeatureNames returns the feature-name set this repository currently
// advertises through the in-repo conformance profile.
func SupportedFeatureNames() []gatewayfeatures.FeatureName {
	return sortedFeatureNamesFromSet(SupportedFeatureNameSet())
}

// SupportedFeatureNamesForOptions returns the sorted feature names advertised by
// a control-plane instance with the provided runtime feature options.
func SupportedFeatureNamesForOptions(options FeatureOptions) []gatewayfeatures.FeatureName {
	return sortedFeatureNamesFromSet(SupportedFeatureNameSetForOptions(options))
}

func sortedFeatureNamesFromSet(items sets.Set[gatewayfeatures.FeatureName]) []gatewayfeatures.FeatureName {
	out := items.UnsortedList()
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

// SupportedFeatures converts the repository's supported feature set into the
// GatewayClass status shape required by the Gateway API.
func SupportedFeatures() []gatewayv1.SupportedFeature {
	names := SupportedFeatureNames()
	return supportedFeaturesFromNames(names)
}

// SupportedFeaturesForOptions converts the runtime feature set into the
// GatewayClass status shape required by the Gateway API.
func SupportedFeaturesForOptions(options FeatureOptions) []gatewayv1.SupportedFeature {
	names := SupportedFeatureNamesForOptions(options)
	return supportedFeaturesFromNames(names)
}

func supportedFeaturesFromNames(names []gatewayfeatures.FeatureName) []gatewayv1.SupportedFeature {
	out := make([]gatewayv1.SupportedFeature, 0, len(names))
	for _, name := range names {
		out = append(out, gatewayv1.SupportedFeature{
			Name: gatewayv1.FeatureName(name),
		})
	}
	return out
}
