package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
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
	taskClient          taskEnqueuer
	aiAgent             aiagent.Client
	aiDeploymentEnabled bool
	aiActorResolver     func(*gin.Context) (aiagent.ActorContext, bool)
	aiTools             *aitool.Service
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
	redisOptions := cfg.RedisOptions()
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: mode, rateLimiter: newRateLimiterWithRedis(redisOptions), oauthStates: newOAuthStateStoreWithRedis(redisOptions), projects: repository.NewProjectRepository(db)}
	if cfg.RedisAddr != "" {
		handlers.taskClient = tasks.NewClientWithRedis(redisOptions)
	}
	handlers.secrets = secret.NewStore(db, handlers.audit)
	aiConfig := aiagent.LoadConfig()
	handlers.aiDeploymentEnabled = aiConfig.Available
	handlers.aiAgent = aiConfig.Client()
	handlers.aiTools = aitool.NewService(db)
	return handlers
}

func (h *Handlers) setAIAgentForTest(client aiagent.Client, deploymentEnabled bool) {
	h.aiAgent = client
	h.aiDeploymentEnabled = deploymentEnabled
}
