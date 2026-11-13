package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

const (
	personalRuleStateKey              = "__personal__"
	noMatchingSharedRulePlaceholderID = "__none__"
	defaultPersonalRemindIntervalMins = 60
	defaultPersonalMaxReminders       = 3
	certificatePatrolIntervalMinutes  = 15
	runtimePatrolTargetBatchLimit     = 200
	certificatePatrolRouteBatchLimit  = 200
)

type runtimeWorkloadObserver interface {
	ObserveWorkloadRuntime(
		ctx context.Context,
		namespace string,
		deploymentTargetID string,
		workloadName string,
		workloadType string,
	) (kubeprovider.WorkloadRuntimeObservation, error)
}

type runtimeDetectedAnomaly struct {
	EventType        string `json:"eventType"`
	Severity         string `json:"severity"`
	Fingerprint      string `json:"fingerprint"`
	PodName          string `json:"podName,omitempty"`
	ContainerName    string `json:"containerName,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ExitCode         *int32 `json:"exitCode,omitempty"`
	RestartCount     int32  `json:"restartCount,omitempty"`
	OOMKilled        bool   `json:"oomKilled,omitempty"`
	ReadyReplicas    int32  `json:"readyReplicas"`
	DesiredReplicas  int32  `json:"desiredReplicas"`
	Summary          string `json:"summary"`
	DiagnosticDetail string `json:"diagnosticDetail,omitempty"`
	ConfirmAfter     int    `json:"confirmAfter"`
}

type ruleAlertNotificationState struct {
	NotifiedCount  int       `json:"notifiedCount"`
	LastNotifiedAt time.Time `json:"lastNotifiedAt"`
}

type runtimeAlertDecision struct {
	Incident             model.RuntimeAlertIncident
	ShouldEmitAlert      bool
	ShouldUpdateInbox    bool
	MarkInboxUnread      bool
	IsReminder           bool
	ReminderCount        int
	EligibleRuleIDs      []string
	IncludePersonalOwner bool
}

func (r *Runner) runtimeObserver(ctx context.Context, target model.DeploymentTarget) (runtimeWorkloadObserver, error) {
	if r.runtimeObserverFactory != nil {
		return r.runtimeObserverFactory(ctx, target)
	}
	manager, err := r.kubernetesManager(ctx, target)
	if err != nil {
		return nil, err
	}
	observer, ok := manager.(runtimeWorkloadObserver)
	if !ok {
		return nil, nil
	}
	return observer, nil
}

func (r *Runner) patrolRuntimeWorkloads(ctx context.Context) error {
	return r.patrolRuntimeWorkloadsAt(ctx, time.Now())
}

func (r *Runner) patrolRuntimeWorkloadsAt(ctx context.Context, now time.Time) error {
	if r.db == nil {
		return nil
	}
	db := r.db.WithContext(ctx)
	var targets []model.DeploymentTarget
	err := db.Table("deployment_targets").
		Select("deployment_targets.*").
		Joins("JOIN applications ON applications.id = deployment_targets.application_id AND applications.deleted_at IS NULL AND applications.delete_status = ?", "active").
		Joins("JOIN projects ON projects.id = deployment_targets.project_id AND projects.deleted_at IS NULL").
		Where("deployment_targets.deleted_at IS NULL AND deployment_targets.enabled = ? AND deployment_targets.delete_status = ?", true, "active").
		Where("EXISTS (SELECT 1 FROM releases WHERE releases.deployment_target_id = deployment_targets.id AND releases.status = ?)", "succeeded").
		Where("NOT EXISTS (SELECT 1 FROM releases WHERE releases.deployment_target_id = deployment_targets.id AND releases.status IN ?)", []string{"pending", "running"}).
		Order("deployment_targets.updated_at desc").
		Limit(runtimePatrolTargetBatchLimit).
		Find(&targets).Error
	if err != nil || len(targets) == 0 {
		return err
	}

	projectsByID := map[string]model.Project{}
	appsByID := map[string]model.Application{}
	for _, target := range targets {
		project, ok := projectsByID[target.ProjectID]
		if !ok {
			if loadErr := db.First(&project, "id = ?", target.ProjectID).Error; loadErr != nil {
				continue
			}
			projectsByID[target.ProjectID] = project
		}
		application, ok := appsByID[target.ApplicationID]
		if !ok {
			if loadErr := db.First(&application, "id = ?", target.ApplicationID).Error; loadErr != nil {
				continue
			}
			appsByID[target.ApplicationID] = application
		}
		r.patrolSingleDeploymentTarget(ctx, target, project, application, now)
	}
	return nil
}

func (r *Runner) patrolSingleDeploymentTarget(
	ctx context.Context,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	now time.Time,
) {
	observer, err := r.runtimeObserver(ctx, target)
	if err != nil || observer == nil {
		if err != nil {
			telemetry.RecordError(ctx, "runtime.patrol_observer_unavailable", err,
				slog.String("deployment_target.id", target.ID))
		}
		return
	}

	namespace := deploymentNamespace(project)
	workloadName := applicationResourceName(target)
	workloadType := strings.TrimSpace(target.WorkloadType)
	if workloadType == "" {
		workloadType = "Deployment"
	}
	observation, err := observer.ObserveWorkloadRuntime(ctx, namespace, target.ID, workloadName, workloadType)
	if err != nil {
		if !isKubernetesNotFound(err) {
			telemetry.RecordError(ctx, "runtime.patrol_observe_failed", err,
				slog.String("deployment_target.id", target.ID))
		}
		return
	}

	anomaly, healthy := classifyRuntimeObservation(observation)
	if !healthy && anomaly == nil {
		// Active rollout convergence in progress; wait for next patrol.
		return
	}

	db := r.db.WithContext(ctx)
	var existing model.RuntimeAlertIncident
	hasOpen := db.Where(
		"deployment_target_id = ? AND status IN ?",
		target.ID,
		[]string{"suspect", "active"},
	).Order("created_at desc").First(&existing).Error == nil

	if healthy {
		if !hasOpen {
			return
		}
		r.resolveRuntimeIncident(ctx, existing, target, project, application, observation, now)
		return
	}

	rules := r.loadCandidateRuntimeRules(ctx, target.ProjectID)
	var currentPtr *model.RuntimeAlertIncident
	if hasOpen {
		currentPtr = &existing
	}
	probeEvent := buildRuntimeProbeEvent(target, project, application, *anomaly, now)
	matchingRules := make([]model.NotificationRule, 0, len(rules))
	for _, rule := range rules {
		if notification.RuleMatchesEvent(rule, probeEvent) {
			matchingRules = append(matchingRules, rule)
		}
	}

	decision := evaluateRuntimeIncidentAnomaly(currentPtr, target, *anomaly, matchingRules, now)
	if hasOpen {
		if saveErr := db.Save(&decision.Incident).Error; saveErr != nil {
			telemetry.RecordError(ctx, "runtime.patrol_save_incident_failed", saveErr)
			return
		}
	} else {
		if createErr := db.Create(&decision.Incident).Error; createErr != nil {
			telemetry.RecordError(ctx, "runtime.patrol_create_incident_failed", createErr)
			return
		}
	}

	if decision.ShouldEmitAlert {
		eventID := r.emitRuntimeAnomalyNotification(ctx, decision, target, project, application, *anomaly, now)
		if eventID != "" {
			_ = db.Model(&model.RuntimeAlertIncident{}).
				Where("id = ?", decision.Incident.ID).
				Update("last_event_id", eventID).Error
		}
	}
	if decision.ShouldUpdateInbox {
		r.publishRuntimeAnomalyInbox(ctx, decision, target, project, application, *anomaly)
	}
}

func (r *Runner) resolveRuntimeIncident(
	ctx context.Context,
	incident model.RuntimeAlertIncident,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	observation kubeprovider.WorkloadRuntimeObservation,
	now time.Time,
) {
	wasActive := incident.Status == "active"
	resolvedAt := now
	incident.Status = "resolved"
	incident.LastObservedAt = now
	incident.ResolvedAt = &resolvedAt
	incident.UpdatedAt = now
	if err := r.db.WithContext(ctx).Save(&incident).Error; err != nil {
		telemetry.RecordError(ctx, "runtime.patrol_resolve_incident_failed", err)
		return
	}
	if !wasActive {
		return
	}

	durationSecs := int64(0)
	if !incident.FirstDetectedAt.IsZero() && now.After(incident.FirstDetectedAt) {
		durationSecs = int64(now.Sub(incident.FirstDetectedAt).Seconds())
	}

	rules := r.loadCandidateRuntimeRules(ctx, target.ProjectID)
	projectRef, appRef, targetRef := notificationEntityRefs(target.ProjectID, target.ApplicationID, target.ID, project, application, target)
	firstDetected := incident.FirstDetectedAt
	recoveryEvent := notification.Event{
		ID:               id.New("evt"),
		Type:             "runtime.recovered",
		Severity:         notification.SeverityInfo,
		Project:          projectRef,
		Application:      appRef,
		DeploymentTarget: targetRef,
		Runtime: notification.RuntimeIncidentContext{
			IncidentID:      incident.ID,
			IncidentKind:    incident.IncidentKind,
			ReadyReplicas:   observation.Workload.ReadyReplicas,
			DesiredReplicas: observation.Workload.DesiredReplicas,
			FirstDetectedAt: &firstDetected,
			ResolvedAt:      &resolvedAt,
			DurationSeconds: durationSecs,
			Summary:         fmt.Sprintf("Workload %s (%s) recovered (%d/%d ready)", application.Name, target.Name, observation.Workload.ReadyReplicas, observation.Workload.DesiredReplicas),
		},
		ResourceOwnerUserID: r.notificationDeploymentTargetOwnerUserID(ctx, target.ProjectID, target.ApplicationID, target.ID, target),
		CorrelationID:       incident.ID,
		DedupKey:            fmt.Sprintf("runtime:%s:recovered", incident.ID),
		OccurredAt:          now,
		Links:               r.notificationLinks(target.ProjectID, target.ApplicationID, "deployments", "runtime", "targetId", target.ID),
		Message:             fmt.Sprintf("%s / %s 运行状态已恢复正常 (%d/%d Ready)", firstNonEmpty(application.Name, application.Identifier), firstNonEmpty(target.Name, target.Stage), observation.Workload.ReadyReplicas, observation.Workload.DesiredReplicas),
	}

	eligibleRuleIDs := make([]string, 0, len(rules))
	for _, rule := range rules {
		if !rule.NotifyOnRecovery {
			continue
		}
		if notification.RuleMatchesEvent(rule, recoveryEvent) {
			eligibleRuleIDs = append(eligibleRuleIDs, rule.ID)
		}
	}
	if len(eligibleRuleIDs) == 0 {
		eligibleRuleIDs = []string{noMatchingSharedRulePlaceholderID}
	}
	recoveryEvent.EligibleRuleIDs = eligibleRuleIDs
	r.emitNotificationEvent(ctx, recoveryEvent)

	actionLink := runtimeTargetActionLink(target.ProjectID, target.ApplicationID, target.ID)
	coalesceKey := firstNonEmpty(incident.InboxCoalesceKey, "runtime_incident:"+incident.ID)
	_ = inbox.NewService(r.db).PublishCoalescedToProjectRoles(
		ctx,
		target.ProjectID,
		[]string{authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper},
		inbox.PublishInput{
			Type:       "runtime.recovered",
			Category:   inbox.CategoryDelivery,
			Priority:   inbox.PriorityNormal,
			TitleKey:   "inbox.runtime.recovered.title",
			ContentKey: "inbox.runtime.recovered.body",
			Params: map[string]any{
				"projectName":     firstNonEmpty(project.Name, project.Identifier),
				"applicationName": firstNonEmpty(application.Name, application.Identifier),
				"targetName":      firstNonEmpty(target.Name, target.Stage),
				"durationSeconds": durationSecs,
			},
			ProjectID:    target.ProjectID,
			ResourceType: "deployment_target",
			ResourceID:   target.ID,
			DeepLink:     actionLink,
			DedupKey:     coalesceKey,
		},
		true,
	)
}

func (r *Runner) loadCandidateRuntimeRules(ctx context.Context, projectID string) []model.NotificationRule {
	var rules []model.NotificationRule
	query := r.db.WithContext(ctx).Where("enabled = ?", true)
	if strings.TrimSpace(projectID) != "" {
		query = query.Where("project_id IN ?", []string{"", strings.TrimSpace(projectID)})
	} else {
		query = query.Where("project_id = ?", "")
	}
	_ = query.Order("created_at asc").Find(&rules).Error
	return rules
}

func buildRuntimeProbeEvent(
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	anomaly runtimeDetectedAnomaly,
	now time.Time,
) notification.Event {
	projectRef, appRef, targetRef := notificationEntityRefs(target.ProjectID, target.ApplicationID, target.ID, project, application, target)
	return notification.Event{
		Type:             anomaly.EventType,
		Severity:         anomaly.Severity,
		Project:          projectRef,
		Application:      appRef,
		DeploymentTarget: targetRef,
		OccurredAt:       now,
	}
}

func evaluateRuntimeIncidentAnomaly(
	existing *model.RuntimeAlertIncident,
	target model.DeploymentTarget,
	anomaly runtimeDetectedAnomaly,
	matchingRules []model.NotificationRule,
	now time.Time,
) runtimeAlertDecision {
	var incident model.RuntimeAlertIncident
	escalated := false
	if existing == nil {
		incidentID := id.New("rai")
		incident = model.RuntimeAlertIncident{
			ID:                    incidentID,
			ProjectID:             target.ProjectID,
			ApplicationID:         target.ApplicationID,
			DeploymentTargetID:    target.ID,
			ResourceID:            target.ID,
			IncidentKind:          anomaly.EventType,
			Status:                "suspect",
			Fingerprint:           anomaly.Fingerprint,
			ConsecutiveFailures:   1,
			FirstDetectedAt:       now,
			LastObservedAt:        now,
			RuleNotificationsJSON: "{}",
			InboxCoalesceKey:      "runtime_incident:" + incidentID,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
	} else {
		incident = *existing
		incident.ConsecutiveFailures++
		incident.LastObservedAt = now
		incident.UpdatedAt = now
		if runtimeAnomalyPriority(anomaly.EventType) < runtimeAnomalyPriority(incident.IncidentKind) {
			incident.IncidentKind = anomaly.EventType
			incident.Fingerprint = anomaly.Fingerprint
			escalated = true
		}
	}
	summaryBytes, _ := json.Marshal(anomaly)
	incident.SummaryJSON = string(summaryBytes)

	confirmRequired := anomaly.ConfirmAfter
	if confirmRequired <= 0 {
		confirmRequired = 1
	}
	if incident.Status != "active" && incident.ConsecutiveFailures < confirmRequired {
		return runtimeAlertDecision{
			Incident: incident,
		}
	}

	firstConfirmation := incident.Status != "active"
	incident.Status = "active"

	states := decodeRuleNotificationStates(incident.RuleNotificationsJSON)
	eligibleRuleIDs := make([]string, 0, len(matchingRules))
	maxReminderCount := 0

	for _, rule := range matchingRules {
		state := states[rule.ID]
		if state.NotifiedCount == 0 || escalated {
			state.NotifiedCount++
			state.LastNotifiedAt = now
			states[rule.ID] = state
			eligibleRuleIDs = append(eligibleRuleIDs, rule.ID)
			continue
		}
		if ruleReminderDue(state, rule.RemindIntervalMinutes, rule.MaxReminders, now) {
			state.NotifiedCount++
			state.LastNotifiedAt = now
			states[rule.ID] = state
			eligibleRuleIDs = append(eligibleRuleIDs, rule.ID)
			if rem := state.NotifiedCount - 1; rem > maxReminderCount {
				maxReminderCount = rem
			}
		}
	}

	personalState := states[personalRuleStateKey]
	includePersonal := false
	if personalState.NotifiedCount == 0 || escalated {
		personalState.NotifiedCount++
		personalState.LastNotifiedAt = now
		states[personalRuleStateKey] = personalState
		includePersonal = true
	} else if ruleReminderDue(personalState, defaultPersonalRemindIntervalMins, defaultPersonalMaxReminders, now) {
		personalState.NotifiedCount++
		personalState.LastNotifiedAt = now
		states[personalRuleStateKey] = personalState
		includePersonal = true
		if rem := personalState.NotifiedCount - 1; rem > maxReminderCount {
			maxReminderCount = rem
		}
	}

	incident.RuleNotificationsJSON = encodeRuleNotificationStates(states)
	shouldEmit := len(eligibleRuleIDs) > 0 || includePersonal
	isReminder := !firstConfirmation && !escalated && maxReminderCount > 0

	if shouldEmit {
		notifiedAt := now
		incident.LastNotifiedAt = &notifiedAt
		incident.NotificationCount++
		if len(eligibleRuleIDs) == 0 {
			eligibleRuleIDs = []string{noMatchingSharedRulePlaceholderID}
		}
	}

	return runtimeAlertDecision{
		Incident:             incident,
		ShouldEmitAlert:      shouldEmit,
		ShouldUpdateInbox:    firstConfirmation || escalated || shouldEmit,
		MarkInboxUnread:      firstConfirmation || escalated,
		IsReminder:           isReminder,
		ReminderCount:        maxReminderCount,
		EligibleRuleIDs:      eligibleRuleIDs,
		IncludePersonalOwner: includePersonal,
	}
}

func ruleReminderDue(state ruleAlertNotificationState, intervalMinutes int, maxReminders int, now time.Time) bool {
	if state.NotifiedCount <= 0 || maxReminders <= 0 || intervalMinutes <= 0 {
		return false
	}
	remindersSent := state.NotifiedCount - 1
	if remindersSent >= maxReminders {
		return false
	}
	nextDue := state.LastNotifiedAt.Add(time.Duration(intervalMinutes) * time.Minute)
	return !now.Before(nextDue)
}

func decodeRuleNotificationStates(raw string) map[string]ruleAlertNotificationState {
	out := map[string]ruleAlertNotificationState{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return out
	}
	_ = json.Unmarshal([]byte(trimmed), &out)
	if out == nil {
		return map[string]ruleAlertNotificationState{}
	}
	return out
}

func encodeRuleNotificationStates(states map[string]ruleAlertNotificationState) string {
	if len(states) == 0 {
		return "{}"
	}
	encoded, err := json.Marshal(states)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func runtimeAnomalyPriority(eventType string) int {
	switch eventType {
	case "runtime.pod_oom_killed":
		return 1
	case "runtime.pod_crash_loop":
		return 2
	case "runtime.pod_start_failed":
		return 3
	case "runtime.workload_degraded":
		return 4
	default:
		return 99
	}
}

func classifyRuntimeObservation(obs kubeprovider.WorkloadRuntimeObservation) (*runtimeDetectedAnomaly, bool) {
	switch obs.Status {
	case "ready", "scaled-to-zero":
		return nil, true
	case "progressing":
		return nil, false
	}

	// Inspect non-terminating pods to pick the highest-priority concrete failure
	var chosenPod *kubeprovider.RuntimePodSnapshot
	bestPriority := 999
	for i := range obs.Pods {
		pod := &obs.Pods[i]
		if pod.Terminating {
			continue
		}
		prio := podStatusAlertPriority(pod.Status)
		if prio < bestPriority {
			bestPriority = prio
			chosenPod = pod
		}
	}

	if chosenPod != nil && bestPriority < 999 {
		containerName, reason, detail, exitCode, restartCount, oom := extractOffendingContainerDetail(*chosenPod)
		eventType := "runtime.pod_start_failed"
		confirmAfter := 2
		switch chosenPod.Status {
		case kubeprovider.PodStatusOOMKilled:
			eventType = "runtime.pod_oom_killed"
			confirmAfter = 1
			oom = true
		case kubeprovider.PodStatusCrashLoopBackOff:
			eventType = "runtime.pod_crash_loop"
			confirmAfter = 1
		}
		summary := buildPodAnomalySummary(eventType, chosenPod.Name, containerName, reason, restartCount)
		return &runtimeDetectedAnomaly{
			EventType:        eventType,
			Severity:         notification.SeverityError,
			Fingerprint:      fmt.Sprintf("%s:%s:%s", eventType, containerName, reason),
			PodName:          chosenPod.Name,
			ContainerName:    containerName,
			Reason:           reason,
			ExitCode:         exitCode,
			RestartCount:     restartCount,
			OOMKilled:        oom,
			ReadyReplicas:    obs.Workload.ReadyReplicas,
			DesiredReplicas:  obs.Workload.DesiredReplicas,
			Summary:          summary,
			DiagnosticDetail: detail,
			ConfirmAfter:     confirmAfter,
		}, false
	}

	severity := notification.SeverityWarning
	if obs.Workload.DesiredReplicas > 0 && obs.Workload.ReadyReplicas == 0 {
		severity = notification.SeverityError
	}
	reason := firstNonEmpty(obs.ObservationCode, "ReplicaUnavailable")
	detail := strings.TrimSpace(obs.Workload.Message)
	if chosenPod != nil && strings.TrimSpace(chosenPod.Message) != "" {
		detail = chosenPod.Message
	}
	summary := fmt.Sprintf("工作负载副本未完全就绪 (%d/%d Ready)", obs.Workload.ReadyReplicas, obs.Workload.DesiredReplicas)
	return &runtimeDetectedAnomaly{
		EventType:        "runtime.workload_degraded",
		Severity:         severity,
		Fingerprint:      "runtime.workload_degraded:" + reason,
		Reason:           reason,
		ReadyReplicas:    obs.Workload.ReadyReplicas,
		DesiredReplicas:  obs.Workload.DesiredReplicas,
		Summary:          summary,
		DiagnosticDetail: detail,
		ConfirmAfter:     2,
	}, false
}

func podStatusAlertPriority(status string) int {
	switch status {
	case kubeprovider.PodStatusOOMKilled:
		return 1
	case kubeprovider.PodStatusCrashLoopBackOff:
		return 2
	case kubeprovider.PodStatusImagePullBackOff,
		kubeprovider.PodStatusErrImagePull,
		kubeprovider.PodStatusCreateContainerConfigError,
		kubeprovider.PodStatusCreateContainerError,
		kubeprovider.PodStatusInitError,
		kubeprovider.PodStatusFailed,
		kubeprovider.PodStatusUnschedulable,
		kubeprovider.PodStatusEvicted:
		return 3
	default:
		return 999
	}
}

func extractOffendingContainerDetail(pod kubeprovider.RuntimePodSnapshot) (
	containerName string,
	reason string,
	detail string,
	exitCode *int32,
	restartCount int32,
	oom bool,
) {
	restartCount = pod.RestartCount
	reason = strings.TrimSpace(pod.Reason)
	detail = strings.TrimSpace(pod.Message)

	for _, cs := range pod.ContainerStatuses {
		isOOM := strings.EqualFold(cs.CurrentState.Reason, "OOMKilled") ||
			(cs.LastTermination != nil && strings.EqualFold(cs.LastTermination.Reason, "OOMKilled"))
		isCrashOrWaitErr := cs.CurrentState.State == "waiting" && cs.CurrentState.Reason != "" && cs.CurrentState.Reason != "PodInitializing" && cs.CurrentState.Reason != "ContainerCreating"
		isTerminatedErr := cs.CurrentState.State == "terminated" && cs.CurrentState.ExitCode != nil && *cs.CurrentState.ExitCode != 0
		if !isOOM && !isCrashOrWaitErr && !isTerminatedErr && cs.LastTermination == nil {
			continue
		}
		containerName = cs.Name
		if cs.RestartCount > restartCount {
			restartCount = cs.RestartCount
		}
		if isOOM {
			oom = true
		}
		if cs.LastTermination != nil {
			codeCopy := cs.LastTermination.ExitCode
			exitCode = &codeCopy
			if cs.LastTermination.Reason != "" {
				reason = cs.LastTermination.Reason
			}
			if cs.LastTermination.Message != "" {
				detail = cs.LastTermination.Message
			}
		}
		if cs.CurrentState.ExitCode != nil {
			codeCopy := *cs.CurrentState.ExitCode
			exitCode = &codeCopy
		}
		if cs.CurrentState.Reason != "" && !isOOM {
			reason = cs.CurrentState.Reason
		} else if isOOM {
			reason = "OOMKilled"
		}
		if cs.CurrentState.Message != "" {
			detail = cs.CurrentState.Message
		}
		break
	}
	if reason == "" {
		reason = pod.Status
	}
	return containerName, reason, detail, exitCode, restartCount, oom
}

func buildPodAnomalySummary(eventType string, podName string, containerName string, reason string, restarts int32) string {
	target := podName
	if containerName != "" {
		target = podName + " (" + containerName + ")"
	}
	switch eventType {
	case "runtime.pod_oom_killed":
		return fmt.Sprintf("实例 %s 因内存超出限制 (OOMKilled) 被终止", target)
	case "runtime.pod_crash_loop":
		return fmt.Sprintf("实例 %s 进入崩溃重启循环 (CrashLoopBackOff，已重启 %d 次)", target, restarts)
	default:
		return fmt.Sprintf("实例 %s 启动失败 (%s)", target, firstNonEmpty(reason, "StartFailed"))
	}
}

func (r *Runner) emitRuntimeAnomalyNotification(
	ctx context.Context,
	decision runtimeAlertDecision,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	anomaly runtimeDetectedAnomaly,
	now time.Time,
) string {
	projectRef, appRef, targetRef := notificationEntityRefs(target.ProjectID, target.ApplicationID, target.ID, project, application, target)
	firstDetected := decision.Incident.FirstDetectedAt
	ownerUserID := ""
	if decision.IncludePersonalOwner {
		ownerUserID = r.notificationDeploymentTargetOwnerUserID(ctx, target.ProjectID, target.ApplicationID, target.ID, target)
	}
	eventID := id.New("evt")
	dedupSuffix := "initial:" + strconv.Itoa(decision.Incident.NotificationCount)
	if decision.IsReminder {
		dedupSuffix = "reminder:" + strconv.Itoa(decision.ReminderCount)
	}
	event := notification.Event{
		ID:               eventID,
		Type:             anomaly.EventType,
		Severity:         anomaly.Severity,
		Project:          projectRef,
		Application:      appRef,
		DeploymentTarget: targetRef,
		Runtime: notification.RuntimeIncidentContext{
			IncidentID:       decision.Incident.ID,
			IncidentKind:     anomaly.EventType,
			PodName:          anomaly.PodName,
			ContainerName:    anomaly.ContainerName,
			Reason:           anomaly.Reason,
			ExitCode:         anomaly.ExitCode,
			RestartCount:     anomaly.RestartCount,
			OOMKilled:        anomaly.OOMKilled,
			ReadyReplicas:    anomaly.ReadyReplicas,
			DesiredReplicas:  anomaly.DesiredReplicas,
			IsReminder:       decision.IsReminder,
			ReminderCount:    decision.ReminderCount,
			FirstDetectedAt:  &firstDetected,
			Summary:          anomaly.Summary,
			DiagnosticDetail: anomaly.DiagnosticDetail,
		},
		ResourceOwnerUserID: ownerUserID,
		EligibleRuleIDs:     decision.EligibleRuleIDs,
		CorrelationID:       decision.Incident.ID,
		DedupKey:            fmt.Sprintf("runtime:%s:%s", decision.Incident.ID, dedupSuffix),
		OccurredAt:          now,
		Links:               r.notificationLinks(target.ProjectID, target.ApplicationID, "deployments", "runtime", "targetId", target.ID),
		Message:             anomaly.Summary,
	}
	r.emitNotificationEvent(ctx, event)
	return eventID
}

func (r *Runner) publishRuntimeAnomalyInbox(
	ctx context.Context,
	decision runtimeAlertDecision,
	target model.DeploymentTarget,
	project model.Project,
	application model.Application,
	anomaly runtimeDetectedAnomaly,
) {
	priority := inbox.PriorityHigh
	if anomaly.EventType == "runtime.pod_oom_killed" || anomaly.EventType == "runtime.pod_crash_loop" {
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
	coalesceKey := firstNonEmpty(decision.Incident.InboxCoalesceKey, "runtime_incident:"+decision.Incident.ID)
	_ = inbox.NewService(r.db).PublishCoalescedToProjectRoles(
		ctx,
		target.ProjectID,
		[]string{authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper},
		inbox.PublishInput{
			Type:       anomaly.EventType,
			Category:   inbox.CategoryDelivery,
			Priority:   priority,
			TitleKey:   titleKey,
			ContentKey: "inbox.runtime.anomaly.body",
			Params: map[string]any{
				"projectName":     firstNonEmpty(project.Name, project.Identifier),
				"applicationName": firstNonEmpty(application.Name, application.Identifier),
				"targetName":      firstNonEmpty(target.Name, target.Stage),
				"podName":         anomaly.PodName,
				"reason":          anomaly.Reason,
				"restartCount":    anomaly.RestartCount,
				"readyReplicas":   anomaly.ReadyReplicas,
				"desiredReplicas": anomaly.DesiredReplicas,
				"occurrenceCount": decision.Incident.ConsecutiveFailures,
			},
			ProjectID:    target.ProjectID,
			ResourceType: "deployment_target",
			ResourceID:   target.ID,
			DeepLink:     runtimeTargetActionLink(target.ProjectID, target.ApplicationID, target.ID),
			DedupKey:     coalesceKey,
		},
		decision.MarkInboxUnread,
	)
}

func runtimeTargetActionLink(projectID string, applicationID string, targetID string) string {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(applicationID) == "" {
		return ""
	}
	base := fmt.Sprintf("/projects/%s/apps/%s?tab=deployments", url.PathEscape(projectID), url.PathEscape(applicationID))
	if strings.TrimSpace(targetID) != "" {
		base += "#targetId=" + url.QueryEscape(targetID)
	}
	return base
}

func (r *Runner) patrolGatewayCertificates(ctx context.Context) error {
	now := time.Now()
	if now.Minute()%certificatePatrolIntervalMinutes != 0 {
		return nil
	}
	return r.patrolGatewayCertificatesAt(ctx, now)
}

func (r *Runner) patrolGatewayCertificatesAt(ctx context.Context, now time.Time) error {
	if r.db == nil {
		return nil
	}
	db := r.db.WithContext(ctx)
	var routes []model.GatewayRoute
	err := db.Table("gateway_routes").
		Select("gateway_routes.*").
		Joins("JOIN applications ON applications.id = gateway_routes.application_id AND applications.deleted_at IS NULL AND applications.delete_status = ?", "active").
		Joins("JOIN projects ON projects.id = gateway_routes.project_id AND projects.deleted_at IS NULL").
		Where("gateway_routes.deleted_at IS NULL AND gateway_routes.enabled = ? AND gateway_routes.delete_status = ? AND gateway_routes.tls_mode = ?", true, "active", "http-challenge").
		Order("gateway_routes.updated_at desc").
		Limit(certificatePatrolRouteBatchLimit).
		Find(&routes).Error
	if err != nil || len(routes) == 0 {
		return err
	}

	for _, route := range routes {
		if strings.TrimSpace(route.DeploymentTargetID) == "" {
			continue
		}
		var project model.Project
		if err := db.First(&project, "id = ?", route.ProjectID).Error; err != nil {
			continue
		}
		var target model.DeploymentTarget
		if err := db.First(&target, "id = ? AND project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
			continue
		}
		namespace := deploymentNamespace(project)
		snapshot, configured, observeErr := r.gatewayCertificateSnapshot(ctx, route, project, target, namespace)
		if observeErr != nil || !configured {
			continue
		}
		cluster, clusterErr := r.runtimeClusterForDeploymentTarget(ctx, target)
		if clusterErr == nil {
			route.CertificateIssuerKind = gatewayCertificateIssuerKind(cluster)
			route.CertificateIssuerName = gatewayCertificateIssuerName(cluster, r.certManagerClusterIssuer)
		}
		route.CertificateStatus = snapshot.Phase
		route.CertificateMessage = snapshot.Message
		route.CertificateNotAfter = snapshot.NotAfter

		isExpired := snapshot.Phase == kubeprovider.CertificateExpired ||
			(snapshot.NotAfter != nil && !snapshot.NotAfter.After(now))
		if isExpired {
			notAfterUnix := int64(0)
			if snapshot.NotAfter != nil {
				notAfterUnix = snapshot.NotAfter.UTC().Unix()
			}
			r.emitPatrolledCertificateEvent(
				ctx,
				route,
				kubeprovider.CertificateExpired,
				firstNonEmpty(snapshot.Message, fmt.Sprintf("TLS certificate for %s has expired", route.Host)),
				fmt.Sprintf("certificate:%s:expired:%d", route.ID, notAfterUnix),
				now,
			)
			continue
		}
		if snapshot.Phase == kubeprovider.CertificateFailed {
			r.emitPatrolledCertificateEvent(
				ctx,
				route,
				kubeprovider.CertificateFailed,
				firstNonEmpty(snapshot.Message, fmt.Sprintf("TLS certificate issuance failed for %s", route.Host)),
				certificateEventDedupKey(route, kubeprovider.CertificateFailed),
				now,
			)
		}
	}
	return nil
}

func (r *Runner) emitPatrolledCertificateEvent(
	ctx context.Context,
	route model.GatewayRoute,
	status string,
	message string,
	dedupKey string,
	now time.Time,
) {
	project, application, target := r.notificationContext(route.ProjectID, route.ApplicationID, route.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(route.ProjectID, route.ApplicationID, route.DeploymentTargetID, project, application, target)
	r.emitNotificationEvent(ctx, notification.Event{
		Type:             "certificate." + status,
		Severity:         notificationSeverity(status),
		Project:          projectRef,
		Application:      applicationRef,
		DeploymentTarget: targetRef,
		Gateway: notification.GatewayContext{
			ID:     route.ID,
			Domain: route.Host,
			Path:   route.Path,
			Status: "active",
		},
		Certificate: notification.CertificateContext{
			RouteID:    route.ID,
			Host:       route.Host,
			Status:     status,
			Message:    strings.TrimSpace(message),
			NotAfter:   route.CertificateNotAfter,
			IssuerKind: route.CertificateIssuerKind,
			IssuerName: route.CertificateIssuerName,
		},
		ResourceOwnerUserID: route.CreatedBy,
		CorrelationID:       route.ID,
		DedupKey:            dedupKey,
		OccurredAt:          now,
		Links:               r.notificationLinks(route.ProjectID, route.ApplicationID, "gateway", "certificate", "", ""),
		Message:             firstNonEmpty(message, "Certificate "+status),
	})
}
