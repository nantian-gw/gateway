package backendlb

import (
	"testing"
	"time"

	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPolicyPrecedesUsesOldestTimestampFirst(t *testing.T) {
	older := backendlbv1alpha2.BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "newer-name",
			CreationTimestamp: metav1.NewTime(time.Unix(10, 0)),
		},
	}
	newer := backendlbv1alpha2.BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "older-name",
			CreationTimestamp: metav1.NewTime(time.Unix(20, 0)),
		},
	}

	if !PolicyPrecedes(older, newer) {
		t.Fatal("expected older policy to take precedence")
	}
	if PolicyPrecedes(newer, older) {
		t.Fatal("expected newer policy to lose precedence")
	}
}

func TestPolicyPrecedesUsesNameAsTieBreaker(t *testing.T) {
	left := backendlbv1alpha2.BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "alpha",
			CreationTimestamp: metav1.NewTime(time.Unix(10, 0)),
		},
	}
	right := backendlbv1alpha2.BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "beta",
			CreationTimestamp: metav1.NewTime(time.Unix(10, 0)),
		},
	}

	if !PolicyPrecedes(left, right) {
		t.Fatal("expected lexical name order to break timestamp ties")
	}
	if PolicyPrecedes(right, left) {
		t.Fatal("expected later lexical name to lose precedence")
	}
}
