package projectapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

const defaultContinuousAuthorizationCheckInterval = 20 * time.Second

// continuousAuthorizationBinding pins a long-lived request to the exact
// session or access-token row that authenticated it. Project authorization is
// deliberately checked separately so identity and resource policy remain
// independently testable.
type continuousAuthorizationBinding struct {
	UserID             string
	SubjectID          string
	AccessTokenSource  string
	AccessTokenScope   string
	OAuthApplicationID string
	OAuthGrantID       string
	OAuthFamilyID      string
	Deadline           time.Time
}

// Runtime terminal tickets and volume download tickets use the same identity
// binding as every other long-lived request; this alias prevents a second
// authorization contract from drifting from the shared monitor.
type runtimeTerminalAuthorizationBinding = continuousAuthorizationBinding

type continuousAuthorizationState struct {
	Session              model.UserSession
	AccessToken          model.AccessToken
	OAuthGrant           model.OAuthGrant
	OAuthApplication     model.OAuthApplication
	User                 model.User
	AuthorizationAllowed bool
}

type runtimeTerminalAuthorizationState = continuousAuthorizationState

func (state continuousAuthorizationState) active(binding continuousAuthorizationBinding, now time.Time) bool {
	return state.AuthorizationAllowed && state.identityActive(binding, now)
}

func (state continuousAuthorizationState) identityActive(binding continuousAuthorizationBinding, now time.Time) bool {
	if binding.UserID == "" || binding.SubjectID == "" || (!binding.Deadline.IsZero() && !binding.Deadline.After(now)) {
		return false
	}
	if tokenID, tokenBound := continuousAuthorizationAccessTokenID(binding.SubjectID); tokenBound {
		token := state.AccessToken
		if token.ID != tokenID || token.UserID != binding.UserID || token.RevokedAt != nil ||
			token.Source != binding.AccessTokenSource || token.Scope != binding.AccessTokenScope ||
			token.OAuthApplicationID != binding.OAuthApplicationID || token.OAuthGrantID != binding.OAuthGrantID ||
			token.OAuthFamilyID != binding.OAuthFamilyID ||
			(token.ExpiresAt != nil && !token.ExpiresAt.After(now)) {
			return false
		}
		switch token.Source {
		case "personal":
		case "oauth":
			if token.OAuthApplicationID == "" || token.OAuthGrantID == "" || token.OAuthFamilyID == "" ||
				state.OAuthGrant.ID != token.OAuthGrantID ||
				state.OAuthGrant.UserID != binding.UserID ||
				state.OAuthGrant.ApplicationID != token.OAuthApplicationID ||
				state.OAuthGrant.RevokedAt != nil ||
				state.OAuthApplication.ID != token.OAuthApplicationID ||
				state.OAuthApplication.RevokedAt != nil {
				return false
			}
		default:
			return false
		}
	} else if state.Session.ID != binding.SubjectID || state.Session.UserID != binding.UserID || !state.Session.ExpiresAt.After(now) {
		return false
	}
	return state.User.ID == binding.UserID && !state.User.Disabled
}

func (h *Handler) currentContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (continuousAuthorizationBinding, bool) {
	if h.host.RequestUsesBearerToken(ctx) {
		token, ok := h.host.CurrentAccessTokenFromContext(ctx)
		if !ok || token.UserID != user.ID {
			return continuousAuthorizationBinding{}, false
		}
		return continuousAuthorizationBindingForAccessToken(user.ID, token), true
	}
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok || session.UserID != user.ID {
		return continuousAuthorizationBinding{}, false
	}
	return continuousAuthorizationBinding{UserID: user.ID, SubjectID: session.ID, Deadline: session.ExpiresAt}, true
}

func continuousAuthorizationBindingForAccessToken(userID string, token model.AccessToken) continuousAuthorizationBinding {
	binding := continuousAuthorizationBinding{
		UserID: userID, SubjectID: continuousAccessTokenSubject(token.ID),
		AccessTokenSource: token.Source, AccessTokenScope: token.Scope,
		OAuthApplicationID: token.OAuthApplicationID, OAuthGrantID: token.OAuthGrantID, OAuthFamilyID: token.OAuthFamilyID,
	}
	if token.ExpiresAt != nil {
		binding.Deadline = *token.ExpiresAt
	}
	return binding
}

func (h *Handler) requireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (continuousAuthorizationBinding, bool) {
	binding, ok := h.currentContinuousAuthorizationBinding(ctx, user)
	if ok {
		return binding, true
	}
	if h.host.RequestUsesBearerToken(ctx) {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.token.invalid")
	} else {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
	}
	return continuousAuthorizationBinding{}, false
}

func writeContinuousAuthorizationRevoked(ctx *gin.Context) {
	writeErrorCode(ctx, http.StatusUnauthorized, "auth.authorization_revoked", "stream authorization expired or was revoked")
}

func continuousAccessTokenSubject(tokenID string) string {
	return "access-token:" + strings.TrimSpace(tokenID)
}

func continuousAuthorizationAccessTokenID(subject string) (string, bool) {
	const prefix = "access-token:"
	tokenID := strings.TrimSpace(strings.TrimPrefix(subject, prefix))
	return tokenID, strings.HasPrefix(subject, prefix) && tokenID != ""
}

func (h *Handler) continuousAuthorizationCheckInterval() time.Duration {
	if h != nil && h.host != nil && h.host.ContinuousAuthorizationInterval() > 0 {
		return h.host.ContinuousAuthorizationInterval()
	}
	return defaultContinuousAuthorizationCheckInterval
}

// monitorContinuousAuthorization performs an immediate fail-closed check
// before a caller can emit headers or consume request content, then repeats the
// same authoritative check for the lifetime of the request.
func (h *Handler) monitorContinuousAuthorization(
	ctx context.Context,
	binding continuousAuthorizationBinding,
	authorizationAllowed func(context.Context, model.User) bool,
	revoke func(),
) (<-chan struct{}, bool) {
	return h.monitorContinuousAuthorizationWithInterval(
		ctx, binding, authorizationAllowed, revoke, h.continuousAuthorizationCheckInterval(),
	)
}

func (h *Handler) monitorContinuousAuthorizationWithInterval(
	ctx context.Context,
	binding continuousAuthorizationBinding,
	authorizationAllowed func(context.Context, model.User) bool,
	revoke func(),
	checkInterval time.Duration,
) (<-chan struct{}, bool) {
	revoked := make(chan struct{})
	revokeOnce := func() {
		if revoke != nil {
			revoke()
		}
		close(revoked)
	}
	if checkInterval <= 0 || !h.continuousAuthorizationActive(ctx, binding, authorizationAllowed) {
		revokeOnce()
		return revoked, false
	}
	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if h.continuousAuthorizationActive(ctx, binding, authorizationAllowed) {
					continue
				}
				revokeOnce()
				return
			}
		}
	}()
	return revoked, true
}

func (h *Handler) continuousAuthorizationActive(
	ctx context.Context,
	binding continuousAuthorizationBinding,
	authorizationAllowed func(context.Context, model.User) bool,
) bool {
	state, ok := h.continuousAuthorizationIdentityState(ctx, binding)
	if !ok || authorizationAllowed == nil {
		return false
	}
	state.AuthorizationAllowed = authorizationAllowed(ctx, state.User)
	return state.active(binding, time.Now())
}

func (h *Handler) continuousAuthorizationIdentityState(ctx context.Context, binding continuousAuthorizationBinding) (continuousAuthorizationState, bool) {
	state := continuousAuthorizationState{}
	db := h.dbWithContext(ctx)
	if db == nil {
		return state, false
	}
	if tokenID, tokenBound := continuousAuthorizationAccessTokenID(binding.SubjectID); tokenBound {
		if err := db.First(&state.AccessToken, "id = ? and user_id = ?", tokenID, binding.UserID).Error; err != nil {
			return state, false
		}
		if state.AccessToken.Source == "oauth" {
			if err := db.First(&state.OAuthGrant, "id = ? and user_id = ? and application_id = ?", state.AccessToken.OAuthGrantID, binding.UserID, state.AccessToken.OAuthApplicationID).Error; err != nil {
				return state, false
			}
			if err := db.First(&state.OAuthApplication, "id = ?", state.AccessToken.OAuthApplicationID).Error; err != nil {
				return state, false
			}
		}
	} else if err := db.First(&state.Session, "id = ? and user_id = ?", binding.SubjectID, binding.UserID).Error; err != nil {
		return state, false
	}
	if err := db.First(&state.User, "id = ? and disabled = ?", binding.UserID, false).Error; err != nil {
		return state, false
	}
	return state, state.identityActive(binding, time.Now())
}

func (h *Handler) projectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	db := h.dbWithContext(ctx)
	if db == nil {
		return false
	}
	var project model.Project
	if err := db.First(&project, "id = ?", projectID).Error; err != nil || !resourceCanMutateDuringDelete(project.DeleteStatus) {
		return false
	}
	_, err := h.projectAuthorizer(ctx).AuthorizeProject(ctx, authz.ProjectSubject{
		UserID: user.ID, PlatformRole: user.Role,
	}, projectID, action)
	return err == nil
}

func replaceRequestContext(ctx *gin.Context, requestCtx context.Context) func() {
	original := ctx.Request
	ctx.Request = original.WithContext(requestCtx)
	return func() { ctx.Request = original }
}
