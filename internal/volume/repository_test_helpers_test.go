package volume

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func validBlankVolumeInput() CreateProjectVolumeInput {
	return CreateProjectVolumeInput{
		ProjectID: "prj_demo", DisplayName: "postgres-data", ClusterID: "rclu_demo", Namespace: "project-demo",
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		CapacityRequest: "10Gi", CapacityBytes: 10 * 1024 * 1024 * 1024, StorageClassName: "standard",
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		ActorID: "usr_demo", IdempotencyKey: "volume-create-demo-0001",
	}
}

func postgresTestProjectVolume(index int) model.ProjectVolume {
	return model.ProjectVolume{
		ID: fmt.Sprintf("pvol_%03d", index), ProjectID: "prj_volume_test", DisplayName: fmt.Sprintf("volume-%03d", index),
		ClusterID: "rclu_volume_test", Namespace: "project-volume-test", ClaimName: fmt.Sprintf("claim-%03d", index),
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		LifecycleState: model.ProjectVolumeLifecycleReady, CapacityRequest: "1Gi", CapacityBytes: 1024 * 1024 * 1024,
		StorageClassName: "standard", AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		CreatedBy: "usr_volume_test", Revision: 1,
	}
}

func installProjectVolumeTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	files := []string{
		"000067_baseline.up.sql", "000068_project_volume_quota_billing.up.sql",
		"000069_volume_transfer_block_manifest.up.sql", "000070_volume_transfer_completion_state.up.sql",
		"000071_volume_transfer_part_leases.up.sql", "000072_volume_transfer_execution_leases.up.sql",
		"000073_volume_transfer_object_ownership.up.sql", "000084_direct_volume_transfer.up.sql",
	}
	var schemaSQL strings.Builder
	for _, file := range files {
		schemaSQL.WriteString(readVolumeMigration(t, file))
		schemaSQL.WriteByte('\n')
	}
	for _, statement := range splitVolumeMigrationStatements(stripVolumeMigrationLineComments(schemaSQL.String())) {
		if statement = strings.TrimSpace(statement); statement != "" {
			if err := db.Exec(statement).Error; err != nil {
				t.Fatalf("install project volume schema statement %q: %v", statement, err)
			}
		}
	}
}

func stripVolumeMigrationLineComments(sql string) string {
	lines := strings.Split(sql, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}

func splitVolumeMigrationStatements(sql string) []string {
	statements := make([]string, 0)
	var current strings.Builder
	var singleQuoted, doubleQuoted bool
	for index := 0; index < len(sql); index++ {
		character := sql[index]
		switch character {
		case '\'':
			current.WriteByte(character)
			if !doubleQuoted {
				if singleQuoted && index+1 < len(sql) && sql[index+1] == '\'' {
					current.WriteByte(sql[index+1])
					index++
					continue
				}
				singleQuoted = !singleQuoted
			}
		case '"':
			current.WriteByte(character)
			if !singleQuoted {
				doubleQuoted = !doubleQuoted
			}
		case ';':
			if singleQuoted || doubleQuoted {
				current.WriteByte(character)
				continue
			}
			statements = append(statements, current.String())
			current.Reset()
		default:
			current.WriteByte(character)
		}
	}
	if trailing := strings.TrimSpace(current.String()); trailing != "" {
		statements = append(statements, trailing)
	}
	return statements
}

func openVolumeTestDB(t *testing.T) *gorm.DB {
	return testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "volume_test"})
}
