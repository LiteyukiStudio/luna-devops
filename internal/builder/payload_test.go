package builder

import (
	"strings"
	"testing"
)

func TestExecutorRunsBuildKitOncePerBuildRun(t *testing.T) {
	script := ExecutorScript()
	if strings.Count(script, "buildctl-daemonless.sh build") != 1 {
		t.Fatalf("executor must contain exactly one BuildKit invocation")
	}
	start := strings.Index(script, "run_build() {")
	end := strings.Index(script, "\n}\n\nrun_hooks \"prePush\"")
	if start < 0 || end <= start {
		t.Fatal("run_build function was not found")
	}
	buildFunction := script[start:end]
	for _, retryMarker := range []string{"while ", "attempt=", "sleep "} {
		if strings.Contains(buildFunction, retryMarker) {
			t.Fatalf("run_build contains inner retry marker %q", retryMarker)
		}
	}
	if strings.Count(script, "run_build \"$@\"") != 1 {
		t.Fatal("executor must call run_build exactly once per task attempt")
	}
	if !strings.Contains(script, "clone_with_retry") {
		t.Fatal("source clone retry must remain independent from BuildKit execution")
	}
}
