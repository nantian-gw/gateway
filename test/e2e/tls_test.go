//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

const testNSTLS = "e2e-tls"

func TestTLSTermination(t *testing.T) {
	ensureNamespace(t, testNSTLS)

	// Read TLS cert and key from testdata.
	certPath := filepath.Join(framework.GatewayRoot(), "test", "testdata", "tls", "server-san.crt")
	keyPath := filepath.Join(framework.GatewayRoot(), "test", "testdata", "tls", "server-san.key")
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read TLS cert %s: %v", certPath, err)
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read TLS key %s: %v", keyPath, err)
	}

	// Create TLS Secret in ControlPlaneNS.
	clientset, err := framework.ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	secretName := "gateway-tls-cert"
	ctx := context.Background()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: framework.ControlPlaneNS,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certBytes,
			"tls.key": keyBytes,
		},
	}

	_, err = clientset.CoreV1().Secrets(framework.ControlPlaneNS).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create TLS secret %s/%s: %v", framework.ControlPlaneNS, secretName, err)
	}
	t.Logf("created TLS Secret %s/%s", framework.ControlPlaneNS, secretName)

	t.Cleanup(func() {
		if err := clientset.CoreV1().Secrets(framework.ControlPlaneNS).Delete(
			context.Background(), secretName, metav1.DeleteOptions{},
		); err != nil {
			t.Logf("cleanup: delete TLS secret %s/%s: %v", framework.ControlPlaneNS, secretName, err)
		}
	})

	// Deploy echo backend.
	framework.DeployEchoBackend(t, testNSTLS, "echo-tls")
	framework.WaitForBackendReady(t, testNSTLS, "echo-tls")

	// Create Gateway with HTTPS listener using raw unstructured.
	dc, err := framework.DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	gwName := "e2e-gw-tls"
	gw := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]interface{}{
				"name":      gwName,
				"namespace": framework.ControlPlaneNS,
			},
			"spec": map[string]interface{}{
				"gatewayClassName": "nantian-gw",
				"listeners": []interface{}{
					map[string]interface{}{
						"name":     "https",
						"port":     int64(443),
						"protocol": "HTTPS",
						"tls": map[string]interface{}{
							"mode": "Terminate",
							"certificateRefs": []interface{}{
								map[string]interface{}{
									"name": secretName,
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = dc.Resource(gatewayGVR).Namespace(framework.ControlPlaneNS).Create(ctx, gw, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create Gateway %s: %v", gwName, err)
	}
	t.Logf("created Gateway %s in %s", gwName, framework.ControlPlaneNS)

	// Wait for Gateway to be accepted and programmed.
	waitForGatewayReady(t, dc, gwName)

	t.Cleanup(func() {
		framework.CleanupResource(t, gatewayGVR, framework.ControlPlaneNS, gwName)
	})

	// Create ReferenceGrant for cross-namespace backend access.
	framework.CreateReferenceGrant(t, testNSTLS, "allow-gw-tls", framework.ControlPlaneNS)

	// Create HTTPRoute for /tls → echo backend.
	framework.CreateHTTPRoute(t, framework.ControlPlaneNS, "tls-route", gwName,
		framework.WithRule("/tls", "PathPrefix", "echo-tls", 8080),
	)

	t.Cleanup(func() {
		framework.CleanupResource(t, httpRouteGVR, framework.ControlPlaneNS, "tls-route")
	})

	// Deploy smoke client for sending HTTS requests.
	ensureNamespace(t, framework.ControlPlaneNS)
	smokePod := deploySmokeClient(t, framework.ControlPlaneNS)

	// Send HTTPS request (curl -k equivalent).
	url := "https://nantian-gw-e2e-gw-tls.nantian-gw.svc.cluster.local/tls/echo"
	framework.ProbeUntil(t, framework.ControlPlaneNS, smokePod, url, 200,
		func(o *framework.HTTPGetOptions) {
			o.Insecure = true
		},
	)

	resp := framework.HTTPGetFromPod(t, framework.ControlPlaneNS, smokePod, url,
		func(o *framework.HTTPGetOptions) {
			o.Insecure = true
		},
	)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 from HTTPS /tls, got %d: %s", resp.StatusCode, resp.Body)
	} else {
		t.Logf("HTTPS /tls returned 200 OK")
	}
}

func TestBackendMTLS(t *testing.T) {
	t.Skip("backend mTLS certificate management not yet implemented in test framework")
}

// waitForGatewayReady polls the Gateway resource until it is Accepted and Programmed.
func waitForGatewayReady(t framework.T, dc dynamic.Interface, name string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(120 * time.Second)

	for time.Now().Before(deadline) {
		gw, err := dc.Resource(gatewayGVR).Namespace(framework.ControlPlaneNS).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		conditions, found, err := unstructured.NestedSlice(gw.Object, "status", "conditions")
		if !found || err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		accepted := false
		programmed := false
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(cond, "type")
			condStatus, _, _ := unstructured.NestedString(cond, "status")
			if condType == "Accepted" && condStatus == "True" {
				accepted = true
			}
			if condType == "Programmed" && condStatus == "True" {
				programmed = true
			}
		}

		if accepted && programmed {
			t.Logf("Gateway %s is accepted and programmed", name)
			return
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("Gateway %s did not become ready within timeout", name)
}
