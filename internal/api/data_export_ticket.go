package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	dataExportTicketTTL       = time.Minute
	dataExportTicketKeyPrefix = "data_export:ticket:"
)

var dataExportMemoryTickets sync.Map

type dataExportAuthorizationBinding struct {
	UserID                    string    `json:"userId"`
	SubjectID                 string    `json:"subjectId"`
	AssertionID               string    `json:"assertionId,omitempty"`
	AssertionRequired         bool      `json:"assertionRequired"`
	AssertionAbsoluteDeadline time.Time `json:"assertionAbsoluteDeadline,omitempty"`
	Deadline                  time.Time `json:"deadline"`
}

type dataExportTicketValue struct {
	Authorization dataExportAuthorizationBinding `json:"authorization"`
	ProjectID     string                         `json:"projectId"`
	ApplicationID string                         `json:"applicationId"`
	TargetID      string                         `json:"targetId"`
	ExpiresAt     time.Time                      `json:"expiresAt"`
}

type dataExportTicketResponse struct {
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (value dataExportTicketValue) matchesResource(projectID, applicationID, targetID string) bool {
	return value.ProjectID == projectID &&
		value.ApplicationID == applicationID &&
		value.TargetID == targetID
}

func (h *Handlers) requireDataExportAuthorizationBinding(ctx *gin.Context, user model.User) (dataExportAuthorizationBinding, bool) {
	subject, ok := h.currentStepUpSubject(ctx, user)
	if !ok {
		if requestUsesBearerToken(ctx) {
			h.audit(user.ID, "mfa.step_up_required", stepUpPurposeDataExport, false, "personal access tokens cannot authorize data export")
			writeErrorCode(ctx, http.StatusForbidden, "mfa.session_required", "个人令牌不能用于数据导出二次验证，请使用 Luna CLI OAuth 登录")
		} else {
			writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		}
		return dataExportAuthorizationBinding{}, false
	}

	binding := dataExportAuthorizationBinding{UserID: user.ID, SubjectID: subject}
	if _, oauth := dataExportOAuthGrantID(subject); oauth {
		token, tokenOK := currentAccessTokenFromContext(ctx)
		if !tokenOK ||
			token.UserID != user.ID ||
			token.Source != "oauth" ||
			token.OAuthApplicationID != lunaCLIApplicationID ||
			token.OAuthGrantID == "" {
			writeErrorCode(ctx, http.StatusForbidden, "mfa.session_required", "Luna CLI OAuth 登录状态无效")
			return dataExportAuthorizationBinding{}, false
		}
		binding.Deadline = time.Now().Add(defaultStepUpAbsoluteTimeout)
		if token.ExpiresAt != nil && token.ExpiresAt.Before(binding.Deadline) {
			binding.Deadline = *token.ExpiresAt
		}
	} else {
		session, sessionOK := h.currentSessionFromCookie(ctx)
		if !sessionOK || session.UserID != user.ID || session.ID != subject {
			writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
			return dataExportAuthorizationBinding{}, false
		}
		binding.Deadline = session.ExpiresAt
		if !h.stepUpMFAEnabled() {
			return binding, true
		}
	}

	// OAuth data export always requires a purpose-bound assertion, even when the
	// platform-wide browser step-up policy is disabled.
	if !h.requireMFAAssertion(ctx, user, stepUpPurposeDataExport) {
		return dataExportAuthorizationBinding{}, false
	}
	now := time.Now()
	var assertion model.StepUpAssertion
	if err := h.db.First(
		&assertion,
		"user_id = ? and session_id = ? and purpose = ? and idle_expires_at > ? and absolute_expires_at > ?",
		user.ID,
		subject,
		stepUpPurposeDataExport,
		now,
		now,
	).Error; err != nil || !stepUpAssertionActive(assertion, now) {
		writeMFARequired(ctx, stepUpPurposeDataExport)
		return dataExportAuthorizationBinding{}, false
	}
	binding.AssertionID = assertion.ID
	binding.AssertionRequired = true
	binding.AssertionAbsoluteDeadline = assertion.AbsoluteExpiresAt
	if assertion.AbsoluteExpiresAt.Before(binding.Deadline) {
		binding.Deadline = assertion.AbsoluteExpiresAt
	}
	return binding, true
}

func dataExportOAuthGrantID(subject string) (string, bool) {
	grantID := strings.TrimSpace(strings.TrimPrefix(subject, "oauth:"))
	return grantID, strings.HasPrefix(subject, "oauth:") && grantID != ""
}

func (h *Handlers) issueDataExportTicket(ctx context.Context, authorization deploymentTargetDataExportAuthorization) (string, time.Time, error) {
	expiresAt := time.Now().Add(dataExportTicketTTL)
	if authorization.binding.Deadline.Before(expiresAt) {
		expiresAt = authorization.binding.Deadline
	}
	if !expiresAt.After(time.Now()) {
		return "", time.Time{}, errors.New("data export authorization expired")
	}
	value := dataExportTicketValue{
		Authorization: authorization.binding,
		ProjectID:     authorization.project.ID,
		ApplicationID: authorization.app.ID,
		TargetID:      authorization.target.ID,
		ExpiresAt:     expiresAt,
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", time.Time{}, err
	}
	if h.rateLimiter != nil && h.rateLimiter.redis != nil {
		ticket := "r_" + randomHex(32)
		redisCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		err = h.rateLimiter.redis.Set(redisCtx, dataExportTicketKeyPrefix+hashToken(ticket), payload, time.Until(expiresAt)).Err()
		cancel()
		if err == nil {
			return ticket, expiresAt, nil
		}
		if h.mode == "production" {
			return "", time.Time{}, err
		}
	}
	if h.mode == "production" {
		return "", time.Time{}, errors.New("Redis is required for production data export tickets")
	}
	ticket := "m_" + randomHex(32)
	dataExportMemoryTickets.Store(hashToken(ticket), value)
	return ticket, expiresAt, nil
}

func (h *Handlers) consumeDataExportTicket(ctx context.Context, ticket string) (dataExportTicketValue, bool, error) {
	var value dataExportTicketValue
	switch {
	case strings.HasPrefix(ticket, "r_"):
		if h.rateLimiter == nil || h.rateLimiter.redis == nil {
			return value, false, errors.New("Redis data export ticket store is unavailable")
		}
		redisCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		raw, err := h.rateLimiter.redis.GetDel(redisCtx, dataExportTicketKeyPrefix+hashToken(ticket)).Bytes()
		cancel()
		if errors.Is(err, redis.Nil) {
			return value, false, nil
		}
		if err != nil {
			return value, false, err
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return value, false, err
		}
	case strings.HasPrefix(ticket, "m_") && h.mode != "production":
		raw, found := dataExportMemoryTickets.LoadAndDelete(hashToken(ticket))
		if !found {
			return value, false, nil
		}
		var ok bool
		value, ok = raw.(dataExportTicketValue)
		if !ok {
			return value, false, errors.New("invalid in-memory data export ticket")
		}
	default:
		return value, false, nil
	}
	if !value.ExpiresAt.After(time.Now()) {
		return dataExportTicketValue{}, false, nil
	}
	return value, true, nil
}

func (h *Handlers) dataExportAuthorizationFromTicket(ctx context.Context, value dataExportTicketValue) (deploymentTargetDataExportAuthorization, bool) {
	binding := value.Authorization
	now := time.Now()
	if !value.ExpiresAt.After(now) || !binding.Deadline.After(now) {
		return deploymentTargetDataExportAuthorization{}, false
	}

	db := h.db.WithContext(ctx)
	var user model.User
	if err := db.First(&user, "id = ? and disabled = ?", binding.UserID, false).Error; err != nil {
		return deploymentTargetDataExportAuthorization{}, false
	}
	if grantID, oauth := dataExportOAuthGrantID(binding.SubjectID); oauth {
		if !binding.AssertionRequired {
			return deploymentTargetDataExportAuthorization{}, false
		}
		var grant model.OAuthGrant
		if err := db.First(
			&grant,
			"id = ? and user_id = ? and application_id = ? and revoked_at is null",
			grantID,
			binding.UserID,
			lunaCLIApplicationID,
		).Error; err != nil {
			return deploymentTargetDataExportAuthorization{}, false
		}
		var application model.OAuthApplication
		if err := db.First(
			&application,
			"id = ? and client_id = ? and revoked_at is null",
			lunaCLIApplicationID,
			lunaCLIClientID,
		).Error; err != nil {
			return deploymentTargetDataExportAuthorization{}, false
		}
	} else {
		var session model.UserSession
		if err := db.First(
			&session,
			"id = ? and user_id = ? and expires_at > ?",
			binding.SubjectID,
			binding.UserID,
			now,
		).Error; err != nil {
			return deploymentTargetDataExportAuthorization{}, false
		}
	}

	if binding.AssertionRequired {
		var assertion model.StepUpAssertion
		if err := db.First(
			&assertion,
			"id = ? and user_id = ? and session_id = ? and purpose = ? and idle_expires_at > ? and absolute_expires_at > ?",
			binding.AssertionID,
			binding.UserID,
			binding.SubjectID,
			stepUpPurposeDataExport,
			now,
			now,
		).Error; err != nil ||
			!stepUpAssertionActive(assertion, now) ||
			assertion.AbsoluteExpiresAt.After(binding.AssertionAbsoluteDeadline) {
			return deploymentTargetDataExportAuthorization{}, false
		}
	} else if h.stepUpMFAEnabled() {
		return deploymentTargetDataExportAuthorization{}, false
	}

	var project model.Project
	if err := db.First(&project, "id = ?", value.ProjectID).Error; err != nil || !resourceCanMutateDuringDelete(project.DeleteStatus) {
		return deploymentTargetDataExportAuthorization{}, false
	}
	if !authz.IsPlatformAdmin(user.Role) {
		var member model.ProjectMember
		if err := db.First(&member, "project_id = ? and user_id = ?", value.ProjectID, user.ID).Error; err != nil ||
			!projectUserRoleAllowed(user, member.Role, []string{"owner", "admin"}) {
			return deploymentTargetDataExportAuthorization{}, false
		}
	}

	var app model.Application
	if err := db.First(&app, "id = ? and project_id = ?", value.ApplicationID, value.ProjectID).Error; err != nil ||
		!applicationCanMutate(app) {
		return deploymentTargetDataExportAuthorization{}, false
	}
	var target model.DeploymentTarget
	if err := db.First(
		&target,
		"id = ? and project_id = ? and application_id = ?",
		value.TargetID,
		value.ProjectID,
		value.ApplicationID,
	).Error; err != nil ||
		!resourceCanMutateDuringDelete(target.DeleteStatus) ||
		!target.DataRetentionEnabled {
		return deploymentTargetDataExportAuthorization{}, false
	}
	return deploymentTargetDataExportAuthorization{
		user: user, project: project, app: app, target: target, binding: binding,
	}, true
}
