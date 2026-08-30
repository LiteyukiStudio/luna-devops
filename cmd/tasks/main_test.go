package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
)

func testTasksConfig() tasksConfig {
	return tasksConfig{Redis: redisconfig.Options{Addr: "127.0.0.1:6379"}}
}

func TestRunRequiresCommand(t *testing.T) {
	var output bytes.Buffer
	err := run(nil, &output, testTasksConfig())
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("err = %v", err)
	}
}

func TestCommandFailureKeepsStdoutClean(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"unsupported"}, &output, testTasksConfig())
	if err == nil {
		t.Fatal("run() error = nil")
	}
	if output.Len() != 0 {
		t.Fatalf("stdout was polluted: %q", output.String())
	}
}
