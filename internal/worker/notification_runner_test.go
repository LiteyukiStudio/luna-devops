package worker

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/notification"
)

func TestNotificationSendErrorShouldSkipRetry(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   bool
	}{
		{name: "bad request", status: 400, want: true},
		{name: "unauthorized", status: 401, want: true},
		{name: "too many requests", status: 429, want: false},
		{name: "server error", status: 500, want: false},
		{name: "network error", status: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notificationSendErrorShouldSkipRetry(notification.SendResult{StatusCode: tt.status})
			if got != tt.want {
				t.Fatalf("notificationSendErrorShouldSkipRetry(%d) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestNotificationDeliverySucceededUpdatesOnlyUsesDeliveryColumns(t *testing.T) {
	finishedAt := time.Date(2026, 7, 11, 12, 56, 40, 0, time.UTC)
	updates := notificationDeliverySucceededUpdates(1250*time.Millisecond, `{"method":"POST"}`, `{"code":0}`, finishedAt)

	if _, exists := updates["last_delivered_at"]; exists {
		t.Fatal("delivery updates must not write channel-only last_delivered_at column")
	}
	if updates["status"] != "succeeded" || updates["duration_millis"] != int64(1250) {
		t.Fatalf("unexpected delivery updates: %#v", updates)
	}
	if got, ok := updates["finished_at"].(*time.Time); !ok || !got.Equal(finishedAt) {
		t.Fatalf("finished_at = %#v, want %s", updates["finished_at"], finishedAt)
	}
}
