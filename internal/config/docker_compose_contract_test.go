package config

import (
	"os"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestDockerComposeSharesPublicBaseURLWithAPIAndWorker(t *testing.T) {
	content, err := os.ReadFile("../../docker-compose.yaml")
	if err != nil {
		t.Fatalf("read docker compose configuration: %v", err)
	}

	var document struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatalf("parse docker compose configuration: %v", err)
	}

	for _, serviceName := range []string{"api", "worker"} {
		service, ok := document.Services[serviceName]
		if !ok {
			t.Fatalf("docker compose service %q is missing", serviceName)
		}
		if got := service.Environment["PUBLIC_BASE_URL"]; got != "${PUBLIC_BASE_URL:-}" {
			t.Fatalf("%s PUBLIC_BASE_URL = %q, want shared compose interpolation", serviceName, got)
		}
	}
}
