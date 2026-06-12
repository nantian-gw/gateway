package translator

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestMergeListenerSetListeners(t *testing.T) {
	host := func(h string) *gatewayv1.Hostname {
		v := gatewayv1.Hostname(h)
		return &v
	}

	t.Run("empty ListenerSets", func(t *testing.T) {
		base := []gatewayv1.Listener{
			{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
		}
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		}

		result := mergeListenerSetListeners(gw, base, nil, nil)
		if len(result) != 1 {
			t.Fatalf("nil sets: expected 1 listener, got %d", len(result))
		}

		result = mergeListenerSetListeners(gw, base, []gatewayv1.ListenerSet{}, nil)
		if len(result) != 1 {
			t.Fatalf("empty sets: expected 1 listener, got %d", len(result))
		}
		if result[0].Name != "http" {
			t.Fatalf("expected base listener 'http', got %s", result[0].Name)
		}
	})

	t.Run("no conflicts", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: allowedListenersFromAll(),
			},
		}
		base := []gatewayv1.Listener{
			{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
		}
		sets := []gatewayv1.ListenerSet{
			listenerSet("ls-https", "default", "gw", []gatewayv1.ListenerEntry{
				{Name: "https", Port: 443, Protocol: gatewayv1.HTTPSProtocolType},
			}),
		}

		result := mergeListenerSetListeners(gw, base, sets, nil)

		if len(result) != 2 {
			t.Fatalf("expected 2 listeners, got %d", len(result))
		}
		names := map[gatewayv1.SectionName]bool{}
		for _, l := range result {
			names[l.Name] = true
		}
		if !names["http"] {
			t.Fatal("expected 'http' listener from Gateway")
		}
		if !names["https"] {
			t.Fatal("expected 'https' listener from ListenerSet")
		}
	})

	t.Run("Gateway wins on port conflict", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: allowedListenersFromAll(),
			},
		}
		base := []gatewayv1.Listener{
			{Name: "http-gw", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
		}
		sets := []gatewayv1.ListenerSet{
			listenerSet("ls-extra", "default", "gw", []gatewayv1.ListenerEntry{
				{Name: "http-ls", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
			}),
		}

		result := mergeListenerSetListeners(gw, base, sets, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 listener (Gateway wins), got %d: %v", len(result), listenerNames(result))
		}
		if result[0].Name != "http-gw" {
			t.Fatalf("expected Gateway listener 'http-gw', got %s", result[0].Name)
		}
	})

	t.Run("Gateway wins on protocol conflict", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: allowedListenersFromAll(),
			},
		}
		base := []gatewayv1.Listener{
			{Name: "http-gw", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
		}
		sets := []gatewayv1.ListenerSet{
			listenerSet("ls-extra", "default", "gw", []gatewayv1.ListenerEntry{
				{Name: "tcp-ls", Port: 80, Protocol: gatewayv1.TCPProtocolType},
			}),
		}

		result := mergeListenerSetListeners(gw, base, sets, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 listener (Gateway wins protocol conflict), got %d: %v", len(result), listenerNames(result))
		}
		if result[0].Name != "http-gw" {
			t.Fatalf("expected Gateway listener 'http-gw', got %s", result[0].Name)
		}
	})

	t.Run("ListenerSet with rejected status is skipped", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: allowedListenersFromAll(),
			},
		}
		base := []gatewayv1.Listener{
			{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
		}
		rejected := listenerSet("ls-rejected", "default", "gw", []gatewayv1.ListenerEntry{
			{Name: "https", Port: 443, Protocol: gatewayv1.HTTPSProtocolType},
		})
		rejected.Generation = 2
		rejected.Status.Conditions = []metav1.Condition{{
			Type:               string(gatewayv1.ListenerSetConditionAccepted),
			Status:             metav1.ConditionFalse,
			Reason:             string(gatewayv1.ListenerSetReasonListenersNotValid),
			ObservedGeneration: 2,
		}}

		result := mergeListenerSetListeners(gw, base, []gatewayv1.ListenerSet{rejected}, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 listener (rejected ListenerSet skipped), got %d: %v", len(result), listenerNames(result))
		}
		if result[0].Name != "http" {
			t.Fatalf("expected base listener 'http', got %s", result[0].Name)
		}
	})

	t.Run("hostname different is not a conflict", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: allowedListenersFromAll(),
			},
		}
		base := []gatewayv1.Listener{
			{Name: "http-foo", Port: 80, Protocol: gatewayv1.HTTPProtocolType, Hostname: host("foo.example.com")},
		}
		sets := []gatewayv1.ListenerSet{
			listenerSet("ls-bar", "default", "gw", []gatewayv1.ListenerEntry{
				{Name: "http-bar", Port: 80, Protocol: gatewayv1.HTTPProtocolType, Hostname: host("bar.example.com")},
			}),
		}

		result := mergeListenerSetListeners(gw, base, sets, nil)

		if len(result) != 2 {
			t.Fatalf("expected 2 listeners (different hostnames), got %d", len(result))
		}
	})

	t.Run("older ListenerSet wins", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: allowedListenersFromAll(),
			},
		}
		base := []gatewayv1.Listener{}
		olderTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		newerTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

		sets := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ls-newer",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(newerTime),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{
						{Name: "http-newer", Port: 8080, Protocol: gatewayv1.HTTPProtocolType},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ls-older",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(olderTime),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{
						{Name: "http-older", Port: 8080, Protocol: gatewayv1.HTTPProtocolType},
					},
				},
			},
		}

		result := mergeListenerSetListeners(gw, base, sets, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 listener (older wins), got %d: %v", len(result), listenerNames(result))
		}
		if result[0].Name != "http-older" {
			t.Fatalf("expected older listener 'http-older', got %s", result[0].Name)
		}
	})

	t.Run("alphabetical tiebreaker", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: allowedListenersFromAll(),
			},
		}
		base := []gatewayv1.Listener{}
		sameTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

		sets := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "b-listener-set",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(sameTime),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{
						{Name: "http-b", Port: 9090, Protocol: gatewayv1.HTTPProtocolType},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "a-listener-set",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(sameTime),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{
						{Name: "http-a", Port: 9090, Protocol: gatewayv1.HTTPProtocolType},
					},
				},
			},
		}

		result := mergeListenerSetListeners(gw, base, sets, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 listener (alphabetical wins), got %d: %v", len(result), listenerNames(result))
		}
		if result[0].Name != "http-a" {
			t.Fatalf("expected alphabetical-first listener 'http-a', got %s", result[0].Name)
		}
	})

	t.Run("Gateway skips ListenerSet not allowed by policy", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: fromPtr(gatewayv1.NamespacesFromSame),
					},
				},
			},
		}
		base := []gatewayv1.Listener{
			{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
		}
		sets := []gatewayv1.ListenerSet{
			listenerSet("ls-other", "other-ns", "gw", []gatewayv1.ListenerEntry{
				{Name: "https", Port: 443, Protocol: gatewayv1.HTTPSProtocolType},
			}),
		}

		result := mergeListenerSetListeners(gw, base, sets, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 listener (other-ns skipped), got %d", len(result))
		}
		if result[0].Name != "http" {
			t.Fatalf("expected base listener 'http', got %s", result[0].Name)
		}
	})

	t.Run("multiple ListenerSets non-overlapping", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: allowedListenersFromAll(),
			},
		}
		base := []gatewayv1.Listener{
			{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
		}
		sets := []gatewayv1.ListenerSet{
			listenerSet("ls-https", "default", "gw", []gatewayv1.ListenerEntry{
				{Name: "https", Port: 443, Protocol: gatewayv1.HTTPSProtocolType},
			}),
			listenerSet("ls-grpc", "default", "gw", []gatewayv1.ListenerEntry{
				{Name: "grpc", Port: 9090, Protocol: gatewayv1.HTTPProtocolType},
			}),
		}

		result := mergeListenerSetListeners(gw, base, sets, nil)

		if len(result) != 3 {
			t.Fatalf("expected 3 listeners, got %d", len(result))
		}
	})

	t.Run("ListenerSet preserves TLS config", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: allowedListenersFromAll(),
			},
		}
		base := []gatewayv1.Listener{}
		tlsMode := gatewayv1.TLSModeTerminate
		sets := []gatewayv1.ListenerSet{
			listenerSet("ls-tls", "default", "gw", []gatewayv1.ListenerEntry{
				{
					Name:     "https",
					Port:     443,
					Protocol: gatewayv1.HTTPSProtocolType,
					TLS: &gatewayv1.ListenerTLSConfig{
						Mode: &tlsMode,
						CertificateRefs: []gatewayv1.SecretObjectReference{
							{Name: "my-cert"},
						},
					},
				},
			}),
		}

		result := mergeListenerSetListeners(gw, base, sets, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 listener, got %d", len(result))
		}
		if result[0].TLS == nil {
			t.Fatal("expected TLS config to be preserved")
		}
		if result[0].TLS.Mode == nil || *result[0].TLS.Mode != gatewayv1.TLSModeTerminate {
			t.Fatal("expected TLS mode Terminate")
		}
		if len(result[0].TLS.CertificateRefs) != 1 || result[0].TLS.CertificateRefs[0].Name != "my-cert" {
			t.Fatal("expected certificate ref 'my-cert'")
		}
	})
}

func TestGatewayAllowsListenerSet(t *testing.T) {
	t.Run("All namespaces", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: fromPtr(gatewayv1.NamespacesFromAll),
					},
				},
			},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "other"},
		}

		if !gatewayAllowsListenerSet(gw, ls, nil) {
			t.Fatal("All should allow any namespace")
		}
	})

	t.Run("Same namespace - matching", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: fromPtr(gatewayv1.NamespacesFromSame),
					},
				},
			},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
		}

		if !gatewayAllowsListenerSet(gw, ls, nil) {
			t.Fatal("Same should allow matching namespace")
		}
	})

	t.Run("Same namespace - different", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: fromPtr(gatewayv1.NamespacesFromSame),
					},
				},
			},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "other"},
		}

		if gatewayAllowsListenerSet(gw, ls, nil) {
			t.Fatal("Same should reject different namespace")
		}
	})

	t.Run("Selector - matching labels", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: fromPtr(gatewayv1.NamespacesFromSelector),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
					},
				},
			},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "app-ns"},
		}
		namespaces := map[string]corev1.Namespace{
			"app-ns": {
				ObjectMeta: metav1.ObjectMeta{
					Name:   "app-ns",
					Labels: map[string]string{"env": "prod"},
				},
			},
		}

		if !gatewayAllowsListenerSet(gw, ls, namespaces) {
			t.Fatal("Selector should allow matching label")
		}
	})

	t.Run("Selector - non-matching labels", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: fromPtr(gatewayv1.NamespacesFromSelector),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
					},
				},
			},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "app-ns"},
		}
		namespaces := map[string]corev1.Namespace{
			"app-ns": {
				ObjectMeta: metav1.ObjectMeta{
					Name:   "app-ns",
					Labels: map[string]string{"env": "staging"},
				},
			},
		}

		if gatewayAllowsListenerSet(gw, ls, namespaces) {
			t.Fatal("Selector should reject non-matching label")
		}
	})

	t.Run("Selector - namespace not found", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: fromPtr(gatewayv1.NamespacesFromSelector),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
					},
				},
			},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "missing-ns"},
		}

		if gatewayAllowsListenerSet(gw, ls, nil) {
			t.Fatal("Missing namespace should reject")
		}
	})

	t.Run("Selector - nil selector", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: fromPtr(gatewayv1.NamespacesFromSelector),
					},
				},
			},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
		}

		if gatewayAllowsListenerSet(gw, ls, nil) {
			t.Fatal("Nil selector should reject")
		}
	})

	t.Run("nil AllowedListeners", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
		}

		if gatewayAllowsListenerSet(gw, ls, nil) {
			t.Fatal("nil AllowedListeners should reject")
		}
	})

	t.Run("nil Namespaces", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{},
			},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
		}

		if gatewayAllowsListenerSet(gw, ls, nil) {
			t.Fatal("nil Namespaces should reject")
		}
	})

	t.Run("nil From", func(t *testing.T) {
		gw := gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
			Spec: gatewayv1.GatewaySpec{
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{},
				},
			},
		}
		ls := gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
		}

		if gatewayAllowsListenerSet(gw, ls, nil) {
			t.Fatal("nil From should reject")
		}
	})
}

// --- helpers ---

func listenerSet(name, namespace, gwName string, entries []gatewayv1.ListenerEntry) gatewayv1.ListenerSet {
	return gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayv1.ListenerSetSpec{
			ParentRef: gatewayv1.ParentGatewayReference{
				Name: gatewayv1.ObjectName(gwName),
			},
			Listeners: entries,
		},
	}
}

func allowedListenersFromAll() *gatewayv1.AllowedListeners {
	return &gatewayv1.AllowedListeners{
		Namespaces: &gatewayv1.ListenerNamespaces{
			From: fromPtr(gatewayv1.NamespacesFromAll),
		},
	}
}

func fromPtr(f gatewayv1.FromNamespaces) *gatewayv1.FromNamespaces {
	return &f
}

func listenerNames(listeners []gatewayv1.Listener) []string {
	names := make([]string, len(listeners))
	for i, l := range listeners {
		names[i] = string(l.Name)
	}
	return names
}
