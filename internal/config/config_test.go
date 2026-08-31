package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSharedFromEnvFile(t *testing.T) {
	resetEnvLoader(t)
	for _, key := range []string{"APP_ENV", "DATABASE_URL", "REDIS_ADDR", "PUBLIC_BASE_URL", "VOLUME_TRANSFER_MAX_BYTES"} {
		unsetEnv(t, key)
	}
	envFile := filepath.Join(t.TempDir(), ".env.local")
	content := []byte(strings.Join([]string{
		"APP_ENV=development",
		"DATABASE_URL=postgres://user:pass@db:5432/app?sslmode=disable",
		"REDIS_ADDR=redis://redis:6379/0",
		"PUBLIC_BASE_URL=https://devops.example.com/",
		"VOLUME_TRANSFER_MAX_BYTES=12Gi",
	}, "\n") + "\n")
	if err := os.WriteFile(envFile, content, 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Setenv("ENV_FILE", envFile)

	cfg, err := LoadShared()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://user:pass@db:5432/app?sslmode=disable" || cfg.RedisAddr != "redis://redis:6379/0" {
		t.Fatalf("shared infrastructure = %#v", cfg)
	}
	if cfg.PublicBaseURL != "https://devops.example.com" || cfg.VolumeTransferMaxBytes != 12*1024*1024*1024 {
		t.Fatalf("shared platform config = %#v", cfg)
	}
}

func TestLoadSharedEnvironmentOverridesEnvFile(t *testing.T) {
	resetEnvLoader(t)
	envFile := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(envFile, []byte("PUBLIC_BASE_URL=https://file.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV_FILE", envFile)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "https://environment.example.com")

	cfg, err := LoadShared()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicBaseURL != "https://environment.example.com" {
		t.Fatalf("PublicBaseURL = %q", cfg.PublicBaseURL)
	}
}

func TestLoadSharedRedisAuthentication(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("REDIS_ADDR", "redis://luna:secret@redis.example.com:6379/4")
	cfg, err := LoadShared()
	if err != nil {
		t.Fatal(err)
	}
	options := cfg.RedisOptions()
	if options.Addr != "redis.example.com:6379" || options.Username != "luna" || options.Password != "secret" || options.DB != 4 {
		t.Fatalf("RedisOptions() = %#v", options)
	}
}

func TestRedisConfigurationErrorsDoNotExposeCredentials(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("APP_ENV", "development")
	secretValue := "must-not-leak-from-redis-url"
	t.Setenv("REDIS_ADDR", "redis://user:"+secretValue+"%zz@redis.example.com:6379/0")

	loaders := map[string]func() error{
		"shared": func() error {
			_, err := LoadShared()
			return err
		},
		"tasks": func() error {
			_, err := LoadTasks()
			return err
		},
	}
	for name, load := range loaders {
		t.Run(name, func(t *testing.T) {
			err := load()
			if err == nil || !strings.Contains(err.Error(), "REDIS_ADDR") {
				t.Fatalf("error = %v", err)
			}
			if strings.Contains(err.Error(), secretValue) {
				t.Fatalf("configuration error exposed Redis credentials: %q", err)
			}
		})
	}
}

func TestLoadSharedRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "public URL", key: "PUBLIC_BASE_URL", value: "studio.example.com", want: "PUBLIC_BASE_URL"},
		{name: "runtime mode", key: "APP_ENV", value: "staging-ish", want: "APP_ENV"},
		{name: "log format", key: "LOG_FORMAT", value: "pretty", want: "LOG_FORMAT"},
		{name: "log color", key: "LOG_COLOR", value: "sometimes", want: "LOG_COLOR"},
		{name: "log level", key: "LOG_LEVEL", value: "trace", want: "LOG_LEVEL"},
		{name: "Redis URL", key: "REDIS_ADDR", value: "redis.example.com:6379", want: "REDIS_ADDR"},
		{name: "volume quantity", key: "VOLUME_TRANSFER_MAX_BYTES", value: "large", want: "VOLUME_TRANSFER_MAX_BYTES"},
		{name: "volume minimum", key: "VOLUME_TRANSFER_MAX_BYTES", value: "512Mi", want: "between 1Gi and 5Ti"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetEnvLoader(t)
			t.Setenv(tt.key, tt.value)
			_, err := LoadShared()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateProductionPublicBaseURLRequiresTLSOutsideLoopback(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		value   string
		wantErr bool
	}{
		{name: "public HTTP", mode: "production", value: "http://devops.example.com", wantErr: true},
		{name: "public HTTPS", mode: "production", value: "https://devops.example.com"},
		{name: "localhost HTTP", mode: "production", value: "http://localhost:8088"},
		{name: "IPv4 loopback HTTP", mode: "production", value: "http://127.0.0.1:8088"},
		{name: "IPv6 loopback HTTP", mode: "production", value: "http://[::1]:8088"},
		{name: "development HTTP", mode: "development", value: "http://devops.local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProductionPublicBaseURL(tt.mode, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestSharedVolumeTransferEnabledRequiresJobImage(t *testing.T) {
	if (Shared{VolumeTransferJobImage: ""}).VolumeTransferEnabled() {
		t.Fatal("volume transfer must be disabled without a job image")
	}
	if !(Shared{VolumeTransferJobImage: "worker:latest"}).VolumeTransferEnabled() {
		t.Fatal("volume transfer must be enabled with a job image")
	}
}

func TestEnvironmentDecoderPreservesPrimitiveSemantics(t *testing.T) {
	type primitiveEnvironment struct {
		Integer  int           `env:"TEST_INT" envDefault:"1"`
		Boolean  bool          `env:"TEST_BOOL" envDefault:"false"`
		Duration time.Duration `env:"TEST_DURATION" envDefault:"1s"`
	}

	decoded, err := decodeEnvironment[primitiveEnvironment](map[string]string{
		"TEST_INT":      " 8 ",
		"TEST_BOOL":     " yes ",
		"TEST_DURATION": "90",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Integer != 8 || !decoded.Boolean || decoded.Duration != 90*time.Second {
		t.Fatalf("decoded primitives = %#v", decoded)
	}

	defaults, err := decodeEnvironment[primitiveEnvironment](map[string]string{
		"TEST_INT":      " ",
		"TEST_BOOL":     "\t",
		"TEST_DURATION": "\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Integer != 1 || defaults.Boolean || defaults.Duration != time.Second {
		t.Fatalf("whitespace defaults = %#v", defaults)
	}
}

func TestEnvironmentDecoderReturnsSafeKeyedErrors(t *testing.T) {
	type primitiveEnvironment struct {
		Integer  int           `env:"TEST_INT" envDefault:"1"`
		Boolean  bool          `env:"TEST_BOOL" envDefault:"false"`
		Duration time.Duration `env:"TEST_DURATION" envDefault:"1s"`
	}

	secretValue := "must-not-echo-in-startup-errors"
	_, err := decodeEnvironment[primitiveEnvironment](map[string]string{
		"TEST_INT":      secretValue,
		"TEST_BOOL":     secretValue,
		"TEST_DURATION": secretValue,
	})
	if err == nil {
		t.Fatal("decoder accepted invalid primitives")
	}
	for _, key := range []string{"TEST_INT", "TEST_BOOL", "TEST_DURATION"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error = %q, want key %s", err, key)
		}
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Fatalf("decoder error exposed raw value: %q", err)
	}
	if _, err := parsePortList("TEST_PORTS", "443,bad", []int{443}); err == nil {
		t.Fatal("port parser accepted invalid input")
	}
}

func TestEnvironmentDecoderRejectsDurationOverflow(t *testing.T) {
	type durationEnvironment struct {
		Timeout time.Duration `env:"TEST_TIMEOUT" envDefault:"1s"`
	}
	for _, raw := range []string{"0.5", "10000000000"} {
		_, err := decodeEnvironment[durationEnvironment](map[string]string{
			"TEST_TIMEOUT": raw,
		})
		if err == nil || !strings.Contains(err.Error(), "TEST_TIMEOUT") {
			t.Fatalf("decode %q error = %v", raw, err)
		}
	}

	for raw, want := range map[string]time.Duration{
		"1.5s":       1500 * time.Millisecond,
		"9223372036": time.Duration(9223372036) * time.Second,
	} {
		decoded, err := decodeEnvironment[durationEnvironment](map[string]string{
			"TEST_TIMEOUT": raw,
		})
		if err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
		if decoded.Timeout != want {
			t.Fatalf("decode %q = %s, want %s", raw, decoded.Timeout, want)
		}
	}
}

func TestEnvironmentDecoderUsesOnlyProvidedSnapshot(t *testing.T) {
	type isolatedEnvironment struct {
		Enabled bool `env:"ISOLATED_ENABLED" envDefault:"false"`
	}
	t.Setenv("ISOLATED_ENABLED", "true")
	decoded, err := decodeEnvironment[isolatedEnvironment](map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Enabled {
		t.Fatal("decoder read process environment outside the provided snapshot")
	}
}

func TestEnvironmentDecoderRejectsAmbiguousFieldNames(t *testing.T) {
	type firstEnvironment struct {
		Enabled bool `env:"FIRST_ENABLED"`
	}
	type secondEnvironment struct {
		Enabled bool `env:"SECOND_ENABLED"`
	}
	type ambiguousEnvironment struct {
		First  firstEnvironment
		Second secondEnvironment
	}

	_, err := decodeEnvironment[ambiguousEnvironment](map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "maps to multiple keys") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadTelemetryUsesNoColorPresenceSemantics(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("NO_COLOR", "")
	cfg, err := LoadTelemetry()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.NoColor {
		t.Fatal("NO_COLOR must be enabled whenever the variable exists")
	}
}

func TestLoadSharedCapturesFreshEnvironmentForEachCall(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "https://first.example.com")
	first, err := LoadShared()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PUBLIC_BASE_URL", "https://second.example.com")
	second, err := LoadShared()
	if err != nil {
		t.Fatal(err)
	}
	if first.PublicBaseURL != "https://first.example.com" || second.PublicBaseURL != "https://second.example.com" {
		t.Fatalf("snapshots = %q / %q", first.PublicBaseURL, second.PublicBaseURL)
	}
}

func TestLoadTasksIgnoresUnownedEnvironment(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("APP_ENV", "not-a-runtime-mode")
	t.Setenv("DATABASE_URL", "not-a-database-url")
	t.Setenv("API_DB_MAX_OPEN_CONNS", "not-an-integer")
	t.Setenv("WORKER_DB_MAX_OPEN_CONNS", "not-an-integer")
	if _, err := LoadTasks(); err != nil {
		t.Fatalf("LoadTasks() validated unowned environment: %v", err)
	}
}

func TestRuntimeModeDefaultsToProduction(t *testing.T) {
	unsetEnv(t, "APP_ENV")
	if got, err := runtimeModeFromValue(os.Getenv("APP_ENV")); err != nil || got != "production" {
		t.Fatalf("runtimeModeFromValue() = %q, %v", got, err)
	}
}

func TestExplicitEnvFileFailureIsReturned(t *testing.T) {
	resetEnvLoader(t)
	envFile := filepath.Join(t.TempDir(), "missing.env")
	t.Setenv("ENV_FILE", envFile)
	if err := LoadEnvironment(); err == nil || !strings.Contains(err.Error(), envFile) {
		t.Fatalf("LoadEnvironment() error = %v, want explicit path", err)
	}
}

func TestExplicitEnvFileFailureDoesNotExposeFileContents(t *testing.T) {
	resetEnvLoader(t)
	envFile := filepath.Join(t.TempDir(), "malformed.env")
	content := "INVALID LINE WITHOUT EQUALS\nSECRET_ENCRYPTION_KEY=must-not-leak\nAI_INTERNAL_SECRET=also-must-not-leak\n"
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV_FILE", envFile)

	err := LoadEnvironment()
	if err == nil {
		t.Fatal("LoadEnvironment() accepted a malformed explicit file")
	}
	for _, secret := range []string{"must-not-leak", "also-must-not-leak", "SECRET_ENCRYPTION_KEY", "AI_INTERNAL_SECRET"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("LoadEnvironment() error exposed file contents: %q", err)
		}
	}
}

func TestExplicitEnvFileDoesNotLoadDefaultEnv(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "APP_ENV")
	unsetEnv(t, "PUBLIC_BASE_URL")
	workDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	if err := os.WriteFile(filepath.Join(workDir, ".env"), []byte("PUBLIC_BASE_URL=https://default.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(workDir, ".env.local")
	if err := os.WriteFile(envFile, []byte("APP_ENV=development\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV_FILE", envFile)
	cfg, err := LoadShared()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicBaseURL != "" {
		t.Fatalf("PublicBaseURL = %q", cfg.PublicBaseURL)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	oldValue, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, oldValue)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func resetEnvLoader(t *testing.T) {
	t.Helper()
	resetEnvLoaderForTest()
	t.Cleanup(resetEnvLoaderForTest)
}
