package gitapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

func TestGitProviderResponsePreservesOAuthReadyAndValidatesEndpoint(t *testing.T) {
	host := &gitAITestHost{}
	handler := New(host)

	providers := []model.GitProvider{
		{
			ID:              "gp_global_github",
			Type:            "github",
			Name:            "Global GitHub",
			BaseURL:         "https://github.com",
			Scope:           "global",
			AuthType:        "oauth",
			ClientID:        "oauth-client-id",
			ClientSecretRef: "secret-id:sec_ref_1",
			Enabled:         true,
		},
		{
			ID:              "gp_global_gitlab",
			Type:            "gitlab",
			Name:            "Global GitLab",
			BaseURL:         "https://gitlab.example.com",
			Scope:           "global",
			AuthType:        "oauth",
			ClientID:        "gitlab-client-id",
			ClientSecretRef: "secret-id:sec_ref_2",
			Enabled:         true,
		},
		{
			ID:              "gp_gitea_empty_base",
			Type:            "gitea",
			Name:            "Broken Gitea",
			BaseURL:         "",
			Scope:           "global",
			AuthType:        "oauth",
			ClientID:        "gitea-client-id",
			ClientSecretRef: "secret-id:sec_ref_3",
			Enabled:         true,
		},
	}

	responses, err := handler.gitProviderResponsesForUser(model.User{ID: "usr_normal", Role: authz.PlatformRoleUser}, providers, t.Context())
	if err != nil {
		t.Fatalf("gitProviderResponsesForUser() error = %v", err)
	}
	if len(responses) != 3 {
		t.Fatalf("len(responses) = %d, want 3", len(responses))
	}
	if responses[0]["clientId"] != "" || responses[0]["oauthReady"] != true {
		t.Fatalf("github global response = %#v, want redacted clientId and oauthReady=true", responses[0])
	}
	if responses[1]["oauthReady"] != false {
		t.Fatalf("gitlab oauthReady = %#v, want false (unsupported oauth provider)", responses[1]["oauthReady"])
	}
	if responses[2]["oauthReady"] != false {
		t.Fatalf("gitea empty baseUrl oauthReady = %#v, want false", responses[2]["oauthReady"])
	}
}

func TestRepositoryBindingFromInputEnforcesSameOriginCloneURLAndSafeArgs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	project := model.Project{ID: "prj_bind_sec", Identifier: "bind-sec", Name: "Bind Sec"}
	app := model.Application{ID: "app_bind_sec", ProjectID: project.ID, Identifier: "web", Name: "Web"}
	provider := model.GitProvider{
		ID: "gitp_bind_sec", Type: "github", Name: "GitHub", BaseURL: "https://github.com",
		Scope: "global", AuthType: "oauth", Enabled: true,
	}
	account := model.GitAccount{
		ID: "gita_bind_sec", ProviderID: provider.ID, Scope: "global", Username: "bot", AccessTokenRef: "secret-id:tok",
	}
	activeAccount := account
	if err := db.Callback().Query().After("gorm:query").Register("test:repo_bind_seed", func(tx *gorm.DB) {
		switch dest := tx.Statement.Dest.(type) {
		case *model.GitAccount:
			*dest = activeAccount
		case *model.GitProvider:
			*dest = provider
		case *model.Application:
			*dest = app
		case *int64:
			// Simulate ScopedResourceProjectBinding bound to prj_other, NOT project.ID.
			hasProjectFilter := false
			for _, v := range tx.Statement.Vars {
				if v == project.ID {
					hasProjectFilter = true
					break
				}
			}
			if hasProjectFilter {
				*dest = 0
			} else {
				*dest = 1
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	host := &gitAITestHost{db: db}
	handler := New(host)
	user := model.User{ID: "usr_dev", Role: authz.PlatformRoleUser}

	newCtx := func() (*gin.Context, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/repository-bindings", nil)
		ctx.Params = gin.Params{{Key: "projectId", Value: project.ID}}
		ctx.Set("luna.devops.current_user", user)
		return ctx, rec
	}

	ctx, rec := newCtx()
	if _, ok := handler.repositoryBindingFromInput(ctx, user, repositoryBindingInput{
		ApplicationID: app.ID,
		GitAccountID:  account.ID,
		Owner:         "org",
		Repo:          "repo",
		CloneURL:      "https://evil.example.com/org/repo.git",
	}); ok || rec.Code != http.StatusBadRequest {
		t.Fatalf("expected cross-origin cloneUrl to be rejected with 400, ok=%t status=%d body=%s", ok, rec.Code, rec.Body.String())
	}

	ctx, rec = newCtx()
	if _, ok := handler.repositoryBindingFromInput(ctx, user, repositoryBindingInput{
		ApplicationID: app.ID,
		GitAccountID:  account.ID,
		Owner:         "org",
		Repo:          "repo",
		DefaultBranch: "--upload-pack=evil",
	}); ok || rec.Code != http.StatusBadRequest {
		t.Fatalf("expected flag-injected defaultBranch to be rejected with 400, ok=%t status=%d", ok, rec.Code)
	}

	ctx, rec = newCtx()
	binding, ok := handler.repositoryBindingFromInput(ctx, user, repositoryBindingInput{
		ApplicationID: app.ID,
		GitAccountID:  account.ID,
		Owner:         "org",
		Repo:          "repo",
		DefaultBranch: "main",
	})
	if !ok || binding.CloneURL != "https://github.com/org/repo.git" {
		t.Fatalf("expected valid same-origin default cloneUrl to succeed, ok=%t binding=%+v body=%s", ok, binding, rec.Body.String())
	}

	// Cross-project project-scoped GitAccount bound only to prj_other must be rejected in project.ID.
	activeAccount = model.GitAccount{
		ID:             "gita_other_proj",
		UserID:         user.ID,
		Scope:          "project",
		ProviderID:     provider.ID,
		Username:       "other-bot",
		AccessTokenRef: "secret-id:tok",
	}

	ctx, rec = newCtx()
	if _, ok := handler.repositoryBindingFromInput(ctx, user, repositoryBindingInput{
		ApplicationID: app.ID,
		GitAccountID:  activeAccount.ID,
		Owner:         "org",
		Repo:          "repo",
		DefaultBranch: "main",
	}); ok || rec.Code != http.StatusForbidden {
		t.Fatalf("expected project-scoped GitAccount from another project to be rejected with 403, ok=%t status=%d body=%s", ok, rec.Code, rec.Body.String())
	}

	// Embedded userinfo / query / fragment in provider baseUrl must be rejected.
	for _, badBaseURL := range []string{
		"https://user:secret@gitea.example.com",
		"https://gitea.example.com?foo=bar",
		"https://gitea.example.com#frag",
		"",
	} {
		if _, err := validateProviderBaseURL("gitea", badBaseURL); err == nil {
			t.Fatalf("expected bad gitea baseUrl %q to be rejected", badBaseURL)
		}
	}
}
