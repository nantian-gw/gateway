package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstantQuery(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("query"); got != "up" {
				t.Errorf("query = %q, want up", got)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}))
		defer srv.Close()

		client := NewPrometheusClient(srv.URL)
		resp, err := client.InstantQuery(context.Background(), "up")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Status != "success" {
			t.Errorf("status = %q, want success", resp.Status)
		}
	})

	t.Run("empty query", func(t *testing.T) {
		client := NewPrometheusClient("http://localhost:9090")
		_, err := client.InstantQuery(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for empty query")
		}
	})

	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		client := NewPrometheusClient(srv.URL)
		_, err := client.InstantQuery(context.Background(), "up")
		if err == nil {
			t.Fatal("expected error for non-200")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		client := NewPrometheusClient(srv.URL)
		_, err := client.InstantQuery(context.Background(), "up")
		if err == nil {
			t.Fatal("expected error for invalid json")
		}
	})

	t.Run("prometheus error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"error","error":"parse error"}`))
		}))
		defer srv.Close()

		client := NewPrometheusClient(srv.URL)
		_, err := client.InstantQuery(context.Background(), "up")
		if err == nil {
			t.Fatal("expected error for prometheus error response")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		client := NewPrometheusClient("http://127.0.0.1:1")
		_, err := client.InstantQuery(context.Background(), "up")
		if err == nil {
			t.Fatal("expected error for connection refused")
		}
	})
}

func TestRangeQuery(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if got := q.Get("query"); got != "rate(http_requests_total[5m])" {
				t.Errorf("query = %q", got)
			}
			if got := q.Get("start"); got != "2024-01-01T00:00:00Z" {
				t.Errorf("start = %q", got)
			}
			if got := q.Get("end"); got != "2024-01-01T01:00:00Z" {
				t.Errorf("end = %q", got)
			}
			if got := q.Get("step"); got != "60s" {
				t.Errorf("step = %q", got)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		}))
		defer srv.Close()

		client := NewPrometheusClient(srv.URL)
		resp, err := client.RangeQuery(
			context.Background(),
			"rate(http_requests_total[5m])",
			"2024-01-01T00:00:00Z",
			"2024-01-01T01:00:00Z",
			"60s",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Status != "success" {
			t.Errorf("status = %q, want success", resp.Status)
		}
	})

	t.Run("empty query", func(t *testing.T) {
		client := NewPrometheusClient("http://localhost:9090")
		_, err := client.RangeQuery(context.Background(), "", "start", "end", "60s")
		if err == nil {
			t.Fatal("expected error for empty query")
		}
	})
}

func TestNewPrometheusClient(t *testing.T) {
	client := NewPrometheusClient("http://prometheus:9090")
	if client.baseURL != "http://prometheus:9090" {
		t.Errorf("baseURL = %q, want http://prometheus:9090", client.baseURL)
	}
	if client.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}
}
