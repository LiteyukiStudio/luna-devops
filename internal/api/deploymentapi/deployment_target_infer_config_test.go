package deploymentapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

type inferConfigTestHost struct {
	aiToolsTestHost
	gitClient gitprovider.Client
	inferFn   func(ctx *gin.Context, user model.User, input aiagent.HeadlessInferenceRequest) (*aiagent.HeadlessInferenceResponse, bool)
	app       model.Application
}

func (h *inferConfigTestHost) AuthorizeProject(_ *gin.Context, _ authz.Action) (model.User, model.Project, bool) {
	return h.user, h.project, true
}

func (h *inferConfigTestHost) FindApplication(_ *gin.Context) (model.Application, bool) {
	return h.app, true
}

func (h *inferConfigTestHost) GitClientForUserBinding(_ *gin.Context, _ model.User, _ model.RepositoryBinding) (gitprovider.Client, bool) {
	return h.gitClient, true
}

func (h *inferConfigTestHost) WriteGitUpstreamError(ctx *gin.Context, err error) {
	writeError(ctx, http.StatusBadGateway, err.Error())
}

func (h *inferConfigTestHost) RunHeadlessInferenceForUser(ctx *gin.Context, user model.User, input aiagent.HeadlessInferenceRequest) (*aiagent.HeadlessInferenceResponse, bool) {
	return h.inferFn(ctx, user, input)
}

func TestInferDeploymentTargetConfigReadsRepoRedactsEnvAndSanitizesOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/git/trees/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"truncated": false,
				"tree": []map[string]string{
					{"path": "Dockerfile", "type": "blob"},
					{"path": "package.json", "type": "blob"},
					{"path": "pnpm-lock.yaml", "type": "blob"},
					{"path": "bun.lockb", "type": "blob"},
					{"path": "compose.yaml", "type": "blob"},
					{"path": ".env.example", "type": "blob"},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/contents/") || strings.HasSuffix(r.URL.Path, "/contents"):
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"path": "Dockerfile", "name": "Dockerfile", "type": "file"},
				{"path": "compose.yaml", "name": "compose.yaml", "type": "file"},
				{"path": ".env.example", "name": ".env.example", "type": "file"},
			})
		case strings.HasSuffix(r.URL.Path, "/contents/Dockerfile"):
			content := base64.StdEncoding.EncodeToString([]byte("FROM node:24-alpine\nARG NPM_TOKEN=dockerfile-inline-npm-token\nEXPOSE 3000\nCMD [\"node\", \"dist/index.js\"]\n"))
			_ = json.NewEncoder(w).Encode(map[string]string{"path": "Dockerfile", "name": "Dockerfile", "encoding": "base64", "content": content})
		case strings.HasSuffix(r.URL.Path, "/contents/compose.yaml"):
			content := base64.StdEncoding.EncodeToString([]byte("services:\n  api:\n    environment:\n      REDIS_PASSWORD: compose-inline-redis-secret\n"))
			_ = json.NewEncoder(w).Encode(map[string]string{"path": "compose.yaml", "name": "compose.yaml", "encoding": "base64", "content": content})
		case strings.HasSuffix(r.URL.Path, "/contents/pnpm-lock.yaml"):
			t.Errorf("lockfile pnpm-lock.yaml should not be read as snippet")
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/contents/bun.lockb"):
			t.Errorf("binary lockfile bun.lockb should not be read as snippet")
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/contents/package.json"):
			content := base64.StdEncoding.EncodeToString([]byte(`{"name":"demo-api","scripts":{"build":"tsc","start":"node dist/index.js"}}`))
			_ = json.NewEncoder(w).Encode(map[string]string{"path": "package.json", "name": "package.json", "encoding": "base64", "content": content})
		case strings.HasSuffix(r.URL.Path, "/contents/.env.example"):
			content := base64.StdEncoding.EncodeToString([]byte("PORT=3000\nJWT_SECRET=super-secret-inline-value\n"))
			_ = json.NewEncoder(w).Encode(map[string]string{"path": ".env.example", "name": ".env.example", "encoding": "base64", "content": content})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gitServer.Close()

	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	project := model.Project{ID: "prj_infer", Identifier: "infer", Name: "Infer", DeleteStatus: "active"}
	app := model.Application{ID: "app_infer", ProjectID: project.ID, Identifier: "web", Name: "Web", DeleteStatus: "active"}
	binding := model.RepositoryBinding{
		ID:            "rpb_infer",
		ProjectID:     project.ID,
		ApplicationID: app.ID,
		Owner:         "acme",
		Repo:          "demo-api",
		DefaultBranch: "main",
	}
	if err := db.Callback().Query().After("gorm:query").Register("test:infer_binding", func(tx *gorm.DB) {
		if dest, ok := tx.Statement.Dest.(*model.RepositoryBinding); ok {
			*dest = binding
		}
	}); err != nil {
		t.Fatal(err)
	}

	var capturedPayload map[string]any
	host := &inferConfigTestHost{
		aiToolsTestHost: aiToolsTestHost{
			db:      db,
			user:    model.User{ID: "usr_infer", Role: authz.PlatformRoleUser},
			project: project,
		},
		app:       app,
		gitClient: gitprovider.NewClientWithPolicy(model.GitProvider{Type: "gitea", BaseURL: gitServer.URL}, "tok", security.AdminEgressPolicy()),
		inferFn: func(_ *gin.Context, _ model.User, input aiagent.HeadlessInferenceRequest) (*aiagent.HeadlessInferenceResponse, bool) {
			capturedPayload = input.ContextPayload
			raw, _ := json.Marshal(map[string]any{
				"summary":             "Node.js 服务，使用仓库 Dockerfile 构建并暴露 3000 端口。",
				"confidence":          "high",
				"buildDefinitionMode": "repository_dockerfile",
				"dockerfilePath":      "../evil",
				"buildContext":        ".",
				"buildArgs": []map[string]any{
					{"key": "NODE_ENV", "value": "production"},
					{"key": "NPM_TOKEN", "value": "leaked-arg-secret"},
				},
				"servicePorts": []map[string]any{
					{"name": "HTTP_BAD!", "port": 3000, "appProtocol": "http"},
					{"name": "http", "port": 8443, "appProtocol": "https"},
				},
				"environmentVariables": []map[string]any{
					{"key": "PORT", "secret": false, "placeholder": "3000"},
					{"key": "JWT_SECRET", "secret": false, "placeholder": "replace-me"},
					{"key": "INVALID-KEY", "secret": false, "placeholder": "ignored"},
				},
				"cpuRequest":    "invalid-qty",
				"memoryRequest": "512Mi",
			})
			return &aiagent.HeadlessInferenceResponse{
				RunID:    "airun_1",
				TaskType: aiagent.HeadlessTaskRepoConfigInfer,
				Output:   raw,
			}, true
		},
	}

	handler := New(host)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Params = gin.Params{
		{Key: "projectId", Value: project.ID},
		{Key: "applicationId", Value: app.ID},
	}
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/"+project.ID+"/applications/"+app.ID+"/deployment-target-config-inference",
		bytes.NewBufferString(`{"repositoryBindingId":"rpb_infer","stage":"prod"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.InferDeploymentTargetConfig(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Ensure .env.example, compose.yaml, and Dockerfile sensitive values were redacted BEFORE being sent to luna-agent!
	payloadJSON, _ := json.Marshal(capturedPayload)
	for _, secretVal := range []string{"super-secret-inline-value", "dockerfile-inline-npm-token", "compose-inline-redis-secret"} {
		if strings.Contains(string(payloadJSON), secretVal) {
			t.Fatalf("sensitive value %q leaked into AI context payload: %s", secretVal, string(payloadJSON))
		}
	}

	var got DeploymentTargetConfigInferenceOutput
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DockerfilePath != "Dockerfile" {
		t.Fatalf("DockerfilePath = %q, want fallback to safe Dockerfile", got.DockerfilePath)
	}
	if len(got.BuildArgs) != 1 || got.BuildArgs[0].Key != "NODE_ENV" {
		t.Fatalf("BuildArgs = %+v, want sensitive NPM_TOKEN stripped and NODE_ENV retained", got.BuildArgs)
	}
	if len(got.ServicePorts) != 2 || got.ServicePorts[0].Name != "http" || got.ServicePorts[1].Name != "port-8443" {
		t.Fatalf("ServicePorts = %+v, want unique port names http and port-8443", got.ServicePorts)
	}
	if len(got.EnvironmentVariables) != 2 || !got.EnvironmentVariables[1].Secret || got.EnvironmentVariables[1].Placeholder != "" {
		t.Fatalf("EnvironmentVariables = %+v, want JWT_SECRET auto-classified as secret with empty placeholder and INVALID-KEY filtered", got.EnvironmentVariables)
	}
	if got.CPURequest != model.DefaultDeploymentCPURequest || got.MemoryRequest != "512Mi" {
		t.Fatalf("CPU/Memory = %s / %s", got.CPURequest, got.MemoryRequest)
	}
	if len(host.audits) == 0 || !strings.Contains(host.audits[0], "deployment_target.ai_infer_config:") {
		t.Fatalf("expected audit log entry, got %v", host.audits)
	}

	// Missing repositoryBindingId -> 400 deployment_target.repository_binding_required
	missingRec := httptest.NewRecorder()
	missingCtx, _ := gin.CreateTestContext(missingRec)
	missingCtx.Request = httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(`{}`))
	missingCtx.Request.Header.Set("Content-Type", "application/json")
	handler.InferDeploymentTargetConfig(missingCtx)
	if missingRec.Code != http.StatusBadRequest || !strings.Contains(missingRec.Body.String(), "deployment_target.repository_binding_required") {
		t.Fatalf("expected 400 deployment_target.repository_binding_required, got %d %s", missingRec.Code, missingRec.Body.String())
	}

	// Deleting application -> 409 application.delete_in_progress
	host.app.DeleteStatus = "deleting"
	deletingRec := httptest.NewRecorder()
	deletingCtx, _ := gin.CreateTestContext(deletingRec)
	deletingCtx.Request = httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(`{"repositoryBindingId":"rpb_infer"}`))
	deletingCtx.Request.Header.Set("Content-Type", "application/json")
	handler.InferDeploymentTargetConfig(deletingCtx)
	if deletingRec.Code != http.StatusConflict || !strings.Contains(deletingRec.Body.String(), "application.delete_in_progress") {
		t.Fatalf("expected 409 application.delete_in_progress, got %d %s", deletingRec.Code, deletingRec.Body.String())
	}
}
