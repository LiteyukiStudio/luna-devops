package model

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeSourceCommitMessageTrimsAndTruncatesOnRuneBoundary(t *testing.T) {
	if got := NormalizeSourceCommitMessage("  fix: bug\n\nbody\n "); got != "fix: bug\n\nbody" {
		t.Fatalf("trimmed message = %q", got)
	}
	long := strings.Repeat("提交", MaxSourceCommitMessageBytes)
	got := NormalizeSourceCommitMessage(long)
	if len(got) > MaxSourceCommitMessageBytes || !utf8.ValidString(got) || !strings.HasPrefix(long, got) {
		t.Fatalf("truncated message len=%d valid=%v", len(got), utf8.ValidString(got))
	}
}
