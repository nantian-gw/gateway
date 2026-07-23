package admin

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testRBACLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestWrapRBACHandler_Disabled(t *testing.T) {
	handler := wrapRBACHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), nil, testRBACLogger())

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", http.NoBody)
	rc := &routeContract{Permission: PermissionRead}
	ctx := context.WithValue(req.Context(), routeContractKey, rc)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestWrapRBACHandler_NoPermissionRequired(t *testing.T) {
	cfg := &AdminRBACConfig{
		Roles: []Role{{Name: "admin", Permissions: []Permission{PermissionAdmin}, MatchUsers: []string{"admin"}}},
	}
	handler := wrapRBACHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), cfg, testRBACLogger())

	req := httptest.NewRequest(http.MethodGet, "/livez", http.NoBody)
	rc := &routeContract{} // no Permission
	ctx := context.WithValue(req.Context(), routeContractKey, rc)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for no-permission route, got %d", w.Code)
	}
}

func TestWrapRBACHandler_Denied_NoIdentity(t *testing.T) {
	cfg := &AdminRBACConfig{
		Roles: []Role{{Name: "admin", Permissions: []Permission{PermissionAdmin}, MatchUsers: []string{"admin"}}},
	}
	handler := wrapRBACHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}), cfg, testRBACLogger())

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", http.NoBody)
	rc := &routeContract{Permission: PermissionRead}
	ctx := context.WithValue(req.Context(), routeContractKey, rc)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for no identity, got %d", w.Code)
	}
}

func TestWrapRBACHandler_Denied_WrongPermissions(t *testing.T) {
	cfg := &AdminRBACConfig{
		Roles: []Role{{
			Name:        "reader",
			Permissions: []Permission{PermissionRead},
			MatchUsers:  []string{"reader"},
		}},
	}
	handler := wrapRBACHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for write endpoint")
	}), cfg, testRBACLogger())

	req := httptest.NewRequest(http.MethodPost, "/v1/resources", http.NoBody)
	rc := &routeContract{Permission: PermissionWriteResources}
	ctx := context.WithValue(req.Context(), routeContractKey, rc)
	ctx = context.WithValue(ctx, identityKey, &Identity{Username: "reader", Subject: "reader"})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong permissions, got %d", w.Code)
	}
}

func TestWrapRBACHandler_Allowed_AdminFullAccess(t *testing.T) {
	cfg := &AdminRBACConfig{
		Roles: []Role{{
			Name:        "admin",
			Permissions: []Permission{PermissionAdmin},
			MatchUsers:  []string{"admin"},
		}},
	}
	handler := wrapRBACHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), cfg, testRBACLogger())

	tests := []struct {
		method     string
		path       string
		permission Permission
	}{
		{http.MethodGet, "/v1/summary", PermissionRead},
		{http.MethodPost, "/v1/resources", PermissionWriteResources},
		{http.MethodPut, "/v1/chatbot/config", PermissionWriteChatbot},
		{http.MethodPost, "/v1/metrics/query", PermissionWriteMetrics},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, http.NoBody)
			rc := &routeContract{Permission: tt.permission}
			ctx := context.WithValue(req.Context(), routeContractKey, rc)
			ctx = context.WithValue(ctx, identityKey, &Identity{Username: "admin", Subject: "admin"})
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 for admin on %s %s, got %d", tt.method, tt.path, w.Code)
			}
		})
	}
}

func TestWrapRBACHandler_Allowed_GroupMatching(t *testing.T) {
	cfg := &AdminRBACConfig{
		Roles: []Role{{
			Name:        "reader",
			Permissions: []Permission{PermissionRead},
			MatchGroups: []string{"readers"},
		}},
	}
	handler := wrapRBACHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), cfg, testRBACLogger())

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", http.NoBody)
	rc := &routeContract{Permission: PermissionRead}
	ctx := context.WithValue(req.Context(), routeContractKey, rc)
	ctx = context.WithValue(ctx, identityKey, &Identity{
		Username: "some-user",
		Groups:   []string{"readers", "viewers"},
		Subject:  "some-user",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for group match, got %d", w.Code)
	}
}

func TestWrapRBACHandler_Allowed_MultipleRoles(t *testing.T) {
	cfg := &AdminRBACConfig{
		Roles: []Role{
			{Name: "reader", Permissions: []Permission{PermissionRead}, MatchUsers: []string{"ops"}},
			{Name: "writer", Permissions: []Permission{PermissionWriteResources}, MatchUsers: []string{"ops"}},
		},
	}
	handler := wrapRBACHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), cfg, testRBACLogger())

	// ops should be able to read AND write resources
	req := httptest.NewRequest(http.MethodPost, "/v1/resources", http.NoBody)
	rc := &routeContract{Permission: PermissionWriteResources}
	ctx := context.WithValue(req.Context(), routeContractKey, rc)
	ctx = context.WithValue(ctx, identityKey, &Identity{Username: "ops", Subject: "ops"})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for ops user with writer role, got %d", w.Code)
	}
}
