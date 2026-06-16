package grpcserver

import (
	"sort"
	"strings"
)

const (
	featureCoreV1                              = "core.v1"
	featureRouteLabelsV1                       = "route.labels.v1"
	featureBackendAIServiceV1                  = "backend.ai_service.v1"
	featureBackendTokenPolicyV1                = "backend.token_policy.v1"
	featureBackendWasmPluginV1                 = "backend.wasm_plugin.v1"
	compatibilityProfileFullV1                 = "full-v1"
	compatibilityProfileLegacyPreNegotiationV1 = "legacy-pre-negotiation-v1"
)

type projectionProfile struct {
	advertised           []string
	effective            []string
	compatibilityProfile string
	projectionKey        string
}

var orderedProjectionFeatures = []string{
	featureCoreV1,
	featureRouteLabelsV1,
	featureBackendAIServiceV1,
	featureBackendTokenPolicyV1,
	featureBackendWasmPluginV1,
}

var legacyFallbackProjectionFeatures = []string{
	featureCoreV1,
	featureBackendAIServiceV1,
	featureBackendTokenPolicyV1,
	featureBackendWasmPluginV1,
}

func canonicalizeSupportedFeatures(features []string) []string {
	if len(features) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(features))
	out := make([]string, 0, len(features))
	for _, feature := range features {
		trimmed := strings.TrimSpace(feature)
		if trimmed == "" {
			continue
		}
		if _, ok := set[trimmed]; ok {
			continue
		}
		set[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}

	sort.Strings(out)
	return out
}

func effectiveProjectionProfile(advertised []string) projectionProfile {
	advertised = canonicalizeSupportedFeatures(advertised)
	if len(advertised) == 0 {
		effective := append([]string(nil), legacyFallbackProjectionFeatures...)
		return projectionProfile{
			effective:            effective,
			compatibilityProfile: compatibilityProfileLegacyPreNegotiationV1,
			projectionKey:        compatibilityProfileLegacyPreNegotiationV1,
		}
	}

	advertisedSet := make(map[string]struct{}, len(advertised)+1)
	for _, feature := range advertised {
		advertisedSet[feature] = struct{}{}
	}
	advertisedSet[featureCoreV1] = struct{}{}

	effective := make([]string, 0, len(advertisedSet))
	for _, feature := range orderedProjectionFeatures {
		if _, ok := advertisedSet[feature]; ok {
			effective = append(effective, feature)
			delete(advertisedSet, feature)
		}
	}

	unknown := make([]string, 0, len(advertisedSet))
	for feature := range advertisedSet {
		unknown = append(unknown, feature)
	}
	sort.Strings(unknown)
	effective = append(effective, unknown...)

	compatibilityProfile := buildCompatibilityProfile(effective)
	if stringSlicesEqual(effective, orderedProjectionFeatures) {
		compatibilityProfile = compatibilityProfileFullV1
	}

	return projectionProfile{
		advertised:           append([]string(nil), advertised...),
		effective:            effective,
		compatibilityProfile: compatibilityProfile,
		projectionKey:        compatibilityProfile,
	}
}

func buildCompatibilityProfile(features []string) string {
	parts := make([]string, 0, len(features)+1)
	parts = append(parts, "compat-v1")
	for _, feature := range features {
		parts = append(parts, strings.ReplaceAll(feature, ".", "_"))
	}
	return strings.Join(parts, "__")
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
