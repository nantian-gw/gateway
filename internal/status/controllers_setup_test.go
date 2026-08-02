package status

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	"github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
)

func TestStatusControllerSetupsStandardModeSkipsExperimentalControllers(t *testing.T) {
	t.Parallel()

	controllers := statusControllerSetups(nil, Options{EnableExperimentalGateway: false})
	for _, controller := range controllers {
		switch controller.(type) {
		case *tcpRouteController, *udpRouteController, *listenerSetController:
			t.Fatalf("standard mode included experimental status controller %T", controller)
		}
	}
}

func TestStatusControllerSetupsExperimentalModeIncludesExperimentalControllers(t *testing.T) {
	t.Parallel()

	controllers := statusControllerSetups(nil, Options{EnableExperimentalGateway: true})
	seen := map[string]bool{}
	for _, controller := range controllers {
		switch controller.(type) {
		case *tcpRouteController:
			seen["tcp"] = true
		case *udpRouteController:
			seen["udp"] = true
		case *tlsRouteController:
			seen["tls"] = true
		case *listenerSetController:
			seen["listenerset"] = true
		}
	}

	for _, name := range []string{"tcp", "udp", "tls", "listenerset"} {
		if !seen[name] {
			t.Fatalf("experimental mode did not include %s status controller", name)
		}
	}
}

func TestResourceSupportedReturnsFalseWhenRESTMappingMissing(t *testing.T) {
	t.Parallel()

	scheme := newScheme(t)
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		mcsv1alpha1.SchemeGroupVersion,
	})

	if resourceSupported(scheme, restMapper, &mcsv1alpha1.ServiceImport{}) {
		t.Fatal("expected ServiceImport to be unsupported without a REST mapping")
	}
}

func TestResourceSupportedReturnsTrueWhenRESTMappingPresent(t *testing.T) {
	t.Parallel()

	scheme := newScheme(t)
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		mcsv1alpha1.SchemeGroupVersion,
	})
	restMapper.Add(mcsv1alpha1.SchemeGroupVersion.WithKind("ServiceImport"), meta.RESTScopeNamespace)

	if !resourceSupported(scheme, restMapper, &mcsv1alpha1.ServiceImport{}) {
		t.Fatal("expected ServiceImport to be supported when a REST mapping exists")
	}
}

func TestSupportedControllersSkipsGatedControllersWhenCRDAbsent(t *testing.T) {
	t.Parallel()

	controllers := statusControllerSetups(nil, Options{EnableExperimentalGateway: true, EnableAiGateway: true})
	supported := func(object client.Object) bool {
		switch object.(type) {
		case *backend.BackendLBPolicy, *routepolicy.RoutePolicy:
			return false
		default:
			return true
		}
	}

	gated := supportedControllers(controllers, supported)
	for _, controller := range gated {
		switch controller.(type) {
		case *backendLBPolicyController, *routePolicyController:
			t.Fatalf("expected controller %T to be skipped when its CRD is absent", controller)
		}
	}
}

func TestSupportedControllersKeepsSupportedExperimentalControllers(t *testing.T) {
	t.Parallel()

	controllers := statusControllerSetups(nil, Options{EnableExperimentalGateway: true})
	gated := supportedControllers(controllers, func(client.Object) bool { return true })

	seen := map[string]bool{}
	for _, controller := range gated {
		switch controller.(type) {
		case *tcpRouteController:
			seen["tcp"] = true
		case *udpRouteController:
			seen["udp"] = true
		case *backendLBPolicyController:
			seen["backendlbp"] = true
		}
	}

	for _, name := range []string{"tcp", "udp", "backendlbp"} {
		if !seen[name] {
			t.Fatalf("expected %s controller to be kept when supported", name)
		}
	}
}
