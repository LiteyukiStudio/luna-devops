package gitapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type repositoryWebhookAudit struct {
	success bool
	message string
}

type repositoryWebhookTestHost struct {
	Host
	db            *gorm.DB
	store         secret.Store
	publicBaseURL string
	mu            sync.Mutex
	audits        []repositoryWebhookAudit
}

func (h *repositoryWebhookTestHost) DBFor(*gin.Context) *gorm.DB { return h.db }
func (h *repositoryWebhookTestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return h.db.WithContext(ctx)
}
func (h *repositoryWebhookTestHost) CanUseScopedResourceByID(model.User, string, string, string, string, context.Context) bool {
	return true
}
func (h *repositoryWebhookTestHost) AuditWithContext(_, _, _ string, success bool, message string, _ context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.audits = append(h.audits, repositoryWebhookAudit{success: success, message: message})
}
func (h *repositoryWebhookTestHost) SecretStore() secret.Store { return h.store }
func (h *repositoryWebhookTestHost) PublicBaseURL() string     { return h.publicBaseURL }
func (h *repositoryWebhookTestHost) EgressPolicyForUser(model.User, context.Context) security.EgressPolicy {
	return security.AdminEgressPolicy()
}

func TestWriteRepositoryWebhookErrorKeepsLocalFailuresOutOfUpstreamCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{
			name:   "persistence",
			err:    fmt.Errorf("%w: database unavailable", errRepositoryWebhookPersistence),
			status: http.StatusInternalServerError,
			code:   "git.webhook_persistence_failed",
		},
		{
			name:   "compensation",
			err:    fmt.Errorf("%w: remote cleanup unavailable", errRepositoryWebhookCompensation),
			status: http.StatusBadGateway,
			code:   "git.webhook_compensation_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj/repository-bindings", nil)

			writeRepositoryWebhookError(ctx, tt.err)

			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.status)
			}
			if !strings.Contains(recorder.Body.String(), `"code":"`+tt.code+`"`) {
				t.Fatalf("response = %s, want code %s", recorder.Body.String(), tt.code)
			}
			if strings.Contains(recorder.Body.String(), "database unavailable") || strings.Contains(recorder.Body.String(), "remote cleanup unavailable") {
				t.Fatalf("response leaked internal failure: %s", recorder.Body.String())
			}
		})
	}
}

func TestWriteRepositoryWebhookErrorDoesNotOverwriteExistingClientResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj/repository-bindings", nil)

	writeRepositoryWebhookError(ctx, fmt.Errorf("%w: account unavailable", errGitClientResponseWritten))

	if recorder.Body.Len() != 0 {
		t.Fatalf("response body = %s, want existing response unchanged", recorder.Body.String())
	}
}

func TestConfigureRepositoryWebhookFailsBeforeRemoteMutationWithoutSecretStore(t *testing.T) {
	host := &repositoryWebhookTestHost{store: secret.Store{}, publicBaseURL: "https://luna.example.test"}
	handler := &Handler{host: host, secrets: host.store}
	ctx := repositoryWebhookTestContext()
	binding := model.RepositoryBinding{ID: "rpb_secret_failure", Owner: "snowykami", Repo: "neo-blog"}

	err := handler.configureRepositoryWebhook(ctx, model.User{ID: "usr_test"}, &binding, false)

	if !errors.Is(err, errRepositoryWebhookPersistence) {
		t.Fatalf("error = %v, want repository webhook persistence failure", err)
	}
	if binding.WebhookEnabled || binding.WebhookID != "" || binding.WebhookSecret != "" {
		t.Fatalf("binding mutated after secret failure: %+v", binding)
	}
	assertRepositoryWebhookAudit(t, host, false, "secret store unavailable")
}

func TestConfigureRepositoryWebhookCompensatesDatabaseFailure(t *testing.T) {
	for _, tt := range []struct {
		name             string
		deleteStatus     int
		wantError        error
		wantAuditMessage string
	}{
		{name: "cleanup succeeds", deleteStatus: http.StatusNoContent, wantError: errRepositoryWebhookPersistence, wantAuditMessage: "persist failed"},
		{name: "cleanup already absent", deleteStatus: http.StatusNotFound, wantError: errRepositoryWebhookPersistence, wantAuditMessage: "persist failed"},
		{name: "cleanup fails", deleteStatus: http.StatusInternalServerError, wantError: errRepositoryWebhookCompensation, wantAuditMessage: "persist and compensation failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			postCount := 0
			deleteCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.Method {
				case http.MethodPost:
					postCount++
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"id":42,"active":true,"config":{"url":"https://luna.example.test/api/v1/git/webhooks/rpb_test"}}`))
				case http.MethodDelete:
					deleteCount++
					w.WriteHeader(tt.deleteStatus)
				default:
					t.Fatalf("unexpected method %s", r.Method)
				}
			}))
			defer server.Close()

			handler, host, db, binding := newRepositoryWebhookIntegrationFixture(t, server.URL)
			forceRepositoryBindingSaveFailure(t, db)

			err := handler.configureRepositoryWebhook(repositoryWebhookTestContext(), model.User{ID: "usr_test"}, &binding, false)

			if !errors.Is(err, tt.wantError) {
				t.Fatalf("error = %v, want %v", err, tt.wantError)
			}
			if postCount != 1 || deleteCount != 1 {
				t.Fatalf("remote calls = POST %d DELETE %d, want one each", postCount, deleteCount)
			}
			var bindingCount int64
			if err := db.Model(&model.RepositoryBinding{}).Where("id = ?", binding.ID).Count(&bindingCount).Error; err != nil {
				t.Fatal(err)
			}
			if bindingCount != 0 {
				t.Fatalf("binding count = %d, failed create must not persist", bindingCount)
			}
			var secretCount int64
			if err := db.Model(&model.SecretValue{}).Count(&secretCount).Error; err != nil {
				t.Fatal(err)
			}
			if secretCount != 0 {
				t.Fatalf("secret count = %d, failed transaction must roll back", secretCount)
			}
			assertRepositoryWebhookAudit(t, host, false, tt.wantAuditMessage)
		})
	}
}

func TestConfigureRepositoryWebhookCommitsBindingSecretAndSuccessAudit(t *testing.T) {
	postCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		postCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":42,"active":true,"config":{"url":"https://luna.example.test/api/v1/git/webhooks/rpb_test"}}`))
	}))
	defer server.Close()

	handler, host, db, binding := newRepositoryWebhookIntegrationFixture(t, server.URL)
	if err := handler.configureRepositoryWebhook(repositoryWebhookTestContext(), model.User{ID: "usr_test"}, &binding, false); err != nil {
		t.Fatalf("configureRepositoryWebhook() error = %v", err)
	}
	if postCount != 1 {
		t.Fatalf("remote create count = %d, want 1", postCount)
	}
	if binding.WebhookID != "42" || !binding.WebhookEnabled || binding.WebhookSecret == "" {
		t.Fatalf("configured binding = %+v", binding)
	}
	var persisted model.RepositoryBinding
	if err := db.First(&persisted, "id = ?", binding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.WebhookID != binding.WebhookID || persisted.WebhookSecret != binding.WebhookSecret || !persisted.WebhookEnabled {
		t.Fatalf("persisted binding = %+v, in-memory binding = %+v", persisted, binding)
	}
	if resolved := handler.secrets.ResolveContext(t.Context(), persisted.WebhookSecret); resolved == "" {
		t.Fatal("persisted webhook secret cannot be resolved")
	}
	assertRepositoryWebhookAudit(t, host, true, "42")
}

func TestConfigureRepositoryWebhookUpdateFailurePreservesAuthoritativeBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":42,"active":true,"config":{"url":"https://luna.example.test/api/v1/git/webhooks/rpb_test"}}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	handler, host, db, binding := newRepositoryWebhookIntegrationFixture(t, server.URL)
	resource := "repository_binding:" + binding.ID + ":webhook"
	oldSecretRef, err := handler.secrets.StoreContextWithDB(t.Context(), db, "old-secret", "usr_test", resource)
	if err != nil {
		t.Fatal(err)
	}
	binding.WebhookID = "7"
	binding.WebhookSecret = oldSecretRef
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	candidate := binding
	candidate.Repo = "new-repository"
	candidate.WebhookID = ""
	candidate.WebhookSecret = ""
	forceRepositoryBindingSaveFailure(t, db)

	err = handler.configureRepositoryWebhook(repositoryWebhookTestContext(), model.User{ID: "usr_test"}, &candidate, false)
	if !errors.Is(err, errRepositoryWebhookPersistence) {
		t.Fatalf("error = %v, want repository webhook persistence failure", err)
	}
	var persisted model.RepositoryBinding
	if err := db.First(&persisted, "id = ?", binding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Repo != binding.Repo || persisted.WebhookID != binding.WebhookID || persisted.WebhookSecret != oldSecretRef {
		t.Fatalf("persisted binding changed after failed update: %+v", persisted)
	}
	if resolved := handler.secrets.ResolveContext(t.Context(), oldSecretRef); resolved != "old-secret" {
		t.Fatalf("old webhook secret = %q, want preserved", resolved)
	}
	assertRepositoryWebhookAudit(t, host, false, "persist failed")
}

func repositoryWebhookTestContext() *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_test/repository-bindings", nil)
	return ctx
}

func newRepositoryWebhookIntegrationFixture(t *testing.T, providerURL string) (*Handler, *repositoryWebhookTestHost, *gorm.DB, model.RepositoryBinding) {
	t.Helper()
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "git_webhook_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.GitProvider{}, &model.GitAccount{}, &model.RepositoryBinding{}, &model.SecretValue{})
		},
	})
	codec, err := secret.NewCodec("repository-webhook-test-key")
	if err != nil {
		t.Fatal(err)
	}
	store := secret.NewStore(db, nil, codec)
	host := &repositoryWebhookTestHost{db: db, store: store, publicBaseURL: "https://luna.example.test"}
	handler := &Handler{host: host, secrets: store}
	provider := model.GitProvider{ID: "gitp_test", Type: "github", Name: "GitHub", BaseURL: providerURL, Scope: "user", OwnerRef: "usr_test", Enabled: true}
	account := model.GitAccount{ID: "gita_test", UserID: "usr_test", ProviderID: provider.ID, Scope: "user", OwnerRef: "usr_test", Username: "snowykami", AccessTokenRef: codec.Encrypt("token")}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	binding := model.RepositoryBinding{
		ID:             "rpb_test",
		ProjectID:      "prj_test",
		ApplicationID:  "app_test",
		GitProviderID:  provider.ID,
		GitAccountID:   account.ID,
		Owner:          "snowykami",
		Repo:           "neo-blog",
		CloneURL:       "https://github.com/snowykami/neo-blog.git",
		DefaultBranch:  "main",
		WebhookEnabled: true,
	}
	return handler, host, db, binding
}

func forceRepositoryBindingSaveFailure(t *testing.T, db *gorm.DB) {
	t.Helper()
	fail := func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "RepositoryBinding" {
			tx.AddError(errors.New("forced repository binding save failure"))
		}
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail_repository_binding_create", fail); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail_repository_binding_update", fail); err != nil {
		t.Fatal(err)
	}
}

func assertRepositoryWebhookAudit(t *testing.T, host *repositoryWebhookTestHost, success bool, message string) {
	t.Helper()
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.audits) != 1 {
		t.Fatalf("audit count = %d, want 1", len(host.audits))
	}
	if host.audits[0].success != success || host.audits[0].message != message {
		t.Fatalf("audit = %+v, want success=%v message=%q", host.audits[0], success, message)
	}
}
