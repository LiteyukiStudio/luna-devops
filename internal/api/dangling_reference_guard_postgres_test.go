package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildruntime"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestGitRegistryClusterAndBuildVariableReferenceGuards(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "reference-guard-test-key")
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "dangling_reference_guards_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.Project{},
				&model.ProjectMember{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.ProjectVolume{},
				&model.ScopedResourceProjectBinding{},
				&model.GitProvider{},
				&model.GitAccount{},
				&model.RepositoryBinding{},
				&model.ArtifactRegistry{},
				&model.RegistryCredential{},
				&model.RuntimeCluster{},
				&model.NetworkEgressException{},
				&model.NetworkEgressExceptionGrant{},
				&model.BuildVariableSet{},
				&model.BuildEnvironmentConfig{},
				&model.BuildRun{},
				&model.BuildJob{},
				&model.NotificationChannel{},
				&model.NotificationRule{},
				&model.NotificationTemplate{},
				&model.SecretValue{},
				&model.AuditLog{},
			)
		},
	})
	enqueuer := &fakeBuildTaskEnqueuer{}
	handlers := &Handlers{
		db:         db,
		projects:   repository.NewProjectRepository(db),
		secrets:    secret.NewStore(db, nil, mustTestSecretCodec(t)),
		taskClient: enqueuer,
	}
	handlers.domains = newDomainHandlers(handlers)

	admin := model.User{ID: "usr_admin", Email: "admin@example.com", Role: authz.PlatformRoleAdmin}
	owner := model.User{ID: "usr_owner", Email: "owner@example.com", Role: authz.PlatformRoleUser}
	prjA := model.Project{ID: "prj_a", Identifier: "alpha", Name: "Alpha", KubernetesNamespace: "luna-alpha", DeleteStatus: "active"}
	prjB := model.Project{ID: "prj_b", Identifier: "beta", Name: "Beta", KubernetesNamespace: "luna-beta", DeleteStatus: "active"}
	appA := model.Application{ID: "app_a", ProjectID: prjA.ID, Identifier: "web", Name: "Web", DeleteStatus: "active"}
	appA2 := model.Application{ID: "app_a2", ProjectID: prjA.ID, Identifier: "worker", Name: "Worker", DeleteStatus: "active"}
	appB := model.Application{ID: "app_b", ProjectID: prjB.ID, Identifier: "api", Name: "API", DeleteStatus: "active"}

	for _, row := range []any{
		&admin, &owner, &prjA, &prjB, &appA, &appA2, &appB,
		&model.ProjectMember{ID: "pm_a", ProjectID: prjA.ID, UserID: owner.ID, Role: authz.ProjectRoleOwner},
		&model.ProjectMember{ID: "pm_b", ProjectID: prjB.ID, UserID: owner.ID, Role: authz.ProjectRoleOwner},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	// 1. GitProvider / GitAccount / RepositoryBinding guards.
	provider := model.GitProvider{
		ID: "gitp_shared", Type: "gitea", Name: "Shared Gitea", BaseURL: "https://git.example.com",
		Scope: "project", AuthType: "pat", Enabled: true,
	}
	account := model.GitAccount{
		ID: "gita_shared", ProviderID: provider.ID, UserID: owner.ID, Scope: "project", Username: "bot",
	}
	repoBinding := model.RepositoryBinding{
		ID: "rpb_web", ProjectID: prjA.ID, ApplicationID: appA.ID, GitProviderID: provider.ID, GitAccountID: account.ID,
		Owner: "org", Repo: "web", CloneURL: "https://git.example.com/org/web.git", DefaultBranch: "main",
	}
	registry := model.ArtifactRegistry{
		ID: "reg_shared", Name: "Shared Harbor", Provider: "harbor", Endpoint: "https://reg.example.com", Scope: "project",
	}
	cluster := model.RuntimeCluster{
		ID: "clu_shared", Name: "Shared Cluster", Endpoint: "https://k8s.example.com:6443", Scope: "project", DeleteStatus: "active",
	}
	varSet := model.BuildVariableSet{
		ID: "bvs_shared", Name: "Shared Vars", Scope: "project", Variables: `{"FOO":"bar"}`, SecretRefs: `{}`, Enabled: true,
	}
	bindings := []model.ScopedResourceProjectBinding{
		{ID: "srpb_gp_a", ResourceType: "git_provider", ResourceID: provider.ID, ProjectID: prjA.ID},
		{ID: "srpb_gp_b", ResourceType: "git_provider", ResourceID: provider.ID, ProjectID: prjB.ID},
		{ID: "srpb_ga_a", ResourceType: "git_account", ResourceID: account.ID, ProjectID: prjA.ID},
		{ID: "srpb_ga_b", ResourceType: "git_account", ResourceID: account.ID, ProjectID: prjB.ID},
		{ID: "srpb_reg_a", ResourceType: "artifact_registry", ResourceID: registry.ID, ProjectID: prjA.ID},
		{ID: "srpb_reg_b", ResourceType: "artifact_registry", ResourceID: registry.ID, ProjectID: prjB.ID},
		{ID: "srpb_clu_a", ResourceType: "runtime_cluster", ResourceID: cluster.ID, ProjectID: prjA.ID},
		{ID: "srpb_clu_b", ResourceType: "runtime_cluster", ResourceID: cluster.ID, ProjectID: prjB.ID},
		{ID: "srpb_bvs_a", ResourceType: "build_variable_set", ResourceID: varSet.ID, ProjectID: prjA.ID},
		{ID: "srpb_bvs_b", ResourceType: "build_variable_set", ResourceID: varSet.ID, ProjectID: prjB.ID},
	}
	targetA := model.DeploymentTarget{
		ID: "dplt_a", ProjectID: prjA.ID, ApplicationID: appA.ID, Name: "prod", Stage: "prod",
		ClusterID: cluster.ID, SourceType: "repository", RepositoryBindingID: repoBinding.ID,
		TargetRegistryID: registry.ID, TargetRepository: "alpha/web", TargetTag: "latest",
		BuildVariableSetIDs: `["bvs_shared"]`, Enabled: true, DeleteStatus: "active",
	}
	targetB := model.DeploymentTarget{
		ID: "dplt_b", ProjectID: prjB.ID, ApplicationID: appB.ID, Name: "prod", Stage: "prod",
		ClusterID: "clu_other", SourceType: "image", ImageRef: "nginx:alpine",
		BuildVariableSetIDs: `["bvs_shared"]`, Enabled: true, DeleteStatus: "active",
	}
	exc := model.NetworkEgressException{ID: "nex_b", RuntimeClusterID: cluster.ID, Name: "Beta DB", CIDR: "10.30.0.0/16", Protocol: "tcp", Reason: "db"}
	grantB := model.NetworkEgressExceptionGrant{ID: "nexg_b", ExceptionID: exc.ID, ProjectID: prjB.ID}

	for _, row := range []any{&provider, &account, &repoBinding, &registry, &cluster, &varSet, &bindings, &targetA, &targetB, &exc, &grantB} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	// Deleting RepositoryBinding referenced by targetA returns 409 git.repository_binding_in_use.
	rec, ctx := newScopedNotificationTestContext(
		http.MethodDelete,
		"/api/v1/projects/prj_a/repository-bindings/"+repoBinding.ID,
		owner,
		gin.Params{{Key: "projectId", Value: prjA.ID}, {Key: "bindingId", Value: repoBinding.ID}},
		"",
	)
	handlers.domains.git.DeleteRepositoryBinding(ctx)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "git.repository_binding_in_use") {
		t.Fatalf("delete referenced repository binding = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Deleting GitProvider referenced by repoBinding returns 409 git.provider_in_use.
	rec, ctx = newScopedNotificationTestContext(
		http.MethodDelete,
		"/api/v1/git/providers/"+provider.ID,
		admin,
		gin.Params{{Key: "providerId", Value: provider.ID}},
		"",
	)
	handlers.domains.git.DeleteGitProvider(ctx)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "git.provider_in_use") {
		t.Fatalf("delete referenced git provider = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Narrowing GitAccount projectIds to remove prjA returns 409 git.account_in_use.
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/git/accounts/"+account.ID,
		owner,
		gin.Params{{Key: "accountId", Value: account.ID}},
		`{"providerId":"gitp_shared","scope":"project","projectIds":["prj_b"],"username":"bot"}`,
	)
	handlers.domains.git.UpdateGitAccount(ctx)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "git.account_in_use") {
		t.Fatalf("narrow git account removing referenced project = %d, body = %s", rec.Code, rec.Body.String())
	}

	// 2. Deleting ArtifactRegistry or narrowing projectIds to remove prjA returns 409 registry.in_use.
	rec, ctx = newScopedNotificationTestContext(
		http.MethodDelete,
		"/api/v1/registries/"+registry.ID,
		owner,
		gin.Params{{Key: "registryId", Value: registry.ID}},
		"",
	)
	handlers.domains.registry.DeleteArtifactRegistry(ctx)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "registry.in_use") {
		t.Fatalf("delete referenced registry = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec, ctx = newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/registries/"+registry.ID,
		owner,
		gin.Params{{Key: "registryId", Value: registry.ID}},
		`{"name":"Shared Harbor","provider":"harbor","endpoint":"https://reg.example.com","scope":"project","projectIds":["prj_b"]}`,
	)
	handlers.domains.registry.UpdateArtifactRegistry(ctx)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "registry.in_use") {
		t.Fatalf("narrow registry removing referenced project = %d, body = %s", rec.Code, rec.Body.String())
	}

	// 3. RuntimeCluster narrowing: removing prjA (which has targetA) returns 409 runtime_cluster.in_use;
	// removing prjB (which has no workloads, only grantB) succeeds and prunes grantB.
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/runtime/clusters/"+cluster.ID,
		owner,
		gin.Params{{Key: "clusterId", Value: cluster.ID}},
		`{"name":"Shared Cluster","endpoint":"https://k8s.example.com:6443","scope":"project","projectIds":["prj_b"]}`,
	)
	handlers.domains.runtime.UpdateRuntimeCluster(ctx)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "runtime_cluster.in_use") {
		t.Fatalf("narrow runtime cluster removing active project = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec, ctx = newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/runtime/clusters/"+cluster.ID,
		owner,
		gin.Params{{Key: "clusterId", Value: cluster.ID}},
		`{"name":"Shared Cluster","endpoint":"https://k8s.example.com:6443","scope":"project","projectIds":["prj_a"]}`,
	)
	handlers.domains.runtime.UpdateRuntimeCluster(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("narrow runtime cluster removing idle project = %d, body = %s", rec.Code, rec.Body.String())
	}
	var remainingGrants int64
	db.Model(&model.NetworkEgressExceptionGrant{}).Where("id = ?", grantB.ID).Count(&remainingGrants)
	if remainingGrants != 0 {
		t.Fatalf("expected egress grant for unbound project to be pruned, got count = %d", remainingGrants)
	}

	// 4. BuildVariableSet narrowing prunes removed project's DeploymentTarget.BuildVariableSetIDs,
	// and disabling a BuildVariableSet skips it cleanly instead of failing build resolution.
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/build/variable-sets/"+varSet.ID,
		owner,
		gin.Params{{Key: "setId", Value: varSet.ID}},
		`{"name":"Shared Vars","scope":"project","projectIds":["prj_a"],"variables":{"FOO":"bar"},"enabled":false}`,
	)
	handlers.domains.build.UpdateBuildVariableSet(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("update build variable set = %d, body = %s", rec.Code, rec.Body.String())
	}
	var refreshedTargetB model.DeploymentTarget
	if err := db.First(&refreshedTargetB, "id = ?", targetB.ID).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(refreshedTargetB.BuildVariableSetIDs, varSet.ID) {
		t.Fatalf("targetB build_variable_set_ids should be pruned after unbinding prj_b, got %q", refreshedTargetB.BuildVariableSetIDs)
	}

	// Disabled BuildVariableSet on targetA does not fail build variable resolution.
	run := model.BuildRun{
		ID: "brn_disabled_set", ProjectID: prjA.ID, ApplicationID: appA.ID, DeploymentTargetID: targetA.ID,
		BuildVariableSetIDs: `["bvs_shared"]`, CreatedBy: owner.ID,
	}
	resolver := buildruntime.Resolver{DB: db, Secrets: handlers.secrets}
	if _, err := resolver.ResolveBuildTask(t.Context(), db, run, model.BuildJob{ID: "bjb_1", BuildRunID: run.ID, ProjectID: prjA.ID}); err != nil && strings.Contains(err.Error(), "variables are unavailable") {
		t.Fatalf("disabled variable set should be skipped, got error = %v", err)
	}

	// 5. RegistryCredential update/delete guards + transactional secret rollback.
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/registries/"+registry.ID+"/credentials",
		owner,
		gin.Params{{Key: "registryId", Value: registry.ID}},
		`{"name":"push-primary","username":"pushbot","password":"secret-password-1","usage":"push-pull","scope":"project","projectIds":["prj_a","prj_b"]}`,
	)
	handlers.domains.registry.CreateRegistryCredential(ctx)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create primary registry credential = %d, body = %s", rec.Code, rec.Body.String())
	}
	var primaryCred model.RegistryCredential
	if err := db.First(&primaryCred, "registry_id = ? and name = ?", registry.ID, "push-primary").Error; err != nil {
		t.Fatal(err)
	}
	var secretCountBefore int64
	db.Model(&model.SecretValue{}).Count(&secretCountBefore)

	// Downgrading the sole push credential covering targetA (prj_a) to pull-only returns 409 registry.credential_in_use
	// and rolls back any newly written SecretValue row.
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/registries/"+registry.ID+"/credentials/"+primaryCred.ID,
		owner,
		gin.Params{{Key: "registryId", Value: registry.ID}, {Key: "credentialId", Value: primaryCred.ID}},
		`{"name":"push-primary","username":"pushbot","password":"rotated-password-rejected","usage":"pull","scope":"project","projectIds":["prj_a","prj_b"]}`,
	)
	handlers.domains.registry.UpdateRegistryCredential(ctx)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "registry.credential_in_use") {
		t.Fatalf("downgrade sole push credential = %d, body = %s", rec.Code, rec.Body.String())
	}
	var secretCountAfterReject int64
	db.Model(&model.SecretValue{}).Count(&secretCountAfterReject)
	if secretCountAfterReject != secretCountBefore {
		t.Fatalf("rejected credential update leaked SecretValue rows: before=%d after=%d", secretCountBefore, secretCountAfterReject)
	}

	// Deleting the sole push credential returns 409 registry.credential_in_use.
	rec, ctx = newScopedNotificationTestContext(
		http.MethodDelete,
		"/api/v1/registries/"+registry.ID+"/credentials/"+primaryCred.ID,
		owner,
		gin.Params{{Key: "registryId", Value: registry.ID}, {Key: "credentialId", Value: primaryCred.ID}},
		"",
	)
	handlers.domains.registry.DeleteRegistryCredential(ctx)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "registry.credential_in_use") {
		t.Fatalf("delete sole push credential = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Adding a second push credential covering prj_a allows deleting primaryCred and clears registry.CredentialRef.
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/registries/"+registry.ID+"/credentials",
		owner,
		gin.Params{{Key: "registryId", Value: registry.ID}},
		`{"name":"push-backup","username":"backupbot","password":"secret-password-2","usage":"push","scope":"project","projectIds":["prj_a"]}`,
	)
	handlers.domains.registry.CreateRegistryCredential(ctx)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create backup registry credential = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec, ctx = newScopedNotificationTestContext(
		http.MethodDelete,
		"/api/v1/registries/"+registry.ID+"/credentials/"+primaryCred.ID,
		owner,
		gin.Params{{Key: "registryId", Value: registry.ID}, {Key: "credentialId", Value: primaryCred.ID}},
		"",
	)
	handlers.domains.registry.DeleteRegistryCredential(ctx)
	if ctx.Writer.Status() != http.StatusNoContent {
		t.Fatalf("delete primary push credential with backup = %d, body = %s", ctx.Writer.Status(), rec.Body.String())
	}
	var refreshedRegistry model.ArtifactRegistry
	if err := db.First(&refreshedRegistry, "id = ?", registry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshedRegistry.CredentialRef != "" {
		t.Fatalf("expected registry credential_ref cleared after deleting referenced credential, got %q", refreshedRegistry.CredentialRef)
	}

	// 6. RuntimeCluster updates remain allowed while a bound project is in delete_status = 'deleting'
	// (so admins can rotate kubeconfig/endpoints to unblock project cleanup), without pruning its egress grant.
	grantA := model.NetworkEgressExceptionGrant{ID: "nexg_a_deleting", ExceptionID: exc.ID, ProjectID: prjA.ID}
	if err := db.Create(&grantA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Project{}).Where("id = ?", prjA.ID).Update("delete_status", "deleting").Error; err != nil {
		t.Fatal(err)
	}
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/runtime/clusters/"+cluster.ID,
		owner,
		gin.Params{{Key: "clusterId", Value: cluster.ID}},
		`{"name":"Shared Cluster","endpoint":"https://k8s-rotated.example.com:6443","scope":"project","projectIds":["prj_a"],"maxConcurrentBuilds":4}`,
	)
	handlers.domains.runtime.UpdateRuntimeCluster(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("update cluster with deleting bound project = %d, body = %s", rec.Code, rec.Body.String())
	}
	var grantACount int64
	db.Model(&model.NetworkEgressExceptionGrant{}).Where("id = ?", grantA.ID).Count(&grantACount)
	if grantACount != 1 {
		t.Fatalf("expected egress grant for deleting bound project to remain intact, count = %d", grantACount)
	}
	if err := db.Model(&model.Project{}).Where("id = ?", prjA.ID).Update("delete_status", "active").Error; err != nil {
		t.Fatal(err)
	}

	// 7. Global NotificationRule rejects non-existent or out-of-scope applicationIds / deploymentTargetIds.
	globalCh := model.NotificationChannel{
		ID: "nch_global_rule_test", Scope: "global", Name: "Global Webhook", AdapterKind: "webhook",
		ConfigJSON: `{"url":"https://hooks.example.com/notify","method":"POST"}`, SecretRefsJSON: `{}`, Enabled: true,
	}
	if err := db.Create(&globalCh).Error; err != nil {
		t.Fatal(err)
	}
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/notifications/rules",
		admin,
		nil,
		`{"name":"Global Invalid App","eventTypes":["release.failed"],"channelIds":["nch_global_rule_test"],"filter":{"scope":"projects","projectIds":["prj_b"],"applicationIds":["app_a"]}}`,
	)
	handlers.domains.notification.CreateNotificationRule(ctx)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "notification.rule_application_not_found") {
		t.Fatalf("create global rule with out-of-scope application = %d, body = %s", rec.Code, rec.Body.String())
	}

	// 8. Shared NotificationChannel remains editable when an already-bound project is deleting,
	// and lingering rules from soft-deleted projects do not brick channel deletion.
	sharedNch := model.NotificationChannel{
		ID: "nch_shared_deleting", Scope: "project", Name: "Shared Ops", AdapterKind: "webhook",
		ConfigJSON: `{"url":"https://hooks.example.com/ops","method":"POST"}`, SecretRefsJSON: `{}`, Enabled: true,
	}
	sharedNchBindings := []model.ScopedResourceProjectBinding{
		{ID: "srpb_nch_del_a", ResourceType: "notification_channel", ResourceID: sharedNch.ID, ProjectID: prjA.ID},
		{ID: "srpb_nch_del_b", ResourceType: "notification_channel", ResourceID: sharedNch.ID, ProjectID: prjB.ID},
	}
	activeRuleA := model.NotificationRule{
		ID: "nrl_active_a", ProjectID: prjA.ID, Name: "Active Rule A",
		EventTypesJSON: `["release.failed"]`, FilterJSON: `{"scope":"projects","projectIds":["prj_a"]}`,
		ChannelIDsJSON: `["nch_shared_deleting"]`, Enabled: true,
	}
	deletedPrj := model.Project{ID: "prj_soft_deleted", Identifier: "gone", Name: "Gone", DeleteStatus: "deleted"}
	ghostRule := model.NotificationRule{
		ID: "nrl_ghost", ProjectID: deletedPrj.ID, Name: "Ghost Rule",
		EventTypesJSON: `["release.failed"]`, FilterJSON: `{"scope":"projects","projectIds":["prj_soft_deleted"]}`,
		ChannelIDsJSON: `["nch_shared_deleting"]`, Enabled: true,
	}
	for _, row := range []any{&sharedNch, &sharedNchBindings, &activeRuleA, &deletedPrj, &ghostRule} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Delete(&deletedPrj).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Project{}).Where("id = ?", prjA.ID).Update("delete_status", "deleting").Error; err != nil {
		t.Fatal(err)
	}
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/notification-channels/"+sharedNch.ID,
		owner,
		gin.Params{{Key: "channelId", Value: sharedNch.ID}},
		`{"name":"Shared Ops Updated","scope":"project","projectIds":["prj_a","prj_b"],"adapterKind":"webhook","config":{"url":"https://hooks.example.com/ops","method":"POST"},"enabled":false}`,
	)
	handlers.domains.notification.UpdateScopedNotificationChannel(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("update shared channel while bound project is deleting = %d, body = %s", rec.Code, rec.Body.String())
	}
	if err := db.Model(&model.Project{}).Where("id = ?", prjA.ID).Update("delete_status", "active").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&activeRuleA).Error; err != nil {
		t.Fatal(err)
	}
	rec, ctx = newScopedNotificationTestContext(
		http.MethodDelete,
		"/api/v1/notification-channels/"+sharedNch.ID,
		owner,
		gin.Params{{Key: "channelId", Value: sharedNch.ID}},
		"",
	)
	handlers.domains.notification.DeleteScopedNotificationChannel(ctx)
	if ctx.Writer.Status() != http.StatusNoContent {
		t.Fatalf("delete channel with only soft-deleted project ghost rule = %d, body = %s", ctx.Writer.Status(), rec.Body.String())
	}

	// 9. Narrowing a BuildVariableSet to scope="user" prunes DeploymentTargets not created by set.OwnerRef.
	targetOtherCreator := model.DeploymentTarget{
		ID: "dplt_other_creator", ProjectID: prjA.ID, ApplicationID: appA2.ID, Name: "staging", Stage: "staging",
		ClusterID: cluster.ID, SourceType: "image", ImageRef: "nginx:alpine",
		BuildVariableSetIDs: `["bvs_shared"]`, CreatedBy: admin.ID, Enabled: true, DeleteStatus: "active",
	}
	if err := db.Create(&targetOtherCreator).Error; err != nil {
		t.Fatal(err)
	}
	rec, ctx = newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/build/variable-sets/"+varSet.ID,
		owner,
		gin.Params{{Key: "setId", Value: varSet.ID}},
		`{"name":"Personal Vars","scope":"user","variables":{"FOO":"bar"},"enabled":true}`,
	)
	handlers.domains.build.UpdateBuildVariableSet(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("narrow build variable set to user scope = %d, body = %s", rec.Code, rec.Body.String())
	}
	if err := db.First(&targetOtherCreator, "id = ?", targetOtherCreator.ID).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(targetOtherCreator.BuildVariableSetIDs, varSet.ID) {
		t.Fatalf("expected target created by another user to prune user-scoped variable set, got %q", targetOtherCreator.BuildVariableSetIDs)
	}
}
