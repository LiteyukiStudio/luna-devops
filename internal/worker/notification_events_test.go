package worker

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
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

	loadedProject, loadedApplication, loadedTarget := (&Runner{db: db}).notificationContext(project.ID, application.ID, target.ID)
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
	_, _, loadedTarget := runner.notificationContext(target.ProjectID, target.ApplicationID, target.ID)
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

	if got := (&Runner{}).notificationBuildImageRef(run); got != run.ImageRef {
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
	if got := (&Runner{}).notificationBuildImageRef(run); got != want {
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
	_, _, lookedUpTarget := runner.notificationContext(config.ProjectID, "", deletedTarget.ID)
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
