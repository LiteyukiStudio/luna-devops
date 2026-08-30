package main

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

func TestAPIStartupHelper(t *testing.T) {
	if os.Getenv("LUNA_TEST_API_STARTUP_HELPER") != "1" {
		return
	}
	if err := run(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestAPIStartupRedisUnavailableLogsCompleteDiagnostics(t *testing.T) {
	for _, format := range []string{"console", "json"} {
		t.Run(format, func(t *testing.T) {
			stdout, stderr, err := runAPIStartupHelper(t, format, "redis://127.0.0.1:1/0", "postgres://devops:devops@127.0.0.1:1/devops?sslmode=disable&connect_timeout=1")
			if err == nil {
				t.Fatal("API startup unexpectedly succeeded")
			}
			if stdout != "" {
				t.Fatalf("stdout was polluted: %q", stdout)
			}
			for _, expected := range []string{"API startup failed", "dependency.redis.unavailable", "connect Redis: ping Redis: dial tcp 127.0.0.1:1", "start Redis or verify REDIS_ADDR"} {
				if !strings.Contains(stderr, expected) {
					t.Fatalf("%s output omitted %q: %s", format, expected, stderr)
				}
			}
			if format == "json" {
				var record map[string]any
				if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &record); decodeErr != nil {
					t.Fatalf("JSON log is invalid: %v\n%s", decodeErr, stderr)
				}
				if record["error.code"] != "dependency.redis.unavailable" || record["error.message"] == "" {
					t.Fatalf("JSON diagnostic fields are incomplete: %#v", record)
				}
			}
		})
	}
}

func TestAPIStartupPostgresUnavailableKeepsDependencyCause(t *testing.T) {
	redis := miniredis.RunT(t)
	_, stderr, err := runAPIStartupHelper(t, "json", "redis://"+redis.Addr()+"/0", "postgres://devops:devops@127.0.0.1:1/devops?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("API startup unexpectedly succeeded")
	}
	for _, expected := range []string{"dependency.postgres.unavailable", "connect PostgreSQL: connect database:", "127.0.0.1:1", "start PostgreSQL or verify DATABASE_URL"} {
		if !strings.Contains(stderr, expected) {
			t.Fatalf("PostgreSQL diagnostic omitted %q: %s", expected, stderr)
		}
	}
}

func TestAPIPortOccupiedReturnsStableListenFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer listener.Close()
	gin.SetMode(gin.ReleaseMode)

	err = runAPIServer(gin.New(), listener.Addr().String())
	if telemetry.ErrorCode(err, "") != "server.listen.failed" {
		t.Fatalf("error code = %q, error = %v", telemetry.ErrorCode(err, ""), err)
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("listen cause was lost: %v", err)
	}
}

func runAPIStartupHelper(t *testing.T, format, redisAddress, databaseURL string) (string, string, error) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestAPIStartupHelper$")
	command.Env = append(os.Environ(),
		"LUNA_TEST_API_STARTUP_HELPER=1",
		"ENV_FILE=",
		"APP_ENV=development",
		"LOG_FORMAT="+format,
		"LOG_COLOR=never",
		"LOG_LEVEL=info",
		"OTEL_EXPORTER_OTLP_ENDPOINT=",
		"REDIS_ADDR="+redisAddress,
		"DATABASE_URL="+databaseURL,
		"METRICS_ENABLED=false",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}
