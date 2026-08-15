package database

import (
	"strings"
	"testing"

	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestSchemaMigrationAuthorityFinalizesFormerStartupMutations(t *testing.T) {
	upMigration, err := sqlmigrations.FS.ReadFile("000067_schema_migration_authority.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := strings.ToLower(string(upMigration))
	for _, expected := range []string{
		"drop column if exists graph_version",
		"drop column if exists service_port",
		"drop column if exists build_config_id",
		"set billing_owner_user_id",
		"set billed_user_id",
		"set deployment_target_id",
		"insert into billing_rate_rules",
	} {
		if !strings.Contains(upSQL, expected) {
			t.Errorf("schema authority migration is missing %q", expected)
		}
	}
	for _, volumeTable := range []string{
		"project_volumes",
		"deployment_volume_mounts",
		"volume_transfers",
		"volume_transfer_parts",
	} {
		if strings.Contains(upSQL, volumeTable) {
			t.Errorf("schema authority migration must not depend on volume table %s", volumeTable)
		}
	}
}

func TestSchemaMigrationAuthorityIsExplicitlyIrreversible(t *testing.T) {
	downMigration, err := sqlmigrations.FS.ReadFile("000067_schema_migration_authority.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	downSQL := strings.ToLower(string(downMigration))
	if !strings.Contains(downSQL, "irreversible") || !strings.Contains(downSQL, "raise exception") {
		t.Fatalf("down migration must fail explicitly instead of pretending to restore discarded data:\n%s", downSQL)
	}
}
