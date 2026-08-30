package api

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRuntimeSecretMutationReplacesAndClearsAtomically(t *testing.T) {
	db := runtimeSecretMutationIntegrationDB(t)
	handlers, set, user := runtimeSecretMutationFixture(t, db, "replace")
	owner := projectRuntimeConfigSetSecretMutationOwner(set.ID, set.ProjectID)

	passwordRef := storeRuntimeSecretFixture(t, handlers, db, "old-password", user.ID, owner.ResourcePrefix+":PASSWORD")
	tokenRef := storeRuntimeSecretFixture(t, handlers, db, "old-token", user.ID, owner.ResourcePrefix+":TOKEN")
	set.SecretRefs = encodeStringMap(map[string]string{"PASSWORD": passwordRef, "TOKEN": tokenRef})
	if err := db.Model(&model.ProjectRuntimeConfigSet{}).Where("id = ?", set.ID).Update("secret_refs", set.SecretRefs).Error; err != nil {
		t.Fatalf("seed secret refs: %v", err)
	}

	prepared, err := prepareRuntimeSecretMutation(runtimeSecretMutationInput{
		Values: map[string]string{"PASSWORD": "new-password"},
		Clear:  []string{"TOKEN"},
	})
	if err != nil {
		t.Fatalf("prepare mutation: %v", err)
	}
	response, err := handlers.mutateRuntimeSecrets(t.Context(), user, prepared, owner)
	if err != nil {
		t.Fatalf("mutate runtime secrets: %v", err)
	}
	if len(response.EnvironmentVariables) != 1 || response.EnvironmentVariables[0].Key != "PASSWORD" || response.EnvironmentVariables[0].ValueMode != runtimeEnvironmentValueModeSecret || !response.EnvironmentVariables[0].Configured {
		t.Fatalf("response variables = %#v, want configured PASSWORD secret", response.EnvironmentVariables)
	}

	var reloaded model.ProjectRuntimeConfigSet
	if err := db.First(&reloaded, "id = ?", set.ID).Error; err != nil {
		t.Fatalf("reload config set: %v", err)
	}
	refs := decodeSecretRefs(reloaded.SecretRefs)
	if got := handlers.secrets.ResolveContext(t.Context(), refs["PASSWORD"]); got != "new-password" {
		t.Fatalf("resolved replacement = %q, want new-password", got)
	}
	if _, exists := refs["TOKEN"]; exists {
		t.Fatal("cleared TOKEN reference is still present")
	}
	assertRuntimeSecretRows(t, db, []string{passwordRef, tokenRef}, 0)
	assertRuntimeSecretAuditCount(t, db, owner.AuditAction, set.ID, 1)
	assertRuntimeSecretAuditCount(t, db, "secret.write", "", 0)
}

func TestRuntimeSecretMutationAllowsPublicValueAndSecretToOverlap(t *testing.T) {
	db := runtimeSecretMutationIntegrationDB(t)
	handlers, set, user := runtimeSecretMutationFixture(t, db, "mode_overlap")
	set.EnvVars = `{"TOKEN":"public-value"}`
	if err := db.Model(&model.ProjectRuntimeConfigSet{}).Where("id = ?", set.ID).Update("env_vars", set.EnvVars).Error; err != nil {
		t.Fatalf("store public runtime fixture: %v", err)
	}

	prepared, err := prepareRuntimeSecretMutation(runtimeSecretMutationInput{Values: map[string]string{"TOKEN": "secret-value"}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := handlers.mutateRuntimeSecrets(t.Context(), user, prepared, projectRuntimeConfigSetSecretMutationOwner(set.ID, set.ProjectID))
	if err != nil {
		t.Fatalf("mutate runtime secrets: %v", err)
	}

	var stored model.ProjectRuntimeConfigSet
	if err := db.First(&stored, "id = ?", set.ID).Error; err != nil {
		t.Fatalf("reload runtime config set: %v", err)
	}
	if stored.EnvVars != set.EnvVars {
		t.Fatalf("public value changed: env=%q, want %q", stored.EnvVars, set.EnvVars)
	}
	refs := decodeSecretRefs(stored.SecretRefs)
	if got := handlers.secrets.ResolveContext(t.Context(), refs["TOKEN"]); got != "secret-value" {
		t.Fatalf("resolved secret = %q, want secret-value", got)
	}
	if len(response.EnvironmentVariables) != 1 || response.EnvironmentVariables[0].Key != "TOKEN" || response.EnvironmentVariables[0].ValueMode != runtimeEnvironmentValueModeSecret {
		t.Fatalf("mutation response = %#v, want configured TOKEN secret", response.EnvironmentVariables)
	}
	combined := runtimeEnvironmentVariables(stored.EnvVars, stored.SecretRefs)
	if len(combined) != 2 || combined[0].ValueMode != runtimeEnvironmentValueModePublic || combined[1].ValueMode != runtimeEnvironmentValueModeSecret {
		t.Fatalf("combined variables = %#v, want both modes with secret rendered separately", combined)
	}
}

func TestRuntimeSecretMutationRollsBackSecretWhenOwnerUpdateFails(t *testing.T) {
	db := runtimeSecretMutationIntegrationDB(t)
	handlers, set, user := runtimeSecretMutationFixture(t, db, "rollback")
	owner := projectRuntimeConfigSetSecretMutationOwner(set.ID, set.ProjectID)
	oldRef := storeRuntimeSecretFixture(t, handlers, db, "old-value", user.ID, owner.ResourcePrefix+":TOKEN")
	set.SecretRefs = encodeStringMap(map[string]string{"TOKEN": oldRef})
	if err := db.Model(&model.ProjectRuntimeConfigSet{}).Where("id = ?", set.ID).Update("secret_refs", set.SecretRefs).Error; err != nil {
		t.Fatalf("seed secret refs: %v", err)
	}
	owner.SaveRefs = func(*gorm.DB, string) error { return errors.New("forced owner update failure") }

	prepared, err := prepareRuntimeSecretMutation(runtimeSecretMutationInput{Values: map[string]string{"TOKEN": "new-value"}})
	if err != nil {
		t.Fatalf("prepare mutation: %v", err)
	}
	if _, err := handlers.mutateRuntimeSecrets(t.Context(), user, prepared, owner); err == nil {
		t.Fatal("mutateRuntimeSecrets() error = nil, want rollback")
	}

	var reloaded model.ProjectRuntimeConfigSet
	if err := db.First(&reloaded, "id = ?", set.ID).Error; err != nil {
		t.Fatalf("reload config set: %v", err)
	}
	if refs := decodeSecretRefs(reloaded.SecretRefs); refs["TOKEN"] != oldRef {
		t.Fatalf("refs after rollback = %#v, want original ref", refs)
	}
	var count int64
	if err := db.Model(&model.SecretValue{}).Count(&count).Error; err != nil {
		t.Fatalf("count secret values: %v", err)
	}
	if count != 1 {
		t.Fatalf("secret row count after rollback = %d, want 1", count)
	}
	assertRuntimeSecretAuditCount(t, db, owner.AuditAction, set.ID, 0)
}

func TestDeploymentTargetRuntimeSecretMutationUsesScopedOwner(t *testing.T) {
	db := runtimeSecretMutationIntegrationDB(t)
	target := model.DeploymentTarget{
		ID: "dt_runtime_secret", ProjectID: "prj_runtime_secret", ApplicationID: "app_runtime_secret",
		Name: "Runtime secret target", Stage: "dev", SecretRefs: "{}", CreatedBy: "usr_runtime_secret",
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create deployment target: %v", err)
	}
	handlers := &Handlers{db: db, secrets: secret.NewStore(db, nil, mustTestSecretCodec(t))}
	user := model.User{ID: "usr_runtime_secret"}
	owner := deploymentTargetRuntimeSecretMutationOwner(target.ID, target.ProjectID, target.ApplicationID)
	prepared, err := prepareRuntimeSecretMutation(runtimeSecretMutationInput{Values: map[string]string{"TOKEN": "target-value"}})
	if err != nil {
		t.Fatalf("prepare mutation: %v", err)
	}
	if _, err := handlers.mutateRuntimeSecrets(t.Context(), user, prepared, owner); err != nil {
		t.Fatalf("mutate deployment target secrets: %v", err)
	}
	var reloaded model.DeploymentTarget
	if err := db.First(&reloaded, "id = ?", target.ID).Error; err != nil {
		t.Fatalf("reload deployment target: %v", err)
	}
	refs := decodeSecretRefs(reloaded.SecretRefs)
	if got := handlers.secrets.ResolveContext(t.Context(), refs["TOKEN"]); got != "target-value" {
		t.Fatalf("deployment target TOKEN = %q, want target-value", got)
	}
	assertRuntimeSecretAuditCount(t, db, owner.AuditAction, target.ID, 1)
}

func TestRuntimeSecretMutationRejectsCorruptRefsWithoutWriting(t *testing.T) {
	db := runtimeSecretMutationIntegrationDB(t)
	handlers, set, user := runtimeSecretMutationFixture(t, db, "corrupt_refs")
	if err := db.Model(&model.ProjectRuntimeConfigSet{}).Where("id = ?", set.ID).Update("secret_refs", "{invalid").Error; err != nil {
		t.Fatalf("seed corrupt refs: %v", err)
	}
	owner := projectRuntimeConfigSetSecretMutationOwner(set.ID, set.ProjectID)
	prepared, err := prepareRuntimeSecretMutation(runtimeSecretMutationInput{Values: map[string]string{"TOKEN": "new-value"}})
	if err != nil {
		t.Fatalf("prepare mutation: %v", err)
	}
	if _, err := handlers.mutateRuntimeSecrets(t.Context(), user, prepared, owner); err == nil {
		t.Fatal("mutateRuntimeSecrets() error = nil, want corrupt-state rejection")
	}
	var reloaded model.ProjectRuntimeConfigSet
	if err := db.First(&reloaded, "id = ?", set.ID).Error; err != nil {
		t.Fatalf("reload config set: %v", err)
	}
	if reloaded.SecretRefs != "{invalid" {
		t.Fatalf("corrupt refs were overwritten with %q", reloaded.SecretRefs)
	}
	var secretCount int64
	if err := db.Model(&model.SecretValue{}).Count(&secretCount).Error; err != nil {
		t.Fatalf("count secret rows: %v", err)
	}
	if secretCount != 0 {
		t.Fatalf("secret row count = %d, want 0", secretCount)
	}
	assertRuntimeSecretAuditCount(t, db, owner.AuditAction, set.ID, 0)
}

func TestRuntimeSecretMutationSerializesConcurrentDifferentKeys(t *testing.T) {
	db := runtimeSecretMutationIntegrationDB(t)
	handlers, set, user := runtimeSecretMutationFixture(t, db, "concurrent")
	firstOwner := projectRuntimeConfigSetSecretMutationOwner(set.ID, set.ProjectID)
	secondOwner := projectRuntimeConfigSetSecretMutationOwner(set.ID, set.ProjectID)
	firstOriginalSave := firstOwner.SaveRefs
	firstReachedSave := make(chan struct{})
	releaseFirstSave := make(chan struct{})
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(releaseFirstSave) }) }
	defer releaseFirst()
	firstOwner.SaveRefs = func(tx *gorm.DB, encoded string) error {
		close(firstReachedSave)
		<-releaseFirstSave
		return firstOriginalSave(tx, encoded)
	}
	firstPrepared, err := prepareRuntimeSecretMutation(runtimeSecretMutationInput{Values: map[string]string{"FIRST_TOKEN": "first"}})
	if err != nil {
		t.Fatalf("prepare first mutation: %v", err)
	}
	secondPrepared, err := prepareRuntimeSecretMutation(runtimeSecretMutationInput{Values: map[string]string{"SECOND_TOKEN": "second"}})
	if err != nil {
		t.Fatalf("prepare second mutation: %v", err)
	}
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := handlers.mutateRuntimeSecrets(t.Context(), user, firstPrepared, firstOwner)
		firstDone <- err
	}()
	select {
	case <-firstReachedSave:
	case <-time.After(2 * time.Second):
		t.Fatal("first mutation did not reach save while holding the owner row lock")
	}
	go func() {
		_, err := handlers.mutateRuntimeSecrets(t.Context(), user, secondPrepared, secondOwner)
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("second mutation completed before the first released its row lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	releaseFirst()
	if err := <-firstDone; err != nil {
		t.Fatalf("first concurrent mutation: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second concurrent mutation: %v", err)
	}

	var reloaded model.ProjectRuntimeConfigSet
	if err := db.First(&reloaded, "id = ?", set.ID).Error; err != nil {
		t.Fatalf("reload config set: %v", err)
	}
	refs := decodeSecretRefs(reloaded.SecretRefs)
	if len(refs) != 2 {
		t.Fatalf("refs after concurrent mutations = %#v, want both keys", refs)
	}
	if got := handlers.secrets.ResolveContext(t.Context(), refs["FIRST_TOKEN"]); got != "first" {
		t.Fatalf("FIRST_TOKEN = %q, want first", got)
	}
	if got := handlers.secrets.ResolveContext(t.Context(), refs["SECOND_TOKEN"]); got != "second" {
		t.Fatalf("SECOND_TOKEN = %q, want second", got)
	}
	assertRuntimeSecretAuditCount(t, db, firstOwner.AuditAction, set.ID, 2)
}

func TestRuntimeSecretMutationRollsBackOwnerUpdateWhenDeleteFails(t *testing.T) {
	db := runtimeSecretMutationIntegrationDB(t)
	handlers, set, user := runtimeSecretMutationFixture(t, db, "delete_failure")
	owner := projectRuntimeConfigSetSecretMutationOwner(set.ID, set.ProjectID)
	oldRef := storeRuntimeSecretFixture(t, handlers, db, "old-value", user.ID, owner.ResourcePrefix+":TOKEN")
	set.SecretRefs = encodeStringMap(map[string]string{"TOKEN": oldRef})
	if err := db.Model(&model.ProjectRuntimeConfigSet{}).Where("id = ?", set.ID).Update("secret_refs", set.SecretRefs).Error; err != nil {
		t.Fatalf("seed secret refs: %v", err)
	}
	callbackName := "test:runtime_secret_delete_failure"
	if err := db.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "SecretValue" {
			tx.AddError(errors.New("forced secret delete failure"))
		}
	}); err != nil {
		t.Fatalf("register delete callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Delete().Remove(callbackName) })

	prepared, err := prepareRuntimeSecretMutation(runtimeSecretMutationInput{Clear: []string{"TOKEN"}})
	if err != nil {
		t.Fatalf("prepare mutation: %v", err)
	}
	if _, err := handlers.mutateRuntimeSecrets(t.Context(), user, prepared, owner); err == nil {
		t.Fatal("mutateRuntimeSecrets() error = nil, want delete rollback")
	}
	assertRuntimeSecretOwnerAndRowUnchanged(t, db, set.ID, oldRef)
	assertRuntimeSecretAuditCount(t, db, owner.AuditAction, set.ID, 0)
}

func TestRuntimeSecretMutationRollsBackWhenAuditFails(t *testing.T) {
	db := runtimeSecretMutationIntegrationDB(t)
	handlers, set, user := runtimeSecretMutationFixture(t, db, "audit_failure")
	owner := projectRuntimeConfigSetSecretMutationOwner(set.ID, set.ProjectID)
	oldRef := storeRuntimeSecretFixture(t, handlers, db, "old-value", user.ID, owner.ResourcePrefix+":TOKEN")
	set.SecretRefs = encodeStringMap(map[string]string{"TOKEN": oldRef})
	if err := db.Model(&model.ProjectRuntimeConfigSet{}).Where("id = ?", set.ID).Update("secret_refs", set.SecretRefs).Error; err != nil {
		t.Fatalf("seed secret refs: %v", err)
	}
	callbackName := "test:runtime_secret_audit_failure"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "AuditLog" {
			tx.AddError(errors.New("forced audit failure"))
		}
	}); err != nil {
		t.Fatalf("register audit callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	prepared, err := prepareRuntimeSecretMutation(runtimeSecretMutationInput{Values: map[string]string{"TOKEN": "new-value"}})
	if err != nil {
		t.Fatalf("prepare mutation: %v", err)
	}
	if _, err := handlers.mutateRuntimeSecrets(t.Context(), user, prepared, owner); err == nil {
		t.Fatal("mutateRuntimeSecrets() error = nil, want audit rollback")
	}
	assertRuntimeSecretOwnerAndRowUnchanged(t, db, set.ID, oldRef)
	assertRuntimeSecretAuditCount(t, db, owner.AuditAction, set.ID, 0)
}

func runtimeSecretMutationIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "runtime-secret-integration-test-key")
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("runtime_secret_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open schema database: %v", err)
	}
	if err := db.AutoMigrate(&model.ProjectRuntimeConfigSet{}, &model.DeploymentTarget{}, &model.SecretValue{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate integration schema: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(5)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		_ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
	})
	return db
}

func runtimeSecretMutationFixture(t *testing.T, db *gorm.DB, suffix string) (*Handlers, model.ProjectRuntimeConfigSet, model.User) {
	t.Helper()
	set := model.ProjectRuntimeConfigSet{
		ID: "prcs_" + suffix, ProjectID: "prj_runtime_secret", Name: "Runtime secrets " + suffix,
		SecretRefs: "{}", Enabled: true, CreatedBy: "usr_runtime_secret",
	}
	if err := db.Create(&set).Error; err != nil {
		t.Fatalf("create runtime config set: %v", err)
	}
	handlers := &Handlers{db: db, secrets: secret.NewStore(db, nil, mustTestSecretCodec(t))}
	return handlers, set, model.User{ID: "usr_runtime_secret"}
}

func storeRuntimeSecretFixture(t *testing.T, handlers *Handlers, db *gorm.DB, value, userID, resource string) string {
	t.Helper()
	ref, err := handlers.secrets.StoreContextWithDB(t.Context(), db, value, userID, resource)
	if err != nil {
		t.Fatalf("store runtime secret fixture: %v", err)
	}
	return ref
}

func assertRuntimeSecretRows(t *testing.T, db *gorm.DB, refs []string, want int64) {
	t.Helper()
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, strings.TrimPrefix(ref, "secret-id:"))
	}
	var count int64
	if err := db.Model(&model.SecretValue{}).Where("id in ?", ids).Count(&count).Error; err != nil {
		t.Fatalf("count superseded secrets: %v", err)
	}
	if count != want {
		t.Fatalf("superseded secret row count = %d, want %d", count, want)
	}
}

func assertRuntimeSecretAuditCount(t *testing.T, db *gorm.DB, action, resource string, want int64) {
	t.Helper()
	var count int64
	query := db.Model(&model.AuditLog{}).Where("action = ?", action)
	if resource != "" {
		query = query.Where("resource = ?", resource)
	}
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("count runtime secret audits: %v", err)
	}
	if count != want {
		t.Fatalf("audit count = %d, want %d", count, want)
	}
}

func assertRuntimeSecretOwnerAndRowUnchanged(t *testing.T, db *gorm.DB, setID, oldRef string) {
	t.Helper()
	var reloaded model.ProjectRuntimeConfigSet
	if err := db.First(&reloaded, "id = ?", setID).Error; err != nil {
		t.Fatalf("reload config set: %v", err)
	}
	if refs := decodeSecretRefs(reloaded.SecretRefs); refs["TOKEN"] != oldRef {
		t.Fatalf("refs after rollback = %#v, want original ref", refs)
	}
	var count int64
	if err := db.Model(&model.SecretValue{}).Count(&count).Error; err != nil {
		t.Fatalf("count secret values: %v", err)
	}
	if count != 1 {
		t.Fatalf("secret row count after rollback = %d, want 1", count)
	}
}
