package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/notificationapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

type notificationHost struct {
	domainHost
}

func (host notificationHost) Secrets() notificationapi.SecretStore {
	return host.handlers.secrets
}

func (host notificationHost) NormalizeRegistrationEmail(value string) (string, error) {
	return normalizedRegistrationEmail(value)
}

func (host notificationHost) AllowPersonalNotificationTest(ctx *gin.Context, userID string) bool {
	return host.handlers.allowPersonalNotificationTest(ctx, userID)
}

func (host notificationHost) InboxService() notificationapi.InboxService {
	return host.handlers.inbox
}

func (host notificationHost) InboxDecisionAvailable() bool {
	return host.handlers.inboxDecision != nil
}

func (host notificationHost) DecideInboxAction(
	ctx context.Context,
	user model.User,
	requestID string,
	decision string,
	expectedVersion int64,
) error {
	if host.handlers.inboxDecision == nil {
		return notificationapi.ErrInboxDecisionUnavailable
	}
	return host.handlers.inboxDecision(ctx, user, requestID, decision, expectedVersion)
}

func (h *Handlers) allowPersonalNotificationTest(ctx *gin.Context, userID string) bool {
	if h.rateLimiter == nil {
		h.rateLimiter = newRateLimiter()
	}
	allowed, err := h.rateLimiter.allow(
		ctx.Request.Context(),
		"notification_test_user:"+transportapi.HashToken(strings.TrimSpace(userID)),
		notificationapi.PersonalNotificationTestRateLimit,
		time.Minute,
	)
	if err != nil {
		if h.mode == "development" {
			return true
		}
		transportapi.WriteErrorCode(ctx, http.StatusServiceUnavailable, "notification.test_rate_limit_unavailable", "personal notification test rate limit is unavailable")
		return false
	}
	if !allowed {
		transportapi.WriteErrorCode(ctx, http.StatusTooManyRequests, "notification.test_rate_limited", "personal notification test rate limit exceeded")
		return false
	}
	return true
}
