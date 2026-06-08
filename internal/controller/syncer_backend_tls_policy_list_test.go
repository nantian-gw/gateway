package controller

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

func TestListBackendTLSPoliciesInNamespaceDoesNotUseTypedV1Alpha3FallbackForProductionReader(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1alpha3.Install)

	syncer := &Syncer{
		client: backendTLSPolicyV1OnlyListClient{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				Build(),
		},
	}

	policies, err := syncer.listBackendTLSPoliciesInNamespace(context.Background(), "default")
	if err != nil {
		t.Fatalf("list BackendTLSPolicies returned error: %v", err)
	}
	if len(policies) != 0 {
		t.Fatalf("expected no BackendTLSPolicies, got %d", len(policies))
	}
}

type backendTLSPolicyV1OnlyListClient struct {
	client.Client
}

func (c backendTLSPolicyV1OnlyListClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if _, ok := list.(*gatewayv1alpha3.BackendTLSPolicyList); ok {
		return fmt.Errorf("typed v1alpha3 BackendTLSPolicy list should not be used")
	}
	return c.Client.List(ctx, list, opts...)
}
