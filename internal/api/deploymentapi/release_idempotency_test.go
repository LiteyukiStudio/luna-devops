package deploymentapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type releaseIdempotencyTestHost struct {
	Host
	db             *gorm.DB
	queueAvailable bool
	dispatched     []model.Release
}

func (h *releaseIdempotencyTestHost) DBFor(*gin.Context) *gorm.DB {
	return h.db
}

func TestReleaseIdempotencyReplaysOnlyTheSameRequest(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "release_idempotency_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Release{})
		},
	})
	release := model.Release{
		ID:                     "rel_idempotent",
		Status:                 "succeeded",
		ProjectID:              "prj_1",
		ApplicationID:          "app_1",
		ImageRef:               "registry.example/app:one",
		CreatedBy:              "usr_1",
		IdempotencyKey:         "release.create:request-1",
		IdempotencyRequestHash: "sha256:same",
	}
	if err := db.Create(&release).Error; err != nil {
		t.Fatalf("create release: %v", err)
	}
	handler := &Handler{host: &releaseIdempotencyTestHost{db: db}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	found, exists, available := handler.findReleaseByIdempotency(
		ctx, "usr_1", "prj_1", release.IdempotencyKey, release.IdempotencyRequestHash,
	)
	if !available || !exists || found.ID != release.ID {
		t.Fatalf("same request replay = (%#v, %v, %v)", found, exists, available)
	}

	_, exists, available = handler.findReleaseByIdempotency(
		ctx, "usr_1", "prj_1", release.IdempotencyKey, "sha256:different",
	)
	if available || exists {
		t.Fatalf("different request reuse = exists:%v available:%v", exists, available)
	}
	if recorder.Code != 409 {
		t.Fatalf("different request status = %d, want 409", recorder.Code)
	}
}

func (h *releaseIdempotencyTestHost) EnqueueDeployRun(_ context.Context, release model.Release) bool {
	h.dispatched = append(h.dispatched, release)
	return h.queueAvailable
}

func TestReleaseIdempotencyRecoversDispatchAfterCommit(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "release_dispatch_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Release{})
		},
	})
	release := model.Release{ID: "rel_recover", ProjectID: "prj_1", ApplicationID: "app_1", ImageRef: "example/app:one", CreatedBy: "usr_1", Status: "pending", IdempotencyKey: "release.create:recover", IdempotencyRequestHash: "same"}
	if err := db.Create(&release).Error; err != nil {
		t.Fatal(err)
	}
	host := &releaseIdempotencyTestHost{db: db}
	handler := &Handler{host: host}
	for _, available := range []bool{false, true} {
		host.queueAvailable = available
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("POST", "/", nil)
		found, exists, ok := handler.findReleaseByIdempotency(ctx, release.CreatedBy, release.ProjectID, release.IdempotencyKey, release.IdempotencyRequestHash)
		if !available {
			if ok || recorder.Code != 503 {
				t.Fatalf("queue failure replay = %v, %d", ok, recorder.Code)
			}
		} else if !ok || !exists || found.ID != release.ID || found.Status != "pending" {
			t.Fatalf("recovered replay = %#v, %v, %v", found, exists, ok)
		}
	}
	var count int64
	db.Model(&model.Release{}).Count(&count)
	if count != 1 || len(host.dispatched) != 2 || host.dispatched[0].ID != host.dispatched[1].ID {
		t.Fatalf("replay created new work: count=%d, payloads=%#v", count, host.dispatched)
	}
}
