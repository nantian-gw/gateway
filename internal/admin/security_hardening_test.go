package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func newFakeTokenReviewClient(t *testing.T, authenticated bool, user string, groups []string, calls *int, audiences *[]string) *fake.Clientset {
	t.Helper()
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if calls != nil {
			*calls++
		}
		tr := action.(k8stesting.CreateAction).GetObject().(*authenticationv1.TokenReview)
		if audiences != nil {
			*audiences = tr.Spec.Audiences
		}
		return true, &authenticationv1.TokenReview{
			Status: authenticationv1.TokenReviewStatus{
				Authenticated: authenticated,
				User:          authenticationv1.UserInfo{Username: user, Groups: groups},
			},
		}, nil
	})
	return cs
}

func TestTokenReviewAuthenticatorPassesAudiencesAndCaches(t *testing.T) {
	var calls int
	var seenAudiences []string
	cs := newFakeTokenReviewClient(t, true, "alice", []string{"devs"}, &calls, &seenAudiences)

	auth := &tokenReviewAuthenticator{
		clientset: cs,
		audiences: []string{"nantian-controlplane-admin"},
		cacheTTL:  time.Minute,
	}

	res, err := auth.authenticate(context.Background(), "token-xyz")
	if err != nil {
		t.Fatalf("authenticate error: %v", err)
	}
	if !res.authenticated || res.username != "alice" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(seenAudiences) != 1 || seenAudiences[0] != "nantian-controlplane-admin" {
		t.Fatalf("audiences not forwarded to TokenReview: %v", seenAudiences)
	}

	if _, err := auth.authenticate(context.Background(), "token-xyz"); err != nil {
		t.Fatalf("second authenticate error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected TokenReview to be cached (1 call), got %d", calls)
	}
}

func TestTokenReviewAuthenticatorAuthorization(t *testing.T) {
	auth := &tokenReviewAuthenticator{
		users:  toSet([]string{"bob"}),
		groups: toSet([]string{"platform"}),
	}

	if !auth.authorizeWrite(authResult{authenticated: true, username: "alice", groups: []string{"platform"}}) {
		t.Fatal("expected group-based authorization to allow")
	}
	if !auth.authorizeWrite(authResult{authenticated: true, username: "bob", groups: []string{"other"}}) {
		t.Fatal("expected user-based authorization to allow")
	}
	if auth.authorizeWrite(authResult{authenticated: true, username: "carol", groups: []string{"ops"}}) {
		t.Fatal("expected authorization to deny identity outside allowlists")
	}

	// No allowlist configured: writes fail closed even for authenticated identities.
	open := &tokenReviewAuthenticator{}
	if open.authorizeWrite(authResult{authenticated: true, username: "anyone"}) {
		t.Fatal("expected writes to be denied when no allowlist configured")
	}
}

func TestRateLimiterHonorsXFFOnlyFromTrustedProxy(t *testing.T) {
	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/v1/summary", http.NoBody)
		r.RemoteAddr = "10.1.2.3:5000"
		r.Header.Set("X-Forwarded-For", "203.0.113.9")
		return r
	}

	untrusted := newRateLimiter(100, 100, nil)
	if got := untrusted.clientIP(req()); got != "10.1.2.3" {
		t.Fatalf("untrusted peer must use RemoteAddr, got %q", got)
	}

	trusted := newRateLimiter(100, 100, []string{"10.0.0.0/8"})
	if got := trusted.clientIP(req()); got != "203.0.113.9" {
		t.Fatalf("trusted peer must honor XFF, got %q", got)
	}
}
