package buildruntime

import (
	"errors"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestExplicitRegistryCredentialQueryFailureDoesNotSelectFallback(t *testing.T) {
	wantErr := errors.New("credential query failed")
	db, queryCount := failingBuildRuntimeQueryDB(t, wantErr)
	registry := model.ArtifactRegistry{ID: "reg_explicit", CredentialRef: "rcred_explicit"}

	_, err := (Resolver{}).registryCredentialForBuild(db, "usr_actor", "prj_demo", registry)
	if !errors.Is(err, wantErr) {
		t.Fatalf("registry credential error = %v, want wrapped query failure", err)
	}
	if *queryCount != 1 {
		t.Fatalf("registry credential query count = %d, want 1 without fallback selection", *queryCount)
	}
}

func TestExplicitDeploymentTargetQueryFailureDoesNotUseZeroValueTarget(t *testing.T) {
	wantErr := errors.New("deployment target query failed")
	db, queryCount := failingBuildRuntimeQueryDB(t, wantErr)
	run := model.BuildRun{DeploymentTargetID: "dplt_explicit", ProjectID: "prj_demo", ApplicationID: "app_demo"}

	_, err := deploymentTargetForBuild(db, run)
	if !errors.Is(err, wantErr) {
		t.Fatalf("deployment target error = %v, want wrapped query failure", err)
	}
	if *queryCount != 1 {
		t.Fatalf("deployment target query count = %d, want 1", *queryCount)
	}
}

func failingBuildRuntimeQueryDB(t *testing.T, queryErr error) (*gorm.DB, *int) {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 port=1 user=buildruntime_test dbname=buildruntime_test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	queryCount := 0
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail_query", func(tx *gorm.DB) {
		queryCount++
		tx.AddError(queryErr)
	}); err != nil {
		t.Fatalf("register failing query callback: %v", err)
	}
	return db, &queryCount
}
