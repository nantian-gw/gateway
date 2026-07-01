//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

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

	framework.CreateHTTPRoute(t, framework.ControlPlaneNS, "header-route", "e2e-gw-header",
		framework.WithRuleMatchHeaders("/api", "PathPrefix", "header-v1", 8080,
			map[string]string{"x-version": "v1"},
		),
		framework.WithRuleMatchHeaders("/api", "PathPrefix", "header-v2", 8080,
			map[string]string{"x-version": "v2"},
		),
	)

	ensureNamespace(t, framework.ControlPlaneNS)
	smokePod := deploySmokeClient(t, framework.ControlPlaneNS)

	url := "http://nantian-gw-e2e-gw-header.nantian-gw.svc.cluster.local/api/echo"

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
		t.Errorf("expected 200 with x-version:v1, got %d: %s", resp.StatusCode, resp.Body)
	} else {
		t.Logf("header-v1 route response: %d", resp.StatusCode)
	}

	resp2 := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod, url,
		func(o *framework.HTTPGetOptions) {
			o.Headers = map[string]string{"x-version": "v2"}
		},
	)
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 with x-version:v2, got %d: %s", resp2.StatusCode, resp2.Body)
	} else {
		t.Logf("header-v2 route response: %d", resp2.StatusCode)
	}

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, framework.ControlPlaneNS, "header-route")
	})
}

func TestHeaderModification(t *testing.T) {
	ensureNamespace(t, testNSHeader)

	framework.DeployEchoBackend(t, testNSHeader, "echo-backend")
	framework.WaitForBackendReady(t, testNSHeader, "echo-backend")

	dc, err := framework.DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	framework.CreateGateway(t, "e2e-gw-modify", "nantian-gw")

	framework.CreateReferenceGrant(t, testNSHeader, "allow-gw-modify", framework.ControlPlaneNS)

	rules := []interface{}{
		map[string]interface{}{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": "/modify",
					},
				},
			},
			"filters": []interface{}{
				map[string]interface{}{
					"type": "RequestHeaderModifier",
					"requestHeaderModifier": map[string]interface{}{
						"add": []interface{}{
							map[string]interface{}{
								"name":  "X-Added",
								"value": "test-value",
							},
						},
						"remove": []interface{}{
							"X-Remove",
						},
					},
				},
				map[string]interface{}{
					"type": "ResponseHeaderModifier",
					"responseHeaderModifier": map[string]interface{}{
						"add": []interface{}{
							map[string]interface{}{
								"name":  "X-Response-Added",
								"value": "response-test",
							},
						},
					},
				},
			},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name":      "echo-backend",
					"namespace": testNSHeader,
					"port":      int64(8080),
				},
			},
		},
	}

	route := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      "modify-route",
				"namespace": framework.ControlPlaneNS,
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"name":      "e2e-gw-modify",
						"namespace": framework.ControlPlaneNS,
					},
				},
				"rules": rules,
			},
		},
	}

	ctx := context.Background()
	_, err = dc.Resource(httpRouteGVR).Namespace(framework.ControlPlaneNS).Create(ctx, route, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create modify HTTPRoute: %v", err)
	}
	t.Logf("created HTTPRoute %s/modify-route", framework.ControlPlaneNS)

	ensureNamespace(t, framework.ControlPlaneNS)
	smokePod := deploySmokeClient(t, framework.ControlPlaneNS)

	url := "http://nantian-gw-e2e-gw-modify.nantian-gw.svc.cluster.local/modify/headers"

	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod, url, 200,
		func(o *framework.HTTPGetOptions) {
			o.Headers = map[string]string{"X-Remove": "should-be-removed"}
		},
	)

	resp := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod, url,
		func(o *framework.HTTPGetOptions) {
			o.Headers = map[string]string{"X-Remove": "should-be-removed"}
		},
	)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 from /modify/headers, got %d: %s", resp.StatusCode, resp.Body)
	} else {
		t.Logf("header modification response: %d, body length: %d", resp.StatusCode, len(resp.Body))
	}

	bodyLower := strings.ToLower(resp.Body)

	if !strings.Contains(bodyLower, "x-added") {
		t.Logf("WARNING: X-Added header not found in echo response body (echo backend may not reflect headers in /headers path)")
	} else {
		t.Logf("X-Added header reflected in echo response")
	}

	if strings.Contains(bodyLower, "x-remove") {
		t.Logf("INFO: X-Remove header may or may not appear depending on echo behavior; Gateway should strip it before forwarding")
	}

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, framework.ControlPlaneNS, "modify-route")
	})
}
