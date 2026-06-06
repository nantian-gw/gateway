package status

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestEvaluateListenerSets(t *testing.T) {
	ns := func(name gatewayv1.Namespace) *gatewayv1.Namespace { return &name }
	from := func(from gatewayv1.FromNamespaces) *gatewayv1.FromNamespaces { return &from }

	t.Run("managed Gateway", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "my-ls",
					Namespace:  "default",
					Generation: 3,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"),
						Name:      "gw1",
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw1",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways)

		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		eval, ok := result["default/my-ls"]
		if !ok {
			t.Fatalf("expected key 'default/my-ls', got %v", result)
		}
		if eval.accepted.Type != string(gatewayv1.ListenerSetConditionAccepted) {
			t.Fatalf("accepted type = %s, want %s", eval.accepted.Type, string(gatewayv1.ListenerSetConditionAccepted))
		}
		if eval.accepted.Status != metav1.ConditionTrue {
			t.Fatalf("accepted status = %s, want True", eval.accepted.Status)
		}
		if eval.accepted.Reason != string(gatewayv1.ListenerSetReasonAccepted) {
			t.Fatalf("accepted reason = %s, want %s", eval.accepted.Reason, string(gatewayv1.ListenerSetReasonAccepted))
		}
		if eval.accepted.Message != "ListenerSet is accepted" {
			t.Fatalf("accepted message = %s, want 'ListenerSet is accepted'", eval.accepted.Message)
		}
		if eval.accepted.ObservedGeneration != 3 {
			t.Fatalf("accepted observedGeneration = %d, want 3", eval.accepted.ObservedGeneration)
		}

		if eval.programmed.Type != string(gatewayv1.ListenerSetConditionProgrammed) {
			t.Fatalf("programmed type = %s, want %s", eval.programmed.Type, string(gatewayv1.ListenerSetConditionProgrammed))
		}
		if eval.programmed.Status != metav1.ConditionTrue {
			t.Fatalf("programmed status = %s, want True", eval.programmed.Status)
		}
		if eval.programmed.Reason != string(gatewayv1.ListenerSetReasonProgrammed) {
			t.Fatalf("programmed reason = %s, want %s", eval.programmed.Reason, string(gatewayv1.ListenerSetReasonProgrammed))
		}
		if eval.programmed.Message != "ListenerSet listeners are programmed" {
			t.Fatalf("programmed message = %s, want 'ListenerSet listeners are programmed'", eval.programmed.Message)
		}
		if eval.programmed.ObservedGeneration != 3 {
			t.Fatalf("programmed observedGeneration = %d, want 3", eval.programmed.ObservedGeneration)
		}
	})

	t.Run("managed Gateway with namespace in ParentRef", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cross-ls",
					Namespace:  "app-ns",
					Generation: 1,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw-cross",
						Namespace: ns("other-ns"),
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"other-ns/gw-cross": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw-cross",
					Namespace: "other-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways)

		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		eval, ok := result["app-ns/cross-ls"]
		if !ok {
			t.Fatalf("expected key 'app-ns/cross-ls', got %v", result)
		}
		if eval.accepted.Type != string(gatewayv1.ListenerSetConditionAccepted) {
			t.Fatalf("accepted type = %s, want %s", eval.accepted.Type, string(gatewayv1.ListenerSetConditionAccepted))
		}
		if eval.accepted.Status != metav1.ConditionTrue {
			t.Fatalf("accepted status = %s, want True", eval.accepted.Status)
		}
	})

	t.Run("unmanaged Gateway", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unmanaged-ls",
					Namespace: "default",
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"),
						Name:      "unmanaged-gw",
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw1",
					Namespace: "default",
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways)

		if len(result) != 0 {
			t.Fatalf("expected 0 results for unmanaged gateway, got %d", len(result))
		}
	})

	t.Run("no managed gateways", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "ls1", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, nil)

		if len(result) != 0 {
			t.Fatalf("expected 0 results with nil managed gateways, got %d", len(result))
		}

		result = evaluateListenerSets(state, lses, map[string]gatewayv1.Gateway{})

		if len(result) != 0 {
			t.Fatalf("expected 0 results with empty managed gateways, got %d", len(result))
		}
	})

	t.Run("empty ListenerSets", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {},
		}

		result := evaluateListenerSets(state, nil, managedGateways)
		if len(result) != 0 {
			t.Fatalf("expected 0 results with nil ListenerSets, got %d", len(result))
		}

		result = evaluateListenerSets(state, []gatewayv1.ListenerSet{}, managedGateways)
		if len(result) != 0 {
			t.Fatalf("expected 0 results with empty ListenerSets, got %d", len(result))
		}
	})

	t.Run("multiple ListenerSets for same Gateway", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "ls-http",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "ls-https",
					Namespace:  "default",
					Generation: 5,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways)

		if len(result) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result))
		}

		eval1, ok := result["default/ls-http"]
		if !ok {
			t.Fatalf("expected key 'default/ls-http', got %v", result)
		}
		if eval1.accepted.ObservedGeneration != 1 {
			t.Fatalf("ls-http observedGeneration = %d, want 1", eval1.accepted.ObservedGeneration)
		}

		eval2, ok := result["default/ls-https"]
		if !ok {
			t.Fatalf("expected key 'default/ls-https', got %v", result)
		}
		if eval2.accepted.ObservedGeneration != 5 {
			t.Fatalf("ls-https observedGeneration = %d, want 5", eval2.accepted.ObservedGeneration)
		}
	})

	t.Run("mixed managed and unmanaged", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-ls",
					Namespace: "default",
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unmanaged-ls",
					Namespace: "default",
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw2",
						Namespace: ns("default"),
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways)

		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		if _, ok := result["default/managed-ls"]; !ok {
			t.Fatalf("expected 'default/managed-ls' in result, got %v", result)
		}
		if _, ok := result["default/unmanaged-ls"]; ok {
			t.Fatal("unmanaged-ls should not be in result")
		}
	})
}