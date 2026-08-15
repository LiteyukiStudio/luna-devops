package aiagent

import (
	"os"
	"strings"
)

type Config struct {
	Available       bool
	BaseURL         string
	ServiceToken    string
	ActorSigningKey string
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
	}
}

func (c Config) Client() (Client, error) {
	if !c.Available {
		return nil, nil
	}
	client, err := NewHTTPClient(c.BaseURL, c.ServiceToken, c.ActorSigningKey)
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
