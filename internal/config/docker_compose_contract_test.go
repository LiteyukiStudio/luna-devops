package config

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

type composeServiceContract struct {
	Environment map[string]string `yaml:"environment"`
}

func TestDockerComposeApplicationEnvironmentAllowlist(t *testing.T) {
	services := loadDockerComposeServices(t)
	applicationServices := []string{"api", "worker", "agent"}

	expected := map[string]map[string]string{
		"APP_ENV":                                 {"api": "production", "worker": "production"},
		"NODE_ENV":                                {"agent": "production"},
		"API_ADDR":                                {"api": ":8080"},
		"HOST":                                    {"agent": "0.0.0.0"},
		"PORT":                                    {"agent": "8091"},
		"LOG_FORMAT":                              allApplicationServices("${LOG_FORMAT:-auto}"),
		"LOG_COLOR":                               allApplicationServices("${LOG_COLOR:-auto}"),
		"LOG_LEVEL":                               allApplicationServices("${LOG_LEVEL:-info}"),
		"PUBLIC_BASE_URL":                         apiAndWorker("${PUBLIC_BASE_URL:?PUBLIC_BASE_URL is required}"),
		"APP_CORS_ORIGINS":                        apiOnly("${APP_CORS_ORIGINS:-}"),
		"APP_ENABLE_HSTS":                         apiOnly("true"),
		"TRUSTED_PROXY_CIDRS":                     apiOnly("${TRUSTED_PROXY_CIDRS:-}"),
		"DATABASE_URL":                            allApplicationServices("postgres://devops:devops@postgres:5432/devops?sslmode=disable"),
		"API_DB_MAX_OPEN_CONNS":                   apiOnly("${API_DB_MAX_OPEN_CONNS:-20}"),
		"API_DB_MAX_IDLE_CONNS":                   apiOnly("${API_DB_MAX_IDLE_CONNS:-5}"),
		"API_DB_CONN_MAX_LIFETIME":                apiOnly("${API_DB_CONN_MAX_LIFETIME:-30m}"),
		"API_DB_CONN_MAX_IDLE_TIME":               apiOnly("${API_DB_CONN_MAX_IDLE_TIME:-5m}"),
		"WORKER_DB_MAX_OPEN_CONNS":                workerOnly("${WORKER_DB_MAX_OPEN_CONNS:-20}"),
		"WORKER_DB_MAX_IDLE_CONNS":                workerOnly("${WORKER_DB_MAX_IDLE_CONNS:-5}"),
		"WORKER_DB_CONN_MAX_LIFETIME":             workerOnly("${WORKER_DB_CONN_MAX_LIFETIME:-30m}"),
		"WORKER_DB_CONN_MAX_IDLE_TIME":            workerOnly("${WORKER_DB_CONN_MAX_IDLE_TIME:-5m}"),
		"AI_DATABASE_MAX_CONNECTIONS":             agentOnly("${AI_DATABASE_MAX_CONNECTIONS:-10}"),
		"AI_DATABASE_CONNECTION_TIMEOUT_MS":       agentOnly("${AI_DATABASE_CONNECTION_TIMEOUT_MS:-5000}"),
		"AI_DATABASE_STATEMENT_TIMEOUT_MS":        agentOnly("${AI_DATABASE_STATEMENT_TIMEOUT_MS:-15000}"),
		"REDIS_ADDR":                              apiAndWorker("redis://default:${REDIS_PASSWORD:?REDIS_PASSWORD is required}@redis:6379/0"),
		"SECRET_ENCRYPTION_KEY":                   apiAndWorker("${SECRET_ENCRYPTION_KEY:?SECRET_ENCRYPTION_KEY is required}"),
		"INITIAL_ADMIN_EMAIL":                     apiOnly("${INITIAL_ADMIN_EMAIL:-}"),
		"INITIAL_ADMIN_NAME":                      apiOnly("${INITIAL_ADMIN_NAME:-}"),
		"INITIAL_ADMIN_PASSWORD":                  apiOnly("${INITIAL_ADMIN_PASSWORD:-}"),
		"INITIAL_ADMIN_LANGUAGE":                  apiOnly("${INITIAL_ADMIN_LANGUAGE:-zh-CN}"),
		"METRICS_ENABLED":                         apiOnly("${METRICS_ENABLED:-false}"),
		"METRICS_ADDR":                            apiOnly("${METRICS_ADDR:-:9090}"),
		"METRICS_PATH":                            apiOnly("${METRICS_PATH:-/metrics}"),
		"OTEL_EXPORTER_OTLP_ENDPOINT":             allApplicationServices("${OTEL_EXPORTER_OTLP_ENDPOINT:-}"),
		"OTEL_RESOURCE_ATTRIBUTES":                allApplicationServices("${OTEL_RESOURCE_ATTRIBUTES:-}"),
		"OTEL_EXPORTER_OTLP_HEADERS":              allApplicationServices("${OTEL_EXPORTER_OTLP_HEADERS:-}"),
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT":      apiOnly("${OTEL_EXPORTER_OTLP_TRACES_ENDPOINT:-}"),
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS":       apiOnly("${OTEL_EXPORTER_OTLP_TRACES_HEADERS:-}"),
		"AI_ASSISTANT_AVAILABLE":                  apiOnly("${AI_ASSISTANT_AVAILABLE:-false}"),
		"AI_AGENT_BASE_URL":                       apiOnly("http://agent:8091"),
		"AI_AGENT_TIMEOUT":                        apiOnly("${AI_AGENT_TIMEOUT:-10s}"),
		"AI_INTERNAL_SECRET":                      apiAndAgent("${AI_INTERNAL_SECRET:-}"),
		"AUTH_MODE":                               agentOnly("bff-hmac"),
		"LUNA_API_BASE_URL":                       agentOnly("http://api:8080"),
		"OTEL_SERVICE_VERSION":                    agentOnly("${DEVOPS_IMAGE_TAG:-nightly}"),
		"AI_OBSERVABILITY_CAPTURE_CONTENT":        agentOnly("${AI_OBSERVABILITY_CAPTURE_CONTENT:-false}"),
		"AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS": agentOnly("${AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS:-false}"),
		"BUILD_EXECUTOR_IMAGE":                    workerOnly("${BUILD_EXECUTOR_IMAGE:-moby/buildkit:v0.24.0-rootless}"),
		"BUILD_EGRESS_MODE":                       workerOnly("${BUILD_EGRESS_MODE:-restricted}"),
		"BUILD_JOB_TIMEOUT_SECONDS":               workerOnly("${BUILD_JOB_TIMEOUT_SECONDS:-1800}"),
		"BUILD_JOB_TTL_SECONDS":                   workerOnly("${BUILD_JOB_TTL_SECONDS:-3600}"),
		"BUILD_CACHE_ENABLED":                     workerOnly("${BUILD_CACHE_ENABLED:-false}"),
		"BUILD_CACHE_TAG":                         workerOnly("${BUILD_CACHE_TAG:-buildcache}"),
		"BUILD_PRIVATE_EGRESS_CIDRS":              workerOnly("${BUILD_PRIVATE_EGRESS_CIDRS:-}"),
		"BUILD_PRIVATE_EGRESS_PORTS":              workerOnly("${BUILD_PRIVATE_EGRESS_PORTS:-443}"),
		"BUILD_BLOCKED_EGRESS_CIDRS":              workerOnly("${BUILD_BLOCKED_EGRESS_CIDRS:-}"),
		"DEPLOY_ROLLOUT_TIMEOUT_SECONDS":          workerOnly("${DEPLOY_ROLLOUT_TIMEOUT_SECONDS:-600}"),
		"CERT_MANAGER_CLUSTER_ISSUER":             workerOnly("${CERT_MANAGER_CLUSTER_ISSUER:-letsencrypt-http01}"),
		"VOLUME_TRANSFER_MAX_BYTES":               apiAndWorker("${VOLUME_TRANSFER_MAX_BYTES:-100Gi}"),
		"VOLUME_TRANSFER_JOB_IMAGE":               apiAndWorker("${VOLUME_TRANSFER_JOB_IMAGE:-}"),
	}

	for name, consumers := range expected {
		for _, serviceName := range applicationServices {
			service, ok := services[serviceName]
			if !ok {
				t.Fatalf("docker compose service %q is missing", serviceName)
			}
			got, exists := service.Environment[name]
			want, shouldExist := consumers[serviceName]
			if exists != shouldExist {
				t.Errorf("%s environment %s presence = %t, want %t", serviceName, name, exists, shouldExist)
				continue
			}
			if shouldExist && got != want {
				t.Errorf("%s environment %s = %q, want %q", serviceName, name, got, want)
			}
		}
	}
	for _, serviceName := range applicationServices {
		for name := range services[serviceName].Environment {
			if _, managed := expected[name]; !managed {
				t.Errorf("%s receives environment %s without an explicit consumer contract", serviceName, name)
			}
		}
	}

	for _, removed := range []string{
		"BOOTSTRAP_TOKEN",
		"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME", "DB_CONN_MAX_IDLE_TIME",
		"AI_AGENT_ADDR",
	} {
		for _, serviceName := range applicationServices {
			if _, exists := services[serviceName].Environment[removed]; exists {
				t.Errorf("%s still receives removed or ambiguous environment %s", serviceName, removed)
			}
		}
	}
}

func TestDockerComposeInterpolationsAreDeclaredInRootEnvExample(t *testing.T) {
	composeContent, err := os.ReadFile("../../docker-compose.yaml")
	if err != nil {
		t.Fatalf("read docker compose configuration: %v", err)
	}
	envContent, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read root environment example: %v", err)
	}

	declared := map[string]bool{}
	for _, line := range strings.Split(string(envContent), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, _, ok := strings.Cut(line, "=")
		if ok {
			declared[strings.TrimSpace(name)] = true
		}
	}

	interpolation := regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)[:}]`)
	for _, match := range interpolation.FindAllStringSubmatch(string(composeContent), -1) {
		if !declared[match[1]] {
			t.Errorf("docker-compose.yaml interpolates %s but .env.example does not declare it", match[1])
		}
	}
}

func loadDockerComposeServices(t *testing.T) map[string]composeServiceContract {
	t.Helper()
	content, err := os.ReadFile("../../docker-compose.yaml")
	if err != nil {
		t.Fatalf("read docker compose configuration: %v", err)
	}
	var document struct {
		Services map[string]composeServiceContract `yaml:"services"`
	}
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatalf("parse docker compose configuration: %v", err)
	}
	return document.Services
}

func allApplicationServices(value string) map[string]string {
	return map[string]string{"api": value, "worker": value, "agent": value}
}

func apiAndWorker(value string) map[string]string {
	return map[string]string{"api": value, "worker": value}
}

func apiAndAgent(value string) map[string]string {
	return map[string]string{"api": value, "agent": value}
}

func apiOnly(value string) map[string]string    { return map[string]string{"api": value} }
func workerOnly(value string) map[string]string { return map[string]string{"worker": value} }
func agentOnly(value string) map[string]string  { return map[string]string{"agent": value} }
