package api

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	sharedconfig "github.com/LiteyukiStudio/devops/internal/config"
)

type Config struct {
	sharedconfig.Shared
	Addr                    string
	DatabaseMaxOpenConns    int
	DatabaseMaxIdleConns    int
	DatabaseConnMaxLifetime time.Duration
	DatabaseConnMaxIdleTime time.Duration
	TrustedProxyCIDRs       []string
	InitialAdmin            InitialAdminConfig
	MetricsEnabled          bool
	MetricsAddr             string
	MetricsPath             string
	AllowedOrigins          []string
	EnableHSTS              bool
	AppVersion              string
	BrowserTraceEndpoint    string
	BrowserTraceHeaders     map[string]string
	AIAgent                 aiagent.Config
}

type InitialAdminConfig struct {
	Email            string
	Name             string
	Password         string
	Language         string
	FreeQuotaCredits string
}

func LoadConfig() (Config, error) {
	shared, sharedErr := sharedconfig.LoadShared()
	cfg := Config{
		Shared: shared,
		Addr:   sharedconfig.String("API_ADDR", ":8080"),
		InitialAdmin: InitialAdminConfig{
			Email:    strings.TrimSpace(sharedconfig.String("INITIAL_ADMIN_EMAIL", "")),
			Name:     strings.TrimSpace(sharedconfig.String("INITIAL_ADMIN_NAME", "")),
			Password: sharedconfig.String("INITIAL_ADMIN_PASSWORD", ""),
			Language: strings.TrimSpace(sharedconfig.String("INITIAL_ADMIN_LANGUAGE", "")),
			FreeQuotaCredits: strings.TrimSpace(sharedconfig.String(
				"LOCAL_ADMIN_FREE_QUOTA_CREDITS",
				"1000",
			)),
		},
		MetricsAddr: sharedconfig.String("METRICS_ADDR", ""),
		MetricsPath: normalizeMetricsPath(sharedconfig.String("METRICS_PATH", "/metrics")),
		AppVersion:  strings.TrimSpace(sharedconfig.String("APP_VERSION", "dev")),
	}

	var errs []error
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.AIAgent, sharedErr = aiagent.LoadConfig()
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.DatabaseMaxOpenConns, sharedErr = sharedconfig.Int("API_DB_MAX_OPEN_CONNS", 20)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.DatabaseMaxIdleConns, sharedErr = sharedconfig.Int("API_DB_MAX_IDLE_CONNS", 5)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.DatabaseConnMaxLifetime, sharedErr = sharedconfig.Duration("API_DB_CONN_MAX_LIFETIME", 30*time.Minute)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.DatabaseConnMaxIdleTime, sharedErr = sharedconfig.Duration("API_DB_CONN_MAX_IDLE_TIME", 5*time.Minute)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	if cfg.DatabaseMaxOpenConns <= 0 {
		errs = append(errs, errors.New("API_DB_MAX_OPEN_CONNS must be positive"))
	}
	if cfg.DatabaseMaxIdleConns < 0 || cfg.DatabaseMaxIdleConns > cfg.DatabaseMaxOpenConns {
		errs = append(errs, errors.New("API_DB_MAX_IDLE_CONNS must be between 0 and API_DB_MAX_OPEN_CONNS"))
	}
	if _, quotaErr := developmentAdminFreeQuotaCredits(cfg.InitialAdmin.FreeQuotaCredits); quotaErr != nil {
		errs = append(errs, quotaErr)
	}

	cfg.MetricsEnabled, sharedErr = sharedconfig.Bool("METRICS_ENABLED", false)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.EnableHSTS, sharedErr = sharedconfig.Bool("APP_ENABLE_HSTS", shared.Mode == "production")
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.TrustedProxyCIDRs, sharedErr = parseTrustedProxyCIDRs(sharedconfig.String("TRUSTED_PROXY_CIDRS", ""))
	if sharedErr != nil {
		errs = append(errs, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS: %w", sharedErr))
	}
	cfg.AllowedOrigins = allowedOriginsFromConfig(shared.Mode, shared.PublicBaseURL, sharedconfig.List("APP_CORS_ORIGINS"))
	cfg.BrowserTraceEndpoint, sharedErr = parseBrowserTraceEndpoint(
		sharedconfig.String("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", ""),
		sharedconfig.String("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
	)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.BrowserTraceHeaders = parseOTLPHeaders(firstNonEmpty(
		sharedconfig.String("OTEL_EXPORTER_OTLP_TRACES_HEADERS", ""),
		sharedconfig.String("OTEL_EXPORTER_OTLP_HEADERS", ""),
	))
	return cfg, errors.Join(errs...)
}

func mustLoadConfig() Config {
	cfg, err := LoadConfig()
	if err != nil {
		panic(err)
	}
	return cfg
}

func configuredOrLoaded(configs []Config) Config {
	if len(configs) > 0 {
		return configs[0]
	}
	return mustLoadConfig()
}

func parseTrustedProxyCIDRs(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	seen := make(map[netip.Prefix]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, err
		}
		prefix = prefix.Masked()
		if _, exists := seen[prefix]; exists {
			continue
		}
		seen[prefix] = struct{}{}
		values = append(values, prefix.String())
	}
	return values, nil
}

func normalizeMetricsPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/metrics"
	}
	if !strings.HasPrefix(value, "/") {
		return "/" + value
	}
	return value
}

func allowedOriginsFromConfig(mode string, publicBaseURL string, configured []string) []string {
	origins := append([]string(nil), configured...)
	if publicBase := originFromURL(publicBaseURL); publicBase != "" {
		origins = append(origins, publicBase)
	}
	if mode == "development" {
		origins = append(origins,
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://localhost:4173",
			"http://127.0.0.1:4173",
			"http://localhost:4174",
			"http://127.0.0.1:4174",
			"http://localhost:4184",
			"http://127.0.0.1:4184",
		)
	}
	return normalizeList(origins, false)
}

func parseBrowserTraceEndpoint(tracesEndpoint string, endpoint string) (string, error) {
	value := strings.TrimSpace(tracesEndpoint)
	appendTracePath := false
	if value == "" {
		value = strings.TrimSpace(endpoint)
		appendTracePath = value != ""
	}
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("OTLP traces endpoint is invalid")
	}
	if appendTracePath {
		parsed.Path = path.Join(parsed.Path, "/v1/traces")
	}
	return parsed.String(), nil
}

func parseOTLPHeaders(value string) map[string]string {
	headers := make(map[string]string)
	for _, entry := range strings.Split(strings.TrimSpace(value), ",") {
		key, rawValue, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		if !ok || key == "" || strings.ContainsAny(key+rawValue, "\r\n") {
			continue
		}
		decoded, err := url.QueryUnescape(rawValue)
		if err != nil || strings.ContainsAny(decoded, "\r\n") {
			continue
		}
		headers[key] = decoded
	}
	return headers
}
