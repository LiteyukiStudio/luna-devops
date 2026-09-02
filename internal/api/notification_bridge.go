package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/notificationapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type notificationHost struct {
	handlers *Handlers
}

func (host notificationHost) DBFor(ctx *gin.Context) *gorm.DB {
	return host.handlers.dbFor(ctx)
}

func (host notificationHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}

func (host notificationHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}

func (host notificationHost) RequirePlatformAdmin(ctx *gin.Context) bool {
	return host.handlers.requirePlatformAdmin(ctx)
}

func (host notificationHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
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

func (h *Handlers) notificationAPI() *notificationapi.Handler {
	return notificationapi.New(notificationHost{handlers: h})
}

func (h *Handlers) GetPlatformMailSettings(ctx *gin.Context) {
	h.notificationAPI().GetPlatformMailSettings(ctx)
}

func (h *Handlers) UpdatePlatformMailSettings(ctx *gin.Context) {
	h.notificationAPI().UpdatePlatformMailSettings(ctx)
}

func (h *Handlers) TestPlatformMailSettings(ctx *gin.Context) {
	h.notificationAPI().TestPlatformMailSettings(ctx)
}

func (h *Handlers) ListNotificationPresets(ctx *gin.Context) {
	h.notificationAPI().ListNotificationPresets(ctx)
}

func (h *Handlers) ListNotificationChannels(ctx *gin.Context) {
	h.notificationAPI().ListNotificationChannels(ctx)
}

func (h *Handlers) CreateNotificationChannel(ctx *gin.Context) {
	h.notificationAPI().CreateNotificationChannel(ctx)
}

func (h *Handlers) UpdateNotificationChannel(ctx *gin.Context) {
	h.notificationAPI().UpdateNotificationChannel(ctx)
}

func (h *Handlers) DeleteNotificationChannel(ctx *gin.Context) {
	h.notificationAPI().DeleteNotificationChannel(ctx)
}

func (h *Handlers) TestNotificationChannel(ctx *gin.Context) {
	h.notificationAPI().TestNotificationChannel(ctx)
}

func (h *Handlers) CreateNotificationChannelFromPreset(ctx *gin.Context) {
	h.notificationAPI().CreateNotificationChannelFromPreset(ctx)
}

func (h *Handlers) ListNotificationTemplates(ctx *gin.Context) {
	h.notificationAPI().ListNotificationTemplates(ctx)
}

func (h *Handlers) CreateNotificationTemplate(ctx *gin.Context) {
	h.notificationAPI().CreateNotificationTemplate(ctx)
}

func (h *Handlers) UpdateNotificationTemplate(ctx *gin.Context) {
	h.notificationAPI().UpdateNotificationTemplate(ctx)
}

func (h *Handlers) DeleteNotificationTemplate(ctx *gin.Context) {
	h.notificationAPI().DeleteNotificationTemplate(ctx)
}

func (h *Handlers) ListNotificationRules(ctx *gin.Context) {
	h.notificationAPI().ListNotificationRules(ctx)
}

func (h *Handlers) CreateNotificationRule(ctx *gin.Context) {
	h.notificationAPI().CreateNotificationRule(ctx)
}

func (h *Handlers) UpdateNotificationRule(ctx *gin.Context) {
	h.notificationAPI().UpdateNotificationRule(ctx)
}

func (h *Handlers) DeleteNotificationRule(ctx *gin.Context) {
	h.notificationAPI().DeleteNotificationRule(ctx)
}

func (h *Handlers) ListNotificationDeliveries(ctx *gin.Context) {
	h.notificationAPI().ListNotificationDeliveries(ctx)
}

func (h *Handlers) GetMyNotificationPreferences(ctx *gin.Context) {
	h.notificationAPI().GetMyNotificationPreferences(ctx)
}

func (h *Handlers) UpdateMyNotificationPreferences(ctx *gin.Context) {
	h.notificationAPI().UpdateMyNotificationPreferences(ctx)
}

func (h *Handlers) ListMyNotificationPresets(ctx *gin.Context) {
	h.notificationAPI().ListMyNotificationPresets(ctx)
}

func (h *Handlers) ListMyNotificationChannels(ctx *gin.Context) {
	h.notificationAPI().ListMyNotificationChannels(ctx)
}

func (h *Handlers) CreateMyNotificationChannel(ctx *gin.Context) {
	h.notificationAPI().CreateMyNotificationChannel(ctx)
}

func (h *Handlers) UpdateMyNotificationChannel(ctx *gin.Context) {
	h.notificationAPI().UpdateMyNotificationChannel(ctx)
}

func (h *Handlers) DeleteMyNotificationChannel(ctx *gin.Context) {
	h.notificationAPI().DeleteMyNotificationChannel(ctx)
}

func (h *Handlers) TestMyNotificationChannel(ctx *gin.Context) {
	h.notificationAPI().TestMyNotificationChannel(ctx)
}

func (h *Handlers) ListMyNotificationDeliveries(ctx *gin.Context) {
	h.notificationAPI().ListMyNotificationDeliveries(ctx)
}

func (h *Handlers) ListInboxMessages(ctx *gin.Context) {
	h.notificationAPI().ListInboxMessages(ctx)
}

func (h *Handlers) GetInboxUnreadCount(ctx *gin.Context) {
	h.notificationAPI().GetInboxUnreadCount(ctx)
}

func (h *Handlers) GetInboxMessage(ctx *gin.Context) {
	h.notificationAPI().GetInboxMessage(ctx)
}

func (h *Handlers) MarkInboxMessageRead(ctx *gin.Context) {
	h.notificationAPI().MarkInboxMessageRead(ctx)
}

func (h *Handlers) MarkAllInboxMessagesRead(ctx *gin.Context) {
	h.notificationAPI().MarkAllInboxMessagesRead(ctx)
}

func (h *Handlers) ArchiveInboxMessage(ctx *gin.Context) {
	h.notificationAPI().ArchiveInboxMessage(ctx)
}

func (h *Handlers) DecideInboxActionRequest(ctx *gin.Context) {
	h.notificationAPI().DecideInboxActionRequest(ctx)
}

func (h *Handlers) StreamInboxChanges(ctx *gin.Context) {
	h.notificationAPI().StreamInboxChanges(ctx)
}

func (h *Handlers) allowPersonalNotificationTest(ctx *gin.Context, userID string) bool {
	if h.rateLimiter == nil {
		h.rateLimiter = newRateLimiter()
	}
	allowed, err := h.rateLimiter.allow(
		ctx.Request.Context(),
		"notification_test_user:"+hashToken(strings.TrimSpace(userID)),
		notificationapi.PersonalNotificationTestRateLimit,
		time.Minute,
	)
	if err != nil {
		if h.mode == "development" {
			return true
		}
		writeErrorCode(ctx, http.StatusServiceUnavailable, "notification.test_rate_limit_unavailable", "personal notification test rate limit is unavailable")
		return false
	}
	if !allowed {
		writeErrorCode(ctx, http.StatusTooManyRequests, "notification.test_rate_limited", "personal notification test rate limit exceeded")
		return false
	}
	return true
}

func writeInboxError(ctx *gin.Context, err error) {
	notificationapi.WriteInboxError(ctx, err)
}

var defaultInboxBroker = notificationapi.DefaultInboxBroker()

type projectMemberInboxInput = notificationapi.ProjectMemberInboxInput

func publishProjectMemberInbox(ctx context.Context, tx *gorm.DB, input projectMemberInboxInput) error {
	return notificationapi.PublishProjectMemberInbox(ctx, tx, input)
}
