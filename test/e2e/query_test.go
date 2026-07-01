//go:build e2e

package e2e

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

	dc, err := framework.DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	rules := []interface{}{
		map[string]interface{}{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": "/search",
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
		map[string]interface{}{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": "/search",
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

	route := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      "query-route",
				"namespace": framework.ControlPlaneNS,
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"name":      "e2e-gw-query",
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
		t.Fatalf("create query HTTPRoute: %v", err)
	}
	t.Logf("created HTTPRoute %s/query-route", framework.ControlPlaneNS)

	ensureNamespace(t, framework.ControlPlaneNS)
	smokePod := deploySmokeClient(t, framework.ControlPlaneNS)

	url := "http://nantian-gw-e2e-gw-query.nantian-gw.svc.cluster.local/search"

	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod, url+"?version=v1", 200)

	resp := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod, url+"?version=v1")
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 from /search?version=v1, got %d: %s", resp.StatusCode, resp.Body)
	} else {
		t.Logf("query version=v1 route response: %d", resp.StatusCode)
	}

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, framework.ControlPlaneNS, "query-route")
	})
}
