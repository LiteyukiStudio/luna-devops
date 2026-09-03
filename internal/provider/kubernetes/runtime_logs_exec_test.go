package kubernetes

import (
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	clientexec "k8s.io/client-go/util/exec"
)

func TestRuntimeExecOutputIsBoundedAcrossStreams(t *testing.T) {
	output := newRuntimeExecOutput(8)
	if _, err := output.writer(false).Write([]byte("stdout")); err != nil {
		t.Fatal(err)
	}
	if _, err := output.writer(true).Write([]byte("stderr")); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, truncated := output.snapshot()
	if stdout != "stdout" || stderr != "st" || !truncated {
		t.Fatalf("bounded output = stdout %q, stderr %q, truncated %v", stdout, stderr, truncated)
	}
	if len(stdout)+len(stderr) != 8 || strings.Contains(stdout+stderr, "derr") {
		t.Fatalf("combined output exceeded its limit: %q / %q", stdout, stderr)
	}
}

func TestRuntimeExecExitCodePreservesCommandStatus(t *testing.T) {
	code, exited := runtimeExecExitCode(clientexec.CodeExitError{Err: errors.New("command failed"), Code: 42})
	if !exited || code != 42 {
		t.Fatalf("runtime exec exit = (%d, %t), want (42, true)", code, exited)
	}
}

func TestRuntimeExecExitCodeRejectsTransportFailure(t *testing.T) {
	code, exited := runtimeExecExitCode(errors.New("transport unavailable"))
	if exited || code != 0 {
		t.Fatalf("transport failure exit = (%d, %t), want (0, false)", code, exited)
	}
}

func TestRuntimeTerminalResultTreatsRemoteExitAsCompletedSession(t *testing.T) {
	result, err := runtimeTerminalResult(clientexec.CodeExitError{Err: errors.New("command failed"), Code: 37})
	if err != nil || result.ExitCode != 37 {
		t.Fatalf("runtime terminal result = (%#v, %v), want exit code 37 without transport error", result, err)
	}
}

func TestRuntimeTerminalResultPreservesTransportFailure(t *testing.T) {
	transportErr := errors.New("transport unavailable")
	if _, err := runtimeTerminalResult(transportErr); !errors.Is(err, transportErr) {
		t.Fatalf("runtime terminal transport error = %v, want %v", err, transportErr)
	}
}

func TestSelectPodContainer(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: "sidecar"},
			},
		},
	}

	selected, err := selectPodContainer(pod, "")
	if err != nil {
		t.Fatalf("selectPodContainer returned error: %v", err)
	}
	if selected != "app" {
		t.Fatalf("selectPodContainer default = %q, want app", selected)
	}

	selected, err = selectPodContainer(pod, "sidecar")
	if err != nil {
		t.Fatalf("selectPodContainer sidecar returned error: %v", err)
	}
	if selected != "sidecar" {
		t.Fatalf("selectPodContainer sidecar = %q, want sidecar", selected)
	}

	if _, err := selectPodContainer(pod, "missing"); err == nil {
		t.Fatal("expected missing container to fail")
	}
}
