package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestNotificationLinksPointToApplicationTabs(t *testing.T) {
	runner := newDryRunWorkerTestRunner(t, Options{PublicBaseURL: "https://devops.example.com/"})

	links := runner.notificationLinks("prj_1", "app_1", "deployments", "release", "", "")

	if links["project"] != "https://devops.example.com/projects/prj_1" {
		t.Fatalf("project link = %q", links["project"])
	}
	if links["application"] != "https://devops.example.com/projects/prj_1/apps/app_1" {
		t.Fatalf("application link = %q", links["application"])
	}
	if links["release"] != "https://devops.example.com/projects/prj_1/apps/app_1?tab=deployments" {
		t.Fatalf("release link = %q", links["release"])
	}
	if links["primary"] != links["release"] {
		t.Fatalf("primary link = %q, release link = %q", links["primary"], links["release"])
	}
}

func TestNotificationLinksStayEmptyWithoutPublicBaseURL(t *testing.T) {
	runner := newDryRunWorkerTestRunner(t, Options{})

	if links := runner.notificationLinks("prj_1", "app_1", "builds", "build", "buildRunId", "brn_1"); links != nil {
		t.Fatalf("links = %#v", links)
	}
}

func TestNotificationBuildLinkFocusesTheBuildRun(t *testing.T) {
	runner := newDryRunWorkerTestRunner(t, Options{PublicBaseURL: "https://devops.example.com"})

	links := runner.notificationLinks("prj_1", "app_1", "builds", "build", "buildRunId", "brn_1")

	want := "https://devops.example.com/projects/prj_1/apps/app_1?tab=builds#buildRunId=brn_1"
	if links["build"] != want || links["primary"] != want {
		t.Fatalf("build links = %#v, want %q", links, want)
	}
}

func TestGatewayAndCertificateDedupKeysIncludeRouteGeneration(t *testing.T) {
	firstGeneration := time.Date(2026, time.August, 28, 1, 2, 3, 123456000, time.UTC)
	secondGeneration := firstGeneration.Add(time.Microsecond)
	first := model.GatewayRoute{ID: "gwr_generation", UpdatedAt: firstGeneration}
	retry := first
	changed := first
	changed.UpdatedAt = secondGeneration

	if gatewayEventDedupKey(first, "applied") != gatewayEventDedupKey(retry, "applied") {
		t.Fatal("same gateway route generation must remain idempotent")
	}
	if gatewayEventDedupKey(first, "applied") == gatewayEventDedupKey(changed, "applied") {
		t.Fatal("different gateway route generations must produce distinct events")
	}

	notAfter := firstGeneration.Add(24 * time.Hour)
	first.CertificateNotAfter = &notAfter
	retry.CertificateNotAfter = nil
	if certificateEventDedupKey(first, "issued") != certificateEventDedupKey(retry, "issued") {
		t.Fatal("certificate retries in the same route generation must remain idempotent")
	}
	if certificateEventDedupKey(first, "issued") == certificateEventDedupKey(changed, "issued") {
		t.Fatal("different route generations must produce distinct certificate events")
	}
}

func TestNotificationEntityRefsRetainAuthoritativeIDsWhenLookupsFail(t *testing.T) {
	project, application, target := notificationEntityRefs(
		"prj_authoritative",
		"app_authoritative",
		"target_authoritative",
		model.Project{},
		model.Application{},
		model.DeploymentTarget{},
	)

	if project.ID != "prj_authoritative" || application.ID != "app_authoritative" || target.ID != "target_authoritative" {
		t.Fatalf("entity refs lost authoritative IDs: project=%#v application=%#v target=%#v", project, application, target)
	}
}

func TestNotificationEntityRefsRetainIDsForSoftDeletedLookups(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "notification_soft_deleted_context_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.Application{}, &model.DeploymentTarget{})
		},
	})
	deletedAt := gorm.DeletedAt{Time: time.Now(), Valid: true}
	project := model.Project{ID: "prj_deleted", Identifier: "deleted", Name: "Deleted project", DeletedAt: deletedAt}
	application := model.Application{ID: "app_deleted", ProjectID: project.ID, Identifier: "deleted-app", Name: "Deleted app", DeletedAt: deletedAt}
	target := model.DeploymentTarget{ID: "target_deleted", ProjectID: project.ID, ApplicationID: application.ID, Name: "Deleted target", Stage: "prod", CreatedBy: "usr_deleted_owner", DeletedAt: deletedAt}
	for _, item := range []any{&project, &application, &target} {
		if err := db.Unscoped().Create(item).Error; err != nil {
			t.Fatalf("create soft-deleted notification context: %v", err)
		}
	}

	loadedProject, loadedApplication, loadedTarget := (&Runner{db: db}).notificationContext(t.Context(), project.ID, application.ID, target.ID)
	projectRef, applicationRef, targetRef := notificationEntityRefs(project.ID, application.ID, target.ID, loadedProject, loadedApplication, loadedTarget)
	if projectRef.ID != project.ID || applicationRef.ID != application.ID || targetRef.ID != target.ID {
		t.Fatalf("soft-deleted refs lost authoritative IDs: project=%#v application=%#v target=%#v", projectRef, applicationRef, targetRef)
	}
	if projectRef.Name != "" || applicationRef.Name != "" || targetRef.Name != "" {
		t.Fatalf("soft-deleted lookup unexpectedly supplied display data: project=%#v application=%#v target=%#v", projectRef, applicationRef, targetRef)
	}
}

func TestNotificationDeploymentTargetOwnerUsesSoftDeletedTargetForBuildAndRelease(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "notification_soft_deleted_target_owner_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.Application{}, &model.DeploymentTarget{})
		},
	})
	target := model.DeploymentTarget{
		ID:            "target_deleted_event_owner",
		ProjectID:     "prj_deleted_event_owner",
		ApplicationID: "app_deleted_event_owner",
		Name:          "Deleted event owner target",
		Stage:         "prod",
		CreatedBy:     "usr_deleted_event_owner",
		DeletedAt:     gorm.DeletedAt{Time: time.Now(), Valid: true},
	}
	if err := db.Unscoped().Create(&target).Error; err != nil {
		t.Fatalf("create soft-deleted event owner target: %v", err)
	}

	runner := &Runner{db: db}
	_, _, loadedTarget := runner.notificationContext(t.Context(), target.ProjectID, target.ApplicationID, target.ID)
	if loadedTarget.ID != "" {
		t.Fatalf("normal notification context unexpectedly loaded soft-deleted target %#v", loadedTarget)
	}
	build := model.BuildRun{
		ProjectID: target.ProjectID, ApplicationID: target.ApplicationID, DeploymentTargetID: target.ID,
	}
	release := model.Release{
		ProjectID: target.ProjectID, ApplicationID: target.ApplicationID, DeploymentTargetID: target.ID,
	}
	for name, scope := range map[string]struct {
		projectID     string
		applicationID string
		targetID      string
	}{
		"build":   {projectID: build.ProjectID, applicationID: build.ApplicationID, targetID: build.DeploymentTargetID},
		"release": {projectID: release.ProjectID, applicationID: release.ApplicationID, targetID: release.DeploymentTargetID},
	} {
		t.Run(name, func(t *testing.T) {
			if got := runner.notificationDeploymentTargetOwnerUserID(t.Context(), scope.projectID, scope.applicationID, scope.targetID, loadedTarget); got != target.CreatedBy {
				t.Fatalf("soft-deleted target owner = %q, want %q", got, target.CreatedBy)
			}
		})
	}

	if got := runner.notificationDeploymentTargetOwnerUserID(t.Context(), "prj_spoofed", target.ApplicationID, target.ID, loadedTarget); got != "" {
		t.Fatalf("cross-project target owner = %q, want empty", got)
	}
	if got := runner.notificationDeploymentTargetOwnerUserID(t.Context(), target.ProjectID, "app_spoofed", target.ID, loadedTarget); got != "" {
		t.Fatalf("cross-application target owner = %q, want empty", got)
	}
}

func TestNotificationDeploymentTargetOwnerReusesScopedLoadedTarget(t *testing.T) {
	target := model.DeploymentTarget{
		ID: "target_loaded_owner", ProjectID: "prj_loaded_owner", ApplicationID: "app_loaded_owner", CreatedBy: "usr_loaded_owner",
	}
	runner := newDryRunWorkerTestRunner(t, Options{})
	if got := runner.notificationDeploymentTargetOwnerUserID(t.Context(), target.ProjectID, target.ApplicationID, target.ID, target); got != target.CreatedBy {
		t.Fatalf("loaded target owner = %q, want %q", got, target.CreatedBy)
	}
	if got := runner.notificationDeploymentTargetOwnerUserID(t.Context(), target.ProjectID, "app_spoofed", target.ID, target); got != "" {
		t.Fatalf("mismatched loaded target owner = %q, want empty", got)
	}
}

func TestNotificationBuildImageRefPrefersPersistedImage(t *testing.T) {
	run := model.BuildRun{
		ImageRef:         "registry.example.com/team/app:resolved",
		TargetRepository: "registry.example.com/team/app",
		TargetTag:        "fallback",
	}

	if got := (&Runner{}).notificationBuildImageRef(t.Context(), run); got != run.ImageRef {
		t.Fatalf("notificationBuildImageRef() = %q, want %q", got, run.ImageRef)
	}
}

func TestNotificationBuildImageRefFallsBackToBuildTarget(t *testing.T) {
	run := model.BuildRun{
		TargetRepository: "registry.example.com/team/app",
		TargetTag:        "{branchSlug}-{shortSha}",
		SourceBranch:     "feature/identifier-notifications",
		SourceCommit:     "1234567890abcdef",
	}

	want := "registry.example.com/team/app:feature-identifier-notifications-1234567890ab"
	if got := (&Runner{}).notificationBuildImageRef(t.Context(), run); got != want {
		t.Fatalf("notificationBuildImageRef() = %q, want %q", got, want)
	}
}

func TestNotificationHookActorUsesInternalBuildOrReleaseCreator(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "notification_hook_actor_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.User{}, &model.BuildRun{}, &model.Release{})
		},
	})
	users := []model.User{
		{ID: "usr_build", Email: "build-user@example.com", Name: "Build User"},
		{ID: "usr_release", Email: "release-user@example.com", Name: "Release User"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	if err := db.Create(&model.BuildRun{
		ID:               "brn_hook_actor",
		ProjectID:        "prj_hook_actor",
		CreatedBy:        "usr_build",
		TriggeredByName:  "External Trigger",
		TriggeredByEmail: "external-trigger@example.com",
	}).Error; err != nil {
		t.Fatalf("create build run: %v", err)
	}
	if err := db.Create(&model.Release{
		ID:            "rel_hook_actor",
		ProjectID:     "prj_hook_actor",
		ApplicationID: "app_hook_actor",
		ImageRef:      "example.invalid/app:test",
		CreatedBy:     "usr_release",
	}).Error; err != nil {
		t.Fatalf("create release: %v", err)
	}

	runner := &Runner{db: db}
	buildActor := runner.notificationHookActor(t.Context(), model.HookRun{
		ProjectID:  "prj_hook_actor",
		BuildRunID: "brn_hook_actor",
	})
	if buildActor.ID != "usr_build" {
		t.Fatalf("build hook actor ID = %q, want internal build creator", buildActor.ID)
	}
	if buildActor.Email != "external-trigger@example.com" {
		t.Fatalf("build hook display email = %q, want trigger metadata", buildActor.Email)
	}

	releaseActor := runner.notificationHookActor(t.Context(), model.HookRun{
		ProjectID: "prj_hook_actor",
		ReleaseID: "rel_hook_actor",
	})
	if releaseActor.ID != "usr_release" || releaseActor.Email != "release-user@example.com" {
		t.Fatalf("release hook actor = %#v, want internal release creator", releaseActor)
	}

	unknownActor := runner.notificationHookActor(t.Context(), model.HookRun{
		ProjectID:  "prj_hook_actor",
		BuildRunID: "brn_external",
	})
	if unknownActor.ID != "" || unknownActor.Email != "" {
		t.Fatalf("unknown hook actor = %#v, want empty", unknownActor)
	}
}

func TestNotificationHookOwnerPrefersDeploymentTargetThenHookConfig(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "notification_hook_owner_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.ProjectHookConfig{}, &model.DeploymentTarget{})
		},
	})
	config := model.ProjectHookConfig{
		ID:        "hook_owner",
		ProjectID: "prj_hook_owner",
		Name:      "Owner hook",
		Script:    "true",
		Shell:     "sh",
		CreatedBy: "usr_hook_owner",
	}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("create hook config: %v", err)
	}

	runner := &Runner{db: db}
	run := model.HookRun{ProjectID: config.ProjectID, ApplicationID: "app_hook_owner", HookConfigID: config.ID}
	runWithTarget := run
	runWithTarget.DeploymentTargetID = "target_owner"
	if got := runner.notificationHookOwnerUserID(t.Context(), runWithTarget, model.DeploymentTarget{ID: runWithTarget.DeploymentTargetID, ProjectID: run.ProjectID, ApplicationID: run.ApplicationID, CreatedBy: "usr_target_owner"}); got != "usr_target_owner" {
		t.Fatalf("owner with deployment target = %q, want target owner", got)
	}
	if got := runner.notificationHookOwnerUserID(t.Context(), run, model.DeploymentTarget{}); got != config.CreatedBy {
		t.Fatalf("owner without deployment target = %q, want hook config owner", got)
	}
	if got := runner.notificationHookOwnerUserID(t.Context(), runWithTarget, model.DeploymentTarget{}); got != "" {
		t.Fatalf("owner for missing deployment target = %q, want empty", got)
	}
	deletedAt := gorm.DeletedAt{Time: time.Now(), Valid: true}
	deletedTarget := model.DeploymentTarget{
		ID:            "target_deleted_owner",
		ProjectID:     config.ProjectID,
		ApplicationID: "app_hook_owner",
		Name:          "Deleted owner target",
		Stage:         "prod",
		CreatedBy:     "usr_deleted_target_owner",
		DeletedAt:     deletedAt,
	}
	if err := db.Unscoped().Create(&deletedTarget).Error; err != nil {
		t.Fatalf("create soft-deleted deployment target: %v", err)
	}
	runWithTarget.DeploymentTargetID = deletedTarget.ID
	_, _, lookedUpTarget := runner.notificationContext(t.Context(), config.ProjectID, "", deletedTarget.ID)
	if got := runner.notificationHookOwnerUserID(t.Context(), runWithTarget, lookedUpTarget); got != deletedTarget.CreatedBy {
		t.Fatalf("owner for soft-deleted deployment target = %q, want %q", got, deletedTarget.CreatedBy)
	}
	spoofedProjectRun := runWithTarget
	spoofedProjectRun.ProjectID = "prj_spoofed"
	if got := runner.notificationHookOwnerUserID(t.Context(), spoofedProjectRun, lookedUpTarget); got != "" {
		t.Fatalf("owner for cross-project deployment target = %q, want empty", got)
	}
	spoofedApplicationRun := runWithTarget
	spoofedApplicationRun.ApplicationID = "app_spoofed"
	if got := runner.notificationHookOwnerUserID(t.Context(), spoofedApplicationRun, lookedUpTarget); got != "" {
		t.Fatalf("owner for cross-application deployment target = %q, want empty", got)
	}
	if got := runner.notificationHookOwnerUserID(t.Context(), model.HookRun{ProjectID: config.ProjectID, HookConfigID: "hook_missing"}, model.DeploymentTarget{}); got != "" {
		t.Fatalf("owner for missing hook config = %q, want empty", got)
	}
}

func TestDeliveryFailuresPublishAndSyncInboxMessagesPostgres(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "luna_delivery_inbox_sync",
		Migrate: func(db *gorm.DB) error {
			return database.MigrateContext(context.Background(), db)
		},
	})
	ctx := context.Background()

	owner := model.User{ID: "usr_inb_owner", Email: "owner@example.test", Name: "Owner", Role: authz.PlatformRoleUser}
	adminActor := model.User{ID: "usr_inb_admin", Email: "admin@example.test", Name: "Admin", Role: authz.PlatformRoleAdmin}
	viewer := model.User{ID: "usr_inb_viewer", Email: "viewer@example.test", Name: "Viewer", Role: authz.PlatformRoleUser}
	for _, u := range []model.User{owner, adminActor, viewer} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}

	proj := model.Project{ID: "prj_inb_1", Name: "Space", Identifier: "space", DeleteStatus: "active"}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	for _, m := range []model.ProjectMember{
		{ID: "pm_inb_1", ProjectID: proj.ID, UserID: owner.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_inb_2", ProjectID: proj.ID, UserID: viewer.ID, Role: authz.ProjectRoleViewer},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	app := model.Application{ID: "app_inb_1", ProjectID: proj.ID, Name: "Web", Identifier: "web", DeleteStatus: "active"}
	if err := db.Create(&app).Error; err != nil {
		t.Fatal(err)
	}
	target := model.DeploymentTarget{ID: "dplt_inb_1", ProjectID: proj.ID, ApplicationID: app.ID, Name: "Prod", Stage: "prod", WorkloadType: "Deployment", Enabled: true, DeleteStatus: "active"}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}

	runner := &Runner{db: db, publicBaseURL: "https://devops.example.test"}
	run := model.BuildRun{
		ID:                 "bldr_inb_1",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Status:             "failed",
		FailureCode:        "job_failed",
		SourceBranch:       "main",
		CreatedBy:          adminActor.ID,
	}

	// 1. Immediate failure publishes Inbox to Owner AND triggering Admin (not Viewer), redacting secrets.
	runner.publishBuildFailureInboxOnly(ctx, run, "pull failed Authorization: Bearer secret_tok_12345", nil)

	var msgs []model.InboxMessage
	if err := db.Where("project_id = ? AND resource_id = ?", proj.ID, run.ID).Find(&msgs).Error; err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 inbox recipients (owner + triggering admin), got %d: %+v", len(msgs), msgs)
	}
	for _, m := range msgs {
		if m.RecipientUserID == viewer.ID {
			t.Fatalf("viewer should not receive build failure inbox message")
		}
		if strings.Contains(m.ParamsJSON, "secret_tok_12345") {
			t.Fatalf("expected secret token redacted from params_json, got %s", m.ParamsJSON)
		}
		if m.DeepLink != "/projects/prj_inb_1/apps/app_inb_1?tab=builds#buildRunId=bldr_inb_1" {
			t.Fatalf("unexpected deep link %q", m.DeepLink)
		}
	}

	// 2. Subsequent AI diagnosis completion updates the SAME Inbox rows in-place with aiRootCauseSummary!
	runner.publishBuildFailureInboxOnly(ctx, run, "pull failed", &notification.AIDiagnosisContext{
		RootCauseSummary: "Dockerfile FROM nginxx:1.27-alpine has a typo",
		Suggestions:      []string{"Change nginxx to nginx"},
	})
	if err := db.Where("project_id = ? AND resource_id = ?", proj.ID, run.ID).Find(&msgs).Error; err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected coalesced 2 rows after AI update, got %d", len(msgs))
	}
	for _, m := range msgs {
		if !strings.Contains(m.ParamsJSON, "Dockerfile FROM nginxx:1.27-alpine has a typo") {
			t.Fatalf("expected AI diagnosis summary in params_json, got %s", m.ParamsJSON)
		}
	}

	// 3. Release failure also publishes with #releaseId deep link.
	rel := model.Release{
		ID:                 "rel_inb_1",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Status:             "failed",
		Revision:           3,
		CreatedBy:          owner.ID,
	}
	runner.publishReleaseFailureInboxOnly(ctx, rel, "ImagePullBackOff", nil)
	var relMsg model.InboxMessage
	if err := db.First(&relMsg, "recipient_user_id = ? AND resource_id = ?", owner.ID, rel.ID).Error; err != nil {
		t.Fatalf("expected release failure inbox message: %v", err)
	}
	if relMsg.DeepLink != "/projects/prj_inb_1/apps/app_inb_1?tab=deployments#releaseId=rel_inb_1" {
		t.Fatalf("unexpected release deep link: %q", relMsg.DeepLink)
	}

	// 4. Revoked ex-member (non-admin) and downgraded viewer MUST NOT receive Inbox messages even as CreatedBy!
	exMember := model.User{ID: "usr_inb_ex", Email: "ex@example.test", Name: "Ex", Role: authz.PlatformRoleUser}
	if err := db.Create(&exMember).Error; err != nil {
		t.Fatal(err)
	}
	exRun := model.BuildRun{
		ID:                 "bldr_inb_ex",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Status:             "failed",
		FailureCode:        "job_failed",
		CreatedBy:          exMember.ID,
	}
	runner.publishBuildFailureInboxOnly(ctx, exRun, "build failed", nil)
	viewerRun := model.BuildRun{
		ID:                 "bldr_inb_viewer",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Status:             "failed",
		FailureCode:        "job_failed",
		CreatedBy:          viewer.ID,
	}
	runner.publishBuildFailureInboxOnly(ctx, viewerRun, "build failed", nil)
	var leakedCount int64
	if err := db.Model(&model.InboxMessage{}).Where("recipient_user_id IN ?", []string{exMember.ID, viewer.ID}).Count(&leakedCount).Error; err != nil {
		t.Fatal(err)
	}
	if leakedCount != 0 {
		t.Fatalf("expected 0 leaked inbox messages to ex-member or viewer, got %d", leakedCount)
	}

	// 5. Superseded / user-canceled builds & releases MUST NOT publish failure Inbox messages!
	runner.publishReleaseFailureInboxOnly(ctx, model.Release{
		ID:                 "rel_inb_superseded",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Status:             "failed",
		FailureCode:        model.ReleaseFailureSuperseded,
		CreatedBy:          owner.ID,
	}, "superseded", nil)
	runner.publishBuildFailureInboxOnly(ctx, model.BuildRun{
		ID:                 "bldr_inb_canceled",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Status:             "failed",
		FailureCode:        model.BuildFailureCanceledByUser,
		CreatedBy:          owner.ID,
	}, "canceled", nil)
	var noiseCount int64
	if err := db.Model(&model.InboxMessage{}).Where("resource_id IN ?", []string{"rel_inb_superseded", "bldr_inb_canceled"}).Count(&noiseCount).Error; err != nil {
		t.Fatal(err)
	}
	if noiseCount != 0 {
		t.Fatalf("expected 0 inbox messages for superseded/canceled runs, got %d", noiseCount)
	}

	// 6. Timed-out build preserves Type == "build.timeout" across initial publish and AI diagnosis update.
	timeoutRun := model.BuildRun{
		ID:                 "bldr_inb_timeout",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Status:             "timeout",
		FailureCode:        "timeout",
		CreatedBy:          owner.ID,
	}
	runner.publishBuildFailureInboxOnly(ctx, timeoutRun, "timed out", nil)
	_ = runner.emitBuildEventWithAIError(ctx, timeoutRun, "failed", "timed out", &notification.AIDiagnosisContext{
		RootCauseSummary: "npm install hung on private registry",
		Suggestions:      []string{"Configure registry mirror", "Configure registry mirror"},
	})
	var timeoutMsg model.InboxMessage
	if err := db.First(&timeoutMsg, "recipient_user_id = ? AND resource_id = ?", owner.ID, timeoutRun.ID).Error; err != nil {
		t.Fatal(err)
	}
	if timeoutMsg.Type != "build.timeout" {
		t.Fatalf("expected timeoutMsg.Type == build.timeout, got %q", timeoutMsg.Type)
	}
	if strings.Count(timeoutMsg.ParamsJSON, "Configure registry mirror") != 1 {
		t.Fatalf("expected deduplicated suggestions in params_json, got %s", timeoutMsg.ParamsJSON)
	}

	// 7. Patrolled background certificate expiration also publishes certificateFailed to Inbox.
	certRoute := model.GatewayRoute{
		ID:                 "gwr_cert_exp",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Host:               "expired.example.test",
		CreatedBy:          owner.ID,
	}
	runner.emitPatrolledCertificateEvent(ctx, certRoute, "expired", "certificate has expired", "certificate:gwr_cert_exp:expired:1700000000", time.Now().UTC())
	var certMsg model.InboxMessage
	if err := db.First(&certMsg, "recipient_user_id = ? AND resource_id = ?", owner.ID, certRoute.ID).Error; err != nil {
		t.Fatalf("expected patrolled certificate expired inbox message: %v", err)
	}
	if certMsg.TitleKey != "inbox.delivery.certificateFailed.title" {
		t.Fatalf("unexpected cert titleKey %q", certMsg.TitleKey)
	}
}
