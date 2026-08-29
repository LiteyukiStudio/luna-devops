package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/joho/godotenv"
	"k8s.io/apimachinery/pkg/api/resource"
)

var envLoadOnce sync.Once

const (
	minimumVolumeTransferBytes = int64(1 * 1024 * 1024 * 1024)
	maximumVolumeTransferBytes = int64(5 * 1024 * 1024 * 1024 * 1024)
)

// Shared contains startup values defined once by the deployment and explicitly
// passed to both API and Worker. Service-specific values belong in the owning
// service's config.go instead.
type Shared struct {
	Mode                   string
	LogFormat              string
	LogColor               string
	LogLevel               string
	PublicBaseURL          string
	DatabaseURL            string
	RedisAddr              string
	VolumeTransferMaxBytes int64
	VolumeTransferJobImage string
}

// LoadShared reads the small, allowlisted configuration contract shared by API
// and Worker. Invalid explicit values fail startup instead of silently falling
// back to defaults.
func LoadShared() (Shared, error) {
	LoadEnvironment()
	mode, modeErr := runtimeModeFromValue(os.Getenv("APP_ENV"))

	shared := Shared{
		Mode:                   mode,
		LogFormat:              strings.ToLower(strings.TrimSpace(String("LOG_FORMAT", "auto"))),
		LogColor:               strings.ToLower(strings.TrimSpace(String("LOG_COLOR", "auto"))),
		LogLevel:               strings.ToLower(strings.TrimSpace(String("LOG_LEVEL", "info"))),
		PublicBaseURL:          strings.TrimRight(strings.TrimSpace(String("PUBLIC_BASE_URL", "")), "/"),
		DatabaseURL:            String("DATABASE_URL", "postgres://devops:devops@localhost:5432/devops?sslmode=disable"),
		RedisAddr:              strings.TrimSpace(String("REDIS_ADDR", "redis://localhost:6379/0")),
		VolumeTransferJobImage: strings.TrimSpace(String("VOLUME_TRANSFER_JOB_IMAGE", "")),
	}

	var errs []error
	if modeErr != nil {
		errs = append(errs, modeErr)
	}
	if err := validateEnum("LOG_FORMAT", shared.LogFormat, "auto", "console", "json"); err != nil {
		errs = append(errs, err)
	}
	if err := validateEnum("LOG_COLOR", shared.LogColor, "auto", "always", "never"); err != nil {
		errs = append(errs, err)
	}
	if err := validateEnum("LOG_LEVEL", shared.LogLevel, "debug", "info", "warn", "error"); err != nil {
		errs = append(errs, err)
	}
	if err := validatePublicBaseURL(shared.PublicBaseURL); err != nil {
		errs = append(errs, err)
	}
	if _, err := redisconfig.Parse(shared.RedisAddr); err != nil {
		errs = append(errs, fmt.Errorf("invalid REDIS_ADDR: %w", err))
	}
	maxBytes, err := ByteQuantity("VOLUME_TRANSFER_MAX_BYTES", 100*1024*1024*1024)
	if err != nil {
		errs = append(errs, err)
	} else {
		shared.VolumeTransferMaxBytes = maxBytes
		if maxBytes < minimumVolumeTransferBytes || maxBytes > maximumVolumeTransferBytes {
			errs = append(errs, errors.New("VOLUME_TRANSFER_MAX_BYTES must be between 1Gi and 5Ti"))
		}
	}
	return shared, errors.Join(errs...)
}

func (c Shared) RedisOptions() redisconfig.Options {
	return redisconfig.MustParse(c.RedisAddr)
}

func (c Shared) VolumeTransferEnabled() bool {
	return c.VolumeTransferJobImage != ""
}

func validatePublicBaseURL(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("PUBLIC_BASE_URL must be an absolute http or https URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("PUBLIC_BASE_URL must not contain credentials, query parameters, or fragments")
	}
	return nil
}

func validateEnum(key string, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of %s", key, strings.Join(allowed, ", "))
}

// LoadEnvironment loads the configured dotenv file before process-wide
// infrastructure such as telemetry is initialized. Loading is idempotent.
func LoadEnvironment() {
	loadEnvFile()
}

func RuntimeMode() string {
	mode, _ := runtimeModeFromValue(os.Getenv("APP_ENV"))
	return mode
}

func runtimeModeFromValue(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "production", "prod":
		return "production", nil
	case "development", "dev", "local":
		return "development", nil
	case "":
		return "production", nil
	default:
		return "production", errors.New("APP_ENV must be production or development")
	}
}

func String(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func Int(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return parsed, nil
}

func Duration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed, nil
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second, nil
	}
	return 0, fmt.Errorf("%s must be a positive duration or number of seconds", key)
}

func ByteQuantity(key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil || quantity.Sign() <= 0 {
		return 0, fmt.Errorf("%s must be a positive byte quantity", key)
	}
	return quantity.Value(), nil
}

func Bool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", key)
	}
}

func List(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func PortList(key string, fallback []int) ([]int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return append([]int(nil), fallback...), nil
	}
	parts := strings.Split(raw, ",")
	values := make([]int, 0, len(parts))
	seen := map[int]bool{}
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 1 || value > 65535 {
			return nil, fmt.Errorf("%s must contain ports between 1 and 65535", key)
		}
		if !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s must contain at least one port", key)
	}
	return values, nil
}

func loadEnvFile() {
	envLoadOnce.Do(loadEnvFileOnce)
}

func loadEnvFileOnce() {
	envFile := strings.TrimSpace(os.Getenv("ENV_FILE"))
	if envFile == "" {
		envFile = ".env"
	}
	loadEnvFiles(envFile)
}

func resetEnvLoaderForTest() {
	envLoadOnce = sync.Once{}
}

func loadEnvFiles(paths ...string) {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if err := godotenv.Load(path); err != nil {
			if RuntimeMode() == "development" {
				attrs := []slog.Attr{
					slog.String("event.name", "config.env_file.not_loaded"),
					slog.String("operation", "config.env_file.load"),
					slog.String("file.path", path),
				}
				attrs = append(attrs, telemetry.ErrorAttrs(err, "config.env_file.not_found")...)
				telemetry.Logger().LogAttrs(context.Background(), slog.LevelDebug,
					"Environment file not loaded; using process environment", attrs...)
			}
			continue
		}
		if RuntimeMode() == "development" {
			slog.Debug("environment file loaded", "event.name", "config.env_file.loaded", "file.path", path)
		}
	}
}
