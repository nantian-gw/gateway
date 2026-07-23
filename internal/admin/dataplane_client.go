package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type DataplaneClientConfig struct {
	Timeout     time.Duration
	BearerToken string
}

type DataplaneClient struct {
	client *http.Client
	token  string
}

func NewDataplaneClient(config DataplaneClientConfig) *DataplaneClient {
	return &DataplaneClient{
		client: &http.Client{
			Timeout: config.Timeout,
		},
		token: config.BearerToken,
	}
}

func (c *DataplaneClient) GetJSON(ctx context.Context, baseURL, path string, out any) error {
	url := baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
