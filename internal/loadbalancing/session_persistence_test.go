package loadbalancing

import (
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ptr[T any](v T) *T {
	return &v
}

func TestValidateSessionPersistence_Nil(t *testing.T) {
	if err := ValidateSessionPersistence(nil); err != nil {
		t.Fatalf("expected nil to be valid, got: %v", err)
	}
}

func TestValidateSessionPersistence_ValidCookie(t *testing.T) {
	sp := &gatewayv1.SessionPersistence{
		SessionName:     ptr("my-session"),
		Type:            ptr(gatewayv1.CookieBasedSessionPersistence),
		AbsoluteTimeout: ptr(gatewayv1.Duration("5m")),
		CookieConfig: &gatewayv1.CookieConfig{
			LifetimeType: ptr(gatewayv1.SessionCookieLifetimeType),
		},
	}
	if err := ValidateSessionPersistence(sp); err != nil {
		t.Fatalf("expected valid cookie config to pass, got: %v", err)
	}
}

func TestValidateSessionPersistence_ValidHeader(t *testing.T) {
	sp := &gatewayv1.SessionPersistence{
		SessionName: ptr("header-session"),
		Type:        ptr(gatewayv1.HeaderBasedSessionPersistence),
	}
	if err := ValidateSessionPersistence(sp); err != nil {
		t.Fatalf("expected valid header config to pass, got: %v", err)
	}
}

func TestValidateSessionPersistence_SessionNameTooLong(t *testing.T) {
	sp := &gatewayv1.SessionPersistence{
		SessionName: ptr(
			"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz" +
				"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz" +
				"abcdefghijklmnopqrstuvwxyzABCDEF",
		),
	}
	if err := ValidateSessionPersistence(sp); err == nil {
		t.Fatal("expected error for sessionName exceeding 128 characters")
	}
}

func TestValidateSessionPersistence_PermanentWithoutAbsoluteTimeout(t *testing.T) {
	sp := &gatewayv1.SessionPersistence{
		Type: ptr(gatewayv1.CookieBasedSessionPersistence),
		CookieConfig: &gatewayv1.CookieConfig{
			LifetimeType: ptr(gatewayv1.PermanentCookieLifetimeType),
		},
	}
	if err := ValidateSessionPersistence(sp); err == nil {
		t.Fatal("expected error for Permanent lifetimeType without absoluteTimeout")
	}
}
