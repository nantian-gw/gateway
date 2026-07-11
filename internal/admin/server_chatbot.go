package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/nantian-gw/gateway/internal/admin/chatbot"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	chatbotConfigNamespace = "nantian-gw"
	chatbotConfigSecret    = "chatbot-config"
	maxChatHistoryMessages = 40
)

// maskAPIKey returns a masked version of the API key for safe display.
// Example: "sk-proj-abc123xyz" → "sk-••••3xyz"
func maskAPIKey(key string) string {
	if len(key) <= 7 {
		return strings.Repeat("*", len(key))
	}
	return key[:3] + "••••" + key[len(key)-4:]
}

// isMaskedKey returns true when the provided key starts with the masking prefix,
// indicating the caller sent a masked value that should not overwrite the real secret.
func isMaskedKey(key string) bool {
	return strings.Contains(key, "••••")
}

// handleChatbotConfig (GET) reads the chatbot configuration from the Kubernetes
// Secret and returns it as JSON with the API key masked for security.
func (s *Server) handleChatbotConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := chatbot.LoadConfig(r.Context(), s.resources.client, chatbotConfigNamespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.respondJSON(w, map[string]any{
				"configured": false,
				"provider":   "",
				"apiEndpoint": "",
				"apiKey":     "",
				"model":      "",
				"temperature": 0.0,
			})
			return
		}
		s.logger.Error("failed to load chatbot config", "error", err)
		s.respondRequestError(w, err)
		return
	}

	s.respondJSON(w, map[string]any{
		"configured":  true,
		"provider":    cfg.Provider,
		"apiEndpoint": cfg.APIEndpoint,
		"apiKey":      maskAPIKey(cfg.APIKey),
		"model":       cfg.Model,
		"temperature": cfg.Temperature,
	})
}

type chatbotConfigRequest struct {
	Provider    string  `json:"provider"`
	APIEndpoint string  `json:"apiEndpoint"`
	APIKey      string  `json:"apiKey"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}

// handleChatbotConfigPut (PUT) writes the chatbot configuration into the
// Kubernetes Secret "chatbot-config" in the nantian-gw namespace.
// If the API key in the request is masked (contains "••••"), the existing
// key is preserved.
func (s *Server) handleChatbotConfigPut(w http.ResponseWriter, r *http.Request) {
	var req chatbotConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondRequestError(w, errInvalidRequest("invalid request body: "+err.Error()))
		return
	}

	k8sClient := s.resources.client
	ctx := r.Context()

	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: chatbotConfigNamespace, Name: chatbotConfigSecret}
	getErr := k8sClient.Get(ctx, key, secret)

	data := map[string]string{}

	// Preserve existing api-key when the request contains a masked key.
	if req.APIKey != "" && !isMaskedKey(req.APIKey) {
		data["api-key"] = req.APIKey
	} else if getErr == nil && secret.Data != nil {
		if existingKey, ok := secret.Data["api-key"]; ok {
			data["api-key"] = string(existingKey)
		}
	}

	if req.Provider != "" {
		data["provider"] = req.Provider
	}
	if req.APIEndpoint != "" {
		data["api-endpoint"] = req.APIEndpoint
	}
	if req.Model != "" {
		data["model"] = req.Model
	}
	if req.Temperature != 0 || len(data) > 0 {
		data["temperature"] = fmt.Sprintf("%.2f", req.Temperature)
	}

	// If the secret doesn't exist, create it; otherwise update.
	if apierrors.IsNotFound(getErr) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: chatbotConfigNamespace,
				Name:      chatbotConfigSecret,
			},
			Data: stringMapToBytes(data),
		}
		if err := k8sClient.Create(ctx, secret); err != nil {
			s.logger.Error("failed to create chatbot config secret", "error", err)
			s.respondRequestError(w, err)
			return
		}
	} else if getErr != nil {
		s.logger.Error("failed to read chatbot config secret", "error", getErr)
		s.respondRequestError(w, getErr)
		return
	} else {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		for k, v := range data {
			secret.Data[k] = []byte(v)
		}
		if err := k8sClient.Update(ctx, secret); err != nil {
			s.logger.Error("failed to update chatbot config secret", "error", err)
			s.respondRequestError(w, err)
			return
		}
	}

	s.logger.Info("chatbot config updated")
	s.respondJSON(w, map[string]any{"status": "ok"})
}

// chatRequest is the payload for a chatbot chat completion request.
type chatRequest struct {
	Prompt  string            `json:"prompt"`
	History []chatbot.Message `json:"history"`
}

// handleChatbotChat (POST) streams an LLM chat completion to the client via SSE.
// It loads the chatbot configuration, builds dynamic RAG context, and streams
// token-level responses. YAML manifests found in the response are intercepted
// and dry-run validated with auto-correction on failure.
func (s *Server) handleChatbotChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondRequestError(w, errInvalidRequest("invalid request body: "+err.Error()))
		return
	}

	if req.Prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	// Load chatbot configuration.
	cfg, err := chatbot.LoadConfig(r.Context(), s.resources.client, chatbotConfigNamespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "chatbot not configured", http.StatusBadRequest)
			return
		}
		s.logger.Error("failed to load chatbot config for chat", "error", err)
		s.respondRequestError(w, errServiceUnavailable("chatbot config unavailable: "+err.Error()))
		return
	}

	if err := cfg.Validate(); err != nil {
		s.logger.Error("invalid chatbot config", "error", err)
		s.respondRequestError(w, errInvalidRequest("invalid chatbot config: "+err.Error()))
		return
	}

	// Build dynamic RAG context from the live cluster.
	ragContext, err := chatbot.BuildRAGContext(r.Context(), s.resources.client, "gateway.networking.k8s.io/nantian-gw", req.Prompt)
	if err != nil {
		s.logger.Warn("failed to build RAG context, continuing without it", "error", err)
		ragContext = ""
	}

	// Prep the system prompt with RAG context.
	systemPrompt := buildSystemPrompt(ragContext)

	// Build the full history (with a bound to prevent unbounded growth).
	history := append([]chatbot.Message(nil), req.History...)
	if len(history) > maxChatHistoryMessages {
		history = history[len(history)-maxChatHistoryMessages:]
	}

	// Create the LLM adapter.
	llm := chatbot.NewOpenAIAdapter(cfg.APIEndpoint, cfg.APIKey, cfg.Model, cfg.Temperature)

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Stream the LLM response.
	chunkChan := make(chan string, 64)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel() // ensure cancel is called on all exit paths

	go func() {
		// Prepend system prompt as first message when needed.
		fullHistory := prependSystemMessage(history, systemPrompt)
		errCh <- llm.ChatCompletionStream(ctx, req.Prompt, fullHistory, chunkChan)
		close(chunkChan)
	}()

	// Accumulate chunks to detect YAML blocks.
	var fullResponse strings.Builder

	for chunk := range chunkChan {
		fullResponse.WriteString(chunk)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", chunk); err != nil {
			cancel() // cancel the LLM request context to stop token generation
			return
		}
		flusher.Flush()
	}

	if err := <-errCh; err != nil {
		s.logger.Error("chatbot streaming error", "error", err)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", escapeSSEData(err.Error()))
		flusher.Flush()
		return
	}

	// After the stream completes, check for YAML manifests and validate.
	responseText := fullResponse.String()
	yamlBlocks := extractYAMLBlocks(responseText)
	if len(yamlBlocks) == 0 {
		// Signal end of stream.
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	// Validate YAML manifests.
	for _, yamlBlock := range yamlBlocks {
		_, err := chatbot.DryRunValidate(r.Context(), "gateway.networking.k8s.io/nantian-gw", yamlBlock)
		if err == nil {
			// Validation succeeded.
			fmt.Fprintf(w, "event: dry_run_status\ndata: {\"success\":true}\n\n")
			fmt.Fprintf(w, "event: manifests\ndata: %s\n\n", escapeSSEData(yamlBlock))
			flusher.Flush()
		} else {
			// Validation failed – stream auto-correction progress.
			fmt.Fprintf(w, "event: dry_run_status\ndata: {\"success\":false,\"error\":\"%s\"}\n\n",
				escapeSSEData(err.Error()))
			fmt.Fprintf(w, "event: manifests\ndata: %s\n\n", escapeSSEData(yamlBlock))
			flusher.Flush()

			runAutoCorrectSSE(r.Context(), w, flusher, llm, systemPrompt, req.Prompt, history, err)
		}
	}

	// Signal end of stream.
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// runAutoCorrectSSE streams the auto-correction loop progress via SSE.
func runAutoCorrectSSE(
	ctx context.Context,
	w http.ResponseWriter,
	flusher http.Flusher,
	llm chatbot.LLMClient,
	systemPrompt string,
	userQuery string,
	history []chatbot.Message,
	initialErr error,
) {
	const maxRetries = 2

	correctionHistory := append([]chatbot.Message(nil), history...)
	correctionHistory = append(correctionHistory,
		chatbot.Message{Role: "system", Content: systemPrompt},
		chatbot.Message{Role: "user", Content: userQuery},
	)

	lastErr := initialErr
	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Fprintf(w, "event: correction_progress\ndata: {\"attempt\":%d,\"maxRetries\":%d}\n\n",
			attempt, maxRetries)
		flusher.Flush()

		feedback := fmt.Sprintf(
			"The YAML manifests you generated failed Kubernetes validation with the following error:\n\n%s\n\nPlease fix the manifests and regenerate them. Output ONLY the corrected YAML, no other text.",
			lastErr.Error(),
		)

		chunkChan := make(chan string, 64)
		errCh := make(chan error, 1)
		go func() {
			errCh <- llm.ChatCompletionStream(ctx, feedback, correctionHistory, chunkChan)
			close(chunkChan)
		}()

		var corrected strings.Builder
		for chunk := range chunkChan {
			corrected.WriteString(chunk)
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}

		if streamErr := <-errCh; streamErr != nil {
			fmt.Fprintf(w, "event: correction_error\ndata: {\"error\":\"%s\"}\n\n",
				escapeSSEData(streamErr.Error()))
			flusher.Flush()
			return
		}

		correctionHistory = append(correctionHistory,
			chatbot.Message{Role: "assistant", Content: corrected.String()},
		)

		// Extract YAML from the corrected response and validate.
		correctedYAMLBlocks := extractYAMLBlocks(corrected.String())
		validated := false
		for _, yb := range correctedYAMLBlocks {
			_, dryRunErr := chatbot.DryRunValidate(ctx, "gateway.networking.k8s.io/nantian-gw", yb)
			if dryRunErr == nil {
				validated = true
				fmt.Fprintf(w, "event: dry_run_status\ndata: {\"success\":true,\"corrected\":true}\n\n")
				fmt.Fprintf(w, "event: manifests\ndata: %s\n\n", escapeSSEData(yb))
				flusher.Flush()
				break
			}
			lastErr = dryRunErr
		}

		if validated {
			return
		}
	}

	fmt.Fprintf(w, "event: correction_failed\ndata: {\"error\":\"validation failed after %d retries\"}\n\n",
		maxRetries)
	flusher.Flush()
}

// yamlBlockPattern matches fenced YAML code blocks.
var yamlBlockPattern = regexp.MustCompile("(?s)```ya?ml\\s*\\n(.*?)```")

// extractYAMLBlocks extracts YAML content from ```yaml ... ``` fenced code blocks
// in the LLM response.
func extractYAMLBlocks(text string) []string {
	matches := yamlBlockPattern.FindAllStringSubmatch(text, -1)
	if matches == nil {
		return nil
	}

	blocks := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			blocks = append(blocks, strings.TrimSpace(m[1]))
		}
	}
	return blocks
}

// escapeSSEData escapes a string for safe inclusion in an SSE data field.
// Splits multi-line content into separate data: lines per SSE spec.
func escapeSSEData(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// buildSystemPrompt constructs the system prompt with RAG context.
func buildSystemPrompt(ragContext string) string {
	var sb strings.Builder
	sb.WriteString("You are a Kubernetes Gateway API expert assistant for Nantian Gateway. ")
	sb.WriteString("You help users understand their gateway topology, troubleshoot routing issues, ")
	sb.WriteString("and generate valid Gateway API YAML manifests.\n\n")
	sb.WriteString("When generating YAML manifests, always wrap them in ```yaml ... ``` code blocks. ")
	sb.WriteString("Use only Gateway API v1 resources: Gateway, GatewayClass, HTTPRoute, GRPCRoute, ")
	sb.WriteString("TLSRoute, TCPRoute, UDPRoute. For backends, reference Services by namespace/name.\n\n")
	sb.WriteString("Verify all gatewayClassName references against known GatewayClasses in the topology.\n\n")

	if ragContext != "" {
		sb.WriteString(ragContext)
	}

	return sb.String()
}

// prependSystemMessage adds a system message at the beginning of the history
// if one is not already present.
func prependSystemMessage(history []chatbot.Message, systemPrompt string) []chatbot.Message {
	for _, m := range history {
		if m.Role == "system" {
			return history
		}
	}

	result := make([]chatbot.Message, 0, len(history)+1)
	result = append(result, chatbot.Message{Role: "system", Content: systemPrompt})
	result = append(result, history...)
	return result
}

func stringMapToBytes(m map[string]string) map[string][]byte {
	out := make(map[string][]byte, len(m))
	for k, v := range m {
		out[k] = []byte(v)
	}
	return out
}