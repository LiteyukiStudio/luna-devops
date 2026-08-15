package api

import (
	"context"
	"net/http"
	"os"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/LiteyukiStudio/devops/internal/runtimecommand"
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
	platformRouter         http.Handler
	inbox                  inboxService
	inboxDecision          inboxDecisionHandler
	runtimeCommands        *runtimecommand.Broker
	volumes                *volume.Service
	volumeClusters         projectVolumeClusterService
	volumeContent          volumeTransferContentService
	volumeTransferMaxBytes int64
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
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: mode, rateLimiter: newRateLimiterWithRedis(redisOptions), oauthStates: newOAuthStateStoreWithRedis(redisOptions), projects: repository.NewProjectRepository(db), volumeTransferMaxBytes: cfg.VolumeTransferMaxBytes}
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
	if cfg.VolumeTransferEnabled() {
		volumeContent, err := newVolumeTransferContentAdapter(handlers, cfg)
		if err != nil {
			telemetry.Logger().Error("Volume transfer content service initialization failed",
				"event.name", "volume_transfer.content_service.initialization_failed",
				"error.type", telemetry.ErrorType(err))
		} else {
			handlers.volumeContent = volumeContent
		}
	}
	aiConfig := aiagent.LoadConfig()
	handlers.aiDeploymentEnabled = aiConfig.Available
	var aiClientErr error
	handlers.aiAgent, aiClientErr = aiConfig.Client()
	if aiClientErr != nil {
		telemetry.Logger().Error("AI Agent client initialization failed",
			"event.name", "ai.agent_client.initialization_failed",
			"error.type", telemetry.ErrorType(aiClientErr),
			"error.message", aiClientErr.Error(),
			"ai.enabled", aiConfig.Available)
	}
	handlers.aiTools = aitool.NewService(
		db,
		aitool.WithWebPolicyProvider(handlers.aiWebEgressPolicyForUser),
		aitool.WithWebProxyProvider(handlers.aiWebProxyPoolForUser),
	)
	handlers.inbox = inbox.NewService(db)
	handlers.inboxDecision = handlers.decideInboxAction
	handlers.runtimeCommands = runtimecommand.NewBroker(runtimecommand.Options{InstanceID: os.Getenv("HOSTNAME")})
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
