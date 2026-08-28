package worker

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/imageref"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
)

func (r *Runner) emitNotificationEvent(ctx context.Context, event notification.Event) {
	if r.db == nil {
		return
	}
	_, _ = (notification.Service{DB: r.db, Enqueuer: r.taskClient}).Emit(ctx, event)
}

func (r *Runner) emitBuildFailed(ctx context.Context, run model.BuildRun, message string) {
	r.emitBuildEvent(ctx, run, "failed", message)
}

func (r *Runner) emitBuildEvent(ctx context.Context, run model.BuildRun, status string, message string) {
	project, application, target := r.notificationContext(run.ProjectID, run.ApplicationID, run.DeploymentTargetID)
	r.emitNotificationEvent(ctx, notification.Event{
		Type:             "build." + status,
		Severity:         notificationSeverity(status),
		Project:          entityRef(project.ID, project.Name, project.Identifier),
		Application:      entityRef(application.ID, application.Name, application.Identifier),
		DeploymentTarget: entityRef(target.ID, target.Name, target.Stage),
		Build: notification.BuildContext{
			ID:      run.ID,
			Status:  status,
			Message: strings.TrimSpace(message),
			Image:   r.notificationBuildImageRef(run),
			GitRef:  firstNonEmpty(run.SourceBranch, run.SourceTag, run.SourceCommit),
			GitSHA:  run.SourceCommit,
		},
		Actor:         r.notificationActor(ctx, run.CreatedBy, run.TriggeredByName, run.TriggeredByEmail),
		CorrelationID: run.ID,
		DedupKey:      "build:" + run.ID + ":" + status,
		OccurredAt:    time.Now(),
		Links:         r.notificationLinks(run.ProjectID, run.ApplicationID, "builds", "build"),
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
	r.emitNotificationEvent(ctx, notification.Event{
		Type:             "release." + status,
		Severity:         notificationSeverity(status),
		Project:          entityRef(project.ID, project.Name, project.Identifier),
		Application:      entityRef(application.ID, application.Name, application.Identifier),
		DeploymentTarget: entityRef(target.ID, target.Name, target.Stage),
		Release: notification.ReleaseContext{
			ID:       release.ID,
			Status:   status,
			Revision: release.Revision,
			ImageRef: release.ImageRef,
			Message:  strings.TrimSpace(message),
		},
		Actor:         r.notificationActor(ctx, release.CreatedBy, "", ""),
		CorrelationID: release.ID,
		DedupKey:      "release:" + release.ID + ":" + status,
		OccurredAt:    time.Now(),
		Links:         r.notificationLinks(release.ProjectID, release.ApplicationID, "deployments", "release"),
		Message:       firstNonEmpty(message, "Release "+status),
	})
}

func (r *Runner) emitHookFailed(ctx context.Context, run model.HookRun, message string) {
	r.emitHookEvent(ctx, run, "failed", message)
}

func (r *Runner) emitHookEvent(ctx context.Context, run model.HookRun, status string, message string) {
	project, application, target := r.notificationContext(run.ProjectID, run.ApplicationID, run.DeploymentTargetID)
	r.emitNotificationEvent(ctx, notification.Event{
		Type:             "hook." + status,
		Severity:         notificationSeverity(status),
		Project:          entityRef(project.ID, project.Name, project.Identifier),
		Application:      entityRef(application.ID, application.Name, application.Identifier),
		DeploymentTarget: entityRef(target.ID, target.Name, target.Stage),
		Hook: notification.HookContext{
			ID:      run.ID,
			Name:    run.Name,
			Phase:   run.Phase,
			Status:  status,
			Message: strings.TrimSpace(message),
		},
		Actor:         r.notificationHookActor(ctx, run),
		CorrelationID: firstNonEmpty(run.ReleaseID, run.BuildRunID, run.ID),
		DedupKey:      "hook:" + run.ID + ":" + status,
		OccurredAt:    time.Now(),
		Links:         r.notificationLinks(run.ProjectID, run.ApplicationID, "deployments", "hook"),
		Message:       firstNonEmpty(message, "Hook "+status),
	})
}

func (r *Runner) emitGatewayApplyFailed(ctx context.Context, route model.GatewayRoute, message string) {
	r.emitGatewayEvent(ctx, route, "apply_failed", message)
}

func (r *Runner) emitGatewayEvent(ctx context.Context, route model.GatewayRoute, status string, message string) {
	project, application, target := r.notificationContext(route.ProjectID, route.ApplicationID, route.DeploymentTargetID)
	r.emitNotificationEvent(ctx, notification.Event{
		Type:             "gateway." + status,
		Severity:         notificationSeverity(status),
		Project:          entityRef(project.ID, project.Name, project.Identifier),
		Application:      entityRef(application.ID, application.Name, application.Identifier),
		DeploymentTarget: entityRef(target.ID, target.Name, target.Stage),
		Gateway: notification.GatewayContext{
			ID:      route.ID,
			Domain:  route.Host,
			Path:    route.Path,
			Status:  status,
			Message: strings.TrimSpace(message),
		},
		Actor:         r.notificationActor(ctx, route.CreatedBy, "", ""),
		CorrelationID: route.ID,
		DedupKey:      "gateway:" + route.ID + ":" + status,
		OccurredAt:    time.Now(),
		Links:         r.notificationLinks(route.ProjectID, route.ApplicationID, "gateway", "gateway"),
		Message:       firstNonEmpty(message, "Gateway route "+status),
	})
}

func (r *Runner) emitCertificateEvent(ctx context.Context, route model.GatewayRoute, status string, message string) {
	project, application, target := r.notificationContext(route.ProjectID, route.ApplicationID, route.DeploymentTargetID)
	r.emitNotificationEvent(ctx, notification.Event{
		Type:             "certificate." + status,
		Severity:         notificationSeverity(status),
		Project:          entityRef(project.ID, project.Name, project.Identifier),
		Application:      entityRef(application.ID, application.Name, application.Identifier),
		DeploymentTarget: entityRef(target.ID, target.Name, target.Stage),
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
		Actor:         r.notificationActor(ctx, route.CreatedBy, "", ""),
		CorrelationID: route.ID,
		DedupKey:      "certificate:" + route.ID + ":" + status + ":" + certificateVersion(route.CertificateNotAfter),
		OccurredAt:    time.Now(),
		Links:         r.notificationLinks(route.ProjectID, route.ApplicationID, "gateway", "certificate"),
		Message:       firstNonEmpty(message, "Certificate "+status),
	})
}

func (r *Runner) notificationHookActor(ctx context.Context, run model.HookRun) notification.ActorContext {
	if r.db == nil {
		return notification.ActorContext{}
	}
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
	if actor.ID == "" || r.db == nil || (actor.Name != "" && actor.Email != "") {
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

func certificateVersion(notAfter *time.Time) string {
	if notAfter == nil {
		return "none"
	}
	return notAfter.UTC().Format(time.RFC3339)
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

func (r *Runner) notificationLinks(projectID string, applicationID string, tab string, primaryKey string) map[string]string {
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
			tabLink := applicationLink + "#tab=" + url.QueryEscape(tab)
			links["primary"] = tabLink
			if strings.TrimSpace(primaryKey) != "" {
				links[primaryKey] = tabLink
			}
		}
	}
	return links
}
