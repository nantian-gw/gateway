//go:build conformance

package conformance

import (
	"testing"

	conformancesuite "sigs.k8s.io/gateway-api/conformance/utils/suite"
)

func TestApplyEnvFeatureOptionsEnablesAllFeatures(t *testing.T) {
	t.Setenv("ALL_FEATURES", "true")

	got := applyEnvFeatureOptions(conformancesuite.ConformanceOptions{})

	if !got.EnableAllSupportedFeatures {
		t.Fatal("ALL_FEATURES=true should enable all supported conformance features")
	}
}

func TestApplyEnvFeatureOptionsLeavesAllFeaturesDisabledByDefault(t *testing.T) {
	t.Setenv("ALL_FEATURES", "")

	got := applyEnvFeatureOptions(conformancesuite.ConformanceOptions{})

	if got.EnableAllSupportedFeatures {
		t.Fatal("empty ALL_FEATURES should not enable all supported conformance features")
	}
}

func TestApplyEnvFeatureOptionsDoesNotDisableExplicitAllFeatures(t *testing.T) {
	t.Setenv("ALL_FEATURES", "false")

	got := applyEnvFeatureOptions(conformancesuite.ConformanceOptions{EnableAllSupportedFeatures: true})

	if !got.EnableAllSupportedFeatures {
		t.Fatal("ALL_FEATURES=false should not disable explicitly enabled all-features mode")
	}
}
