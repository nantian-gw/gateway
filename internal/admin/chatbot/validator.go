package chatbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backendlb "github.com/nantian-gw/gateway/internal/gatewayexp/backendlb"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator"
)

const (
	validatorGatewayClassControllerIndex = "nantian.dev/infrastructure.gatewayclass.controller-name"
	validatorGatewayClassGatewayIndex    = "nantian.dev/infrastructure.gateway.gatewayclass-name"
)

func validatorScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(gatewayv1alpha2.Install(scheme))
	utilruntime.Must(gatewayv1beta1.Install(scheme))
	utilruntime.Must(gatewayv1alpha3.Install(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(discoveryv1.AddToScheme(scheme))
	utilruntime.Must(backendlb.Install(scheme))
	utilruntime.Must(mcsv1alpha1.Install(scheme))
	return scheme
}

// DryRunValidate parses the given multi-document YAML manifests into
// Kubernetes objects, creates them in a stateless fake client, runs the
// translator, and returns the compiled ir.Snapshot on success.
//
// On failure it returns a descriptive error that can be fed back to an
// LLM for auto-correction.
func DryRunValidate(ctx context.Context, controllerName string, yamlManifests string) (*ir.Snapshot, error) {
	scheme := validatorScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, validatorGatewayClassControllerIndex, func(obj client.Object) []string {
			gwClass, ok := obj.(*gatewayv1.GatewayClass)
			if !ok || gwClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gwClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, validatorGatewayClassGatewayIndex, func(obj client.Object) []string {
			gw, ok := obj.(*gatewayv1.Gateway)
			if !ok || gw.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gw.Spec.GatewayClassName)}
		}).
		Build()

	docs := splitYAMLDocuments(yamlManifests)
	if len(docs) == 0 {
		return nil, fmt.Errorf("dry-run: no YAML documents found")
	}

	for i, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		jsonBytes, err := yaml.YAMLToJSON([]byte(doc))
		if err != nil {
			return nil, fmt.Errorf("dry-run: document %d: YAML to JSON conversion: %w", i+1, err)
		}

		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(jsonBytes, obj); err != nil {
			return nil, fmt.Errorf("dry-run: document %d: decode JSON: %w", i+1, err)
		}

		if err := fakeClient.Create(ctx, obj); err != nil {
			return nil, fmt.Errorf("dry-run: document %d: create in fake client: %w", i+1, err)
		}
	}

	logger := slog.New(slog.DiscardHandler)
	t := translator.New(controllerName, logger)
	snapshot, err := t.Build(ctx, fakeClient)
	if err != nil {
		return nil, fmt.Errorf("dry-run: translator build failed: %w", err)
	}

	return snapshot, nil
}

// collectFullStreamingResponse accumulates all chunks from a streaming
// LLM call and returns the complete response text.
func collectFullStreamingResponse(ctx context.Context, llm LLMClient, prompt string, history []Message) (string, error) {
	chunkChan := make(chan string, 64)
	errCh := make(chan error, 1)

	go func() {
		errCh <- llm.ChatCompletionStream(ctx, prompt, history, chunkChan)
		close(chunkChan)
	}()

	var sb strings.Builder
	for chunk := range chunkChan {
		sb.WriteString(chunk)
	}

	if err := <-errCh; err != nil {
		return "", fmt.Errorf("collect streaming response: %w", err)
	}

	return sb.String(), nil
}

const (
	defaultMaxRetries = 2
	correctionPrompt  = "The YAML manifests you generated failed Kubernetes validation with the following error:\n\n%s\n\nPlease fix the manifests and regenerate them. Output ONLY the corrected YAML, no other text."
)

// backoffDuration returns an exponentially increasing duration with ±25% jitter,
// capped at max.
func backoffDuration(attempt int, base time.Duration, max time.Duration) time.Duration {
	d := base * time.Duration(math.Pow(2, float64(attempt)))
	if d > max {
		d = max
	}
	// Add ±25% jitter
	jitter := time.Duration(float64(d) * 0.25 * (2*float64(time.Now().UnixNano()%100)/100 - 1))
	return d + jitter
}

// AutoCorrectGenerate sends a user query to the LLM, feeds the generated
// YAML manifests through DryRunValidate, and auto-corrects on failure.
//
// It retries up to maxRetries times (default 2), appending the compiler
// error as feedback to the conversation history on each retry.
//
// On success it returns the validated ir.Snapshot. On persistent failure
// it returns the last validation error.
func AutoCorrectGenerate(
	ctx context.Context,
	llm LLMClient,
	systemPrompt string,
	controllerName string,
	userQuery string,
	maxRetries int,
) (*ir.Snapshot, error) {
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	history := []Message{
		{Role: "system", Content: systemPrompt},
	}

	llmResponse, err := collectFullStreamingResponse(ctx, llm, userQuery, history)
	if err != nil {
		return nil, fmt.Errorf("auto-correct: initial generation: %w", err)
	}

	history = append(history,
		Message{Role: "assistant", Content: llmResponse},
	)

	snapshot, err := DryRunValidate(ctx, controllerName, llmResponse)
	if err == nil {
		return snapshot, nil
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		delay := backoffDuration(attempt-1, 1*time.Second, 30*time.Second)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}

		feedback := fmt.Sprintf(correctionPrompt, err.Error())

		correctedResponse, retryErr := collectFullStreamingResponse(ctx, llm, feedback, history)
		if retryErr != nil {
			return nil, fmt.Errorf("auto-correct: retry %d generation: %w", attempt, retryErr)
		}

		history = append(history,
			Message{Role: "assistant", Content: correctedResponse},
		)

		snapshot, err = DryRunValidate(ctx, controllerName, correctedResponse)
		if err == nil {
			return snapshot, nil
		}
	}

	return nil, fmt.Errorf("auto-correct: validation failed after %d retries: %w", maxRetries, err)
}

func splitYAMLDocuments(raw string) []string {
	parts := strings.Split(raw, "\n---")
	if len(parts) == 1 {
		parts = strings.Split(raw, "\r\n---")
	}

	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || trimmed == "---" {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "---\n")
		result = append(result, trimmed)
	}
	return result
}