package worker

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/imageref"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"unicode/utf8"
)

func (r *Runner) emitNotificationEvent(ctx context.Context, event notification.Event) {
	_ = r.emitNotificationEventError(ctx, event)
}

func (r *Runner) emitNotificationEventError(ctx context.Context, event notification.Event) error {
	var enqueuer notification.DeliveryEnqueuer
	if r.taskClient != nil {
		enqueuer = r.taskClient
	}
	if r.notificationDeliveryEnqueuer != nil {
		enqueuer = r.notificationDeliveryEnqueuer
	}
	if _, err := (notification.Service{DB: r.db, Enqueuer: enqueuer}).Emit(ctx, event); err != nil {
		telemetry.RecordError(ctx, "notification.event_emit_failed", err,
			slog.String("notification.event_type", event.Type))
		return err
	}
	return nil
}

func (r *Runner) emitBuildFailed(ctx context.Context, run model.BuildRun, message string) {
	r.emitBuildFailedWithAI(ctx, run, message, nil)
}

func (r *Runner) emitBuildFailedWithAI(ctx context.Context, run model.BuildRun, message string, aiDiag *notification.AIDiagnosisContext) {
	_ = r.emitBuildEventWithAIError(ctx, run, "failed", message, aiDiag)
}

func (r *Runner) emitBuildEvent(ctx context.Context, run model.BuildRun, status string, message string) {
	r.emitBuildEventWithAI(ctx, run, status, message, nil)
}

func (r *Runner) emitBuildEventWithAI(ctx context.Context, run model.BuildRun, status string, message string, aiDiag *notification.AIDiagnosisContext) {
	_ = r.emitBuildEventWithAIError(ctx, run, status, message, aiDiag)
}

func (r *Runner) emitBuildEventWithAIError(ctx context.Context, run model.BuildRun, status string, message string, aiDiag *notification.AIDiagnosisContext) error {
	project, application, target := r.notificationContext(ctx, run.ProjectID, run.ApplicationID, run.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(run.ProjectID, run.ApplicationID, run.DeploymentTargetID, project, application, target)
	if status == "failed" || status == "timeout" {
		r.publishBuildFailureInboxWithContext(ctx, run, status, project, application, target, message, aiDiag)
	}
	return r.emitNotificationEventError(ctx, notification.Event{
		Type:             "build." + status,
		Severity:         notificationSeverity(status),
		Project:          projectRef,
		Application:      applicationRef,
		DeploymentTarget: targetRef,
		Build: notification.BuildContext{
			ID:      run.ID,
			Status:  status,
			Message: strings.TrimSpace(message),
			Image:   r.notificationBuildImageRef(ctx, run),
			GitRef:  firstNonEmpty(run.SourceBranch, run.SourceTag, run.SourceCommit),
			GitSHA:  run.SourceCommit,
		},
		AIDiagnosis: aiDiag,
		Actor:       r.notificationActor(ctx, run.CreatedBy, run.TriggeredByName, run.TriggeredByEmail),
		ResourceOwnerUserID: r.notificationDeploymentTargetOwnerUserID(
			ctx,
			run.ProjectID,
			run.ApplicationID,
			run.DeploymentTargetID,
			target,
		),
		CorrelationID: run.ID,
		DedupKey:      "build:" + run.ID + ":" + status,
		OccurredAt:    time.Now(),
		Links:         r.notificationLinks(run.ProjectID, run.ApplicationID, "builds", "build", "buildRunId", run.ID),
		Message:       firstNonEmpty(message, "Build "+status),
	})
}

func (r *Runner) notificationBuildImageRef(ctx context.Context, run model.BuildRun) string {
	if imageRef := strings.TrimSpace(run.ImageRef); imageRef != "" {
		return imageRef
	}
	var registry model.ArtifactRegistry
	if r.db != nil && strings.TrimSpace(run.TargetRegistryID) != "" {
		_ = r.db.WithContext(ctx).First(&registry, "id = ?", run.TargetRegistryID).Error
	}
	return imageref.BuildImageRef(registry, run)
}

func (r *Runner) emitReleaseFailed(ctx context.Context, release model.Release, message string) {
	r.emitReleaseEventWithAI(ctx, release, "failed", message, nil)
}

func (r *Runner) emitReleaseEvent(ctx context.Context, release model.Release, status string, message string) {
	r.emitReleaseEventWithAI(ctx, release, status, message, nil)
}

func (r *Runner) emitReleaseEventWithAI(ctx context.Context, release model.Release, status string, message string, aiDiag *notification.AIDiagnosisContext) {
	_ = r.emitReleaseEventWithAIError(ctx, release, status, message, aiDiag)
}

func (r *Runner) emitReleaseEventWithAIError(ctx context.Context, release model.Release, status string, message string, aiDiag *notification.AIDiagnosisContext) error {
	project, application, target := r.notificationContext(ctx, release.ProjectID, release.ApplicationID, release.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(release.ProjectID, release.ApplicationID, release.DeploymentTargetID, project, application, target)
	if status == "failed" {
		r.publishReleaseFailureInboxWithContext(ctx, release, project, application, target, message, aiDiag)
	}
	focusKey := ""
	if strings.TrimSpace(release.ID) != "" {
		focusKey = "releaseId"
	}
	return r.emitNotificationEventError(ctx, notification.Event{
		Type:             "release." + status,
		Severity:         notificationSeverity(status),
		Project:          projectRef,
		Application:      applicationRef,
		DeploymentTarget: targetRef,
		Release: notification.ReleaseContext{
			ID:       release.ID,
			Status:   status,
			Revision: release.Revision,
			ImageRef: release.ImageRef,
			Message:  strings.TrimSpace(message),
		},
		AIDiagnosis: aiDiag,
		Actor:       r.notificationActor(ctx, release.CreatedBy, "", ""),
		ResourceOwnerUserID: r.notificationDeploymentTargetOwnerUserID(
			ctx,
			release.ProjectID,
			release.ApplicationID,
			release.DeploymentTargetID,
			target,
		),
		CorrelationID: release.ID,
		DedupKey:      "release:" + release.ID + ":" + status,
		OccurredAt:    time.Now(),
		Links:         r.notificationLinks(release.ProjectID, release.ApplicationID, "deployments", "release", focusKey, release.ID),
		Message:       firstNonEmpty(message, "Release "+status),
	})
}

func (r *Runner) emitHookFailed(ctx context.Context, run model.HookRun, message string) {
	r.emitHookEvent(ctx, run, "failed", message)
}

func (r *Runner) emitHookEvent(ctx context.Context, run model.HookRun, status string, message string) {
	project, application, target := r.notificationContext(ctx, run.ProjectID, run.ApplicationID, run.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(run.ProjectID, run.ApplicationID, run.DeploymentTargetID, project, application, target)
	actor := r.notificationHookActor(ctx, run)
	if status == "failed" {
		r.publishProjectDeliveryInbox(ctx, run.ProjectID, []string{actor.ID}, inbox.PublishInput{
			Type:         "hook.failed",
			Category:     inbox.CategoryDelivery,
			Priority:     inbox.PriorityHigh,
			ActorID:      actor.ID,
			ProjectID:    run.ProjectID,
			ResourceType: "hook_run",
			ResourceID:   run.ID,
			TitleKey:     "inbox.delivery.hookFailed.title",
			ContentKey:   "inbox.delivery.hookFailed.body",
			Params: map[string]any{
				"projectName":     firstNonEmpty(project.Name, project.Identifier, run.ProjectID),
				"applicationName": firstNonEmpty(application.Name, application.Identifier, run.ApplicationID, "-"),
				"hookName":        firstNonEmpty(run.Name, run.ID),
				"phase":           firstNonEmpty(run.Phase, "-"),
				"reason":          truncateInboxReason(telemetry.RedactText(firstNonEmpty(strings.TrimSpace(message), "hook_failed"))),
			},
			DeepLink: relativeResourceDeepLink(run.ProjectID, run.ApplicationID, "deployments", "", ""),
			GroupKey: "hook:" + firstNonEmpty(run.ApplicationID, run.ProjectID),
			DedupKey: "delivery_hook:" + run.ID,
		})
	}
	r.emitNotificationEvent(ctx, notification.Event{
		Type:             "hook." + status,
		Severity:         notificationSeverity(status),
		Project:          projectRef,
		Application:      applicationRef,
		DeploymentTarget: targetRef,
		Hook: notification.HookContext{
			ID:      run.ID,
			Name:    run.Name,
			Phase:   run.Phase,
			Status:  status,
			Message: strings.TrimSpace(message),
		},
		Actor:               actor,
		ResourceOwnerUserID: r.notificationHookOwnerUserID(ctx, run, target),
		CorrelationID:       firstNonEmpty(run.ReleaseID, run.BuildRunID, run.ID),
		DedupKey:            "hook:" + run.ID + ":" + status,
		OccurredAt:          time.Now(),
		Links:               r.notificationLinks(run.ProjectID, run.ApplicationID, "deployments", "hook", "", ""),
		Message:             firstNonEmpty(message, "Hook "+status),
	})
}

func (r *Runner) emitGatewayApplyFailed(ctx context.Context, route model.GatewayRoute, actorID string, message string) {
	r.emitGatewayEvent(ctx, route, actorID, "apply_failed", message)
}

func (r *Runner) emitGatewayEvent(ctx context.Context, route model.GatewayRoute, actorID string, status string, message string) {
	project, application, target := r.notificationContext(ctx, route.ProjectID, route.ApplicationID, route.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(route.ProjectID, route.ApplicationID, route.DeploymentTargetID, project, application, target)
	if status == "apply_failed" {
		r.publishProjectDeliveryInbox(ctx, route.ProjectID, []string{actorID, route.CreatedBy}, inbox.PublishInput{
			Type:         "gateway.apply_failed",
			Category:     inbox.CategoryDelivery,
			Priority:     inbox.PriorityHigh,
			ActorID:      strings.TrimSpace(actorID),
			ProjectID:    route.ProjectID,
			ResourceType: "gateway_route",
			ResourceID:   route.ID,
			TitleKey:     "inbox.delivery.gatewayFailed.title",
			ContentKey:   "inbox.delivery.gatewayFailed.body",
			Params: map[string]any{
				"projectName":     firstNonEmpty(project.Name, project.Identifier, route.ProjectID),
				"applicationName": firstNonEmpty(application.Name, application.Identifier, route.ApplicationID, "-"),
				"domain":          firstNonEmpty(route.Host, route.ID),
				"reason":          truncateInboxReason(telemetry.RedactText(firstNonEmpty(strings.TrimSpace(message), "gateway_apply_failed"))),
			},
			DeepLink: relativeResourceDeepLink(route.ProjectID, route.ApplicationID, "gateway", "", ""),
			GroupKey: "gateway:" + route.ID,
			DedupKey: "delivery_" + gatewayEventDedupKey(route, status),
		})
	}
	r.emitNotificationEvent(ctx, notification.Event{
		Type:             "gateway." + status,
		Severity:         notificationSeverity(status),
		Project:          projectRef,
		Application:      applicationRef,
		DeploymentTarget: targetRef,
		Gateway: notification.GatewayContext{
			ID:      route.ID,
			Domain:  route.Host,
			Path:    route.Path,
			Status:  status,
			Message: strings.TrimSpace(message),
		},
		Actor:               r.notificationActor(ctx, actorID, "", ""),
		ResourceOwnerUserID: route.CreatedBy,
		CorrelationID:       route.ID,
		DedupKey:            gatewayEventDedupKey(route, status),
		OccurredAt:          time.Now(),
		Links:               r.notificationLinks(route.ProjectID, route.ApplicationID, "gateway", "gateway", "", ""),
		Message:             firstNonEmpty(message, "Gateway route "+status),
	})
}

func (r *Runner) emitCertificateEvent(ctx context.Context, route model.GatewayRoute, actorID string, status string, message string) {
	project, application, target := r.notificationContext(ctx, route.ProjectID, route.ApplicationID, route.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(route.ProjectID, route.ApplicationID, route.DeploymentTargetID, project, application, target)
	dedupKey := certificateEventDedupKey(route, status)
	r.publishCertificateFailureInbox(ctx, route, actorID, status, message, dedupKey, project, application)
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
			Status: route.Status,
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
		Actor:               r.notificationActor(ctx, actorID, "", ""),
		ResourceOwnerUserID: route.CreatedBy,
		CorrelationID:       route.ID,
		DedupKey:            dedupKey,
		OccurredAt:          time.Now(),
		Links:               r.notificationLinks(route.ProjectID, route.ApplicationID, "gateway", "certificate", "", ""),
		Message:             firstNonEmpty(message, "Certificate "+status),
	})
}

func (r *Runner) publishCertificateFailureInbox(
	ctx context.Context,
	route model.GatewayRoute,
	actorID string,
	status string,
	message string,
	dedupKey string,
	project model.Project,
	application model.Application,
) {
	if status != "failed" && status != "expired" {
		return
	}
	r.publishProjectDeliveryInbox(ctx, route.ProjectID, []string{actorID, route.CreatedBy}, inbox.PublishInput{
		Type:         "certificate." + status,
		Category:     inbox.CategoryDelivery,
		Priority:     inbox.PriorityHigh,
		ActorID:      strings.TrimSpace(actorID),
		ProjectID:    route.ProjectID,
		ResourceType: "gateway_route",
		ResourceID:   route.ID,
		TitleKey:     "inbox.delivery.certificateFailed.title",
		ContentKey:   "inbox.delivery.certificateFailed.body",
		Params: map[string]any{
			"projectName":     firstNonEmpty(project.Name, project.Identifier, route.ProjectID),
			"applicationName": firstNonEmpty(application.Name, application.Identifier, route.ApplicationID, "-"),
			"domain":          firstNonEmpty(route.Host, route.ID),
			"reason":          truncateInboxReason(telemetry.RedactText(firstNonEmpty(strings.TrimSpace(message), "certificate_"+status))),
		},
		DeepLink: relativeResourceDeepLink(route.ProjectID, route.ApplicationID, "gateway", "", ""),
		GroupKey: "certificate:" + route.ID,
		DedupKey: "delivery_" + strings.TrimSpace(dedupKey),
	})
}

func (r *Runner) publishBuildFailureInboxOnly(ctx context.Context, run model.BuildRun, message string, aiDiag *notification.AIDiagnosisContext) {
	if r.db == nil {
		return
	}
	project, application, target := r.notificationContext(ctx, run.ProjectID, run.ApplicationID, run.DeploymentTargetID)
	status := strings.TrimSpace(run.Status)
	if status == "" {
		status = "failed"
	}
	r.publishBuildFailureInboxWithContext(ctx, run, status, project, application, target, message, aiDiag)
}

func (r *Runner) publishBuildFailureInboxWithContext(
	ctx context.Context,
	run model.BuildRun,
	status string,
	project model.Project,
	application model.Application,
	target model.DeploymentTarget,
	message string,
	aiDiag *notification.AIDiagnosisContext,
) {
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.ProjectID) == "" {
		return
	}
	effectiveStatus := firstNonEmpty(strings.TrimSpace(run.Status), strings.TrimSpace(status), "failed")
	if !isDiagnosableBuildFailure(effectiveStatus, run.FailureCode) {
		return
	}
	inboxType := "build.failed"
	if effectiveStatus == "timeout" || strings.TrimSpace(status) == "timeout" {
		inboxType = "build.timeout"
	}
	params := map[string]any{
		"projectName":     firstNonEmpty(project.Name, project.Identifier, run.ProjectID),
		"applicationName": firstNonEmpty(application.Name, application.Identifier, run.ApplicationID, "-"),
		"targetName":      firstNonEmpty(target.Name, target.Stage, "-"),
		"gitRef":          firstNonEmpty(run.SourceBranch, run.SourceTag, shortInboxCommit(run.SourceCommit), "-"),
		"reason":          truncateInboxReason(telemetry.RedactText(firstNonEmpty(strings.TrimSpace(message), strings.TrimSpace(run.FailureCode), "build_failed"))),
	}
	attachRedactedAIDiagnosisInboxParams(params, aiDiag)
	r.publishProjectDeliveryInbox(ctx, run.ProjectID, []string{run.CreatedBy}, inbox.PublishInput{
		Type:         inboxType,
		Category:     inbox.CategoryDelivery,
		Priority:     inbox.PriorityHigh,
		ActorID:      strings.TrimSpace(run.CreatedBy),
		ProjectID:    run.ProjectID,
		ResourceType: "build_run",
		ResourceID:   run.ID,
		TitleKey:     "inbox.delivery.buildFailed.title",
		ContentKey:   "inbox.delivery.buildFailed.body",
		Params:       params,
		DeepLink:     relativeResourceDeepLink(run.ProjectID, run.ApplicationID, "builds", "buildRunId", run.ID),
		GroupKey:     "build:" + firstNonEmpty(run.ApplicationID, run.ProjectID),
		DedupKey:     "delivery_build:" + run.ID,
	})
}

func (r *Runner) publishReleaseFailureInboxOnly(ctx context.Context, release model.Release, message string, aiDiag *notification.AIDiagnosisContext) {
	if r.db == nil {
		return
	}
	project, application, target := r.notificationContext(ctx, release.ProjectID, release.ApplicationID, release.DeploymentTargetID)
	r.publishReleaseFailureInboxWithContext(ctx, release, project, application, target, message, aiDiag)
}

func (r *Runner) publishReleaseFailureInboxWithContext(
	ctx context.Context,
	release model.Release,
	project model.Project,
	application model.Application,
	target model.DeploymentTarget,
	message string,
	aiDiag *notification.AIDiagnosisContext,
) {
	if strings.TrimSpace(release.ID) == "" || strings.TrimSpace(release.ProjectID) == "" {
		return
	}
	effectiveStatus := firstNonEmpty(strings.TrimSpace(release.Status), "failed")
	if !isDiagnosableReleaseFailure(effectiveStatus, release.FailureCode) {
		return
	}
	params := map[string]any{
		"projectName":     firstNonEmpty(project.Name, project.Identifier, release.ProjectID),
		"applicationName": firstNonEmpty(application.Name, application.Identifier, release.ApplicationID, "-"),
		"targetName":      firstNonEmpty(target.Name, target.Stage, "-"),
		"revision":        release.Revision,
		"reason":          truncateInboxReason(telemetry.RedactText(firstNonEmpty(strings.TrimSpace(message), strings.TrimSpace(release.FailureCode), "release_failed"))),
	}
	attachRedactedAIDiagnosisInboxParams(params, aiDiag)
	r.publishProjectDeliveryInbox(ctx, release.ProjectID, []string{release.CreatedBy}, inbox.PublishInput{
		Type:         "release.failed",
		Category:     inbox.CategoryDelivery,
		Priority:     inbox.PriorityHigh,
		ActorID:      strings.TrimSpace(release.CreatedBy),
		ProjectID:    release.ProjectID,
		ResourceType: "release",
		ResourceID:   release.ID,
		TitleKey:     "inbox.delivery.releaseFailed.title",
		ContentKey:   "inbox.delivery.releaseFailed.body",
		Params:       params,
		DeepLink:     relativeResourceDeepLink(release.ProjectID, release.ApplicationID, "deployments", "releaseId", release.ID),
		GroupKey:     "release:" + firstNonEmpty(release.DeploymentTargetID, release.ProjectID),
		DedupKey:     "delivery_release:" + release.ID,
	})
}

func attachRedactedAIDiagnosisInboxParams(params map[string]any, aiDiag *notification.AIDiagnosisContext) {
	if params == nil || aiDiag == nil {
		return
	}
	summary := strings.TrimSpace(telemetry.RedactText(aiDiag.RootCauseSummary))
	if summary == "" {
		return
	}
	params["aiRootCauseSummary"] = summary
	if len(aiDiag.Suggestions) > 0 {
		seen := make(map[string]struct{}, len(aiDiag.Suggestions))
		clean := make([]string, 0, len(aiDiag.Suggestions))
		for _, raw := range aiDiag.Suggestions {
			item := strings.TrimSpace(telemetry.RedactText(raw))
			if item == "" {
				continue
			}
			if _, dup := seen[item]; dup {
				continue
			}
			seen[item] = struct{}{}
			clean = append(clean, item)
		}
		if len(clean) > 0 {
			params["aiSuggestions"] = clean
		}
	}
}

func (r *Runner) publishProjectDeliveryInbox(ctx context.Context, projectID string, extraActors []string, input inbox.PublishInput) {
	if r == nil || r.db == nil || strings.TrimSpace(projectID) == "" {
		return
	}
	roles := []string{authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper}
	if err := inbox.NewService(r.db).PublishCoalescedToProjectRolesAndUsers(ctx, projectID, roles, extraActors, input, true); err != nil {
		telemetry.RecordError(ctx, "inbox.delivery_failure_publish_failed", err,
			slog.String("inbox.type", strings.TrimSpace(input.Type)),
		)
	}
}

func relativeResourceDeepLink(projectID string, applicationID string, tab string, focusKey string, focusID string) string {
	projectID = strings.TrimSpace(projectID)
	applicationID = strings.TrimSpace(applicationID)
	if projectID == "" {
		return ""
	}
	if applicationID == "" {
		return "/projects/" + url.PathEscape(projectID)
	}
	link := "/projects/" + url.PathEscape(projectID) + "/apps/" + url.PathEscape(applicationID)
	if trimmedTab := strings.TrimSpace(tab); trimmedTab != "" {
		link += "?tab=" + url.QueryEscape(trimmedTab)
	}
	if trimmedKey := strings.TrimSpace(focusKey); trimmedKey != "" && strings.TrimSpace(focusID) != "" {
		link += "#" + url.QueryEscape(trimmedKey) + "=" + url.QueryEscape(strings.TrimSpace(focusID))
	}
	return link
}

func shortInboxCommit(commit string) string {
	trimmed := strings.TrimSpace(commit)
	if len(trimmed) > 8 {
		return trimmed[:8]
	}
	return trimmed
}

func truncateInboxReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if len(trimmed) <= 280 {
		return trimmed
	}
	end := 280
	for end > 0 && !utf8.RuneStart(trimmed[end]) {
		end--
	}
	return strings.ToValidUTF8(trimmed[:end], "") + "…"
}

func (r *Runner) notificationHookOwnerUserID(ctx context.Context, run model.HookRun, target model.DeploymentTarget) string {
	if targetID := strings.TrimSpace(run.DeploymentTargetID); targetID != "" {
		return r.notificationDeploymentTargetOwnerUserID(ctx, run.ProjectID, run.ApplicationID, targetID, target)
	}
	if strings.TrimSpace(run.HookConfigID) == "" {
		return ""
	}
	var config model.ProjectHookConfig
	if err := r.db.WithContext(ctx).First(&config, "id = ? and project_id = ?", run.HookConfigID, run.ProjectID).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(config.CreatedBy)
}

func (r *Runner) notificationDeploymentTargetOwnerUserID(
	ctx context.Context,
	projectID string,
	applicationID string,
	targetID string,
	target model.DeploymentTarget,
) string {
	projectID = strings.TrimSpace(projectID)
	applicationID = strings.TrimSpace(applicationID)
	targetID = strings.TrimSpace(targetID)
	if projectID == "" || applicationID == "" || targetID == "" {
		return ""
	}
	if strings.TrimSpace(target.ID) == targetID &&
		strings.TrimSpace(target.ProjectID) == projectID &&
		strings.TrimSpace(target.ApplicationID) == applicationID {
		return strings.TrimSpace(target.CreatedBy)
	}
	var owner struct {
		CreatedBy string
	}
	if err := r.db.WithContext(ctx).Unscoped().Model(&model.DeploymentTarget{}).
		Select("created_by").
		Where("id = ? and project_id = ? and application_id = ?", targetID, projectID, applicationID).
		Take(&owner).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(owner.CreatedBy)
}

func (r *Runner) notificationHookActor(ctx context.Context, run model.HookRun) notification.ActorContext {
	if buildRunID := strings.TrimSpace(run.BuildRunID); buildRunID != "" {
		var buildRun model.BuildRun
		if err := r.db.WithContext(ctx).First(&buildRun, "id = ? and project_id = ?", buildRunID, run.ProjectID).Error; err == nil {
			return r.notificationActor(ctx, buildRun.CreatedBy, buildRun.TriggeredByName, buildRun.TriggeredByEmail)
		}
	}
	if releaseID := strings.TrimSpace(run.ReleaseID); releaseID != "" {
		var release model.Release
		if err := r.db.WithContext(ctx).First(&release, "id = ? and project_id = ?", releaseID, run.ProjectID).Error; err == nil {
			return r.notificationActor(ctx, release.CreatedBy, "", "")
		}
	}
	return notification.ActorContext{}
}

func (r *Runner) notificationActor(ctx context.Context, userID string, name string, email string) notification.ActorContext {
	actor := notification.ActorContext{ID: strings.TrimSpace(userID), Name: strings.TrimSpace(name), Email: strings.TrimSpace(email)}
	if actor.ID == "" || (actor.Name != "" && actor.Email != "") {
		return actor
	}
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", actor.ID).Error; err == nil {
		actor.Name = firstNonEmpty(actor.Name, user.Name)
		actor.Email = firstNonEmpty(actor.Email, user.Email)
	}
	return actor
}

func notificationSeverity(status string) string {
	switch status {
	case "failed", "apply_failed", "expired":
		return notification.SeverityError
	case "pending":
		return notification.SeverityWarning
	default:
		return notification.SeverityInfo
	}
}

func gatewayEventDedupKey(route model.GatewayRoute, status string) string {
	return "gateway:" + route.ID + ":" + gatewayRouteGeneration(route) + ":" + strings.TrimSpace(status)
}

func certificateEventDedupKey(route model.GatewayRoute, status string) string {
	return "certificate:" + route.ID + ":" + gatewayRouteGeneration(route) + ":" + strings.TrimSpace(status)
}

func gatewayRouteGeneration(route model.GatewayRoute) string {
	if route.UpdatedAt.IsZero() {
		return "0"
	}
	return strconv.FormatInt(route.UpdatedAt.UTC().UnixMicro(), 10)
}

func (r *Runner) notificationContext(ctx context.Context, projectID string, applicationID string, targetID string) (model.Project, model.Application, model.DeploymentTarget) {
	var project model.Project
	var application model.Application
	var target model.DeploymentTarget
	if r.db == nil {
		return project, application, target
	}
	db := r.db.WithContext(ctx)
	_ = db.First(&project, "id = ?", projectID).Error
	if strings.TrimSpace(applicationID) != "" {
		_ = db.First(&application, "id = ?", applicationID).Error
	}
	if strings.TrimSpace(targetID) != "" {
		_ = db.First(&target, "id = ?", targetID).Error
	}
	return project, application, target
}

func entityRef(id string, name string, identifier string) notification.EntityRef {
	return notification.EntityRef{ID: id, Name: name, Identifier: identifier}
}

func notificationEntityRefs(
	projectID string,
	applicationID string,
	targetID string,
	project model.Project,
	application model.Application,
	target model.DeploymentTarget,
) (notification.EntityRef, notification.EntityRef, notification.EntityRef) {
	return entityRef(strings.TrimSpace(projectID), project.Name, project.Identifier),
		entityRef(strings.TrimSpace(applicationID), application.Name, application.Identifier),
		entityRef(strings.TrimSpace(targetID), target.Name, target.Stage)
}

func (r *Runner) notificationLinks(projectID string, applicationID string, tab string, primaryKey string, focusKey string, focusID string) map[string]string {
	base := strings.TrimRight(strings.TrimSpace(r.publicBaseURL), "/")
	if base == "" {
		return nil
	}
	links := map[string]string{}
	if strings.TrimSpace(projectID) != "" {
		links["project"] = fmt.Sprintf("%s/projects/%s", base, url.PathEscape(projectID))
		links["primary"] = links["project"]
	}
	if strings.TrimSpace(projectID) != "" && strings.TrimSpace(applicationID) != "" {
		applicationLink := fmt.Sprintf("%s/projects/%s/apps/%s", base, url.PathEscape(projectID), url.PathEscape(applicationID))
		links["application"] = applicationLink
		links["primary"] = applicationLink
		if strings.TrimSpace(tab) != "" {
			tabLink := applicationLink + "?tab=" + url.QueryEscape(tab)
			if strings.TrimSpace(focusKey) != "" && strings.TrimSpace(focusID) != "" {
				tabLink += "#" + url.QueryEscape(focusKey) + "=" + url.QueryEscape(focusID)
			}
			links["primary"] = tabLink
			if strings.TrimSpace(primaryKey) != "" {
				links[primaryKey] = tabLink
			}
		}
	}
	return links
}
