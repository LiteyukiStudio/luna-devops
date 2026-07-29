package aiagent

import (
	"os"
	"strings"
)

type Config struct {
	Available              bool
	BaseURL                string
	ServiceToken           string
	ActorSigningKey        string
	APIServicePrivateKey   string
	ActorContextPrivateKey string
}

func LoadConfig() Config {
	baseURL := strings.TrimSpace(os.Getenv("AI_AGENT_BASE_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AI_AGENT_ADDR"))
	}
	return Config{
		Available:              parseBool(os.Getenv("AI_ASSISTANT_AVAILABLE")),
		BaseURL:                baseURL,
		ServiceToken:           strings.TrimSpace(os.Getenv("AI_AGENT_SERVICE_TOKEN")),
		ActorSigningKey:        strings.TrimSpace(os.Getenv("AI_ACTOR_CONTEXT_SIGNING_KEY")),
		APIServicePrivateKey:   os.Getenv("AI_API_SERVICE_PRIVATE_KEY"),
		ActorContextPrivateKey: os.Getenv("AI_ACTOR_CONTEXT_PRIVATE_KEY"),
	}
}

func (c Config) Client() Client {
	if !c.Available {
		return nil
	}
	if strings.TrimSpace(c.APIServicePrivateKey) != "" || strings.TrimSpace(c.ActorContextPrivateKey) != "" {
		client, err := NewJWTHTTPClient(c.BaseURL, c.APIServicePrivateKey, c.ActorContextPrivateKey)
		if err != nil {
			return nil
		}
		return client
	}
	client, err := NewHTTPClient(c.BaseURL, c.ServiceToken, c.ActorSigningKey)
	if err != nil {
		return nil
	}
	return client
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
