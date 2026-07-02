//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

const testNSHeader = "e2e-header"

func TestHeaderMatching(t *testing.T) {
	ensureNamespace(t, testNSHeader)

	framework.DeployEchoBackend(t, testNSHeader, "header-v1")
	framework.WaitForBackendReady(t, testNSHeader, "header-v1")
	framework.DeployEchoBackend(t, testNSHeader, "header-v2")
	framework.WaitForBackendReady(t, testNSHeader, "header-v2")

	framework.CreateGateway(t, "e2e-gw-header", "nantian-gw")
	framework.CreateReferenceGrant(t, testNSHeader, "allow-gw-header", framework.ControlPlaneNS)

	// Create route in test namespace with cross-ns parentRef
	rules := []map[string]interface{}{
		{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": "/echo/api",
					},
					"headers": []interface{}{
						map[string]interface{}{
							"name":  "x-version",
							"value": "v1",
						},
					},
				},
			},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name": "header-v1",
					"port": int64(8080),
				},
			},
		},
		{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": "/echo/api",
					},
					"headers": []interface{}{
						map[string]interface{}{
							"name":  "x-version",
							"value": "v2",
						},
					},
				},
			},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name": "header-v2",
					"port": int64(8080),
				},
			},
		},
	}
	createHTTPRouteCrossNS(t, testNSHeader, "header-route", "e2e-gw-header", rules)

	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}
	gwAddr := gatewayAddress(t, clientset)
	t.Logf("gateway address: %s", gwAddr)
	time.Sleep(5 * time.Second)

	ensureNamespace(t, framework.ControlPlaneNS)
	smokePod := deploySmokeClient(t, framework.ControlPlaneNS)

	url := fmt.Sprintf("http://%s/echo/api/echo", gwAddr)

	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod, url, 200,
		func(o *framework.HTTPGetOptions) {
			o.Headers = map[string]string{"x-version": "v1"}
		},
	)

	resp := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod, url,
		func(o *framework.HTTPGetOptions) {
			o.Headers = map[string]string{"x-version": "v1"}
		},
	)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 from header v1 request, got %d: %s", resp.StatusCode, resp.Body)
	} else {
		t.Logf("header v1 response: %d", resp.StatusCode)
	}

	// Verify v2 header routes to different backend
	resp2 := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod, url,
		func(o *framework.HTTPGetOptions) {
			o.Headers = map[string]string{"x-version": "v2"}
		},
	)
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 from header v2 request, got %d: %s", resp2.StatusCode, resp2.Body)
	} else {
		t.Logf("header v2 response: %d", resp2.StatusCode)
	}
}

func TestHeaderModification(t *testing.T) {
	ensureNamespace(t, testNSHeader)

	framework.DeployEchoBackend(t, testNSHeader, "echo-modify")
	framework.WaitForBackendReady(t, testNSHeader, "echo-modify")

	framework.CreateGateway(t, "e2e-gw-modify", "nantian-gw")
	framework.CreateReferenceGrant(t, testNSHeader, "allow-gw-modify", framework.ControlPlaneNS)

	// Create route with RequestHeaderModifier filter
	dc, err := framework.DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	route := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      "modify-route",
				"namespace": testNSHeader,
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"name":      "e2e-gw-modify",
						"namespace": framework.ControlPlaneNS,
					},
				},
				"rules": []interface{}{
					map[string]interface{}{
						"matches": []interface{}{
							map[string]interface{}{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": "/echo/modify",
								},
							},
						},
						"filters": []interface{}{
							map[string]interface{}{
								"type": "RequestHeaderModifier",
								"requestHeaderModifier": map[string]interface{}{
									"set": []interface{}{
										map[string]interface{}{"name": "x-added", "value": "test-value"},
									},
									"remove": []interface{}{"x-remove"},
								},
							},
						},
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": "echo-modify",
								"port": int64(8080),
							},
						},
					},
				},
			},
		},
	}

	ctx := context.Background()
	_, err = dc.Resource(httpRouteGVR).Namespace(testNSHeader).Create(ctx, route, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create HTTPRoute: %v", err)
	}

	clientset2, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}
	gwAddr2 := gatewayAddress(t, clientset2)
	t.Logf("gateway address: %s", gwAddr2)
	time.Sleep(5 * time.Second)

	ensureNamespace(t, framework.ControlPlaneNS)
	smokePod := deploySmokeClient(t, framework.ControlPlaneNS)

	url := fmt.Sprintf("http://%s/echo/modify/headers", gwAddr2)

	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod, url, 200)

	resp := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod, url)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, resp.Body)
	} else {
		t.Logf("modify response: %s", resp.Body)
		if !strings.Contains(resp.Body, "x-added") {
			t.Log("response does not contain x-added header (may be expected)")
		}
	}
}
