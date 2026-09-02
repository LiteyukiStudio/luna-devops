package applicationapi

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

func (h *Handlers) emitServiceBindingEvent(ctx context.Context, user model.User, project model.Project, binding model.ServiceBinding, status, severity string) {
	var sourceApplication model.Application
	var sourceTarget model.DeploymentTarget
	_ = h.dbWithContext(ctx).WithContext(ctx).First(&sourceApplication, "id = ?", binding.SourceApplicationID).Error
	_ = h.dbWithContext(ctx).WithContext(ctx).First(&sourceTarget, "id = ?", binding.SourceDeploymentTargetID).Error

	links := map[string]string{}
	if base := strings.TrimRight(strings.TrimSpace(h.host.PublicBaseURL()), "/"); base != "" {
		links["projectTopology"] = fmt.Sprintf("%s/projects/%s?tab=topology", base, project.ID)
	}
	if len(links) == 0 {
		links = nil
	}
	event := serviceBindingNotificationEvent(user, project, binding, sourceApplication, sourceTarget, links, status, severity)
	if _, err := (notification.Service{DB: h.dbWithContext(ctx), Enqueuer: h.host.NotificationEnqueuer()}).Emit(ctx, event); err != nil {
		telemetry.RecordError(ctx, "notification.event_emit_failed", err,
			slog.String("notification.event_type", event.Type))
	}
}

func serviceBindingNotificationEvent(
	user model.User,
	project model.Project,
	binding model.ServiceBinding,
	sourceApplication model.Application,
	sourceTarget model.DeploymentTarget,
	links map[string]string,
	status string,
	severity string,
) notification.Event {
	return notification.Event{
		Type:             "service_binding." + status,
		Severity:         severity,
		Project:          notification.EntityRef{ID: project.ID, Name: project.Name, Identifier: project.Identifier},
		Application:      notification.EntityRef{ID: binding.SourceApplicationID, Name: sourceApplication.Name, Identifier: sourceApplication.Identifier},
		DeploymentTarget: notification.EntityRef{ID: binding.SourceDeploymentTargetID, Name: sourceTarget.Name, Identifier: sourceTarget.Stage},
		ServiceBinding: notification.ServiceBindingContext{
			ID: binding.ID, Status: status,
			SourceDeploymentTargetID: binding.SourceDeploymentTargetID,
			TargetApplicationID:      binding.TargetApplicationID,
			TargetDeploymentTargetID: binding.TargetDeploymentTargetID,
		},
		Actor:               notification.ActorContext{ID: user.ID, Name: user.Name, Email: user.Email},
		ResourceOwnerUserID: binding.CreatedBy,
		CorrelationID:       binding.ID,
		DedupKey:            fmt.Sprintf("service_binding:%s:%s:%d", binding.ID, status, binding.UpdatedAt.UnixNano()),
		OccurredAt:          time.Now(),
		Links:               links,
		Message:             "Service binding " + status,
	}
}
