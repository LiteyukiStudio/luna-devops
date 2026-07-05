package gatewayprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Reporter interface {
	Report(ctx context.Context, window RouteUsageWindow) error
}

type APIReporter struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewAPIReporter(baseURL string, token string, timeout time.Duration) *APIReporter {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &APIReporter{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: timeout},
	}
}

func (r *APIReporter) Report(ctx context.Context, window RouteUsageWindow) error {
	payload := gatewayTrafficPayload{
		RouteID:       window.RouteID,
		ResponseBytes: window.ResponseBytes,
		RequestCount:  window.RequestCount,
		PeriodStart:   window.PeriodStart.UTC().Format(time.RFC3339),
		PeriodEnd:     window.PeriodEnd.UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/api/v1/billing/gateway-traffic", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("report gateway traffic returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

type gatewayTrafficPayload struct {
	RouteID       string `json:"routeId"`
	ResponseBytes int64  `json:"responseBytes"`
	RequestCount  int64  `json:"requestCount"`
	PeriodStart   string `json:"periodStart"`
	PeriodEnd     string `json:"periodEnd"`
}
