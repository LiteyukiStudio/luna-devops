package worker

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/imageref"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

func (r *Runner) emitNotificationEvent(ctx context.Context, event notification.Event) {
	if _, err := (notification.Service{DB: r.db, Enqueuer: r.taskClient}).Emit(ctx, event); err != nil {
		telemetry.RecordError(ctx, "notification.event_emit_failed", err,
			slog.String("notification.event_type", event.Type))
	}
}

func (r *Runner) emitBuildFailed(ctx context.Context, run model.BuildRun, message string) {
	r.emitBuildEvent(ctx, run, "failed", message)
}

func (r *Runner) emitBuildEvent(ctx context.Context, run model.BuildRun, status string, message string) {
	project, application, target := r.notificationContext(run.ProjectID, run.ApplicationID, run.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(run.ProjectID, run.ApplicationID, run.DeploymentTargetID, project, application, target)
	r.emitNotificationEvent(ctx, notification.Event{
		Type:             "build." + status,
		Severity:         notificationSeverity(status),
		Project:          projectRef,
		Application:      applicationRef,
		DeploymentTarget: targetRef,
		Build: notification.BuildContext{
			ID:      run.ID,
			Status:  status,
			Message: strings.TrimSpace(message),
			Image:   r.notificationBuildImageRef(run),
			GitRef:  firstNonEmpty(run.SourceBranch, run.SourceTag, run.SourceCommit),
			GitSHA:  run.SourceCommit,
		},
		Actor: r.notificationActor(ctx, run.CreatedBy, run.TriggeredByName, run.TriggeredByEmail),
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

func (r *Runner) notificationBuildImageRef(run model.BuildRun) string {
	if imageRef := strings.TrimSpace(run.ImageRef); imageRef != "" {
		return imageRef
	}
	var registry model.ArtifactRegistry
	if r.db != nil && strings.TrimSpace(run.TargetRegistryID) != "" {
		_ = r.db.First(&registry, "id = ?", run.TargetRegistryID).Error
	}
	return imageref.BuildImageRef(registry, run)
}

func (r *Runner) emitReleaseFailed(ctx context.Context, release model.Release, message string) {
	r.emitReleaseEvent(ctx, release, "failed", message)
}

func (r *Runner) emitReleaseEvent(ctx context.Context, release model.Release, status string, message string) {
	project, application, target := r.notificationContext(release.ProjectID, release.ApplicationID, release.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(release.ProjectID, release.ApplicationID, release.DeploymentTargetID, project, application, target)
	r.emitNotificationEvent(ctx, notification.Event{
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
		Actor: r.notificationActor(ctx, release.CreatedBy, "", ""),
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
		Links:         r.notificationLinks(release.ProjectID, release.ApplicationID, "deployments", "release", "", ""),
		Message:       firstNonEmpty(message, "Release "+status),
	})
}

func (r *Runner) emitHookFailed(ctx context.Context, run model.HookRun, message string) {
	r.emitHookEvent(ctx, run, "failed", message)
}

func (r *Runner) emitHookEvent(ctx context.Context, run model.HookRun, status string, message string) {
	project, application, target := r.notificationContext(run.ProjectID, run.ApplicationID, run.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(run.ProjectID, run.ApplicationID, run.DeploymentTargetID, project, application, target)
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
		Actor:               r.notificationHookActor(ctx, run),
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
	project, application, target := r.notificationContext(route.ProjectID, route.ApplicationID, route.DeploymentTargetID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(route.ProjectID, route.ApplicationID, route.DeploymentTargetID, project, application, target)
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
		DedupKey:            certificateEventDedupKey(route, status),
		OccurredAt:          time.Now(),
		Links:               r.notificationLinks(route.ProjectID, route.ApplicationID, "gateway", "certificate", "", ""),
		Message:             firstNonEmpty(message, "Certificate "+status),
	})
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

func (r *Runner) notificationContext(projectID string, applicationID string, targetID string) (model.Project, model.Application, model.DeploymentTarget) {
	var project model.Project
	var application model.Application
	var target model.DeploymentTarget
	_ = r.db.First(&project, "id = ?", projectID).Error
	if strings.TrimSpace(applicationID) != "" {
		_ = r.db.First(&application, "id = ?", applicationID).Error
	}
	if strings.TrimSpace(targetID) != "" {
		_ = r.db.First(&target, "id = ?", targetID).Error
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
