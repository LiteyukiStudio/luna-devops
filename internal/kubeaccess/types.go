package kubeaccess

import (
	"context"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

const (
	CredentialStatusActive  = "active"
	CredentialStatusExpired = "expired"
	CredentialStatusRevoked = "revoked"
)

var (
	ErrInputInvalid          = errors.New("kube credential input is invalid")
	ErrScopeInvalid          = errors.New("kube credential scope is invalid")
	ErrContextInvalid        = errors.New("kube credential context is invalid")
	ErrCredentialNotFound    = errors.New("kube credential is not found")
	ErrCredentialInvalid     = errors.New("kube credential is invalid")
	ErrPermissionDenied      = errors.New("kube credential permission is denied")
	ErrGatewayDisabled       = errors.New("kube gateway is disabled")
	ErrGatewayReconciling    = errors.New("kube gateway is reconciling")
	ErrGatewayUnavailable    = errors.New("kube gateway is unavailable")
	ErrPublicBaseURLRequired = errors.New("public base url is required")
)

type ContextInput struct {
	ProjectID        string `json:"projectId"`
	RuntimeClusterID string `json:"runtimeClusterId"`
	ApplicationID    string `json:"applicationId,omitempty"`
}

type CreateInput struct {
	Name          string         `json:"name"`
	ExpiresInDays int            `json:"expiresInDays"`
	Scopes        []string       `json:"scopes"`
	Contexts      []ContextInput `json:"contexts"`
}

type CredentialSummary struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Scopes       []string   `json:"scopes"`
	Status       string     `json:"status"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	BindingCount int64      `json:"bindingCount"`
}

type BindingSummary struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"projectId"`
	RuntimeClusterID string    `json:"runtimeClusterId"`
	ApplicationID    string    `json:"applicationId,omitempty"`
	Namespace        string    `json:"namespace"`
	ContextName      string    `json:"contextName"`
	CreatedAt        time.Time `json:"createdAt"`
}

type CreateResult struct {
	Credential CredentialSummary `json:"credential"`
	Bindings   []BindingSummary  `json:"bindings"`
	Kubeconfig string            `json:"kubeconfig"`
}

type PageOptions struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
	Search    string
	Status    string
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	Page       int    `json:"page"`
	PageSize   int    `json:"pageSize"`
	SortBy     string `json:"sortBy"`
	SortOrder  string `json:"sortOrder"`
	Total      int64  `json:"total"`
	TotalPages int    `json:"totalPages"`
}

type GatewayReadiness interface {
	RequireReady(ctx context.Context, cluster model.RuntimeCluster, project model.Project) error
}

type Authentication struct {
	Token       model.AccessToken
	User        model.User
	Binding     model.KubeAccessBinding
	Project     model.Project
	Cluster     model.RuntimeCluster
	Application *model.Application
	Access      authz.ProjectAccess
}

func CredentialStatus(token model.AccessToken, now time.Time) string {
	if token.RevokedAt != nil {
		return CredentialStatusRevoked
	}
	if token.ExpiresAt != nil && !token.ExpiresAt.After(now) {
		return CredentialStatusExpired
	}
	return CredentialStatusActive
}
