package status

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

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
