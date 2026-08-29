package api

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigOwnsAPIEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
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
	if len(cfg.TrustedProxyCIDRs) != 2 || cfg.TrustedProxyCIDRs[0] != "10.0.0.0/8" || cfg.TrustedProxyCIDRs[1] != "fd00::/8" {
		t.Fatalf("TrustedProxyCIDRs = %#v", cfg.TrustedProxyCIDRs)
	}
	if !containsString(cfg.AllowedOrigins, "https://devops.example.com") || !containsString(cfg.AllowedOrigins, "https://admin.example.com") {
		t.Fatalf("AllowedOrigins = %#v", cfg.AllowedOrigins)
	}
}

func TestLoadConfigBuildsBrowserTraceRelay(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318/otel")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "api-key=secret%20value,bad,nope=x%0D%0Ay")
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
}

func TestLoadConfigRejectsInvalidAPIEnvironment(t *testing.T) {
	tests := []struct {
		key   string
		value string
		want  string
	}{
		{key: "API_DB_MAX_OPEN_CONNS", value: "0", want: "must be positive"},
		{key: "API_DB_MAX_IDLE_CONNS", value: "21", want: "must be between"},
		{key: "METRICS_ENABLED", value: "sometimes", want: "METRICS_ENABLED"},
		{key: "LOCAL_ADMIN_FREE_QUOTA_CREDITS", value: "-1", want: "free quota"},
		{key: "AI_ASSISTANT_AVAILABLE", value: "sometimes", want: "AI_ASSISTANT_AVAILABLE"},
		{key: "TRUSTED_PROXY_CIDRS", value: "not-a-cidr", want: "TRUSTED_PROXY_CIDRS"},
		{key: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", value: "://bad", want: "OTLP traces endpoint"},
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
