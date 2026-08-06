package metrics

import (
	"context"
	jsoniter "github.com/json-iterator/go"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultQueryTimeout = 2 * time.Second

// PrometheusResponse mirrors the Prometheus HTTP API v1 response envelope.
type PrometheusResponse struct {
	Status string          `json:"status"`
	Data   jsoniter.RawMessage `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// PrometheusClient queries a Prometheus HTTP API.
type PrometheusClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPrometheusClient returns a client for the given Prometheus base URL.
func NewPrometheusClient(baseURL string) *PrometheusClient {
	return &PrometheusClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultQueryTimeout,
		},
	}
}

// InstantQuery executes an instant PromQL query against the Prometheus HTTP API
// and returns the raw JSON response.
//
// Endpoint: GET /api/v1/query
func (c *PrometheusClient) InstantQuery(ctx context.Context, query string) (*PrometheusResponse, error) {
	if query == "" {
		return nil, fmt.Errorf("prometheus: query is empty")
	}

	apiURL := fmt.Sprintf("%s/api/v1/query?query=%s", c.baseURL, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("prometheus: failed to build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("prometheus: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var promResp PrometheusResponse
	if err := jsoniter.Unmarshal(body, &promResp); err != nil {
		return nil, fmt.Errorf("prometheus: failed to parse response: %w", err)
	}

	if promResp.Status != "success" {
		return nil, fmt.Errorf("prometheus: query error: %s", promResp.Error)
	}

	return &promResp, nil
}

// RangeQuery executes a range PromQL query against the Prometheus HTTP API
// and returns the raw JSON response.
//
// Endpoint: GET /api/v1/query_range
func (c *PrometheusClient) RangeQuery(ctx context.Context, query, start, end, step string) (*PrometheusResponse, error) {
	if query == "" {
		return nil, fmt.Errorf("prometheus: query is empty")
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("start", start)
	params.Set("end", end)
	params.Set("step", step)
	apiURL := fmt.Sprintf("%s/api/v1/query_range?%s", c.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("prometheus: failed to build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("prometheus: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var promResp PrometheusResponse
	if err := jsoniter.Unmarshal(body, &promResp); err != nil {
		return nil, fmt.Errorf("prometheus: failed to parse response: %w", err)
	}

	if promResp.Status != "success" {
		return nil, fmt.Errorf("prometheus: query error: %s", promResp.Error)
	}

	return &promResp, nil
}
