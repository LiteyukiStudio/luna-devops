package aiapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	aiRunIDHeader      = "X-Luna-AI-Run-ID"
	aiToolCallIDHeader = "X-Luna-AI-Tool-Call-ID"
)

func aiOperationScopesAllowed(role string, scopes []string) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, rawScope := range scopes {
		scope := strings.TrimSpace(rawScope)
		if scope == "" {
			return false
		}
		if scope == "web:read" {
			continue
		}
		normalized := authz.NormalizeOAuthScope(scope)
		if normalized == "" || normalized != scope || !authz.UserCanAuthorizeOAuthScope(role, normalized) {
			return false
		}
	}
	return true
}

func constantTimeTokenEqual(expected, actual string) bool {
	return expected != "" && len(expected) == len(actual) && subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

func (h *Handler) requireAIAgentService(ctx *gin.Context) bool {
	actual := strings.TrimSpace(strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer "))
	expected := strings.TrimSpace(h.host.AIAgentCallbackServiceToken())
	previous := strings.TrimSpace(h.host.AIAgentPreviousCallbackServiceToken())
	if expected == "" {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.agent_service_not_configured", "Agent service identity is not configured")
		return false
	}
	if !constantTimeTokenEqual(expected, actual) && !constantTimeTokenEqual(previous, actual) {
		writeErrorCode(ctx, http.StatusUnauthorized, "ai.agent_service_unauthorized", "Agent service identity is invalid")
		return false
	}
	return true
}

func (h *Handler) resolveActiveAIPlatformUser(ctx *gin.Context, userID, sessionID string) (model.User, bool) {
	now := time.Now()
	var user model.User
	if h.dbFor(ctx).First(&user, "id = ? and disabled = ?", userID, false).Error != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, false
	}
	var session model.UserSession
	if h.dbFor(ctx).First(&session, "id = ? and user_id = ? and expires_at > ?", sessionID, userID, now).Error != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return model.User{}, false
	}
	h.host.SetCurrentUser(ctx, user)
	return user, true
}

func (h *Handler) currentAIPlatformUser(ctx *gin.Context) (model.User, bool) {
	actor, ok := h.host.AIPlatformActor(ctx.Request.Context())
	if !ok || actor.UserID == "" || actor.SessionID == "" {
		return model.User{}, false
	}
	user, _ := h.resolveActiveAIPlatformUser(ctx, actor.UserID, actor.SessionID)
	return user, true
}
