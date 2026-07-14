//go:build conformance

package conformance

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gatewayconformance "sigs.k8s.io/gateway-api/conformance"
	gatewaytests "sigs.k8s.io/gateway-api/conformance/tests"
	conformancesuite "sigs.k8s.io/gateway-api/conformance/utils/suite"
	"sigs.k8s.io/yaml"

	"github.com/nantian-gw/gateway/internal/gwapi"
)

const (
	defaultUsableStaticAddress   = "IPAddress=127.0.0.1"
	defaultUnusableStaticAddress = "IPAddress=203.0.113.13"
)

func TestGatewayAPIConformance(t *testing.T) {
	options := applyEnvFeatureOptions(gatewayconformance.DefaultOptions(t))
	options, expandedAllFeatures := patchAllFeatures(options)

	// Kind cluster timing: resource propagation delay requires generous timeouts.
	options.TimeoutConfig.CreateTimeout = 30 * time.Second
	options.TimeoutConfig.DeleteTimeout = 30 * time.Second
	options.TimeoutConfig.GetTimeout = 30 * time.Second
	options.TimeoutConfig.TestIsolation = 30 * time.Second

	options.SkipTests = []string{
		// Not yet implemented or fixed:
		"GatewayHTTPListenerIsolation",
		"HTTPRouteListenerHostnameMatching",
		"HTTPRouteRedirectPortAndScheme",
		"ListenerSetHTTPRouting",
		"BackendTLSPolicy",
	}

	manifestFS, err := gatewayAPIManifestFS()
	if err != nil {
		t.Fatalf("build conformance manifest fs: %v", err)
	}
	if len(manifestFS) > 0 {
		options.ManifestFS = append(manifestFS, &gatewayconformance.Manifests)
	}
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

func applyEnvFeatureOptions(options conformancesuite.ConformanceOptions) conformancesuite.ConformanceOptions {
	if envFlagEnabled("ALL_FEATURES") {
		options.EnableAllSupportedFeatures = true
	}
	return options
}

func envFlagEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func patchAllFeatures(options conformancesuite.ConformanceOptions) (conformancesuite.ConformanceOptions, bool) {
	if !options.EnableAllSupportedFeatures {
		return options, false
	}

	options.SupportedFeatures = gwapi.SupportedFeatureNameSet()
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

	cSuite.Setup(t, gatewaytests.ConformanceTests)
	if err := cSuite.Run(t, gatewaytests.ConformanceTests); err != nil {
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

func gatewayAPIManifestFS() ([]fs.FS, error) {
	out := []fs.FS{os.DirFS("manifests")}

	workDir := strings.TrimSpace(os.Getenv("GATEWAY_API_WORK_DIR"))
	if workDir != "" {
		out = append(out, os.DirFS(filepath.Join(workDir, "conformance")))
	}

	out = append(out, &gatewayconformance.Manifests)
	return out, nil
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
