package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
)

const (
	maxDeliveryDiagnosisRawTailWindowBytes = 32 * 1024
	maxDeliveryDiagnosisLogTailBytes       = 6 * 1024
	deliveryDiagnosisAgentTimeout          = 45 * time.Second
	deliveryDiagnosisStaleRunningAge       = 3 * time.Minute
	deliveryDiagnosisReconcileBatchSize    = 50
)

type aiDeliveryDiagnosisEnqueuer interface {
	EnqueueAIDeliveryDiagnosis(ctx context.Context, payload tasks.AIDeliveryDiagnosisPayload) (*asynq.TaskInfo, error)
}

type deliveryDiagnosisLLMOutput struct {
	RootCauseSummary string                  `json:"rootCauseSummary"`
	Confidence       string                  `json:"confidence"`
	Suggestions      []string                `json:"suggestions"`
	ProposedAction   *model.AIProposedAction `json:"proposedAction"`
}

func isDiagnosableBuildFailure(status, failureCode string) bool {
	if status != "failed" && status != "timeout" {
		return false
	}
	code := strings.TrimSpace(failureCode)
	return code != model.BuildFailureCanceledByUser && code != model.BuildFailureSuperseded
}

func isDiagnosableReleaseFailure(status, failureCode string) bool {
	if status != "failed" {
		return false
	}
	code := strings.TrimSpace(failureCode)
	return code != model.ReleaseFailureCanceledByUser && code != model.ReleaseFailureSuperseded
}

func isAutomatedBuildTrigger(triggerType string) bool {
	switch strings.ToLower(strings.TrimSpace(triggerType)) {
	case "webhook", "push", "tag":
		return true
	default:
		return false
	}
}

func preferredBuildBillingUserID(run model.BuildRun) string {
	if isAutomatedBuildTrigger(run.TriggerType) {
		return ""
	}
	return strings.TrimSpace(run.CreatedBy)
}

func preferredReleaseBillingUserID(db *gorm.DB, release model.Release) string {
	createdBy := strings.TrimSpace(release.CreatedBy)
	if createdBy == "" {
		return ""
	}
	isAutoDeploy := strings.HasPrefix(strings.TrimSpace(release.IdempotencyKey), "auto-deploy:") ||
		strings.TrimSpace(release.Message) == "auto deploy from build"
	if isAutoDeploy && strings.TrimSpace(release.BuildRunID) != "" && db != nil {
		var run model.BuildRun
		if err := db.Select("id", "trigger_type").First(&run, "id = ? AND project_id = ?", release.BuildRunID, release.ProjectID).Error; err == nil {
			if isAutomatedBuildTrigger(run.TriggerType) {
				return ""
			}
		}
	}
	return createdBy
}

func (r *Runner) onBuildFailureTerminal(ctx context.Context, run model.BuildRun, message string) {
	if isDiagnosableBuildFailure(run.Status, run.FailureCode) {
		var db *gorm.DB
		if r.db != nil {
			db = r.db.WithContext(ctx)
		}
		if r.tryScheduleDeliveryDiagnosis(
			ctx,
			run.ProjectID,
			run.ApplicationID,
			run.DeploymentTargetID,
			model.AIDeliverySubjectBuildRun,
			run.ID,
			run.FailureCode,
			preferredBuildBillingUserID(run),
			db,
		) {
			r.publishBuildFailureInboxOnly(ctx, run, message, nil)
			return
		}
	}
	r.emitBuildFailedWithAI(ctx, run, message, nil)
}

func (r *Runner) onReleaseFailureTerminal(ctx context.Context, release model.Release, message string) {
	if isDiagnosableReleaseFailure(release.Status, release.FailureCode) {
		var db *gorm.DB
		if r.db != nil {
			db = r.db.WithContext(ctx)
		}
		if r.tryScheduleDeliveryDiagnosis(
			ctx,
			release.ProjectID,
			release.ApplicationID,
			release.DeploymentTargetID,
			model.AIDeliverySubjectRelease,
			release.ID,
			release.FailureCode,
			preferredReleaseBillingUserID(db, release),
			db,
		) {
			r.publishReleaseFailureInboxOnly(ctx, release, message, nil)
			return
		}
	}
	r.emitReleaseEventWithAI(ctx, release, "failed", message, nil)
}

func (r *Runner) tryScheduleDeliveryDiagnosis(
	ctx context.Context,
	projectID string,
	applicationID string,
	deploymentTargetID string,
	subjectType string,
	subjectID string,
	failureCode string,
	preferredUserID string,
	db *gorm.DB,
) (scheduled bool) {
	if r.aiAgent == nil || !r.aiAgentAvailable || r.aiDeliveryDiagnosisEnqueuer == nil || db == nil {
		return false
	}
	projectID = strings.TrimSpace(projectID)
	subjectID = strings.TrimSpace(subjectID)
	if projectID == "" || subjectID == "" {
		return false
	}

	ctx, end := telemetry.StartOperation(
		ctx,
		"ai_automation",
		"schedule_delivery_diagnosis",
		attribute.String("resource.type", subjectType),
		attribute.String("resource.id", subjectID),
	)
	var opErr error
	defer func() { end(opErr) }()
	db = db.WithContext(ctx)

	enabled, accessMode := r.readPlatformAISettings(db)
	if !enabled {
		return false
	}

	var cfg model.ProjectAIAutomationConfig
	if err := db.First(&cfg, "project_id = ?", projectID).Error; err != nil || !cfg.DeliveryDiagnosisEnabled {
		return false
	}

	billedUser, ok := resolveProjectAIBillingUser(ctx, db, projectID, preferredUserID, cfg.DeliveryBillingUserID, accessMode)
	if !ok {
		return false
	}

	now := time.Now().UTC()
	diagID := id.New("aidg")
	reserved := false
	txErr := db.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector != nil && tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "ai_delivery_diag:"+projectID).Error; err != nil {
				return err
			}
		}
		var existing int64
		if err := tx.Model(&model.AIDeliveryDiagnosis{}).
			Where("subject_type = ? AND subject_id = ?", subjectType, subjectID).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return nil
		}
		if cfg.DeliveryCooldownSeconds > 0 {
			cutoff := now.Add(-time.Duration(cfg.DeliveryCooldownSeconds) * time.Second)
			var recentCount int64
			if err := tx.Model(&model.AIDeliveryDiagnosis{}).
				Where("project_id = ? AND created_at > ?", projectID, cutoff).
				Count(&recentCount).Error; err != nil {
				return err
			}
			if recentCount > 0 {
				return nil
			}
		}
		dailyLimit := cfg.DeliveryDailyLimit
		if dailyLimit <= 0 {
			dailyLimit = model.DefaultDeliveryDiagnosisDailyLimit
		}
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		var todayCount int64
		if err := tx.Model(&model.AIDeliveryDiagnosis{}).
			Where("project_id = ? AND created_at >= ?", projectID, dayStart).
			Count(&todayCount).Error; err != nil {
			return err
		}
		if todayCount >= int64(dailyLimit) {
			return nil
		}
		row := model.AIDeliveryDiagnosis{
			ID:                 diagID,
			ProjectID:          projectID,
			ApplicationID:      strings.TrimSpace(applicationID),
			DeploymentTargetID: strings.TrimSpace(deploymentTargetID),
			SubjectType:        subjectType,
			SubjectID:          subjectID,
			BilledUserID:       billedUser.ID,
			Status:             model.AIDiagnosisStatusRunning,
			FailureCode:        strings.TrimSpace(failureCode),
			Confidence:         "medium",
			SuggestionsJSON:    "[]",
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		reserved = true
		return nil
	})
	if txErr != nil {
		opErr = telemetry.WrapError("ai.schedule_failed", "", "ai_automation.schedule_delivery_diagnosis", txErr)
		telemetry.LogWarn(ctx, "failed to reserve delivery AI diagnosis", "worker.ai_delivery_diagnosis.reserve_failed", "ai_automation.schedule_delivery_diagnosis", "ai.schedule_failed", opErr,
			slog.String("resource.type", subjectType),
			slog.String("resource.id", subjectID),
		)
		return false
	}
	if !reserved {
		return false
	}

	_, enqErr := r.aiDeliveryDiagnosisEnqueuer.EnqueueAIDeliveryDiagnosis(ctx, tasks.AIDeliveryDiagnosisPayload{
		DiagnosisID: diagID,
		ProjectID:   projectID,
		SubjectType: subjectType,
		SubjectID:   subjectID,
	})
	if enqErr != nil {
		opErr = telemetry.WrapError("ai.enqueue_failed", "", "ai_automation.schedule_delivery_diagnosis", enqErr)
		telemetry.LogWarn(ctx, "failed to enqueue delivery AI diagnosis", "worker.ai_delivery_diagnosis.enqueue_failed", "ai_automation.schedule_delivery_diagnosis", "ai.enqueue_failed", opErr,
			slog.String("resource.type", subjectType),
			slog.String("resource.id", subjectID),
		)
		finished := time.Now().UTC()
		// Synchronous caller will emit the fallback alert immediately, so mark notification_emitted_at.
		_ = db.Model(&model.AIDeliveryDiagnosis{}).Where("id = ?", diagID).Updates(map[string]any{
			"status":                  model.AIDiagnosisStatusFailed,
			"error_code":              "ai.enqueue_failed",
			"notification_emitted_at": &finished,
			"finished_at":             &finished,
			"updated_at":              finished,
		}).Error
		return false
	}
	telemetry.Logger().InfoContext(ctx, "delivery failure AI diagnosis scheduled",
		slog.String("event.name", "worker.ai_delivery_diagnosis.scheduled"),
		slog.String("operation", "ai_automation.schedule_delivery_diagnosis"),
		slog.String("outcome", "succeeded"),
		slog.String("resource.type", subjectType),
		slog.String("resource.id", subjectID),
	)
	return true
}

func (r *Runner) readPlatformAISettings(db *gorm.DB) (enabled bool, accessMode string) {
	accessMode = "all_authenticated"
	var rows []model.AppConfig
	if err := db.Where("key IN ?", []string{"ai.assistant.enabled", "ai.access.mode"}).Find(&rows).Error; err != nil {
		return false, accessMode
	}
	for _, row := range rows {
		switch row.Key {
		case "ai.assistant.enabled":
			enabled = strings.EqualFold(strings.TrimSpace(row.Value), "true")
		case "ai.access.mode":
			if trimmed := strings.TrimSpace(row.Value); trimmed != "" {
				accessMode = trimmed
			}
		}
	}
	return enabled, accessMode
}

func userAuthorizedForProjectAIBilling(db *gorm.DB, projectID string, user model.User, accessMode string) bool {
	if user.Disabled {
		return false
	}
	if strings.EqualFold(accessMode, "admins") && !authz.IsPlatformAdmin(user.Role) {
		return false
	}
	if !authz.IsPlatformAdmin(user.Role) {
		var memberCount int64
		if err := db.Model(&model.ProjectMember{}).
			Where("project_id = ? AND user_id = ?", projectID, user.ID).
			Count(&memberCount).Error; err != nil || memberCount == 0 {
			return false
		}
	}
	return true
}

func resolveProjectAIBillingUser(
	ctx context.Context,
	db *gorm.DB,
	projectID string,
	preferredUserID string,
	fallbackUserID string,
	accessMode string,
) (model.User, bool) {
	candidates := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(preferredUserID); trimmed != "" && trimmed != "system" {
		candidates = append(candidates, trimmed)
	}
	if trimmed := strings.TrimSpace(fallbackUserID); trimmed != "" && (len(candidates) == 0 || candidates[0] != trimmed) {
		candidates = append(candidates, trimmed)
	}
	billingSvc := billing.Service{DB: db}
	for _, uid := range candidates {
		var user model.User
		if err := db.First(&user, "id = ? AND disabled = ?", uid, false).Error; err != nil {
			continue
		}
		if !userAuthorizedForProjectAIBilling(db, projectID, user, accessMode) {
			continue
		}
		if err := billingSvc.EnsureAIAvailableBalance(ctx, user.ID); err != nil {
			continue
		}
		return user, true
	}
	return model.User{}, false
}

func (r *Runner) handleAIDeliveryDiagnosis(ctx context.Context, task *asynq.Task) (retErr error) {
	var payload tasks.AIDeliveryDiagnosisPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("%w: invalid AI delivery diagnosis payload: %v", asynq.SkipRetry, err)
	}

	ctx, end := telemetry.StartOperation(
		ctx,
		"ai_automation",
		"execute_delivery_diagnosis",
		attribute.String("resource.type", payload.SubjectType),
		attribute.String("resource.id", payload.SubjectID),
	)
	defer func() { end(retErr) }()

	db := r.db.WithContext(ctx)
	var diag model.AIDeliveryDiagnosis
	if err := db.First(&diag, "id = ? AND project_id = ?", payload.DiagnosisID, payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		retErr = telemetry.WrapError("ai.storage_unavailable", "", "ai_automation.execute_delivery_diagnosis", err)
		telemetry.LogError(ctx, "failed to load AI delivery diagnosis row", "worker.ai_delivery_diagnosis.load_failed", "ai_automation.execute_delivery_diagnosis", "ai.storage_unavailable", retErr,
			slog.String("resource.type", payload.SubjectType),
			slog.String("resource.id", payload.SubjectID),
		)
		return retErr
	}

	var aiNotificationCtx *notification.AIDiagnosisContext
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("panic in handleAIDeliveryDiagnosis: %v", recovered)
			retErr = telemetry.WrapError("ai.internal_error", "", "ai_automation.execute_delivery_diagnosis", panicErr)
			telemetry.LogError(ctx, "panic during delivery AI diagnosis", "worker.ai_delivery_diagnosis.panic", "ai_automation.execute_delivery_diagnosis", "ai.internal_error", retErr,
				slog.String("resource.type", diag.SubjectType),
				slog.String("resource.id", diag.SubjectID),
			)
			r.markDeliveryDiagnosisFailed(ctx, &diag, "ai.internal_error")
		}
		emitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if diag.NotificationEmittedAt != nil {
			if aiNotificationCtx != nil {
				r.syncDeliveryDiagnosisInbox(emitCtx, &diag, aiNotificationCtx)
			}
			return
		}
		if emitErr := r.emitAndStampDeliveryFailureNotification(emitCtx, &diag, aiNotificationCtx); emitErr != nil && retErr == nil {
			retErr = emitErr
		}
	}()

	if diag.Status == model.AIDiagnosisStatusSucceeded {
		aiNotificationCtx = toNotificationAIDiagnosis(diag.ToView())
		return nil
	}
	if diag.Status == model.AIDiagnosisStatusFailed {
		return nil
	}

	if r.aiAgent == nil || !r.aiAgentAvailable {
		r.failDeliveryDiagnosisWithLog(ctx, &diag, "ai.agent_unavailable", errors.New("AI agent client is unavailable"))
		return nil
	}

	enabled, accessMode := r.readPlatformAISettings(db)
	if !enabled {
		r.failDeliveryDiagnosisWithLog(ctx, &diag, "ai.disabled", errors.New("AI assistant is disabled"))
		return nil
	}

	var billedUser model.User
	if err := db.First(&billedUser, "id = ? AND disabled = ?", diag.BilledUserID, false).Error; err != nil {
		r.failDeliveryDiagnosisWithLog(ctx, &diag, "auth.account.disabled", err)
		return nil
	}
	if !userAuthorizedForProjectAIBilling(db, diag.ProjectID, billedUser, accessMode) {
		r.failDeliveryDiagnosisWithLog(ctx, &diag, "ai.user_not_allowed", errors.New("billed user is no longer authorized in project"))
		return nil
	}
	if err := (billing.Service{DB: db}).EnsureAIAvailableBalance(ctx, billedUser.ID); err != nil {
		code := "billing.wallet_unavailable"
		if errors.Is(err, billing.ErrAIWalletBalanceInsufficient) {
			code = "ai.wallet_balance_insufficient"
		}
		r.failDeliveryDiagnosisWithLog(ctx, &diag, code, err)
		return nil
	}

	contextPayload, err := r.buildDeliveryDiagnosisContextPayload(ctx, diag)
	if err != nil {
		r.failDeliveryDiagnosisWithLog(ctx, &diag, "ai.context_unavailable", err)
		return nil
	}

	locale := strings.TrimSpace(billedUser.Language)
	if locale == "" {
		locale = "zh-CN"
	}
	inferCtx, cancel := context.WithTimeout(ctx, deliveryDiagnosisAgentTimeout)
	defer cancel()

	resp, err := aiagent.RunHeadlessInference(
		inferCtx,
		r.aiAgent,
		aiagent.ActorContext{
			UserID:       billedUser.ID,
			SessionID:    "worker-delivery-diag:" + diag.ID,
			PlatformRole: billedUser.Role,
			ProjectID:    diag.ProjectID,
			Locale:       locale,
			RequestID:    id.New("req"),
		},
		aiagent.HeadlessInferenceRequest{
			ActorUserID:    billedUser.ID,
			ProjectID:      diag.ProjectID,
			TaskType:       aiagent.HeadlessTaskDeliveryFailureDiagnosis,
			MaxToolTier:    aiagent.HeadlessToolTierL0,
			MaxSteps:       4,
			ContextPayload: contextPayload,
		},
	)
	if err != nil {
		errCode := "ai.agent_unavailable"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(inferCtx.Err(), context.DeadlineExceeded) {
			errCode = "ai.timeout"
		}
		var upErr *aiagent.UpstreamError
		if errors.As(err, &upErr) && strings.TrimSpace(upErr.Code) != "" {
			errCode = strings.TrimSpace(upErr.Code)
		}
		r.failDeliveryDiagnosisWithLog(ctx, &diag, errCode, err)
		return nil
	}

	var parsed deliveryDiagnosisLLMOutput
	if err := json.Unmarshal(resp.Output, &parsed); err != nil || strings.TrimSpace(parsed.RootCauseSummary) == "" {
		r.failDeliveryDiagnosisWithLog(ctx, &diag, "ai.structured_output_invalid", errors.New("headless output missing rootCauseSummary"))
		return nil
	}

	confidence := strings.ToLower(strings.TrimSpace(parsed.Confidence))
	if confidence != "high" && confidence != "medium" && confidence != "low" {
		confidence = "medium"
	}
	cleanSuggestions := make([]string, 0, len(parsed.Suggestions))
	for _, s := range parsed.Suggestions {
		if trimmed := telemetry.RedactText(strings.TrimSpace(s)); trimmed != "" && len(cleanSuggestions) < 6 {
			cleanSuggestions = append(cleanSuggestions, trimmed)
		}
	}
	suggestionsBytes, _ := json.Marshal(cleanSuggestions)

	proposedJSON := sanitizeProposedActionJSON(parsed.ProposedAction, diag.ProjectID)

	finished := time.Now().UTC()
	rootSummary := telemetry.RedactText(strings.TrimSpace(parsed.RootCauseSummary))
	persistRes := db.Model(&model.AIDeliveryDiagnosis{}).
		Where("id = ? AND status = ?", diag.ID, model.AIDiagnosisStatusRunning).
		Updates(map[string]any{
			"ai_run_id":            resp.RunID,
			"status":               model.AIDiagnosisStatusSucceeded,
			"root_cause_summary":   rootSummary,
			"confidence":           confidence,
			"suggestions_json":     string(suggestionsBytes),
			"proposed_action_json": proposedJSON,
			"error_code":           "",
			"finished_at":          &finished,
			"updated_at":           finished,
		})
	if persistRes.Error != nil {
		retErr = telemetry.WrapError("ai.storage_unavailable", "", "ai_automation.execute_delivery_diagnosis", persistRes.Error)
		telemetry.LogError(ctx, "failed to persist succeeded delivery AI diagnosis", "worker.ai_delivery_diagnosis.persist_failed", "ai_automation.execute_delivery_diagnosis", "ai.storage_unavailable", retErr,
			slog.String("resource.type", diag.SubjectType),
			slog.String("resource.id", diag.SubjectID),
		)
		return retErr
	}
	if persistRes.RowsAffected == 0 {
		_ = db.First(&diag, "id = ?", diag.ID).Error
		if diag.Status == model.AIDiagnosisStatusSucceeded {
			aiNotificationCtx = toNotificationAIDiagnosis(diag.ToView())
		}
		return nil
	}

	diag.Status = model.AIDiagnosisStatusSucceeded
	diag.RootCauseSummary = rootSummary
	diag.Confidence = confidence
	diag.SuggestionsJSON = string(suggestionsBytes)
	diag.ProposedActionJSON = proposedJSON
	diag.FinishedAt = &finished
	aiNotificationCtx = toNotificationAIDiagnosis(diag.ToView())
	telemetry.Logger().InfoContext(ctx, "delivery failure AI diagnosis completed",
		slog.String("event.name", "worker.ai_delivery_diagnosis.completed"),
		slog.String("operation", "ai_automation.execute_delivery_diagnosis"),
		slog.String("outcome", "succeeded"),
		slog.String("resource.type", diag.SubjectType),
		slog.String("resource.id", diag.SubjectID),
	)
	return nil
}

func sanitizeProposedActionJSON(raw *model.AIProposedAction, projectID string) string {
	if raw == nil {
		return ""
	}
	toolName := strings.TrimSpace(raw.ToolName)
	if !model.IsAllowedAIRemediationTool(toolName) {
		return ""
	}
	redactedArgs, _ := redactJSONValue(raw.Arguments, 0).(map[string]any)
	if redactedArgs == nil {
		redactedArgs = map[string]any{}
	}
	redactedArgs["project"] = strings.TrimSpace(projectID)
	sanitized := model.AIProposedAction{
		ToolName:  toolName,
		Arguments: redactedArgs,
		Summary:   telemetry.RedactText(strings.TrimSpace(raw.Summary)),
	}
	actionBytes, err := json.Marshal(sanitized)
	if err != nil {
		return ""
	}
	return string(actionBytes)
}

func redactJSONValue(v any, depth int) any {
	if depth > 8 || v == nil {
		return nil
	}
	switch typed := v.(type) {
	case string:
		return telemetry.RedactText(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactJSONValue(item, depth+1))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			out[k] = redactJSONValue(val, depth+1)
		}
		return out
	default:
		return v
	}
}

func (r *Runner) failDeliveryDiagnosisWithLog(ctx context.Context, diag *model.AIDeliveryDiagnosis, errorCode string, cause error) {
	wrapped := telemetry.WrapError(errorCode, "", "ai_automation.execute_delivery_diagnosis", cause)
	telemetry.LogWarn(ctx, "delivery failure AI diagnosis did not complete", "worker.ai_delivery_diagnosis.failed", "ai_automation.execute_delivery_diagnosis", errorCode, wrapped,
		slog.String("resource.type", diag.SubjectType),
		slog.String("resource.id", diag.SubjectID),
	)
	r.markDeliveryDiagnosisFailed(ctx, diag, errorCode)
}

func (r *Runner) markDeliveryDiagnosisFailed(ctx context.Context, diag *model.AIDeliveryDiagnosis, errorCode string) {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	finished := time.Now().UTC()
	res := r.db.WithContext(persistCtx).Model(&model.AIDeliveryDiagnosis{}).
		Where("id = ? AND status = ?", diag.ID, model.AIDiagnosisStatusRunning).
		Updates(map[string]any{
			"status":      model.AIDiagnosisStatusFailed,
			"error_code":  errorCode,
			"finished_at": &finished,
			"updated_at":  finished,
		})
	if res.Error == nil && res.RowsAffected > 0 {
		diag.Status = model.AIDiagnosisStatusFailed
		diag.ErrorCode = errorCode
		diag.FinishedAt = &finished
	} else if res.Error == nil {
		_ = r.db.WithContext(persistCtx).First(diag, "id = ?", diag.ID).Error
	}
}

func (r *Runner) emitAndStampDeliveryFailureNotification(
	ctx context.Context,
	diag *model.AIDeliveryDiagnosis,
	aiDiag *notification.AIDiagnosisContext,
) error {
	if diag == nil || r.db == nil {
		return nil
	}
	db := r.db.WithContext(ctx)
	stampEmitted := func() error {
		emittedAt := time.Now().UTC()
		if err := db.Model(&model.AIDeliveryDiagnosis{}).Where("id = ?", diag.ID).Updates(map[string]any{
			"notification_emitted_at": &emittedAt,
			"updated_at":              emittedAt,
		}).Error; err != nil {
			return err
		}
		diag.NotificationEmittedAt = &emittedAt
		return nil
	}

	var emitErr error
	switch diag.SubjectType {
	case model.AIDeliverySubjectBuildRun:
		var run model.BuildRun
		if err := db.First(&run, "id = ? AND project_id = ?", diag.SubjectID, diag.ProjectID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return stampEmitted()
			}
			return err
		}
		msg := r.authoritativeBuildFailureMessage(db, run)
		emitErr = r.scoped(ctx).emitBuildEventWithAIError(ctx, run, "failed", msg, aiDiag)
	case model.AIDeliverySubjectRelease:
		var release model.Release
		if err := db.First(&release, "id = ? AND project_id = ?", diag.SubjectID, diag.ProjectID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return stampEmitted()
			}
			return err
		}
		msg := firstNonEmpty(strings.TrimSpace(release.Message), strings.TrimSpace(release.FailureCode))
		emitErr = r.scoped(ctx).emitReleaseEventWithAIError(ctx, release, "failed", msg, aiDiag)
	default:
		return stampEmitted()
	}
	if emitErr != nil {
		return emitErr
	}
	return stampEmitted()
}

func (r *Runner) syncDeliveryDiagnosisInbox(
	ctx context.Context,
	diag *model.AIDeliveryDiagnosis,
	aiDiag *notification.AIDiagnosisContext,
) {
	if diag == nil || r.db == nil || aiDiag == nil {
		return
	}
	db := r.db.WithContext(ctx)
	switch diag.SubjectType {
	case model.AIDeliverySubjectBuildRun:
		var run model.BuildRun
		if err := db.First(&run, "id = ? AND project_id = ?", diag.SubjectID, diag.ProjectID).Error; err == nil {
			msg := r.authoritativeBuildFailureMessage(db, run)
			r.scoped(ctx).publishBuildFailureInboxOnly(ctx, run, msg, aiDiag)
		}
	case model.AIDeliverySubjectRelease:
		var release model.Release
		if err := db.First(&release, "id = ? AND project_id = ?", diag.SubjectID, diag.ProjectID).Error; err == nil {
			msg := firstNonEmpty(strings.TrimSpace(release.Message), strings.TrimSpace(release.FailureCode))
			r.scoped(ctx).publishReleaseFailureInboxOnly(ctx, release, msg, aiDiag)
		}
	}
}

func (r *Runner) reconcileAIDeliveryDiagnoses(ctx context.Context) error {
	if r.db == nil {
		return nil
	}
	ctx, end := telemetry.StartOperation(ctx, "ai_automation", "reconcile_delivery_diagnoses")
	var retErr error
	defer func() { end(retErr) }()

	now := time.Now().UTC()
	staleCutoff := now.Add(-deliveryDiagnosisStaleRunningAge)
	db := r.db.WithContext(ctx)

	if err := db.Model(&model.AIDeliveryDiagnosis{}).
		Where("status = ? AND created_at < ?", model.AIDiagnosisStatusRunning, staleCutoff).
		Updates(map[string]any{
			"status":      model.AIDiagnosisStatusFailed,
			"error_code":  "ai.timeout",
			"finished_at": &now,
			"updated_at":  now,
		}).Error; err != nil {
		retErr = err
		return err
	}

	var pendingNotifications []model.AIDeliveryDiagnosis
	if err := db.Where(
		"status IN ? AND notification_emitted_at IS NULL",
		[]string{model.AIDiagnosisStatusSucceeded, model.AIDiagnosisStatusFailed},
	).Order("created_at asc").
		Limit(deliveryDiagnosisReconcileBatchSize).
		Find(&pendingNotifications).Error; err != nil {
		retErr = err
		return err
	}

	var errs []error
	for i := range pendingNotifications {
		row := &pendingNotifications[i]
		var aiCtx *notification.AIDiagnosisContext
		if row.Status == model.AIDiagnosisStatusSucceeded {
			aiCtx = toNotificationAIDiagnosis(row.ToView())
		}
		if err := r.emitAndStampDeliveryFailureNotification(ctx, row, aiCtx); err != nil {
			errs = append(errs, err)
		}
	}
	retErr = errors.Join(errs...)
	return retErr
}

func (r *Runner) authoritativeBuildFailureMessage(db *gorm.DB, run model.BuildRun) string {
	var job model.BuildJob
	if err := db.Where("build_run_id = ? AND project_id = ?", run.ID, run.ProjectID).
		Order("created_at desc").
		First(&job).Error; err == nil {
		if msg := strings.TrimSpace(job.Message); msg != "" {
			return msg
		}
	}
	return firstNonEmpty(strings.TrimSpace(run.FailureCode), "build_failed")
}

func toNotificationAIDiagnosis(view model.AIDeliveryDiagnosisView) *notification.AIDiagnosisContext {
	if strings.TrimSpace(view.RootCauseSummary) == "" {
		return nil
	}
	out := &notification.AIDiagnosisContext{
		RootCauseSummary: view.RootCauseSummary,
		Confidence:       view.Confidence,
		Suggestions:      view.Suggestions,
	}
	if view.ProposedAction != nil {
		out.ProposedActionSummary = firstNonEmpty(view.ProposedAction.Summary, view.ProposedAction.ToolName)
	}
	return out
}

func (r *Runner) buildDeliveryDiagnosisContextPayload(
	ctx context.Context,
	diag model.AIDeliveryDiagnosis,
) (map[string]any, error) {
	db := r.db.WithContext(ctx)
	payload := map[string]any{
		"projectId":          diag.ProjectID,
		"applicationId":      diag.ApplicationID,
		"deploymentTargetId": diag.DeploymentTargetID,
		"subjectType":        diag.SubjectType,
		"subjectId":          diag.SubjectID,
		"failureCode":        diag.FailureCode,
	}

	if strings.TrimSpace(diag.DeploymentTargetID) != "" {
		var target model.DeploymentTarget
		if err := db.First(&target, "id = ? AND project_id = ?", diag.DeploymentTargetID, diag.ProjectID).Error; err == nil {
			payload["deploymentTarget"] = sanitizeTargetSnapshotForAI(target)
		}
	}

	switch diag.SubjectType {
	case model.AIDeliverySubjectBuildRun:
		var run model.BuildRun
		if err := db.First(&run, "id = ? AND project_id = ?", diag.SubjectID, diag.ProjectID).Error; err != nil {
			return nil, err
		}
		payload["failureMessage"] = telemetry.RedactText(r.authoritativeBuildFailureMessage(db, run))
		payload["buildRun"] = map[string]any{
			"id":                   run.ID,
			"status":               run.Status,
			"failureCode":          run.FailureCode,
			"triggerType":          run.TriggerType,
			"sourceBranch":         run.SourceBranch,
			"sourceTag":            run.SourceTag,
			"sourceCommit":         run.SourceCommit,
			"buildDefinitionMode":  run.BuildDefinitionMode,
			"buildTemplateId":      run.BuildTemplateID,
			"buildTemplateVersion": run.BuildTemplateVersion,
			"dockerfilePath":       run.DockerfilePath,
			"buildContext":         run.BuildContext,
			"buildDirectory":       run.BuildDirectory,
			"buildCpuRequest":      run.BuildCPURequest,
			"buildMemoryRequest":   run.BuildMemoryRequest,
			"buildTimeoutSeconds":  run.BuildTimeoutSeconds,
		}
		var logs []model.BuildLog
		_ = db.Where("build_run_id = ? AND project_id = ?", run.ID, run.ProjectID).Order("created_at desc").Limit(1).Find(&logs).Error
		if len(logs) > 0 {
			payload["logTail"] = redactedLogTail(logs[0].Content)
		}
	case model.AIDeliverySubjectRelease:
		var release model.Release
		if err := db.First(&release, "id = ? AND project_id = ?", diag.SubjectID, diag.ProjectID).Error; err != nil {
			return nil, err
		}
		payload["failureMessage"] = telemetry.RedactText(strings.TrimSpace(release.Message))
		payload["release"] = map[string]any{
			"id":                 release.ID,
			"type":               release.Type,
			"status":             release.Status,
			"failureCode":        release.FailureCode,
			"revision":           release.Revision,
			"imageRef":           release.ImageRef,
			"message":            telemetry.RedactText(release.Message),
			"deploymentTargetId": release.DeploymentTargetID,
		}
		var logs []model.ReleaseLog
		_ = db.Where("release_id = ? AND project_id = ?", release.ID, release.ProjectID).Order("created_at desc").Limit(1).Find(&logs).Error
		if len(logs) > 0 {
			payload["logTail"] = redactedLogTail(logs[0].Content)
		}
	}
	return payload, nil
}

func redactedLogTail(content string) string {
	rawWindow := tailValidUTF8(content, maxDeliveryDiagnosisRawTailWindowBytes)
	if rawWindow == "" {
		return ""
	}
	return tailValidUTF8(telemetry.RedactText(rawWindow), maxDeliveryDiagnosisLogTailBytes)
}

func sanitizeTargetSnapshotForAI(target model.DeploymentTarget) map[string]any {
	return map[string]any{
		"id":                   target.ID,
		"name":                 target.Name,
		"stage":                target.Stage,
		"workloadType":         target.WorkloadType,
		"sourceType":           target.SourceType,
		"replicas":             target.Replicas,
		"cpuRequest":           target.CPURequest,
		"memoryRequest":        target.MemoryRequest,
		"buildDefinitionMode":  target.BuildDefinitionMode,
		"buildTemplateId":      target.BuildTemplateID,
		"buildTemplateVersion": target.BuildTemplateVersion,
		"dockerfilePath":       target.DockerfilePath,
		"buildContext":         target.BuildContext,
		"buildDirectory":       target.BuildDirectory,
		"servicePorts":         target.ServicePorts,
		"containerCommand":     telemetry.RedactText(target.ContainerCommand),
		"containerArgs":        telemetry.RedactText(target.ContainerArgs),
		"readinessProbe":       telemetry.RedactText(target.ReadinessProbe),
		"livenessProbe":        telemetry.RedactText(target.LivenessProbe),
		"startupProbe":         telemetry.RedactText(target.StartupProbe),
	}
}

func tailValidUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 || s == "" {
		return ""
	}
	if len(s) <= maxBytes {
		return strings.ToValidUTF8(s, "")
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return strings.ToValidUTF8(s[start:], "")
}
