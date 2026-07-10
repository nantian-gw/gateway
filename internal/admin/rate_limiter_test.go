package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterMiddlewareLimitsRequestsPerRemoteAddress(t *testing.T) {
	limiter := newRateLimiter(2, time.Minute, nil)
	var calls int
	handler := limiter.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
		req.RemoteAddr = "192.0.2.10:12345"
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d, want %d", i+1, recorder.Code, http.StatusNoContent)
		}
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("third request status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if calls != 2 {
		t.Fatalf("handler calls = %d, want 2", calls)
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.RemoteAddr = "192.0.2.11:12345"
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("different remote status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRateLimiterMiddlewareResetsAfterWindow(t *testing.T) {
	limiter := newRateLimiter(1, time.Nanosecond, nil)
	var calls int
	handler := limiter.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	time.Sleep(time.Millisecond)

	recorder := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("request after reset status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if calls != 2 {
		t.Fatalf("handler calls = %d, want 2", calls)
	}
}
