package aiagent

import (
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

func LoadConfig() Config {
	baseURL := strings.TrimSpace(os.Getenv("AI_AGENT_BASE_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AI_AGENT_ADDR"))
	}
	keys, _ := LoadInternalKeys()
	return Config{
		Available:       parseBool(os.Getenv("AI_ASSISTANT_AVAILABLE")),
		BaseURL:         baseURL,
		ServiceToken:    keys.ServiceToken,
		ActorSigningKey: keys.ActorSigningKey,
		Timeout:         strings.TrimSpace(os.Getenv("AI_AGENT_TIMEOUT")),
	}
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

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
