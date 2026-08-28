package worker

import (
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func TestGatewayApplyPayloadMatchesRouteGeneration(t *testing.T) {
	updatedAt := time.Date(2026, time.August, 28, 7, 8, 9, 123456000, time.FixedZone("test", 8*60*60))
	route := model.GatewayRoute{ID: "gwr_generation", UpdatedAt: updatedAt}
	payload := tasks.GatewayApplyPayload{RouteUpdatedAtUnixMicro: updatedAt.UTC().UnixMicro()}

	if !gatewayApplyPayloadMatchesRoute(payload, route) {
		t.Fatal("equal gateway route generations must match")
	}
	payload.RouteUpdatedAtUnixMicro++
	if gatewayApplyPayloadMatchesRoute(payload, route) {
		t.Fatal("different gateway route generations must not match")
	}
	payload.RouteUpdatedAtUnixMicro = 0
	if gatewayApplyPayloadMatchesRoute(payload, route) {
		t.Fatal("missing gateway route generation must not match")
	}
}

func TestStaleGatewayApplySkipsRetryAndFailureEvent(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "gateway_apply_generation_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.GatewayRoute{}, &model.PlatformEvent{})
		},
	})
	route := model.GatewayRoute{
		ID:                 "gwr_stale",
		ProjectID:          "prj_stale",
		ApplicationID:      "app_stale",
		DeploymentTargetID: "target_stale",
		Host:               "stale.example.test",
		Path:               "/",
		CreatedBy:          "usr_owner",
		Enabled:            true,
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create gateway route: %v", err)
	}
	if err := db.First(&route, "id = ?", route.ID).Error; err != nil {
		t.Fatalf("reload gateway route: %v", err)
	}
	task, err := tasks.NewGatewayApplyTask(tasks.GatewayApplyPayload{
		GatewayRouteID:          route.ID,
		ProjectID:               route.ProjectID,
		ActorID:                 "usr_old_operator",
		RouteUpdatedAtUnixMicro: route.UpdatedAt.UTC().Add(-time.Microsecond).UnixMicro(),
	})
	if err != nil {
		t.Fatalf("create stale gateway task: %v", err)
	}

	err = (&Runner{db: db}).handleGatewayApply(t.Context(), task)
	if !errors.Is(err, asynq.SkipRetry) || !errors.Is(err, errGatewayApplyTaskStale) {
		t.Fatalf("stale gateway task error = %v, want SkipRetry and stale sentinel", err)
	}
	var eventCount int64
	if err := db.Model(&model.PlatformEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatalf("count platform events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("stale gateway task emitted %d platform events", eventCount)
	}
}
