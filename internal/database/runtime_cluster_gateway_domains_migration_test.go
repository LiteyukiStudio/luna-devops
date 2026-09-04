package database

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestUnifyRuntimeClusterGatewayDomainsMigration(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "runtime_cluster_gateway_domains_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(100); err != nil {
		t.Fatalf("migrate isolated database to version 100: %v", err)
	}

	early := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	clusters := []struct {
		id        string
		name      string
		root      string
		suffixes  string
		isDefault bool
		createdAt time.Time
	}{
		{"rcl_earliest", "Earliest", "ignored-earliest-root.example.test", "earliest.example.test", false, early},
		{"rcl_gateway_domain", "Gateway Domain", "ignored-root.example.test", " Existing.Example.Test.; secondary.example.test;existing.example.test", false, early.Add(time.Hour)},
		{"rcl_default", "Default", "ignored-default-root.example.test", "default.example.test", true, early.Add(2 * time.Hour)},
		{"rcl_root_fallback", "Root Fallback", ".Root-Fallback.Example.Test.", " ; ", false, early.Add(3 * time.Hour)},
		{"rcl_hidden_fallback", "Hidden Fallback", "", "", false, early.Add(4 * time.Hour)},
		{"rcl_separator_restore", "Separator Restore", "separator-root.example.test", "separator-root.example.test", false, early.Add(5 * time.Hour)},
		{"rcl_comma_restore", "Comma Restore", "comma-root.example.test", "comma-root.example.test", false, early.Add(6 * time.Hour)},
	}
	for _, cluster := range clusters {
		if err := db.Exec(`INSERT INTO runtime_clusters (id, name, gateway_root_domain, gateway_domain_suffixes, is_default, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			cluster.id, cluster.name, cluster.root, cluster.suffixes, cluster.isDefault, cluster.createdAt, cluster.createdAt).Error; err != nil {
			t.Fatalf("insert runtime cluster %s: %v", cluster.id, err)
		}
	}
	if err := db.Exec(`INSERT INTO projects (id, identifier, kubernetes_namespace, name) VALUES (?, ?, ?, ?)`,
		"prj_gateway_domain", "gateway-domain", "luna-gateway-domain", "Gateway Domain").Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := db.Exec(`INSERT INTO applications (id, project_id, identifier, name) VALUES (?, ?, ?, ?)`,
		"app_gateway_domain", "prj_gateway_domain", "api", "API").Error; err != nil {
		t.Fatalf("insert application: %v", err)
	}
	targets := []struct {
		id        string
		clusterID string
	}{
		{"dplt_gateway_domain", "rcl_gateway_domain"},
		{"dplt_gateway_default", ""},
	}
	for _, target := range targets {
		stage := "prod"
		if target.id == "dplt_gateway_default" {
			stage = "staging"
		}
		if err := db.Exec(`INSERT INTO deployment_targets (id, project_id, application_id, environment_id, name, stage, cluster_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			target.id, "prj_gateway_domain", "app_gateway_domain", "legacy_environment", target.id, stage, target.clusterID).Error; err != nil {
			t.Fatalf("insert deployment target %s: %v", target.id, err)
		}
	}
	routes := []struct {
		id       string
		targetID string
		host     string
		suffix   string
	}{
		{"gwr_gateway_domain", "dplt_gateway_domain", "api.route.example.test", "ROUTE.Example.Test."},
		{"gwr_gateway_default", "dplt_gateway_default", "api.default-route.example.test", "default-route.example.test"},
		{"gwr_gateway_duplicate", "dplt_gateway_domain", "api.existing.example.test", "existing.example.test"},
	}
	for _, route := range routes {
		if err := db.Exec(`INSERT INTO gateway_routes (id, project_id, application_id, environment_id, deployment_target_id, host, domain_suffix) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			route.id, "prj_gateway_domain", "app_gateway_domain", "legacy_environment", route.targetID, route.host, route.suffix).Error; err != nil {
			t.Fatalf("insert gateway route %s: %v", route.id, err)
		}
	}
	for key, value := range map[string]string{
		"gateway.rootDomain":   " Hidden.Config.Example.Test. ",
		"gateway.publicScheme": "https",
		"retained.setting":     "retained",
	} {
		if err := db.Exec(`INSERT INTO app_configs (key, value) VALUES (?, ?)`, key, value).Error; err != nil {
			t.Fatalf("insert app config %s: %v", key, err)
		}
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply runtime cluster gateway domain migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 101)
	assertGatewayDomainMigrationState(t, db, false)
	assertRuntimeClusterSuffixes(t, db, "rcl_gateway_domain", []string{"existing.example.test", "secondary.example.test", "route.example.test"})
	assertRuntimeClusterSuffixes(t, db, "rcl_default", []string{"default.example.test", "default-route.example.test"})
	assertRuntimeClusterSuffixes(t, db, "rcl_root_fallback", []string{"root-fallback.example.test"})
	assertRuntimeClusterSuffixes(t, db, "rcl_hidden_fallback", []string{"hidden.config.example.test"})

	if err := db.Table("runtime_clusters").Where("id = ?", "rcl_separator_restore").Update("gateway_domain_suffixes", " first.example.test ; second.example.test,third.example.test").Error; err != nil {
		t.Fatalf("seed separator-delimited suffixes before rollback: %v", err)
	}
	if err := db.Table("runtime_clusters").Where("id = ?", "rcl_comma_restore").Update("gateway_domain_suffixes", " comma-first.example.test, comma-second.example.test").Error; err != nil {
		t.Fatalf("seed comma-delimited suffixes before rollback: %v", err)
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back runtime cluster gateway domain migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 100)
	assertGatewayDomainMigrationState(t, db, true)
	assertRuntimeClusterRootDomain(t, db, "rcl_gateway_domain", "existing.example.test")
	assertRuntimeClusterRootDomain(t, db, "rcl_separator_restore", "first.example.test")
	assertRuntimeClusterRootDomain(t, db, "rcl_comma_restore", "comma-first.example.test")

	if err := db.Table("runtime_clusters").Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
		t.Fatalf("remove global default before reapply: %v", err)
	}
	if err := db.Table("runtime_clusters").Where("id = ?", "rcl_default").Update("gateway_domain_suffixes", "default.example.test").Error; err != nil {
		t.Fatalf("remove previously projected route suffix from old default: %v", err)
	}
	if err := db.Exec(`INSERT INTO runtime_clusters (id, name, gateway_root_domain, gateway_domain_suffixes) VALUES (?, ?, ?, ?)`,
		"rcl_apps_local", "Apps Local", "", "").Error; err != nil {
		t.Fatalf("insert apps.local fallback cluster: %v", err)
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("reapply runtime cluster gateway domain migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 101)
	assertGatewayDomainMigrationState(t, db, false)
	assertRuntimeClusterSuffixes(t, db, "rcl_earliest", []string{"earliest.example.test", "default-route.example.test"})
	assertRuntimeClusterSuffixes(t, db, "rcl_default", []string{"default.example.test"})
	assertRuntimeClusterSuffixes(t, db, "rcl_apps_local", []string{"apps.local"})
}

func assertGatewayDomainMigrationState(t *testing.T, db *gorm.DB, rootDomainPresent bool) {
	t.Helper()
	if got := db.Migrator().HasColumn("runtime_clusters", "gateway_root_domain"); got != rootDomainPresent {
		t.Errorf("runtime_clusters.gateway_root_domain presence = %t, want %t", got, rootDomainPresent)
	}
	var obsoleteCount int64
	if err := db.Table("app_configs").Where("key IN ?", []string{"gateway.rootDomain", "gateway.publicScheme"}).Count(&obsoleteCount).Error; err != nil {
		t.Fatalf("count obsolete gateway app configs: %v", err)
	}
	if obsoleteCount != 0 {
		t.Errorf("obsolete gateway app config count = %d, want 0", obsoleteCount)
	}
	var retainedCount int64
	if err := db.Table("app_configs").Where("key = ?", "retained.setting").Count(&retainedCount).Error; err != nil {
		t.Fatalf("count retained app config: %v", err)
	}
	if retainedCount != 1 {
		t.Errorf("retained app config count = %d, want 1", retainedCount)
	}
}

func assertRuntimeClusterSuffixes(t *testing.T, db *gorm.DB, clusterID string, want []string) {
	t.Helper()
	var encoded string
	if err := db.Table("runtime_clusters").Select("gateway_domain_suffixes").Where("id = ?", clusterID).Scan(&encoded).Error; err != nil {
		t.Fatalf("read gateway domain suffixes for %s: %v", clusterID, err)
	}
	if got := strings.Split(encoded, "\n"); !slices.Equal(got, want) {
		t.Fatalf("gateway domain suffixes for %s = %#v, want %#v", clusterID, got, want)
	}
}

func assertRuntimeClusterRootDomain(t *testing.T, db *gorm.DB, clusterID string, want string) {
	t.Helper()
	var got string
	if err := db.Table("runtime_clusters").Select("gateway_root_domain").Where("id = ?", clusterID).Scan(&got).Error; err != nil {
		t.Fatalf("read restored gateway root domain for %s: %v", clusterID, err)
	}
	if got != want {
		t.Fatalf("restored gateway root domain for %s = %q, want %q", clusterID, got, want)
	}
}
