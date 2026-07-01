//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

const testNSRouting = "e2e-routing"

func ensureNamespace(t *testing.T, ns string) {
	t.Helper()
	namespaceYAML := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, ns)
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader([]byte(namespaceYAML))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ensure namespace %s: %v", ns, err)
	}
	t.Logf("namespace %s ready", ns)
}

func deploySmokeClient(t *testing.T, ns string) string {
	t.Helper()
	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	podName := "smoke-client"
	ctx := context.Background()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "curl",
					Image:   "curlimages/curl:8.16.0",
					Command: []string{"sleep", "infinity"},
				},
			},
		},
	}

	_, err = clientset.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create smoke-client pod in %s: %v", ns, err)
	}

	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		p, err := clientset.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if p.Status.Phase == corev1.PodRunning {
			t.Logf("smoke-client pod %s/%s is running", ns, podName)
			return podName
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("smoke-client pod did not become ready within timeout")
	return ""
}

func TestBasicRoutingPathPrefix(t *testing.T) {
	ensureNamespace(t, testNSRouting)

	// Deploy echo backends
	framework.DeployEchoBackend(t, testNSRouting, "echo-a")
	framework.WaitForBackendReady(t, testNSRouting, "echo-a")
	framework.DeployEchoBackend(t, testNSRouting, "echo-b")
	framework.WaitForBackendReady(t, testNSRouting, "echo-b")

	// Create Gateway in control plane namespace
	framework.CreateGateway(t, "e2e-gw", "nantian-gw")

	// Create ReferenceGrant to allow cross-namespace service references
	framework.CreateReferenceGrant(t, testNSRouting, "allow-gw", framework.ControlPlaneNS)

	// Create HTTPRoute in test namespace with cross-namespace backend refs
	rules := []map[string]interface{}{
		{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": "/a",
					},
				},
			},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name":      "echo-a",
					"namespace": testNSRouting,
					"port":      int64(8080),
				},
			},
		},
		{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": "/b",
					},
				},
			},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name":      "echo-b",
					"namespace": testNSRouting,
					"port":      int64(8080),
				},
			},
		},
	}
	createHTTPRouteCrossNS(t, testNSRouting, "echo-route", "e2e-gw", rules)

	// Cleanup
	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, testNSRouting, "echo-route")
	})

	// Allow time for the route to propagate to the data plane
	time.Sleep(5 * time.Second)

	// Deploy smoke-client pod in control plane namespace for internal access
	ensureNamespace(t, framework.ControlPlaneNS)
	smokePod := deploySmokeClient(t, framework.ControlPlaneNS)

	// Verify routing: PathPrefix /a → echo-a
	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod, "http://nantian-gw-e2e-gw.nantian-gw.svc.cluster.local/a/hello", 200)

	resp := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod,
		"http://nantian-gw-e2e-gw.nantian-gw.svc.cluster.local/a/hello")
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 from /a/hello, got %d: %s", resp.StatusCode, resp.Body)
	} else {
		t.Logf("PathPrefix /a response: %d %s", resp.StatusCode, resp.Body)
	}

	// Verify routing: PathPrefix /b → echo-b
	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod,
		"http://nantian-gw-e2e-gw.nantian-gw.svc.cluster.local/b/world", 200)

	resp2 := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod,
		"http://nantian-gw-e2e-gw.nantian-gw.svc.cluster.local/b/world")
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 from /b/world, got %d: %s", resp2.StatusCode, resp2.Body)
	} else {
		t.Logf("PathPrefix /b response: %d %s", resp2.StatusCode, resp2.Body)
	}

	// Cleanup
	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, framework.ControlPlaneNS, "echo-route")
	})
}

func TestBasicRoutingPathExact(t *testing.T) {
	ensureNamespace(t, testNSRouting)

	// Deploy echo-exact backend
	framework.DeployEchoBackend(t, testNSRouting, "echo-exact")
	framework.WaitForBackendReady(t, testNSRouting, "echo-exact")

	// Create Gateway if not already present (idempotent)
	framework.CreateGateway(t, "e2e-gw-exact", "nantian-gw")

	// Create ReferenceGrant for cross-ns access
	framework.CreateReferenceGrant(t, testNSRouting, "allow-gw-exact", framework.ControlPlaneNS)

	// Create HTTPRoute with PathExact match
	rules := []map[string]interface{}{
		{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "Exact",
						"value": "/exact",
					},
				},
			},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name":      "echo-exact",
					"namespace": testNSRouting,
					"port":      int64(8080),
				},
			},
		},
	}
	createHTTPRouteCrossNS(t, framework.ControlPlaneNS, "exact-route", "e2e-gw-exact", rules)

	// Deploy smoke-client
	ensureNamespace(t, framework.ControlPlaneNS)
	smokePod := deploySmokeClient(t, framework.ControlPlaneNS)

	// Verify /exact returns 200
	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod,
		"http://nantian-gw-e2e-gw-exact.nantian-gw.svc.cluster.local/exact", 200)

	resp := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod,
		"http://nantian-gw-e2e-gw-exact.nantian-gw.svc.cluster.local/exact")
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 from /exact, got %d: %s", resp.StatusCode, resp.Body)
	} else {
		t.Logf("PathExact /exact response: %d", resp.StatusCode)
	}

	// Verify /exact/sub does NOT return 200 (different match, should not route)
	resp2 := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod,
		"http://nantian-gw-e2e-gw-exact.nantian-gw.svc.cluster.local/exact/sub")
	if resp2.StatusCode == 200 {
		t.Errorf("expected non-200 from /exact/sub, got %d: %s", resp2.StatusCode, resp2.Body)
	} else {
		t.Logf("PathExact /exact/sub correctly returned non-200: %d", resp2.StatusCode)
	}

	// Cleanup
	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, framework.ControlPlaneNS, "exact-route")
	})
}
