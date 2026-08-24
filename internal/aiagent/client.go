package aiagent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

var ErrUnavailable = errors.New("ai agent is unavailable")

type ActorContext struct {
	UserID           string `json:"userId"`
	SessionID        string `json:"sessionId"`
	OAuthGrantID     string `json:"oauthGrantId,omitempty"`
	RunID            string `json:"runId,omitempty"`
	ProjectID        string `json:"projectId,omitempty"`
	Locale           string `json:"locale"`
	IssuedAt         int64  `json:"issuedAt"`
	ExpiresAt        int64  `json:"expiresAt"`
	RequestID        string `json:"requestId"`
	SessionExpiresAt int64  `json:"-"`
}

type Request struct {
	Method         string
	Path           string
	Query          url.Values
	Body           []byte
	ContentType    string
	LastEventID    string
	IdempotencyKey string
	Stream         bool
}

type Response struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

type Client interface {
	Do(context.Context, ActorContext, Request) (*Response, error)
}

type HTTPClient struct {
	baseURL      *url.URL
	httpClient   *http.Client
	runClient    *http.Client
	streamClient *http.Client
	serviceToken string
	actorSignKey []byte
	now          func() time.Time
}

func NewHTTPClient(baseURL, serviceToken, actorSigningKey string) (*HTTPClient, error) {
	return NewHTTPClientWithTimeout(baseURL, serviceToken, actorSigningKey, 10*time.Second)
}

func NewHTTPClientWithTimeout(baseURL, serviceToken, actorSigningKey string, timeout time.Duration) (*HTTPClient, error) {
	client, err := newBaseHTTPClient(baseURL, timeout)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(serviceToken) == "" || strings.TrimSpace(actorSigningKey) == "" {
		return nil, fmt.Errorf("%w: missing AI agent trust material", ErrUnavailable)
	}
	client.serviceToken = strings.TrimSpace(serviceToken)
	client.actorSignKey = []byte(actorSigningKey)
	return client, nil
}

func newBaseHTTPClient(baseURL string, timeout time.Duration) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: invalid AI agent URL", ErrUnavailable)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: unsupported AI agent URL scheme", ErrUnavailable)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("%w: invalid AI agent timeout", ErrUnavailable)
	}
	return &HTTPClient{
		baseURL:      parsed,
		httpClient:   telemetry.InstrumentHTTPClient(&http.Client{Timeout: timeout}),
		runClient:    telemetry.InstrumentHTTPClient(&http.Client{Timeout: timeout}),
		streamClient: telemetry.InstrumentHTTPClient(&http.Client{}),
		now:          time.Now,
	}, nil
}

func (c *HTTPClient) Do(ctx context.Context, actor ActorContext, input Request) (*Response, error) {
	target := *c.baseURL
	target.Path = strings.TrimRight(target.Path, "/") + "/" + strings.TrimLeft(input.Path, "/")
	target.RawQuery = input.Query.Encode()

	request, err := http.NewRequestWithContext(ctx, input.Method, target.String(), bytes.NewReader(input.Body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if input.Stream {
		request.Header.Set("Accept", "text/event-stream")
	}
	if input.ContentType != "" {
		request.Header.Set("Content-Type", input.ContentType)
	}
	if input.LastEventID != "" {
		request.Header.Set("Last-Event-ID", input.LastEventID)
	}
	if input.IdempotencyKey != "" {
		request.Header.Set("Idempotency-Key", input.IdempotencyKey)
	}

	now := c.now()
	actor.IssuedAt = now.Unix()
	actor.ExpiresAt = now.Add(time.Minute).Unix()
	encoded, signature, signErr := c.signActor(actor)
	if signErr != nil {
		return nil, signErr
	}
	request.Header.Set("Authorization", "Bearer "+c.serviceToken)
	request.Header.Set("X-Luna-Actor-Context", encoded)
	request.Header.Set("X-Luna-Actor-Signature", signature)

	client := c.httpClient
	if input.Stream {
		client = c.streamClient
	} else if input.Method == http.MethodPost && (strings.HasSuffix(input.Path, "/turns") || strings.HasSuffix(input.Path, "/runs")) {
		client = c.runClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return &Response{StatusCode: response.StatusCode, Header: response.Header, Body: response.Body}, nil
}

func (c *HTTPClient) signActor(actor ActorContext) (string, string, error) {
	payload, err := json.Marshal(actor)
	if err != nil {
		return "", "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, c.actorSignKey)
	_, _ = mac.Write([]byte(encoded))
	return encoded, "sha256=" + hex.EncodeToString(mac.Sum(nil)), nil
}
