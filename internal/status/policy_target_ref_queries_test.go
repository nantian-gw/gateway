package status

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
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

func TestSetupIndexesIgnoresMissingStatusBackendLBPolicyCRD(t *testing.T) {
	indexer := &fakeFieldIndexer{
		errs: map[string]error{
			statusBackendLBPolicyTargetRefIndex: &meta.NoKindMatchError{
				GroupKind: schema.GroupKind{
					Group: backendlbv1alpha2.GroupVersion.Group,
					Kind:  "BackendLBPolicy",
				},
				SearchedVersions: []string{backendlbv1alpha2.GroupVersion.Version},
			},
		},
	}

	if err := SetupIndexes(context.Background(), indexer); err != nil {
		t.Fatalf("SetupIndexes returned error for missing optional BackendLBPolicy CRD: %v", err)
	}
}

func TestSetupIndexesIgnoresMissingStatusBackendTLSPolicyCRD(t *testing.T) {
	indexer := &fakeFieldIndexer{
		errs: map[string]error{
			statusBackendTLSPolicyTargetRefIndex: &meta.NoKindMatchError{
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

func TestSetupIndexesReturnsUnexpectedStatusBackendLBPolicyIndexError(t *testing.T) {
	indexer := &fakeFieldIndexer{
		errs: map[string]error{
			statusBackendLBPolicyTargetRefIndex: errors.New("boom"),
		},
	}

	err := SetupIndexes(context.Background(), indexer)
	if err == nil {
		t.Fatal("SetupIndexes returned nil error, want wrapped BackendLBPolicy index failure")
	}
	if !strings.Contains(err.Error(), "index BackendLBPolicy target refs: boom") {
		t.Fatalf("SetupIndexes returned %q, want BackendLBPolicy index failure", err)
	}
}

func TestStatusBackendPolicyTargetRefIndexKeysDeduplicateAndSortValues(t *testing.T) {
	raw, err := gatewayapi.EncodeBackendTLSPolicyV1(&gatewayv1alpha3.BackendTLSPolicy{
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: mcsv1alpha1.GroupName,
						Kind:  "ServiceImport",
						Name:  "imported",
					},
				},
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "echo",
					},
				},
				{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Kind: "Service",
						Name: "echo",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("EncodeBackendTLSPolicyV1 returned error: %v", err)
	}

	gotTLS := statusBackendTLSPolicyTargetRefIndexKeys(raw)
	wantTLS := []string{
		backendPolicyTargetRefIndexValue("", "Service", "echo"),
		backendPolicyTargetRefIndexValue(mcsv1alpha1.GroupName, "ServiceImport", "imported"),
	}
	if !reflect.DeepEqual(gotTLS, wantTLS) {
		t.Fatalf("statusBackendTLSPolicyTargetRefIndexKeys() = %#v, want %#v", gotTLS, wantTLS)
	}

	gotLB := statusBackendLBPolicyTargetRefIndexKeys(&backendlbv1alpha2.BackendLBPolicy{
		Spec: backendlbv1alpha2.BackendLBPolicySpec{
			TargetRefs: []backendlbv1alpha2.LocalPolicyTargetReference{
				{Group: mcsv1alpha1.GroupName, Kind: "ServiceImport", Name: "imported"},
				{Kind: "Service", Name: "echo"},
				{Kind: "Service", Name: "echo"},
			},
		},
	})
	wantLB := []string{
		backendPolicyTargetRefIndexValue("", "Service", "echo"),
		backendPolicyTargetRefIndexValue(mcsv1alpha1.GroupName, "ServiceImport", "imported"),
	}
	if !reflect.DeepEqual(gotLB, wantLB) {
		t.Fatalf("statusBackendLBPolicyTargetRefIndexKeys() = %#v, want %#v", gotLB, wantLB)
	}
}
