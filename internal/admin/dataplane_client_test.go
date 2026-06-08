package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDataplaneAdminClientGetJSONAddsBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewDataplaneAdminClient(DataplaneAdminClientConfig{
		Timeout:     time.Second,
		BearerToken: "secret",
	})

	var out map[string]bool
	if err := client.GetJSON(context.Background(), server.URL, "/v1/summary", &out); err != nil {
		t.Fatal(err)
	}
	if !out["ok"] {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestDataplaneAdminClientReturnsErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer server.Close()

	client := NewDataplaneAdminClient(DataplaneAdminClientConfig{
		Timeout: time.Second,
	})

	var out map[string]string
	err := client.GetJSON(context.Background(), server.URL, "/v1/summary", &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDataplaneAdminClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewDataplaneAdminClient(DataplaneAdminClientConfig{
		Timeout: 50 * time.Millisecond,
	})

	var out map[string]bool
	err := client.GetJSON(context.Background(), server.URL, "/v1/summary", &out)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}