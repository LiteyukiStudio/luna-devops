package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/shopspring/decimal"
)

// LoadAPI is the only API startup adapter that reads deployment environment.
func LoadAPI() (APIConfig, error) {
	snapshot, environmentErr := loadEnvironmentSnapshot()
	shared, sharedErr := loadSharedFrom(snapshot, environmentErr)
	raw, decodeErr := decodeEnvironment[apiEnvironment](snapshot)
	cfg, validationErr := buildAPI(raw, shared, decodeErr)
	return cfg, errors.Join(sharedErr, decodeErr, validationErr)
}

func buildAPI(raw apiEnvironment, shared Shared, decodeErr error) (APIConfig, error) {
	hstsEnabled := shared.Mode == "production"
	if raw.EnableHSTS != nil {
		hstsEnabled = *raw.EnableHSTS
	}
	if environmentKeyFailed(decodeErr, "APP_ENABLE_HSTS") {
		hstsEnabled = false
	}

	cfg := APIConfig{
		Shared:                  shared,
		Addr:                    strings.TrimSpace(raw.Addr),
		DatabaseMaxOpenConns:    raw.Database.MaxOpenConns,
		DatabaseMaxIdleConns:    raw.Database.MaxIdleConns,
		DatabaseConnMaxLifetime: raw.Database.ConnMaxLifetime,
		DatabaseConnMaxIdleTime: raw.Database.ConnMaxIdleTime,
		InitialAdmin: InitialAdminConfig{
			Email:            strings.TrimSpace(raw.InitialAdmin.Email),
			Name:             strings.TrimSpace(raw.InitialAdmin.Name),
			Password:         raw.InitialAdmin.Password,
			Language:         strings.TrimSpace(raw.InitialAdmin.Language),
			FreeQuotaCredits: strings.TrimSpace(raw.InitialAdmin.FreeQuotaCredits),
		},
		MetricsEnabled: raw.MetricsEnabled,
		MetricsAddr:    strings.TrimSpace(raw.MetricsAddr),
		MetricsPath:    normalizeMetricsPath(raw.MetricsPath),
		EnableHSTS:     hstsEnabled,
		AppVersion:     strings.TrimSpace(raw.AppVersion),
	}

	var errs []error
	if shared.Mode == "production" && shared.PublicBaseURL == "" {
		errs = append(errs, errors.New("PUBLIC_BASE_URL is required by API in production"))
	}
	if err := validateListenAddress("API_ADDR", cfg.Addr, true); err != nil {
		errs = append(errs, err)
	}
	if cfg.MetricsAddr != "" {
		if err := validateListenAddress("METRICS_ADDR", cfg.MetricsAddr, false); err != nil {
			errs = append(errs, err)
		}
	}
	if cfg.InitialAdmin.Language != "" {
		if err := validateEnum("INITIAL_ADMIN_LANGUAGE", cfg.InitialAdmin.Language, "zh-CN", "en-US"); err != nil {
			errs = append(errs, err)
		}
	}
	if quota, err := decimal.NewFromString(cfg.InitialAdmin.FreeQuotaCredits); err != nil || quota.IsNegative() {
		errs = append(errs, errors.New("development administrator free quota must be a non-negative decimal"))
	}
	if cfg.DatabaseMaxOpenConns <= 0 {
		errs = append(errs, errors.New("API_DB_MAX_OPEN_CONNS must be positive"))
	}
	if cfg.DatabaseMaxIdleConns < 0 || cfg.DatabaseMaxIdleConns > cfg.DatabaseMaxOpenConns {
		errs = append(errs, errors.New("API_DB_MAX_IDLE_CONNS must be between 0 and API_DB_MAX_OPEN_CONNS"))
	}

	var err error
	cfg.TrustedProxyCIDRs, err = parseTrustedProxyCIDRList(splitList(raw.TrustedProxyCIDRs))
	errs = appendError(errs, err)
	configuredOrigins, err := parseOrigins("APP_CORS_ORIGINS", splitList(raw.CORSOrigins))
	errs = appendError(errs, err)
	cfg.AllowedOrigins = allowedOrigins(shared.Mode, shared.PublicBaseURL, configuredOrigins)

	traceEndpoint := strings.TrimSpace(raw.TraceEndpoint)
	if traceEndpoint == "" {
		cfg.BrowserTraceEndpoint, err = signalEndpoint(shared.Telemetry.Endpoint, "v1/traces")
	} else {
		err = validateHTTPURL("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", traceEndpoint)
		cfg.BrowserTraceEndpoint = traceEndpoint
	}
	if err != nil {
		errs = append(errs, fmt.Errorf("OTLP traces endpoint is invalid: %w", err))
	}
	traceHeaders := strings.TrimSpace(raw.TraceHeaders)
	if traceHeaders == "" {
		cfg.BrowserTraceHeaders = cloneMap(shared.Telemetry.Headers)
	} else {
		cfg.BrowserTraceHeaders, err = parseKeyValueList("OTEL_EXPORTER_OTLP_TRACES_HEADERS", traceHeaders)
		errs = appendError(errs, err)
	}

	aiBaseURL := strings.TrimSpace(raw.AIAgentBaseURL)
	var baseURLErr error
	if aiBaseURL != "" {
		baseURLErr = validateHTTPURL("AI_AGENT_BASE_URL", aiBaseURL)
		errs = appendError(errs, baseURLErr)
	}
	if !environmentKeyFailed(decodeErr, "AI_ASSISTANT_AVAILABLE") &&
		!environmentKeyFailed(decodeErr, "AI_AGENT_TIMEOUT") &&
		baseURLErr == nil {
		var aiConfigErr error
		cfg.AIAgent, aiConfigErr = aiagent.NewConfig(
			raw.AIAssistantAvailable,
			aiBaseURL,
			raw.AIAgentTimeout,
			raw.AIInternalSecret,
		)
		errs = appendError(errs, aiConfigErr)
	}

	return cfg, errors.Join(errs...)
}
