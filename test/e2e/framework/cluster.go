//go:build e2e

package framework

import (
	"os"
	"os/exec"
	"path/filepath"
)

func CreateKindCluster(t T, name string) {
	t.Helper()

	configPath := KindConfigPath()
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("kind config not found at %s: %v", configPath, err)
	}

	t.Logf("creating kind cluster %q with config %s", name, configPath)

	cmd := exec.Command("kind", "create", "cluster",
		"--name", name,
		"--config", configPath,
		"--wait", "5m",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("kind create cluster failed: %v", err)
	}

	cmd = exec.Command("kubectl", "wait",
		"--for=condition=ready", "node", "--all",
		"--timeout=2m",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("wait for nodes ready failed: %v", err)
	}

	t.Logf("kind cluster %q is ready", name)
}

func DeleteKindCluster(t T, name string) {
	t.Helper()

	t.Logf("deleting kind cluster %q", name)

	cmd := exec.Command("kind", "delete", "cluster", "--name", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Logf("warning: kind delete cluster failed (may not exist): %v", err)
	}
}

func KindConfigPath() string {
	if p := os.Getenv("KIND_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(GatewayRoot(), "scripts", "ci", "kind-ci-config.yaml")
}

func GatewayRoot() string {
	if p := os.Getenv("GATEWAY_ROOT"); p != "" {
		return p
	}
	cwd, _ := os.Getwd()
	return cwd
}
