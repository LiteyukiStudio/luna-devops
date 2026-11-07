package builder

import "testing"

func observeAll(counter *StepCounter, lines ...string) {
	for _, line := range lines {
		counter.Observe(line)
	}
}

func assertSteps(t *testing.T, counter *StepCounter, wantCompleted int, wantTotal int) {
	t.Helper()
	if completed, total := counter.Counts(); completed != wantCompleted || total != wantTotal {
		t.Fatalf("steps = %d/%d, want %d/%d", completed, total, wantCompleted, wantTotal)
	}
}

func TestStepCounterCountsSingleStageSteps(t *testing.T) {
	counter := NewStepCounter()
	observeAll(counter,
		"#1 [internal] load build definition from Dockerfile",
		"#1 DONE 0.1s",
		"#5 [1/3] FROM docker.io/library/alpine:3.20@sha256:abc",
		"#5 DONE 1.2s",
		"#6 [2/3] RUN apk add --no-cache curl",
		"#6 0.512 fetch https://dl-cdn.alpinelinux.org/alpine",
	)
	assertSteps(t, counter, 1, 3)
	if !counter.Observe("#6 DONE 3.4s") {
		t.Fatal("completing a step should report a change")
	}
	observeAll(counter, "#7 [3/3] COPY . /app", "#7 CACHED")
	assertSteps(t, counter, 3, 3)
}

func TestStepCounterAggregatesStagesAndDeduplicatesRepeatedVertexLines(t *testing.T) {
	counter := NewStepCounter()
	observeAll(counter,
		"#3 [builder 1/4] FROM docker.io/library/golang:1.24",
		"#4 [runner 1/3] FROM docker.io/library/alpine:3.20",
		"#3 DONE 2.0s",
		"#4 CACHED",
		"#9 [builder 2/4] RUN go build ./...",
	)
	assertSteps(t, counter, 2, 7)
	// 同一顶点会多次输出步骤行（例如 COPY 的多个子进度），只计一次。
	observeAll(counter, "#16 [runner 2/3] COPY --from=builder /out /app", "#16 [runner 2/3] COPY --from=builder /out /app", "#16 DONE 0.9s")
	assertSteps(t, counter, 3, 7)
	if counter.Observe("#16 DONE 0.9s") {
		t.Fatal("repeated completion should not report a change")
	}
}

func TestStepCounterIgnoresFailedAndUnrelatedLines(t *testing.T) {
	counter := NewStepCounter()
	observeAll(counter,
		"Cloning into '/workspace/source'...",
		"#2 [auth] library/alpine:pull token for registry-1.docker.io",
		"#2 DONE 0.0s",
		"#6 [2/2] RUN false",
		"#6 ERROR: process \"/bin/sh -c false\" did not complete successfully: exit code: 1",
		"#20 exporting to image",
		"#20 DONE 1.0s",
	)
	assertSteps(t, counter, 0, 2)
}

func TestStepCounterTracksCurrentInstruction(t *testing.T) {
	counter := NewStepCounter()
	if counter.Current() != "" {
		t.Fatalf("current before any step = %q", counter.Current())
	}
	observeAll(counter,
		"#3 [builder 1/2] FROM docker.io/library/golang:1.24",
		"#4 [runner 1/2] FROM docker.io/library/alpine:3.20",
		"#9 [builder 2/2] RUN go build ./...",
	)
	if got := counter.Current(); got != "[builder 2/2] RUN go build ./..." {
		t.Fatalf("current = %q", got)
	}
	// 最近开始的步骤完成后，回退到仍在执行的更早步骤。
	counter.Observe("#9 DONE 12.0s")
	if got := counter.Current(); got != "[runner 1/2] FROM docker.io/library/alpine:3.20" {
		t.Fatalf("current after newest step done = %q", got)
	}
	observeAll(counter, "#3 DONE 1.0s", "#4 DONE 1.0s")
	if got := counter.Current(); got != "[builder 2/2] RUN go build ./..." {
		t.Fatalf("current after all done = %q", got)
	}
}
