package notificationapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"gorm.io/gorm"
)

// RegisterAITools registers all 23 AI domain facade tools owned by notificationapi
// (16 ops inbox & notification tools + 7 admin mail & global notification tools).
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	h.registerOpsInboxAITools(reg)
	h.registerOpsPersonalNotificationAITools(reg)
	h.registerOpsProjectNotificationAITools(reg)
	h.registerAdminNotificationAITools(reg)
}

type ruleChannelTargetPatchInput struct {
	Channel string   `json:"channel" desc:"目标 SMTP 渠道 ID 或名称"`
	To      []string `json:"to,omitempty" desc:"收件人邮箱列表"`
	Cc      []string `json:"cc,omitempty" desc:"抄送邮箱列表"`
	ClearCc *bool    `json:"clearCc,omitempty" desc:"显式清空该渠道抄送列表"`
}

func (h *Handler) aiActor(tc *aitool.ToolContext) (model.User, error) {
	if tc == nil {
		return model.User{}, aitool.NewError(http.StatusUnauthorized, "auth.unauthorized", "authentication required")
	}
	if strings.TrimSpace(tc.User.ID) != "" {
		return tc.User, nil
	}
	if tc.Gin != nil {
		if user, ok := h.currentUser(tc.Gin); ok && strings.TrimSpace(user.ID) != "" {
			return user, nil
		}
	}
	return model.User{}, aitool.NewError(http.StatusUnauthorized, "auth.unauthorized", "authentication required")
}

func (h *Handler) aiRequireAdmin(tc *aitool.ToolContext) (model.User, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return model.User{}, err
	}
	if !authz.IsPlatformAdmin(user.Role) {
		return model.User{}, aitool.Forbidden("auth.forbidden", "platform administrator permission is required")
	}
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		return model.User{}, notifProjectScopeMismatch("platform administration tools require an unbound global conversation")
	}
	return user, nil
}

func (h *Handler) aiDB(tc *aitool.ToolContext) *gorm.DB {
	if tc != nil && tc.DB != nil {
		return tc.DB
	}
	if tc != nil && tc.Gin != nil {
		return h.dbFor(tc.Gin)
	}
	return h.dbWithContext(tc.Context())
}

func notifProjectScopeMismatch(detail string) *aitool.ToolError {
	err := aitool.Forbidden("ai.project_scope_mismatch", detail)
	err.Hint = "Switch to an unbound global conversation when operating across projects or global platform resources."
	return err
}

func (h *Handler) authorizeProjectForAI(tc *aitool.ToolContext, projectRef string, action authz.Action) (model.User, model.Project, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return model.User{}, model.Project{}, err
	}
	db := h.aiDB(tc)
	projectRef = strings.TrimSpace(projectRef)
	if projectRef == "" {
		return model.User{}, model.Project{}, aitool.BadRequest("project.required", "project is required")
	}

	var project model.Project
	if convID := strings.TrimSpace(tc.ConversationProjectID); convID != "" {
		if err := db.First(&project, "id = ? AND deleted_at IS NULL", convID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.User{}, model.Project{}, notifProjectScopeMismatch("bound conversation project no longer exists")
			}
			return model.User{}, model.Project{}, aitool.NewError(http.StatusServiceUnavailable, "project.lookup_unavailable", "project storage is unavailable")
		}
		if projectRef != project.ID && projectRef != project.Identifier {
			return model.User{}, model.Project{}, notifProjectScopeMismatch("requested project does not match the bound conversation project")
		}
	} else {
		resolved, resolveErr := aitool.ResolveProject(db, projectRef)
		if resolveErr != nil {
			return model.User{}, model.Project{}, resolveErr
		}
		project = resolved
	}

	if authz.IsPlatformAdmin(user.Role) {
		return user, project, nil
	}
	allowed, roleErr := h.host.ProjectRoleActionAllowed(tc.Context(), user, project.ID, action)
	if roleErr != nil {
		return model.User{}, model.Project{}, aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", "project authorization service is temporarily unavailable")
	}
	if !allowed {
		return model.User{}, model.Project{}, aitool.Forbidden("auth.forbidden", "you do not have permission to perform this project notification operation")
	}
	return user, project, nil
}

func ensureProjectMutableForAI(project model.Project) error {
	status := strings.TrimSpace(project.DeleteStatus)
	if status == "" || status == "active" || status == "delete_failed" {
		return nil
	}
	return aitool.Conflict("project.delete_in_progress", "project space is currently being deleted")
}

func mapInboxToolError(err error) error {
	if te, ok := aitool.AsToolError(err); ok && te != nil {
		return te
	}
	switch {
	case errors.Is(err, inbox.ErrInvalidInput), errors.Is(err, inbox.ErrInvalidDeepLink):
		return aitool.BadRequest("inbox.request_invalid", "inbox request is invalid")
	case errors.Is(err, inbox.ErrNotFound):
		return aitool.NotFound("inbox.not_found", "inbox resource was not found")
	case errors.Is(err, inbox.ErrSelfHealSessionRequired):
		return aitool.Forbidden("inbox.self_heal_requires_ui_confirmation", "runtime self-healing proposals must be confirmed in the Inbox UI")
	case errors.Is(err, inbox.ErrSelfHealForbidden):
		return aitool.Forbidden("inbox.self_heal_forbidden", "runtime self-heal action is forbidden")
	case errors.Is(err, inbox.ErrSelfHealStateDrifted):
		return aitool.Conflict("inbox.self_heal_state_drifted", "runtime self-heal target state has drifted")
	case errors.Is(err, inbox.ErrActionRequestStale):
		return aitool.Conflict("inbox.action_request_stale", "inbox action request version is stale; refresh with ops_list_inbox")
	case errors.Is(err, inbox.ErrActionRequestExpired):
		return aitool.NewError(http.StatusGone, "inbox.action_request_expired", "inbox action request has expired")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferInvalid):
		return aitool.BadRequest("inbox.billing_owner_transfer_invalid", "billing owner transfer input is invalid")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferForbidden):
		return aitool.Forbidden("inbox.billing_owner_transfer_forbidden", "billing owner transfer is forbidden")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferConflict):
		return aitool.Conflict("inbox.billing_owner_transfer_conflict", "billing owner transfer conflicts with current state")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferStale):
		return aitool.Conflict("inbox.billing_owner_transfer_stale", "inbox action request version is stale; refresh with ops_list_inbox")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferNotFound):
		return aitool.NotFound("inbox.billing_owner_transfer_not_found", "billing owner transfer request was not found")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferExpired):
		return aitool.NewError(http.StatusGone, "inbox.billing_owner_transfer_expired", "inbox action request has expired")
	case errors.Is(err, ErrInboxDecisionUnavailable):
		return aitool.NewError(http.StatusServiceUnavailable, "inbox.decision_unavailable", "inbox decision handler is unavailable")
	default:
		return aitool.NewError(http.StatusInternalServerError, "inbox.operation_failed", err.Error())
	}
}

func isDeliveryRetryableAt(delivery model.NotificationDelivery, now time.Time) bool {
	switch strings.TrimSpace(delivery.Status) {
	case "failed", "enqueue_failed":
		return true
	case "sending":
		staleBefore := now.Add(-tasks.PolicyForType(tasks.TypeNotificationDeliver).Timeout)
		return delivery.StartedAt == nil || !delivery.StartedAt.After(staleBefore)
	default:
		return false
	}
}

func ensureDeliveryRetryable(delivery model.NotificationDelivery, now time.Time) error {
	if isDeliveryRetryableAt(delivery, now) {
		return nil
	}
	return aitool.NewErrorWithHint(
		http.StatusConflict,
		"notification.delivery_not_retryable",
		fmt.Sprintf("notification delivery %s has status %q and cannot be retried", delivery.ID, delivery.Status),
		"Only failed, enqueue_failed, or timed-out sending deliveries can be retried.",
	)
}

func isSensitiveHeaderKey(headerKey string) bool {
	lower := strings.ToLower(strings.TrimSpace(headerKey))
	switch lower {
	case "authorization", "proxy-authorization", "cookie", "set-cookie":
		return true
	}
	for _, marker := range []string{"token", "secret", "key", "password", "signature", "credential"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// validateUpsertConfigNoPlainSecrets enforces R11: prevents embedding raw passwords,
// URL userinfo credentials, or literal secret headers inside upsertConfig.
func validateUpsertConfigNoPlainSecrets(upsertConfig map[string]any) error {
	if len(upsertConfig) == 0 {
		return nil
	}
	for k, v := range upsertConfig {
		lower := strings.ToLower(strings.TrimSpace(k))
		if lower == "password" || lower == "passwordref" {
			return aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"notification.config_secret_forbidden",
				fmt.Sprintf("field %q is not allowed in upsertConfig", k),
				"Pass SMTP passwords or secret tokens via upsertSecrets instead of upsertConfig.",
			)
		}
		if lower == "url" {
			if rawURL, ok := v.(string); ok && strings.TrimSpace(rawURL) != "" && !strings.Contains(rawURL, "{{") {
				if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && parsed.User != nil {
					return aitool.NewErrorWithHint(
						http.StatusBadRequest,
						"notification.config_secret_forbidden",
						"embedding credentials in webhook URL userinfo is forbidden",
						"Store credentials in upsertSecrets and reference them with {{.Secrets.<Key>}} placeholders.",
					)
				}
			}
		}
		if lower == "headers" && v != nil {
			headerMap := map[string]string{}
			switch typed := v.(type) {
			case map[string]any:
				for hk, hv := range typed {
					headerMap[hk] = fmt.Sprint(hv)
				}
			case map[string]string:
				maps.Copy(headerMap, typed)
			}
			for hk, hv := range headerMap {
				trimmedVal := strings.TrimSpace(hv)
				if trimmedVal == "" {
					continue
				}
				if isSensitiveHeaderKey(hk) && !strings.Contains(trimmedVal, "{{.Secrets.") {
					return aitool.NewErrorWithHint(
						http.StatusBadRequest,
						"notification.config_secret_forbidden",
						fmt.Sprintf("sensitive header %q must use a {{.Secrets.<Key>}} template placeholder instead of a literal secret", hk),
						"Example: set upsertConfig.headers = {\"Authorization\": \"Bearer {{.Secrets.BearerToken}}\"} and pass BearerToken in upsertSecrets.",
					)
				}
			}
		}
	}
	return nil
}

func mergeChannelConfigPatch(existingConfigJSON string, upsertConfig map[string]any, removeConfigKeys []string, clearSMTPPasswordRef bool) (string, error) {
	if err := validateUpsertConfigNoPlainSecrets(upsertConfig); err != nil {
		return "", err
	}
	current := map[string]any{}
	if strings.TrimSpace(existingConfigJSON) != "" {
		if err := json.Unmarshal([]byte(existingConfigJSON), &current); err != nil {
			current = map[string]any{}
		}
	}
	for _, rmKey := range removeConfigKeys {
		rmKey = strings.TrimSpace(rmKey)
		if rmKey != "" {
			delete(current, rmKey)
		}
	}
	for k, v := range upsertConfig {
		trimmedKey := strings.TrimSpace(k)
		if trimmedKey == "" || v == nil {
			continue
		}
		if strings.EqualFold(trimmedKey, "headers") {
			existingHeaders := map[string]any{}
			if prev, ok := current["headers"].(map[string]any); ok {
				maps.Copy(existingHeaders, prev)
			}
			if nextHeaders, ok := v.(map[string]any); ok {
				for hk, hv := range nextHeaders {
					if hv == nil {
						delete(existingHeaders, hk)
					} else {
						existingHeaders[hk] = hv
					}
				}
				current["headers"] = existingHeaders
				continue
			}
		}
		current[trimmedKey] = v
	}
	if clearSMTPPasswordRef {
		delete(current, "passwordRef")
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return "", aitool.BadRequest("notification.config_invalid", err.Error())
	}
	return string(encoded), nil
}

func normalizeSecretMutations(adapterKind string, upsertSecrets map[string]string, clearSecretKeys []string) (map[string]string, []string, []string, error) {
	toSet := map[string]string{}
	for rawKey, rawVal := range upsertSecrets {
		key := strings.TrimSpace(rawKey)
		val := strings.TrimSpace(rawVal)
		if key == "" {
			continue
		}
		if adapterKind == notification.AdapterKindSMTP && strings.EqualFold(key, "password") {
			key = "password"
		}
		if val == "" {
			return nil, nil, nil, aitool.BadRequest("notification.secret_invalid", fmt.Sprintf("upsertSecrets[%q] cannot be empty; use clearSecretKeys to clear a secret", rawKey))
		}
		toSet[key] = val
	}

	var toClear []string
	seenClear := map[string]struct{}{}
	for _, rawKey := range clearSecretKeys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		if adapterKind == notification.AdapterKindSMTP && strings.EqualFold(key, "password") {
			key = "password"
		}
		if _, conflict := toSet[key]; conflict {
			return nil, nil, nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("secret key %q cannot appear in both upsertSecrets and clearSecretKeys", key))
		}
		if _, dup := seenClear[key]; !dup {
			seenClear[key] = struct{}{}
			toClear = append(toClear, key)
		}
	}

	var sensitiveKeys []string
	for k := range toSet {
		sensitiveKeys = append(sensitiveKeys, k)
	}
	sensitiveKeys = append(sensitiveKeys, toClear...)
	sort.Strings(sensitiveKeys)
	return toSet, toClear, sensitiveKeys, nil
}

func applyChannelTargetPatches(
	existingTargetsJSON string,
	upsert []ruleChannelTargetPatchInput,
	remove []string,
	resolveChannelID func(ref string) (string, error),
) (json.RawMessage, error) {
	targets, err := notification.DecodeChannelTargets([]byte(firstNonEmpty(existingTargetsJSON, "{}")))
	if err != nil || targets == nil {
		targets = map[string]notification.ChannelTargetOverride{}
	}
	for _, rmRef := range remove {
		chID, resolveErr := resolveChannelID(rmRef)
		if resolveErr != nil {
			// Also allow removing a stale raw ID directly
			delete(targets, strings.TrimSpace(rmRef))
			continue
		}
		delete(targets, chID)
	}
	for _, item := range upsert {
		chID, resolveErr := resolveChannelID(item.Channel)
		if resolveErr != nil {
			return nil, resolveErr
		}
		prev := targets[chID]
		if item.To != nil {
			prev.To = normalizedNotificationRuleValues(item.To)
		}
		if item.ClearCc != nil && *item.ClearCc {
			prev.Cc = nil
		} else if item.Cc != nil {
			prev.Cc = normalizedNotificationRuleValues(item.Cc)
		}
		targets[chID] = prev
	}
	encoded, encErr := notification.EncodeChannelTargets(targets)
	if encErr != nil {
		return nil, aitool.BadRequest("notification.rule_targets_invalid", encErr.Error())
	}
	return json.RawMessage(encoded), nil
}

func applyStringSetPatch(existing []string, add []string, remove []string) []string {
	removeSet := make(map[string]struct{}, len(remove))
	for _, item := range remove {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			removeSet[trimmed] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	var out []string
	for _, item := range existing {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, drop := removeSet[trimmed]; drop {
			continue
		}
		if _, dup := seen[trimmed]; !dup {
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	for _, item := range add {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, drop := removeSet[trimmed]; drop {
			continue
		}
		if _, dup := seen[trimmed]; !dup {
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	return out
}

func resolveRuleFilterTargetInProject(db *gorm.DB, projectID, ref string) (model.DeploymentTarget, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.DeploymentTarget{}, aitool.BadRequest("deployment_target.required", "target reference cannot be empty")
	}
	if appRef, stageOrName, hasColon := strings.Cut(ref, ":"); hasColon {
		app, err := aitool.ResolveApplication(db, projectID, appRef)
		if err != nil {
			return model.DeploymentTarget{}, err
		}
		return aitool.ResolveDeploymentTarget(db, projectID, app.ID, stageOrName)
	}

	var byID model.DeploymentTarget
	if err := db.Where("project_id = ? AND id = ? AND deleted_at IS NULL", projectID, ref).First(&byID).Error; err == nil {
		return byID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.DeploymentTarget{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var matches []model.DeploymentTarget
	if err := db.Where("project_id = ? AND (stage = ? OR name = ?) AND deleted_at IS NULL", projectID, ref, ref).
		Order("created_at asc, id asc").
		Find(&matches).Error; err != nil {
		return model.DeploymentTarget{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		candidates := make([]aitool.AmbiguousCandidate, len(matches))
		for i, item := range matches {
			candidates[i] = aitool.AmbiguousCandidate{ID: item.ID, Name: item.Name, Stage: item.Stage}
		}
		err := aitool.AmbiguousResource("deployment_target", ref, candidates)
		err.Hint = "Specify the exact deployment target ID (dplt_xxx) or <appIdentifier>:<stage>."
		return model.DeploymentTarget{}, err
	}
	return model.DeploymentTarget{}, aitool.NotFound("deployment_target.not_found", fmt.Sprintf("deployment target %q was not found in this project", ref))
}

func parseOptionalTestRecipient(raw *string, fallbackEmail string) (string, error) {
	if raw != nil && strings.TrimSpace(*raw) != "" {
		addr, err := mail.ParseAddress(strings.TrimSpace(*raw))
		if err != nil || !strings.EqualFold(addr.Address, strings.TrimSpace(*raw)) {
			return "", aitool.BadRequest("notification.test_recipient_invalid", "testRecipient must be a valid email address")
		}
		return addr.Address, nil
	}
	if trimmed := strings.TrimSpace(fallbackEmail); trimmed != "" {
		if addr, err := mail.ParseAddress(trimmed); err == nil {
			return addr.Address, nil
		}
	}
	return "", nil
}

func notifBoolOrDefault(ptr *bool, def bool) bool {
	if ptr == nil {
		return def
	}
	return *ptr
}

func notifStringOrDefault(ptr *string, def string) string {
	if ptr == nil {
		return def
	}
	if v := strings.TrimSpace(*ptr); v != "" {
		return v
	}
	return def
}
