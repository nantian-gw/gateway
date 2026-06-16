package ci

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type kindClusterConfig struct {
	Kind       string     `yaml:"kind"`
	APIVersion string     `yaml:"apiVersion"`
	Nodes      []kindNode `yaml:"nodes"`
}

type kindNode struct {
	Role              string            `yaml:"role"`
	ExtraPortMappings []kindPortMapping `yaml:"extraPortMappings"`
}

type kindPortMapping struct {
	ContainerPort int    `yaml:"containerPort"`
	HostPort      int    `yaml:"hostPort"`
	Protocol      string `yaml:"protocol"`
}

type controlPlaneConfig struct {
	Features  controlPlaneFeatures  `yaml:"features"`
	Dashboard controlPlaneDashboard `yaml:"dashboard"`
}

type controlPlaneFeatures struct {
	EnableExperimentalGateway bool `yaml:"enableExperimentalGateway"`
	EnableAiGateway           bool `yaml:"enableAiGateway"`
}

type controlPlaneDashboard struct {
	Enabled      bool                              `yaml:"enabled"`
	Capabilities controlPlaneDashboardCapabilities `yaml:"capabilities"`
}

type controlPlaneDashboardCapabilities struct {
	AIOverview      bool `yaml:"aiOverview"`
	AIServices      bool `yaml:"aiServices"`
	AITokenPolicies bool `yaml:"aiTokenPolicies"`
	AICost          bool `yaml:"aiCost"`
	AITraces        bool `yaml:"aiTraces"`
	AIUsage         bool `yaml:"aiUsage"`
	WasmPlugins     bool `yaml:"wasmPlugins"`
	Chatbot         bool `yaml:"chatbot"`
}

type kustomizationConfig struct {
	Patches []kustomizationPatch `yaml:"patches"`
}

type kustomizationPatch struct {
	Path string `yaml:"path"`
}

type deploymentReplicaConfig struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		Replicas *int `yaml:"replicas"`
	} `yaml:"spec"`
}

func TestKindCIConfigExposesConformancePorts(t *testing.T) {
	data := readFile(t, "kind-ci-config.yaml")

	var config kindClusterConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse kind-ci-config.yaml: %v", err)
	}

	if config.Kind != "Cluster" {
		t.Fatalf("kind = %q, want Cluster", config.Kind)
	}
	if config.APIVersion != "kind.x-k8s.io/v1alpha4" {
		t.Fatalf("apiVersion = %q, want kind.x-k8s.io/v1alpha4", config.APIVersion)
	}

	var controlPlane *kindNode
	controlPlaneCount := 0
	for idx := range config.Nodes {
		if config.Nodes[idx].Role == "control-plane" {
			controlPlaneCount++
			controlPlane = &config.Nodes[idx]
		}
	}
	if controlPlane == nil {
		t.Fatalf("kind-ci-config.yaml has no control-plane node")
	}
	if controlPlaneCount != 1 {
		t.Fatalf("kind-ci-config.yaml has %d control-plane nodes, want 1", controlPlaneCount)
	}

	mappings := make(map[string]kindPortMapping, len(controlPlane.ExtraPortMappings))
	for _, mapping := range controlPlane.ExtraPortMappings {
		protocol := strings.ToUpper(mapping.Protocol)
		key := portKey(mapping.HostPort, protocol)
		if _, exists := mappings[key]; exists {
			t.Fatalf("duplicate host mapping for %s", key)
		}
		mapping.Protocol = protocol
		mappings[key] = mapping
	}

	required := []kindPortMapping{
		{HostPort: 80, ContainerPort: 30080, Protocol: "TCP"},
		{HostPort: 443, ContainerPort: 30443, Protocol: "TCP"},
		{HostPort: 8080, ContainerPort: 32080, Protocol: "TCP"},
		{HostPort: 8090, ContainerPort: 32090, Protocol: "TCP"},
		{HostPort: 8443, ContainerPort: 32443, Protocol: "TCP"},
		{HostPort: 8883, ContainerPort: 31883, Protocol: "TCP"},
		{HostPort: 5300, ContainerPort: 31300, Protocol: "UDP"},
	}

	for _, want := range required {
		got, ok := mappings[portKey(want.HostPort, want.Protocol)]
		if !ok {
			t.Fatalf("missing host port mapping %d/%s", want.HostPort, want.Protocol)
		}
		if got.ContainerPort != want.ContainerPort {
			t.Fatalf(
				"host port mapping %d/%s containerPort = %d, want %d",
				want.HostPort,
				want.Protocol,
				got.ContainerPort,
				want.ContainerPort,
			)
		}
	}
}

func TestWorkflowsUseSharedKindClusterHelper(t *testing.T) {
	for _, workflow := range []string{
		repoPath(".github", "workflows", "e2e.yml"),
		repoPath(".github", "workflows", "conformance.yml"),
	} {
		t.Run(filepath.Base(workflow), func(t *testing.T) {
			contents := string(readFile(t, workflow))
			if !strings.Contains(contents, "scripts/ci/create-kind-cluster.sh") {
				t.Fatalf("%s does not call scripts/ci/create-kind-cluster.sh", workflow)
			}
			if strings.Contains(contents, `kind create cluster --name "$CLUSTER_NAME" --wait 5m`) {
				t.Fatalf("%s still creates a plain kind cluster without --config", workflow)
			}
		})
	}
}

func TestReleaseWorkflowUsesCurrentCIEntrypoints(t *testing.T) {
	contents := string(readFile(t, repoPath(".github", "workflows", "release.yml")))

	for _, want := range []string{
		`checkout_ref: ${{ steps.vars.outputs.checkout_ref }}`,
		`path: release-src`,
		`go-version-file: release-src/go.mod`,
		`cache-dependency-path: release-src/go.sum`,
		`working-directory: release-src`,
		`run: go test -count=1 -timeout 5m ./...`,
		`run: scripts/ci/install-kind-tools.sh`,
		`run: scripts/ci/create-kind-cluster.sh`,
		`run: scripts/ci/install-gateway-api-crds.sh`,
		`run: scripts/ci/load-kind-images.sh`,
		`run: scripts/ci/deploy-kind-conformance.sh`,
		`run: CLUSTER_NAME="$CLUSTER_NAME" GATEWAY_HTTP_PORT=80 ./test/e2e/smoke/run.sh --no-cleanup --skip-bootstrap`,
		`go test -tags=conformance -count=1 -v -timeout 30m ./conformance/ \`,
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("release workflow missing %q", want)
		}
	}

	for _, unwanted := range []string{
		`working-directory: controlplane`,
		`./tests/e2e/run-kind.sh`,
		`./tests/conformance/run.sh`,
		`scripts/archive-conformance-report.sh`,
		`scripts/publish-conformance-reports.sh`,
	} {
		if strings.Contains(contents, unwanted) {
			t.Fatalf("release workflow still contains stale path %q", unwanted)
		}
	}
}

func TestReleaseWorkflowUsesReleaseTaggedDependencyImages(t *testing.T) {
	contents := string(readFile(t, repoPath(".github", "workflows", "release.yml")))

	for _, want := range []string{
		`dataplane_image: ${{ steps.vars.outputs.dataplane_image }}`,
		`dashboard_image: ${{ steps.vars.outputs.dashboard_image }}`,
		`DATAPLANE_IMAGE: ${{ needs.metadata.outputs.dataplane_image }}`,
		`DASHBOARD_IMAGE: ${{ needs.metadata.outputs.dashboard_image }}`,
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("release workflow missing %q", want)
		}
	}

	for _, unwanted := range []string{
		`DATAPLANE_IMAGE: ghcr.io/nantian-gw/dataplane:latest`,
		`DASHBOARD_IMAGE: ghcr.io/nantian-gw/dashboard:latest`,
	} {
		if strings.Contains(contents, unwanted) {
			t.Fatalf("release workflow must not pin dependency image to %q", unwanted)
		}
	}
}

func TestSecurityScanWorkflowUsesExistingHelper(t *testing.T) {
	contents := string(readFile(t, repoPath(".github", "workflows", "security-scans.yml")))
	helperPath := repoPath("scripts", "ci", "run-security-scans.sh")

	if !strings.Contains(contents, `path: release-src`) {
		t.Fatalf("security scan workflow must checkout the release source into release-src")
	}
	if !strings.Contains(contents, `go-version-file: release-src/go.mod`) {
		t.Fatalf("security scan workflow must resolve the Go toolchain from release-src")
	}
	if !strings.Contains(contents, `SCAN_ROOT=release-src scripts/ci/run-security-scans.sh`) {
		t.Fatalf("security scan workflow must scan release-src through the branch helper")
	}
	if strings.Contains(contents, `run: scripts/ci/run-security-scans.sh`) &&
		!strings.Contains(contents, `SCAN_ROOT=release-src scripts/ci/run-security-scans.sh`) {
		t.Fatalf("security scan workflow must not depend on a helper file from the checked-out release tag")
	}

	if _, err := os.Stat(helperPath); err != nil {
		t.Fatalf("security scan helper %s is missing: %v", helperPath, err)
	}
}

func TestSmokeScriptForwardsToProgrammedGatewayListener(t *testing.T) {
	contents := string(readFile(t, repoPath("test", "e2e", "smoke", "run.sh")))

	for _, want := range []string{
		`GATEWAY_HOST="${GATEWAY_HOST:-127.0.0.1}"`,
		`GATEWAY_HTTP_PORT="${GATEWAY_HTTP_PORT:-80}"`,
		`wait_for_gateway_programmed`,
		`http://${GATEWAY_HOST}:${GATEWAY_HTTP_PORT}/echo`,
		`request_deadline=`,
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("smoke script missing %q", want)
		}
	}

	for _, unwanted := range []string{
		`LOCAL_HTTP_PORT="${LOCAL_HTTP_PORT:-10080}"`,
		`service/$DATA_PLANE_SVC`,
		`kubectl port-forward`,
		`pod/$dataplane_pod`,
		`dataplane_pod=$(kubectl get pod`,
		`port-forward to $dataplane_pod exited before request succeeded`,
		`10080:10080`,
	} {
		if strings.Contains(contents, unwanted) {
			t.Fatalf("smoke script still contains stale pod-forward pattern %q", unwanted)
		}
	}
}

func TestCIEntrypointsUseCurrentDeployResourceNames(t *testing.T) {
	conformanceWorkflow := string(readFile(t, repoPath(".github", "workflows", "conformance.yml")))
	if !strings.Contains(conformanceWorkflow, "-gateway-class nantian-gw") {
		t.Fatalf("conformance workflow does not use GatewayClass nantian-gw")
	}
	if strings.Contains(conformanceWorkflow, "-gateway-class nantian \\") {
		t.Fatalf("conformance workflow still uses old GatewayClass nantian")
	}

	smokeScript := string(readFile(t, repoPath("test", "e2e", "smoke", "run.sh")))
	for _, want := range []string{
		`GATEWAY_CLASS_NAME="${GATEWAY_CLASS_NAME:-nantian-gw}"`,
		`CONTROL_PLANE_DEPLOYMENT="nantian-gw-controlplane"`,
		`gatewayClassName: $GATEWAY_CLASS_NAME`,
	} {
		if !strings.Contains(smokeScript, want) {
			t.Fatalf("smoke script missing %q", want)
		}
	}

	for _, oldName := range []string{
		"nantian-controlplane",
		"gatewayClassName: nantian",
		"app=nantian-dataplane",
	} {
		if strings.Contains(smokeScript, oldName) {
			t.Fatalf("smoke script still contains old resource name %q", oldName)
		}
	}
}

func TestSecurityScanHelperSupportsAlternateScanRoot(t *testing.T) {
	contents := string(readFile(t, repoPath("scripts", "ci", "run-security-scans.sh")))

	for _, want := range []string{
		`SCAN_ROOT="${SCAN_ROOT:-${1:-.}}"`,
		`cd "$SCAN_ROOT"`,
		`osv-scanner scan source -r . --format json --output-file`,
		`grype dir:. -o json --file`,
		`kubescape scan framework nsa`,
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("security scan helper missing %q", want)
		}
	}
}

func TestConformanceOverlayDoesNotEnableExperimentalGatewayFeatures(t *testing.T) {
	data := readFile(t, repoPath("deploy", "kubernetes", "overlays", "kind-conformance", "controlplane-config.yaml"))

	var config controlPlaneConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse kind-conformance controlplane config: %v", err)
	}

	if config.Features.EnableExperimentalGateway {
		t.Fatalf("kind conformance controlplane config must not enable experimental Gateway API support")
	}
	if config.Features.EnableAiGateway {
		t.Fatalf("kind conformance controlplane config should not enable AI Gateway features")
	}
}

func TestCheckedInControlplaneConfigsDeclareDashboardCapabilityPolicy(t *testing.T) {
	for _, path := range []string{
		repoPath("configs", "controlplane", "config.yaml"),
		repoPath("deploy", "kubernetes", "overlays", "production", "controlplane-config.yaml"),
		repoPath("deploy", "kubernetes", "overlays", "kind-conformance", "controlplane-config.yaml"),
	} {
		t.Run(path, func(t *testing.T) {
			var config controlPlaneConfig
			if err := yaml.Unmarshal(readFile(t, path), &config); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}

			if !config.Dashboard.Enabled {
				t.Fatalf("%s dashboard.enabled = false, want true", path)
			}

			caps := config.Dashboard.Capabilities
			for _, check := range []struct {
				name string
				got  bool
			}{
				{"aiOverview", caps.AIOverview},
				{"aiServices", caps.AIServices},
				{"aiTokenPolicies", caps.AITokenPolicies},
				{"aiCost", caps.AICost},
				{"aiTraces", caps.AITraces},
				{"aiUsage", caps.AIUsage},
				{"wasmPlugins", caps.WasmPlugins},
				{"chatbot", caps.Chatbot},
			} {
				if !check.got {
					t.Fatalf("%s dashboard.capabilities.%s = false, want true", path, check.name)
				}
			}
		})
	}
}

func TestConformanceOverlayUsesSingleDataplaneReplica(t *testing.T) {
	overlayDir := repoPath("deploy", "kubernetes", "overlays", "kind-conformance")
	data := readFile(t, filepath.Join(overlayDir, "kustomization.yaml"))

	var config kustomizationConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse kind-conformance kustomization: %v", err)
	}

	var patchPath string
	for _, patch := range config.Patches {
		if strings.Contains(patch.Path, "dataplane-replicas.yaml") {
			patchPath = patch.Path
			break
		}
	}
	if patchPath == "" {
		t.Fatalf("kind-conformance overlay should statically patch dataplane replicas to one")
	}

	patchData := readFile(t, filepath.Clean(filepath.Join(overlayDir, patchPath)))
	var patch deploymentReplicaConfig
	if err := yaml.Unmarshal(patchData, &patch); err != nil {
		t.Fatalf("parse dataplane replica patch: %v", err)
	}
	if patch.Kind != "Deployment" {
		t.Fatalf("dataplane replica patch kind = %q, want Deployment", patch.Kind)
	}
	if patch.Metadata.Name != "nantian-gw-dataplane" {
		t.Fatalf("dataplane replica patch targets %q, want nantian-gw-dataplane", patch.Metadata.Name)
	}
	if patch.Spec.Replicas == nil || *patch.Spec.Replicas != 1 {
		t.Fatalf("dataplane replica patch replicas = %v, want 1", patch.Spec.Replicas)
	}
}

func TestDeployKindConformanceSupportsEnvDrivenExperimentalMode(t *testing.T) {
	contents := string(readFile(t, repoPath("scripts", "ci", "deploy-kind-conformance.sh")))

	for _, want := range []string{
		`CONFORMANCE_EXPERIMENTAL="${CONFORMANCE_EXPERIMENTAL:-${ALL_FEATURES:-false}}"`,
		`if [[ "$CONFORMANCE_EXPERIMENTAL" == "true" ]]; then`,
		`enableExperimentalGateway: true`,
		`"$overlay/controlplane-config.yaml"`,
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("deploy-kind-conformance.sh missing %q", want)
		}
	}

	if strings.Contains(contents, `deploy/kubernetes/overlays/kind-conformance/controlplane-config.yaml`) {
		t.Fatalf("deploy-kind-conformance.sh should only patch the copied temporary overlay")
	}
}

func TestConformanceWorkflowHasScheduledAllFeaturesJob(t *testing.T) {
	contents := string(readFile(t, repoPath(".github", "workflows", "conformance.yml")))

	for _, want := range []string{
		"Gateway API Conformance (All Features)",
		"github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'",
		`CLUSTER_NAME: conformance-all-features`,
		`CONFORMANCE_EXPERIMENTAL: "true"`,
		`ALL_FEATURES: "true"`,
		"conformance-all-features-results",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("conformance workflow missing %q", want)
		}
	}

	if strings.Contains(contents, `            -all-features \`) {
		t.Fatalf("conformance workflow should use ALL_FEATURES env instead of hard-coding -all-features")
	}
}

func portKey(port int, protocol string) string {
	return fmt.Sprintf("%s/%d", protocol, port)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func repoPath(elem ...string) string {
	return filepath.Join(append([]string{"..", ".."}, elem...)...)
}
