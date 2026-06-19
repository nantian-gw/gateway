package chatbot

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
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
func NewOpenAIAdapter(endpoint, apiKey, model string, temperature float64) LLMClient {
	endpoint = strings.TrimRight(endpoint, "/")

	insecureSkipVerify := os.Getenv("CHATBOT_INSECURE_TLS") == "true"
	if insecureSkipVerify {
		slog.Warn("CHATBOT_INSECURE_TLS is enabled — TLS certificate verification is disabled for LLM API calls")
	}

	transport := defaultLLMTransport
	if insecureSkipVerify {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	return &openAIAdapter{
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    model,
		temp:     temperature,
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
		messages = append(messages, openAIMessage{Role: m.Role, Content: m.Content})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: prompt})

	body := openAIChatRequest{
		Model:       a.model,
		Messages:    messages,
		Temperature: a.temp,
		Stream:      true,
	}

	reqBytes, err := json.Marshal(body)
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
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("openai adapter: unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return a.readSSEStream(resp.Body, chunkChan)
}

func (a *openAIAdapter) readSSEStream(r io.Reader, chunkChan chan<- string) error {
	scanner := bufio.NewScanner(r)

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
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		return "", fmt.Errorf("parse stream chunk: %w", err)
	}

	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			return choice.Delta.Content, nil
		}
	}

	return "", nil
}