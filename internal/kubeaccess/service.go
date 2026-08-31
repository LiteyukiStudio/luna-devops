package kubeaccess

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/credential"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
)

const maxCredentialContexts = 20

type Service struct {
	store         Store
	authorizer    authz.ProjectAuthorizer
	publicBaseURL string
	readiness     GatewayReadiness
	now           func() time.Time
}

func NewService(store Store, authorizer authz.ProjectAuthorizer, publicBaseURL string, readiness GatewayReadiness) *Service {
	return &Service{store: store, authorizer: authorizer, publicBaseURL: strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"), readiness: readiness, now: time.Now}
}

func NormalizeStoredScopes(scopeText string) ([]string, error) {
	return authz.NormalizeKubeScopes(strings.FieldsFunc(scopeText, func(r rune) bool { return r == ',' || r == ' ' }))
}

func (s *Service) Create(ctx context.Context, user model.User, input CreateInput) (CreateResult, error) {
	if s == nil || s.store == nil || s.authorizer == nil {
		return CreateResult{}, errors.New("kube access service is unavailable")
	}
	name := strings.TrimSpace(input.Name)
	if length := utf8.RuneCountInString(name); length < 1 || length > 64 {
		return CreateResult{}, ErrInputInvalid
	}
	days := input.ExpiresInDays
	if days == 0 {
		days = 7
	}
	if days != 1 && days != 7 && days != 30 {
		return CreateResult{}, ErrInputInvalid
	}
	scopes, err := authz.NormalizeKubeScopes(input.Scopes)
	if err != nil {
		return CreateResult{}, ErrScopeInvalid
	}
	if len(input.Contexts) < 1 || len(input.Contexts) > maxCredentialContexts {
		return CreateResult{}, ErrContextInvalid
	}

	now := s.now().UTC()
	plaintext, tokenHash := credential.Generate("luna_devops_kube_", 32)
	token := model.AccessToken{
		ID: id.New("tok"), UserID: user.ID, Name: name, Scope: strings.Join(scopes, ","),
		TokenHash: tokenHash, Source: model.AccessTokenSourceKubeconfig,
	}
	expiresAt := now.Add(time.Duration(days) * 24 * time.Hour)
	token.ExpiresAt = &expiresAt

	seen := make(map[string]bool, len(input.Contexts))
	bindings := make([]model.KubeAccessBinding, 0, len(input.Contexts))
	summaries := make([]BindingSummary, 0, len(input.Contexts))
	for _, requested := range input.Contexts {
		projectID := strings.TrimSpace(requested.ProjectID)
		clusterID := strings.TrimSpace(requested.RuntimeClusterID)
		applicationID := strings.TrimSpace(requested.ApplicationID)
		key := strings.Join([]string{projectID, clusterID, applicationID}, "\x00")
		if projectID == "" || clusterID == "" || seen[key] {
			return CreateResult{}, ErrContextInvalid
		}
		seen[key] = true
		project, cluster, err := s.validateContext(ctx, user, projectID, clusterID, applicationID, scopes)
		if err != nil {
			return CreateResult{}, err
		}
		var applicationRef *string
		if applicationID != "" {
			value := applicationID
			applicationRef = &value
		}
		binding := model.KubeAccessBinding{
			ID: id.New("kbd"), AccessTokenID: token.ID, ProjectID: projectID,
			RuntimeClusterID: clusterID, ApplicationID: applicationRef, CreatedAt: now,
		}
		bindings = append(bindings, binding)
		summaries = append(summaries, BindingSummary{
			ID: binding.ID, ProjectID: projectID, RuntimeClusterID: clusterID,
			ApplicationID: applicationID, Namespace: project.KubernetesNamespace,
			ContextName: ContextName(projectID, cluster.ID, applicationID), CreatedAt: now,
		})
	}
	kubeconfig, err := RenderKubeconfig(s.publicBaseURL, token.ID, plaintext, summaries)
	if err != nil {
		return CreateResult{}, err
	}
	if err := s.store.CreateCredential(ctx, token, bindings); err != nil {
		return CreateResult{}, err
	}
	return CreateResult{
		Credential: credentialSummary(token, int64(len(bindings)), now),
		Bindings:   summaries,
		Kubeconfig: kubeconfig,
	}, nil
}

func (s *Service) validateContext(ctx context.Context, user model.User, projectID, clusterID, applicationID string, scopes []string) (model.Project, model.RuntimeCluster, error) {
	project, err := s.store.FindProject(ctx, projectID)
	if err != nil || strings.TrimSpace(project.KubernetesNamespace) == "" {
		if err == nil {
			err = ErrContextInvalid
		}
		return model.Project{}, model.RuntimeCluster{}, err
	}
	subject := authz.ProjectSubject{UserID: user.ID, PlatformRole: user.Role}
	for _, action := range grantActions(scopes) {
		if _, err := s.authorizer.AuthorizeProject(ctx, subject, project.ID, action); err != nil {
			if errors.Is(err, authz.ErrProjectAccessDenied) {
				return model.Project{}, model.RuntimeCluster{}, ErrPermissionDenied
			}
			return model.Project{}, model.RuntimeCluster{}, err
		}
	}
	cluster, err := s.store.FindRuntimeCluster(ctx, clusterID)
	if err != nil {
		return model.Project{}, model.RuntimeCluster{}, err
	}
	allowed, err := s.clusterCanServeProject(ctx, user, cluster, project.ID)
	if err != nil {
		return model.Project{}, model.RuntimeCluster{}, err
	}
	if !allowed {
		return model.Project{}, model.RuntimeCluster{}, ErrPermissionDenied
	}
	if !cluster.KubeGatewayEnabled {
		return model.Project{}, model.RuntimeCluster{}, ErrGatewayDisabled
	}
	if s.readiness != nil {
		if err := s.readiness.RequireReady(ctx, cluster, project); err != nil {
			return model.Project{}, model.RuntimeCluster{}, err
		}
	}
	if applicationID != "" {
		if _, err := s.store.FindApplication(ctx, applicationID, project.ID); err != nil {
			return model.Project{}, model.RuntimeCluster{}, err
		}
	}
	return project, cluster, nil
}

func grantActions(scopes []string) []authz.Action {
	actions := []authz.Action{authz.ActionProjectRead}
	for _, scope := range scopes {
		switch scope {
		case authz.KubeScopeWrite:
			actions = append(actions, authz.ActionDeploymentUpdate)
		case authz.KubeScopeConnect:
			actions = append(actions, authz.ActionDeploymentExec)
		}
	}
	return actions
}

func (s *Service) clusterCanServeProject(ctx context.Context, user model.User, cluster model.RuntimeCluster, projectID string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(cluster.Scope)) {
	case "global", "":
		return true, nil
	case "user":
		return strings.TrimSpace(cluster.OwnerRef) == user.ID, nil
	case "project":
		return s.store.RuntimeClusterBoundToProject(ctx, cluster.ID, projectID)
	default:
		return false, nil
	}
}

func (s *Service) List(ctx context.Context, userID string, options PageOptions) (Page[CredentialSummary], error) {
	if s == nil || s.store == nil {
		return Page[CredentialSummary]{}, errors.New("kube access service is unavailable")
	}
	if options.Status != "" && options.Status != CredentialStatusActive && options.Status != CredentialStatusExpired && options.Status != CredentialStatusRevoked {
		return Page[CredentialSummary]{}, ErrInputInvalid
	}
	return s.store.ListCredentials(ctx, strings.TrimSpace(userID), options, s.now().UTC())
}

func (s *Service) ListBindings(ctx context.Context, userID, credentialID string, options PageOptions) (Page[BindingSummary], error) {
	if s == nil || s.store == nil {
		return Page[BindingSummary]{}, errors.New("kube access service is unavailable")
	}
	return s.store.ListBindings(ctx, strings.TrimSpace(userID), strings.TrimSpace(credentialID), options)
}

func (s *Service) Revoke(ctx context.Context, userID, credentialID string) error {
	if s == nil || s.store == nil {
		return errors.New("kube access service is unavailable")
	}
	return s.store.RevokeCredential(ctx, strings.TrimSpace(userID), strings.TrimSpace(credentialID), s.now().UTC())
}

func (s *Service) Authenticate(ctx context.Context, plaintext, bindingID string) (Authentication, error) {
	if s == nil || s.store == nil || s.authorizer == nil || strings.TrimSpace(plaintext) == "" || strings.TrimSpace(bindingID) == "" {
		return Authentication{}, ErrCredentialInvalid
	}
	now := s.now().UTC()
	token, err := s.store.FindTokenByHash(ctx, credential.Hash(strings.TrimSpace(plaintext)), now)
	if err != nil {
		return Authentication{}, err
	}
	return s.authenticationFromToken(ctx, token, bindingID)
}

func (s *Service) authenticationFromToken(ctx context.Context, token model.AccessToken, bindingID string) (Authentication, error) {
	if _, err := NormalizeStoredScopes(token.Scope); err != nil {
		return Authentication{}, ErrCredentialInvalid
	}
	binding, err := s.store.FindBinding(ctx, bindingID, token.ID)
	if err != nil {
		return Authentication{}, err
	}
	user, err := s.store.FindUser(ctx, token.UserID)
	if err != nil {
		return Authentication{}, err
	}
	// A credential's transport scopes are capability ceilings, not a role
	// snapshot. Re-authentication verifies current project membership here and
	// leaves each Kubernetes action to the request authorizer. This preserves
	// read access after a member is downgraded while still denying writes.
	project, cluster, err := s.validateContext(ctx, user, binding.ProjectID, binding.RuntimeClusterID, dereference(binding.ApplicationID), []string{authz.KubeScopeRead})
	if err != nil {
		if errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrContextInvalid) || errors.Is(err, ErrGatewayDisabled) {
			return Authentication{}, ErrCredentialInvalid
		}
		return Authentication{}, err
	}
	access, err := s.authorizer.AuthorizeProject(ctx, authz.ProjectSubject{UserID: user.ID, PlatformRole: user.Role}, project.ID, authz.ActionProjectRead)
	if err != nil {
		return Authentication{}, ErrCredentialInvalid
	}
	var application *model.Application
	if binding.ApplicationID != nil {
		value, err := s.store.FindApplication(ctx, *binding.ApplicationID, project.ID)
		if err != nil {
			return Authentication{}, ErrCredentialInvalid
		}
		application = &value
	}
	return Authentication{Token: token, User: user, Binding: binding, Project: project, Cluster: cluster, Application: application, Access: access}, nil
}

func (s *Service) Revalidate(ctx context.Context, authentication Authentication) (Authentication, error) {
	if authentication.Token.ID == "" || authentication.Binding.ID == "" {
		return Authentication{}, ErrCredentialInvalid
	}
	// Revalidation intentionally re-runs every authoritative lookup instead of
	// trusting the role, namespace or cluster snapshot in Authentication.
	if s == nil || s.store == nil || s.authorizer == nil {
		return Authentication{}, ErrCredentialInvalid
	}
	token, err := s.store.FindTokenByID(ctx, authentication.Token.ID, s.now().UTC())
	if err != nil {
		return Authentication{}, err
	}
	return s.authenticationFromToken(ctx, token, authentication.Binding.ID)
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
