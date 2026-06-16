package grpcserver

import (
	"reflect"
	"testing"
)

func TestCanonicalizeSupportedFeaturesTrimsSortsAndDeduplicates(t *testing.T) {
	got := canonicalizeSupportedFeatures([]string{" route.labels.v1 ", "", "core.v1", "core.v1"})
	want := []string{featureCoreV1, featureRouteLabelsV1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonicalizeSupportedFeatures() = %#v, want %#v", got, want)
	}
}

func TestEffectiveProjectionProfileUsesLegacyFallbackForEmptyAdvertisement(t *testing.T) {
	got := effectiveProjectionProfile(nil)
	wantEffective := []string{
		featureCoreV1,
		featureBackendAIServiceV1,
		featureBackendTokenPolicyV1,
		featureBackendWasmPluginV1,
	}
	if len(got.advertised) != 0 {
		t.Fatalf("advertised features = %#v, want empty", got.advertised)
	}
	if !reflect.DeepEqual(got.effective, wantEffective) {
		t.Fatalf("effective features = %#v, want %#v", got.effective, wantEffective)
	}
	if got.compatibilityProfile != compatibilityProfileLegacyPreNegotiationV1 {
		t.Fatalf("compatibility profile = %q, want %q", got.compatibilityProfile, compatibilityProfileLegacyPreNegotiationV1)
	}
	if got.projectionKey != compatibilityProfileLegacyPreNegotiationV1 {
		t.Fatalf("projection key = %q, want %q", got.projectionKey, compatibilityProfileLegacyPreNegotiationV1)
	}
}
