package api

import (
	"context"
	"log/slog"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

const rememberCookiePrefix = "lyd_remember_"
const sessionCookieName = "lyd_session"
const currentProjectRoleContextKey = "currentProjectRole"

type Handlers struct {
	db                     *gorm.DB
	config                 Config
	configs                *configCache
	mode                   string
	rateLimiter            *rateLimiter
	oauthStates            oauthStateStore
	projects               repository.ProjectRepository
	secrets                secret.Store
	taskClient             taskEnqueuer
	aiAgent                aiagent.Client
	aiDeploymentEnabled    bool
	aiActorResolver        func(*gin.Context) (aiagent.ActorContext, string, bool)
	aiTools                *aitool.Service
	inbox                  inboxService
	inboxDecision          inboxDecisionHandler
	volumes                *volume.Service
	volumeClusters         projectVolumeClusterService
	volumeContent          volumeTransferContentService
	volumeTransferMaxBytes int64
	volumeTransferEnabled  bool
}

type inboxService interface {
	List(ctx context.Context, input inbox.ListInput) (inbox.ListResult, error)
	Get(ctx context.Context, userID, messageID string) (model.InboxMessage, error)
	GetActionRequest(ctx context.Context, userID, requestID string) (model.InboxActionRequest, error)
	GetActionRequests(ctx context.Context, userID string, requestIDs []string) (map[string]model.InboxActionRequest, error)
	UnreadCount(ctx context.Context, userID string) (int64, error)
	MarkRead(ctx context.Context, userID, messageID string) error
	MarkAllRead(ctx context.Context, userID string) error
	Archive(ctx context.Context, userID, messageID string) error
}

type inboxDecisionHandler func(
	ctx context.Context,
	user model.User,
	requestID string,
	decision string,
	expectedVersion int64,
) error

type taskEnqueuer interface {
	EnqueueBuildRun(ctx context.Context, payload tasks.BuildRunPayload) (*asynq.TaskInfo, error)
	EnqueueDeployRun(ctx context.Context, payload tasks.DeployRunPayload) (*asynq.TaskInfo, error)
	EnqueueGatewayApply(ctx context.Context, payload tasks.GatewayApplyPayload) (*asynq.TaskInfo, error)
	EnqueueApplicationDelete(ctx context.Context, payload tasks.ApplicationDeletePayload) (*asynq.TaskInfo, error)
	EnqueueResourceCleanup(ctx context.Context, payload tasks.ResourceCleanupPayload) (*asynq.TaskInfo, error)
	EnqueueNotificationDeliver(ctx context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error)
}

func NewHandlers(db *gorm.DB) *Handlers {
	return NewHandlersWithConfig(db, mustLoadConfig())
}

func NewHandlersWithConfig(db *gorm.DB, cfg Config) *Handlers {
	redisOptions := cfg.RedisOptions()
	handlers := &Handlers{
		db: db, config: cfg, configs: newConfigCache(db), mode: cfg.Mode, rateLimiter: newRateLimiterWithRedis(redisOptions),
		oauthStates: newOAuthStateStoreWithRedis(redisOptions), projects: repository.NewProjectRepository(db),
		volumeTransferMaxBytes: cfg.VolumeTransferMaxBytes, volumeTransferEnabled: cfg.VolumeTransferEnabled(),
	}
	if cfg.RedisAddr != "" {
		handlers.taskClient = tasks.NewClientWithRedis(redisOptions)
	}
	handlers.secrets = secret.NewStore(db, func(ctx context.Context, userID, action, resource string, success bool, message string) {
		handlers.auditWithContext(userID, action, resource, success, message, ctx)
	})
	var volumeTasks volumeTaskEnqueuer
	if candidate, ok := handlers.taskClient.(volumeTaskEnqueuer); ok {
		volumeTasks = candidate
	}
	handlers.volumeClusters = newProjectVolumeClusterAdapter(db, handlers.secrets)
	handlers.volumes = volume.NewGormService(db, volumeOperationDispatcher{tasks: volumeTasks}).
		WithExistingClaimInspector(handlers.volumeClusters)
	volumeContent, err := newVolumeTransferContentAdapter(handlers, cfg)
	if err != nil {
		telemetry.LogError(context.Background(), "Volume transfer content service initialization failed",
			"volume_transfer.content_service.initialization_failed", "volume_transfer.content_service.initialize",
			"provider.request.failed", err)
	} else {
		handlers.volumeContent = volumeContent
	}
	aiConfig := cfg.AIAgent
	handlers.aiDeploymentEnabled = aiConfig.Available
	var aiClientErr error
	handlers.aiAgent, aiClientErr = aiConfig.Client()
	if aiClientErr != nil {
		telemetry.LogError(context.Background(), "AI Agent client initialization failed",
			"ai.agent_client.initialization_failed", "ai.agent_client.initialize",
			"agent.startup.failed", aiClientErr,
			slog.Bool("ai.enabled", aiConfig.Available))
	}
	handlers.aiTools = aitool.NewService(
		db,
		aitool.WithWebPolicyProvider(handlers.aiWebEgressPolicyForUser),
		aitool.WithWebProxyProvider(handlers.aiWebProxyPoolForUser),
	)
	handlers.inbox = inbox.NewService(db)
	handlers.inboxDecision = handlers.decideInboxAction
	return handlers
}

// dbFor binds every database operation performed by an HTTP handler to the
// request context. The GORM OpenTelemetry plugin uses this context to attach
// SQL spans below the inbound HTTP span and to observe cancellation.
func (h *Handlers) dbFor(ctx *gin.Context) *gorm.DB {
	if h == nil || h.db == nil {
		return nil
	}
	if ctx == nil || ctx.Request == nil {
		panic("request database context is required")
	}
	return h.db.WithContext(ctx.Request.Context())
}

// dbWithContext is the non-Gin counterpart used by request-triggered helpers
// and services that already accept a standard library context.
func (h *Handlers) dbWithContext(ctx context.Context) *gorm.DB {
	if h == nil || h.db == nil {
		return nil
	}
	if ctx == nil {
		panic("api database context is required")
	}
	return h.db.WithContext(ctx)
}

func requestContext(ctx *gin.Context) context.Context {
	if ctx == nil || ctx.Request == nil {
		panic("request context is required")
	}
	return ctx.Request.Context()
}

func (h *Handlers) setAIAgentForTest(client aiagent.Client, deploymentEnabled bool) {
	h.aiAgent = client
	h.aiDeploymentEnabled = deploymentEnabled
}
