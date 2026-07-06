package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

const rememberCookiePrefix = "lyd_remember_"
const sessionCookieName = "lyd_session"

type Handlers struct {
	db                  *gorm.DB
	configs             *configCache
	mode                string
	rateLimiter         *rateLimiter
	oauthStates         oauthStateStore
	projects            repository.ProjectRepository
	secrets             secret.Store
	branchCache         *gitBranchCache
	registrySearchCache *registrySearchCache
	gatewayTrafficState gatewayTrafficRuntimeStateStore
	taskClient          taskEnqueuer
}

type taskEnqueuer interface {
	EnqueueBuildRun(ctx context.Context, payload tasks.BuildRunPayload) (*asynq.TaskInfo, error)
	EnqueueDeployRun(ctx context.Context, payload tasks.DeployRunPayload) (*asynq.TaskInfo, error)
	EnqueueGatewayApply(ctx context.Context, payload tasks.GatewayApplyPayload) (*asynq.TaskInfo, error)
	EnqueueApplicationDelete(ctx context.Context, payload tasks.ApplicationDeletePayload) (*asynq.TaskInfo, error)
	EnqueueResourceCleanup(ctx context.Context, payload tasks.ResourceCleanupPayload) (*asynq.TaskInfo, error)
	EnqueueSystemComponentApply(ctx context.Context, payload tasks.SystemComponentApplyPayload) (*asynq.TaskInfo, error)
	EnqueueNotificationDeliver(ctx context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error)
}

func NewHandlers(db *gorm.DB) *Handlers {
	mode := config.RuntimeMode()
	if mode == "development" {
		ensureDevelopmentAdmin(db)
	}
	cfg := config.Load()
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: mode, rateLimiter: newRateLimiter(cfg.RedisAddr), oauthStates: newOAuthStateStore(cfg.RedisAddr), projects: repository.NewProjectRepository(db), branchCache: newGitBranchCache(), registrySearchCache: newRegistrySearchCache(), gatewayTrafficState: newGatewayTrafficRuntimeStateStore(cfg.RedisAddr)}
	if cfg.RedisAddr != "" {
		handlers.taskClient = tasks.NewClient(cfg.RedisAddr)
	}
	handlers.secrets = secret.NewStore(db, handlers.audit)
	return handlers
}

func (h *Handlers) gatewayTrafficRuntimeStore() gatewayTrafficRuntimeStateStore {
	if h.gatewayTrafficState == nil {
		h.gatewayTrafficState = newMemoryGatewayTrafficRuntimeStateStore()
	}
	return h.gatewayTrafficState
}
