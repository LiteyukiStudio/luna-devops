package runtimecluster

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

// The previous resolvers reused the *gorm.DB returned by ActiveScope, so the
// "is_default = true" condition of the first lookup leaked into the fallback
// lookup and the fallback never matched. ResolveForTarget must issue a single
// query without an is_default filter, even when the caller's handle already
// carries conditions.
func TestResolveForTargetIssuesOneIsolatedQuery(t *testing.T) {
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	var statements []string
	if err := db.Callback().Query().After("gorm:query").Register("test:capture", func(tx *gorm.DB) {
		statements = append(statements, tx.Statement.SQL.String())
	}); err != nil {
		t.Fatalf("register capture callback: %v", err)
	}
	polluted := ActiveScope(db).Where("is_default = ?", true)

	if _, err := ResolveForTarget(polluted, ""); err != nil {
		t.Fatalf("ResolveForTarget(default) error = %v", err)
	}
	if _, err := ResolveForTarget(polluted, " rc_explicit "); err != nil {
		t.Fatalf("ResolveForTarget(explicit) error = %v", err)
	}
	if len(statements) != 2 {
		t.Fatalf("captured %d statements, want 2: %#v", len(statements), statements)
	}
	defaultSQL, explicitSQL := statements[0], statements[1]
	for _, want := range []string{"delete_status = ?", "scope = ?", "ORDER BY is_default desc,created_at asc,id asc"} {
		if !strings.Contains(defaultSQL, want) {
			t.Fatalf("default lookup SQL %q does not contain %q", defaultSQL, want)
		}
	}
	if strings.Contains(defaultSQL, "is_default = ?") {
		t.Fatalf("default lookup SQL filters on is_default and cannot fall back: %q", defaultSQL)
	}
	if !strings.Contains(explicitSQL, "id = ?") || strings.Contains(explicitSQL, "scope = ?") || strings.Contains(explicitSQL, "is_default") {
		t.Fatalf("explicit lookup SQL = %q, want only an active id lookup", explicitSQL)
	}
}

func TestResolveForTargetMatrix(t *testing.T) {
	db := testdb.Open(t, testdb.Options{SchemaPrefix: "runtimecluster_resolve"})
	if err := db.AutoMigrate(&model.RuntimeCluster{}); err != nil {
		t.Fatalf("migrate runtime clusters: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	create := func(cluster model.RuntimeCluster, offset time.Duration) {
		t.Helper()
		cluster.Name = cluster.ID
		if cluster.DeleteStatus == "" {
			cluster.DeleteStatus = DeleteStatusActive
		}
		cluster.CreatedAt = base.Add(offset)
		if err := db.Create(&cluster).Error; err != nil {
			t.Fatalf("create cluster %s: %v", cluster.ID, err)
		}
	}
	expect := func(name, clusterID, want string) {
		t.Helper()
		cluster, err := ResolveForTarget(db, clusterID)
		if want == "" {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("%s: ResolveForTarget(%q) = %q, %v; want not found", name, clusterID, cluster.ID, err)
			}
			return
		}
		if err != nil || cluster.ID != want {
			t.Fatalf("%s: ResolveForTarget(%q) = %q, %v; want %q", name, clusterID, cluster.ID, err, want)
		}
	}

	expect("no clusters", "", "")
	create(model.RuntimeCluster{ID: "rc_project_oldest", Scope: "project"}, 0)
	create(model.RuntimeCluster{ID: "rc_user_oldest", Scope: "user", OwnerRef: "usr_1"}, time.Minute)
	expect("project and user clusters are never implicit", "", "")
	expect("explicit project cluster", "rc_project_oldest", "rc_project_oldest")

	create(model.RuntimeCluster{ID: "rc_global_deleting", Scope: ScopeGlobal, DeleteStatus: DeleteStatusDeleting}, 2*time.Minute)
	create(model.RuntimeCluster{ID: "rc_global_old", Scope: ScopeGlobal}, 3*time.Minute)
	create(model.RuntimeCluster{ID: "rc_global_new", Scope: ScopeGlobal}, 4*time.Minute)
	expect("no default falls back to earliest active global cluster", "", "rc_global_old")
	expect("explicit inactive cluster", "rc_global_deleting", "")
	expect("explicit missing cluster", "rc_missing", "")

	if err := db.Model(&model.RuntimeCluster{}).Where("id = ?", "rc_global_new").Update("is_default", true).Error; err != nil {
		t.Fatalf("mark default: %v", err)
	}
	expect("marked global default wins", "", "rc_global_new")
	expect("explicit cluster ignores default", "rc_global_old", "rc_global_old")

	if err := db.Model(&model.RuntimeCluster{}).Where("id = ?", "rc_global_new").Update("delete_status", DeleteStatusDeleting).Error; err != nil {
		t.Fatalf("mark default deleting: %v", err)
	}
	expect("inactive default falls back", "", "rc_global_old")
}
