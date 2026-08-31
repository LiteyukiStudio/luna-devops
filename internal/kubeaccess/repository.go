package kubeaccess

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"gorm.io/gorm"
)

type Store interface {
	CreateCredential(ctx context.Context, token model.AccessToken, bindings []model.KubeAccessBinding) error
	ListCredentials(ctx context.Context, userID string, options PageOptions, now time.Time) (Page[CredentialSummary], error)
	ListBindings(ctx context.Context, userID, credentialID string, options PageOptions) (Page[BindingSummary], error)
	RevokeCredential(ctx context.Context, userID, credentialID string, now time.Time) error
	FindTokenByHash(ctx context.Context, tokenHash string, now time.Time) (model.AccessToken, error)
	FindTokenByID(ctx context.Context, tokenID string, now time.Time) (model.AccessToken, error)
	FindBinding(ctx context.Context, bindingID, tokenID string) (model.KubeAccessBinding, error)
	FindUser(ctx context.Context, userID string) (model.User, error)
	FindProject(ctx context.Context, projectID string) (model.Project, error)
	FindRuntimeCluster(ctx context.Context, clusterID string) (model.RuntimeCluster, error)
	FindApplication(ctx context.Context, applicationID, projectID string) (model.Application, error)
	RuntimeClusterBoundToProject(ctx context.Context, clusterID, projectID string) (bool, error)
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) database(ctx context.Context) (*gorm.DB, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("kube access repository is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("kube access repository context is required")
	}
	return r.db.WithContext(ctx), nil
}

func (r *Repository) CreateCredential(ctx context.Context, token model.AccessToken, bindings []model.KubeAccessBinding) error {
	db, err := r.database(ctx)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&token).Error; err != nil {
			return err
		}
		if len(bindings) == 0 {
			return ErrContextInvalid
		}
		return tx.Create(&bindings).Error
	})
}

func (r *Repository) ListCredentials(ctx context.Context, userID string, options PageOptions, now time.Time) (Page[CredentialSummary], error) {
	db, err := r.database(ctx)
	if err != nil {
		return Page[CredentialSummary]{}, err
	}
	options = normalizedPageOptions(options, "createdAt")
	query := db.Model(&model.AccessToken{}).
		Where("user_id = ? and source = ?", strings.TrimSpace(userID), model.AccessTokenSourceKubeconfig)
	if options.Search != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(options.Search)
		pattern := "%" + escaped + "%"
		query = query.Where("(LOWER(name) LIKE LOWER(?) ESCAPE '\\' OR LOWER(scope) LIKE LOWER(?) ESCAPE '\\')", pattern, pattern)
	}
	switch options.Status {
	case CredentialStatusActive:
		query = query.Where("revoked_at is null and expires_at > ?", now)
	case CredentialStatusExpired:
		query = query.Where("revoked_at is null and expires_at <= ?", now)
	case CredentialStatusRevoked:
		query = query.Where("revoked_at is not null")
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return Page[CredentialSummary]{}, err
	}
	orders := map[string]string{
		"name":      "name",
		"createdAt": "created_at",
		"expiresAt": "expires_at",
		"status":    "CASE WHEN revoked_at IS NOT NULL THEN 2 WHEN expires_at <= CURRENT_TIMESTAMP THEN 1 ELSE 0 END",
	}
	if orders[options.SortBy] == "" {
		options.SortBy = "createdAt"
	}
	var tokens []model.AccessToken
	if err := query.Order(orders[options.SortBy] + " " + options.SortOrder).
		Limit(options.PageSize).Offset((options.Page - 1) * options.PageSize).Find(&tokens).Error; err != nil {
		return Page[CredentialSummary]{}, err
	}
	counts := make(map[string]int64, len(tokens))
	if len(tokens) > 0 {
		ids := make([]string, 0, len(tokens))
		for _, token := range tokens {
			ids = append(ids, token.ID)
		}
		var rows []struct {
			AccessTokenID string
			Count         int64
		}
		if err := db.Model(&model.KubeAccessBinding{}).
			Select("access_token_id, count(*) AS count").Where("access_token_id in ?", ids).
			Group("access_token_id").Scan(&rows).Error; err != nil {
			return Page[CredentialSummary]{}, err
		}
		for _, row := range rows {
			counts[row.AccessTokenID] = row.Count
		}
	}
	items := make([]CredentialSummary, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, credentialSummary(token, counts[token.ID], now))
	}
	return newPage(items, total, options), nil
}

func (r *Repository) ListBindings(ctx context.Context, userID, credentialID string, options PageOptions) (Page[BindingSummary], error) {
	db, err := r.database(ctx)
	if err != nil {
		return Page[BindingSummary]{}, err
	}
	var token model.AccessToken
	if err := db.First(&token, "id = ? and user_id = ? and source = ?", strings.TrimSpace(credentialID), strings.TrimSpace(userID), model.AccessTokenSourceKubeconfig).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Page[BindingSummary]{}, ErrCredentialNotFound
		}
		return Page[BindingSummary]{}, err
	}
	options = normalizedPageOptions(options, "createdAt")
	query := db.Model(&model.KubeAccessBinding{}).Where("access_token_id = ?", token.ID)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return Page[BindingSummary]{}, err
	}
	orders := map[string]string{"createdAt": "kube_access_bindings.created_at", "projectId": "project_id", "runtimeClusterId": "runtime_cluster_id"}
	if orders[options.SortBy] == "" {
		options.SortBy = "createdAt"
	}
	type bindingRow struct {
		ID               string
		ProjectID        string
		RuntimeClusterID string
		ApplicationID    *string
		Namespace        string
		CreatedAt        time.Time
	}
	var rows []bindingRow
	if err := query.Select("kube_access_bindings.id, kube_access_bindings.project_id, kube_access_bindings.runtime_cluster_id, kube_access_bindings.application_id, projects.kubernetes_namespace AS namespace, kube_access_bindings.created_at").
		Joins("LEFT JOIN projects ON projects.id = kube_access_bindings.project_id").
		Order(orders[options.SortBy] + " " + options.SortOrder).
		Limit(options.PageSize).Offset((options.Page - 1) * options.PageSize).Scan(&rows).Error; err != nil {
		return Page[BindingSummary]{}, err
	}
	items := make([]BindingSummary, 0, len(rows))
	for _, row := range rows {
		applicationID := ""
		if row.ApplicationID != nil {
			applicationID = *row.ApplicationID
		}
		items = append(items, BindingSummary{
			ID: row.ID, ProjectID: row.ProjectID, RuntimeClusterID: row.RuntimeClusterID,
			ApplicationID: applicationID, Namespace: row.Namespace,
			ContextName: ContextName(row.ProjectID, row.RuntimeClusterID, applicationID), CreatedAt: row.CreatedAt,
		})
	}
	return newPage(items, total, options), nil
}

func (r *Repository) RevokeCredential(ctx context.Context, userID, credentialID string, now time.Time) error {
	db, err := r.database(ctx)
	if err != nil {
		return err
	}
	result := db.Model(&model.AccessToken{}).
		Where("id = ? and user_id = ? and source = ?", strings.TrimSpace(credentialID), strings.TrimSpace(userID), model.AccessTokenSourceKubeconfig).
		Update("revoked_at", gorm.Expr("COALESCE(revoked_at, ?)", now))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCredentialNotFound
	}
	return nil
}

func (r *Repository) FindTokenByHash(ctx context.Context, tokenHash string, now time.Time) (model.AccessToken, error) {
	db, err := r.database(ctx)
	if err != nil {
		return model.AccessToken{}, err
	}
	var token model.AccessToken
	err = db.First(&token, "token_hash = ? and source = ? and revoked_at is null and expires_at > ?", tokenHash, model.AccessTokenSourceKubeconfig, now).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AccessToken{}, ErrCredentialInvalid
	}
	return token, err
}

func (r *Repository) FindTokenByID(ctx context.Context, tokenID string, now time.Time) (model.AccessToken, error) {
	db, err := r.database(ctx)
	if err != nil {
		return model.AccessToken{}, err
	}
	var token model.AccessToken
	err = db.First(&token, "id = ? and source = ? and revoked_at is null and expires_at > ?", strings.TrimSpace(tokenID), model.AccessTokenSourceKubeconfig, now).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AccessToken{}, ErrCredentialInvalid
	}
	return token, err
}

func (r *Repository) FindBinding(ctx context.Context, bindingID, tokenID string) (model.KubeAccessBinding, error) {
	db, err := r.database(ctx)
	if err != nil {
		return model.KubeAccessBinding{}, err
	}
	var binding model.KubeAccessBinding
	err = db.First(&binding, "id = ? and access_token_id = ?", strings.TrimSpace(bindingID), strings.TrimSpace(tokenID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.KubeAccessBinding{}, ErrCredentialInvalid
	}
	return binding, err
}

func (r *Repository) FindUser(ctx context.Context, userID string) (model.User, error) {
	db, err := r.database(ctx)
	if err != nil {
		return model.User{}, err
	}
	var user model.User
	err = db.First(&user, "id = ? and disabled = ?", strings.TrimSpace(userID), false).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, ErrCredentialInvalid
	}
	return user, err
}

func (r *Repository) FindProject(ctx context.Context, projectID string) (model.Project, error) {
	db, err := r.database(ctx)
	if err != nil {
		return model.Project{}, err
	}
	var project model.Project
	err = db.First(&project, "id = ? and delete_status = ?", strings.TrimSpace(projectID), "active").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Project{}, ErrContextInvalid
	}
	return project, err
}

func (r *Repository) FindRuntimeCluster(ctx context.Context, clusterID string) (model.RuntimeCluster, error) {
	db, err := r.database(ctx)
	if err != nil {
		return model.RuntimeCluster{}, err
	}
	var cluster model.RuntimeCluster
	err = runtimecluster.ActiveScope(db).First(&cluster, "id = ? and type in ?", strings.TrimSpace(clusterID), []string{"kubernetes", "k3s"}).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.RuntimeCluster{}, ErrContextInvalid
	}
	return cluster, err
}

func (r *Repository) FindApplication(ctx context.Context, applicationID, projectID string) (model.Application, error) {
	db, err := r.database(ctx)
	if err != nil {
		return model.Application{}, err
	}
	var application model.Application
	err = db.First(&application, "id = ? and project_id = ? and delete_status = ?", strings.TrimSpace(applicationID), strings.TrimSpace(projectID), "active").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Application{}, ErrContextInvalid
	}
	return application, err
}

func (r *Repository) RuntimeClusterBoundToProject(ctx context.Context, clusterID, projectID string) (bool, error) {
	db, err := r.database(ctx)
	if err != nil {
		return false, err
	}
	var count int64
	err = db.Model(&model.ScopedResourceProjectBinding{}).
		Where("resource_type = ? and resource_id = ? and project_id = ?", "runtime_cluster", strings.TrimSpace(clusterID), strings.TrimSpace(projectID)).
		Count(&count).Error
	return count > 0, err
}

func normalizedPageOptions(options PageOptions, defaultSort string) PageOptions {
	if options.Page < 1 {
		options.Page = 1
	}
	if options.PageSize < 1 {
		options.PageSize = 20
	}
	if options.PageSize > 100 {
		options.PageSize = 100
	}
	options.SortOrder = strings.ToLower(strings.TrimSpace(options.SortOrder))
	if options.SortOrder != "asc" {
		options.SortOrder = "desc"
	}
	options.Search = strings.TrimSpace(options.Search)
	options.Status = strings.ToLower(strings.TrimSpace(options.Status))
	credentialSorts := map[string]bool{"name": true, "createdAt": true, "expiresAt": true, "status": true}
	bindingSorts := map[string]bool{"createdAt": true, "projectId": true, "runtimeClusterId": true}
	if (!credentialSorts[options.SortBy] && !bindingSorts[options.SortBy]) || options.SortBy == "" {
		options.SortBy = defaultSort
	}
	return options
}

func newPage[T any](items []T, total int64, options PageOptions) Page[T] {
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(options.PageSize) - 1) / int64(options.PageSize))
	}
	return Page[T]{Items: items, Page: options.Page, PageSize: options.PageSize, SortBy: options.SortBy, SortOrder: options.SortOrder, Total: total, TotalPages: totalPages}
}

func credentialSummary(token model.AccessToken, bindingCount int64, now time.Time) CredentialSummary {
	scopes, _ := NormalizeStoredScopes(token.Scope)
	return CredentialSummary{
		ID: token.ID, Name: token.Name, Scopes: scopes, Status: CredentialStatus(token, now),
		ExpiresAt: token.ExpiresAt, CreatedAt: token.CreatedAt, BindingCount: bindingCount,
	}
}
