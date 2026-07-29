package aiagent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	baseURL           *url.URL
	httpClient        *http.Client
	runClient         *http.Client
	streamClient      *http.Client
	serviceToken      string
	actorSignKey      []byte
	servicePrivateKey ed25519.PrivateKey
	actorPrivateKey   ed25519.PrivateKey
	now               func() time.Time
}

func NewJWTHTTPClient(baseURL, servicePrivateKey, actorPrivateKey string) (*HTTPClient, error) {
	client, err := newBaseHTTPClient(baseURL)
	if err != nil {
		return nil, err
	}
	client.servicePrivateKey, err = parseEd25519PrivateKey(servicePrivateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid API service private key: %v", ErrUnavailable, err)
	}
	client.actorPrivateKey, err = parseEd25519PrivateKey(actorPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid Actor Context private key: %v", ErrUnavailable, err)
	}
	return client, nil
}

func NewHTTPClient(baseURL, serviceToken, actorSigningKey string) (*HTTPClient, error) {
	client, err := newBaseHTTPClient(baseURL)
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

func newBaseHTTPClient(baseURL string) (*HTTPClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: invalid AI agent URL", ErrUnavailable)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: unsupported AI agent URL scheme", ErrUnavailable)
	}
	return &HTTPClient{
		baseURL:      parsed,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		runClient:    &http.Client{Timeout: 10 * time.Second},
		streamClient: &http.Client{},
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
	if len(c.servicePrivateKey) != 0 {
		serviceJWT, signErr := signEdDSAJWT(c.servicePrivateKey, map[string]any{
			"iss": "luna-api", "aud": "luna-agent", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
			"jti": actor.RequestID,
		})
		if signErr != nil {
			return nil, signErr
		}
		actorJWT, signErr := signEdDSAJWT(c.actorPrivateKey, map[string]any{
			"iss": "luna-api", "aud": "luna-agent", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
			"jti": actor.RequestID, "actor": actor,
		})
		if signErr != nil {
			return nil, signErr
		}
		request.Header.Set("Authorization", "Bearer "+serviceJWT)
		request.Header.Set("X-Luna-Actor-Context", actorJWT)
	} else {
		encoded, signature, signErr := c.signActor(actor)
		if signErr != nil {
			return nil, signErr
		}
		request.Header.Set("Authorization", "Bearer "+c.serviceToken)
		request.Header.Set("X-Luna-Actor-Context", encoded)
		request.Header.Set("X-Luna-Actor-Signature", signature)
	}

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

func parseEd25519PrivateKey(value string) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(value)))
	if block == nil {
		return nil, errors.New("PEM block is missing")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	privateKey, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("key is not Ed25519")
	}
	return privateKey, nil
}

func signEdDSAJWT(privateKey ed25519.PrivateKey, claims map[string]any) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	signature := ed25519.Sign(privateKey, []byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
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
