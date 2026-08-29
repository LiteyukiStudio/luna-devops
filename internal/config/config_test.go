package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSharedFromEnvFile(t *testing.T) {
	resetEnvLoader(t)
	for _, key := range []string{"DATABASE_URL", "REDIS_ADDR", "PUBLIC_BASE_URL", "VOLUME_TRANSFER_MAX_BYTES"} {
		unsetEnv(t, key)
	}
	envFile := filepath.Join(t.TempDir(), ".env.local")
	content := []byte(strings.Join([]string{
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

func TestSharedVolumeTransferEnabledRequiresJobImage(t *testing.T) {
	if (Shared{VolumeTransferJobImage: ""}).VolumeTransferEnabled() {
		t.Fatal("volume transfer must be disabled without a job image")
	}
	if !(Shared{VolumeTransferJobImage: "worker:latest"}).VolumeTransferEnabled() {
		t.Fatal("volume transfer must be enabled with a job image")
	}
}

func TestStrictEnvironmentParsers(t *testing.T) {
	t.Setenv("TEST_INT", "invalid")
	if _, err := Int("TEST_INT", 1); err == nil {
		t.Fatal("Int accepted invalid input")
	}
	t.Setenv("TEST_BOOL", "sometimes")
	if _, err := Bool("TEST_BOOL", false); err == nil {
		t.Fatal("Bool accepted invalid input")
	}
	t.Setenv("TEST_DURATION", "0")
	if _, err := Duration("TEST_DURATION", time.Second); err == nil {
		t.Fatal("Duration accepted invalid input")
	}
	t.Setenv("TEST_PORTS", "443,bad")
	if _, err := PortList("TEST_PORTS", []int{443}); err == nil {
		t.Fatal("PortList accepted invalid input")
	}
}

func TestRuntimeModeDefaultsToProduction(t *testing.T) {
	unsetEnv(t, "APP_ENV")
	if got := RuntimeMode(); got != "production" {
		t.Fatalf("RuntimeMode() = %q", got)
	}
}

func TestLoadEnvFileLogsPathInDevelopment(t *testing.T) {
	resetEnvLoader(t)
	envFile := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(envFile, []byte("APP_ENV=development\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("ENV_FILE", envFile)

	var output bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })
	LoadEnvironment()
	if got := output.String(); !strings.Contains(got, "environment file loaded") || !strings.Contains(got, envFile) {
		t.Fatalf("log output = %q", got)
	}
}

func TestExplicitEnvFileDoesNotLoadDefaultEnv(t *testing.T) {
	resetEnvLoader(t)
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
