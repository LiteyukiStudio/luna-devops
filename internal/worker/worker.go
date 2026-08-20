package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observability"
	dnsprovider "github.com/LiteyukiStudio/devops/internal/provider/dns"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/provider/volumestore"
	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type Runner struct {
	db                           *gorm.DB
	secrets                      secret.Store
	deployRolloutTimeoutSeconds  int64
	certManagerClusterIssuer     string
	publicBaseURL                string
	buildExecutorImage           string
	buildNPMRegistry             string
	buildEgressMode              string
	buildCacheEnabled            bool
	buildCacheTag                string
	buildJobTimeoutSeconds       int64
	buildJobTTLSeconds           int64
	buildPrivateEgressCIDRs      []string
	buildPrivateEgressPorts      []int
	buildBlockedEgressCIDRs      []string
	dnsResolver                  dnsprovider.Resolver
	taskClient                   *tasks.Client
	runAutomaticRetention        func(context.Context, time.Time) error
	namespaceFactory             func(kubeconfig string) (kubeprovider.NamespaceManager, error)
	kubernetesManagerFactory     func(environment model.Environment) (kubeprovider.NamespaceManager, error)
	projectVolumeProviderFactory func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error)
	volumeTransferJobFactory     func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error)
	volumeService                volumeWorkerService
	volumeTaskEnqueuer           volumeTaskEnqueuer
	volumeTransferStore          volumestore.Store
	volumeTransferCallbackURL    string
	volumeTransferJobImage       string
	volumeTransferMaxBytes       int64
	workerMetrics                *observability.WorkerMetrics
}

const (
	hookPhasePreDeployment  = "preDeployment"
	hookPhasePostDeployment = "postDeployment"
)

type Options struct {
	DeployRolloutTimeoutSeconds int64
	CertManagerClusterIssuer    string
	PublicBaseURL               string
	WorkerMetrics               *observability.WorkerMetrics
	BuildExecutorImage          string
	BuildNPMRegistry            string
	BuildEgressMode             string
	BuildCacheEnabled           bool
	BuildCacheTag               string
	BuildJobTimeoutSeconds      int64
	BuildJobTTLSeconds          int64
	BuildPrivateEgressCIDRs     []string
	BuildPrivateEgressPorts     []int
	BuildBlockedEgressCIDRs     []string
	VolumeTransferStore         volumestore.Store
	VolumeTransferCallbackURL   string
	VolumeTransferJobImage      string
	VolumeTransferMaxBytes      int64
}

func Run(redisAddr string, db *gorm.DB, options Options) error {
	return RunWithRedis(redisconfig.Options{Addr: redisAddr}, db, options)
}

func RunWithRedis(redisOptions redisconfig.Options, db *gorm.DB, options Options) error {
	runner := NewRunner(db, options)
	scheduler, err := startSchedulerWithRedis(redisOptions)
	if err != nil {
		return err
	}
	defer scheduler.Shutdown()

	server := asynq.NewServer(
		redisOptions.Asynq(),
		asynq.Config{
			Concurrency: 4,
			Logger:      asynqTelemetryLogger{},
			LogLevel:    asynq.WarnLevel,
			Queues: map[string]int{
				tasks.QueueDeploy: 3,
				tasks.QueueBuild:  2,
				tasks.QueueLight:  1,
			},
		},
	)

	mux := asynq.NewServeMux()
	mux.Use(taskTelemetryMiddleware)
	if options.WorkerMetrics != nil {
		mux.Use(options.WorkerMetrics.Middleware)
	}
	registerTaskHandlers(mux, runner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.taskClient = tasks.NewClientWithRedis(redisOptions)
	runner.volumeTaskEnqueuer = runner.taskClient
	defer runner.taskClient.Close()
	go runner.syncBuildJobStatus(ctx)

	return server.Run(mux)
}

func registerTaskHandlers(mux *asynq.ServeMux, runner *Runner) {
	mux.HandleFunc(tasks.TypeBuildRun, runner.withTaskEvents((*Runner).handleBuildRun))
	mux.HandleFunc(tasks.TypeDeployRun, runner.withTaskEvents((*Runner).handleDeployRun))
	mux.HandleFunc(tasks.TypeGatewayApply, runner.withTaskEvents((*Runner).handleGatewayApply))
	mux.HandleFunc(tasks.TypeApplicationDelete, runner.withTaskEvents((*Runner).handleApplicationDelete))
	mux.HandleFunc(tasks.TypeResourceCleanup, runner.withTaskEvents((*Runner).handleResourceCleanup))
	mux.HandleFunc(tasks.TypeNotificationDeliver, runner.withTaskEvents((*Runner).handleNotificationDeliver))
	mux.HandleFunc(tasks.TypeGitAccountRefresh, runner.withTaskEvents((*Runner).handleGitAccountRefresh))
	mux.HandleFunc(tasks.TypeSyncStatus, runner.withTaskEvents((*Runner).handleSyncStatus))
	mux.HandleFunc(tasks.TypeBillingAI, runner.withTaskEvents((*Runner).handleBillingAI))
	mux.HandleFunc(tasks.TypeBillingRuntime, runner.withTaskEvents((*Runner).handleBillingRuntime))
	mux.HandleFunc(tasks.TypeRetentionRun, runner.withTaskEvents((*Runner).handleRetentionRun))
	mux.HandleFunc(tasks.TypeVolumeProvision, runner.withTaskEvents((*Runner).handleVolumeProvision))
	mux.HandleFunc(tasks.TypeVolumeImport, runner.withTaskEvents((*Runner).handleVolumeImport))
	mux.HandleFunc(tasks.TypeVolumeExport, runner.withTaskEvents((*Runner).handleVolumeExport))
	mux.HandleFunc(tasks.TypeVolumeDelete, runner.withTaskEvents((*Runner).handleVolumeDelete))
	mux.HandleFunc(tasks.TypeVolumeReconcile, runner.withTaskEvents((*Runner).handleVolumeReconcile))
	mux.HandleFunc(tasks.TypeVolumeTransferCleanup, runner.withTaskEvents((*Runner).handleVolumeTransferCleanup))
}

func (r *Runner) withTaskEvents(handler func(*Runner, context.Context, *asynq.Task) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		runner := r.scoped(ctx)
		envelope := taskEnvelopeFromPayload(task.Type(), task.Payload())
		_ = runner.recordTaskEvent(envelope, "running", "")
		err := handler(runner, ctx, task)
		if err != nil {
			status := "failed"
			if errors.Is(err, errBuildCapacityUnavailable) {
				status = "waiting"
			}
			_ = runner.recordTaskEvent(envelope, status, err.Error())
			return err
		}
		_ = runner.recordTaskEvent(envelope, "succeeded", "")
		return nil
	}
}

func (r *Runner) scoped(ctx context.Context) *Runner {
	if r == nil || r.db == nil {
		return r
	}
	copy := *r
	copy.db = r.db.WithContext(ctx)
	copy.secrets = secret.NewStore(copy.db, nil)
	return &copy
}

func taskEnvelopeFromPayload(taskType string, payload []byte) tasks.TaskEnvelope {
	var raw struct {
		Envelope tasks.TaskEnvelope `json:"envelope"`
	}
	_ = json.Unmarshal(payload, &raw)
	envelope := raw.Envelope
	if strings.TrimSpace(envelope.TaskType) == "" {
		envelope.TaskType = taskType
	}
	if strings.TrimSpace(envelope.TaskID) == "" {
		envelope.TaskID = taskType
	}
	if strings.TrimSpace(envelope.DedupeKey) == "" {
		envelope.DedupeKey = envelope.TaskID
	}
	if strings.TrimSpace(envelope.TraceID) == "" {
		envelope.TraceID = envelope.TaskID
	}
	return envelope
}

func (r *Runner) recordTaskEvent(envelope tasks.TaskEnvelope, status string, message string) error {
	if r.db == nil {
		return nil
	}
	return r.db.Create(&model.WorkerTaskEvent{
		ID:          id.New("tke"),
		TaskID:      envelope.TaskID,
		TaskType:    envelope.TaskType,
		DedupeKey:   envelope.DedupeKey,
		ActorID:     envelope.ActorID,
		ResourceRef: envelope.ResourceRef,
		Status:      status,
		Message:     message,
		Attempt:     envelope.Attempt,
	}).Error
}

func NewRunner(db *gorm.DB, options Options) *Runner {
	deployRolloutTimeoutSeconds := options.DeployRolloutTimeoutSeconds
	if deployRolloutTimeoutSeconds <= 0 {
		deployRolloutTimeoutSeconds = 600
	}
	certManagerClusterIssuer := strings.TrimSpace(options.CertManagerClusterIssuer)
	if certManagerClusterIssuer == "" {
		certManagerClusterIssuer = "letsencrypt-http01"
	}
	buildExecutorImage := strings.TrimSpace(options.BuildExecutorImage)
	if buildExecutorImage == "" {
		buildExecutorImage = "moby/buildkit:v0.24.0-rootless"
	}
	buildCacheTag := strings.TrimSpace(options.BuildCacheTag)
	if buildCacheTag == "" {
		buildCacheTag = "buildcache"
	}
	buildEgressMode := strings.ToLower(strings.TrimSpace(options.BuildEgressMode))
	if buildEgressMode != "permissive" {
		buildEgressMode = "restricted"
	}
	buildJobTimeoutSeconds := options.BuildJobTimeoutSeconds
	if buildJobTimeoutSeconds <= 0 {
		buildJobTimeoutSeconds = 1800
	}
	buildJobTTLSeconds := options.BuildJobTTLSeconds
	if buildJobTTLSeconds <= 0 {
		buildJobTTLSeconds = 3600
	}
	volumeTransferMaxBytes := options.VolumeTransferMaxBytes
	if volumeTransferMaxBytes <= 0 {
		volumeTransferMaxBytes = 100 * 1024 * 1024 * 1024
	}
	return &Runner{
		db:                          db,
		secrets:                     secret.NewStore(db, nil),
		deployRolloutTimeoutSeconds: deployRolloutTimeoutSeconds,
		certManagerClusterIssuer:    certManagerClusterIssuer,
		publicBaseURL:               strings.TrimRight(strings.TrimSpace(options.PublicBaseURL), "/"),
		buildExecutorImage:          buildExecutorImage,
		buildNPMRegistry:            strings.TrimSpace(options.BuildNPMRegistry),
		buildEgressMode:             buildEgressMode,
		buildCacheEnabled:           options.BuildCacheEnabled,
		buildCacheTag:               buildCacheTag,
		buildJobTimeoutSeconds:      buildJobTimeoutSeconds,
		buildJobTTLSeconds:          buildJobTTLSeconds,
		buildPrivateEgressCIDRs:     append([]string(nil), options.BuildPrivateEgressCIDRs...),
		buildPrivateEgressPorts:     append([]int(nil), options.BuildPrivateEgressPorts...),
		buildBlockedEgressCIDRs:     append([]string(nil), options.BuildBlockedEgressCIDRs...),
		dnsResolver:                 dnsprovider.NewNetResolver(),
		workerMetrics:               options.WorkerMetrics,
		runAutomaticRetention:       newAutomaticRetentionRunner(db),
		volumeService:               volume.NewGormService(db),
		volumeTransferStore:         options.VolumeTransferStore,
		volumeTransferCallbackURL:   strings.TrimRight(strings.TrimSpace(options.VolumeTransferCallbackURL), "/"),
		volumeTransferJobImage:      strings.TrimSpace(options.VolumeTransferJobImage),
		volumeTransferMaxBytes:      volumeTransferMaxBytes,
		namespaceFactory: func(kubeconfig string) (kubeprovider.NamespaceManager, error) {
			return kubeprovider.NewClientFromKubeconfig(kubeconfig)
		},
	}
}
