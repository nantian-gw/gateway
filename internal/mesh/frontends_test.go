package mesh

import (
	"reflect"
	"sort"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestListenerProtocolForServicePortUsesAppProtocolHints(t *testing.T) {
	appH2C := "kubernetes.io/h2c"
	appWS := "kubernetes.io/ws"   //nolint:gosec
	appWSS := "kubernetes.io/wss" //nolint:gosec
	appGRPC := "grpc"

	tests := []struct {
		name string
		port corev1.ServicePort
		want string
	}{
		{
			name: "h2c maps to http listener",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, AppProtocol: &appH2C},
			want: "HTTP",
		},
		{
			name: "ws maps to http listener",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, AppProtocol: &appWS},
			want: "HTTP",
		},
		{
			name: "wss maps to tls passthrough listener",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, AppProtocol: &appWSS},
			want: "TLS_PASSTHROUGH",
		},
		{
			name: "grpc maps to grpc listener",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, AppProtocol: &appGRPC},
			want: "GRPC",
		},
		{
			name: "udp stays udp",
			port: corev1.ServicePort{Protocol: corev1.ProtocolUDP, AppProtocol: &appH2C},
			want: "UDP",
		},
		{
			name: "plain tcp without hints stays tcp",
			port: corev1.ServicePort{Protocol: corev1.ProtocolTCP, Name: "db", Port: 5432},
			want: "TCP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ListenerProtocolForServicePort(tt.port); got != tt.want {
				t.Fatalf("ListenerProtocolForServicePort() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParentServiceRef(t *testing.T) {
	group := gatewayv1.Group(gatewayv1.GroupName)
	serviceKind := gatewayv1.Kind(FrontendKindService)
	gatewayKind := gatewayv1.Kind("Gateway")
	namespace := gatewayv1.Namespace("backends")

	tests := []struct {
		name             string
		ref              gatewayv1.ParentReference
		defaultNamespace string
		want             ServiceParentKey
		wantOK           bool
	}{
		{
			name: "uses route namespace when parent namespace is empty",
			ref: gatewayv1.ParentReference{
				Kind: &serviceKind,
				Name: "orders",
			},
			defaultNamespace: "default",
			want:             ServiceParentKey{Namespace: "default", Name: "orders"},
			wantOK:           true,
		},
		{
			name: "uses explicit parent namespace",
			ref: gatewayv1.ParentReference{
				Kind:      &serviceKind,
				Namespace: &namespace,
				Name:      "orders",
			},
			defaultNamespace: "default",
			want:             ServiceParentKey{Namespace: "backends", Name: "orders"},
			wantOK:           true,
		},
		{
			name: "rejects non-empty group",
			ref: gatewayv1.ParentReference{
				Group: &group,
				Kind:  &serviceKind,
				Name:  "orders",
			},
			defaultNamespace: "default",
		},
		{
			name: "rejects gateway kind",
			ref: gatewayv1.ParentReference{
				Kind: &gatewayKind,
				Name: "gateway",
			},
			defaultNamespace: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParentServiceRef(tt.ref, tt.defaultNamespace)
			if ok != tt.wantOK {
				t.Fatalf("ParentServiceRef() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("ParentServiceRef() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRouteUsesOnlyServiceParents(t *testing.T) {
	serviceKind := gatewayv1.Kind(FrontendKindService)
	gatewayKind := gatewayv1.Kind("Gateway")

	tests := []struct {
		name string
		refs []gatewayv1.ParentReference
		want bool
	}{
		{
			name: "empty parent refs are not service-only",
		},
		{
			name: "single service parent is service-only",
			refs: []gatewayv1.ParentReference{{
				Kind: &serviceKind,
				Name: "orders",
			}},
			want: true,
		},
		{
			name: "mixed service and gateway parents are not service-only",
			refs: []gatewayv1.ParentReference{
				{Kind: &serviceKind, Name: "orders"},
				{Kind: &gatewayKind, Name: "edge"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RouteUsesOnlyServiceParents(tt.refs, "default"); got != tt.want {
				t.Fatalf("RouteUsesOnlyServiceParents() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpandServiceFrontendsFiltersSortsAndAssignsStablePorts(t *testing.T) {
	appProtocolHTTP := "http"
	appProtocolGRPC := "grpc"
	services := []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "ignored"},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{
				Name: "http",
				Port: 80,
			}}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "orders"},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
				{
					Name:        "grpc",
					Port:        9090,
					AppProtocol: &appProtocolGRPC,
				},
				{
					Name:        "web",
					Port:        80,
					AppProtocol: &appProtocolHTTP,
				},
			}},
		},
	}

	got := ExpandServiceFrontends(services, []ServiceParentKey{{Namespace: "apps", Name: "orders"}})
	if len(got) != 2 {
		t.Fatalf("ExpandServiceFrontends() returned %d items, want 2: %#v", len(got), got)
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i].Key() < got[j].Key() }) {
		t.Fatalf("ExpandServiceFrontends() did not sort by key: %#v", got)
	}

	wantKeys := []string{"apps/orders/80", "apps/orders/9090"}
	gotKeys := []string{got[0].Key(), got[1].Key()}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("frontend keys = %#v, want %#v", gotKeys, wantKeys)
	}

	seenPorts := map[int32]struct{}{}
	for _, item := range got {
		if item.ListenPort < FrontendPortBase || item.ListenPort >= FrontendPortBase+FrontendPortCount {
			t.Fatalf("ListenPort %d out of expected range", item.ListenPort)
		}
		if _, exists := seenPorts[item.ListenPort]; exists {
			t.Fatalf("duplicate ListenPort %d in %#v", item.ListenPort, got)
		}
		seenPorts[item.ListenPort] = struct{}{}
		if item.ListenerName() != "mesh/"+item.Namespace+"/"+item.Name+"/"+strconv.Itoa(int(item.ListenPort)) {
			t.Fatalf("unexpected ListenerName() = %q", item.ListenerName())
		}
	}
	if got[0].Protocol != "HTTP" {
		t.Fatalf("port 80 protocol = %q, want HTTP", got[0].Protocol)
	}
	if got[1].Protocol != "GRPC" {
		t.Fatalf("port 9090 protocol = %q, want GRPC", got[1].Protocol)
	}
}

func TestServiceFrontendPortMetadata(t *testing.T) {
	item := ServiceFrontendPort{
		Namespace:   "apps",
		Name:        "orders",
		ServicePort: 8080,
		ListenPort:  23456,
	}

	want := map[string]string{
		FrontendKindMetadataKey:      FrontendKindService,
		FrontendNamespaceMetadataKey: "apps",
		FrontendNameMetadataKey:      "orders",
		FrontendPortMetadataKey:      "8080",
	}
	if got := item.Metadata(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Metadata() = %#v, want %#v", got, want)
	}
	if got := item.ListenerName(); got != "mesh/apps/orders/23456" {
		t.Fatalf("ListenerName() = %q, want mesh/apps/orders/23456", got)
	}
}

func TestShadowServiceName(t *testing.T) {
	if got := ShadowServiceName("apps", "orders"); got != "nantian-gw-shadow-orders" {
		t.Fatalf("ShadowServiceName(short) = %q, want nantian-gw-shadow-orders", got)
	}

	longName := "orders-" +
		"abcdefghijklmnopqrstuvwxyz" +
		"abcdefghijklmnopqrstuvwxyz" +
		"abcdefghijklmnopqrstuvwxyz"
	got := ShadowServiceName("apps", longName)
	if len(got) != 63 {
		t.Fatalf("ShadowServiceName(long) length = %d, want 63: %q", len(got), got)
	}
	if got == ShadowServiceName("other", longName) {
		t.Fatalf("ShadowServiceName() should include namespace in the hash suffix")
	}
}
