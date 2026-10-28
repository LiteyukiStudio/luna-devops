package buildapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type buildIdempotencyTestHost struct {
	Host
	db         *gorm.DB
	queueErr   error
	dispatched []tasks.BuildRunPayload
}

func (h *buildIdempotencyTestHost) DBFor(*gin.Context) *gorm.DB {
	return h.db
}

func TestBuildRunIdempotencyReplaysOnlyTheSameRequest(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "build_idempotency_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.BuildRun{})
		},
	})
	run := model.BuildRun{
		ID:                     "bldr_idempotent",
		Status:                 "succeeded",
		ProjectID:              "prj_1",
		CreatedBy:              "usr_1",
		IdempotencyKey:         "build.trigger:request-1",
		IdempotencyRequestHash: "sha256:same",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create build run: %v", err)
	}
	handler := New(&buildIdempotencyTestHost{db: db})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	found, exists, available := handler.findBuildRunByIdempotency(
		ctx, "usr_1", "prj_1", run.IdempotencyKey, run.IdempotencyRequestHash,
	)
	if !available || !exists || found.ID != run.ID {
		t.Fatalf("same request replay = (%#v, %v, %v)", found, exists, available)
	}

	_, exists, available = handler.findBuildRunByIdempotency(
		ctx, "usr_1", "prj_1", run.IdempotencyKey, "sha256:different",
	)
	if available || exists {
		t.Fatalf("different request reuse = exists:%v available:%v", exists, available)
	}
	if recorder.Code != 409 {
		t.Fatalf("different request status = %d, want 409", recorder.Code)
	}
}

func (h *buildIdempotencyTestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return h.db.WithContext(ctx)
}

func (h *buildIdempotencyTestHost) BuildQueueAvailable() bool {
	return true
}

func (h *buildIdempotencyTestHost) EnqueueBuildRun(_ context.Context, payload tasks.BuildRunPayload) error {
	h.dispatched = append(h.dispatched, payload)
	return h.queueErr
}

func TestBuildRunIdempotencyRecoversDispatchAfterCommit(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "build_dispatch_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.BuildRun{}, &model.BuildJob{})
		},
	})
	host := &buildIdempotencyTestHost{db: db, queueErr: errors.New("queue unavailable")}
	handler := New(host)
	run := model.BuildRun{ID: "bldr_recover", ProjectID: "prj_1", CreatedBy: "usr_1", Status: "queued", IdempotencyKey: "build.trigger:recover", IdempotencyRequestHash: "same"}
	if _, err := handler.queueBuildRun(t.Context(), model.User{ID: "usr_1"}, run); err == nil {
		t.Fatal("expected dispatch failure")
	}
	var persisted model.BuildRun
	if err := db.First(&persisted, "id = ?", run.ID).Error; err != nil || persisted.Status != "queued" {
		t.Fatalf("failed dispatch must remain recoverable: %#v, %v", persisted, err)
	}
	for _, unavailable := range []bool{true, false} {
		if !unavailable {
			host.queueErr = nil
		}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("POST", "/", nil)
		found, exists, available := handler.findBuildRunByIdempotency(ctx, run.CreatedBy, run.ProjectID, run.IdempotencyKey, run.IdempotencyRequestHash)
		if unavailable {
			if available || recorder.Code != 503 {
				t.Fatalf("queue failure replay = %v, %d", available, recorder.Code)
			}
		} else if !available || !exists || found.ID != run.ID {
			t.Fatalf("recovered replay = %#v, %v, %v", found, exists, available)
		}
	}
	var count int64
	db.Model(&model.BuildRun{}).Count(&count)
	if count != 1 || len(host.dispatched) != 3 || host.dispatched[0] != host.dispatched[2] {
		t.Fatalf("replay created new work: count=%d, payloads=%#v", count, host.dispatched)
	}
}
