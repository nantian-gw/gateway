package chatbot

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	jsoniter "github.com/json-iterator/go"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// Message represents a single turn in a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMClient defines the streaming chat interface used by the chatbot engine.
type LLMClient interface {
	// ChatCompletionStream sends a prompt (with conversation history) to the
	// LLM and streams token-level chunks via chunkChan. The caller MUST close
	// chunkChan after the returned error is received.
	ChatCompletionStream(ctx context.Context, prompt string, history []Message, chunkChan chan<- string) error
}

// openAIAdapter implements LLMClient for OpenAI-compatible endpoints
// (OpenAI, DeepSeek, Ollama, and any /v1/chat/completions-compatible API).
type openAIAdapter struct {
	endpoint string
	apiKey   string
	model    string
	temp     float64
	client   *http.Client
	logger   *slog.Logger
}

const (
	openAIChatCompletionsPath = "/v1/chat/completions"
	defaultLLMTimeout         = 120 * time.Second
	defaultLLMIdleTimeout     = 90 * time.Second
)

var defaultLLMTransport = &http.Transport{
	MaxIdleConns:    10,
	IdleConnTimeout: defaultLLMIdleTimeout,
}

// devInsecureTransport returns an http.Transport that skips TLS certificate
// verification. It MUST only be used when CHATBOT_INSECURE_TLS is explicitly
// enabled for development environments. The InsecureSkipVerify field is
// intentionally exposed here — gosec G402 will flag it, which is correct.
// This function exists to isolate the security exception to a single,
// auditable location.
func devInsecureTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // gosec G402: development-only, gated behind CHATBOT_INSECURE_TLS env var
		},
	}
}

// openAIStreamResponse represents a single SSE chunk returned by the server.
type openAIStreamResponse struct {
	Choices []openAIStreamChoice `json:"choices"`
}

type openAIStreamChoice struct {
	Delta openAIStreamDelta `json:"delta"`
	Index int               `json:"index"`
}

type openAIStreamDelta struct {
	Content string `json:"content,omitempty"`
}

// NewOpenAIAdapter creates an LLMClient backed by an OpenAI-compatible API.
func NewOpenAIAdapter(endpoint, apiKey, model string, temperature float64, logger *slog.Logger) LLMClient {
	endpoint = strings.TrimRight(endpoint, "/")

	// CHATBOT_INSECURE_TLS is for development only. Production deployments
	// must configure proper CA certificates instead.
	//
	// SECURITY: InsecureSkipVerify disables TLS certificate verification.
	// This is intentionally scoped behind an env var so it cannot be enabled
	// accidentally through config files. The env var name is deliberately
	// verbose and alarming to discourage use in production.
	insecureSkipVerify := os.Getenv("CHATBOT_INSECURE_TLS") == "true"
	if insecureSkipVerify {
		warnLogger := logger
		if warnLogger == nil {
			warnLogger = slog.Default()
		}
		warnLogger.Warn("CHATBOT_INSECURE_TLS is enabled — TLS certificate verification is disabled for LLM API calls. This is a development-only setting.")
	}

	transport := defaultLLMTransport
	if insecureSkipVerify {
		transport = devInsecureTransport()
	}

	return &openAIAdapter{
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    model,
		temp:     temperature,
		logger:   logger,
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultLLMTimeout,
		},
	}
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	Stream      bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (a *openAIAdapter) ChatCompletionStream(
	ctx context.Context,
	prompt string,
	history []Message,
	chunkChan chan<- string,
) error {
	messages := make([]openAIMessage, 0, len(history)+1)
	for _, m := range history {
		messages = append(messages, openAIMessage(m))
	}
	messages = append(messages, openAIMessage{Role: "user", Content: prompt})

	body := openAIChatRequest{
		Model:       a.model,
		Messages:    messages,
		Temperature: a.temp,
		Stream:      true,
	}

	reqBytes, err := jsoniter.Marshal(body)
	if err != nil {
		return fmt.Errorf("openai adapter: marshal request: %w", err)
	}

	url := a.endpoint + openAIChatCompletionsPath

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return fmt.Errorf("openai adapter: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("openai adapter: http post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("openai adapter: unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return a.readSSEStream(resp.Body, chunkChan)
}

func (a *openAIAdapter) readSSEStream(r io.Reader, chunkChan chan<- string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			return nil
		}

		token, err := parseStreamChunk(payload)
		if err != nil {
			return fmt.Errorf("openai adapter: %w", err)
		}

		if token != "" {
			chunkChan <- token
		}
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("openai adapter: read stream: %w", err)
	}

	return nil
}

func parseStreamChunk(raw string) (string, error) {
	var chunk openAIStreamResponse
	if err := jsoniter.Unmarshal([]byte(raw), &chunk); err != nil {
		return "", fmt.Errorf("parse stream chunk: %w", err)
	}

	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			return choice.Delta.Content, nil
		}
	}

	return "", nil
}