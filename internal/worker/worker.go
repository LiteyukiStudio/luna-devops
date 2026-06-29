package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	dnsprovider "github.com/LiteyukiStudio/devops/internal/provider/dns"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type Runner struct {
	db                          *gorm.DB
	secrets                     secret.Store
	deployRolloutTimeoutSeconds int64
	certManagerClusterIssuer    string
	buildExecutorImage          string
	buildNPMRegistry            string
	buildEgressMode             string
	buildCacheEnabled           bool
	buildCacheTag               string
	buildJobTimeoutSeconds      int64
	buildJobTTLSeconds          int64
	buildPrivateEgressCIDRs     []string
	buildPrivateEgressPorts     []int
	buildBlockedEgressCIDRs     []string
	dnsResolver                 dnsprovider.Resolver
	taskClient                  *tasks.Client
	namespaceFactory            func(kubeconfig string) (kubeprovider.NamespaceManager, error)
	kubernetesManagerFactory    func(environment model.Environment) (kubeprovider.NamespaceManager, error)
}

const (
	hookPhasePreDeployment  = "preDeployment"
	hookPhasePostDeployment = "postDeployment"
)

type Options struct {
	DeployRolloutTimeoutSeconds int64
	CertManagerClusterIssuer    string
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
}

func Run(redisAddr string, db *gorm.DB, options Options) error {
	runner := NewRunner(db, options)
	scheduler, err := startScheduler(redisAddr)
	if err != nil {
		return err
	}
	defer scheduler.Shutdown()

	server := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 4,
			Queues: map[string]int{
				tasks.QueueDeploy: 3,
				tasks.QueueBuild:  2,
				tasks.QueueLight:  1,
			},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeBuildRun, runner.withTaskEvents(runner.handleBuildRun))
	mux.HandleFunc(tasks.TypeDeployRun, runner.withTaskEvents(runner.handleDeployRun))
	mux.HandleFunc(tasks.TypeGatewayApply, runner.withTaskEvents(runner.handleGatewayApply))
	mux.HandleFunc(tasks.TypeApplicationDelete, runner.withTaskEvents(runner.handleApplicationDelete))
	mux.HandleFunc(tasks.TypeResourceCleanup, runner.withTaskEvents(runner.handleResourceCleanup))
	mux.HandleFunc(tasks.TypeGitAccountRefresh, runner.withTaskEvents(runner.handleGitAccountRefresh))
	mux.HandleFunc(tasks.TypeSyncStatus, runner.withTaskEvents(runner.handleSyncStatus))
	mux.HandleFunc(tasks.TypeBillingRuntime, runner.withTaskEvents(runner.handleBillingRuntime))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.taskClient = tasks.NewClient(redisAddr)
	defer runner.taskClient.Close()
	go runner.syncBuildJobStatus(ctx)

	return server.Run(mux)
}

func (r *Runner) withTaskEvents(handler func(context.Context, *asynq.Task) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		envelope := taskEnvelopeFromPayload(task.Type(), task.Payload())
		_ = r.recordTaskEvent(envelope, "running", "")
		err := handler(ctx, task)
		if err != nil {
			status := "failed"
			if errors.Is(err, errBuildCapacityUnavailable) {
				status = "waiting"
			}
			_ = r.recordTaskEvent(envelope, status, err.Error())
			return err
		}
		_ = r.recordTaskEvent(envelope, "succeeded", "")
		return nil
	}
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
	if buildEgressMode != "restricted" {
		buildEgressMode = "permissive"
	}
	buildJobTimeoutSeconds := options.BuildJobTimeoutSeconds
	if buildJobTimeoutSeconds <= 0 {
		buildJobTimeoutSeconds = 1800
	}
	buildJobTTLSeconds := options.BuildJobTTLSeconds
	if buildJobTTLSeconds <= 0 {
		buildJobTTLSeconds = 3600
	}
	return &Runner{
		db:                          db,
		secrets:                     secret.NewStore(db, nil),
		deployRolloutTimeoutSeconds: deployRolloutTimeoutSeconds,
		certManagerClusterIssuer:    certManagerClusterIssuer,
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
		namespaceFactory: func(kubeconfig string) (kubeprovider.NamespaceManager, error) {
			return kubeprovider.NewClientFromKubeconfig(kubeconfig)
		},
	}
}
