package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestWorkerStartupHelper(t *testing.T) {
	if os.Getenv("LUNA_TEST_WORKER_STARTUP_HELPER") != "1" {
		return
	}
	os.Exit(runMain())
}

func TestWorkerStartupRedisUnavailableLogsCompleteDiagnostics(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestWorkerStartupHelper$")
	command.Env = append(os.Environ(),
		"LUNA_TEST_WORKER_STARTUP_HELPER=1",
		"ENV_FILE=",
		"APP_ENV=development",
		"LOG_FORMAT=console",
		"LOG_COLOR=never",
		"LOG_LEVEL=info",
		"OTEL_EXPORTER_OTLP_ENDPOINT=",
		"REDIS_ADDR=redis://127.0.0.1:1/0",
		"DATABASE_URL=postgres://devops:devops@127.0.0.1:1/devops?sslmode=disable&connect_timeout=1",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err == nil {
		t.Fatal("Worker startup unexpectedly succeeded")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout was polluted: %q", stdout.String())
	}
	for _, expected := range []string{"Worker startup failed", "dependency.redis.unavailable", "connect Redis: ping Redis: dial tcp 127.0.0.1:1", "start Redis or verify REDIS_ADDR"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("Worker diagnostic omitted %q: %s", expected, stderr.String())
		}
	}
}
