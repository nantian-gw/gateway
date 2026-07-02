//go:build e2e

package e2e

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

func TestConformanceReadiness(t *testing.T) {
	dc, err := framework.DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}
	ctx := context.Background()
	if _, err := dc.Resource(gatewayGVRClass).Get(ctx, "nantian-gw", metav1.GetOptions{}); err != nil {
		t.Fatalf("GatewayClass nantian-gw not found: %v", err)
	}
	t.Logf("GatewayClass nantian-gw exists")

	framework.CreateGateway(t, "nantian-gw", "nantian-gw")
	t.Log("conformance environment ready")
}

var gatewayGVRClass = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "gatewayclasses",
}
