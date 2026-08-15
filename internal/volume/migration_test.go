package volume

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProjectVolumeMigrationIsExpandOnlyAndConstrained(t *testing.T) {
	t.Parallel()
	up := readVolumeMigration(t, "000066_project_volume_center.up.sql")
	for _, required := range []string{
		"CREATE TABLE project_volumes",
		"CREATE TABLE deployment_volume_mounts",
		"CREATE TABLE volume_transfers",
		"CREATE TABLE volume_transfer_parts",
		"source_snapshot_name text NOT NULL DEFAULT ''",
		"source_kind = 'snapshot_restore' AND source_snapshot_name <> ''",
		"source_kind <> 'snapshot_restore' AND source_snapshot_name = ''",
		"idx_deployment_volume_mounts_exclusive_active",
		"idx_volume_transfers_expired_objects",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("expand migration is missing %q", required)
		}
	}
	upper := strings.ToUpper(up)
	if strings.Contains(upper, "DROP TABLE") || strings.Contains(upper, "DROP COLUMN") || strings.Contains(upper, "ALTER TABLE DEPLOYMENT_TARGETS") {
		t.Fatal("expand migration must not remove or rewrite the legacy deployment-target volume contract")
	}

	down := readVolumeMigration(t, "000066_project_volume_center.down.sql")
	for _, legacy := range []string{"deployment_targets", "retained_volumes"} {
		if strings.Contains(strings.ToLower(down), "drop table if exists "+legacy) {
			t.Fatalf("contract migration unexpectedly drops legacy table %q", legacy)
		}
	}
}

func readVolumeMigration(t *testing.T, name string) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve volume migration test source")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "migrations", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(content)
}
