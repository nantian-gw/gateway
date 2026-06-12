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
	Features controlPlaneFeatures `yaml:"features"`
}

type controlPlaneFeatures struct {
	EnableExperimentalGateway bool `yaml:"enableExperimentalGateway"`
	EnableAiGateway           bool `yaml:"enableAiGateway"`
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

func TestSmokeScriptForwardsToProgrammedGatewayListener(t *testing.T) {
	contents := string(readFile(t, repoPath("test", "e2e", "smoke", "run.sh")))

	for _, want := range []string{
		`LOCAL_HTTP_PORT="${LOCAL_HTTP_PORT:-10080}"`,
		`GATEWAY_HTTP_PORT="${GATEWAY_HTTP_PORT:-80}"`,
		`"${LOCAL_HTTP_PORT}:${GATEWAY_HTTP_PORT}"`,
		`request_deadline=`,
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("smoke script missing %q", want)
		}
	}

	if strings.Contains(contents, "10080:10080") {
		t.Fatalf("smoke script still forwards stale dataplane port 10080 to 10080")
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
		`DATA_PLANE_SELECTOR="app=nantian-gw-dataplane"`,
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

func TestConformanceOverlayEnablesAdvertisedExperimentalGatewayFeatures(t *testing.T) {
	data := readFile(t, repoPath("deploy", "kubernetes", "overlays", "kind-conformance", "controlplane-config.yaml"))

	var config controlPlaneConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse kind-conformance controlplane config: %v", err)
	}

	if !config.Features.EnableExperimentalGateway {
		t.Fatalf("kind conformance controlplane config must enable experimental Gateway API support for advertised ListenerSet/TCPRoute/UDPRoute/TLSRoute features")
	}
	if config.Features.EnableAiGateway {
		t.Fatalf("kind conformance controlplane config should not enable AI Gateway features")
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
