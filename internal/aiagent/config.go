package aiagent

import (
	"fmt"
	"strings"
	"time"
)

type Config struct {
	Available                    bool
	BaseURL                      string
	ServiceToken                 string
	ActorSigningKey              string
	CallbackServiceToken         string
	PreviousCallbackServiceToken string
	Timeout                      time.Duration
}

// NewConfig builds an immutable AI Agent client snapshot from values already
// parsed by the process configuration adapter.
func NewConfig(available bool, baseURL string, timeout time.Duration, internalSecret string) (Config, error) {
	return NewConfigWithPrevious(available, baseURL, timeout, internalSecret, "")
}

func NewConfigWithPrevious(available bool, baseURL string, timeout time.Duration, internalSecret, previousInternalSecret string) (Config, error) {
	cfg := Config{
		Available: available,
		BaseURL:   strings.TrimSpace(baseURL),
		Timeout:   timeout,
	}
	if cfg.Timeout <= 0 {
		return Config{}, fmt.Errorf("%w: AI_AGENT_TIMEOUT must be a positive duration", ErrUnavailable)
	}
	if strings.TrimSpace(internalSecret) != "" {
		keys, err := DeriveInternalKeys(internalSecret)
		if err != nil {
			return Config{}, err
		}
		cfg.ServiceToken = keys.ServiceToken
		cfg.ActorSigningKey = keys.ActorSigningKey
		cfg.CallbackServiceToken = keys.CallbackServiceToken
	}
	if strings.TrimSpace(previousInternalSecret) != "" {
		previousKeys, err := DeriveInternalKeys(previousInternalSecret)
		if err != nil {
			return Config{}, fmt.Errorf("AI_INTERNAL_SECRET_PREVIOUS must contain at least 32 bytes")
		}
		if strings.TrimSpace(previousInternalSecret) != strings.TrimSpace(internalSecret) {
			cfg.PreviousCallbackServiceToken = previousKeys.CallbackServiceToken
		}
	}
	if !cfg.Available {
		return cfg, nil
	}
	if _, err := cfg.Client(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Client() (Client, error) {
	if !c.Available {
		return nil, nil
	}
	if c.Timeout <= 0 {
		return nil, fmt.Errorf("%w: invalid AI_AGENT_TIMEOUT", ErrUnavailable)
	}
	client, err := NewHTTPClientWithTimeout(c.BaseURL, c.ServiceToken, c.ActorSigningKey, c.Timeout)
	if err != nil {
		return nil, err
	}
	return client, nil
}
