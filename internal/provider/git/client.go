package gitprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
)

const gitHTTPTimeout = 15 * time.Second

type Client struct {
	httpClient *http.Client
	provider   model.GitProvider
	token      string
	policy     security.EgressPolicy
}

func NewClient(provider model.GitProvider, token string) Client {
	return NewClientWithPolicy(provider, token, security.PublicEgressPolicy())
}

func NewClientWithPolicy(provider model.GitProvider, token string, policy security.EgressPolicy) Client {
	return Client{
		httpClient: security.NewHTTPClient(policy, gitHTTPTimeout),
		provider:   provider,
		token:      strings.TrimSpace(token),
		policy:     policy,
	}
}

func (c Client) getJSON(ctx context.Context, requestURL string, output any) error {
	if _, err := c.policy.ValidateURL(requestURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeGitResponse(resp, output)
}

func (c Client) postJSON(ctx context.Context, requestURL string, input, output any) error {
	if _, err := c.policy.ValidateURL(requestURL); err != nil {
		return err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeGitResponse(resp, output)
}

func (c Client) authorize(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	if c.provider.Type == "github" {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (c Client) apiURL(apiPath string, params map[string]string) string {
	base := strings.TrimRight(c.provider.BaseURL, "/")
	switch c.provider.Type {
	case "github":
		if base == "" || base == "https://github.com" {
			base = "https://api.github.com"
		} else if !strings.Contains(base, "/api/") {
			base += "/api/v3"
		}
	case "gitea":
		base += "/api/v1"
	}
	parsed, _ := url.Parse(base + apiPath)
	query := parsed.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (c Client) CurrentUser(ctx context.Context) (UserResponse, error) {
	switch c.provider.Type {
	case "github", "gitea":
		var user UserResponse
		err := c.getJSON(ctx, c.apiURL("/user", nil), &user)
		return user, err
	default:
		return UserResponse{}, fmt.Errorf("git provider type %q is not supported", c.provider.Type)
	}
}

func decodeGitResponse(resp *http.Response, output any) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeUpstreamError(resp.StatusCode, body)
	}
	if output == nil || len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, output)
}

func decodeUpstreamError(statusCode int, body []byte) error {
	var response struct {
		Message          string                `json:"message"`
		DocumentationURL string                `json:"documentation_url"`
		Errors           []UpstreamErrorDetail `json:"errors"`
	}
	if err := json.Unmarshal(body, &response); err == nil && (strings.TrimSpace(response.Message) != "" || len(response.Errors) > 0) {
		return &UpstreamError{
			StatusCode:       statusCode,
			Message:          strings.TrimSpace(response.Message),
			DocumentationURL: strings.TrimSpace(response.DocumentationURL),
			Details:          response.Errors,
		}
	}
	return &UpstreamError{
		StatusCode: statusCode,
		Message:    http.StatusText(statusCode),
	}
}
