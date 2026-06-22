package gwapi

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestEffectiveListenersKeepsUnresolvedAndPendingListeners(t *testing.T) {
	gateway := gatewayv1.Gateway{
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{Name: "accepted-false", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
				{Name: "resolved-false", Protocol: gatewayv1.HTTPSProtocolType, Port: 443},
				{Name: "programmed-false", Protocol: gatewayv1.TCPProtocolType, Port: 9000},
				{Name: "no-status", Protocol: gatewayv1.UDPProtocolType, Port: 5300},
			},
		},
		Status: gatewayv1.GatewayStatus{
			Listeners: []gatewayv1.ListenerStatus{
				{
					Name: "accepted-false",
					Conditions: []metav1.Condition{{
						Type:   string(gatewayv1.ListenerConditionAccepted),
						Status: metav1.ConditionFalse,
					}},
				},
				{
					Name: "resolved-false",
					Conditions: []metav1.Condition{{
						Type:   string(gatewayv1.ListenerConditionResolvedRefs),
						Status: metav1.ConditionFalse,
					}},
				},
				{
					Name: "programmed-false",
					Conditions: []metav1.Condition{{
						Type:   string(gatewayv1.ListenerConditionProgrammed),
						Status: metav1.ConditionFalse,
					}},
				},
			},
		},
	}

	listeners := EffectiveListeners(gateway)
	if len(listeners) != 3 {
		t.Fatalf("EffectiveListeners() len = %d, want 3 (%#v)", len(listeners), listeners)
	}
	if listeners[0].Name != "resolved-false" {
		t.Fatalf("listener[0].Name = %q, want resolved-false", listeners[0].Name)
	}
	if listeners[1].Name != "programmed-false" {
		t.Fatalf("listener[1].Name = %q, want programmed-false", listeners[1].Name)
	}
	if listeners[2].Name != "no-status" {
		t.Fatalf("listener[2].Name = %q, want no-status", listeners[2].Name)
	}
}

func TestInfrastructureListenersRequireProgrammedWhenConditionIsPresent(t *testing.T) {
	gateway := gatewayv1.Gateway{
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{Name: "accepted-false", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
				{Name: "resolved-false", Protocol: gatewayv1.HTTPSProtocolType, Port: 443},
				{Name: "programmed-false", Protocol: gatewayv1.TCPProtocolType, Port: 9000},
				{Name: "ready", Protocol: gatewayv1.TCPProtocolType, Port: 9001},
				{Name: "no-status", Protocol: gatewayv1.UDPProtocolType, Port: 5300},
			},
		},
		Status: gatewayv1.GatewayStatus{
			Listeners: []gatewayv1.ListenerStatus{
				{
					Name: "accepted-false",
					Conditions: []metav1.Condition{{
						Type:   string(gatewayv1.ListenerConditionAccepted),
						Status: metav1.ConditionFalse,
					}},
				},
				{
					Name: "resolved-false",
					Conditions: []metav1.Condition{{
						Type:   string(gatewayv1.ListenerConditionResolvedRefs),
						Status: metav1.ConditionFalse,
					}},
				},
				{
					Name: "programmed-false",
					Conditions: []metav1.Condition{{
						Type:   string(gatewayv1.ListenerConditionProgrammed),
						Status: metav1.ConditionFalse,
					}},
				},
				{
					Name: "ready",
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1.ListenerConditionAccepted),
							Status: metav1.ConditionTrue,
						},
						{
							Type:   string(gatewayv1.ListenerConditionResolvedRefs),
							Status: metav1.ConditionTrue,
						},
						{
							Type:   string(gatewayv1.ListenerConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
		},
	}

	listeners := InfrastructureListeners(gateway)
	if len(listeners) != 3 {
		t.Fatalf("InfrastructureListeners() len = %d, want 3 (%#v)", len(listeners), listeners)
	}
	if listeners[0].Name != "resolved-false" {
		t.Fatalf("listener[0].Name = %q, want resolved-false", listeners[0].Name)
	}
	if listeners[1].Name != "ready" {
		t.Fatalf("listener[1].Name = %q, want ready", listeners[1].Name)
	}
	if listeners[2].Name != "no-status" {
		t.Fatalf("listener[2].Name = %q, want no-status", listeners[2].Name)
	}
}

func TestInfrastructureListenersKeepProgrammedListenersWithUnresolvedRefs(t *testing.T) {
	gateway := gatewayv1.Gateway{
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{Name: "partially-resolved", Protocol: gatewayv1.HTTPSProtocolType, Port: 443},
			},
		},
		Status: gatewayv1.GatewayStatus{
			Listeners: []gatewayv1.ListenerStatus{
				{
					Name: "partially-resolved",
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1.ListenerConditionAccepted),
							Status: metav1.ConditionTrue,
						},
						{
							Type:   string(gatewayv1.ListenerConditionResolvedRefs),
							Status: metav1.ConditionFalse,
						},
						{
							Type:   string(gatewayv1.ListenerConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
		},
	}

	listeners := InfrastructureListeners(gateway)
	if len(listeners) != 1 {
		t.Fatalf("InfrastructureListeners() len = %d, want 1 (%#v)", len(listeners), listeners)
	}
	if listeners[0].Name != "partially-resolved" {
		t.Fatalf("listener[0].Name = %q, want partially-resolved", listeners[0].Name)
	}
}

func TestEffectiveListenersIgnoreStaleAcceptedFalseCondition(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 2,
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{Name: "http", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
			},
		},
		Status: gatewayv1.GatewayStatus{
			Listeners: []gatewayv1.ListenerStatus{{
				Name: "http",
				Conditions: []metav1.Condition{{
					Type:               string(gatewayv1.ListenerConditionAccepted),
					Status:             metav1.ConditionFalse,
					Reason:             string(gatewayv1.ListenerReasonInvalid),
					ObservedGeneration: 1,
				}},
			}},
		},
	}

	listeners := EffectiveListeners(gateway)
	if len(listeners) != 1 {
		t.Fatalf("EffectiveListeners() len = %d, want 1 (%#v)", len(listeners), listeners)
	}
	if listeners[0].Name != "http" {
		t.Fatalf("listener[0].Name = %q, want http", listeners[0].Name)
	}
}

func TestInfrastructureListenersIgnoreStaleProgrammedFalseCondition(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 2,
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{Name: "http", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
			},
		},
		Status: gatewayv1.GatewayStatus{
			Listeners: []gatewayv1.ListenerStatus{{
				Name: "http",
				Conditions: []metav1.Condition{
					{
						Type:               string(gatewayv1.ListenerConditionAccepted),
						Status:             metav1.ConditionTrue,
						ObservedGeneration: 1,
					},
					{
						Type:               string(gatewayv1.ListenerConditionProgrammed),
						Status:             metav1.ConditionFalse,
						Reason:             string(gatewayv1.ListenerReasonInvalid),
						ObservedGeneration: 1,
					},
				},
			}},
		},
	}

	listeners := InfrastructureListeners(gateway)
	if len(listeners) != 1 {
		t.Fatalf("InfrastructureListeners() len = %d, want 1 (%#v)", len(listeners), listeners)
	}
	if listeners[0].Name != "http" {
		t.Fatalf("listener[0].Name = %q, want http", listeners[0].Name)
	}
}
