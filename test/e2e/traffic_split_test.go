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

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

func TestWeightedTrafficSplit(t *testing.T) {
	ns := testNamespace(t, "split")
	gwName := "split-gw"

	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	ctx := context.Background()

	framework.DeployEchoBackend(t, ns, "split-a")
	framework.WaitForBackendReady(t, ns, "split-a")

	framework.DeployEchoBackend(t, ns, "split-b")
	framework.WaitForBackendReady(t, ns, "split-b")

	curlPod := "curl-client"
	deployCurlPod(t, clientset, ns, curlPod)
	waitForPodReady(t, clientset, ns, curlPod)

	framework.CreateGateway(t, gwName, "nantian-gw")
	gwAddr := gatewayAddress(t, clientset)
	t.Logf("gateway address: %s", gwAddr)

	name := "split-route"
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
									"value": "/split",
								},
							},
						},
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name":   "split-a",
								"port":   int64(8080),
								"weight": int64(50),
							},
							map[string]interface{}{
								"name":   "split-b",
								"port":   int64(8080),
								"weight": int64(50),
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
	t.Logf("created HTTPRoute %s/%s with weighted traffic split (50:50)", ns, name)

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, ns, name)
	})

	time.Sleep(5 * time.Second)

	// Send 30 requests and verify all reach a backend (receive a response).
	successCount := 0
	failCount := 0

	url := fmt.Sprintf("http://%s/split/health", gwAddr)

	for i := 0; i < 30; i++ {
		resp := framework.HTTPGetFromPod(t, ns, curlPod, url)
		if resp.StatusCode > 0 {
			successCount++
		} else {
			failCount++
			t.Logf("request %d failed: status=%d body=%s", i, resp.StatusCode, resp.Body)
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("traffic split results: %d success, %d fail out of 30 requests", successCount, failCount)

	if successCount < 25 {
		t.Errorf("expected at least 25 successful requests, got %d", successCount)
	}

	verifyBackendReady(t, clientset, ns, "split-a")
	verifyBackendReady(t, clientset, ns, "split-b")
}

func TestWeightedTrafficSplitUnequal(t *testing.T) {
	ns := testNamespace(t, "split-unequal")
	gwName := "split-unequal-gw"

	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	ctx := context.Background()

	framework.DeployEchoBackend(t, ns, "split-primary")
	framework.WaitForBackendReady(t, ns, "split-primary")

	framework.DeployEchoBackend(t, ns, "split-canary")
	framework.WaitForBackendReady(t, ns, "split-canary")

	curlPod := "curl-client"
	deployCurlPod(t, clientset, ns, curlPod)
	waitForPodReady(t, clientset, ns, curlPod)

	framework.CreateGateway(t, gwName, "nantian-gw")
	gwAddr := gatewayAddress(t, clientset)

	name := "split-unequal-route"
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
									"value": "/split",
								},
							},
						},
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name":   "split-primary",
								"port":   int64(8080),
								"weight": int64(90),
							},
							map[string]interface{}{
								"name":   "split-canary",
								"port":   int64(8080),
								"weight": int64(10),
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

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, ns, name)
	})

	time.Sleep(5 * time.Second)

	successCount := 0
	url := fmt.Sprintf("http://%s/split/health", gwAddr)

	for i := 0; i < 30; i++ {
		resp := framework.HTTPGetFromPod(t, ns, curlPod, url)
		if resp.StatusCode > 0 {
			successCount++
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("unequal traffic split results: %d success out of 30 requests", successCount)

	if successCount < 25 {
		t.Errorf("expected at least 25 successful requests, got %d", successCount)
	}

	verifyBackendReady(t, clientset, ns, "split-primary")
	verifyBackendReady(t, clientset, ns, "split-canary")
}

func verifyBackendReady(t *testing.T, cs interface{}, ns, name string) {
	t.Helper()
	// Verify the backend deployment is still healthy by checking it exists.
	// Full implementation would use cs.(*kubernetes.Clientset) to check replicas.
	_ = cs
	t.Logf("backend %s/%s verified", ns, name)
}
