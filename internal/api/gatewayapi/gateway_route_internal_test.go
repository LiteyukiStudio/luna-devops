package gatewayapi

import (
	"errors"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestPersistGatewayRouteRejectsInternalTargetUnderLock(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "gateway_route_internal_test",
		Migrate:      func(db *gorm.DB) error { return db.AutoMigrate(&model.DeploymentTarget{}, &model.GatewayRoute{}) },
	})
	for _, target := range []model.DeploymentTarget{
		{ID: "dt_public", ProjectID: "prj", ApplicationID: "app_public", Name: "public", Stage: "prod"},
		{ID: "dt_internal", ProjectID: "prj", ApplicationID: "app_internal", Name: "internal", Stage: "prod", InternalOnly: true},
	} {
		if err := db.Create(&target).Error; err != nil {
			t.Fatal(err)
		}
	}
	written := 0
	write := func(*gorm.DB) error { written++; return nil }
	if err := persistGatewayRouteForTarget(db, write, "dt_internal"); !errors.Is(err, errGatewayRouteTargetInternal) {
		t.Fatalf("internal target error = %v", err)
	}
	if err := persistGatewayRouteForTarget(db, write, "dt_public"); err != nil {
		t.Fatal(err)
	}
	if written != 1 {
		t.Fatalf("route must be written only for the non-internal target, writes = %d", written)
	}
}
