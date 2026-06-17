package ci

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmokeScriptMarksEarlyBootstrapFailureAsFailed(t *testing.T) {
	stubDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatalf("create stub dir: %v", err)
	}

	kubectlPath := filepath.Join(stubDir, "kubectl")
	kubectlStub := `#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "create" ]]; then
  printf 'apiVersion: v1\nkind: Namespace\nmetadata:\n  name: nantian-e2e\n'
  exit 0
fi

if [[ "${1:-}" == "apply" ]]; then
  cat >/dev/null
  exit 0
fi

if [[ "${1:-}" == "wait" ]]; then
  echo "error: timed out waiting for the condition on pods/echo-test" >&2
  exit 1
fi

echo "unexpected kubectl invocation: $*" >&2
exit 1
`
	if err := os.WriteFile(kubectlPath, []byte(kubectlStub), 0o755); err != nil {
		t.Fatalf("write kubectl stub: %v", err)
	}

	scriptPath, err := filepath.Abs(repoPath("test", "e2e", "smoke", "run.sh"))
	if err != nil {
		t.Fatalf("resolve smoke script path: %v", err)
	}

	cmd := exec.Command("bash", scriptPath, "--no-cleanup", "--skip-bootstrap")
	cmd.Env = append(
		os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TIMEOUT=1",
	)

	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("expected smoke script to fail, output:\n%s", output)
	}

	text := string(output)
	if !strings.Contains(text, "Smoke test FAILED") {
		t.Fatalf("expected FAILED summary, output:\n%s", text)
	}
	if strings.Contains(text, "Smoke test PASSED") {
		t.Fatalf("unexpected PASSED summary on failure, output:\n%s", text)
	}
}
