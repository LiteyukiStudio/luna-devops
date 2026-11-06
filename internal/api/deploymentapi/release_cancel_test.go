package deploymentapi

import (
	"context"
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

type releaseCancelTestHost struct {
	Host
	db      *gorm.DB
	actions []authz.Action
	audits  []string
}

func (h *releaseCancelTestHost) DBFor(*gin.Context) *gorm.DB { return h.db }

func (h *releaseCancelTestHost) AuthorizeProject(_ *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	h.actions = append(h.actions, action)
	return model.User{ID: "usr_cancel"}, model.Project{ID: "prj_cancel"}, true
}

func (h *releaseCancelTestHost) AuditWithContext(_, action, resource string, _ bool, _ string, _ context.Context) {
	h.audits = append(h.audits, action+":"+resource)
}

func cancelReleaseRequest(handler *Handler, releaseID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_cancel/releases/"+releaseID+"/cancel", nil)
	ctx.Params = gin.Params{{Key: "projectId", Value: "prj_cancel"}, {Key: "releaseId", Value: releaseID}}
	handler.CancelRelease(ctx)
	return recorder
}

func TestCancelReleaseIsGuardedAuditedAndIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "release_cancel_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Release{})
		},
	})
	for _, release := range []model.Release{
		{ID: "rel_running", ProjectID: "prj_cancel", ApplicationID: "app_1", ImageRef: "example/app:1", Status: "running"},
		{ID: "rel_done", ProjectID: "prj_cancel", ApplicationID: "app_1", ImageRef: "example/app:0", Status: "succeeded"},
		{ID: "rel_other", ProjectID: "prj_other", ApplicationID: "app_2", ImageRef: "example/app:2", Status: "pending"},
	} {
		if err := db.Create(&release).Error; err != nil {
			t.Fatalf("create release: %v", err)
		}
	}
	host := &releaseCancelTestHost{db: db}
	handler := &Handler{host: host}

	for attempt := range 2 {
		recorder := cancelReleaseRequest(handler, "rel_running")
		if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("attempt %d status = %d cache=%q body=%s", attempt, recorder.Code, recorder.Header().Get("Cache-Control"), recorder.Body.String())
		}
		var body model.Release
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode release: %v", err)
		}
		if body.Status != "canceled" || body.FailureCode != model.ReleaseFailureCanceledByUser || body.FinishedAt == nil {
			t.Fatalf("attempt %d release = %+v", attempt, body)
		}
	}
	if len(host.audits) != 1 || host.audits[0] != "release.cancel:rel_running" {
		t.Fatalf("audits = %v, want one cancel audit", host.audits)
	}
	for _, action := range host.actions {
		if action != authz.ActionDeploymentRelease {
			t.Fatalf("authorized action = %s, want %s", action, authz.ActionDeploymentRelease)
		}
	}

	recorder := cancelReleaseRequest(handler, "rel_done")
	if recorder.Code != http.StatusConflict {
		t.Fatalf("finished release cancel status = %d, want 409", recorder.Code)
	}
	var errorBody struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &errorBody); err != nil || errorBody.Code != releaseNotCancelableCode {
		t.Fatalf("finished release cancel body = %s", recorder.Body.String())
	}
	var done model.Release
	if err := db.First(&done, "id = ?", "rel_done").Error; err != nil || done.Status != "succeeded" {
		t.Fatalf("finished release = %+v, %v", done, err)
	}

	if recorder := cancelReleaseRequest(handler, "rel_other"); recorder.Code != http.StatusNotFound {
		t.Fatalf("cross-project cancel status = %d, want 404", recorder.Code)
	}
}
