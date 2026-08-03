//go:build e2e

package framework

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	ControlPlaneNS = "nantian-gw"

	gatewayAPIVersion      = "v1.5.1"
	gatewayAPIBaseURL      = "https://github.com/kubernetes-sigs/gateway-api/releases/download/" + gatewayAPIVersion
	gatewayAPIStandard     = "standard-install.yaml"
	gatewayAPIExperimental = "experimental-install.yaml"
)

func InstallGatewayAPICRDs(t T) {
	t.Helper()

	standardURL := gatewayAPIBaseURL + "/" + gatewayAPIStandard
	experimentalURL := gatewayAPIBaseURL + "/" + gatewayAPIExperimental

	t.Log("installing Gateway API standard CRDs")
	runKubectl(t, "apply", "-f", standardURL)

	t.Log("installing Gateway API experimental CRDs")
	if err := runKubectlNoFail(t, "apply", "-f", experimentalURL); err != nil {
		t.Logf("experimental CRDs install reported error (may be expected): %v", err)
	}

	t.Log("waiting for GatewayClass CRD to be established")
	runKubectl(t, "wait", "--for=condition=established",
		"crd/gatewayclasses.gateway.networking.k8s.io",
		"--timeout=60s")
}

func DeployNantianGW(t T) {
	t.Helper()

	overlayDir := filepath.Join(GatewayRoot(), "deploy", "kubernetes", "overlays", "kind-conformance")

	t.Logf("building kustomize overlay from %s", overlayDir)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("kustomize", "build",
		overlayDir,
		"--load-restrictor", "LoadRestrictionsNone",
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("kustomize build failed: %v\nstderr: %s", err, stderr.String())
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = bytes.NewReader(stdout.Bytes())
	applyCmd.Stdout = os.Stdout
	applyCmd.Stderr = os.Stderr
	if err := applyCmd.Run(); err != nil {
		t.Fatalf("kubectl apply failed: %v", err)
	}

	t.Log("waiting for nantian-gw pods to be ready")
	for i := 0; i < 24; i++ {
		cmd := exec.Command("kubectl", "wait",
			"--for=condition=ready", "pod", "--all",
			"-n", ControlPlaneNS,
			"--timeout=15s",
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			t.Log("nantian-gw deployment ready")
			return
		}
		if i < 23 {
			time.Sleep(15 * time.Second)
		}
	}
	t.Fatal("nantian-gw pods did not become ready within timeout")
}

func runKubectl(t T, args ...string) {
	t.Helper()
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("kubectl %v failed: %v", args, err)
	}
}

func runKubectlNoFail(t T, args ...string) error {
	t.Helper()
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
