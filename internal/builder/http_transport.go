package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPTransport struct {
	apiURL  string
	token   string
	agentID string
	client  *http.Client
}

func NewHTTPTransport(options Options) (*HTTPTransport, error) {
	if strings.TrimSpace(options.APIURL) == "" {
		return nil, errors.New("builder api url is required")
	}
	if strings.TrimSpace(options.Token) == "" {
		return nil, errors.New("builder token is required")
	}
	return &HTTPTransport{
		apiURL:  strings.TrimRight(options.APIURL, "/"),
		token:   strings.TrimSpace(options.Token),
		agentID: strings.TrimSpace(options.AgentID),
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (t *HTTPTransport) Heartbeat(ctx context.Context, heartbeat Heartbeat) error {
	return t.post(ctx, "/api/v1/builder/heartbeat", heartbeat, nil)
}

func (t *HTTPTransport) Claim(ctx context.Context) (Task, error) {
	var task Task
	err := t.post(ctx, "/api/v1/builder/tasks/claim", map[string]any{"agentId": t.agentID}, &task)
	return task, err
}

func (t *HTTPTransport) AppendLogs(ctx context.Context, jobID string, content string) error {
	return t.post(ctx, fmt.Sprintf("/api/v1/builder/tasks/%s/logs?agentId=%s", jobID, url.QueryEscape(t.agentID)), map[string]string{"content": content}, nil)
}

func (t *HTTPTransport) Complete(ctx context.Context, jobID string, result Result) error {
	return t.post(ctx, fmt.Sprintf("/api/v1/builder/tasks/%s/complete?agentId=%s", jobID, url.QueryEscape(t.agentID)), result, nil)
}

func (t *HTTPTransport) Fail(ctx context.Context, jobID string, message string) error {
	return t.post(ctx, fmt.Sprintf("/api/v1/builder/tasks/%s/fail?agentId=%s", jobID, url.QueryEscape(t.agentID)), map[string]string{"message": message}, nil)
}

func (t *HTTPTransport) Close() error {
	return nil
}

func (t *HTTPTransport) post(ctx context.Context, path string, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+t.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent && output != nil {
		return errNoTask
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("builder api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if output == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(output)
}
