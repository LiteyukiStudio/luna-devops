package aiagent

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Available       bool
	BaseURL         string
	ServiceToken    string
	ActorSigningKey string
	Timeout         string
}

func LoadConfig() (Config, error) {
	baseURL := strings.TrimSpace(os.Getenv("AI_AGENT_BASE_URL"))
	available, err := parseBool(os.Getenv("AI_ASSISTANT_AVAILABLE"))
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Available: available,
		BaseURL:   baseURL,
		Timeout:   strings.TrimSpace(os.Getenv("AI_AGENT_TIMEOUT")),
	}
	if cfg.Timeout != "" {
		parsed, parseErr := time.ParseDuration(cfg.Timeout)
		if parseErr != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("%w: AI_AGENT_TIMEOUT must be a positive duration", ErrUnavailable)
		}
	}
	if !cfg.Available {
		return cfg, nil
	}
	keys, err := LoadInternalKeys()
	if err != nil {
		return Config{}, err
	}
	cfg.ServiceToken = keys.ServiceToken
	cfg.ActorSigningKey = keys.ActorSigningKey
	if _, err := cfg.Client(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Client() (Client, error) {
	if !c.Available {
		return nil, nil
	}
	timeout := 10 * time.Second
	if c.Timeout != "" {
		parsed, err := time.ParseDuration(c.Timeout)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("%w: invalid AI_AGENT_TIMEOUT", ErrUnavailable)
		}
		timeout = parsed
	}
	client, err := NewHTTPClientWithTimeout(c.BaseURL, c.ServiceToken, c.ActorSigningKey, timeout)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "", "0", "false", "no", "off":
		return false, nil
	default:
		return false, errors.New("AI_ASSISTANT_AVAILABLE must be a boolean")
	}
}
