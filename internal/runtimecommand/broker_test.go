package runtimecommand

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBrokerMaintainsShellStateAndBinding(t *testing.T) {
	broker := NewBroker(Options{IdleTTL: time.Minute, AbsoluteTTL: time.Minute})
	t.Cleanup(func() { _ = broker.Shutdown(context.Background()) })
	binding := Binding{UserID: "usr_1", SubjectID: "ses_1", AgentRunID: "run_1", ProjectID: "prj_1", ReleaseID: "rel_1", DeploymentTargetID: "dpt_1"}
	snapshot, err := broker.Create(context.Background(), binding, fakeShell)
	if err != nil {
		t.Fatal(err)
	}
	first, err := broker.Execute(context.Background(), snapshot.ID, binding, "cd /tmp")
	if err != nil || first.ExitCode != 0 {
		t.Fatalf("first command = %#v, %v", first, err)
	}
	second, err := broker.Execute(context.Background(), snapshot.ID, binding, "pwd")
	if err != nil || strings.TrimSpace(second.Stdout) != "/tmp" {
		t.Fatalf("persistent pwd = %#v, %v", second, err)
	}
	other := binding
	other.AgentRunID = "run_2"
	if _, err := broker.Execute(context.Background(), snapshot.ID, other, "pwd"); !errors.Is(err, ErrBindingMismatch) {
		t.Fatalf("binding mismatch = %v", err)
	}
}

func TestBrokerExpiresIdleSession(t *testing.T) {
	now := time.Now()
	broker := NewBroker(Options{IdleTTL: time.Second, AbsoluteTTL: time.Minute, Now: func() time.Time { return now }})
	t.Cleanup(func() { _ = broker.Shutdown(context.Background()) })
	binding := Binding{UserID: "usr_1", SubjectID: "ses_1", ProjectID: "prj_1", ReleaseID: "rel_1", DeploymentTargetID: "dpt_1"}
	snapshot, err := broker.Create(context.Background(), binding, fakeShell)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, err := broker.Execute(context.Background(), snapshot.ID, binding, "pwd"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired execute = %v", err)
	}
}

func TestBrokerClosesSession(t *testing.T) {
	broker := NewBroker(Options{})
	t.Cleanup(func() { _ = broker.Shutdown(context.Background()) })
	binding := Binding{UserID: "usr_1", SubjectID: "ses_1", ProjectID: "prj_1", ReleaseID: "rel_1", DeploymentTargetID: "dpt_1"}
	snapshot, err := broker.Create(context.Background(), binding, fakeShell)
	if err != nil {
		t.Fatal(err)
	}
	if err := broker.Close(context.Background(), snapshot.ID, binding); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Execute(context.Background(), snapshot.ID, binding, "pwd"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("closed execute = %v", err)
	}
}

func TestBrokerRejectsAnotherInstanceOwner(t *testing.T) {
	first := NewBroker(Options{InstanceID: "api-one"})
	second := NewBroker(Options{InstanceID: "api-two"})
	t.Cleanup(func() { _ = first.Shutdown(context.Background()); _ = second.Shutdown(context.Background()) })
	binding := Binding{UserID: "usr_1", SubjectID: "ses_1", ProjectID: "prj_1", ReleaseID: "rel_1", DeploymentTargetID: "dpt_1"}
	snapshot, err := first.Create(context.Background(), binding, fakeShell)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Execute(context.Background(), snapshot.ID, binding, "pwd"); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("owner mismatch = %v", err)
	}
}

func TestBrokerBoundsOutputAndKeepsCompletionMarker(t *testing.T) {
	broker := NewBroker(Options{OutputLimit: 1024, CommandTTL: time.Second})
	t.Cleanup(func() { _ = broker.Shutdown(context.Background()) })
	binding := Binding{UserID: "usr_1", SubjectID: "ses_1", ProjectID: "prj_1", ReleaseID: "rel_1", DeploymentTargetID: "dpt_1"}
	snapshot, err := broker.Create(context.Background(), binding, fakeShell)
	if err != nil {
		t.Fatal(err)
	}
	result, err := broker.Execute(context.Background(), snapshot.ID, binding, "yes x | head -c 5000")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || result.ExitCode != 0 || len(result.Stdout) > 1024 {
		t.Fatalf("bounded output = %#v", result)
	}
}

func TestBrokerTimeoutInvalidatesUnknownShellState(t *testing.T) {
	broker := NewBroker(Options{CommandTTL: 20 * time.Millisecond})
	t.Cleanup(func() { _ = broker.Shutdown(context.Background()) })
	binding := Binding{UserID: "usr_1", SubjectID: "ses_1", ProjectID: "prj_1", ReleaseID: "rel_1", DeploymentTargetID: "dpt_1"}
	snapshot, err := broker.Create(context.Background(), binding, fakeShell)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Execute(context.Background(), snapshot.ID, binding, "sleep 1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout = %v", err)
	}
	if _, err := broker.Execute(context.Background(), snapshot.ID, binding, "pwd"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("execute after timeout = %v", err)
	}
}

func TestCommandMarkersWaitForCompleteStatusTerminator(t *testing.T) {
	begin := "\x1eLUNA_BEGIN_token\x1f\n"
	end := "\x1eLUNA_END_token:"
	if hasCompleteCommandMarkers([]byte(begin+"output\n"+end), begin, end) {
		t.Fatal("incomplete end marker must not finish the command")
	}
	if hasCompleteCommandMarkers([]byte(begin+"output\n"+end+"0"), begin, end) {
		t.Fatal("status without terminator must not finish the command")
	}
	if !hasCompleteCommandMarkers([]byte(begin+"output\n"+end+"0\x1f\n"), begin, end) {
		t.Fatal("complete status marker must finish the command")
	}
}

func fakeShell(ctx context.Context, input io.Reader, output io.Writer) error {
	command := exec.CommandContext(ctx, "/bin/sh")
	command.Stdin = input
	command.Stdout = output
	command.Stderr = output
	return command.Run()
}
