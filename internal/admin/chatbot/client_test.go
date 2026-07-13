package chatbot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatCompletionStream_ReceivesChunks(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher support")
		}

		chunks := []string{
			`data: {"choices": [{"delta": {"content": "Hello"}, "index": 0}]}`,
			`data: {"choices": [{"delta": {"content": " "}, "index": 0}]}`,
			`data: {"choices": [{"delta": {"content": "World"}, "index": 0}]}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			if _, err := w.Write([]byte(chunk + "\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewOpenAIAdapter(server.URL, "sk-test", "gpt-4o", 0.1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunkChan := make(chan string, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ChatCompletionStream(ctx, "Say hello", nil, chunkChan)
		close(chunkChan)
	}()

	var result strings.Builder
	for chunk := range chunkChan {
		result.WriteString(chunk)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("unexpected streaming error: %v", err)
	}

	if result.String() != "Hello World" {
		t.Errorf("result = %q, want %q", result.String(), "Hello World")
	}
}

func TestChatCompletionStream_WithHistory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		w.Write([]byte(`data: {"choices": [{"delta": {"content": "Acknowledged."}, "index": 0}]}` + "\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n"))
	}))
	defer server.Close()

	client := NewOpenAIAdapter(server.URL, "sk-test", "gpt-4o", 0.1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	history := []Message{
		{Role: "user", Content: "What is a Gateway?"},
		{Role: "assistant", Content: "A Gateway manages inbound traffic."},
	}

	chunkChan := make(chan string, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ChatCompletionStream(ctx, "Thanks", history, chunkChan)
		close(chunkChan)
	}()

	var result strings.Builder
	for chunk := range chunkChan {
		result.WriteString(chunk)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("unexpected streaming error: %v", err)
	}

	if result.String() != "Acknowledged." {
		t.Errorf("result = %q, want %q", result.String(), "Acknowledged.")
	}
}

func TestChatCompletionStream_ServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOpenAIAdapter(server.URL, "sk-test", "gpt-4o", 0.1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunkChan := make(chan string, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ChatCompletionStream(ctx, "test", nil, chunkChan)
		close(chunkChan)
	}()

	for range chunkChan {
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error for 500 status, got nil")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("expected status 500 in error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error")
	}
}

func TestChatCompletionStream_EmptyStream(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: [DONE]\n"))
	}))
	defer server.Close()

	client := NewOpenAIAdapter(server.URL, "sk-test", "gpt-4o", 0.1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunkChan := make(chan string, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ChatCompletionStream(ctx, "test", nil, chunkChan)
		close(chunkChan)
	}()

	var count int
	for range chunkChan {
		count++
	}

	if err := <-errCh; err != nil {
		t.Fatalf("unexpected streaming error: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 chunks, got %d", count)
	}
}

func TestChatCompletionStream_MalformedJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		w.Write([]byte("data: {invalid}\n"))
		flusher.Flush()
	}))
	defer server.Close()

	client := NewOpenAIAdapter(server.URL, "sk-test", "gpt-4o", 0.1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunkChan := make(chan string, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ChatCompletionStream(ctx, "test", nil, chunkChan)
		close(chunkChan)
	}()

	for range chunkChan {
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected parse error for malformed JSON, got nil")
		}
		if !strings.Contains(err.Error(), "parse") {
			t.Errorf("expected 'parse' in error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error")
	}
}

func TestChatCompletionStream_NoAuthWhenAPIKeyEmpty(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("expected no Authorization header when API key is empty")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: [DONE]\n"))
	}))
	defer server.Close()

	client := NewOpenAIAdapter(server.URL, "", "gpt-4o", 0.1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunkChan := make(chan string, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ChatCompletionStream(ctx, "test", nil, chunkChan)
		close(chunkChan)
	}()

	for range chunkChan {
	}

	if err := <-errCh; err != nil {
		t.Fatalf("unexpected streaming error: %v", err)
	}
}

func TestChatCompletionStream_SSECommentsIgnored(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		w.Write([]byte(": this is a comment\n"))
		flusher.Flush()
		w.Write([]byte(`data: {"choices": [{"delta": {"content": "X"}}]}` + "\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n"))
	}))
	defer server.Close()

	client := NewOpenAIAdapter(server.URL, "sk-test", "gpt-4o", 0.1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunkChan := make(chan string, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ChatCompletionStream(ctx, "test", nil, chunkChan)
		close(chunkChan)
	}()

	var result strings.Builder
	for chunk := range chunkChan {
		result.WriteString(chunk)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("unexpected streaming error: %v", err)
	}

	if result.String() != "X" {
		t.Errorf("result = %q, want %q", result.String(), "X")
	}
}