package aitool

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/LiteyukiStudio/devops/internal/security"
)

var (
	ErrForbidden          = errors.New("AI tool target is forbidden")
	ErrInvalidInput       = errors.New("AI tool input is invalid")
	ErrWebTargetBlocked   = errors.New("AI web target is blocked")
	ErrWebRequestFailed   = errors.New("AI web request failed")
	ErrWebContentRejected = errors.New("AI web content is not readable")
)

const defaultDuckDuckGoSearchEndpoint = "https://html.duckduckgo.com/html/"

type Request struct {
	OperationID string
	UserID      string
	Arguments   map[string]any
}

type Result struct {
	Value     any
	Truncated bool
}

type Service struct {
	webPolicyProvider WebPolicyProvider
	webProxyProvider  WebProxyProvider
	webSearchEndpoint string
	webProxyCursor    atomic.Uint64
}

type WebPolicyProvider func(context.Context, string) (security.EgressPolicy, error)
type WebProxyProvider func(context.Context, string) ([]string, error)

type ServiceOption func(*Service)

func WithWebPolicyProvider(provider WebPolicyProvider) ServiceOption {
	return func(service *Service) {
		service.webPolicyProvider = provider
	}
}

func WithWebProxyProvider(provider WebProxyProvider) ServiceOption {
	return func(service *Service) {
		service.webProxyProvider = provider
	}
}

func WithWebSearchEndpoint(endpoint string) ServiceOption {
	return func(service *Service) {
		service.webSearchEndpoint = strings.TrimSpace(endpoint)
	}
}

func NewService(options ...ServiceOption) *Service {
	service := &Service{webSearchEndpoint: defaultDuckDuckGoSearchEndpoint}
	for _, option := range options {
		option(service)
	}
	if service.webSearchEndpoint == "" {
		service.webSearchEndpoint = defaultDuckDuckGoSearchEndpoint
	}
	return service
}

func (s *Service) Execute(ctx context.Context, input Request) (Result, error) {
	if s == nil || strings.TrimSpace(input.UserID) == "" {
		return Result{}, ErrForbidden
	}
	switch input.OperationID {
	case "webSearch":
		return s.webSearch(ctx, input.UserID, input.Arguments)
	case "fetchWebPage":
		return s.fetchWebPage(ctx, input.UserID, input.Arguments)
	default:
		return Result{}, ErrForbidden
	}
}

func stringArgument(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return value
}

func intArgument(arguments map[string]any, key string) int {
	switch value := arguments[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}
