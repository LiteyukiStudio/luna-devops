package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
)

const (
	defaultRuntimeDiagnosisInlineTimeout = 15 * time.Second
	runtimeDiagnosisAgentTimeout         = 45 * time.Second
	runtimeDiagnosisAsyncTotalTimeout    = 90 * time.Second
	runtimeAutoHealRequestTimeout        = 25 * time.Second
	runtimeDiagnosisStaleRunningAge      = 3 * time.Minute
	runtimeDiagnosisReconcileBatchSize   = 50
	runtimeSelfHealProposalTTL           = 24 * time.Hour
	runtimeCancelFlushTimeout            = 5 * time.Second
)

type runtimeInboxSelfHealProposal struct {
	IncidentID              string
	ApplicationID           string
	DeploymentTargetID      string
	ToolName                string
	Arguments               map[string]any
	ExpectedSummary         string
	BaselineReleaseID       string
	BaselineTargetUpdatedAt string
	CreatedAt               string
}

type runtimeDiagnosisOutcome struct {
	succeeded         bool
	autoHealSucceeded bool
	aiDiagCtx         *notification.AIDiagnosisContext
	inboxExtraParams  map[string]any
	pendingProposal   *runtimeInboxSelfHealProposal
	billedUserID      string
}

type runtimeInlineCoordinator struct {
	mu       sync.Mutex
	finished bool
	timedOut bool
	outcome  runtimeDiagnosisOutcome
}

type internalAutoHealRequest struct {
	ActorUserID  string          `json:"actorUserId"`
	ProjectID    string          `json:"projectId"`
	IncidentID   string          `json:"incidentId"`
	DiagnosisID  string          `json:"diagnosisId,omitempty"`
	SelfHealMode string          `json:"selfHealMode"`
	AllowedTools []string        `json:"allowedTools"`
	ToolName     string          `json:"toolName"`
	Arguments    json.RawMessage `json:"arguments"`
}

type internalAutoHealResponse struct {
	ToolName           string `json:"toolName"`
	ApprovalTier       string `json:"approvalTier"`
	PreviewSummary     string `json:"previewSummary"`
	BeforeAfterSummary string `json:"beforeAfterSummary"`
}

type internalAutoHealErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// WaitRuntimeDiagnosisAsync waits for any in-flight asynchronous (>15s) runtime diagnosis goroutines.
func (r *Runner) WaitRuntimeDiagnosisAsync() {
	if r != nil && r.runtimeDiagnosisAsyncWG != nil {
		r.runtimeDiagnosisAsyncWG.Wait()
	}
}

func (r *Runner) effectiveRuntimeDiagnosisInlineTimeout(budget time.Duration) time.Duration {
	perTarget := defaultRuntimeDiagnosisInlineTimeout
	if r != nil && r.runtimeDiagnosisInlineTimeout > 0 {
		perTarget = r.runtimeDiagnosisInlineTimeout
	}
	if budget <= 0 {
		return 0
	}
	if budget < perTarget {
		return budget
	}
	return perTarget
}

func (r *Runner) emitAndCommitRuntimeAnomalyAlert(
	ctx context.Context,
	db *gorm.DB,
	decision runtimeAlertDecision,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	anomaly runtimeDetectedAnomaly,
	now time.Time,
	aiDiag *notification.AIDiagnosisContext,
) {
	eventID := r.emitRuntimeAnomalyNotification(ctx, decision, target, project, application, anomaly, now, aiDiag)
	updates := map[string]any{
		"rule_notifications_json": decision.Incident.RuleNotificationsJSON,
		"notification_count":      decision.Incident.NotificationCount,
		"last_notified_at":        decision.Incident.LastNotifiedAt,
		"updated_at":              now,
	}
	if eventID != "" {
		updates["last_event_id"] = eventID
	}
	_ = db.WithContext(ctx).Model(&model.RuntimeAlertIncident{}).
		Where("id = ?", decision.Incident.ID).
		Updates(updates).Error
}

func (r *Runner) dispatchRuntimeAnomalyAlertAndInbox(
	ctx context.Context,
	decision runtimeAlertDecision,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	observation kubeprovider.WorkloadRuntimeObservation,
	anomaly runtimeDetectedAnomaly,
	now time.Time,
	inlineWaitBudget time.Duration,
) {
	db := r.db.WithContext(ctx)

	if !decision.ShouldEmitAlert {
		if decision.ShouldUpdateInbox {
			r.publishRuntimeAnomalyInbox(ctx, decision, target, project, application, anomaly)
		}
		return
	}

	diagRow, cfg, billedUser, scheduled := r.tryReserveRuntimeDiagnosis(
		ctx,
		db,
		decision.Incident,
		target,
		anomaly,
		now,
	)
	if !scheduled {
		r.emitAndCommitRuntimeAnomalyAlert(ctx, db, decision, target, project, application, anomaly, now, nil)
		if decision.ShouldUpdateInbox {
			r.publishRuntimeAnomalyInbox(ctx, decision, target, project, application, anomaly)
		}
		return
	}

	inlineTimeout := r.effectiveRuntimeDiagnosisInlineTimeout(inlineWaitBudget)
	coord := &runtimeInlineCoordinator{
		timedOut: inlineTimeout <= 0,
	}
	doneCh := make(chan struct{})
	if r.runtimeDiagnosisAsyncWG != nil {
		r.runtimeDiagnosisAsyncWG.Add(1)
	}

	bgDetached := context.WithoutCancel(ctx)
	go func() {
		bgBaseCtx, bgCancel := context.WithTimeout(bgDetached, runtimeDiagnosisAsyncTotalTimeout)
		defer bgCancel()
		if r.runtimeDiagnosisAsyncWG != nil {
			defer r.runtimeDiagnosisAsyncWG.Done()
		}
		defer func() {
			if rec := recover(); rec != nil {
				panicErr := fmt.Errorf("panic in runtime AI diagnosis: %v", rec)
				telemetry.LogError(bgBaseCtx, "runtime AI diagnosis panicked", "worker.ai_runtime_diagnosis.panic", "ai_automation.execute_runtime_diagnosis", "ai.internal_panic", panicErr,
					slog.String("resource.type", "runtime_incident"),
					slog.String("resource.id", decision.Incident.ID),
				)
				finishedAt := maxTime(time.Now().UTC(), diagRow.CreatedAt)
				_ = r.db.WithContext(bgBaseCtx).Model(&model.AIRuntimeIncidentDiagnosis{}).
					Where("id = ? AND status = ?", diagRow.ID, model.AIDiagnosisStatusRunning).
					Updates(map[string]any{
						"status":      model.AIDiagnosisStatusFailed,
						"error_code":  "ai.internal_panic",
						"finished_at": &finishedAt,
						"updated_at":  finishedAt,
					}).Error
			}
			close(doneCh)
		}()

		outcome := r.executeRuntimeDiagnosisAndSelfHeal(
			bgBaseCtx,
			diagRow,
			cfg,
			billedUser,
			decision.Incident,
			application,
			target,
			observation,
			anomaly,
		)

		coord.mu.Lock()
		coord.finished = true
		coord.outcome = outcome
		wasTimedOut := coord.timedOut
		coord.mu.Unlock()

		if wasTimedOut && outcome.succeeded {
			r.applyAsyncRuntimeDiagnosisToInbox(
				bgBaseCtx,
				diagRow.ID,
				decision,
				target,
				project,
				application,
				anomaly,
				outcome,
			)
		} else if wasTimedOut {
			syncedAt := maxTime(time.Now().UTC(), diagRow.CreatedAt)
			_ = r.db.WithContext(bgBaseCtx).Model(&model.AIRuntimeIncidentDiagnosis{}).
				Where("id = ?", diagRow.ID).
				Update("inbox_synced_at", &syncedAt).Error
		}
	}()

	// If cumulative pass budget was exhausted, emit base alert immediately without blocking patrol.
	if inlineTimeout <= 0 {
		r.emitAndCommitRuntimeAnomalyAlert(ctx, db, decision, target, project, application, anomaly, now, nil)
		if decision.ShouldUpdateInbox {
			r.publishRuntimeAnomalyInbox(ctx, decision, target, project, application, anomaly)
		}
		return
	}

	timer := time.NewTimer(inlineTimeout)
	defer timer.Stop()

	select {
	case <-doneCh:
		coord.mu.Lock()
		outcome := coord.outcome
		coord.mu.Unlock()
		r.finalizeInlineRuntimeDiagnosisAlertAndInbox(
			ctx,
			db,
			diagRow.ID,
			decision,
			target,
			project,
			application,
			anomaly,
			now,
			outcome,
		)
		return

	case <-timer.C:
		coord.mu.Lock()
		if coord.finished {
			outcome := coord.outcome
			coord.mu.Unlock()
			r.finalizeInlineRuntimeDiagnosisAlertAndInbox(
				ctx,
				db,
				diagRow.ID,
				decision,
				target,
				project,
				application,
				anomaly,
				now,
				outcome,
			)
			return
		}
		coord.timedOut = true
		coord.mu.Unlock()

		telemetry.Logger().InfoContext(ctx, "runtime AI diagnosis exceeded inline budget; emitting base alert immediately",
			slog.String("event.name", "worker.ai_runtime_diagnosis.inline_timeout"),
			slog.String("operation", "ai_automation.schedule_runtime_diagnosis"),
			slog.String("outcome", "deferred_to_inbox"),
			slog.String("resource.type", "runtime_incident"),
			slog.String("resource.id", decision.Incident.ID),
		)

		r.emitAndCommitRuntimeAnomalyAlert(ctx, db, decision, target, project, application, anomaly, now, nil)
		if decision.ShouldUpdateInbox {
			r.publishRuntimeAnomalyInbox(ctx, decision, target, project, application, anomaly)
		}
		return

	case <-ctx.Done():
		flushCtx, flushCancel := context.WithTimeout(bgDetached, runtimeCancelFlushTimeout)
		defer flushCancel()
		flushDB := r.db.WithContext(flushCtx)

		coord.mu.Lock()
		if coord.finished {
			outcome := coord.outcome
			coord.mu.Unlock()
			r.finalizeInlineRuntimeDiagnosisAlertAndInbox(
				flushCtx,
				flushDB,
				diagRow.ID,
				decision,
				target,
				project,
				application,
				anomaly,
				now,
				outcome,
			)
			return
		}
		coord.timedOut = true
		coord.mu.Unlock()

		r.emitAndCommitRuntimeAnomalyAlert(flushCtx, flushDB, decision, target, project, application, anomaly, now, nil)
		if decision.ShouldUpdateInbox {
			r.publishRuntimeAnomalyInbox(flushCtx, decision, target, project, application, anomaly)
		}
	}
}

func (r *Runner) finalizeInlineRuntimeDiagnosisAlertAndInbox(
	ctx context.Context,
	db *gorm.DB,
	diagnosisID string,
	decision runtimeAlertDecision,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	anomaly runtimeDetectedAnomaly,
	now time.Time,
	outcome runtimeDiagnosisOutcome,
) {
	r.emitAndCommitRuntimeAnomalyAlert(ctx, db, decision, target, project, application, anomaly, now, outcome.aiDiagCtx)
	if decision.ShouldUpdateInbox || outcome.succeeded {
		inboxDecision := decision
		if outcome.succeeded {
			inboxDecision.MarkInboxUnread = true
		}
		r.publishRuntimeAnomalyInboxWithOptions(
			ctx,
			inboxDecision,
			target,
			project,
			application,
			anomaly,
			outcome.inboxExtraParams,
			outcome.pendingProposal,
			outcome.billedUserID,
			outcome.autoHealSucceeded,
		)
	}
	syncedAt := maxTime(time.Now().UTC(), now.UTC())
	_ = db.WithContext(ctx).Model(&model.AIRuntimeIncidentDiagnosis{}).
		Where("id = ?", diagnosisID).
		Updates(map[string]any{
			"inbox_synced_at": &syncedAt,
			"updated_at":      syncedAt,
		}).Error
}

func (r *Runner) applyAsyncRuntimeDiagnosisToInbox(
	ctx context.Context,
	diagnosisID string,
	decision runtimeAlertDecision,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	anomaly runtimeDetectedAnomaly,
	outcome runtimeDiagnosisOutcome,
) {
	db := r.db.WithContext(ctx)
	var liveInc model.RuntimeAlertIncident
	if err := db.First(&liveInc, "id = ? AND project_id = ?", decision.Incident.ID, target.ProjectID).Error; err != nil || liveInc.Status != "active" {
		syncedAt := time.Now().UTC()
		_ = db.Model(&model.AIRuntimeIncidentDiagnosis{}).
			Where("id = ?", diagnosisID).
			Update("inbox_synced_at", &syncedAt).Error
		return
	}

	inboxDecision := decision
	inboxDecision.Incident = liveInc
	inboxDecision.MarkInboxUnread = true
	r.publishRuntimeAnomalyInboxWithOptions(
		ctx,
		inboxDecision,
		target,
		project,
		application,
		anomaly,
		outcome.inboxExtraParams,
		outcome.pendingProposal,
		outcome.billedUserID,
		outcome.autoHealSucceeded,
	)
	syncedAt := time.Now().UTC()
	_ = db.Model(&model.AIRuntimeIncidentDiagnosis{}).
		Where("id = ?", diagnosisID).
		Updates(map[string]any{
			"inbox_synced_at": &syncedAt,
			"updated_at":      syncedAt,
		}).Error
}

func (r *Runner) tryReserveRuntimeDiagnosis(
	ctx context.Context,
	db *gorm.DB,
	incident model.RuntimeAlertIncident,
	target model.DeploymentTarget,
	anomaly runtimeDetectedAnomaly,
	now time.Time,
) (model.AIRuntimeIncidentDiagnosis, model.ProjectAIAutomationConfig, model.User, bool) {
	if r == nil || r.aiAgent == nil || !r.aiAgentAvailable || db == nil {
		return model.AIRuntimeIncidentDiagnosis{}, model.ProjectAIAutomationConfig{}, model.User{}, false
	}
	projectID := strings.TrimSpace(target.ProjectID)
	if projectID == "" || strings.TrimSpace(incident.ID) == "" {
		return model.AIRuntimeIncidentDiagnosis{}, model.ProjectAIAutomationConfig{}, model.User{}, false
	}

	ctx, end := telemetry.StartOperation(
		ctx,
		"ai_automation",
		"schedule_runtime_diagnosis",
		attribute.String("resource.type", "runtime_incident"),
		attribute.String("resource.id", incident.ID),
	)
	var opErr error
	defer func() { end(opErr) }()
	db = db.WithContext(ctx)

	enabled, accessMode := r.readPlatformAISettings(db)
	if !enabled {
		return model.AIRuntimeIncidentDiagnosis{}, model.ProjectAIAutomationConfig{}, model.User{}, false
	}

	var cfg model.ProjectAIAutomationConfig
	if err := db.First(&cfg, "project_id = ?", projectID).Error; err != nil || !cfg.RuntimeDiagnosisEnabled {
		return model.AIRuntimeIncidentDiagnosis{}, model.ProjectAIAutomationConfig{}, model.User{}, false
	}
	if strings.TrimSpace(cfg.RuntimeBillingUserID) == "" {
		return model.AIRuntimeIncidentDiagnosis{}, model.ProjectAIAutomationConfig{}, model.User{}, false
	}

	billedUser, ok := resolveProjectAIBillingUser(ctx, db, projectID, "", cfg.RuntimeBillingUserID, accessMode)
	if !ok {
		return model.AIRuntimeIncidentDiagnosis{}, model.ProjectAIAutomationConfig{}, model.User{}, false
	}

	nowUTC := now.UTC()
	diagRow := model.AIRuntimeIncidentDiagnosis{
		ID:                 id.New("aird"),
		ProjectID:          projectID,
		ApplicationID:      strings.TrimSpace(target.ApplicationID),
		DeploymentTargetID: strings.TrimSpace(target.ID),
		IncidentID:         strings.TrimSpace(incident.ID),
		AnomalyType:        strings.TrimSpace(anomaly.EventType),
		BilledUserID:       billedUser.ID,
		Status:             model.AIDiagnosisStatusRunning,
		Confidence:         "medium",
		SuggestionsJSON:    "[]",
		AutoHealMode:       cfg.EffectiveSelfHealModeForStage(target.Stage),
		AutoHealStatus:     model.AIAutoHealStatusNone,
		CreatedAt:          nowUTC,
		UpdatedAt:          nowUTC,
	}

	reserved := false
	txErr := db.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector != nil && tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "ai_runtime_diag:"+projectID).Error; err != nil {
				return err
			}
		}
		var activeRunning int64
		if err := tx.Model(&model.AIRuntimeIncidentDiagnosis{}).
			Where("(incident_id = ? OR deployment_target_id = ?) AND status = ?", incident.ID, target.ID, model.AIDiagnosisStatusRunning).
			Count(&activeRunning).Error; err != nil {
			return err
		}
		if activeRunning > 0 {
			return nil
		}
		if cfg.RuntimeCooldownSeconds > 0 {
			cutoff := nowUTC.Add(-time.Duration(cfg.RuntimeCooldownSeconds) * time.Second)
			var recent int64
			if err := tx.Model(&model.AIRuntimeIncidentDiagnosis{}).
				Where("project_id = ? AND created_at > ?", projectID, cutoff).
				Count(&recent).Error; err != nil {
				return err
			}
			if recent > 0 {
				return nil
			}
		}
		dailyLimit := cfg.RuntimeDailyLimit
		if dailyLimit <= 0 {
			dailyLimit = model.DefaultRuntimeDiagnosisDailyLimit
		}
		dayStart := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
		var todayCount int64
		if err := tx.Model(&model.AIRuntimeIncidentDiagnosis{}).
			Where("project_id = ? AND created_at >= ?", projectID, dayStart).
			Count(&todayCount).Error; err != nil {
			return err
		}
		if todayCount >= int64(dailyLimit) {
			return nil
		}
		if err := tx.Create(&diagRow).Error; err != nil {
			return err
		}
		reserved = true
		return nil
	})
	if txErr != nil {
		opErr = telemetry.WrapError("ai.schedule_failed", "", "ai_automation.schedule_runtime_diagnosis", txErr)
		telemetry.LogWarn(ctx, "failed to reserve runtime AI diagnosis", "worker.ai_runtime_diagnosis.reserve_failed", "ai_automation.schedule_runtime_diagnosis", "ai.schedule_failed", opErr,
			slog.String("resource.type", "runtime_incident"),
			slog.String("resource.id", incident.ID),
		)
		return model.AIRuntimeIncidentDiagnosis{}, model.ProjectAIAutomationConfig{}, model.User{}, false
	}
	if !reserved {
		return model.AIRuntimeIncidentDiagnosis{}, model.ProjectAIAutomationConfig{}, model.User{}, false
	}
	return diagRow, cfg, billedUser, true
}

func (r *Runner) executeRuntimeDiagnosisAndSelfHeal(
	parentCtx context.Context,
	diagRow model.AIRuntimeIncidentDiagnosis,
	cfg model.ProjectAIAutomationConfig,
	billedUser model.User,
	incident model.RuntimeAlertIncident,
	application model.Application,
	target model.DeploymentTarget,
	observation kubeprovider.WorkloadRuntimeObservation,
	anomaly runtimeDetectedAnomaly,
) (outcome runtimeDiagnosisOutcome) {
	ctx, end := telemetry.StartOperation(
		parentCtx,
		"ai_automation",
		"execute_runtime_diagnosis",
		attribute.String("resource.type", "runtime_incident"),
		attribute.String("resource.id", incident.ID),
	)
	var opErr error
	defer func() { end(opErr) }()

	outcome.billedUserID = billedUser.ID
	db := r.db.WithContext(ctx)

	// Re-verify live billing user eligibility right before LLM spend.
	enabled, accessMode := r.readPlatformAISettings(db)
	if !enabled {
		r.failRuntimeDiagnosisRow(ctx, db, diagRow.ID, diagRow.CreatedAt, "ai.disabled", errors.New("AI assistant disabled"))
		return outcome
	}
	liveUser, ok := resolveProjectAIBillingUser(ctx, db, diagRow.ProjectID, "", billedUser.ID, accessMode)
	if !ok {
		r.failRuntimeDiagnosisRow(ctx, db, diagRow.ID, diagRow.CreatedAt, "ai.billing_unavailable", errors.New("runtime billing user ineligible or insufficient wallet balance"))
		return outcome
	}
	outcome.billedUserID = liveUser.ID

	var liveTarget model.DeploymentTarget
	if err := db.First(&liveTarget, "id = ? AND project_id = ? AND deleted_at IS NULL", target.ID, diagRow.ProjectID).Error; err == nil {
		target = liveTarget
	}
	var liveApp model.Application
	if err := db.First(&liveApp, "id = ? AND project_id = ? AND deleted_at IS NULL", target.ApplicationID, diagRow.ProjectID).Error; err == nil {
		application = liveApp
	}

	baselineReleaseID := strings.TrimSpace(observation.Workload.ReleaseID)
	var recentReleases []model.Release
	_ = db.Where("project_id = ? AND deployment_target_id = ?", diagRow.ProjectID, target.ID).
		Order("created_at desc").
		Limit(3).
		Find(&recentReleases).Error
	if baselineReleaseID == "" && len(recentReleases) > 0 {
		baselineReleaseID = recentReleases[0].ID
	}

	contextPayload := buildRuntimeDiagnosisContextPayload(diagRow.ProjectID, incident, target, observation, anomaly, recentReleases)

	locale := strings.TrimSpace(liveUser.Language)
	if locale == "" {
		locale = "zh-CN"
	}
	actor := aiagent.ActorContext{
		UserID:       liveUser.ID,
		SessionID:    "worker-runtime-diag:" + diagRow.ID,
		PlatformRole: liveUser.Role,
		ProjectID:    diagRow.ProjectID,
		Locale:       locale,
		RequestID:    id.New("req"),
	}
	callCtx, cancel := context.WithTimeout(ctx, runtimeDiagnosisAgentTimeout)
	resp, err := aiagent.RunHeadlessInference(callCtx, r.aiAgent, actor, aiagent.HeadlessInferenceRequest{
		ActorUserID:    liveUser.ID,
		ProjectID:      diagRow.ProjectID,
		TaskType:       aiagent.HeadlessTaskIncidentDiagnosis,
		MaxToolTier:    aiagent.HeadlessToolTierL0,
		MaxSteps:       5,
		ContextPayload: contextPayload,
	})
	cancel()
	if err != nil {
		errCode := "ai.agent_unavailable"
		if errors.Is(err, context.DeadlineExceeded) {
			errCode = "ai.timeout"
		}
		var upErr *aiagent.UpstreamError
		if errors.As(err, &upErr) && strings.TrimSpace(upErr.Code) != "" {
			errCode = strings.TrimSpace(upErr.Code)
		}
		opErr = err
		r.failRuntimeDiagnosisRow(ctx, db, diagRow.ID, diagRow.CreatedAt, errCode, err)
		return outcome
	}

	var parsed deliveryDiagnosisLLMOutput
	if err := json.Unmarshal(resp.Output, &parsed); err != nil || strings.TrimSpace(parsed.RootCauseSummary) == "" {
		opErr = errors.New("runtime headless output missing rootCauseSummary")
		r.failRuntimeDiagnosisRow(ctx, db, diagRow.ID, diagRow.CreatedAt, "ai.structured_output_invalid", opErr)
		return outcome
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
	rootSummary := telemetry.RedactText(strings.TrimSpace(parsed.RootCauseSummary))

	sanitizedAction, proposedJSON := sanitizeRuntimeHealAction(parsed.ProposedAction, diagRow.ProjectID, application, target)

	// Reload latest project config so immediate policy downgrades take effect before auto-healing.
	var latestCfg model.ProjectAIAutomationConfig
	if err := db.First(&latestCfg, "project_id = ?", diagRow.ProjectID).Error; err == nil {
		cfg = latestCfg
	}
	effectiveMode := cfg.EffectiveSelfHealModeForStage(target.Stage)
	autoHealStatus := model.AIAutoHealStatusNone
	autoHealTool := ""
	autoHealSummary := ""
	circuitBreakerTripped := false
	createManualProposal := sanitizedAction != nil
	refNow := maxTime(time.Now().UTC(), diagRow.CreatedAt)

	if sanitizedAction != nil && effectiveMode != model.AISelfHealModeSuggestOnly && cfg.AllowsAutoHealTool(sanitizedAction.ToolName) {
		var liveInc model.RuntimeAlertIncident
		incidentActive := db.First(&liveInc, "id = ? AND project_id = ?", incident.ID, diagRow.ProjectID).Error == nil && liveInc.Status == "active"
		if incidentActive {
			if effectiveMode == model.AISelfHealModeAutoL1 && isKnownL2RuntimeHealTool(sanitizedAction.ToolName) {
				// Short-circuit L2 delivery mutations under auto_l1 without an unnecessary HTTP 403 round-trip.
				autoHealStatus = model.AIAutoHealStatusNone
			} else {
				windowSecs := cfg.CircuitBreakerWindowSeconds
				if windowSecs < 60 {
					windowSecs = model.DefaultRuntimeCircuitBreakerWindowSeconds
				}
				maxConsecutive := cfg.MaxConsecutiveAutoHeals
				if maxConsecutive < 1 {
					maxConsecutive = model.DefaultRuntimeMaxConsecutiveAutoHeals
				}
				windowCutoff := refNow.Add(-time.Duration(windowSecs) * time.Second)
				var recentSucceededHeals int64
				_ = db.Model(&model.AIRuntimeIncidentDiagnosis{}).
					Where("deployment_target_id = ? AND auto_heal_status = ? AND created_at >= ?",
						target.ID, model.AIAutoHealStatusSucceeded, windowCutoff).
					Count(&recentSucceededHeals).Error

				if recentSucceededHeals >= int64(maxConsecutive) {
					circuitBreakerTripped = true
					autoHealStatus = model.AIAutoHealStatusCircuitBroken
					effectiveMode = model.AISelfHealModeSuggestOnly
					telemetry.Logger().WarnContext(ctx, "runtime AI auto-heal circuit breaker tripped; downgrading to suggest_only",
						slog.String("event.name", "worker.ai_runtime_self_heal.circuit_breaker_tripped"),
						slog.String("operation", "ai_automation.execute_runtime_diagnosis"),
						slog.String("outcome", "circuit_broken"),
						slog.String("resource.type", "deployment_target"),
						slog.String("resource.id", target.ID),
					)
				} else {
					healOut, statusCode, errCode, healErr := r.invokeInternalAutoHeal(
						ctx,
						liveUser,
						diagRow.ProjectID,
						incident.ID,
						diagRow.ID,
						effectiveMode,
						cfg.AllowedAutoHealTools(),
						*sanitizedAction,
					)
					if healErr == nil {
						autoHealStatus = model.AIAutoHealStatusSucceeded
						autoHealTool = healOut.ToolName
						autoHealSummary = telemetry.RedactText(firstNonEmpty(healOut.BeforeAfterSummary, healOut.PreviewSummary, sanitizedAction.Summary))
						createManualProposal = false
						outcome.autoHealSucceeded = true
						r.expirePendingRuntimeSelfHealRequests(ctx, diagRow.ProjectID, incident.ID, refNow)
						telemetry.Logger().InfoContext(ctx, "runtime AI auto-heal executed",
							slog.String("event.name", "worker.ai_runtime_self_heal.succeeded"),
							slog.String("operation", "ai_automation.execute_runtime_diagnosis"),
							slog.String("outcome", "succeeded"),
							slog.String("resource.type", "deployment_target"),
							slog.String("resource.id", target.ID),
						)
					} else if statusCode == http.StatusConflict && errCode == "ai.self_heal_circuit_broken" {
						circuitBreakerTripped = true
						autoHealStatus = model.AIAutoHealStatusCircuitBroken
						effectiveMode = model.AISelfHealModeSuggestOnly
					} else if statusCode == http.StatusForbidden && (errCode == "ai.self_heal_tier_exceeded" || errCode == "ai.self_heal_tool_not_whitelisted" || errCode == "ai.self_heal_not_permitted") {
						autoHealStatus = model.AIAutoHealStatusNone
					} else {
						autoHealStatus = model.AIAutoHealStatusFailed
						telemetry.LogWarn(ctx, "runtime AI auto-heal execution failed; falling back to manual inbox proposal", "worker.ai_runtime_self_heal.failed", "ai_automation.execute_runtime_diagnosis", firstNonEmpty(errCode, "ai.auto_heal_failed"), healErr,
							slog.String("resource.type", "deployment_target"),
							slog.String("resource.id", target.ID),
						)
					}
				}
			}
		} else {
			createManualProposal = false
		}
	}

	finishedAt := maxTime(time.Now().UTC(), diagRow.CreatedAt)
	_ = db.Model(&model.AIRuntimeIncidentDiagnosis{}).
		Where("id = ? AND status = ?", diagRow.ID, model.AIDiagnosisStatusRunning).
		Updates(map[string]any{
			"ai_run_id":               resp.RunID,
			"status":                  model.AIDiagnosisStatusSucceeded,
			"root_cause_summary":      rootSummary,
			"confidence":              confidence,
			"suggestions_json":        string(suggestionsBytes),
			"proposed_action_json":    proposedJSON,
			"auto_heal_mode":          effectiveMode,
			"auto_heal_status":        autoHealStatus,
			"auto_heal_summary":       autoHealSummary,
			"circuit_breaker_tripped": circuitBreakerTripped,
			"error_code":              "",
			"finished_at":             &finishedAt,
			"updated_at":              finishedAt,
		}).Error

	diagCtx := &notification.AIDiagnosisContext{
		RootCauseSummary:      rootSummary,
		Confidence:            confidence,
		Suggestions:           cleanSuggestions,
		AutoHealSummary:       autoHealSummary,
		CircuitBreakerTripped: circuitBreakerTripped,
	}
	if createManualProposal && sanitizedAction != nil {
		diagCtx.ProposedActionSummary = firstNonEmpty(sanitizedAction.Summary, sanitizedAction.ToolName)
	}

	extraParams := map[string]any{
		"aiRootCauseSummary":      rootSummary,
		"aiConfidence":            confidence,
		"aiSuggestions":           cleanSuggestions,
		"aiCircuitBreakerTripped": circuitBreakerTripped,
	}
	if autoHealSummary != "" {
		extraParams["aiAutoHealSummary"] = autoHealSummary
		extraParams["aiAutoHealTool"] = autoHealTool
	}

	var pendingProposal *runtimeInboxSelfHealProposal
	if createManualProposal && sanitizedAction != nil {
		targetUpdatedAt := ""
		if !target.UpdatedAt.IsZero() {
			targetUpdatedAt = target.UpdatedAt.UTC().Format(time.RFC3339Nano)
		}
		pendingProposal = &runtimeInboxSelfHealProposal{
			IncidentID:              incident.ID,
			ApplicationID:           target.ApplicationID,
			DeploymentTargetID:      target.ID,
			ToolName:                sanitizedAction.ToolName,
			Arguments:               sanitizedAction.Arguments,
			ExpectedSummary:         firstNonEmpty(sanitizedAction.Summary, sanitizedAction.ToolName),
			BaselineReleaseID:       baselineReleaseID,
			BaselineTargetUpdatedAt: targetUpdatedAt,
			CreatedAt:               finishedAt.Format(time.RFC3339),
		}
	}

	outcome.succeeded = true
	outcome.aiDiagCtx = diagCtx
	outcome.inboxExtraParams = extraParams
	outcome.pendingProposal = pendingProposal
	return outcome
}

func isKnownL2RuntimeHealTool(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "delivery_restart_target", "delivery_rollback_target", "delivery_scale_target", "delivery_update_target_runtime", "delivery_deploy_target":
		return true
	default:
		return false
	}
}

func (r *Runner) invokeInternalAutoHeal(
	ctx context.Context,
	actorUser model.User,
	projectID string,
	incidentID string,
	diagnosisID string,
	selfHealMode string,
	allowedTools []string,
	action model.AIProposedAction,
) (*internalAutoHealResponse, int, string, error) {
	ctx, end := telemetry.StartOperation(
		ctx,
		"ai_automation",
		"execute_auto_heal",
		attribute.String("resource.type", "runtime_incident"),
		attribute.String("resource.id", incidentID),
	)
	var opErr error
	defer func() { end(opErr) }()

	baseURL := strings.TrimRight(strings.TrimSpace(r.internalAPIBaseURL), "/")
	token := strings.TrimSpace(r.aiCallbackServiceToken)
	if baseURL == "" || token == "" {
		opErr = errors.New("internalAPIBaseURL and callback service token are required for auto-heal")
		return nil, 0, "ai.auto_heal_unconfigured", opErr
	}

	rawArgs, err := json.Marshal(action.Arguments)
	if err != nil {
		opErr = err
		return nil, 0, "ai.tool_arguments_invalid", err
	}
	reqBody, err := json.Marshal(internalAutoHealRequest{
		ActorUserID:  actorUser.ID,
		ProjectID:    projectID,
		IncidentID:   incidentID,
		DiagnosisID:  diagnosisID,
		SelfHealMode: selfHealMode,
		AllowedTools: allowedTools,
		ToolName:     action.ToolName,
		Arguments:    rawArgs,
	})
	if err != nil {
		opErr = err
		return nil, 0, "ai.request_encode_failed", err
	}

	reqCtx, cancel := context.WithTimeout(ctx, runtimeAutoHealRequestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/internal/v1/ai/tools/auto-heal", bytes.NewReader(reqBody))
	if err != nil {
		opErr = err
		return nil, 0, "ai.request_build_failed", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	if signingKey := strings.TrimSpace(r.aiActorSigningKey); signingKey != "" {
		now := time.Now().UTC()
		actor := aiagent.ActorContext{
			UserID:       actorUser.ID,
			SessionID:    "auto-heal:" + actorUser.ID,
			PlatformRole: actorUser.Role,
			ProjectID:    projectID,
			Locale:       firstNonEmpty(strings.TrimSpace(actorUser.Language), "zh-CN"),
			RequestID:    id.New("req"),
			IssuedAt:     now.Unix(),
			ExpiresAt:    now.Add(time.Minute).Unix(),
		}
		if encoded, sig, signErr := signInternalActorHeader(actor, []byte(signingKey)); signErr == nil {
			httpReq.Header.Set("X-Luna-Actor-Context", encoded)
			httpReq.Header.Set("X-Luna-Actor-Signature", sig)
		}
	}

	client := r.internalAutoHealHTTPClient
	if client == nil {
		client = &http.Client{
			Timeout:   runtimeAutoHealRequestTimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		opErr = err
		return nil, 0, "ai.auto_heal_unreachable", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody internalAutoHealErrorBody
		_ = json.Unmarshal(bodyBytes, &errBody)
		code := firstNonEmpty(strings.TrimSpace(errBody.Code), "ai.auto_heal_failed")
		msg := firstNonEmpty(strings.TrimSpace(errBody.Message), fmt.Sprintf("auto-heal returned HTTP %d", resp.StatusCode))
		opErr = errors.New(msg)
		return nil, resp.StatusCode, code, opErr
	}

	var out internalAutoHealResponse
	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		opErr = err
		return nil, resp.StatusCode, "ai.auto_heal_decode_failed", err
	}
	return &out, resp.StatusCode, "", nil
}

func signInternalActorHeader(actor aiagent.ActorContext, key []byte) (string, string, error) {
	payload, err := json.Marshal(actor)
	if err != nil {
		return "", "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(encoded))
	return encoded, "sha256=" + hex.EncodeToString(mac.Sum(nil)), nil
}

func (r *Runner) failRuntimeDiagnosisRow(ctx context.Context, db *gorm.DB, diagnosisID string, createdAt time.Time, errorCode string, cause error) {
	finishedAt := maxTime(time.Now().UTC(), createdAt)
	_ = db.Model(&model.AIRuntimeIncidentDiagnosis{}).
		Where("id = ? AND status = ?", diagnosisID, model.AIDiagnosisStatusRunning).
		Updates(map[string]any{
			"status":      model.AIDiagnosisStatusFailed,
			"error_code":  strings.TrimSpace(errorCode),
			"finished_at": &finishedAt,
			"updated_at":  finishedAt,
		}).Error
	telemetry.LogWarn(ctx, "runtime incident AI diagnosis failed", "worker.ai_runtime_diagnosis.failed", "ai_automation.execute_runtime_diagnosis", firstNonEmpty(errorCode, "ai.failed"), cause,
		slog.String("resource.type", "ai_runtime_incident_diagnosis"),
		slog.String("resource.id", diagnosisID),
	)
}

func workerMatchesApp(candidate string, app model.Application, fallbackAppID string) bool {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return true
	}
	return trimmed == app.ID ||
		trimmed == fallbackAppID ||
		(app.Identifier != "" && strings.EqualFold(trimmed, app.Identifier)) ||
		(app.Name != "" && strings.EqualFold(trimmed, app.Name))
}

func workerMatchesTarget(candidate string, target model.DeploymentTarget) bool {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return true
	}
	return trimmed == target.ID ||
		(target.Stage != "" && strings.EqualFold(trimmed, target.Stage)) ||
		(target.Name != "" && strings.EqualFold(trimmed, target.Name)) ||
		(target.KubernetesName != "" && strings.EqualFold(trimmed, target.KubernetesName))
}

func workerPositiveIntArg(raw any) bool {
	switch v := raw.(type) {
	case float64:
		return math.Trunc(v) == v && v >= 1 && v <= 1000
	case int:
		return v >= 1 && v <= 1000
	case int64:
		return v >= 1 && v <= 1000
	default:
		return false
	}
}

func isSafeWorkerRuntimeHealArguments(toolName string, args map[string]any) bool {
	switch strings.TrimSpace(toolName) {
	case "delivery_scale_target":
		return workerPositiveIntArg(args["replicas"])
	case "delivery_deploy_target":
		if rawImg, exists := args["imageRef"]; exists && rawImg != nil {
			if s, ok := rawImg.(string); !ok || strings.TrimSpace(s) != "" {
				return false
			}
		}
		return true
	case "delivery_update_target_runtime":
		allowedKeys := map[string]struct{}{
			"project":       {},
			"application":   {},
			"target":        {},
			"cpuRequest":    {},
			"memoryRequest": {},
			"autoScaling":   {},
		}
		for k, v := range args {
			if v == nil {
				continue
			}
			if _, ok := allowedKeys[k]; !ok {
				return false
			}
		}
		if rawAuto, exists := args["autoScaling"]; exists && rawAuto != nil {
			autoMap, ok := rawAuto.(map[string]any)
			if !ok {
				return false
			}
			allowedAutoKeys := map[string]struct{}{
				"enabled":       {},
				"minReplicas":   {},
				"maxReplicas":   {},
				"cpuPercent":    {},
				"memoryPercent": {},
			}
			for k, v := range autoMap {
				if v == nil {
					continue
				}
				if _, ok := allowedAutoKeys[k]; !ok {
					return false
				}
			}
			if minRep, exists := autoMap["minReplicas"]; exists && minRep != nil && !workerPositiveIntArg(minRep) {
				return false
			}
			if maxRep, exists := autoMap["maxReplicas"]; exists && maxRep != nil && !workerPositiveIntArg(maxRep) {
				return false
			}
		}
		return true
	default:
		return true
	}
}

func sanitizeRuntimeHealAction(
	raw *model.AIProposedAction,
	projectID string,
	app model.Application,
	target model.DeploymentTarget,
) (*model.AIProposedAction, string) {
	if raw == nil {
		return nil, ""
	}
	toolName := strings.TrimSpace(raw.ToolName)
	if !model.IsAllowedAIRuntimeHealTool(toolName) {
		return nil, ""
	}
	redactedArgs, _ := redactJSONValue(raw.Arguments, 0).(map[string]any)
	if redactedArgs == nil {
		redactedArgs = map[string]any{}
	}
	if rawProj, exists := redactedArgs["project"]; exists && rawProj != nil {
		if projStr, ok := rawProj.(string); !ok || (strings.TrimSpace(projStr) != "" && strings.TrimSpace(projStr) != strings.TrimSpace(projectID)) {
			return nil, ""
		}
	}
	if rawApp, exists := redactedArgs["application"]; exists && rawApp != nil {
		appStr, ok := rawApp.(string)
		if !ok || !workerMatchesApp(appStr, app, target.ApplicationID) {
			return nil, ""
		}
	}
	if rawTgt, exists := redactedArgs["target"]; exists && rawTgt != nil {
		tgtStr, ok := rawTgt.(string)
		if !ok || !workerMatchesTarget(tgtStr, target) {
			return nil, ""
		}
	}
	redactedArgs["project"] = strings.TrimSpace(projectID)
	redactedArgs["application"] = strings.TrimSpace(firstNonEmpty(app.ID, target.ApplicationID))
	redactedArgs["target"] = strings.TrimSpace(target.ID)

	if !isSafeWorkerRuntimeHealArguments(toolName, redactedArgs) {
		return nil, ""
	}

	sanitized := model.AIProposedAction{
		ToolName:  toolName,
		Arguments: redactedArgs,
		Summary:   telemetry.RedactText(strings.TrimSpace(raw.Summary)),
	}
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		return nil, ""
	}
	return &sanitized, string(encoded)
}

func buildRuntimeDiagnosisContextPayload(
	projectID string,
	incident model.RuntimeAlertIncident,
	target model.DeploymentTarget,
	observation kubeprovider.WorkloadRuntimeObservation,
	anomaly runtimeDetectedAnomaly,
	recentReleases []model.Release,
) map[string]any {
	pods := make([]map[string]any, 0, min(len(observation.Pods), 6))
	for i := 0; i < len(observation.Pods) && i < 6; i++ {
		p := observation.Pods[i]
		containers := make([]map[string]any, 0, len(p.ContainerStatuses))
		for _, c := range p.ContainerStatuses {
			cMap := map[string]any{
				"name":         c.Name,
				"image":        c.Image,
				"ready":        c.Ready,
				"restartCount": c.RestartCount,
				"state":        c.CurrentState.State,
				"reason":       c.CurrentState.Reason,
				"message":      telemetry.RedactText(c.CurrentState.Message),
			}
			if c.LastTermination != nil {
				cMap["lastTermination"] = map[string]any{
					"reason":   c.LastTermination.Reason,
					"exitCode": c.LastTermination.ExitCode,
					"message":  telemetry.RedactText(c.LastTermination.Message),
				}
			}
			containers = append(containers, cMap)
		}
		pods = append(pods, map[string]any{
			"name":       p.Name,
			"phase":      p.Phase,
			"status":     p.Status,
			"ready":      p.Ready,
			"restarts":   p.RestartCount,
			"reason":     p.Reason,
			"message":    telemetry.RedactText(p.Message),
			"containers": containers,
		})
	}

	releases := make([]map[string]any, 0, len(recentReleases))
	for _, rel := range recentReleases {
		releases = append(releases, map[string]any{
			"id":          rel.ID,
			"revision":    rel.Revision,
			"type":        rel.Type,
			"status":      rel.Status,
			"failureCode": rel.FailureCode,
			"imageRef":    rel.ImageRef,
			"message":     telemetry.RedactText(rel.Message),
			"createdAt":   rel.CreatedAt,
		})
	}

	return map[string]any{
		"projectId":          projectID,
		"applicationId":      target.ApplicationID,
		"deploymentTargetId": target.ID,
		"incident": map[string]any{
			"id":                  incident.ID,
			"kind":                anomaly.EventType,
			"consecutiveFailures": incident.ConsecutiveFailures,
			"podName":             anomaly.PodName,
			"containerName":       anomaly.ContainerName,
			"reason":              anomaly.Reason,
			"exitCode":            anomaly.ExitCode,
			"restartCount":        anomaly.RestartCount,
			"oomKilled":           anomaly.OOMKilled,
			"readyReplicas":       anomaly.ReadyReplicas,
			"desiredReplicas":     anomaly.DesiredReplicas,
			"summary":             telemetry.RedactText(anomaly.Summary),
			"diagnosticDetail":    telemetry.RedactText(anomaly.DiagnosticDetail),
		},
		"deploymentTarget": sanitizeTargetSnapshotForAI(target),
		"workload": map[string]any{
			"phase":             observation.Workload.Phase,
			"readyReplicas":     observation.Workload.ReadyReplicas,
			"desiredReplicas":   observation.Workload.DesiredReplicas,
			"availableReplicas": observation.Workload.AvailableReplicas,
			"releaseId":         observation.Workload.ReleaseID,
			"message":           telemetry.RedactText(observation.Workload.Message),
		},
		"pods":           pods,
		"recentReleases": releases,
	}
}

func (r *Runner) publishRuntimeAnomalyInboxWithAI(
	ctx context.Context,
	decision runtimeAlertDecision,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	anomaly runtimeDetectedAnomaly,
	extraParams map[string]any,
	proposal *runtimeInboxSelfHealProposal,
	requesterUserID string,
) {
	r.publishRuntimeAnomalyInboxWithOptions(ctx, decision, target, project, application, anomaly, extraParams, proposal, requesterUserID, false)
}

func (r *Runner) publishRuntimeAnomalyInboxWithOptions(
	ctx context.Context,
	decision runtimeAlertDecision,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	anomaly runtimeDetectedAnomaly,
	extraParams map[string]any,
	proposal *runtimeInboxSelfHealProposal,
	requesterUserID string,
	clearActionRequest bool,
) {
	if r == nil || r.db == nil {
		return
	}
	db := r.db.WithContext(ctx)

	// Preserve existing AI diagnosis fields across subsequent reminder patrols when extraParams is nil.
	if len(extraParams) == 0 && strings.TrimSpace(decision.Incident.ID) != "" {
		var latestDiag model.AIRuntimeIncidentDiagnosis
		if err := db.Where("incident_id = ? AND status = ?", decision.Incident.ID, model.AIDiagnosisStatusSucceeded).
			Order("created_at desc").
			First(&latestDiag).Error; err == nil {
			extraParams = extraInboxParamsFromDiagnosisRow(latestDiag)
		}
	}

	priority := inbox.PriorityHigh
	if anomaly.EventType == "runtime.pod_oom_killed" || anomaly.EventType == "runtime.pod_crash_loop" {
		priority = inbox.PriorityCritical
	}
	if cb, ok := extraParams["aiCircuitBreakerTripped"].(bool); ok && cb {
		priority = inbox.PriorityCritical
	}

	titleKey := "inbox.runtime.degraded.title"
	switch anomaly.EventType {
	case "runtime.pod_oom_killed":
		titleKey = "inbox.runtime.oomKilled.title"
	case "runtime.pod_crash_loop":
		titleKey = "inbox.runtime.crashLoop.title"
	case "runtime.pod_start_failed":
		titleKey = "inbox.runtime.startFailed.title"
	}

	params := map[string]any{
		"projectName":     firstNonEmpty(project.Name, project.Identifier),
		"applicationName": firstNonEmpty(application.Name, application.Identifier),
		"targetName":      firstNonEmpty(target.Name, target.Stage),
		"podName":         anomaly.PodName,
		"reason":          anomaly.Reason,
		"restartCount":    anomaly.RestartCount,
		"readyReplicas":   anomaly.ReadyReplicas,
		"desiredReplicas": anomaly.DesiredReplicas,
		"occurrenceCount": decision.Incident.ConsecutiveFailures,
	}
	for k, v := range extraParams {
		params[k] = v
	}

	coalesceKey := firstNonEmpty(decision.Incident.InboxCoalesceKey, "runtime_incident:"+decision.Incident.ID)
	baseInput := inbox.PublishInput{
		Type:               anomaly.EventType,
		Category:           inbox.CategoryDelivery,
		Priority:           priority,
		TitleKey:           titleKey,
		ContentKey:         "inbox.runtime.anomaly.body",
		Params:             params,
		ClearActionRequest: clearActionRequest,
		ProjectID:          target.ProjectID,
		ResourceType:       "deployment_target",
		ResourceID:         target.ID,
		DeepLink:           runtimeTargetActionLink(target.ProjectID, target.ApplicationID, target.ID),
		DedupKey:           coalesceKey,
	}

	svc := inbox.NewService(r.db)
	roles := []string{authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper}
	if proposal == nil {
		_ = svc.PublishCoalescedToProjectRoles(ctx, target.ProjectID, roles, baseInput, decision.MarkInboxUnread)
		return
	}

	recipientIDs := r.runtimeSelfHealRecipientUserIDs(db, target.ProjectID, roles, requesterUserID)
	expiresAt := time.Now().UTC().Add(runtimeSelfHealProposalTTL)
	requester := firstNonEmpty(strings.TrimSpace(requesterUserID), recipientIDsFirst(recipientIDs))

	for _, uid := range recipientIDs {
		actionReqID := r.ensureMemberRuntimeSelfHealActionRequest(
			ctx,
			db,
			svc,
			target.ProjectID,
			requester,
			uid,
			*proposal,
			expiresAt,
		)
		memberInput := baseInput
		memberInput.RecipientUserID = uid
		if baseDedup := strings.TrimSpace(baseInput.DedupKey); baseDedup != "" {
			memberInput.DedupKey = uid + ":" + baseDedup
		}
		if actionReqID != "" {
			memberInput.ActionRequestID = actionReqID
			memberInput.Category = inbox.CategoryAction
		}
		_, _, _ = svc.PublishOrUpdate(ctx, memberInput, decision.MarkInboxUnread)
	}
}

func recipientIDsFirst(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func (r *Runner) runtimeSelfHealRecipientUserIDs(db *gorm.DB, projectID string, roles []string, extraUserID string) []string {
	var members []model.ProjectMember
	_ = db.Select("user_id").Where("project_id = ? AND role IN ?", projectID, roles).Find(&members).Error
	seen := make(map[string]struct{}, len(members)+1)
	out := make([]string, 0, len(members)+1)
	for _, m := range members {
		uid := strings.TrimSpace(m.UserID)
		if uid == "" {
			continue
		}
		if _, exists := seen[uid]; !exists {
			seen[uid] = struct{}{}
			out = append(out, uid)
		}
	}
	if extra := strings.TrimSpace(extraUserID); extra != "" {
		if _, exists := seen[extra]; !exists {
			var u model.User
			if err := db.Select("id").First(&u, "id = ? AND disabled = ?", extra, false).Error; err == nil {
				seen[extra] = struct{}{}
				out = append(out, extra)
			}
		}
	}
	return out
}

func (r *Runner) ensureMemberRuntimeSelfHealActionRequest(
	ctx context.Context,
	db *gorm.DB,
	svc *inbox.Service,
	projectID string,
	requesterUserID string,
	recipientUserID string,
	proposal runtimeInboxSelfHealProposal,
	expiresAt time.Time,
) string {
	payload := map[string]any{
		"incidentId":              proposal.IncidentID,
		"applicationId":           proposal.ApplicationID,
		"deploymentTargetId":      proposal.DeploymentTargetID,
		"toolName":                proposal.ToolName,
		"arguments":               proposal.Arguments,
		"expectedSummary":         proposal.ExpectedSummary,
		"baselineReleaseId":       proposal.BaselineReleaseID,
		"baselineTargetUpdatedAt": proposal.BaselineTargetUpdatedAt,
		"createdAt":               proposal.CreatedAt,
	}
	var existing model.InboxActionRequest
	if err := db.Where(
		"recipient_user_id = ? AND project_id = ? AND type = ? AND status IN ? AND resource_id = ?",
		recipientUserID,
		projectID,
		inbox.ActionRequestTypeRuntimeSelfHeal,
		[]string{inbox.ActionRequestStatusPending, inbox.ActionRequestStatusProcessing},
		proposal.DeploymentTargetID,
	).First(&existing).Error; err == nil && existing.ID != "" {
		var p map[string]any
		if json.Unmarshal([]byte(existing.PayloadJSON), &p) == nil {
			if incID, _ := p["incidentId"].(string); strings.TrimSpace(incID) == strings.TrimSpace(proposal.IncidentID) {
				return existing.ID
			}
		}
		if existing.Status == inbox.ActionRequestStatusPending {
			encodedPayload, _ := json.Marshal(payload)
			_ = db.Model(&model.InboxActionRequest{}).
				Where("id = ? AND status = ?", existing.ID, inbox.ActionRequestStatusPending).
				Updates(map[string]any{
					"payload_json": string(encodedPayload),
					"expires_at":   &expiresAt,
					"row_version":  existing.RowVersion + 1,
					"updated_at":   time.Now().UTC(),
				}).Error
		}
		return existing.ID
	}

	reqRow, err := svc.CreateActionRequest(ctx, inbox.CreateActionRequestInput{
		Type:            inbox.ActionRequestTypeRuntimeSelfHeal,
		RequesterUserID: firstNonEmpty(requesterUserID, recipientUserID),
		RecipientUserID: recipientUserID,
		ProjectID:       projectID,
		ResourceType:    "deployment_target",
		ResourceID:      proposal.DeploymentTargetID,
		Payload:         payload,
		ExpiresAt:       &expiresAt,
	})
	if err != nil {
		return ""
	}
	return reqRow.ID
}

func (r *Runner) expirePendingRuntimeSelfHealRequests(ctx context.Context, projectID, incidentID string, now time.Time) {
	if r == nil || r.db == nil || strings.TrimSpace(incidentID) == "" {
		return
	}
	db := r.db.WithContext(ctx)
	var pending []model.InboxActionRequest
	if err := db.Where(
		"project_id = ? AND type = ? AND status = ?",
		projectID,
		inbox.ActionRequestTypeRuntimeSelfHeal,
		inbox.ActionRequestStatusPending,
	).Find(&pending).Error; err != nil {
		return
	}
	for _, row := range pending {
		var p map[string]any
		if json.Unmarshal([]byte(row.PayloadJSON), &p) == nil {
			if incID, _ := p["incidentId"].(string); strings.TrimSpace(incID) == strings.TrimSpace(incidentID) {
				_ = db.Model(&model.InboxActionRequest{}).
					Where("id = ? AND status = ?", row.ID, inbox.ActionRequestStatusPending).
					Updates(map[string]any{
						"status":      inbox.ActionRequestStatusExpired,
						"row_version": row.RowVersion + 1,
						"updated_at":  now.UTC(),
					}).Error
			}
		}
	}
}

func extraInboxParamsFromDiagnosisRow(row model.AIRuntimeIncidentDiagnosis) map[string]any {
	var suggestions []string
	_ = json.Unmarshal([]byte(row.SuggestionsJSON), &suggestions)
	if suggestions == nil {
		suggestions = []string{}
	}
	params := map[string]any{
		"aiRootCauseSummary":      row.RootCauseSummary,
		"aiConfidence":            row.Confidence,
		"aiSuggestions":           suggestions,
		"aiCircuitBreakerTripped": row.CircuitBreakerTripped,
	}
	if strings.TrimSpace(row.AutoHealSummary) != "" {
		params["aiAutoHealSummary"] = row.AutoHealSummary
		var action model.AIProposedAction
		if json.Unmarshal([]byte(row.ProposedActionJSON), &action) == nil && action.ToolName != "" {
			params["aiAutoHealTool"] = action.ToolName
		}
	}
	return params
}

func (r *Runner) reconcileAIRuntimeDiagnoses(ctx context.Context) error {
	if r == nil || r.db == nil {
		return nil
	}
	ctx, end := telemetry.StartOperation(ctx, "ai_automation", "reconcile_runtime_diagnoses")
	var retErr error
	defer func() { end(retErr) }()

	now := time.Now().UTC()
	staleCutoff := now.Add(-runtimeDiagnosisStaleRunningAge)
	db := r.db.WithContext(ctx)

	if err := db.Model(&model.AIRuntimeIncidentDiagnosis{}).
		Where("status = ? AND created_at < ?", model.AIDiagnosisStatusRunning, staleCutoff).
		Updates(map[string]any{
			"status":          model.AIDiagnosisStatusFailed,
			"error_code":      "ai.timeout",
			"finished_at":     &now,
			"inbox_synced_at": &now,
			"updated_at":      now,
		}).Error; err != nil {
		retErr = err
		return err
	}

	var unsynced []model.AIRuntimeIncidentDiagnosis
	if err := db.Where("status = ? AND inbox_synced_at IS NULL", model.AIDiagnosisStatusSucceeded).
		Order("created_at asc").
		Limit(runtimeDiagnosisReconcileBatchSize).
		Find(&unsynced).Error; err != nil {
		retErr = err
		return err
	}

	for _, row := range unsynced {
		var inc model.RuntimeAlertIncident
		if err := db.First(&inc, "id = ? AND project_id = ?", row.IncidentID, row.ProjectID).Error; err != nil || inc.Status != "active" {
			_ = db.Model(&model.AIRuntimeIncidentDiagnosis{}).
				Where("id = ?", row.ID).
				Update("inbox_synced_at", &now).Error
			continue
		}
		var project model.Project
		var app model.Application
		var target model.DeploymentTarget
		if db.First(&project, "id = ?", row.ProjectID).Error != nil ||
			db.First(&app, "id = ?", row.ApplicationID).Error != nil ||
			db.First(&target, "id = ?", row.DeploymentTargetID).Error != nil {
			_ = db.Model(&model.AIRuntimeIncidentDiagnosis{}).
				Where("id = ?", row.ID).
				Update("inbox_synced_at", &now).Error
			continue
		}
		var anomaly runtimeDetectedAnomaly
		_ = json.Unmarshal([]byte(inc.SummaryJSON), &anomaly)
		if anomaly.EventType == "" {
			anomaly.EventType = inc.IncidentKind
		}

		var proposal *runtimeInboxSelfHealProposal
		if row.AutoHealStatus != model.AIAutoHealStatusSucceeded && strings.TrimSpace(row.ProposedActionJSON) != "" {
			var action model.AIProposedAction
			if json.Unmarshal([]byte(row.ProposedActionJSON), &action) == nil {
				sanitized, _ := sanitizeRuntimeHealAction(&action, row.ProjectID, app, target)
				if sanitized != nil {
					var baseRel model.Release
					baseRelID := ""
					if err := db.Where("project_id = ? AND deployment_target_id = ? AND created_at <= ?",
						row.ProjectID, target.ID, row.CreatedAt).
						Order("created_at desc").
						First(&baseRel).Error; err == nil {
						baseRelID = baseRel.ID
					}
					targetUpdatedAt := ""
					if !target.UpdatedAt.IsZero() {
						if target.UpdatedAt.After(row.CreatedAt) {
							targetUpdatedAt = row.CreatedAt.UTC().Format(time.RFC3339Nano)
						} else {
							targetUpdatedAt = target.UpdatedAt.UTC().Format(time.RFC3339Nano)
						}
					}
					proposal = &runtimeInboxSelfHealProposal{
						IncidentID:              inc.ID,
						ApplicationID:           target.ApplicationID,
						DeploymentTargetID:      target.ID,
						ToolName:                sanitized.ToolName,
						Arguments:               sanitized.Arguments,
						ExpectedSummary:         firstNonEmpty(sanitized.Summary, sanitized.ToolName),
						BaselineReleaseID:       baseRelID,
						BaselineTargetUpdatedAt: targetUpdatedAt,
						CreatedAt:               row.CreatedAt.UTC().Format(time.RFC3339),
					}
				}
			}
		}

		r.publishRuntimeAnomalyInboxWithOptions(
			ctx,
			runtimeAlertDecision{
				Incident:          inc,
				ShouldUpdateInbox: true,
				MarkInboxUnread:   true,
			},
			target,
			project,
			applicationOrEmpty(app),
			anomaly,
			extraInboxParamsFromDiagnosisRow(row),
			proposal,
			row.BilledUserID,
			row.AutoHealStatus == model.AIAutoHealStatusSucceeded,
		)
		_ = db.Model(&model.AIRuntimeIncidentDiagnosis{}).
			Where("id = ?", row.ID).
			Update("inbox_synced_at", &now).Error
	}
	return nil
}

func applicationOrEmpty(app model.Application) model.Application {
	return app
}
