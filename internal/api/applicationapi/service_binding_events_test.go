package applicationapi

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestServiceBindingNotificationEventKeepsAuthoritativeResourceIDsWhenLookupsMiss(t *testing.T) {
	event := serviceBindingNotificationEvent(
		model.User{ID: "usr_operator"},
		model.Project{ID: "prj_binding"},
		model.ServiceBinding{
			ID:                       "sbd_missing_context",
			CreatedBy:                "usr_owner",
			SourceApplicationID:      "app_deleted",
			SourceDeploymentTargetID: "dpt_deleted",
		},
		model.Application{},
		model.DeploymentTarget{},
		nil,
		"invalid",
		"error",
	)

	if event.Project.ID != "prj_binding" || event.Application.ID != "app_deleted" || event.DeploymentTarget.ID != "dpt_deleted" {
		t.Fatalf("event resource ids = project %q application %q target %q", event.Project.ID, event.Application.ID, event.DeploymentTarget.ID)
	}
	if event.Actor.ID != "usr_operator" || event.ResourceOwnerUserID != "usr_owner" {
		t.Fatalf("event recipients = actor %q owner %q", event.Actor.ID, event.ResourceOwnerUserID)
	}
}
