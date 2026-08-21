package volume

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAvailableVolumeFilterIncludesFirstConsumerOperations(t *testing.T) {
	t.Parallel()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "postgres://volume:volume@127.0.0.1:1/volume?sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	statement := applyProjectVolumeFilters(db.Model(&model.ProjectVolume{}), ProjectVolumeListOptions{
		Availability: model.ProjectVolumeAvailabilityAvailable,
	}).Find(&[]model.ProjectVolume{}).Statement
	if sql := statement.SQL.String(); !strings.Contains(sql, "pending_operation IN") || !strings.Contains(sql, "NOT EXISTS") {
		t.Fatalf("available filter SQL does not include attachability and reservation checks: %s", sql)
	}
	variables := fmt.Sprint(statement.Vars)
	for _, expected := range []string{model.ProjectVolumeLifecycleProvisioning, OperationProvision, OperationExpand} {
		if !strings.Contains(variables, expected) {
			t.Fatalf("available filter variables %s do not include %q", variables, expected)
		}
	}
}
