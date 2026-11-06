package buildapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

type buildDeleteGuardTestHost struct {
	Host
	db *gorm.DB
}

func (h *buildDeleteGuardTestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return h.db.WithContext(ctx)
}

func (h *buildDeleteGuardTestHost) ApplicationCanMutate(model.Application) bool { return true }

func TestPrepareBuildRunRequestRejectsDeletingProjectAndTarget(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "build_delete_guard_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.Application{}, &model.DeploymentTarget{})
		},
	})
	project := model.Project{ID: "prj_guard", Name: "Guard", Identifier: "guard", DeleteStatus: "deleting"}
	application := model.Application{ID: "app_guard", ProjectID: project.ID, Name: "Guard", Identifier: "guard"}
	target := model.DeploymentTarget{
		ID: "dplt_guard", ProjectID: project.ID, ApplicationID: application.ID, Name: "Dev", Stage: "dev",
		KubernetesName: "guard-dev", Enabled: true, DeleteStatus: "deleting",
	}
	for _, value := range []any{&project, &application, &target} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	handler := New(&buildDeleteGuardTestHost{db: db})
	run := model.BuildRun{ProjectID: project.ID, ApplicationID: application.ID, DeploymentTargetID: target.ID}

	err := handler.prepareBuildRunRequest(model.User{ID: "usr_guard"}, &run, t.Context())
	var requestErr buildRunRequestError
	if !errors.As(err, &requestErr) || requestErr.status != http.StatusConflict || requestErr.code != "project.delete_in_progress" {
		t.Fatalf("deleting project error = %#v", err)
	}

	if err := db.Model(&model.Project{}).Where("id = ?", project.ID).Update("delete_status", "active").Error; err != nil {
		t.Fatalf("activate project: %v", err)
	}
	if _, err := handler.deploymentTargetForBuildRun(application, target.ID, t.Context()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleting deployment target lookup error = %v, want not found", err)
	}
	if err := db.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Update("delete_status", "delete_failed").Error; err != nil {
		t.Fatalf("mark target delete_failed: %v", err)
	}
	if found, err := handler.deploymentTargetForBuildRun(application, target.ID, t.Context()); err != nil || found.ID != target.ID {
		t.Fatalf("delete_failed deployment target = %+v, %v", found, err)
	}
}
