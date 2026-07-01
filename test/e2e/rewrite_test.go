//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

func TestURLRewrite(t *testing.T) {
	ns := testNamespace(t, "rewrite")
	gwName := "rewrite-gw"

	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	ctx := context.Background()

	// Deploy echo backend.
	framework.DeployEchoBackend(t, ns, "echo")
	framework.WaitForBackendReady(t, ns, "echo")

	// Deploy curl client pod.
	curlPod := "curl-client"
	deployCurlPod(t, clientset, ns, curlPod)
	waitForPodReady(t, clientset, ns, curlPod)

	// Deploy gateway and wait for ready.
	framework.CreateGateway(t, gwName, "nantian-gw")
	gwAddr := gatewayAddress(t, clientset)
	t.Logf("gateway address: %s", gwAddr)

	// Create HTTPRoute with URLRewrite filter using raw unstructured.
	name := "rewrite-route"
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
									"value": "/echo/api",
								},
							},
						},
						"filters": []interface{}{
							map[string]interface{}{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": "/echo",
									},
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
	t.Logf("created HTTPRoute %s/%s with URLRewrite filter", ns, name)

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, ns, name)
	})

	// Give the control plane a moment to reconcile.
	time.Sleep(5 * time.Second)

	// Send request and verify rewrite: /echo/api/hello → /echo/hello
	url := fmt.Sprintf("http://%s/echo/api/hello", gwAddr)
	probeUntilBodyContains(t, ns, curlPod, url, `"path":"/echo/hello"`, 120*time.Second)
}

func TestURLRewriteNonMatchingPath(t *testing.T) {
	ns := testNamespace(t, "rewrite2")
	gwName := "rewrite2-gw"

	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	ctx := context.Background()

	framework.DeployEchoBackend(t, ns, "echo2")
	framework.WaitForBackendReady(t, ns, "echo2")

	curlPod := "curl-client"
	deployCurlPod(t, clientset, ns, curlPod)
	waitForPodReady(t, clientset, ns, curlPod)

	framework.CreateGateway(t, gwName, "nantian-gw")
	gwAddr := gatewayAddress(t, clientset)

	name := "rewrite2-route"
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
									"value": "/echo/api",
								},
							},
						},
						"filters": []interface{}{
							map[string]interface{}{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": "/echo",
									},
								},
							},
						},
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": "echo2",
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

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, ns, name)
	})

	time.Sleep(5 * time.Second)

	// Request to /other should NOT match the /api PathPrefix rule.
	url := fmt.Sprintf("http://%s/echo/other/hello", gwAddr)
	resp := framework.HTTPGetFromPod(t, ns, curlPod, url)
	t.Logf("non-matching path response: status=%d body=%s", resp.StatusCode, resp.Body)

	// Should get a 404 or no matching route response.
	if resp.StatusCode == 200 {
		// 200 means the gateway has a catch-all; that's fine for the gateway,
		// but verify the path was NOT rewritten.
		if strings.Contains(resp.Body, `"path":"/echo`) {
			t.Error("expected non-matching path to not be rewritten")
		}
	}
}

// testNamespace creates a namespace for the test and registers cleanup.
func testNamespace(t *testing.T, prefix string) string {
	t.Helper()

	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	ns := fmt.Sprintf("e2e-%s-%d", prefix, time.Now().UnixNano()%100000)
	ctx := context.Background()

	_, err = clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create namespace %s: %v", ns, err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		if err := clientset.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{}); err != nil {
			t.Logf("cleanup: delete namespace %s: %v", ns, err)
		}
	})

	return ns
}

// deployCurlPod deploys a minimal alpine/curl pod for running HTTP requests.
func deployCurlPod(t *testing.T, clientset *kubernetes.Clientset, ns, name string) {
	t.Helper()

	ctx := context.Background()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{"app": name},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "curl",
					Image:   os.Getenv("CURL_IMAGE"),
					Command: []string{"sleep", "infinity"},
				},
			},
		},
	}
	if pod.Spec.Containers[0].Image == "" {
		pod.Spec.Containers[0].Image = "curlimages/curl:latest"
	}

	_, err := clientset.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create curl pod %s/%s: %v", ns, name, err)
	}
}

// waitForPodReady waits for a pod to be running.
func waitForPodReady(t *testing.T, clientset *kubernetes.Clientset, ns, name string) {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		pod, err := clientset.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if pod.Status.Phase == corev1.PodRunning {
			t.Logf("pod %s/%s is running", ns, name)
			return
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("pod %s/%s did not become ready within timeout", ns, name)
}

// gatewayAddress returns the address of the gateway dataplane for HTTP requests.
func gatewayAddress(t *testing.T, clientset *kubernetes.Clientset) string {
	t.Helper()

	ctx := context.Background()
	pods, err := clientset.CoreV1().Pods(framework.ControlPlaneNS).List(ctx, metav1.ListOptions{
		LabelSelector: "app=nantian-gw-dataplane",
	})
	if err != nil {
		t.Fatalf("list dataplane pods: %v", err)
	}
	if len(pods.Items) == 0 {
		t.Fatal("no dataplane pods found")
	}

	return pods.Items[0].Status.PodIP + ":80"
}

// probeUntilBodyContains retries GET to url until the response body contains substr.
func probeUntilBodyContains(t *testing.T, podNS, podName, url, substr string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp := framework.HTTPGetFromPod(t, podNS, podName, url)
		if resp.StatusCode == 200 && strings.Contains(resp.Body, substr) {
			t.Logf("request to %s returned body containing %q", url, substr)
			return
		}
		if resp.StatusCode == 200 {
			t.Logf("request to %s returned 200 but missing %q: %s", url, substr, resp.Body)
		} else {
			t.Logf("request to %s returned status %d", url, resp.StatusCode)
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("request to %s did not return body containing %q within %v", url, substr, timeout)
}
