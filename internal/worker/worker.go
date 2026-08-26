package worker

import (
	"context"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observability"
	dnsprovider "github.com/LiteyukiStudio/devops/internal/provider/dns"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
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
	BuildEgressMode             string
	BuildCacheEnabled           bool
	BuildCacheTag               string
	BuildJobTimeoutSeconds      int64
	BuildJobTTLSeconds          int64
	BuildPrivateEgressCIDRs     []string
	BuildPrivateEgressPorts     []int
	BuildBlockedEgressCIDRs     []string
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
	mux.HandleFunc(tasks.TypeBuildRun, runner.withTaskContext((*Runner).handleBuildRun))
	mux.HandleFunc(tasks.TypeDeployRun, runner.withTaskContext((*Runner).handleDeployRun))
	mux.HandleFunc(tasks.TypeGatewayApply, runner.withTaskContext((*Runner).handleGatewayApply))
	mux.HandleFunc(tasks.TypeApplicationDelete, runner.withTaskContext((*Runner).handleApplicationDelete))
	mux.HandleFunc(tasks.TypeResourceCleanup, runner.withTaskContext((*Runner).handleResourceCleanup))
	mux.HandleFunc(tasks.TypeNotificationDeliver, runner.withTaskContext((*Runner).handleNotificationDeliver))
	mux.HandleFunc(tasks.TypeGitAccountRefresh, runner.withTaskContext((*Runner).handleGitAccountRefresh))
	mux.HandleFunc(tasks.TypeSyncStatus, runner.withTaskContext((*Runner).handleSyncStatus))
	mux.HandleFunc(tasks.TypeBillingAI, runner.withTaskContext((*Runner).handleBillingAI))
	mux.HandleFunc(tasks.TypeBillingRuntime, runner.withTaskContext((*Runner).handleBillingRuntime))
	mux.HandleFunc(tasks.TypeRetentionRun, runner.withTaskContext((*Runner).handleRetentionRun))
	mux.HandleFunc(tasks.TypeVolumeProvision, runner.withTaskContext((*Runner).handleVolumeProvision))
	mux.HandleFunc(tasks.TypeVolumeImport, runner.withTaskContext((*Runner).handleVolumeImport))
	mux.HandleFunc(tasks.TypeVolumeExport, runner.withTaskContext((*Runner).handleVolumeExport))
	mux.HandleFunc(tasks.TypeVolumeDelete, runner.withTaskContext((*Runner).handleVolumeDelete))
	mux.HandleFunc(tasks.TypeVolumeReconcile, runner.withTaskContext((*Runner).handleVolumeReconcile))
	mux.HandleFunc(tasks.TypeVolumeTransferCleanup, runner.withTaskContext((*Runner).handleVolumeTransferCleanup))
}

func (r *Runner) withTaskContext(handler func(*Runner, context.Context, *asynq.Task) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		return handler(r.scoped(ctx), ctx, task)
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
		volumeTransferJobImage:      strings.TrimSpace(options.VolumeTransferJobImage),
		volumeTransferMaxBytes:      volumeTransferMaxBytes,
		namespaceFactory: func(kubeconfig string) (kubeprovider.NamespaceManager, error) {
			return kubeprovider.NewClientFromKubeconfig(kubeconfig)
		},
	}
}
