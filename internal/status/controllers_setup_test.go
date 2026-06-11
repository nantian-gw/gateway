package status

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func TestStatusControllerSetupsStandardModeSkipsExperimentalControllers(t *testing.T) {
	t.Parallel()

	controllers := statusControllerSetups(nil, Options{EnableExperimentalGateway: false})
	for _, controller := range controllers {
		switch controller.(type) {
		case *tcpRouteController, *udpRouteController, *tlsRouteController, *listenerSetController:
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
