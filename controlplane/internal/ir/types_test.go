package ir

import (
	"math/rand"
	"testing"
	"time"
)

func TestSnapshotNormalizeStabilizesDigestAcrossOrderingDifferences(t *testing.T) {
	left := Snapshot{
		Listeners: []Listener{
			{
				Name:           "default/gw/http",
				Address:        "0.0.0.0",
				Addresses:      []string{"192.0.2.20", "192.0.2.10"},
				Port:           80,
				Protocol:       "HTTP",
				Hostnames:      []string{"b.example.com", "a.example.com"},
				AttachedRoutes: []string{"default/b", "default/a"},
				Status: &ListenerStatus{
					Conditions: []ConditionStatus{
						{Type: "Programmed", Status: "True"},
						{Type: "Accepted", Status: "True"},
					},
				},
			},
			{
				Name:     "default/gw/https",
				Address:  "0.0.0.0",
				Port:     443,
				Protocol: "HTTPS",
			},
		},
		HTTPRoutes: []HTTPRoute{
			{
				Name:      "b-route",
				Namespace: "default",
				Hostnames: []string{"b.example.com", "a.example.com"},
				ParentRefs: []ParentRef{
					{Name: "gw", Namespace: "default", SectionName: "http", Port: 80},
					{Name: "gw", Namespace: "default", SectionName: "https", Port: 443},
				},
				Rules: []HTTPRule{
					{
						Matches: []HTTPMatch{
							{
								Headers: []HeaderMatch{
									{Name: "x-b", Value: "2"},
									{Name: "x-a", Value: "1"},
								},
								QueryParams: []QueryMatch{
									{Name: "q-b", Value: "2"},
									{Name: "q-a", Value: "1"},
								},
							},
						},
					},
				},
				Status: &RouteStatus{
					Parents: []RouteParentStatus{
						{
							ControllerName: "c2",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "https"},
							Conditions: []ConditionStatus{
								{Type: "ResolvedRefs", Status: "True"},
								{Type: "Accepted", Status: "True"},
							},
						},
						{
							ControllerName: "c1",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "http"},
						},
					},
				},
			},
			{
				Name:      "a-route",
				Namespace: "default",
			},
		},
		GRPCRoutes: []GRPCRoute{
			{
				Name:      "grpc",
				Namespace: "default",
				Hostnames: []string{"z.example.com", "a.example.com"},
				ParentRefs: []ParentRef{
					{Name: "gw", Namespace: "default", SectionName: "https"},
					{Name: "gw", Namespace: "default", SectionName: "http"},
				},
				Rules: []GRPCRule{
					{
						Matches: []GRPCMatch{
							{
								Headers: []HeaderMatch{
									{Name: "x-b", Value: "2"},
									{Name: "x-a", Value: "1"},
								},
							},
						},
					},
				},
			},
		},
		StreamRoutes: []StreamRoute{
			{Name: "tls-route", Namespace: "default", Kind: "TLS", ParentRefs: []ParentRef{{Name: "gw", Namespace: "default", SectionName: "tls"}, {Name: "gw", Namespace: "default", SectionName: "passthrough"}}},
			{Name: "tcp-route", Namespace: "default", Kind: "TCP"},
		},
		Backends: []BackendCluster{
			{
				Name:      "echo:8080",
				Namespace: "default",
				Protocol:  "HTTP",
				Endpoints: []BackendEndpoint{
					{Address: "10.0.0.2", Port: 8080, Healthy: true},
					{Address: "10.0.0.1", Port: 8080, Healthy: true},
				},
			},
			{
				Name:      "grpc:9090",
				Namespace: "default",
				Protocol:  "GRPC",
			},
		},
		Secrets: []SecretMaterial{
			{Namespace: "default", Name: "b"},
			{Namespace: "default", Name: "a"},
		},
		Workloads: []Workload{
			{Namespace: "default", Name: "b", IP: "10.0.0.2"},
			{Namespace: "default", Name: "a", IP: "10.0.0.1"},
		},
	}

	right := Snapshot{
		Listeners: []Listener{
			{
				Name:     "default/gw/https",
				Address:  "0.0.0.0",
				Port:     443,
				Protocol: "HTTPS",
			},
			{
				Name:           "default/gw/http",
				Address:        "0.0.0.0",
				Addresses:      []string{"192.0.2.10", "192.0.2.20"},
				Port:           80,
				Protocol:       "HTTP",
				Hostnames:      []string{"a.example.com", "b.example.com"},
				AttachedRoutes: []string{"default/a", "default/b"},
				Status: &ListenerStatus{
					Conditions: []ConditionStatus{
						{Type: "Accepted", Status: "True"},
						{Type: "Programmed", Status: "True"},
					},
				},
			},
		},
		HTTPRoutes: []HTTPRoute{
			{
				Name:      "a-route",
				Namespace: "default",
			},
			{
				Name:      "b-route",
				Namespace: "default",
				Hostnames: []string{"a.example.com", "b.example.com"},
				ParentRefs: []ParentRef{
					{Name: "gw", Namespace: "default", SectionName: "https", Port: 443},
					{Name: "gw", Namespace: "default", SectionName: "http", Port: 80},
				},
				Rules: []HTTPRule{
					{
						Matches: []HTTPMatch{
							{
								Headers: []HeaderMatch{
									{Name: "x-a", Value: "1"},
									{Name: "x-b", Value: "2"},
								},
								QueryParams: []QueryMatch{
									{Name: "q-a", Value: "1"},
									{Name: "q-b", Value: "2"},
								},
							},
						},
					},
				},
				Status: &RouteStatus{
					Parents: []RouteParentStatus{
						{
							ControllerName: "c1",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "http"},
						},
						{
							ControllerName: "c2",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "https"},
							Conditions: []ConditionStatus{
								{Type: "Accepted", Status: "True"},
								{Type: "ResolvedRefs", Status: "True"},
							},
						},
					},
				},
			},
		},
		GRPCRoutes: []GRPCRoute{
			{
				Name:      "grpc",
				Namespace: "default",
				Hostnames: []string{"a.example.com", "z.example.com"},
				ParentRefs: []ParentRef{
					{Name: "gw", Namespace: "default", SectionName: "http"},
					{Name: "gw", Namespace: "default", SectionName: "https"},
				},
				Rules: []GRPCRule{
					{
						Matches: []GRPCMatch{
							{
								Headers: []HeaderMatch{
									{Name: "x-a", Value: "1"},
									{Name: "x-b", Value: "2"},
								},
							},
						},
					},
				},
			},
		},
		StreamRoutes: []StreamRoute{
			{Name: "tcp-route", Namespace: "default", Kind: "TCP"},
			{Name: "tls-route", Namespace: "default", Kind: "TLS", ParentRefs: []ParentRef{{Name: "gw", Namespace: "default", SectionName: "passthrough"}, {Name: "gw", Namespace: "default", SectionName: "tls"}}},
		},
		Backends: []BackendCluster{
			{
				Name:      "grpc:9090",
				Namespace: "default",
				Protocol:  "GRPC",
			},
			{
				Name:      "echo:8080",
				Namespace: "default",
				Protocol:  "HTTP",
				Endpoints: []BackendEndpoint{
					{Address: "10.0.0.1", Port: 8080, Healthy: true},
					{Address: "10.0.0.2", Port: 8080, Healthy: true},
				},
			},
		},
		Secrets: []SecretMaterial{
			{Namespace: "default", Name: "a"},
			{Namespace: "default", Name: "b"},
		},
		Workloads: []Workload{
			{Namespace: "default", Name: "a", IP: "10.0.0.1"},
			{Namespace: "default", Name: "b", IP: "10.0.0.2"},
		},
	}

	if err := left.Normalize(); err != nil {
		t.Fatalf("normalize left snapshot: %v", err)
	}
	if err := right.Normalize(); err != nil {
		t.Fatalf("normalize right snapshot: %v", err)
	}

	if left.ID != right.ID {
		t.Fatalf("expected stable digest, got %q and %q", left.ID, right.ID)
	}
}

func TestSnapshotDigestIgnoresStatusSummaries(t *testing.T) {
	left := Snapshot{
		Listeners: []Listener{{
			Name:           "default/gw/http",
			Address:        "0.0.0.0",
			Port:           80,
			Protocol:       "HTTP",
			AttachedRoutes: []string{"default/route"},
			Status: &ListenerStatus{
				AttachedRoutes: 1,
				Conditions: []ConditionStatus{{
					Type:               "Accepted",
					Status:             "True",
					ObservedGeneration: 1,
					LastTransitionTime: time.Unix(10, 0).UTC(),
				}},
			},
		}},
		HTTPRoutes: []HTTPRoute{{
			Name:      "route",
			Namespace: "default",
			Status: &RouteStatus{
				Parents: []RouteParentStatus{{
					ControllerName: "gateway.networking.k8s.io/aether-gateway",
					ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "http"},
					Conditions: []ConditionStatus{{
						Type:               "Accepted",
						Status:             "True",
						ObservedGeneration: 1,
						LastTransitionTime: time.Unix(10, 0).UTC(),
					}},
				}},
			},
		}},
	}

	right := Snapshot{
		Listeners: []Listener{{
			Name:           "default/gw/http",
			Address:        "0.0.0.0",
			Port:           80,
			Protocol:       "HTTP",
			AttachedRoutes: []string{"default/route"},
			Status: &ListenerStatus{
				AttachedRoutes: 7,
				Conditions: []ConditionStatus{{
					Type:               "Accepted",
					Status:             "False",
					Reason:             "Pending",
					Message:            "listener status is still converging",
					ObservedGeneration: 9,
					LastTransitionTime: time.Unix(20, 0).UTC(),
				}},
			},
		}},
		HTTPRoutes: []HTTPRoute{{
			Name:      "route",
			Namespace: "default",
			Status: &RouteStatus{
				Parents: []RouteParentStatus{{
					ControllerName: "gateway.networking.k8s.io/aether-gateway",
					ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "http"},
					Conditions: []ConditionStatus{{
						Type:               "Accepted",
						Status:             "False",
						Reason:             "Pending",
						Message:            "route status is still converging",
						ObservedGeneration: 9,
						LastTransitionTime: time.Unix(20, 0).UTC(),
					}},
				}},
			},
		}},
	}

	if err := left.Normalize(); err != nil {
		t.Fatalf("normalize left snapshot: %v", err)
	}
	if err := right.Normalize(); err != nil {
		t.Fatalf("normalize right snapshot: %v", err)
	}

	if left.ID != right.ID {
		t.Fatalf("expected status-only changes to be ignored, got %q and %q", left.ID, right.ID)
	}

	if left.Listeners[0].Status == nil || right.Listeners[0].Status == nil {
		t.Fatal("expected normalize to preserve status fields on the stored snapshot")
	}
}

func TestSnapshotNormalizeStableAcrossPermutationsProperty(t *testing.T) {
	expected := snapshotPropertyFixture()
	if err := expected.Normalize(); err != nil {
		t.Fatalf("normalize expected snapshot: %v", err)
	}

	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 64; i++ {
		seed := rng.Uint64()
		shuffled := shuffledSnapshotForProperty(snapshotPropertyFixture(), seed)
		if err := shuffled.Normalize(); err != nil {
			t.Fatalf("normalize shuffled snapshot for seed %d: %v", seed, err)
		}
		if expected.ID != shuffled.ID {
			t.Fatalf("seed %d produced different digest: want %s, got %s", seed, expected.ID, shuffled.ID)
		}
	}
}

func TestSnapshotDigestReturnsNormalizedIDWithoutRehashing(t *testing.T) {
	snapshot := snapshotPropertyFixture()
	if err := snapshot.Normalize(); err != nil {
		t.Fatalf("normalize snapshot: %v", err)
	}

	want := snapshot.ID
	snapshot.Listeners[0], snapshot.Listeners[1] = snapshot.Listeners[1], snapshot.Listeners[0]

	got, err := snapshot.Digest()
	if err != nil {
		t.Fatalf("digest snapshot: %v", err)
	}
	if got != want {
		t.Fatalf("digest = %q, want normalized ID %q", got, want)
	}
}

func TestSnapshotNormalizeRefreshesDigestAfterMutation(t *testing.T) {
	snapshot := snapshotPropertyFixture()
	if err := snapshot.Normalize(); err != nil {
		t.Fatalf("normalize snapshot: %v", err)
	}

	before := snapshot.ID
	snapshot.Listeners[0].Hostnames = append(snapshot.Listeners[0].Hostnames, "z.example.com")

	if err := snapshot.Normalize(); err != nil {
		t.Fatalf("re-normalize snapshot: %v", err)
	}
	if snapshot.ID == before {
		t.Fatalf("expected digest to change after content mutation, still %q", snapshot.ID)
	}
}

func TestSnapshotDigestComputesWhenIDMissing(t *testing.T) {
	snapshot := snapshotPropertyFixture()
	if err := snapshot.Normalize(); err != nil {
		t.Fatalf("normalize snapshot: %v", err)
	}
	want := snapshot.ID
	snapshot.ID = ""

	got, err := snapshot.Digest()
	if err != nil {
		t.Fatalf("digest snapshot: %v", err)
	}
	if got != want {
		t.Fatalf("digest = %q, want computed digest %q", got, want)
	}
}

func snapshotPropertyFixture() *Snapshot {
	return &Snapshot{
		GeneratedAt: time.Unix(1_700_000_000, 123_000_000).UTC(),
		Listeners: []Listener{
			{
				Name:           "default/gw/https",
				Address:        "0.0.0.0",
				Addresses:      []string{"192.0.2.20", "192.0.2.10"},
				Port:           443,
				Protocol:       "HTTPS",
				Hostnames:      []string{"b.example.com", "a.example.com"},
				AttachedRoutes: []string{"default/b", "default/a"},
				Status: &ListenerStatus{
					Conditions: []ConditionStatus{
						{Type: "Programmed", Status: "True"},
						{Type: "Accepted", Status: "True"},
					},
				},
			},
			{
				Name:           "default/gw/http",
				Address:        "0.0.0.0",
				Addresses:      []string{"198.51.100.20", "198.51.100.10"},
				Port:           80,
				Protocol:       "HTTP",
				Hostnames:      []string{"d.example.com", "c.example.com"},
				AttachedRoutes: []string{"default/d", "default/c"},
				Status: &ListenerStatus{
					Conditions: []ConditionStatus{
						{Type: "ResolvedRefs", Status: "True"},
						{Type: "Accepted", Status: "True"},
					},
				},
			},
		},
		HTTPRoutes: []HTTPRoute{
			{
				Name:      "b-route",
				Namespace: "default",
				Hostnames: []string{"b.example.com", "a.example.com"},
				ParentRefs: []ParentRef{
					{Name: "gw", Namespace: "default", SectionName: "https", Port: 443},
					{Name: "gw", Namespace: "default", SectionName: "http", Port: 80},
				},
				Rules: []HTTPRule{{
					Matches: []HTTPMatch{{
						Headers: []HeaderMatch{
							{Name: "x-b", Value: "2"},
							{Name: "x-a", Value: "1"},
						},
						QueryParams: []QueryMatch{
							{Name: "q-b", Value: "2"},
							{Name: "q-a", Value: "1"},
						},
					}},
				}},
				Status: &RouteStatus{
					Parents: []RouteParentStatus{
						{
							ControllerName: "c2",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "https"},
							Conditions: []ConditionStatus{
								{Type: "ResolvedRefs", Status: "True"},
								{Type: "Accepted", Status: "True"},
							},
						},
						{
							ControllerName: "c1",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "http"},
						},
					},
				},
			},
			{
				Name:      "a-route",
				Namespace: "default",
			},
		},
		GRPCRoutes: []GRPCRoute{
			{
				Name:      "grpc",
				Namespace: "default",
				Hostnames: []string{"z.example.com", "a.example.com"},
				ParentRefs: []ParentRef{
					{Name: "gw", Namespace: "default", SectionName: "https"},
					{Name: "gw", Namespace: "default", SectionName: "http"},
				},
				Rules: []GRPCRule{{
					Matches: []GRPCMatch{{
						Headers: []HeaderMatch{
							{Name: "x-b", Value: "2"},
							{Name: "x-a", Value: "1"},
						},
					}},
				}},
				Status: &RouteStatus{
					Parents: []RouteParentStatus{
						{
							ControllerName: "c2",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "https"},
							Conditions: []ConditionStatus{
								{Type: "ResolvedRefs", Status: "True"},
								{Type: "Accepted", Status: "True"},
							},
						},
						{
							ControllerName: "c1",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "http"},
						},
					},
				},
			},
		},
		StreamRoutes: []StreamRoute{
			{
				Name:      "tls-route",
				Namespace: "default",
				Kind:      "TLS",
				ParentRefs: []ParentRef{
					{Name: "gw", Namespace: "default", SectionName: "tls"},
					{Name: "gw", Namespace: "default", SectionName: "passthrough"},
				},
				Status: &RouteStatus{
					Parents: []RouteParentStatus{
						{
							ControllerName: "c2",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "tls"},
							Conditions: []ConditionStatus{
								{Type: "Accepted", Status: "True"},
								{Type: "ResolvedRefs", Status: "True"},
							},
						},
						{
							ControllerName: "c1",
							ParentRef:      ParentRef{Name: "gw", Namespace: "default", SectionName: "passthrough"},
						},
					},
				},
			},
			{Name: "tcp-route", Namespace: "default", Kind: "TCP"},
		},
		Backends: []BackendCluster{
			{
				Name:      "echo:8080",
				Namespace: "default",
				Protocol:  "HTTP",
				Endpoints: []BackendEndpoint{
					{Address: "10.0.0.2", Port: 8080, Healthy: true},
					{Address: "10.0.0.1", Port: 8080, Healthy: true},
				},
			},
			{
				Name:      "grpc:9090",
				Namespace: "default",
				Protocol:  "GRPC",
				Endpoints: []BackendEndpoint{
					{Address: "10.0.1.2", Port: 9090, Healthy: true},
					{Address: "10.0.1.1", Port: 9090, Healthy: true},
				},
			},
		},
		Secrets: []SecretMaterial{
			{Namespace: "default", Name: "b"},
			{Namespace: "default", Name: "a"},
		},
		Workloads: []Workload{
			{Namespace: "default", Name: "b", IP: "10.0.0.2"},
			{Namespace: "default", Name: "a", IP: "10.0.0.1"},
		},
	}
}

func shuffledSnapshotForProperty(base *Snapshot, seed uint64) *Snapshot {
	out := base.Clone()
	rng := rand.New(rand.NewSource(int64(seed)))

	shuffleListenersForProperty(rng, out.Listeners)
	shuffleHTTPRoutesForProperty(rng, out.HTTPRoutes)
	shuffleGRPCRoutesForProperty(rng, out.GRPCRoutes)
	shuffleStreamRoutesForProperty(rng, out.StreamRoutes)
	shuffleBackendsForProperty(rng, out.Backends)
	shuffleSecretMaterialsForProperty(rng, out.Secrets)
	shuffleWorkloadsForProperty(rng, out.Workloads)

	return out
}

func shuffleListenersForProperty(rng *rand.Rand, items []Listener) {
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	for idx := range items {
		rng.Shuffle(len(items[idx].Addresses), func(i, j int) {
			items[idx].Addresses[i], items[idx].Addresses[j] = items[idx].Addresses[j], items[idx].Addresses[i]
		})
		rng.Shuffle(len(items[idx].Hostnames), func(i, j int) {
			items[idx].Hostnames[i], items[idx].Hostnames[j] = items[idx].Hostnames[j], items[idx].Hostnames[i]
		})
		rng.Shuffle(len(items[idx].AttachedRoutes), func(i, j int) {
			items[idx].AttachedRoutes[i], items[idx].AttachedRoutes[j] = items[idx].AttachedRoutes[j], items[idx].AttachedRoutes[i]
		})
		if items[idx].Status != nil {
			rng.Shuffle(len(items[idx].Status.Conditions), func(i, j int) {
				items[idx].Status.Conditions[i], items[idx].Status.Conditions[j] = items[idx].Status.Conditions[j], items[idx].Status.Conditions[i]
			})
		}
	}
}

func shuffleHTTPRoutesForProperty(rng *rand.Rand, items []HTTPRoute) {
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	for idx := range items {
		rng.Shuffle(len(items[idx].Hostnames), func(i, j int) {
			items[idx].Hostnames[i], items[idx].Hostnames[j] = items[idx].Hostnames[j], items[idx].Hostnames[i]
		})
		rng.Shuffle(len(items[idx].ParentRefs), func(i, j int) {
			items[idx].ParentRefs[i], items[idx].ParentRefs[j] = items[idx].ParentRefs[j], items[idx].ParentRefs[i]
		})
		for ruleIdx := range items[idx].Rules {
			for matchIdx := range items[idx].Rules[ruleIdx].Matches {
				rng.Shuffle(len(items[idx].Rules[ruleIdx].Matches[matchIdx].Headers), func(i, j int) {
					items[idx].Rules[ruleIdx].Matches[matchIdx].Headers[i], items[idx].Rules[ruleIdx].Matches[matchIdx].Headers[j] = items[idx].Rules[ruleIdx].Matches[matchIdx].Headers[j], items[idx].Rules[ruleIdx].Matches[matchIdx].Headers[i]
				})
				rng.Shuffle(len(items[idx].Rules[ruleIdx].Matches[matchIdx].QueryParams), func(i, j int) {
					items[idx].Rules[ruleIdx].Matches[matchIdx].QueryParams[i], items[idx].Rules[ruleIdx].Matches[matchIdx].QueryParams[j] = items[idx].Rules[ruleIdx].Matches[matchIdx].QueryParams[j], items[idx].Rules[ruleIdx].Matches[matchIdx].QueryParams[i]
				})
			}
		}
		if items[idx].Status != nil {
			rng.Shuffle(len(items[idx].Status.Parents), func(i, j int) {
				items[idx].Status.Parents[i], items[idx].Status.Parents[j] = items[idx].Status.Parents[j], items[idx].Status.Parents[i]
			})
			for parentIdx := range items[idx].Status.Parents {
				rng.Shuffle(len(items[idx].Status.Parents[parentIdx].Conditions), func(i, j int) {
					items[idx].Status.Parents[parentIdx].Conditions[i], items[idx].Status.Parents[parentIdx].Conditions[j] = items[idx].Status.Parents[parentIdx].Conditions[j], items[idx].Status.Parents[parentIdx].Conditions[i]
				})
			}
		}
	}
}

func shuffleGRPCRoutesForProperty(rng *rand.Rand, items []GRPCRoute) {
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	for idx := range items {
		rng.Shuffle(len(items[idx].Hostnames), func(i, j int) {
			items[idx].Hostnames[i], items[idx].Hostnames[j] = items[idx].Hostnames[j], items[idx].Hostnames[i]
		})
		rng.Shuffle(len(items[idx].ParentRefs), func(i, j int) {
			items[idx].ParentRefs[i], items[idx].ParentRefs[j] = items[idx].ParentRefs[j], items[idx].ParentRefs[i]
		})
		for ruleIdx := range items[idx].Rules {
			for matchIdx := range items[idx].Rules[ruleIdx].Matches {
				rng.Shuffle(len(items[idx].Rules[ruleIdx].Matches[matchIdx].Headers), func(i, j int) {
					items[idx].Rules[ruleIdx].Matches[matchIdx].Headers[i], items[idx].Rules[ruleIdx].Matches[matchIdx].Headers[j] = items[idx].Rules[ruleIdx].Matches[matchIdx].Headers[j], items[idx].Rules[ruleIdx].Matches[matchIdx].Headers[i]
				})
			}
		}
		if items[idx].Status != nil {
			rng.Shuffle(len(items[idx].Status.Parents), func(i, j int) {
				items[idx].Status.Parents[i], items[idx].Status.Parents[j] = items[idx].Status.Parents[j], items[idx].Status.Parents[i]
			})
			for parentIdx := range items[idx].Status.Parents {
				rng.Shuffle(len(items[idx].Status.Parents[parentIdx].Conditions), func(i, j int) {
					items[idx].Status.Parents[parentIdx].Conditions[i], items[idx].Status.Parents[parentIdx].Conditions[j] = items[idx].Status.Parents[parentIdx].Conditions[j], items[idx].Status.Parents[parentIdx].Conditions[i]
				})
			}
		}
	}
}

func shuffleStreamRoutesForProperty(rng *rand.Rand, items []StreamRoute) {
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	for idx := range items {
		rng.Shuffle(len(items[idx].ParentRefs), func(i, j int) {
			items[idx].ParentRefs[i], items[idx].ParentRefs[j] = items[idx].ParentRefs[j], items[idx].ParentRefs[i]
		})
		if items[idx].Status != nil {
			rng.Shuffle(len(items[idx].Status.Parents), func(i, j int) {
				items[idx].Status.Parents[i], items[idx].Status.Parents[j] = items[idx].Status.Parents[j], items[idx].Status.Parents[i]
			})
			for parentIdx := range items[idx].Status.Parents {
				rng.Shuffle(len(items[idx].Status.Parents[parentIdx].Conditions), func(i, j int) {
					items[idx].Status.Parents[parentIdx].Conditions[i], items[idx].Status.Parents[parentIdx].Conditions[j] = items[idx].Status.Parents[parentIdx].Conditions[j], items[idx].Status.Parents[parentIdx].Conditions[i]
				})
			}
		}
	}
}

func shuffleBackendsForProperty(rng *rand.Rand, items []BackendCluster) {
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	for idx := range items {
		rng.Shuffle(len(items[idx].Endpoints), func(i, j int) {
			items[idx].Endpoints[i], items[idx].Endpoints[j] = items[idx].Endpoints[j], items[idx].Endpoints[i]
		})
	}
}

func shuffleSecretMaterialsForProperty(rng *rand.Rand, items []SecretMaterial) {
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
}

func shuffleWorkloadsForProperty(rng *rand.Rand, items []Workload) {
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
}
