package worker

import (
	"testing"

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
