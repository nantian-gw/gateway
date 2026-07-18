package policies

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
)

type fakeFieldIndexer struct {
	errs map[string]error
}

func (f *fakeFieldIndexer) IndexField(
	_ context.Context,
	_ client.Object,
	field string,
	_ client.IndexerFunc,
) error {
	if f == nil || f.errs == nil {
		return nil
	}
	return f.errs[field]
}

func TestSetupIndexesIgnoresMissingBackendLBPolicyCRD(t *testing.T) {
	indexer := &fakeFieldIndexer{
		errs: map[string]error{
			BackendLBPolicyTargetRefIndex: &metav1.NoKindMatchError{
				GroupKind: schema.GroupKind{
					Group: backend.GroupVersion.Group,
					Kind:  "BackendLBPolicy",
				},
				SearchedVersions: []string{backend.GroupVersion.Version},
			},
		},
	}

	if err := SetupIndexes(context.Background(), indexer); err != nil {
		t.Fatalf("SetupIndexes returned error for missing optional BackendLBPolicy CRD: %v", err)
	}
}

func TestSetupIndexesIgnoresMissingBackendTLSPolicyCRD(t *testing.T) {
	indexer := &fakeFieldIndexer{
		errs: map[string]error{
			BackendTLSPolicyTargetRefIndex: &metav1.NoKindMatchError{
				GroupKind: gatewayapi.BackendTLSPolicyV1GVK.GroupKind(),
				SearchedVersions: []string{
					gatewayapi.BackendTLSPolicyV1GVK.Version,
				},
			},
		},
	}

	if err := SetupIndexes(context.Background(), indexer); err != nil {
		t.Fatalf("SetupIndexes returned error for missing optional BackendTLSPolicy CRD: %v", err)
	}
}

func TestSetupIndexesReturnsUnexpectedBackendLBPolicyIndexError(t *testing.T) {
	indexer := &fakeFieldIndexer{
		errs: map[string]error{
			BackendLBPolicyTargetRefIndex: errors.New("boom"),
		},
	}

	err := SetupIndexes(context.Background(), indexer)
	if err == nil {
		t.Fatalf("SetupIndexes returned nil error, want wrapped BackendLBPolicy index failure")
	}
	if !strings.Contains(err.Error(), "index BackendLBPolicy target refs: boom") {
		t.Fatalf("SetupIndexes returned %q, want BackendLBPolicy index failure", err)
	}
}
