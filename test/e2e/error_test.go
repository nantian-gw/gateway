//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

func TestBackendUnavailable(t *testing.T) {
	ns := testNamespace(t, "error")
	gwName := "error-gw"

	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	ctx := context.Background()

	framework.DeployEchoBackend(t, ns, "echo")
	framework.WaitForBackendReady(t, ns, "echo")

	curlPod := "curl-client"
	deployCurlPod(t, clientset, ns, curlPod)
	waitForPodReady(t, clientset, ns, curlPod)

	framework.CreateGateway(t, gwName, "nantian-gw")
	gwAddr := gatewayAddress(t, clientset)
	t.Logf("gateway address: %s", gwAddr)

	name := "error-route"
	dc, err := framework.DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	route := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"name":      gwName,
						"namespace": framework.ControlPlaneNS,
					},
				},
				"rules": []interface{}{
					map[string]interface{}{
						"matches": []interface{}{
							map[string]interface{}{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": "/echo/error",
								},
							},
						},
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": "echo",
								"port": int64(8080),
							},
						},
					},
				},
			},
		},
	}

	httpRouteGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}
	_, err = dc.Resource(httpRouteGVR).Namespace(ns).Create(ctx, route, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create HTTPRoute %s/%s: %v", ns, name, err)
	}
	t.Logf("created HTTPRoute %s/%s", ns, name)

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, ns, name)
	})

	time.Sleep(5 * time.Second)

	url := fmt.Sprintf("http://%s/echo/error/health", gwAddr)

	// Verify the route works when backend is healthy.
	probeUntilStatus(t, ns, curlPod, url, 200, 60*time.Second)
	t.Log("backend is healthy, route returns 200")

	// Scale backend to zero and wait for health checks to detect.
	framework.ScaleBackendToZero(t, ns, "echo")
	t.Log("scaled backend to 0, waiting for health checks to detect...")

	// Active health checks: interval 3s, unhealthyThreshold 2.
	// Wait for at least 3 health check cycles.
	time.Sleep(15 * time.Second)

	// Verify the route returns a 5xx (503 Service Unavailable).
	resp := framework.HTTPGetFromPod(t, ns, curlPod, url)
	t.Logf("after scale-to-zero: status=%d body=%s", resp.StatusCode, resp.Body)

	if resp.StatusCode == 200 {
		t.Error("expected non-200 status after scaling backend to zero")
	}
	if resp.StatusCode == 0 {
		t.Error("expected a response from gateway (0 means connection failure)")
	}

	// Verify consistent 5xx over multiple retries.
	errorCount := 0
	for i := 0; i < 5; i++ {
		r := framework.HTTPGetFromPod(t, ns, curlPod, url)
		if r.StatusCode >= 500 && r.StatusCode < 600 {
			errorCount++
		} else if r.StatusCode == 200 {
			t.Logf("unexpected 200 after scale-to-zero (attempt %d)", i)
		}
		time.Sleep(1 * time.Second)
	}
	if errorCount < 3 {
		t.Errorf("expected at least 3 5xx responses, got %d", errorCount)
	}
	t.Logf("got %d/5 5xx responses after scale-to-zero", errorCount)

	// Scale back up and verify recovery.
	scaleBackendUp(t, clientset, ns, "echo", 1)
	framework.WaitForBackendReady(t, ns, "echo")
	t.Log("backend scaled back up, verifying recovery")

	probeUntilStatus(t, ns, curlPod, url, 200, 60*time.Second)
	t.Log("backend recovered, route returns 200 again")
}

func scaleBackendUp(t *testing.T, clientset *kubernetes.Clientset, ns, name string, replicas int32) {
	t.Helper()

	ctx := context.Background()
	deploy, err := clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment %s/%s: %v", ns, name, err)
	}

	deploy.Spec.Replicas = &replicas
	_, err = clientset.AppsV1().Deployments(ns).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("scale deployment %s/%s to %d: %v", ns, name, replicas, err)
	}

	t.Logf("scaled backend %s/%s to %d replicas", ns, name, replicas)
}

func probeUntilStatus(t *testing.T, podNS, podName, url string, expectedStatus int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp := framework.HTTPGetFromPod(t, podNS, podName, url)
		if resp.StatusCode == expectedStatus {
			t.Logf("probe %s returned expected status %d", url, expectedStatus)
			return
		}
		t.Logf("probe %s returned status %d, waiting for %d", url, resp.StatusCode, expectedStatus)
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("probe %s did not return status %d within %v", url, expectedStatus, timeout)
}
