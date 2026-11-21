package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func TestRuntimePatrolAIShortTimeoutAsyncInboxAndCircuitBreakerPostgres(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "luna_ai_runtime_patrol",
		Migrate: func(db *gorm.DB) error {
			return database.MigrateContext(context.Background(), db)
		},
	})
	ctx := context.Background()

	if err := db.Create(&model.AppConfig{Key: "ai.assistant.enabled", Value: "true"}).Error; err != nil {
		t.Fatal(err)
	}

	owner := model.User{ID: "usr_rt_ai_owner", Email: "owner@example.test", Name: "Owner", Role: authz.PlatformRoleUser, Language: "zh-CN"}
	dev := model.User{ID: "usr_rt_ai_dev", Email: "dev@example.test", Name: "Dev", Role: authz.PlatformRoleUser, Language: "zh-CN"}
	for _, u := range []model.User{owner, dev} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.UserWallet{
		UserID:         owner.ID,
		BalanceCredits: decimal.NewFromInt(200),
	}).Error; err != nil {
		t.Fatal(err)
	}

	project := model.Project{ID: "prj_rt_ai", Name: "Runtime AI", Identifier: "runtime-ai", DeleteStatus: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	for _, m := range []model.ProjectMember{
		{ID: "pm_rt_ai_1", ProjectID: project.ID, UserID: owner.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_rt_ai_2", ProjectID: project.ID, UserID: dev.ID, Role: authz.ProjectRoleDeveloper},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}

	app := model.Application{ID: "app_rt_ai", ProjectID: project.ID, Name: "Payment Worker", Identifier: "payment-worker", DeleteStatus: "active"}
	if err := db.Create(&app).Error; err != nil {
		t.Fatal(err)
	}
	target := model.DeploymentTarget{
		ID:            "dplt_rt_ai",
		ProjectID:     project.ID,
		ApplicationID: app.ID,
		Name:          "prod",
		Stage:         "prod",
		WorkloadType:  "Deployment",
		Replicas:      2,
		Enabled:       true,
		DeleteStatus:  "active",
		CreatedBy:     owner.ID,
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Release{
		ID:                 "rel_rt_ai_1",
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Revision:           1,
		Status:             "succeeded",
		ImageRef:           "registry.example.test/payment-worker:v1",
		CreatedBy:          owner.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}

	channel := model.NotificationChannel{
		ID:             "nch_rt_ai",
		Name:           "Webhook",
		Scope:          "project",
		ProjectID:      project.ID,
		AdapterKind:    notification.AdapterKindWebhook,
		ConfigJSON:     `{"method":"POST","url":"https://example.test/hook"}`,
		SecretRefsJSON: `{}`,
		Enabled:        true,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ScopedResourceProjectBinding{
		ID:           "srpb_rt_ai",
		ResourceType: notification.ScopedResourceChannelType,
		ResourceID:   channel.ID,
		ProjectID:    project.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	rule := model.NotificationRule{
		ID:                    "nrl_rt_ai",
		ProjectID:             project.ID,
		Name:                  "Runtime Crash Alerts",
		EventTypesJSON:        `["runtime.pod_oom_killed","runtime.pod_crash_loop"]`,
		FilterJSON:            `{"scope":"all"}`,
		ChannelIDsJSON:        `["nch_rt_ai"]`,
		ChannelTargetsJSON:    `{}`,
		RemindIntervalMinutes: 10,
		MaxReminders:          5,
		NotifyOnRecovery:      true,
		TimeZone:              "UTC",
		Enabled:               true,
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}

	var agentDelayMs atomic.Int64
	var billedActors []string
	fakeAgent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delay := agentDelayMs.Load()
		if delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		var body struct {
			ActorUserID string `json:"actorUserId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		billedActors = append(billedActors, body.ActorUserID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"runId": "airun_rt_1",
			"taskType": "incident_diagnosis",
			"output": {
				"rootCauseSummary": "Container OOMKilled due to unbounded cache; Authorization: Bearer secret-token-xyz",
				"confidence": "high",
				"suggestions": ["Raise memoryRequest to 512Mi"],
				"proposedAction": {
					"toolName": "delivery_restart_target",
					"arguments": {"application": "app_rt_ai", "target": "dplt_rt_ai"},
					"summary": "Rolling restart payment-worker prod"
				}
			}
		}`))
	}))
	defer fakeAgent.Close()

	var autoHealCalls atomic.Int32
	fakeAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/ai/tools/auto-heal" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer cb_tok_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		autoHealCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"toolName": "delivery_restart_target",
			"approvalTier": "L2",
			"previewSummary": "Rolling restart dplt_rt_ai",
			"beforeAfterSummary": "Rolling restart dplt_rt_ai; restartedAt: none -> now"
		}`))
	}))
	defer fakeAPI.Close()

	agentClient, err := aiagent.NewHTTPClientWithTimeout(
		fakeAgent.URL,
		"svc_tok_test",
		"0123456789abcdef0123456789abcdef",
		5*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}

	oomObs := kubeprovider.WorkloadRuntimeObservation{
		Status:          "degraded",
		ObservationCode: "runtime.oom_killed",
		Workload:        kubeprovider.DeploymentSnapshot{ReadyReplicas: 0, DesiredReplicas: 2},
		Pods: []kubeprovider.RuntimePodSnapshot{
			{
				Name:         "payment-worker-oom",
				Status:       kubeprovider.PodStatusOOMKilled,
				RestartCount: 4,
				ContainerStatuses: []kubeprovider.RuntimeContainerStatusSnapshot{
					{
						Name:         "worker",
						RestartCount: 4,
						CurrentState: kubeprovider.RuntimeContainerStateSnapshot{State: "waiting", Reason: "CrashLoopBackOff"},
						LastTermination: &kubeprovider.RuntimeContainerTerminationSnapshot{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}

	observer := &fakeRuntimeObserver{observation: oomObs}
	enqueuer := &noopNotificationDeliveryEnqueuer{}
	runner := newRunner(db, Options{
		PublicBaseURL:      "https://devops.example.test",
		InternalAPIBaseURL: fakeAPI.URL,
		AIAgent: aiagent.Config{
			Available:            true,
			BaseURL:              fakeAgent.URL,
			ServiceToken:         "svc_tok_test",
			CallbackServiceToken: "cb_tok_test",
			ActorSigningKey:      "0123456789abcdef0123456789abcdef",
			Timeout:              5 * time.Second,
		},
	})
	runner.aiAgent = agentClient
	runner.aiAgentAvailable = true
	runner.notificationDeliveryEnqueuer = enqueuer
	runner.runtimeObserverFactory = func(_ context.Context, _ model.DeploymentTarget) (runtimeWorkloadObserver, error) {
		return observer, nil
	}

	// Configure suggest_only first and test >15s inline timeout (set inline timeout to 25ms, agent takes 120ms)!
	if err := db.Create(&model.ProjectAIAutomationConfig{
		ProjectID:                   project.ID,
		RuntimeDiagnosisEnabled:     true,
		RuntimeBillingUserID:        owner.ID,
		RuntimeCooldownSeconds:      0,
		RuntimeDailyLimit:           10,
		SelfHealMode:                model.AISelfHealModeSuggestOnly,
		StageSelfHealModesJSON:      "{}",
		AllowedAutoHealToolsJSON:    "[]",
		MaxConsecutiveAutoHeals:     1,
		CircuitBreakerWindowSeconds: 1800,
	}).Error; err != nil {
		t.Fatal(err)
	}

	runner.runtimeDiagnosisInlineTimeout = 25 * time.Millisecond
	agentDelayMs.Store(120)

	t0 := time.Now().UTC()
	start := time.Now()
	if err := runner.patrolRuntimeWorkloadsAt(ctx, t0); err != nil {
		t.Fatalf("patrol t0: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 110*time.Millisecond {
		t.Fatalf("expected patrol to emit base alert immediately at ~25ms inline timeout without waiting 120ms, took %v", elapsed)
	}

	// Base PlatformEvent MUST already exist BEFORE async AI finishes!
	var initialEvents []model.PlatformEvent
	if err := db.Where("project_id = ? AND type = ?", project.ID, "runtime.pod_oom_killed").Find(&initialEvents).Error; err != nil || len(initialEvents) != 1 {
		t.Fatalf("expected 1 immediate base PlatformEvent before async AI completes, got %d (%v)", len(initialEvents), err)
	}
	if strings.Contains(initialEvents[0].DetailJSON, "aiDiagnosis") {
		t.Fatalf("expected timed-out base alert NOT to block waiting for aiDiagnosis, got %s", initialEvents[0].DetailJSON)
	}

	// Wait for background async diagnosis goroutine to finish updating Inbox + ActionRequest!
	runner.WaitRuntimeDiagnosisAsync()

	if len(billedActors) != 1 || billedActors[0] != owner.ID {
		t.Fatalf("expected personal wallet actor %s billed once, got %v", owner.ID, billedActors)
	}

	// Both Owner and Dev MUST now have coalesced InboxMessage updated with redacted AI diagnosis + own ActionRequest!
	for _, uid := range []string{owner.ID, dev.ID} {
		var msgs []model.InboxMessage
		if err := db.Where("recipient_user_id = ? AND project_id = ?", uid, project.ID).Find(&msgs).Error; err != nil || len(msgs) != 1 {
			t.Fatalf("expected 1 coalesced inbox message for %s, got %d (%v)", uid, len(msgs), err)
		}
		if msgs[0].ActionRequestID == "" || msgs[0].Category != inbox.CategoryAction {
			t.Fatalf("expected async diagnosis to attach ActionRequest to %s inbox message, got %+v", uid, msgs[0])
		}
		if strings.Contains(msgs[0].ParamsJSON, "secret-token-xyz") || !strings.Contains(msgs[0].ParamsJSON, "unbounded cache") {
			t.Fatalf("expected redacted AI root cause in %s inbox params, got %s", uid, msgs[0].ParamsJSON)
		}
	}

	// Now switch project policy to auto_l2 (MaxConsecutiveAutoHeals = 1) with fast inline response (<15s)!
	if err := db.Model(&model.ProjectAIAutomationConfig{}).
		Where("project_id = ?", project.ID).
		Update("self_heal_mode", model.AISelfHealModeAutoL2).Error; err != nil {
		t.Fatal(err)
	}
	runner.runtimeDiagnosisInlineTimeout = 2 * time.Second
	agentDelayMs.Store(0)

	// Patrol at t0 + 11m (triggers reminder #1 -> inline diagnosis + auto_l2 auto-heal #1 succeeds!)
	if err := runner.patrolRuntimeWorkloadsAt(ctx, t0.Add(11*time.Minute)); err != nil {
		t.Fatalf("patrol t0+11m: %v", err)
	}
	if autoHealCalls.Load() != 1 {
		t.Fatalf("expected 1 auto_l2 execution at t0+11m, got %d", autoHealCalls.Load())
	}
	var eventsAfterAutoHeal []model.PlatformEvent
	_ = db.Where("project_id = ? AND type = ?", project.ID, "runtime.pod_oom_killed").Order("occurred_at desc").Find(&eventsAfterAutoHeal).Error
	if len(eventsAfterAutoHeal) < 2 || !strings.Contains(eventsAfterAutoHeal[0].DetailJSON, "Rolling restart dplt_rt_ai") {
		t.Fatalf("expected inline auto-heal before/after summary merged into alert event, got %+v", eventsAfterAutoHeal)
	}

	// Patrol at t0 + 22m (still crashing after 1 auto-heal -> MaxConsecutiveAutoHeals=1 MUST trip anti-flapping circuit breaker!)
	if err := runner.patrolRuntimeWorkloadsAt(ctx, t0.Add(22*time.Minute)); err != nil {
		t.Fatalf("patrol t0+22m: %v", err)
	}
	if autoHealCalls.Load() != 1 {
		t.Fatalf("expected circuit breaker to block second auto-heal (calls still 1), got %d", autoHealCalls.Load())
	}

	var latestDiag model.AIRuntimeIncidentDiagnosis
	if err := db.Where("project_id = ?", project.ID).Order("created_at desc").First(&latestDiag).Error; err != nil {
		t.Fatal(err)
	}
	if !latestDiag.CircuitBreakerTripped || latestDiag.AutoHealStatus != model.AIAutoHealStatusCircuitBroken {
		t.Fatalf("expected circuit_breaker_tripped=true and status=circuit_broken, got %+v", latestDiag)
	}

	// Finally, enforce daily limit: set RuntimeDailyLimit = 3 (3 diagnoses already ran today) -> 4th reminder does NOT call AI!
	if err := db.Model(&model.ProjectAIAutomationConfig{}).
		Where("project_id = ?", project.ID).
		Update("runtime_daily_limit", 3).Error; err != nil {
		t.Fatal(err)
	}
	prevCalls := len(billedActors)
	if err := runner.patrolRuntimeWorkloadsAt(ctx, t0.Add(33*time.Minute)); err != nil {
		t.Fatalf("patrol t0+33m: %v", err)
	}
	if len(billedActors) != prevCalls {
		t.Fatalf("expected daily limit=3 to cap further AI calls, got %d calls", len(billedActors))
	}
}
