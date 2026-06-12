//go:build conformance

package conformance

import (
	"io/fs"
	"strings"
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

func TestGatewayAPIManifestFSIncludesLocalOverlayByDefault(t *testing.T) {
	t.Setenv("GATEWAY_API_WORK_DIR", "")

	got, err := gatewayAPIManifestFS()
	if err != nil {
		t.Fatalf("gatewayAPIManifestFS() returned error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("gatewayAPIManifestFS() should include the in-repo conformance manifest overlay")
	}
	if _, err := fs.ReadFile(got[0], "tests/mesh/httproute-303-redirect.yaml"); err != nil {
		t.Fatalf("first manifest FS should contain local mesh redirect overlay: %v", err)
	}
}

func TestGatewayAPIManifestFSLocalOverlayAddsMesh307Redirect(t *testing.T) {
	t.Setenv("GATEWAY_API_WORK_DIR", "")

	got, err := gatewayAPIManifestFS()
	if err != nil {
		t.Fatalf("gatewayAPIManifestFS() returned error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("gatewayAPIManifestFS() should include the in-repo conformance manifest overlay")
	}

	data, err := fs.ReadFile(got[0], "tests/mesh/httproute-303-redirect.yaml")
	if err != nil {
		t.Fatalf("read local mesh redirect overlay: %v", err)
	}
	contents := string(data)
	for _, want := range []string{
		"name: mesh-307-redirect",
		"value: /temporary",
		"statusCode: 307",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("local mesh redirect overlay missing %q:\n%s", want, contents)
		}
	}

	trimmed := strings.TrimSpace(contents)
	if !strings.HasPrefix(trimmed, "---\n") {
		t.Fatalf("local mesh redirect overlay must start with a YAML document separator so upstream manifests can be appended safely:\n%s", contents)
	}
	if !strings.HasSuffix(trimmed, "\n---") {
		t.Fatalf("local mesh redirect overlay must end with a YAML document separator so upstream manifests can be appended safely:\n%s", contents)
	}
}
