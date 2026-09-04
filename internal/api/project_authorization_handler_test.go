package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type projectAuthorizationDBObservation struct {
	queries             int
	mutations           int
	projectErr          error
	memberErr           error
	projectLookupID     string
	membershipProjectID string
}

func newProjectAuthorizationTestDB(t *testing.T, role string, memberErr error) (*gorm.DB, *projectAuthorizationDBObservation) {
	return newProjectAuthorizationTestDBWithErrors(t, role, nil, memberErr)
}

func newProjectAuthorizationTestDBWithErrors(t *testing.T, role string, projectErr, memberErr error) (*gorm.DB, *projectAuthorizationDBObservation) {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	observation := &projectAuthorizationDBObservation{projectErr: projectErr, memberErr: memberErr}
	if err := db.Callback().Query().Replace("gorm:query", func(query *gorm.DB) {
		observation.queries++
		switch destination := query.Statement.Dest.(type) {
		case *model.Project:
			observation.projectLookupID = firstQueryString(query)
			if observation.projectErr != nil {
				query.AddError(observation.projectErr)
				return
			}
			*destination = model.Project{ID: observation.projectLookupID, Identifier: "authz"}
		case *model.ProjectMember:
			observation.membershipProjectID = firstQueryString(query)
			if observation.memberErr != nil {
				query.AddError(observation.memberErr)
				return
			}
			*destination = model.ProjectMember{ID: "mem_authz", ProjectID: observation.membershipProjectID, UserID: "usr_authz", Role: role}
		default:
			query.AddError(errors.New("unexpected query after authorization"))
		}
		query.RowsAffected = 1
	}); err != nil {
		t.Fatalf("replace query callback: %v", err)
	}
	countMutation := func(*gorm.DB) { observation.mutations++ }
	if err := db.Callback().Create().Before("gorm:create").Register("test:observe_authz_create", countMutation); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:observe_authz_update", countMutation); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:observe_authz_delete", countMutation); err != nil {
		t.Fatal(err)
	}
	return db, observation
}

func TestProjectLookupDatabaseFailureIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := newProjectAuthorizationTestDBWithErrors(t, "", errors.New("database unavailable"), nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj_authz", nil)
	ctx.Params = gin.Params{{Key: "projectId", Value: "prj_authz"}}

	handlers := &Handlers{db: db}
	handlers.domains = newDomainHandlers(handlers)
	if _, ok := handlers.findProject(ctx); ok {
		t.Fatal("project lookup unexpectedly succeeded")
	}
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"code":"project.lookup_unavailable"`) {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func firstQueryString(query *gorm.DB) string {
	where, ok := query.Statement.Clauses["WHERE"].Expression.(clause.Where)
	if !ok {
		return ""
	}
	for _, expression := range where.Exprs {
		expr, ok := expression.(clause.Expr)
		if !ok {
			continue
		}
		for _, variable := range expr.Vars {
			if value, ok := variable.(string); ok {
				return value
			}
		}
	}
	return ""
}

func TestAuthorizeProjectByIDReplacesExistingProjectParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, observation := newProjectAuthorizationTestDB(t, authz.ProjectRoleViewer, nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/resources", nil)
	ctx.Params = gin.Params{{Key: "projectId", Value: "prj_original"}}
	ctx.Set(currentUserContextKey, model.User{ID: "usr_authz", Role: authz.PlatformRoleUser})

	handlers := &Handlers{db: db}
	handlers.domains = newDomainHandlers(handlers)
	_, project, ok := handlers.authorizeProjectByID(ctx, "prj_requested", authz.ActionProjectRead)
	if !ok {
		t.Fatalf("authorization failed: %d %s", recorder.Code, recorder.Body.String())
	}
	if project.ID != "prj_requested" || observation.projectLookupID != "prj_requested" || observation.membershipProjectID != "prj_requested" {
		t.Fatalf("authorized project = %q, lookup = %q, membership = %q", project.ID, observation.projectLookupID, observation.membershipProjectID)
	}
	if got := ctx.Param("projectId"); got != "prj_original" {
		t.Fatalf("projectId after authorization = %q, want original", got)
	}
}

func TestDeleteDeploymentTargetDeveloperDeniedBeforeSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, observation := newProjectAuthorizationTestDB(t, authz.ProjectRoleDeveloper, nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/projects/prj_authz/applications/app_authz/deployment-targets/dplt_authz", nil)
	ctx.Params = gin.Params{
		{Key: "projectId", Value: "prj_authz"},
		{Key: "applicationId", Value: "app_authz"},
		{Key: "targetId", Value: "dplt_authz"},
	}
	ctx.Set(currentUserContextKey, model.User{ID: "usr_authz", Role: authz.PlatformRoleUser})

	handlers := &Handlers{db: db}
	handlers.domains = newDomainHandlers(handlers)
	handlers.domains.deployment.DeleteDeploymentTarget(ctx)

	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"code":"auth.forbidden"`) {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	if observation.queries != 2 {
		t.Fatalf("query count = %d, want only project and membership authorization queries", observation.queries)
	}
	if observation.mutations != 0 {
		t.Fatalf("mutation count = %d, denied deletion must not touch deployment target or routes", observation.mutations)
	}
}

func TestProjectAuthorizationDatabaseFailureIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, observation := newProjectAuthorizationTestDB(t, "", errors.New("database unavailable"))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj_authz", nil)
	ctx.Params = gin.Params{{Key: "projectId", Value: "prj_authz"}}
	ctx.Set(currentUserContextKey, model.User{ID: "usr_authz", Role: authz.PlatformRoleUser})

	handlers := &Handlers{db: db}
	handlers.domains = newDomainHandlers(handlers)
	_, _, ok := handlers.authorizeProject(ctx, authz.ActionProjectRead)
	if ok {
		t.Fatal("authorization unexpectedly succeeded")
	}
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"code":"auth.project_authorization_unavailable"`) {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	if observation.mutations != 0 {
		t.Fatalf("mutation count = %d, unavailable authorization must fail closed", observation.mutations)
	}
}
