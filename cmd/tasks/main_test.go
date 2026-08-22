package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresCommand(t *testing.T) {
	var output bytes.Buffer
	err := run(nil, &output)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("err = %v", err)
	}
}

func TestCommandFailureKeepsStdoutClean(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"unsupported"}, &output)
	if err == nil {
		t.Fatal("run() error = nil")
	}
	if output.Len() != 0 {
		t.Fatalf("stdout was polluted: %q", output.String())
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("stdout contains ANSI: %q", output.String())
	}
}
