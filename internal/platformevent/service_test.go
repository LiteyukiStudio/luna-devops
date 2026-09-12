package platformevent

import (
	"strings"
	"testing"
	"time"
)

func TestNewRecordDerivesEventMetadata(t *testing.T) {
	occurredAt := time.Date(2026, 7, 11, 9, 30, 0, 0, time.UTC)
	event := NewRecord(RecordInput{
		Type:       "gateway.apply_failed",
		ProjectID:  "prj_1",
		ResourceID: "gwr_1",
		DedupKey:   "gateway:gwr_1:apply_failed",
		OccurredAt: occurredAt,
	})

	if event.ID == "" || !strings.HasPrefix(event.ID, "evt_") {
		t.Fatalf("event id = %q, want generated evt id", event.ID)
	}
	if event.Category != "gateway" || event.Status != StatusFailed {
		t.Fatalf("category/status = %q/%q, want gateway/failed", event.Category, event.Status)
	}
	if event.SummaryKey != "events.types.gateway_apply_failed" {
		t.Fatalf("summary key = %q", event.SummaryKey)
	}
	if event.DedupKey == nil || *event.DedupKey != "gateway:gwr_1:apply_failed" {
		t.Fatalf("dedup key = %#v", event.DedupKey)
	}
	if !event.OccurredAt.Equal(occurredAt) {
		t.Fatalf("occurred at = %s, want %s", event.OccurredAt, occurredAt)
	}
}

func TestNewRecordRedactsSensitiveContent(t *testing.T) {
	event := NewRecord(RecordInput{
		Type:    "build.failed",
		Message: "request failed Authorization: Bearer top-secret password=hunter2",
		Detail: map[string]string{
			"url": "https://x-access-token:git-token@example.com/repo?access_token=query-token",
		},
		Links: map[string]string{
			"detail": "https://example.com/build?token=link-token",
		},
	})

	for _, value := range []string{event.Message, event.DetailJSON, event.LinksJSON} {
		for _, secret := range []string{"top-secret", "hunter2", "git-token", "query-token", "link-token"} {
			if strings.Contains(value, secret) {
				t.Fatalf("stored event contains secret %q: %s", secret, value)
			}
		}
	}
	if !strings.Contains(event.Message, redactedValue) || !strings.Contains(event.DetailJSON, redactedValue) {
		t.Fatalf("redacted marker missing: message=%q detail=%q", event.Message, event.DetailJSON)
	}
}

func TestStatusForType(t *testing.T) {
	tests := map[string]string{
		"build.started":        StatusInProgress,
		"build.succeeded":      StatusSucceeded,
		"release.failed":       StatusFailed,
		"gateway.apply_failed": StatusFailed,
		"hook.canceled":        StatusCanceled,
	}
	for eventType, expected := range tests {
		if actual := StatusForType(eventType); actual != expected {
			t.Fatalf("StatusForType(%q) = %q, want %q", eventType, actual, expected)
		}
	}
}

func TestCatalogContainsUniqueTypes(t *testing.T) {
	seen := map[string]bool{}
	for _, entry := range Catalog() {
		if seen[entry.Type] {
			t.Fatalf("duplicate catalog entry %q", entry.Type)
		}
		seen[entry.Type] = true
		if entry.Category != CategoryForType(entry.Type) {
			t.Fatalf("catalog category for %q = %q", entry.Type, entry.Category)
		}
	}
}
