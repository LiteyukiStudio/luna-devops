package worker

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
)

func TestClassifyRuntimeObservationAndDebounceRules(t *testing.T) {
	exit137 := int32(137)
	oomObs := kubeprovider.WorkloadRuntimeObservation{
		Status:          "degraded",
		ObservationCode: "runtime.oom_killed",
		Workload:        kubeprovider.DeploymentSnapshot{ReadyReplicas: 0, DesiredReplicas: 1},
		Pods: []kubeprovider.RuntimePodSnapshot{
			{
				Name:         "api-7d9f-oom",
				Status:       kubeprovider.PodStatusOOMKilled,
				RestartCount: 3,
				ContainerStatuses: []kubeprovider.RuntimeContainerStatusSnapshot{
					{
						Name:         "server",
						RestartCount: 3,
						CurrentState: kubeprovider.RuntimeContainerStateSnapshot{
							State:  "waiting",
							Reason: "CrashLoopBackOff",
						},
						LastTermination: &kubeprovider.RuntimeContainerTerminationSnapshot{
							Reason:   "OOMKilled",
							ExitCode: exit137,
							Message:  "Container killed due to memory limit",
						},
					},
				},
			},
		},
	}

	anomaly, healthy := classifyRuntimeObservation(oomObs)
	if healthy || anomaly == nil {
		t.Fatalf("expected OOM anomaly, got healthy=%v anomaly=%#v", healthy, anomaly)
	}
	if anomaly.EventType != "runtime.pod_oom_killed" || anomaly.ConfirmAfter != 1 {
		t.Fatalf("unexpected anomaly = %#v", anomaly)
	}
	if !anomaly.OOMKilled || anomaly.ExitCode == nil || *anomaly.ExitCode != 137 || anomaly.ContainerName != "server" {
		t.Fatalf("expected OOM container details, got %#v", anomaly)
	}

	target := model.DeploymentTarget{ID: "dplt_1", ProjectID: "prj_1", ApplicationID: "app_1"}
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	oomDecision := evaluateRuntimeIncidentAnomaly(nil, target, *anomaly, nil, t0)
	if oomDecision.Incident.Status != "active" || !oomDecision.ShouldEmitAlert || !oomDecision.MarkInboxUnread {
		t.Fatalf("expected immediate active alert on 1st OOM patrol, got %#v", oomDecision)
	}

	// Start failure requires 2 consecutive patrols before alerting.
	imgPullObs := kubeprovider.WorkloadRuntimeObservation{
		Status:          "degraded",
		ObservationCode: "runtime.image_pull_back_off",
		Workload:        kubeprovider.DeploymentSnapshot{ReadyReplicas: 0, DesiredReplicas: 1},
		Pods: []kubeprovider.RuntimePodSnapshot{
			{
				Name:   "api-img-err",
				Status: kubeprovider.PodStatusImagePullBackOff,
				ContainerStatuses: []kubeprovider.RuntimeContainerStatusSnapshot{
					{
						Name: "server",
						CurrentState: kubeprovider.RuntimeContainerStateSnapshot{
							State:   "waiting",
							Reason:  "ImagePullBackOff",
							Message: "Back-off pulling image registry.example.com/app:missing",
						},
					},
				},
			},
		},
	}
	startAnomaly, healthy := classifyRuntimeObservation(imgPullObs)
	if healthy || startAnomaly == nil || startAnomaly.EventType != "runtime.pod_start_failed" || startAnomaly.ConfirmAfter != 2 {
		t.Fatalf("unexpected start anomaly = %#v", startAnomaly)
	}

	tick1 := evaluateRuntimeIncidentAnomaly(nil, target, *startAnomaly, nil, t0)
	if tick1.Incident.Status != "suspect" || tick1.ShouldEmitAlert || tick1.ShouldUpdateInbox {
		t.Fatalf("expected 1st ImagePullBackOff patrol to stay suspect without alerting, got %#v", tick1)
	}
	tick2 := evaluateRuntimeIncidentAnomaly(&tick1.Incident, target, *startAnomaly, nil, t0.Add(time.Minute))
	if tick2.Incident.Status != "active" || !tick2.ShouldEmitAlert || !tick2.MarkInboxUnread {
		t.Fatalf("expected 2nd consecutive ImagePullBackOff patrol to confirm and alert, got %#v", tick2)
	}
}

func TestPerRuleRemindIntervalAndMaxReminders(t *testing.T) {
	target := model.DeploymentTarget{ID: "dplt_1", ProjectID: "prj_1", ApplicationID: "app_1"}
	anomaly := runtimeDetectedAnomaly{
		EventType:       "runtime.pod_crash_loop",
		Severity:        notification.SeverityError,
		Fingerprint:     "runtime.pod_crash_loop:app:CrashLoopBackOff",
		PodName:         "app-crash-1",
		ContainerName:   "app",
		Reason:          "CrashLoopBackOff",
		RestartCount:    4,
		DesiredReplicas: 1,
		Summary:         "CrashLoopBackOff",
		ConfirmAfter:    1,
	}
	rules := []model.NotificationRule{
		{
			ID:                    "rule_once",
			Enabled:               true,
			RemindIntervalMinutes: 15,
			MaxReminders:          0, // once-only
		},
		{
			ID:                    "rule_fast_x2",
			Enabled:               true,
			RemindIntervalMinutes: 15,
			MaxReminders:          2, // initial + up to 2 reminders
		},
		{
			ID:                    "rule_default_60m",
			Enabled:               true,
			RemindIntervalMinutes: 60,
			MaxReminders:          3,
		},
	}

	t0 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	d0 := evaluateRuntimeIncidentAnomaly(nil, target, anomaly, rules, t0)
	if !d0.ShouldEmitAlert || d0.IsReminder {
		t.Fatalf("t0 expected initial alert, got %#v", d0)
	}
	if !slices.Equal(d0.EligibleRuleIDs, []string{"rule_once", "rule_fast_x2", "rule_default_60m"}) {
		t.Fatalf("t0 eligible rules = %v", d0.EligibleRuleIDs)
	}
	incident := d0.Incident

	// t + 10m: no rule due yet
	d10 := evaluateRuntimeIncidentAnomaly(&incident, target, anomaly, rules, t0.Add(10*time.Minute))
	if d10.ShouldEmitAlert {
		t.Fatalf("t+10m expected no alert, got %#v", d10)
	}
	incident = d10.Incident

	// t + 15m: only rule_fast_x2 is due (reminder #1)
	d15 := evaluateRuntimeIncidentAnomaly(&incident, target, anomaly, rules, t0.Add(15*time.Minute))
	if !d15.ShouldEmitAlert || !d15.IsReminder || d15.ReminderCount != 1 {
		t.Fatalf("t+15m expected reminder #1, got %#v", d15)
	}
	if !slices.Equal(d15.EligibleRuleIDs, []string{"rule_fast_x2"}) || d15.IncludePersonalOwner {
		t.Fatalf("t+15m eligible = %v, includePersonal=%v", d15.EligibleRuleIDs, d15.IncludePersonalOwner)
	}
	incident = d15.Incident

	// t + 30m: rule_fast_x2 reminder #2
	d30 := evaluateRuntimeIncidentAnomaly(&incident, target, anomaly, rules, t0.Add(30*time.Minute))
	if !d30.ShouldEmitAlert || !slices.Equal(d30.EligibleRuleIDs, []string{"rule_fast_x2"}) || d30.ReminderCount != 2 {
		t.Fatalf("t+30m expected rule_fast_x2 reminder #2, got %#v", d30)
	}
	incident = d30.Incident

	// t + 45m: rule_fast_x2 exhausted maxReminders=2, rule_default_60m not due yet -> silent!
	d45 := evaluateRuntimeIncidentAnomaly(&incident, target, anomaly, rules, t0.Add(45*time.Minute))
	if d45.ShouldEmitAlert {
		t.Fatalf("t+45m expected silence after rule_fast_x2 exhausted maxReminders=2, got %#v", d45)
	}
	incident = d45.Incident

	// t + 60m: rule_default_60m + personal default 60m are due, while rule_once and rule_fast_x2 remain silent!
	d60 := evaluateRuntimeIncidentAnomaly(&incident, target, anomaly, rules, t0.Add(60*time.Minute))
	if !d60.ShouldEmitAlert || !slices.Equal(d60.EligibleRuleIDs, []string{"rule_default_60m"}) || !d60.IncludePersonalOwner {
		t.Fatalf("t+60m expected rule_default_60m + personal owner, got %#v", d60)
	}
}

type fakeRuntimeObserver struct {
	observation kubeprovider.WorkloadRuntimeObservation
	err         error
}

func (f *fakeRuntimeObserver) ObserveWorkloadRuntime(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
) (kubeprovider.WorkloadRuntimeObservation, error) {
	return f.observation, f.err
}

type noopNotificationDeliveryEnqueuer struct {
	enqueued []tasks.NotificationDeliverPayload
}

func (n *noopNotificationDeliveryEnqueuer) EnqueueNotificationDeliver(_ context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error) {
	n.enqueued = append(n.enqueued, payload)
	return &asynq.TaskInfo{ID: payload.DeliveryID}, nil
}

func TestPatrolRuntimeWorkloadsEndToEndWithInboxCoalescingAndRecovery(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_alert_patrol_test",
	})
	ctx := context.Background()

	owner := model.User{ID: "usr_rt_owner", Email: "rt-owner@example.com", Name: "Owner", Role: authz.PlatformRoleUser}
	dev := model.User{ID: "usr_rt_dev", Email: "rt-dev@example.com", Name: "Dev", Role: authz.PlatformRoleUser}
	viewer := model.User{ID: "usr_rt_viewer", Email: "rt-viewer@example.com", Name: "Viewer", Role: authz.PlatformRoleUser}
	for _, u := range []model.User{owner, dev, viewer} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("create user %s: %v", u.ID, err)
		}
	}

	project := model.Project{ID: "prj_rt_1", Name: "Runtime Space", Identifier: "rt-space"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	for _, member := range []model.ProjectMember{
		{ID: "pm_rt_1", ProjectID: project.ID, UserID: owner.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_rt_2", ProjectID: project.ID, UserID: dev.ID, Role: authz.ProjectRoleDeveloper},
		{ID: "pm_rt_3", ProjectID: project.ID, UserID: viewer.ID, Role: authz.ProjectRoleViewer},
	} {
		if err := db.Create(&member).Error; err != nil {
			t.Fatalf("create member: %v", err)
		}
	}

	app := model.Application{ID: "app_rt_1", ProjectID: project.ID, Name: "Order API", Identifier: "order-api", DeleteStatus: "active"}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	target := model.DeploymentTarget{
		ID:            "dplt_rt_1",
		ProjectID:     project.ID,
		ApplicationID: app.ID,
		Name:          "Production",
		Stage:         "prod",
		WorkloadType:  "Deployment",
		Replicas:      2,
		Enabled:       true,
		DeleteStatus:  "active",
		CreatedBy:     owner.ID,
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	rel := model.Release{
		ID:                 "rel_rt_1",
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Revision:           1,
		Status:             "succeeded",
		ImageRef:           "registry.example.com/order-api:v1",
		CreatedBy:          owner.ID,
	}
	if err := db.Create(&rel).Error; err != nil {
		t.Fatalf("create release: %v", err)
	}

	channel := model.NotificationChannel{
		ID:          "nch_rt_proj",
		Name:        "Feishu Ops",
		Scope:       "project",
		ProjectID:   project.ID,
		AdapterKind: notification.AdapterKindWebhook,
		ConfigJSON:  `{"method":"POST","url":"https://open.feishu.cn/open-apis/bot/v2/hook/demo"}`,
		Enabled:     true,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&model.ScopedResourceProjectBinding{
		ID:           "srpb_rt_1",
		ResourceType: notification.ScopedResourceChannelType,
		ResourceID:   channel.ID,
		ProjectID:    project.ID,
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}
	rule := model.NotificationRule{
		ID:                    "nrl_rt_proj",
		ProjectID:             project.ID,
		Name:                  "Runtime Alerts",
		EventTypesJSON:        `["runtime.pod_oom_killed","runtime.pod_crash_loop"]`,
		FilterJSON:            `{"scope":"project"}`,
		ChannelIDsJSON:        `["nch_rt_proj"]`,
		RemindIntervalMinutes: 60,
		MaxReminders:          2,
		NotifyOnRecovery:      true,
		TimeZone:              "Asia/Shanghai",
		Enabled:               true,
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}

	observer := &fakeRuntimeObserver{
		observation: kubeprovider.WorkloadRuntimeObservation{
			Status:          "degraded",
			ObservationCode: "runtime.crash_loop_back_off",
			Workload:        kubeprovider.DeploymentSnapshot{ReadyReplicas: 0, DesiredReplicas: 2},
			Pods: []kubeprovider.RuntimePodSnapshot{
				{
					Name:         "order-api-crash",
					Status:       kubeprovider.PodStatusCrashLoopBackOff,
					RestartCount: 6,
					ContainerStatuses: []kubeprovider.RuntimeContainerStatusSnapshot{
						{
							Name:         "order-api",
							RestartCount: 6,
							CurrentState: kubeprovider.RuntimeContainerStateSnapshot{State: "waiting", Reason: "CrashLoopBackOff"},
						},
					},
				},
			},
		},
	}
	enqueuer := &noopNotificationDeliveryEnqueuer{}
	runner := newRunner(db, Options{PublicBaseURL: "https://devops.example.com"})
	runner.notificationDeliveryEnqueuer = enqueuer
	runner.runtimeObserverFactory = func(_ context.Context, _ model.DeploymentTarget) (runtimeWorkloadObserver, error) {
		return observer, nil
	}

	t0 := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	if err := runner.patrolRuntimeWorkloadsAt(ctx, t0); err != nil {
		t.Fatalf("patrol t0: %v", err)
	}
	// Second patrol 1m later should coalesce Inbox in-place and NOT duplicate deliveries!
	if err := runner.patrolRuntimeWorkloadsAt(ctx, t0.Add(time.Minute)); err != nil {
		t.Fatalf("patrol t0+1m: %v", err)
	}

	var ownerInbox []model.InboxMessage
	if err := db.Where("recipient_user_id = ?", owner.ID).Find(&ownerInbox).Error; err != nil {
		t.Fatalf("query owner inbox: %v", err)
	}
	if len(ownerInbox) != 1 {
		t.Fatalf("expected 1 coalesced inbox message for owner, got %d", len(ownerInbox))
	}
	var viewerInboxCount int64
	db.Model(&model.InboxMessage{}).Where("recipient_user_id = ?", viewer.ID).Count(&viewerInboxCount)
	if viewerInboxCount != 0 {
		t.Fatalf("expected viewer to receive 0 runtime inbox alerts, got %d", viewerInboxCount)
	}

	var crashDeliveries []model.NotificationDelivery
	if err := db.Where("channel_id = ? AND event_type = ?", channel.ID, "runtime.pod_crash_loop").Find(&crashDeliveries).Error; err != nil {
		t.Fatalf("query crash deliveries: %v", err)
	}
	if len(crashDeliveries) != 1 {
		t.Fatalf("expected 1 throttled delivery for rule at t0/t0+1m, got %d", len(crashDeliveries))
	}

	// Now simulate workload recovery at t0 + 5m!
	observer.observation = kubeprovider.WorkloadRuntimeObservation{
		Status:          "ready",
		ObservationCode: "deployment_target.ready",
		Workload:        kubeprovider.DeploymentSnapshot{Phase: kubeprovider.DeploymentSucceeded, ReadyReplicas: 2, DesiredReplicas: 2},
	}
	if err := runner.patrolRuntimeWorkloadsAt(ctx, t0.Add(5*time.Minute)); err != nil {
		t.Fatalf("patrol recovery: %v", err)
	}

	var incident model.RuntimeAlertIncident
	if err := db.First(&incident, "deployment_target_id = ?", target.ID).Error; err != nil {
		t.Fatalf("query incident: %v", err)
	}
	if incident.Status != "resolved" || incident.ResolvedAt == nil {
		t.Fatalf("expected resolved incident, got %#v", incident)
	}

	var recoveryDeliveries []model.NotificationDelivery
	if err := db.Where("channel_id = ? AND event_type = ?", channel.ID, "runtime.recovered").Find(&recoveryDeliveries).Error; err != nil {
		t.Fatalf("query recovery deliveries: %v", err)
	}
	if len(recoveryDeliveries) != 1 {
		t.Fatalf("expected 1 runtime.recovered delivery via NotifyOnRecovery, got %d", len(recoveryDeliveries))
	}

	if err := db.Where("recipient_user_id = ?", owner.ID).Find(&ownerInbox).Error; err != nil {
		t.Fatalf("query owner inbox after recovery: %v", err)
	}
	if len(ownerInbox) != 1 || ownerInbox[0].TitleKey != "inbox.runtime.recovered.title" {
		t.Fatalf("expected coalesced inbox message updated in-place to recovered title, got %#v", ownerInbox)
	}
}
