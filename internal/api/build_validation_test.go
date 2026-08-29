package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type buildValidationDBObservation struct {
	queryCount  int
	createCount int
}

func newMissingPushCredentialValidationDB(t *testing.T, parentSpanContext trace.SpanContext) (*gorm.DB, *buildValidationDBObservation) {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	observation := &buildValidationDBObservation{}
	if err := db.Callback().Query().Replace("gorm:query", func(query *gorm.DB) {
		observation.queryCount++
		if parentSpanContext.IsValid() {
			gotSpanContext := trace.SpanContextFromContext(query.Statement.Context)
			if gotSpanContext.TraceID() != parentSpanContext.TraceID() || gotSpanContext.SpanID() != parentSpanContext.SpanID() {
				t.Errorf("database query lost parent trace context: got %s/%s, want %s/%s",
					gotSpanContext.TraceID(), gotSpanContext.SpanID(), parentSpanContext.TraceID(), parentSpanContext.SpanID())
			}
		}
		switch destination := query.Statement.Dest.(type) {
		case *model.Project:
			*destination = model.Project{ID: "prj_test", Identifier: "test"}
		case *model.Application:
			*destination = model.Application{ID: "app_test", ProjectID: "prj_test", Identifier: "api"}
		case *model.DeploymentTarget:
			*destination = model.DeploymentTarget{
				ID:                  "dplt_test",
				ProjectID:           "prj_test",
				ApplicationID:       "app_test",
				Enabled:             true,
				SourceType:          "repository",
				RepositoryBindingID: "rbind_test",
				TargetRegistryID:    "areg_test",
				TargetRepository:    "team/api",
				TargetTag:           "latest",
			}
		case *model.RepositoryBinding:
			*destination = model.RepositoryBinding{ID: "rbind_test", ProjectID: "prj_test", ApplicationID: "app_test"}
		case *model.ArtifactRegistry:
			*destination = model.ArtifactRegistry{ID: "areg_test", Provider: "harbor", Endpoint: "https://registry.example.com"}
		case *model.RegistryCredential:
			query.AddError(gorm.ErrRecordNotFound)
		default:
			t.Errorf("unexpected database query destination %T", query.Statement.Dest)
		}
		query.RowsAffected = 1
	}); err != nil {
		t.Fatalf("replace query callback: %v", err)
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:observe_build_create", func(*gorm.DB) {
		observation.createCount++
	}); err != nil {
		t.Fatalf("register create callback: %v", err)
	}
	return db, observation
}

func TestPrepareBuildRunRequestReturnsCredentialRequiredConflictAndPreservesContext(t *testing.T) {
	parentSpanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:     trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	requestContext := trace.ContextWithRemoteSpanContext(context.Background(), parentSpanContext)

	db, observation := newMissingPushCredentialValidationDB(t, parentSpanContext)

	handlers := &Handlers{db: db}
	run := model.BuildRun{
		ProjectID:          "prj_test",
		ApplicationID:      "app_test",
		DeploymentTargetID: "dplt_test",
		TargetRegistryID:   "areg_test",
	}
	err := handlers.prepareBuildRunRequest(model.User{ID: "usr_test"}, &run, requestContext)
	if err == nil {
		t.Fatal("expected missing registry push credential to reject the build run")
	}
	var requestErr buildRunRequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("error type = %T, want buildRunRequestError", err)
	}
	if requestErr.status != http.StatusConflict || requestErr.code != buildPushCredentialRequiredCode {
		t.Fatalf("error status/code = %d/%q", requestErr.status, requestErr.code)
	}
	if observation.queryCount == 0 {
		t.Fatal("expected build validation to query through the request context")
	}
}

func TestCreateQueuedBuildRunRejectsMissingPushCredentialWithoutSideEffects(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	for _, test := range []struct {
		name        string
		triggerType string
	}{
		{name: "trigger", triggerType: "manual"},
		{name: "retry", triggerType: "retry"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, observation := newMissingPushCredentialValidationDB(t, trace.SpanContext{})
			taskClient := &fakeBuildTaskEnqueuer{}
			handlers := &Handlers{db: db, taskClient: taskClient}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_test/build-runs", nil)

			handlers.createQueuedBuildRun(ctx, model.User{ID: "usr_test"}, model.BuildRun{
				ID:                  "bldr_" + test.name,
				ProjectID:           "prj_test",
				ApplicationID:       "app_test",
				DeploymentTargetID:  "dplt_test",
				TargetRegistryID:    "areg_test",
				TriggerType:         test.triggerType,
				BuildVariableSetIDs: "[]",
			}, "", http.StatusCreated)

			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if recorder.Code != http.StatusConflict || body["code"] != buildPushCredentialRequiredCode {
				t.Fatalf("status/code = %d/%#v", recorder.Code, body["code"])
			}
			if observation.createCount != 0 {
				t.Fatalf("validation failure created %d database records", observation.createCount)
			}
			if taskClient.buildPayload.BuildRunID != "" {
				t.Fatalf("validation failure dispatched build task: %#v", taskClient.buildPayload)
			}
		})
	}
}

func TestBuildRegistryPushCredentialRequiredResponseBoundary(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		language      string
		wantMessage   string
		wantDetail    string
		forbidDetails bool
	}{
		{
			name:          "production Chinese",
			mode:          "production",
			language:      "zh-CN",
			wantMessage:   "errors." + buildPushCredentialRequiredCode,
			forbidDetails: true,
		},
		{
			name:          "production English",
			mode:          "production",
			language:      "en-US",
			wantMessage:   "errors." + buildPushCredentialRequiredCode,
			forbidDetails: true,
		},
		{
			name:        "development detail",
			mode:        "development",
			language:    "zh-CN",
			wantMessage: "errors.resource.conflict",
			wantDetail:  "目标镜像站缺少可用推送凭据",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			setRuntimeMode(ctx, test.mode)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_test/build-runs/trigger", nil)
			ctx.Request.Header.Set("Accept-Language", test.language)

			writeBuildRunRequestError(ctx, buildRunPublicConflict(
				buildPushCredentialRequiredCode,
				"目标镜像站缺少可用推送凭据",
			))

			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["code"] != buildPushCredentialRequiredCode {
				t.Fatalf("code = %#v", body["code"])
			}
			if body["message"] != test.wantMessage {
				t.Fatalf("message = %#v, want %q", body["message"], test.wantMessage)
			}
			if test.forbidDetails {
				if _, exists := body["detail"]; exists {
					t.Fatalf("production response leaked detail: %#v", body)
				}
			} else if body["developerDetail"] != test.wantDetail {
				t.Fatalf("developerDetail = %#v, want %q", body["developerDetail"], test.wantDetail)
			}
			if body["requestId"] == nil || body["requestId"] == "" {
				t.Fatalf("response is missing requestId: %#v", body)
			}
		})
	}
}

func TestWriteBuildRunRequestErrorKeepsGenericValidationSemantics(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_test/build-runs/trigger", nil)

	writeBuildRunRequestError(ctx, buildRunBadRequest("部署配置不存在或不可用"))

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusBadRequest || body["code"] != "request.invalid" {
		t.Fatalf("generic validation status/code changed: %d/%#v", recorder.Code, body["code"])
	}
}

func TestWriteLocalizedErrorCodeKeepsStableCodeIndependentFromMessageKey(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_test/build-runs/trigger", nil)

	writeLocalizedErrorCode(ctx, http.StatusConflict, "build.test_conflict", "development detail", buildPushCredentialRequiredCode)

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "build.test_conflict" {
		t.Fatalf("code = %#v, want stable caller-provided code", body["code"])
	}
	if body["message"] != "errors."+buildPushCredentialRequiredCode {
		t.Fatalf("message = %#v, want stable frontend localization key", body["message"])
	}
	if _, exists := body["detail"]; exists {
		t.Fatalf("production response leaked detail: %#v", body)
	}
}
