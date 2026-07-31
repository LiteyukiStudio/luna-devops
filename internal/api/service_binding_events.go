package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
)

func (h *Handlers) emitServiceBindingEvent(ctx context.Context, user model.User, project model.Project, binding model.ServiceBinding, status, severity string) {
	var sourceApplication model.Application
	var sourceTarget model.DeploymentTarget
	_ = h.dbWithContext(ctx).WithContext(ctx).First(&sourceApplication, "id = ?", binding.SourceApplicationID).Error
	_ = h.dbWithContext(ctx).WithContext(ctx).First(&sourceTarget, "id = ?", binding.SourceDeploymentTargetID).Error

	links := map[string]string{}
	if base := strings.TrimRight(strings.TrimSpace(externalBaseURL()), "/"); base != "" {
		links["projectTopology"] = fmt.Sprintf("%s/projects/%s?tab=topology", base, project.ID)
	}
	if len(links) == 0 {
		links = nil
	}
	_, _ = (notification.Service{DB: h.dbWithContext(ctx), Enqueuer: h.taskClient}).Emit(ctx, notification.Event{
		Type:             "service_binding." + status,
		Severity:         severity,
		Project:          notification.EntityRef{ID: project.ID, Name: project.Name, Identifier: project.Identifier},
		Application:      notification.EntityRef{ID: sourceApplication.ID, Name: sourceApplication.Name, Identifier: sourceApplication.Identifier},
		DeploymentTarget: notification.EntityRef{ID: sourceTarget.ID, Name: sourceTarget.Name, Identifier: sourceTarget.Stage},
		ServiceBinding: notification.ServiceBindingContext{
			ID: binding.ID, Status: status,
			SourceDeploymentTargetID: binding.SourceDeploymentTargetID,
			TargetApplicationID:      binding.TargetApplicationID,
			TargetDeploymentTargetID: binding.TargetDeploymentTargetID,
		},
		Actor:         notification.ActorContext{ID: user.ID, Name: user.Name, Email: user.Email},
		CorrelationID: binding.ID,
		DedupKey:      fmt.Sprintf("service_binding:%s:%s:%d", binding.ID, status, binding.UpdatedAt.UnixNano()),
		OccurredAt:    time.Now(),
		Links:         links,
		Message:       "Service binding " + status,
	})
}
