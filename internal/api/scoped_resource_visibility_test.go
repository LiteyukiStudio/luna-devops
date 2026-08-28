package api

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestScopedResourceVisibilityQueryKeepsUserAndProjectRangesSeparate(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}

	relatedSQL := scopedResourceVisibilitySQL(db, "usr_current", "", []string{"prj_member"}, false, false)
	for _, expected := range []string{
		"scope = 'global'",
		"scope = 'user' and owner_ref = 'usr_current'",
		"scope = 'project'",
		"resource_type = 'runtime_cluster'",
		"project_id in ('prj_member')",
	} {
		if !strings.Contains(relatedSQL, expected) {
			t.Fatalf("related query is missing %q: %s", expected, relatedSQL)
		}
	}

	allSQL := scopedResourceVisibilitySQL(db, "usr_current", "", nil, true, true)
	if !strings.Contains(allSQL, "scope = 'project'") || strings.Contains(allSQL, "scoped_resource_project_bindings") {
		t.Fatalf("all query must add every project-scoped row directly: %s", allSQL)
	}
	if !strings.Contains(allSQL, "scope = 'user' and owner_ref = 'usr_current'") || strings.Contains(allSQL, "usr_other") {
		t.Fatalf("all query exposed another user's scope: %s", allSQL)
	}

	accessSQL := scopedResourceVisibilitySQL(db, "usr_current", "", nil, true, false)
	if !strings.Contains(accessSQL, "scoped_resource_project_bindings") {
		t.Fatalf("known-resource access must retain the existing bound-project filter: %s", accessSQL)
	}

	explicitSQL := scopedResourceVisibilitySQL(db, "usr_current", "prj_selected", []string{"prj_member"}, true, true)
	if !strings.Contains(explicitSQL, "project_id = 'prj_selected'") || strings.Contains(explicitSQL, "prj_member") {
		t.Fatalf("explicit project must override broader visibility: %s", explicitSQL)
	}
}

func scopedResourceVisibilitySQL(db *gorm.DB, userID, projectID string, projectIDs []string, includeAllProjects, includeUnboundProjectScope bool) string {
	var clusters []model.RuntimeCluster
	statement := applyScopedResourceVisibilityQuery(
		db.Model(&model.RuntimeCluster{}),
		db,
		scopedResourceRuntimeCluster,
		userID,
		projectID,
		projectIDs,
		includeAllProjects,
		includeUnboundProjectScope,
	).Find(&clusters).Statement
	return db.Dialector.Explain(statement.SQL.String(), statement.Vars...)
}
