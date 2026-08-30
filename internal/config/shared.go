package config

import (
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/secret"
)

const developmentEncryptionKey = "luna-devops-local-secret"

// LoadTelemetry loads the common process logging and OpenTelemetry contract
// for helper binaries that own a separate domain-specific configuration.
func LoadTelemetry() (TelemetryConfig, error) {
	snapshot, environmentErr := loadEnvironmentSnapshot()
	if environmentErr != nil {
		return TelemetryConfig{}, environmentErr
	}
	raw, decodeErr := decodeEnvironment[telemetryEnvironment](snapshot)
	telemetry, validationErr := buildTelemetry(raw, snapshot)
	return telemetry, errors.Join(decodeErr, validationErr)
}

// LoadShared reads and validates the common API/Worker deployment contract.
func LoadShared() (Shared, error) {
	snapshot, environmentErr := loadEnvironmentSnapshot()
	return loadSharedFrom(snapshot, environmentErr)
}

func loadSharedFrom(snapshot map[string]string, environmentErr error) (Shared, error) {
	if environmentErr != nil {
		return Shared{}, environmentErr
	}
	raw, decodeErr := decodeEnvironment[sharedEnvironment](snapshot)
	shared, validationErr := buildShared(raw, snapshot)
	return shared, errors.Join(decodeErr, validationErr)
}

// LoadTasks loads the small contract required by the task administration CLI.
func LoadTasks() (TasksConfig, error) {
	snapshot, environmentErr := loadEnvironmentSnapshot()
	if environmentErr != nil {
		return TasksConfig{}, environmentErr
	}
	raw, decodeErr := decodeEnvironment[tasksEnvironment](snapshot)
	telemetryRaw, telemetryDecodeErr := decodeEnvironment[telemetryEnvironment](snapshot)
	telemetry, telemetryErr := buildTelemetry(telemetryRaw, snapshot)

	address := strings.TrimSpace(raw.RedisAddr)
	redisOptions, redisErr := redisconfig.Parse(address)
	if redisErr != nil {
		redisErr = errors.New("REDIS_ADDR is invalid")
	}
	return TasksConfig{Redis: redisOptions, Telemetry: telemetry}, errors.Join(
		decodeErr,
		telemetryDecodeErr,
		telemetryErr,
		redisErr,
	)
}

func buildShared(raw sharedEnvironment, snapshot map[string]string) (Shared, error) {
	mode, modeErr := runtimeModeFromValue(raw.Mode)
	telemetry, telemetryErr := buildTelemetry(raw.Telemetry, snapshot)
	secretKeyMaterial := strings.TrimSpace(raw.SecretEncryptionKey)
	if mode == "development" && secretKeyMaterial == "" {
		secretKeyMaterial = developmentEncryptionKey
	}
	shared := Shared{
		Mode:                   mode,
		PublicBaseURL:          strings.TrimRight(strings.TrimSpace(raw.PublicBaseURL), "/"),
		DatabaseURL:            strings.TrimSpace(raw.DatabaseURL),
		RedisAddr:              strings.TrimSpace(raw.RedisAddr),
		VolumeTransferJobImage: strings.TrimSpace(raw.VolumeTransferJobImage),
		Telemetry:              telemetry,
	}

	var errs []error
	errs = appendError(errs, modeErr)
	errs = appendError(errs, telemetryErr)
	if err := validatePublicBaseURL(shared.PublicBaseURL); err != nil {
		errs = append(errs, err)
	}
	if err := validateDatabaseURL(shared.DatabaseURL); err != nil {
		errs = append(errs, err)
	}
	if _, err := redisconfig.Parse(shared.RedisAddr); err != nil {
		errs = append(errs, errors.New("REDIS_ADDR is invalid"))
	}
	if shared.Mode == "production" && secretKeyMaterial == "" {
		errs = append(errs, errors.New("SECRET_ENCRYPTION_KEY is required in production"))
	} else {
		secretCodec, codecErr := secret.NewCodec(secretKeyMaterial)
		shared.SecretCodec = secretCodec
		errs = appendError(errs, codecErr)
	}
	maxBytes, err := parseByteQuantity(
		"VOLUME_TRANSFER_MAX_BYTES",
		raw.VolumeTransferMaxBytes,
		100*1024*1024*1024,
	)
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

func buildTelemetry(raw telemetryEnvironment, snapshot map[string]string) (TelemetryConfig, error) {
	cfg := TelemetryConfig{
		Endpoint:  strings.TrimRight(strings.TrimSpace(raw.Endpoint), "/"),
		LogFormat: strings.ToLower(strings.TrimSpace(raw.LogFormat)),
		LogColor:  strings.ToLower(strings.TrimSpace(raw.LogColor)),
		LogLevel:  strings.ToLower(strings.TrimSpace(raw.LogLevel)),
	}
	_, cfg.NoColor = snapshot["NO_COLOR"]
	var errs []error
	if err := validateEnum("LOG_FORMAT", cfg.LogFormat, "auto", "console", "json"); err != nil {
		errs = append(errs, err)
	}
	if err := validateEnum("LOG_COLOR", cfg.LogColor, "auto", "always", "never"); err != nil {
		errs = append(errs, err)
	}
	if err := validateEnum("LOG_LEVEL", cfg.LogLevel, "debug", "info", "warn", "error"); err != nil {
		errs = append(errs, err)
	}
	if cfg.Endpoint != "" {
		if err := validateHTTPURL("OTEL_EXPORTER_OTLP_ENDPOINT", cfg.Endpoint); err != nil {
			errs = append(errs, err)
		}
	}
	var err error
	cfg.Headers, err = parseKeyValueList("OTEL_EXPORTER_OTLP_HEADERS", raw.Headers)
	errs = appendError(errs, err)
	cfg.ResourceAttributes, err = parseKeyValueList("OTEL_RESOURCE_ATTRIBUTES", raw.ResourceAttributes)
	errs = appendError(errs, err)
	return cfg, errors.Join(errs...)
}
