package kubeaccess

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

type authenticationStore struct {
	token       model.AccessToken
	binding     model.KubeAccessBinding
	user        model.User
	project     model.Project
	cluster     model.RuntimeCluster
	application *model.Application
}

func (store authenticationStore) CreateCredential(context.Context, model.AccessToken, []model.KubeAccessBinding) error {
	return errors.New("not implemented")
}
func (store authenticationStore) ListCredentials(context.Context, string, PageOptions, time.Time) (Page[CredentialSummary], error) {
	return Page[CredentialSummary]{}, errors.New("not implemented")
}
func (store authenticationStore) ListBindings(context.Context, string, string, PageOptions) (Page[BindingSummary], error) {
	return Page[BindingSummary]{}, errors.New("not implemented")
}
func (store authenticationStore) RevokeCredential(context.Context, string, string, time.Time) error {
	return errors.New("not implemented")
}
func (store authenticationStore) FindTokenByHash(context.Context, string, time.Time) (model.AccessToken, error) {
	return store.token, nil
}
func (store authenticationStore) FindTokenByID(context.Context, string, time.Time) (model.AccessToken, error) {
	return store.token, nil
}
func (store authenticationStore) FindBinding(context.Context, string, string) (model.KubeAccessBinding, error) {
	return store.binding, nil
}
func (store authenticationStore) FindUser(context.Context, string) (model.User, error) {
	return store.user, nil
}
func (store authenticationStore) FindProject(context.Context, string) (model.Project, error) {
	return store.project, nil
}
func (store authenticationStore) FindRuntimeCluster(context.Context, string) (model.RuntimeCluster, error) {
	return store.cluster, nil
}
func (store authenticationStore) FindApplication(context.Context, string, string) (model.Application, error) {
	if store.application == nil {
		return model.Application{}, ErrContextInvalid
	}
	return *store.application, nil
}
func (store authenticationStore) RuntimeClusterBoundToProject(context.Context, string, string) (bool, error) {
	return true, nil
}

type readOnlyAuthorizer struct{}

func (readOnlyAuthorizer) AuthorizeProject(_ context.Context, _ authz.ProjectSubject, _ string, action authz.Action) (authz.ProjectAccess, error) {
	if action != authz.ActionProjectRead {
		return authz.ProjectAccess{}, authz.ErrProjectAccessDenied
	}
	return authz.ProjectAccess{Role: authz.ProjectRoleViewer}, nil
}

func TestAuthenticationUsesStoredScopesAsPerRequestCeiling(t *testing.T) {
	store := authenticationStore{
		token:   model.AccessToken{ID: "tok_one", UserID: "usr_one", Scope: "kube:read,kube:write", Source: model.AccessTokenSourceKubeconfig},
		binding: model.KubeAccessBinding{ID: "kbd_one", AccessTokenID: "tok_one", ProjectID: "prj_one", RuntimeClusterID: "clu_one"},
		user:    model.User{ID: "usr_one"},
		project: model.Project{ID: "prj_one", KubernetesNamespace: "project-one"},
		cluster: model.RuntimeCluster{ID: "clu_one", Scope: "global", KubeGatewayEnabled: true},
	}
	service := NewService(store, readOnlyAuthorizer{}, "https://devops.example.com", nil)

	authentication, err := service.authenticationFromToken(t.Context(), store.token, store.binding.ID)
	if err != nil {
		t.Fatalf("authenticationFromToken() error = %v", err)
	}
	if authentication.Access.Role != authz.ProjectRoleViewer {
		t.Fatalf("access role = %q, want viewer", authentication.Access.Role)
	}
}

func TestAuthenticationRejectsInvalidStoredScopes(t *testing.T) {
	store := authenticationStore{token: model.AccessToken{ID: "tok_one", Scope: "kube:admin"}}
	service := NewService(store, readOnlyAuthorizer{}, "https://devops.example.com", nil)

	_, err := service.authenticationFromToken(t.Context(), store.token, "kbd_one")
	if !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("authenticationFromToken() error = %v, want ErrCredentialInvalid", err)
	}
}
