package conformance

import (
	"slices"
	"testing"

	gatewaytests "sigs.k8s.io/gateway-api/conformance/tests"
	conformancesuite "sigs.k8s.io/gateway-api/conformance/utils/suite"
)

func TestPatchGatewayAPIConformanceTestsCorrectsMeshHTTPRoute307RedirectManifest(t *testing.T) {
	t.Parallel()

	tests := patchGatewayAPIConformanceTests(gatewaytests.ConformanceTests)

	index := slices.IndexFunc(tests, func(test conformancesuite.ConformanceTest) bool {
		return test.ShortName == "MeshHTTPRoute307Redirect"
	})
	if index == -1 {
		t.Fatal("MeshHTTPRoute307Redirect conformance test not found")
	}

	got := tests[index].Manifests
	want := []string{"tests/mesh/httproute-307-redirect.yaml"}
	if !slices.Equal(got, want) {
		t.Fatalf("MeshHTTPRoute307Redirect manifests = %v, want %v", got, want)
	}
}
