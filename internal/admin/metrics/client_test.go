package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstantQuerySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "up" {
			t.Errorf("unexpected query: %s", r.URL.Query().Get("query"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PrometheusResponse{
			Status: "success",
			Data:   json.RawMessage(`{"resultType":"vector","result":[]}`),
		})
	}))
	defer server.Close()

	client := NewPrometheusClient(server.URL)
	resp, err := client.InstantQuery(context.Background(), "up")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Status != "success" {
		t.Errorf("expected status success, got %s", resp.Status)
	}
}

func TestInstantQueryPrometheusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PrometheusResponse{
			Status: "error",
			Error:  "bad_data",
		})
	}))
	defer server.Close()

	client := NewPrometheusClient(server.URL)
	_, err := client.InstantQuery(context.Background(), "invalid{")
	if err == nil {
		t.Fatal("expected error for Prometheus error response")
	}
}

func TestInstantQueryConnectionRefused(t *testing.T) {
	client := NewPrometheusClient("http://127.0.0.1:1")
	_, err := client.InstantQuery(context.Background(), "up")
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestInstantQueryEmptyQuery(t *testing.T) {
	client := NewPrometheusClient("http://example.com")
	_, err := client.InstantQuery(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestInstantQueryNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewPrometheusClient(server.URL)
	_, err := client.InstantQuery(context.Background(), "up")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}