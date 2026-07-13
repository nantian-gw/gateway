package chatbot

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ragContextBudget bounds the rendered detail section (characters). ~12KB keeps
// the context within a few thousand tokens for typical models.
const ragContextBudget = 12000

// BuildRAGContext queries the live cluster, builds a lightweight index scoped to
// the managed GatewayClasses, selects the resources most relevant to query, and
// renders a budgeted two-section Markdown context. It never fails on a single
// missing or unregistered kind. When no GatewayClass is managed by
// controllerName it returns a short notice instead of an empty topology.
func BuildRAGContext(ctx context.Context, cl client.Client, controllerName, query string, logger *slog.Logger) (string, error) {
	if logger == nil {
		logger = slog.Default()
	}
	index, err := collectIndex(ctx, cl, controllerName, logger)
	if err != nil {
		return "", fmt.Errorf("build rag context: %w", err)
	}
	if !index.hasManagedClass {
		return fmt.Sprintf("No managed GatewayClasses found for controller %s", controllerName), nil
	}
	selected, usedFallback := selectRelevant(index, query)
	return renderContext(index, selected, ragContextBudget, usedFallback), nil
}
