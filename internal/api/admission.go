package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	errOIDCDisabled             = errors.New("OIDC login is disabled")
	errOIDCEmailRequired        = errors.New("OIDC login requires a non-empty verified email")
	errOIDCGroupDenied          = errors.New("OIDC groups are not allowed")
	errOIDCAdmissionDenied      = errors.New("OIDC admission denied")
	errOIDCInvalidIdentity      = errors.New("OIDC identity is invalid")
	errOIDCRegistrationDisabled = errors.New("OIDC registration is disabled")
)

const defaultAdmissionPolicyID = "auth_admission_policy_default"

func (h *Handlers) GetAuthAdmissionPolicy(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	policy, err := h.ensureAdmissionPolicy(ctx.Request.Context())
	if err != nil {
		writeAdmissionPolicyUnavailable(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, admissionPolicyResponse(policy))
}

func (h *Handlers) UpdateAuthAdmissionPolicy(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}

	var input authAdmissionPolicyInput
	if !bindJSON(ctx, &input) {
		return
	}

	policy, err := h.ensureAdmissionPolicy(ctx.Request.Context())
	if err != nil {
		writeAdmissionPolicyUnavailable(ctx, err)
		return
	}
	policy.AllowLocalLogin = input.AllowLocalLogin
	policy.AllowOIDCLogin = input.AllowOIDCLogin
	policy.RequireVerifiedOIDCEmail = input.RequireVerifiedOIDCEmail
	policy.AllowedEmailDomains = strings.Join(normalizeList(input.AllowedEmailDomains, false), ",")
	policy.AllowedOIDCGroups = strings.Join(normalizeList(input.AllowedOIDCGroups, true), ",")
	policy.InvitedEmails = strings.Join(normalizeList(input.InvitedEmails, false), ",")
	policy.DefaultRole = normalizeUserRole(input.DefaultRole)

	if err := h.dbFor(ctx).Save(&policy).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, admissionPolicyResponse(policy))
}

func (h *Handlers) evaluateAdmission(claims oidcIdentityClaims, contexts ...context.Context) error {
	policy, err := h.ensureAdmissionPolicy(firstContext(contexts))
	if err != nil {
		return err
	}
	if !policy.AllowOIDCLogin {
		return errOIDCDisabled
	}

	email, ok := oidcAdmissionEmail(claims, policy.RequireVerifiedOIDCEmail)
	if !ok {
		return errOIDCEmailRequired
	}

	allowedGroups := splitCSV(policy.AllowedOIDCGroups)
	if len(allowedGroups) > 0 && !hasIntersection(normalizeList(claims.Groups, true), allowedGroups) {
		return errOIDCGroupDenied
	}

	if len(allowedGroups) == 0 && len(splitCSV(policy.AllowedEmailDomains)) == 0 && len(splitCSV(policy.InvitedEmails)) == 0 {
		return nil
	}

	if containsString(splitCSV(policy.InvitedEmails), email) {
		return nil
	}
	if containsString(splitCSV(policy.AllowedEmailDomains), emailDomain(email)) {
		return nil
	}
	if len(allowedGroups) > 0 && hasIntersection(normalizeList(claims.Groups, true), allowedGroups) {
		return nil
	}

	return errOIDCAdmissionDenied
}

func (h *Handlers) ensureAdmissionPolicy(contexts ...context.Context) (model.AuthAdmissionPolicy, error) {
	var policy model.AuthAdmissionPolicy
	err := h.dbWithContext(firstContext(contexts)).First(&policy, "id = ?", defaultAdmissionPolicyID).Error
	if err == nil {
		return policy, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AuthAdmissionPolicy{}, fmt.Errorf("load authentication admission policy: %w", err)
	}

	policy = model.AuthAdmissionPolicy{
		ID:                       defaultAdmissionPolicyID,
		AllowLocalLogin:          true,
		AllowOIDCLogin:           true,
		RequireVerifiedOIDCEmail: true,
		DefaultRole:              authz.PlatformRoleUser,
	}
	if err := h.dbWithContext(firstContext(contexts)).Create(&policy).Error; err != nil {
		var existing model.AuthAdmissionPolicy
		if reloadErr := h.dbWithContext(firstContext(contexts)).First(&existing, "id = ?", defaultAdmissionPolicyID).Error; reloadErr == nil {
			return existing, nil
		}
		return model.AuthAdmissionPolicy{}, fmt.Errorf("initialize authentication admission policy: %w", err)
	}
	return policy, nil
}

func writeAdmissionPolicyUnavailable(ctx *gin.Context, err error) {
	requestID := requestID(ctx)
	telemetry.Logger().ErrorContext(ctx.Request.Context(), "authentication admission policy unavailable",
		slog.String("event.name", "auth.admission_policy.unavailable"),
		slog.String("request_id", requestID),
		slog.String("error.type", telemetry.ErrorType(err)),
	)
	writeErrorCode(
		ctx,
		http.StatusServiceUnavailable,
		"auth.admission_policy_unavailable",
		"authentication admission policy is temporarily unavailable",
	)
}

func admissionPolicyResponse(policy model.AuthAdmissionPolicy) gin.H {
	return gin.H{
		"id":                       policy.ID,
		"allowLocalLogin":          policy.AllowLocalLogin,
		"allowOidcLogin":           policy.AllowOIDCLogin,
		"requireVerifiedOidcEmail": policy.RequireVerifiedOIDCEmail,
		"allowedEmailDomains":      jsonList(splitCSV(policy.AllowedEmailDomains)),
		"allowedOidcGroups":        jsonList(splitCSV(policy.AllowedOIDCGroups)),
		"invitedEmails":            jsonList(splitCSV(policy.InvitedEmails)),
		"defaultRole":              policy.DefaultRole,
	}
}

func (h *Handlers) audit(userID, action, resource string, success bool, message string) {
	h.auditWithContext(userID, action, resource, success, message, context.Background())
}

func (h *Handlers) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	entry := map[string]any{
		"id":         id.New("aud"),
		"user_id":    strings.TrimSpace(userID),
		"action":     action,
		"resource":   resource,
		"success":    success,
		"message":    message,
		"created_at": time.Now(),
	}
	if err := h.dbWithContext(ctx).Model(&model.AuditLog{}).Create(entry).Error; err != nil {
		telemetry.Logger().ErrorContext(ctx, "audit write failed",
			slog.String("event.name", "audit.write.failed"),
			slog.String("audit.action", action),
			slog.String("resource.type", resource),
			slog.Bool("outcome.success", success),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
	}
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return normalizeList(strings.Split(value, ","), false)
}

func jsonList(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func normalizeList(values []string, preserveCase bool) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if !preserveCase {
			normalized = strings.ToLower(normalized)
		}
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		result = append(result, normalized)
	}
	return result
}

func hasIntersection(left, right []string) bool {
	set := map[string]bool{}
	for _, item := range right {
		set[item] = true
	}
	for _, item := range left {
		if set[item] {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func emailDomain(email string) string {
	_, domain, found := strings.Cut(email, "@")
	if !found {
		return ""
	}
	return strings.ToLower(domain)
}

func oidcAdmissionEmail(claims oidcIdentityClaims, requireVerified bool) (string, bool) {
	email := normalizeEmail(claims.Email)
	if email == "" {
		return "", false
	}
	if requireVerified && !claims.EmailVerified {
		return "", false
	}
	return email, true
}

type authAdmissionPolicyInput struct {
	AllowLocalLogin          bool     `json:"allowLocalLogin"`
	AllowOIDCLogin           bool     `json:"allowOidcLogin"`
	RequireVerifiedOIDCEmail bool     `json:"requireVerifiedOidcEmail"`
	AllowedEmailDomains      []string `json:"allowedEmailDomains"`
	AllowedOIDCGroups        []string `json:"allowedOidcGroups"`
	InvitedEmails            []string `json:"invitedEmails"`
	DefaultRole              string   `json:"defaultRole"`
}
