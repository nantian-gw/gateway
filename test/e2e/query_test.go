//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

const testNSQuery = "e2e-query"

func TestQueryMatching(t *testing.T) {
	ensureNamespace(t, testNSQuery)

	framework.DeployEchoBackend(t, testNSQuery, "query-a")
	framework.WaitForBackendReady(t, testNSQuery, "query-a")
	framework.DeployEchoBackend(t, testNSQuery, "query-b")
	framework.WaitForBackendReady(t, testNSQuery, "query-b")

	framework.CreateGateway(t, "e2e-gw-query", "nantian-gw")
	framework.CreateReferenceGrant(t, testNSQuery, "allow-gw-query", framework.ControlPlaneNS)

	// Create HTTPRoute in test namespace with cross-namespace backend refs
	rules := []map[string]interface{}{
		{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": "/echo/search",
					},
					"queryParams": []interface{}{
						map[string]interface{}{
							"name":  "version",
							"value": "v1",
						},
					},
				},
			},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name":      "query-a",
					"namespace": testNSQuery,
					"port":      int64(8080),
				},
			},
		},
		{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": "/echo/search",
					},
					"queryParams": []interface{}{
						map[string]interface{}{
							"name":  "version",
							"value": "v2",
						},
					},
				},
			},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name":      "query-b",
					"namespace": testNSQuery,
					"port":      int64(8080),
				},
			},
		},
	}
	createHTTPRouteCrossNS(t, testNSQuery, "query-route", "e2e-gw-query", rules)

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, testNSQuery, "query-route")
	})

	// Get the data plane address for direct access
	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}
	gwAddr := gatewayAddress(t, clientset)
	t.Logf("gateway address: %s", gwAddr)

	// Wait for route to propagate to data plane
	time.Sleep(5 * time.Second)

	// Deploy smoke-client pod in control plane namespace for internal HTTP calls
	ensureNamespace(t, framework.ControlPlaneNS)
	smokePod := deploySmokeClient(t, framework.ControlPlaneNS)

	// Verify query version=v1 routes to query-a
	urlV1 := fmt.Sprintf("http://%s/echo/search?version=v1", gwAddr)
	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod, urlV1, 200)

	resp := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod, urlV1)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 from /echo/search?version=v1, got %d: %s", resp.StatusCode, resp.Body)
	} else {
		t.Logf("query version=v1 route response: %d", resp.StatusCode)
	}

	// Verify query version=v2 routes to query-b
	urlV2 := fmt.Sprintf("http://%s/echo/search?version=v2", gwAddr)
	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod, urlV2, 200)

	resp2 := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod, urlV2)
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 from /echo/search?version=v2, got %d: %s", resp2.StatusCode, resp2.Body)
	} else {
		t.Logf("query version=v2 route response: %d", resp2.StatusCode)
	}
}
