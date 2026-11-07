package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestGetProjectReportsVolumeTransferConfiguration(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "project_detail_transfer_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.ProjectMember{}, &model.User{})
		},
	})
	project := model.Project{ID: "prj_transfer", Identifier: "transfer", Name: "Transfer", KubernetesNamespace: "ns-transfer", DeleteStatus: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	for _, enabled := range []bool{false, true} {
		handlers := &Handlers{db: db, configs: &configCache{values: map[string]string{}}, volumeTransferEnabled: enabled}
		handlers.domains = newDomainHandlers(handlers)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID, nil)
		ctx.Params = gin.Params{{Key: "projectId", Value: project.ID}}
		ctx.Set(currentUserContextKey, model.User{ID: "usr_transfer", Role: authz.PlatformRoleAdmin})
		handlers.domains.project.GetProject(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("get project = %d %s", recorder.Code, recorder.Body.String())
		}
		var body struct {
			VolumeTransferEnabled *bool `json:"volumeTransferEnabled"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.VolumeTransferEnabled == nil || *body.VolumeTransferEnabled != enabled {
			t.Fatalf("volumeTransferEnabled = %v, want %v (%s)", body.VolumeTransferEnabled, enabled, recorder.Body.String())
		}
	}
}
