package api

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigOwnsAPIEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SECRET_ENCRYPTION_KEY", "api-config-test-secret-key")
	t.Setenv("PUBLIC_BASE_URL", "https://devops.example.com/app/")
	t.Setenv("APP_CORS_ORIGINS", "https://admin.example.com")
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.1.7/8,fd00::1234/8,10.0.0.0/8")
	t.Setenv("API_DB_MAX_OPEN_CONNS", "8")
	t.Setenv("API_DB_MAX_IDLE_CONNS", "3")
	t.Setenv("API_DB_CONN_MAX_LIFETIME", "12m")
	t.Setenv("API_DB_CONN_MAX_IDLE_TIME", "90")
	t.Setenv("INITIAL_ADMIN_EMAIL", " Admin@Example.com ")
	t.Setenv("INITIAL_ADMIN_NAME", " Platform Admin ")
	t.Setenv("INITIAL_ADMIN_PASSWORD", " password with spaces ")
	t.Setenv("INITIAL_ADMIN_LANGUAGE", " en-US ")
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_PATH", "metrics")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicBaseURL != "https://devops.example.com/app" || cfg.DatabaseMaxOpenConns != 8 || cfg.DatabaseMaxIdleConns != 3 {
		t.Fatalf("API config = %#v", cfg)
	}
	if cfg.DatabaseConnMaxLifetime != 12*time.Minute || cfg.DatabaseConnMaxIdleTime != 90*time.Second {
		t.Fatalf("API database durations = %s / %s", cfg.DatabaseConnMaxLifetime, cfg.DatabaseConnMaxIdleTime)
	}
	if cfg.InitialAdmin.Email != "Admin@Example.com" || cfg.InitialAdmin.Name != "Platform Admin" || cfg.InitialAdmin.Password != " password with spaces " || cfg.InitialAdmin.Language != "en-US" {
		t.Fatalf("InitialAdmin = %#v", cfg.InitialAdmin)
	}
	if !cfg.MetricsEnabled || cfg.MetricsPath != "/metrics" || !cfg.EnableHSTS {
		t.Fatalf("API runtime config = %#v", cfg)
	}
	if !cfg.SecretCodec.Available() {
		t.Fatal("API config did not construct the Secret codec")
	}
	if len(cfg.TrustedProxyCIDRs) != 2 || cfg.TrustedProxyCIDRs[0] != "10.0.0.0/8" || cfg.TrustedProxyCIDRs[1] != "fd00::/8" {
		t.Fatalf("TrustedProxyCIDRs = %#v", cfg.TrustedProxyCIDRs)
	}
	if !containsString(cfg.AllowedOrigins, "https://devops.example.com") || !containsString(cfg.AllowedOrigins, "https://admin.example.com") {
		t.Fatalf("AllowedOrigins = %#v", cfg.AllowedOrigins)
	}
}

func TestLoadConfigNormalizesListenAddress(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("API_ADDR", " :8080 ")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
}

func TestLoadConfigHSTSDefaultsAndOverrides(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		value     string
		want      bool
		wantError bool
	}{
		{name: "production blank uses default", mode: "production", value: " ", want: true},
		{name: "development blank uses default", mode: "development", value: " ", want: false},
		{name: "production can disable", mode: "production", value: "false", want: false},
		{name: "development can enable", mode: "development", value: "true", want: true},
		{name: "invalid fails closed", mode: "production", value: "sometimes", want: false, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tt.mode)
			t.Setenv("APP_ENABLE_HSTS", tt.value)
			if tt.mode == "production" {
				t.Setenv("SECRET_ENCRYPTION_KEY", "hsts-test-encryption-key")
				t.Setenv("PUBLIC_BASE_URL", "https://devops.example.com")
			}
			cfg, err := LoadConfig()
			if tt.wantError && err == nil {
				t.Fatal("LoadConfig() accepted invalid HSTS value")
			}
			if !tt.wantError && err != nil {
				t.Fatal(err)
			}
			if cfg.EnableHSTS != tt.want {
				t.Fatalf("EnableHSTS = %t, want %t", cfg.EnableHSTS, tt.want)
			}
		})
	}
}

func TestLoadConfigBuildsBrowserTraceRelay(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318/otel")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "api-key=secret%20value")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BrowserTraceEndpoint != "http://collector:4318/otel/v1/traces" {
		t.Fatalf("BrowserTraceEndpoint = %q", cfg.BrowserTraceEndpoint)
	}
	if cfg.BrowserTraceHeaders["api-key"] != "secret value" || len(cfg.BrowserTraceHeaders) != 1 {
		t.Fatalf("BrowserTraceHeaders = %#v", cfg.BrowserTraceHeaders)
	}
	cfg.BrowserTraceHeaders["api-key"] = "changed"
	if cfg.Telemetry.Headers["api-key"] != "secret value" {
		t.Fatal("browser trace headers share mutable state with telemetry headers")
	}
}

func TestLoadConfigKeepsExplicitBrowserTraceEndpoint(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318/otel")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://traces:4318/custom/v1/traces")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BrowserTraceEndpoint != "http://traces:4318/custom/v1/traces" {
		t.Fatalf("BrowserTraceEndpoint = %q", cfg.BrowserTraceEndpoint)
	}
}

func TestLoadConfigRejectsInvalidAPIEnvironment(t *testing.T) {
	tests := []struct {
		key   string
		value string
		want  string
	}{
		{key: "API_DB_MAX_OPEN_CONNS", value: "0", want: "must be positive"},
		{key: "API_ADDR", value: "8080", want: "IP:port"},
		{key: "METRICS_ADDR", value: ":70000", want: "between 1 and 65535"},
		{key: "API_DB_MAX_IDLE_CONNS", value: "21", want: "must be between"},
		{key: "METRICS_ENABLED", value: "sometimes", want: "METRICS_ENABLED"},
		{key: "LOCAL_ADMIN_FREE_QUOTA_CREDITS", value: "-1", want: "free quota"},
		{key: "AI_ASSISTANT_AVAILABLE", value: "sometimes", want: "AI_ASSISTANT_AVAILABLE"},
		{key: "AI_AGENT_BASE_URL", value: "grpc://agent:8091", want: "AI_AGENT_BASE_URL"},
		{key: "TRUSTED_PROXY_CIDRS", value: "not-a-cidr", want: "TRUSTED_PROXY_CIDRS"},
		{key: "APP_CORS_ORIGINS", value: "https://admin.example.com/path", want: "without paths"},
		{key: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", value: "://bad", want: "OTLP traces endpoint"},
		{key: "OTEL_EXPORTER_OTLP_TRACES_HEADERS", value: "bad", want: "key=value"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			_, err := LoadConfig()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadConfigIgnoresWorkerEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("WORKER_DB_MAX_OPEN_CONNS", "not-an-integer")
	t.Setenv("BUILD_CACHE_ENABLED", "not-a-boolean")
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() validated Worker environment: %v", err)
	}
}

func TestLoadConfigKeepsTelemetryWhenAPIEnvironmentIsInvalid(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("METRICS_ENABLED", "not-a-boolean")
	cfg, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() accepted invalid API environment")
	}
	if cfg.Telemetry.LogLevel != "debug" {
		t.Fatalf("Telemetry.LogLevel = %q", cfg.Telemetry.LogLevel)
	}
}

func TestLoadConfigDoesNotExposeInvalidPrimitiveValue(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	secretValue := "must-not-appear-in-config-error"
	t.Setenv("AI_AGENT_TIMEOUT", secretValue)
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "AI_AGENT_TIMEOUT") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Fatalf("configuration error exposed raw value: %q", err)
	}
}

func TestLoadConfigReportsPrefixedEnvironmentKeyWithoutRawValue(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	rawValue := "must-not-appear-in-prefixed-error"
	t.Setenv("API_DB_MAX_OPEN_CONNS", rawValue)
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "API_DB_MAX_OPEN_CONNS") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), rawValue) {
		t.Fatalf("configuration error exposed raw value: %q", err)
	}
}
