package conformance

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gatewayconformance "sigs.k8s.io/gateway-api/conformance"
	gatewaytests "sigs.k8s.io/gateway-api/conformance/tests"
	conformancesuite "sigs.k8s.io/gateway-api/conformance/utils/suite"
	gatewayfeatures "sigs.k8s.io/gateway-api/pkg/features"
	"sigs.k8s.io/yaml"
)

const (
	defaultUsableStaticAddress   = "IPAddress=127.0.0.1"
	defaultUnusableStaticAddress = "IPAddress=203.0.113.13"

	meshHTTPRoute307RedirectTest     = "MeshHTTPRoute307Redirect"
	meshHTTPRoute307RedirectManifest = "tests/mesh/httproute-307-redirect.yaml"
	meshHTTPRoute303RedirectManifest = "tests/mesh/httproute-303-redirect.yaml"
)

var (
	conformanceNamespaces = []string{
		"gateway-conformance-infra",
		"gateway-conformance-app-backend",
		"gateway-conformance-web-backend",
		"gateway-conformance-mesh",
		"gateway-conformance-mesh-consumer",
	}
	foreignGatewayResourceKinds = []schema.GroupVersionKind{
		{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway"},
		{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"},
		{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GRPCRoute"},
		{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "TCPRoute"},
		{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "UDPRoute"},
		{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "TLSRoute"},
	}
)

func TestGatewayAPIConformance(t *testing.T) {
	failOnForeignGatewayResources(t)

	options := gatewayconformance.DefaultOptions(t)
	options, expandedAllFeatures := patchAllFeatures(options)
	options = avoidEmptyFeatureInference(options)

	manifestFS, err := gatewayAPIManifestFS()
	if err != nil {
		t.Fatalf("build conformance manifest fs: %v", err)
	}
	options.ManifestFS = manifestFS
	options.UsableNetworkAddresses = parseGatewayAddresses(
		os.Getenv("CONFORMANCE_USABLE_ADDRESSES"),
		defaultUsableStaticAddress,
	)
	options.UnusableNetworkAddresses = parseGatewayAddresses(
		os.Getenv("CONFORMANCE_UNUSABLE_ADDRESSES"),
		defaultUnusableStaticAddress,
	)

	runConformanceWithOptions(t, options, expandedAllFeatures)
}

func failOnForeignGatewayResources(t *testing.T) {
	t.Helper()

	if strings.EqualFold(strings.TrimSpace(os.Getenv("ALLOW_FOREIGN_GATEWAY_RESOURCES")), "true") {
		return
	}

	k8sClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{})
	if err != nil {
		t.Fatalf("build kubernetes client for foreign resource preflight: %v", err)
	}

	ctx := context.Background()
	var foreign []string

	for _, gvk := range foreignGatewayResourceKinds {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk.GroupVersion().WithKind(gvk.Kind + "List"))

		if err := k8sClient.List(ctx, list); err != nil {
			if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
				continue
			}
			t.Fatalf("list %s for foreign resource preflight: %v", gvk.String(), err)
		}

		for _, item := range list.Items {
			if slices.Contains(conformanceNamespaces, item.GetNamespace()) {
				continue
			}
			foreign = append(foreign, fmt.Sprintf("%s\t%s\t%s", item.GetKind(), item.GetNamespace(), item.GetName()))
		}
	}

	if len(foreign) == 0 {
		return
	}

	t.Fatalf(
		"found gateway api resources outside conformance namespaces:\n%s\nremove the resources above or rerun with ALLOW_FOREIGN_GATEWAY_RESOURCES=true",
		strings.Join(foreign, "\n"),
	)
}

func patchAllFeatures(options conformancesuite.ConformanceOptions) (conformancesuite.ConformanceOptions, bool) {
	if !options.EnableAllSupportedFeatures {
		return options, false
	}

	// Keep ALL_FEATURES=true expanded to the repository's explicit feature list.
	// This makes conformance logs show the exact declared feature set and avoids
	// treating upstream all-features as a claim that every upstream feature is supported.
	options.SupportedFeatures = gatewayfeatures.SetsToNamesSet(gatewayfeatures.AllFeatures)
	options.SupportedFeatures.Insert(gatewayfeatures.SupportUDPRoute)
	options.EnableAllSupportedFeatures = false
	return options, true
}

func runConformanceWithOptions(t *testing.T, opts conformancesuite.ConformanceOptions, expandedAllFeatures bool) {
	if err := opts.Implementation.Validate(); err != nil && opts.ReportOutputPath != "" {
		t.Fatalf("supplied Implementation details are not valid: %v", err)
	}

	if opts.ManifestFS == nil {
		opts.ManifestFS = []fs.FS{&gatewayconformance.Manifests}
	}

	t.Log("Running conformance tests with:")
	logConformanceOptions(t, opts, expandedAllFeatures)

	cSuite, err := conformancesuite.NewConformanceTestSuite(opts)
	if err != nil {
		t.Fatalf("error initializing conformance suite: %v", err)
	}

	conformanceTests := patchGatewayAPIConformanceTests(gatewaytests.ConformanceTests)
	cSuite.Setup(t, conformanceTests)
	if err := cSuite.Run(t, conformanceTests); err != nil {
		t.Fatalf("run conformance suite: %v", err)
	}

	if opts.ReportOutputPath == "" {
		return
	}

	report, err := cSuite.Report()
	if err != nil {
		t.Fatalf("error generating conformance profile report: %v", err)
	}
	if err := writeConformanceReport(t.Logf, report, opts.ReportOutputPath); err != nil {
		t.Fatalf("error writing report: %v", err)
	}
}

func logConformanceOptions(t *testing.T, opts conformancesuite.ConformanceOptions, expandedAllFeatures bool) {
	t.Logf("  GatewayClass: %s", opts.GatewayClassName)
	t.Logf("  Cleanup Resources: %t", opts.CleanupBaseResources)
	t.Logf("  Debug: %t", opts.Debug)
	if expandedAllFeatures {
		t.Logf("  Enable All Features: true (expanded locally to explicit Supported Features + UDPRoute)")
	} else {
		t.Logf("  Enable All Features: %t", opts.EnableAllSupportedFeatures)
	}
	t.Logf("  Supported Features: %v", opts.SupportedFeatures.UnsortedList())
	t.Logf("  ExemptFeatures: %v", opts.ExemptFeatures.UnsortedList())
	t.Logf("  ConformanceProfiles: %v", opts.ConformanceProfiles.UnsortedList())
}

func writeConformanceReport(logf func(string, ...any), report any, output string) error {
	rawReport, err := yaml.Marshal(report)
	if err != nil {
		return err
	}

	if output != "" {
		if err = os.WriteFile(output, rawReport, 0o600); err != nil {
			return err
		}
	}
	logf("Conformance report:\n%s", string(rawReport))
	return nil
}

func avoidEmptyFeatureInference(options conformancesuite.ConformanceOptions) conformancesuite.ConformanceOptions {
	if options.EnableAllSupportedFeatures || options.SupportedFeatures.Len() > 0 || options.ExemptFeatures.Len() > 0 || options.RunTest != "" {
		return options
	}
	if options.ConformanceProfiles.Len() == 0 {
		return options
	}

	if options.SupportedFeatures == nil {
		options.SupportedFeatures = gatewayfeatures.SetsToNamesSet()
	}
	options.SupportedFeatures.Insert(gatewayfeatures.SupportGateway)
	return options
}

func patchGatewayAPIConformanceTests(tests []conformancesuite.ConformanceTest) []conformancesuite.ConformanceTest {
	patched := slices.Clone(tests)
	for i := range patched {
		if patched[i].ShortName != meshHTTPRoute307RedirectTest {
			continue
		}
		// Gateway API conformance v1.5.1 accidentally points the mesh 307 test
		// at the 303 manifest. Keep the shim local until the upstream test is fixed.
		if slices.Equal(patched[i].Manifests, []string{meshHTTPRoute303RedirectManifest}) {
			patched[i].Manifests = []string{meshHTTPRoute307RedirectManifest}
		}
	}
	return patched
}

func gatewayAPIManifestFS() ([]fs.FS, error) {
	workDir := strings.TrimSpace(os.Getenv("GATEWAY_API_WORK_DIR"))
	if workDir == "" {
		return nil, fmt.Errorf("GATEWAY_API_WORK_DIR is required")
	}

	return []fs.FS{os.DirFS(filepath.Join(workDir, "conformance"))}, nil
}

func parseGatewayAddresses(raw string, fallback string) []gatewayv1beta1.GatewaySpecAddress {
	if strings.TrimSpace(raw) == "" {
		raw = fallback
	}

	parts := strings.Split(raw, ",")
	addresses := make([]gatewayv1beta1.GatewaySpecAddress, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}

		addressType := gatewayv1beta1.IPAddressType
		value := item
		if typed := strings.SplitN(item, "=", 2); len(typed) == 2 {
			addressType = gatewayv1beta1.AddressType(strings.TrimSpace(typed[0]))
			value = strings.TrimSpace(typed[1])
		}

		addresses = append(addresses, gatewayv1beta1.GatewaySpecAddress{
			Type:  &addressType,
			Value: value,
		})
	}

	return addresses
}
