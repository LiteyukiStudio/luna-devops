package gitprovider

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
		httpClient: NewHTTPClientForPolicy(policy, gitHTTPTimeout),
		provider:   provider,
		token:      strings.TrimSpace(token),
		policy:     policy,
	}
}

type gitPublicRedirectContextKey struct{}

type gitOriginScopedTransport struct {
	primary http.RoundTripper
	public  http.RoundTripper
}

func (t gitOriginScopedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.public != nil && req.Context().Value(gitPublicRedirectContextKey{}) == true {
		return t.public.RoundTrip(req)
	}
	return t.primary.RoundTrip(req)
}

func (t gitOriginScopedTransport) CloseIdleConnections() {
	type idleCloser interface{ CloseIdleConnections() }
	if closer, ok := t.primary.(idleCloser); ok {
		closer.CloseIdleConnections()
	}
	if closer, ok := t.public.(idleCloser); ok {
		closer.CloseIdleConnections()
	}
}

func NewHTTPClientForPolicy(policy security.EgressPolicy, timeout time.Duration) *http.Client {
	client := security.NewHTTPClient(policy, timeout)
	if policy.AllowPrivateNetwork {
		publicPolicy := policy
		publicPolicy.AllowPrivateNetwork = false
		publicPolicy.ApplyIPFilterToNames = true
		publicClient := security.NewHTTPClient(publicPolicy, timeout)
		client.Transport = gitOriginScopedTransport{primary: client.Transport, public: publicClient.Transport}
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("stopped after 5 redirects")
		}
		if req.URL.User != nil {
			return security.ErrInvalidURL
		}
		initial := via[0].URL
		sawHTTPS := false
		crossedOrigin := !sameGitURLOrigin(initial, req.URL)
		for _, prev := range via {
			if strings.EqualFold(prev.URL.Scheme, "https") {
				sawHTTPS = true
			}
			if !sameGitURLOrigin(initial, prev.URL) {
				crossedOrigin = true
			}
		}
		if sawHTTPS && !strings.EqualFold(req.URL.Scheme, "https") {
			return errors.New("refusing https to http redirect")
		}
		if _, err := policy.ValidateURL(req.URL.String()); err != nil {
			return err
		}
		if crossedOrigin {
			req.Header.Del("Authorization")
			if policy.AllowPrivateNetwork {
				publicPolicy := policy
				publicPolicy.AllowPrivateNetwork = false
				publicPolicy.ApplyIPFilterToNames = true
				if err := publicPolicy.ValidateProxyTarget(req.Context(), req.URL); err != nil {
					return err
				}
				*req = *req.WithContext(context.WithValue(req.Context(), gitPublicRedirectContextKey{}, true))
			}
		}
		return nil
	}
	return client
}

func sameGitURLOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(effectiveGitHostPort(left), effectiveGitHostPort(right))
}

func effectiveGitHostPort(u *url.URL) string {
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	if port == "" {
		return host
	}
	return host + ":" + port
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

func (c Client) delete(ctx context.Context, requestURL string) error {
	if _, err := c.policy.ValidateURL(requestURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, requestURL, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return decodeGitResponse(resp, nil)
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
