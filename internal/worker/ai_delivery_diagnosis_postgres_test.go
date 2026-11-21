package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type recordingAIDiagEnqueuer struct {
	payloads []tasks.AIDeliveryDiagnosisPayload
}

func (e *recordingAIDiagEnqueuer) EnqueueAIDeliveryDiagnosis(_ context.Context, payload tasks.AIDeliveryDiagnosisPayload) (*asynq.TaskInfo, error) {
	e.payloads = append(e.payloads, payload)
	return &asynq.TaskInfo{ID: "task_" + payload.DiagnosisID}, nil
}

func TestDeliveryFailureAIDiagnosisBillingCooldownAndNotificationPostgres(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "luna_ai_delivery_diag",
		Migrate: func(db *gorm.DB) error {
			return database.MigrateContext(context.Background(), db)
		},
	})

	if err := db.Create(&model.AppConfig{Key: "ai.assistant.enabled", Value: "true"}).Error; err != nil {
		t.Fatal(err)
	}
	owner := model.User{ID: "usr_rule_owner", Email: "owner@example.test", Name: "Owner", Role: authz.PlatformRoleUser, Language: "zh-CN"}
	triggerUser := model.User{ID: "usr_trigger", Email: "dev@example.test", Name: "Dev", Role: authz.PlatformRoleUser, Language: "zh-CN"}
	gitAccountOwner := model.User{ID: "usr_git_owner", Email: "git@example.test", Name: "GitOwner", Role: authz.PlatformRoleUser, Language: "zh-CN"}
	brokeUser := model.User{ID: "usr_broke", Email: "broke@example.test", Name: "Broke", Role: authz.PlatformRoleUser, Language: "zh-CN"}
	for _, u := range []model.User{owner, triggerUser, gitAccountOwner, brokeUser} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.UserWallet{ID: "wlt_owner", UserID: owner.ID, BalanceCredits: decimal.NewFromInt(50)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserWallet{ID: "wlt_trigger", UserID: triggerUser.ID, BalanceCredits: decimal.NewFromInt(20)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserWallet{ID: "wlt_git_owner", UserID: gitAccountOwner.ID, BalanceCredits: decimal.NewFromInt(30)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserWallet{ID: "wlt_broke", UserID: brokeUser.ID, BalanceCredits: decimal.Zero}).Error; err != nil {
		t.Fatal(err)
	}

	proj := model.Project{ID: "prj_diag", Name: "Diag", Identifier: "diag", DeleteStatus: "active"}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	for _, uid := range []string{owner.ID, triggerUser.ID, gitAccountOwner.ID, brokeUser.ID} {
		if err := db.Create(&model.ProjectMember{ID: "pm_" + uid, ProjectID: proj.ID, UserID: uid, Role: authz.ProjectRoleDeveloper}).Error; err != nil {
			t.Fatal(err)
		}
	}
	app := model.Application{ID: "app_diag", ProjectID: proj.ID, Name: "API", Identifier: "api", DeleteStatus: "active"}
	if err := db.Create(&app).Error; err != nil {
		t.Fatal(err)
	}
	target := model.DeploymentTarget{
		ID: "dt_diag", ProjectID: proj.ID, ApplicationID: app.ID, Name: "Prod", Stage: "prod",
		KubernetesName: "api-prod", Replicas: 1, DeleteStatus: "active",
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ProjectAIAutomationConfig{
		ProjectID:                proj.ID,
		DeliveryDiagnosisEnabled: true,
		DeliveryBillingUserID:    owner.ID,
		DeliveryCooldownSeconds:  60,
		DeliveryDailyLimit:       3,
	}).Error; err != nil {
		t.Fatal(err)
	}

	var capturedActorIDs []string
	agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aiagent.HeadlessInferenceRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		capturedActorIDs = append(capturedActorIDs, req.ActorUserID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"runId":     "airun_diag_1",
			"taskType":  req.TaskType,
			"modelId":   "aimod_1",
			"modelName": "gemini-test",
			"output": map[string]any{
				"rootCauseSummary": "Dockerfile 指定的 pnpm-lock.yaml 未复制到镜像工作目录。",
				"confidence":       "high",
				"suggestions":      []string{"在 RUN pnpm install --frozen-lockfile 前添加 COPY pnpm-lock.yaml ./"},
				"proposedAction": map[string]any{
					"toolName":  "build_configure_target_pipeline",
					"arguments": map[string]any{"project": proj.ID, "application": app.ID, "target": target.ID, "buildContext": "."},
					"summary":   "修正部署目标构建上下文为仓库根目录",
				},
			},
			"usage": map[string]any{"inputTokens": 80, "outputTokens": 40, "totalTokens": 120},
		})
	}))
	defer agentSrv.Close()

	aiClient, err := aiagent.NewHTTPClient(agentSrv.URL, "svc-token-32-bytes-minimum-test!", "actor-key-32-bytes-minimum-test!")
	if err != nil {
		t.Fatal(err)
	}

	enq := &recordingAIDiagEnqueuer{}
	runner := &Runner{
		db:                           db,
		aiAgent:                      aiClient,
		aiAgentAvailable:             true,
		aiDeliveryDiagnosisEnqueuer:  enq,
		notificationDeliveryEnqueuer: &noopNotificationDeliveryEnqueuer{},
		publicBaseURL:                "https://devops.example.test",
	}

	// 1. User-canceled build does NOT enqueue diagnosis, but DOES emit notification event immediately.
	canceledRun := model.BuildRun{
		ID: "brun_canceled", ProjectID: proj.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		Status: "canceled", FailureCode: model.BuildFailureCanceledByUser, TriggerType: "manual", CreatedBy: triggerUser.ID,
	}
	if err := db.Create(&canceledRun).Error; err != nil {
		t.Fatal(err)
	}
	runner.onBuildFailureTerminal(t.Context(), canceledRun, "Canceled by user")
	if len(enq.payloads) != 0 {
		t.Fatalf("expected 0 enqueued diagnoses for canceled build, got %d", len(enq.payloads))
	}

	// 2. Manual failed build with CreatedBy = triggerUser bills triggerUser's personal wallet, executes async diagnosis, and attaches AIDiagnosis to PlatformEvent!
	now := time.Now().UTC()
	failedRun := model.BuildRun{
		ID: "brun_fail_1", ProjectID: proj.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		Status: "failed", FailureCode: model.BuildFailureJobFailed, TriggerType: "manual", CreatedBy: triggerUser.ID, FinishedAt: &now,
	}
	if err := db.Create(&failedRun).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.BuildLog{
		ID: "blog_1", ProjectID: proj.ID, BuildRunID: failedRun.ID, BuildJobID: "bjob_1",
		Content: "ERR_PNPM_NO_LOCKFILE Cannot install with frozen-lockfile",
	}).Error; err != nil {
		t.Fatal(err)
	}

	runner.onBuildFailureTerminal(t.Context(), failedRun, "Build failed")
	if len(enq.payloads) != 1 {
		t.Fatalf("expected 1 enqueued diagnosis, got %d", len(enq.payloads))
	}

	task, err := tasks.NewAIDeliveryDiagnosisTask(enq.payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.handleAIDeliveryDiagnosis(t.Context(), task); err != nil {
		t.Fatalf("handleAIDeliveryDiagnosis error: %v", err)
	}

	if len(capturedActorIDs) != 1 || capturedActorIDs[0] != triggerUser.ID {
		t.Fatalf("expected manual triggerUser (%s) to be billed first, got %v", triggerUser.ID, capturedActorIDs)
	}

	var savedDiag model.AIDeliveryDiagnosis
	if err := db.First(&savedDiag, "id = ?", enq.payloads[0].DiagnosisID).Error; err != nil {
		t.Fatal(err)
	}
	view := savedDiag.ToView()
	if view.Status != model.AIDiagnosisStatusSucceeded ||
		!strings.Contains(view.RootCauseSummary, "pnpm-lock.yaml") ||
		view.ProposedAction == nil ||
		view.ProposedAction.ToolName != "build_configure_target_pipeline" ||
		savedDiag.NotificationEmittedAt == nil {
		t.Fatalf("unexpected saved diagnosis view: %+v (emittedAt=%v)", view, savedDiag.NotificationEmittedAt)
	}

	var evt model.PlatformEvent
	if err := db.Where("correlation_id = ? AND type = ?", failedRun.ID, "build.failed").First(&evt).Error; err != nil {
		t.Fatalf("expected build.failed platform_event after diagnosis, err=%v", err)
	}
	if !strings.Contains(evt.DetailJSON, "pnpm-lock.yaml") {
		t.Fatalf("expected build.failed notification payload to contain AI diagnosis summary, got: %s", evt.DetailJSON)
	}

	// 3. Second failure within 60s cooldown skips diagnosis and immediately emits base release.failed notification!
	failedRelease := model.Release{
		ID: "rel_cooldown", ProjectID: proj.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		ImageRef: "img:v1", Status: "failed", FailureCode: model.ReleaseFailureRolloutFailed, CreatedBy: "",
	}
	if err := db.Create(&failedRelease).Error; err != nil {
		t.Fatal(err)
	}
	runner.onReleaseFailureTerminal(t.Context(), failedRelease, "Rollout failed")
	if len(enq.payloads) != 1 {
		t.Fatalf("expected cooldown to skip second diagnosis enqueue, got %d", len(enq.payloads))
	}
	var relEvt model.PlatformEvent
	if err := db.Where("correlation_id = ? AND type = ?", failedRelease.ID, "release.failed").First(&relEvt).Error; err != nil {
		t.Fatalf("expected immediate release.failed notification when skipped by cooldown: %v", err)
	}

	// 4. Webhook build (TriggerType = "webhook", CreatedBy = gitAccountOwner) MUST NOT drain gitAccountOwner's wallet; bills rule owner instead!
	if err := db.Model(&model.AIDeliveryDiagnosis{}).Where("id = ?", savedDiag.ID).
		Update("created_at", time.Now().UTC().Add(-2*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	webhookRun := model.BuildRun{
		ID: "brun_webhook", ProjectID: proj.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		Status: "failed", FailureCode: model.BuildFailureJobFailed, TriggerType: "webhook", CreatedBy: gitAccountOwner.ID, FinishedAt: &now,
	}
	if err := db.Create(&webhookRun).Error; err != nil {
		t.Fatal(err)
	}
	runner.onBuildFailureTerminal(t.Context(), webhookRun, "Webhook build failed")
	if len(enq.payloads) != 2 {
		t.Fatalf("expected 2nd diagnosis after cooldown elapsed, got %d", len(enq.payloads))
	}
	task2, _ := tasks.NewAIDeliveryDiagnosisTask(enq.payloads[1])
	if err := runner.handleAIDeliveryDiagnosis(t.Context(), task2); err != nil {
		t.Fatal(err)
	}
	if len(capturedActorIDs) != 2 || capturedActorIDs[1] != owner.ID {
		t.Fatalf("expected webhook build to bill bound rule owner (%s) rather than gitAccountOwner (%s), got %v", owner.ID, gitAccountOwner.ID, capturedActorIDs)
	}

	// 5. Manual release triggered by brokeUser ($0 balance) falls back cleanly to rule owner wallet!
	if err := db.Model(&model.AIDeliveryDiagnosis{}).Where("project_id = ?", proj.ID).
		Update("created_at", time.Now().UTC().Add(-5*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	brokeRelease := model.Release{
		ID: "rel_broke_fallback", ProjectID: proj.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		ImageRef: "img:v2", Status: "failed", FailureCode: model.ReleaseFailureRolloutFailed, CreatedBy: brokeUser.ID, Message: "Rollout failed",
	}
	if err := db.Create(&brokeRelease).Error; err != nil {
		t.Fatal(err)
	}
	runner.onReleaseFailureTerminal(t.Context(), brokeRelease, "Rollout failed")
	if len(enq.payloads) != 3 {
		t.Fatalf("expected brokeUser ($0) to fall back to rule owner and enqueue 3rd diagnosis, got %d", len(enq.payloads))
	}
	task3, _ := tasks.NewAIDeliveryDiagnosisTask(enq.payloads[2])
	if err := runner.handleAIDeliveryDiagnosis(t.Context(), task3); err != nil {
		t.Fatal(err)
	}
	if len(capturedActorIDs) != 3 || capturedActorIDs[2] != owner.ID {
		t.Fatalf("expected brokeUser ($0) fallback to bill rule owner (%s), got %v", owner.ID, capturedActorIDs)
	}

	// 6. Fourth failure hits daily_limit = 3 -> skipped!
	if err := db.Model(&model.AIDeliveryDiagnosis{}).Where("project_id = ?", proj.ID).
		Update("created_at", time.Now().UTC().Add(-10*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	dailyCapRelease := model.Release{
		ID: "rel_daily_cap", ProjectID: proj.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		ImageRef: "img:v3", Status: "failed", FailureCode: model.ReleaseFailureRolloutFailed, CreatedBy: triggerUser.ID,
	}
	if err := db.Create(&dailyCapRelease).Error; err != nil {
		t.Fatal(err)
	}
	runner.onReleaseFailureTerminal(t.Context(), dailyCapRelease, "Rollout failed")
	if len(enq.payloads) != 3 {
		t.Fatalf("expected daily limit=3 to block 4th diagnosis today, got %d", len(enq.payloads))
	}

	// 7. Reconciler sweeps stale running diagnosis (>3m) -> marks failed + emits fallback notification!
	staleRun := model.BuildRun{
		ID: "brun_stale", ProjectID: proj.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		Status: "failed", FailureCode: model.BuildFailureJobFailed, TriggerType: "manual", CreatedBy: triggerUser.ID,
	}
	if err := db.Create(&staleRun).Error; err != nil {
		t.Fatal(err)
	}
	staleDiag := model.AIDeliveryDiagnosis{
		ID:          "aidg_stale",
		ProjectID:   proj.ID,
		SubjectType: model.AIDeliverySubjectBuildRun,
		SubjectID:   staleRun.ID,
		Status:      model.AIDiagnosisStatusRunning,
		FailureCode: model.BuildFailureJobFailed,
		Confidence:  "medium",
		CreatedAt:   time.Now().UTC().Add(-5 * time.Minute),
		UpdatedAt:   time.Now().UTC().Add(-5 * time.Minute),
	}
	if err := db.Create(&staleDiag).Error; err != nil {
		t.Fatal(err)
	}
	if err := runner.reconcileAIDeliveryDiagnoses(t.Context()); err != nil {
		t.Fatalf("reconcileAIDeliveryDiagnoses error: %v", err)
	}
	var reconciled model.AIDeliveryDiagnosis
	if err := db.First(&reconciled, "id = ?", staleDiag.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reconciled.Status != model.AIDiagnosisStatusFailed || reconciled.ErrorCode != "ai.timeout" || reconciled.NotificationEmittedAt == nil {
		t.Fatalf("expected stale running diagnosis to reconcile to failed/ai.timeout with notification_emitted_at set, got %+v", reconciled)
	}
	var staleEvt model.PlatformEvent
	if err := db.Where("correlation_id = ? AND type = ?", staleRun.ID, "build.failed").First(&staleEvt).Error; err != nil {
		t.Fatalf("expected fallback build.failed PlatformEvent after reconciler sweep: %v", err)
	}

	// 8. Deleted subject BuildRun stamps notification_emitted_at cleanly so reconciler batch is never starved!
	orphanDiag := model.AIDeliveryDiagnosis{
		ID:          "aidg_deleted_subject",
		ProjectID:   proj.ID,
		SubjectType: model.AIDeliverySubjectBuildRun,
		SubjectID:   "brun_deleted_missing",
		Status:      model.AIDiagnosisStatusFailed,
		ErrorCode:   "ai.timeout",
		Confidence:  "medium",
		CreatedAt:   time.Now().UTC().Add(-6 * time.Minute),
		UpdatedAt:   time.Now().UTC().Add(-6 * time.Minute),
	}
	if err := db.Create(&orphanDiag).Error; err != nil {
		t.Fatal(err)
	}
	if err := runner.reconcileAIDeliveryDiagnoses(t.Context()); err != nil {
		t.Fatalf("reconcileAIDeliveryDiagnoses orphan error: %v", err)
	}
	var reconciledOrphan model.AIDeliveryDiagnosis
	if err := db.First(&reconciledOrphan, "id = ?", orphanDiag.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reconciledOrphan.NotificationEmittedAt == nil {
		t.Fatalf("expected deleted subject diagnosis to stamp notification_emitted_at, got nil")
	}

	// 9. Webhook BuildRun -> auto-deploy Release failing via finishDeployRelease (overwriting Message) still bills rule owner, NOT gitAccountOwner!
	if err := db.Model(&model.ProjectAIAutomationConfig{}).Where("project_id = ?", proj.ID).Updates(map[string]any{
		"delivery_cooldown_seconds": 0,
		"delivery_daily_limit":      20,
	}).Error; err != nil {
		t.Fatal(err)
	}
	autoRel := model.Release{
		ID:                 "rel_webhook_auto",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		BuildRunID:         webhookRun.ID,
		ImageRef:           "img:webhook",
		Type:               "deploy",
		Status:             "running",
		Revision:           5,
		Message:            "auto deploy from build",
		CreatedBy:          gitAccountOwner.ID,
		IdempotencyKey:     "auto-deploy:" + webhookRun.ID + ":" + target.ID,
	}
	if err := db.Create(&autoRel).Error; err != nil {
		t.Fatal(err)
	}
	transitioned, err := runner.finishDeployRelease(t.Context(), autoRel, "failed", model.ReleaseFailureRolloutTimeout, "Rollout timed out")
	if err != nil || !transitioned {
		t.Fatalf("finishDeployRelease failed: transitioned=%v err=%v", transitioned, err)
	}
	if len(enq.payloads) != 4 {
		t.Fatalf("expected 4th diagnosis enqueued for webhook auto-deploy release, got %d", len(enq.payloads))
	}
	task4, _ := tasks.NewAIDeliveryDiagnosisTask(enq.payloads[3])
	if err := runner.handleAIDeliveryDiagnosis(t.Context(), task4); err != nil {
		t.Fatal(err)
	}
	if capturedActorIDs[len(capturedActorIDs)-1] != owner.ID {
		t.Fatalf("expected failed webhook auto-deploy release to bill rule owner (%s) rather than gitAccountOwner (%s), got %v", owner.ID, gitAccountOwner.ID, capturedActorIDs)
	}
}
