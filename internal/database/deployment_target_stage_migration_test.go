package database

import (
	"os"
	"strings"
	"testing"
)

func TestDeploymentTargetStageMigrationScopesUniquenessToActiveRows(t *testing.T) {
	sql, err := os.ReadFile("../../migrations/000061_deployment_target_active_stage_unique.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	content := string(sql)
	for _, fragment := range []string{
		"system_component_installations",
		"'sys-'",
		"idx_deployment_targets_application_stage_active",
		"deployment_targets(application_id, stage)",
		"WHERE deleted_at IS NULL",
	} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
