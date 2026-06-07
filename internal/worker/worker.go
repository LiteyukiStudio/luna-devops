package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	builderagent "github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	dnsprovider "github.com/LiteyukiStudio/devops/internal/provider/dns"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Runner struct {
	db                          *gorm.DB
	secrets                     secret.Store
	deployRolloutTimeoutSeconds int64
	certManagerClusterIssuer    string
	dnsResolver                 dnsprovider.Resolver
	namespaceFactory            func(kubeconfig string) (kubeprovider.NamespaceManager, error)
}

type Options struct {
	DeployRolloutTimeoutSeconds int64
	CertManagerClusterIssuer    string
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
				tasks.QueueLight:  1,
			},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeDeployRun, runner.withTaskEvents(runner.handleDeployRun))
	mux.HandleFunc(tasks.TypeGatewayApply, runner.withTaskEvents(runner.handleGatewayApply))
	mux.HandleFunc(tasks.TypeGitAccountRefresh, runner.withTaskEvents(runner.handleGitAccountRefresh))
	mux.HandleFunc(tasks.TypeSyncStatus, runner.withTaskEvents(logTask))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.consumeBuilderEvents(ctx, redisAddr)
	go runner.syncBuilderAgentStatus(ctx)

	return server.Run(mux)
}

func (r *Runner) withTaskEvents(handler func(context.Context, *asynq.Task) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		envelope := taskEnvelopeFromPayload(task.Type(), task.Payload())
		_ = r.recordTaskEvent(envelope, "running", "")
		err := handler(ctx, task)
		if err != nil {
			_ = r.recordTaskEvent(envelope, "failed", err.Error())
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

func (r *Runner) consumeBuilderEvents(ctx context.Context, redisAddr string) {
	client := builderagent.NewRedisClient(redisAddr)
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("builder event redis close failed: %v", err)
		}
	}()
	if err := ensureRedisGroup(ctx, client, builderagent.RedisEventStream, builderagent.RedisEventGroup); err != nil {
		log.Printf("builder event group init failed: %v", err)
		return
	}
	consumer := "worker-" + id.New("bev")
	for {
		streams, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    builderagent.RedisEventGroup,
			Consumer: consumer,
			Streams:  []string{builderagent.RedisEventStream, ">"},
			Count:    8,
			Block:    5 * time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("builder event read failed: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				if err := r.handleBuilderEvent(message); err != nil {
					log.Printf("builder event handle failed: id=%s err=%v", message.ID, err)
					continue
				}
				if err := client.XAck(ctx, builderagent.RedisEventStream, builderagent.RedisEventGroup, message.ID).Err(); err != nil {
					log.Printf("builder event ack failed: id=%s err=%v", message.ID, err)
				}
				if err := client.XDel(ctx, builderagent.RedisEventStream, message.ID).Err(); err != nil {
					log.Printf("builder event delete failed: id=%s err=%v", message.ID, err)
				}
			}
		}
	}
}

func ensureRedisGroup(ctx context.Context, client *redis.Client, stream string, group string) error {
	err := client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (r *Runner) handleBuilderEvent(message redis.XMessage) error {
	eventType, _ := message.Values["type"].(string)
	agentID, _ := message.Values["agentId"].(string)
	jobID, _ := message.Values["jobId"].(string)
	payload, _ := message.Values["payload"].(string)
	switch strings.TrimSpace(eventType) {
	case "heartbeat":
		var raw struct {
			Heartbeat builderagent.Heartbeat `json:"heartbeat"`
		}
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			return err
		}
		return r.recordBuilderHeartbeat(raw.Heartbeat)
	case "claimed":
		var raw struct {
			BuildRunID string `json:"buildRunId"`
			ProjectID  string `json:"projectId"`
		}
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			return err
		}
		return r.markBuilderJobClaimed(jobID, agentID, raw.BuildRunID, raw.ProjectID)
	case "log":
		var raw struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			return err
		}
		return r.appendBuilderLog(jobID, agentID, raw.Content)
	case "complete":
		var raw struct {
			Result builderagent.Result `json:"result"`
		}
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			return err
		}
		return r.completeBuilderJob(jobID, agentID, raw.Result)
	case "fail":
		var raw struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			return err
		}
		return r.failBuilderJob(jobID, agentID, raw.Message)
	default:
		return fmt.Errorf("unknown builder event type: %s", eventType)
	}
}

func (r *Runner) recordBuilderHeartbeat(heartbeat builderagent.Heartbeat) error {
	agentID := strings.TrimSpace(heartbeat.AgentID)
	if agentID == "" {
		return errors.New("builder agent id is required")
	}
	now := time.Now()
	agent := model.BuilderAgent{
		ID:                 agentID,
		Name:               fallbackString(strings.TrimSpace(heartbeat.Name), agentID),
		Labels:             strings.Join(normalizeWorkerStringList(heartbeat.Labels), ","),
		Scopes:             strings.Join(normalizeWorkerStringList(heartbeat.Scopes), ","),
		Executor:           fallbackString(strings.TrimSpace(heartbeat.Executor), "docker"),
		Status:             "online",
		MaxConcurrency:     fallbackWorkerInt(heartbeat.MaxConcurrency, 1),
		CurrentConcurrency: heartbeat.CurrentConcurrency,
		LastHeartbeatAt:    &now,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "labels", "scopes", "executor", "status", "max_concurrency", "current_concurrency", "last_heartbeat_at", "updated_at",
		}),
	}).Create(&agent).Error
}

func (r *Runner) syncBuilderAgentStatus(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := r.markStaleBuilderAgentsOffline(); err != nil {
			log.Printf("builder agent status sync failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) markStaleBuilderAgentsOffline() error {
	if r.db == nil {
		return nil
	}
	staleBefore := time.Now().Add(-90 * time.Second)
	return r.db.Model(&model.BuilderAgent{}).
		Where("status = ? and (last_heartbeat_at is null or last_heartbeat_at < ?)", "online", staleBefore).
		Updates(map[string]any{
			"status":              "offline",
			"current_concurrency": 0,
		}).Error
}

func (r *Runner) markBuilderJobClaimed(jobID string, agentID string, buildRunID string, projectID string) error {
	now := time.Now()
	leaseUntil := now.Add(5 * time.Minute)
	return r.db.Transaction(func(tx *gorm.DB) error {
		var job model.BuildJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "id = ?", jobID).Error; err != nil {
			return err
		}
		if job.Status != "queued" && job.BuilderID != agentID {
			return fmt.Errorf("build job %s is already claimed by %s", job.ID, job.BuilderID)
		}
		if projectID == "" {
			projectID = job.ProjectID
		}
		if buildRunID == "" {
			buildRunID = job.BuildRunID
		}
		if err := tx.Model(&model.BuildJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":      "running",
			"builder_id":  agentID,
			"lease_until": &leaseUntil,
			"log_ref":     "builder:" + agentID + "/" + job.ID,
			"message":     "builder task claimed",
			"attempts":    job.Attempts + 1,
			"started_at":  nullableWorkerStartTime(job.StartedAt, now),
			"finished_at": nil,
		}).Error; err != nil {
			return err
		}
		runUpdates := map[string]any{"status": "running"}
		var run model.BuildRun
		if err := tx.First(&run, "id = ? and project_id = ?", buildRunID, projectID).Error; err != nil {
			return err
		}
		if run.StartedAt == nil {
			runUpdates["started_at"] = &now
		}
		return tx.Model(&model.BuildRun{}).Where("id = ?", run.ID).Updates(runUpdates).Error
	})
}

func (r *Runner) appendBuilderLog(jobID string, agentID string, content string) error {
	content = trimWorkerBuildLogContent(content)
	if content == "" {
		return nil
	}
	var job model.BuildJob
	if err := r.db.First(&job, "id = ? and builder_id = ?", jobID, agentID).Error; err != nil {
		return err
	}
	var existing model.BuildLog
	err := r.db.First(&existing, "build_job_id = ?", job.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(&model.BuildLog{
			ID:         id.New("blog"),
			BuildRunID: job.BuildRunID,
			BuildJobID: job.ID,
			ProjectID:  job.ProjectID,
			Content:    content,
		}).Error
	}
	if err != nil {
		return err
	}
	existing.Content = trimWorkerBuildLogContent(existing.Content + "\n" + content)
	return r.db.Save(&existing).Error
}

func (r *Runner) completeBuilderJob(jobID string, agentID string, result builderagent.Result) error {
	var job model.BuildJob
	if err := r.db.First(&job, "id = ? and builder_id = ?", jobID, agentID).Error; err != nil {
		return err
	}
	var run model.BuildRun
	if err := r.db.First(&run, "id = ? and project_id = ?", job.BuildRunID, job.ProjectID).Error; err != nil {
		return err
	}
	finishedAt := time.Now()
	imageRef := fallbackString(strings.TrimSpace(result.ImageRef), run.ImageRef)
	imageDigest := strings.TrimSpace(result.ImageDigest)
	sourceCommit := fallbackString(strings.TrimSpace(result.SourceCommit), run.SourceCommit)
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.BuildJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":      "succeeded",
			"message":     fallbackString(strings.TrimSpace(result.Message), "builder task succeeded"),
			"lease_until": nil,
			"finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.BuildRun{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status":        "succeeded",
			"image_ref":     imageRef,
			"image_digest":  imageDigest,
			"source_commit": sourceCommit,
			"finished_at":   &finishedAt,
		}).Error; err != nil {
			return err
		}
		if imageRef != "" {
			image := containerImageFromWorkerBuildRun(run, imageRef, imageDigest, sourceCommit)
			if image.ID != "" {
				return tx.Create(&image).Error
			}
		}
		return nil
	})
}

func (r *Runner) failBuilderJob(jobID string, agentID string, message string) error {
	var job model.BuildJob
	if err := r.db.First(&job, "id = ? and builder_id = ?", jobID, agentID).Error; err != nil {
		return err
	}
	finishedAt := time.Now()
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.BuildJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":      "failed",
			"message":     fallbackString(strings.TrimSpace(message), "builder task failed"),
			"lease_until": nil,
			"finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.BuildRun{}).Where("id = ?", job.BuildRunID).Updates(map[string]any{
			"status":      "failed",
			"finished_at": &finishedAt,
		}).Error
	})
}

func nullableWorkerStartTime(existing *time.Time, now time.Time) any {
	if existing != nil {
		return existing
	}
	return &now
}

func fallbackString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func fallbackWorkerInt(value int, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}

func normalizeWorkerStringList(values []string) []string {
	seen := map[string]bool{}
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		output = append(output, value)
	}
	return output
}

func trimWorkerBuildLogContent(content string) string {
	const maxLogBytes = 2 * 1024 * 1024
	content = strings.TrimRight(content, "\n")
	if len(content) <= maxLogBytes {
		return content
	}
	return content[len(content)-maxLogBytes:]
}

func containerImageFromWorkerBuildRun(run model.BuildRun, imageRef string, digest string, sourceCommit string) model.ContainerImage {
	if strings.TrimSpace(run.TargetRegistryID) == "" || strings.TrimSpace(run.TargetRepository) == "" {
		return model.ContainerImage{}
	}
	return model.ContainerImage{
		ID:            id.New("img"),
		ProjectID:     run.ProjectID,
		ApplicationID: run.ApplicationID,
		RegistryID:    run.TargetRegistryID,
		Repository:    strings.Trim(strings.TrimSpace(run.TargetRepository), "/"),
		Tag:           fallbackString(strings.TrimSpace(run.TargetTag), "latest"),
		Digest:        strings.TrimSpace(digest),
		ImageRef:      strings.TrimSpace(imageRef),
		SourceType:    "build",
		BuildRunID:    run.ID,
		SourceCommit:  strings.TrimSpace(sourceCommit),
		ScanStatus:    "unknown",
		CreatedBy:     run.CreatedBy,
	}
}

type periodicTaskSpec struct {
	Cron    string
	Task    *asynq.Task
	Queue   string
	Timeout time.Duration
}

func periodicTaskSpecs() ([]periodicTaskSpec, error) {
	gitRefreshTask, err := tasks.NewGitAccountRefreshTask(tasks.GitAccountRefreshPayload{ActorID: "system"})
	if err != nil {
		return nil, err
	}
	return []periodicTaskSpec{
		{Cron: "@every 5m", Task: gitRefreshTask, Queue: tasks.QueueLight, Timeout: 10 * time.Minute},
		{Cron: "@every 1m", Task: asynq.NewTask(tasks.TypeSyncStatus, []byte("{}")), Queue: tasks.QueueLight, Timeout: 5 * time.Minute},
	}, nil
}

func startScheduler(redisAddr string) (*asynq.Scheduler, error) {
	scheduler := asynq.NewScheduler(asynq.RedisClientOpt{Addr: redisAddr}, &asynq.SchedulerOpts{})
	specs, err := periodicTaskSpecs()
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		if _, err := scheduler.Register(spec.Cron, spec.Task, asynq.Queue(spec.Queue), asynq.Timeout(spec.Timeout)); err != nil {
			return nil, err
		}
	}
	go func() {
		if err := scheduler.Run(); err != nil {
			log.Printf("run scheduler: %v", err)
		}
	}()
	return scheduler, nil
}

func (r *Runner) handleGitAccountRefresh(ctx context.Context, task *asynq.Task) error {
	var payload tasks.GitAccountRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	accounts, err := r.gitAccountsDueForRefresh(time.Now())
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if err := r.refreshGitAccount(ctx, account); err != nil {
			log.Printf("refresh git account %s: %v", account.ID, err)
		}
	}
	return nil
}

func (r *Runner) handleGatewayApply(ctx context.Context, task *asynq.Task) error {
	var payload tasks.GatewayApplyPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}

	var route model.GatewayRoute
	if err := r.db.First(&route, "id = ? and project_id = ?", payload.GatewayRouteID, payload.ProjectID).Error; err != nil {
		return err
	}
	var project model.Project
	if err := r.db.First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		return err
	}
	var application model.Application
	if err := r.db.First(&application, "id = ? and project_id = ?", route.ApplicationID, payload.ProjectID).Error; err != nil {
		return err
	}
	var environment model.Environment
	if err := r.db.First(&environment, "id = ? and project_id = ?", route.EnvironmentID, payload.ProjectID).Error; err != nil {
		return err
	}

	namespace := deploymentNamespace(project, environment)
	if err := r.ensureProjectNamespace(ctx, namespace, project); err != nil {
		_ = r.db.Model(&route).Updates(map[string]any{"status": "failed"}).Error
		return err
	}
	if err := r.applyGatewayIngress(ctx, route, project, application, environment, namespace); err != nil {
		_ = r.db.Model(&route).Updates(map[string]any{"status": "failed"}).Error
		return err
	}
	certificateStatus, err := r.applyGatewayCertificate(ctx, route, project, namespace)
	if err != nil {
		_ = r.db.Model(&route).Updates(map[string]any{"status": "failed", "certificate_status": "failed"}).Error
		return err
	}
	updates := map[string]any{"status": "active", "dns_status": r.gatewayDNSStatus(ctx, route)}
	if certificateStatus != "" {
		updates["certificate_status"] = certificateStatus
	}
	return r.db.Model(&route).Updates(updates).Error
}

func (r *Runner) handleDeployRun(ctx context.Context, task *asynq.Task) error {
	var payload tasks.DeployRunPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}

	var release model.Release
	if err := r.db.First(&release, "id = ? and project_id = ?", payload.ReleaseID, payload.ProjectID).Error; err != nil {
		return err
	}
	var project model.Project
	if err := r.db.First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		return err
	}
	var application model.Application
	if err := r.db.First(&application, "id = ? and project_id = ?", release.ApplicationID, payload.ProjectID).Error; err != nil {
		return err
	}
	var environment model.Environment
	if err := r.db.First(&environment, "id = ? and project_id = ?", release.EnvironmentID, payload.ProjectID).Error; err != nil {
		return err
	}

	now := time.Now()
	if release.StartedAt == nil {
		if err := r.db.Model(&release).Updates(map[string]any{"status": "running", "started_at": &now}).Error; err != nil {
			return err
		}
	}

	namespace := deploymentNamespace(project, environment)
	if err := r.ensureProjectNamespace(ctx, namespace, project); err != nil {
		finishedAt := time.Now()
		_ = r.db.Model(&release).Updates(map[string]any{"status": "failed", "message": err.Error(), "finished_at": &finishedAt}).Error
		return err
	}
	if err := r.applyApplicationResources(ctx, release, project, application, environment, namespace); err != nil {
		finishedAt := time.Now()
		_ = r.db.Model(&release).Updates(map[string]any{"status": "failed", "message": err.Error(), "finished_at": &finishedAt}).Error
		return err
	}
	if err := r.db.Model(&release).Updates(map[string]any{
		"status":  "running",
		"message": fmt.Sprintf("Deployment/Service/ConfigMap/Secret 已下发到命名空间 %s", namespace),
	}).Error; err != nil {
		return err
	}
	message, err := r.waitForDeploymentRollout(ctx, release, application, environment, namespace)
	if err != nil {
		finishedAt := time.Now()
		_ = r.db.Model(&release).Updates(releaseFinishUpdates("failed", err.Error(), finishedAt)).Error
		return err
	}
	return r.finishDeployRelease(release, "succeeded", firstNonEmpty(message, "Deployment rollout completed"))
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
	return &Runner{
		db:                          db,
		secrets:                     secret.NewStore(db, nil),
		deployRolloutTimeoutSeconds: deployRolloutTimeoutSeconds,
		certManagerClusterIssuer:    certManagerClusterIssuer,
		dnsResolver:                 dnsprovider.NewNetResolver(),
		namespaceFactory: func(kubeconfig string) (kubeprovider.NamespaceManager, error) {
			return kubeprovider.NewClientFromKubeconfig(kubeconfig)
		},
	}
}

func (r *Runner) gitAccountsDueForRefresh(now time.Time) ([]model.GitAccount, error) {
	var accounts []model.GitAccount
	err := r.db.Where("status = ? and refresh_token_ref <> '' and expires_at is not null and expires_at <= ?", "connected", now.Add(5*time.Minute)).
		Find(&accounts).Error
	return accounts, err
}

func gitAccountDueForWorkerRefresh(account model.GitAccount, now time.Time) bool {
	return account.Status == "connected" &&
		strings.TrimSpace(account.RefreshTokenRef) != "" &&
		account.ExpiresAt != nil &&
		!account.ExpiresAt.After(now.Add(5*time.Minute))
}

func (r *Runner) refreshGitAccount(ctx context.Context, account model.GitAccount) error {
	var provider model.GitProvider
	if err := r.db.First(&provider, "id = ? and enabled = ?", account.ProviderID, true).Error; err != nil {
		return err
	}
	refreshToken := r.secrets.Resolve(account.RefreshTokenRef)
	if strings.TrimSpace(refreshToken) == "" {
		return r.expireGitAccount(account, "git account has no refresh token")
	}
	oauthConfig, err := gitprovider.OAuthConfig(provider, "", r.secrets.Resolve(provider.ClientSecretRef))
	if err != nil {
		return r.expireGitAccount(account, "git OAuth provider configuration is invalid")
	}
	tokenSource := oauthConfig.TokenSource(ctx, &oauth2.Token{
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	})
	token, err := tokenSource.Token()
	if err != nil {
		return r.expireGitAccount(account, "git token refresh failed")
	}
	account.AccessTokenRef = r.secrets.Store(token.AccessToken, account.UserID, "git_account:"+account.ID+":access")
	if token.RefreshToken != "" {
		account.RefreshTokenRef = r.secrets.Store(token.RefreshToken, account.UserID, "git_account:"+account.ID+":refresh")
	}
	if !token.Expiry.IsZero() {
		account.ExpiresAt = &token.Expiry
	}
	account.Status = "connected"
	if err := r.db.Save(&account).Error; err != nil {
		return err
	}
	return r.auditGitAccountRefresh(account, true, account.Username)
}

func (r *Runner) expireGitAccount(account model.GitAccount, message string) error {
	account.Status = "expired"
	if err := r.db.Save(&account).Error; err != nil {
		return err
	}
	return r.auditGitAccountRefresh(account, false, message)
}

func (r *Runner) auditGitAccountRefresh(account model.GitAccount, success bool, message string) error {
	entry := model.AuditLog{
		ID:       id.New("aud"),
		UserID:   account.UserID,
		Action:   "git_account.refresh",
		Resource: account.ID,
		Success:  success,
		Message:  message,
	}
	return r.db.Create(&entry).Error
}

func (r *Runner) ensureProjectNamespace(ctx context.Context, namespace string, project model.Project) error {
	manager, err := r.kubernetesManager()
	if err != nil {
		return err
	}
	return manager.EnsureNamespace(ctx, namespace, map[string]string{
		"app.kubernetes.io/managed-by": "liteyuki-devops",
		"liteyuki.devops/scope":        "project",
		"liteyuki.devops/project-id":   project.ID,
	})
}

func (r *Runner) applyGatewayIngress(ctx context.Context, route model.GatewayRoute, project model.Project, application model.Application, environment model.Environment, namespace string) error {
	manager, err := r.kubernetesManager()
	if err != nil {
		return err
	}
	return manager.ApplyGatewayIngress(ctx, gatewayIngressSpec(route, project, application, environment, namespace))
}

func (r *Runner) gatewayDNSStatus(ctx context.Context, route model.GatewayRoute) string {
	if err := dnsprovider.CheckCNAME(ctx, r.dnsResolver, route.Host, route.CNAMETarget); err != nil {
		return "failed"
	}
	return "verified"
}

func (r *Runner) applyGatewayCertificate(ctx context.Context, route model.GatewayRoute, project model.Project, namespace string) (string, error) {
	if strings.TrimSpace(route.TLSMode) != "http-challenge" {
		return "", nil
	}
	manager, err := r.kubernetesManager()
	if err != nil {
		return "", err
	}
	spec := gatewayCertificateSpec(route, project, namespace, r.certManagerClusterIssuer)
	if err := manager.ApplyCertificate(ctx, spec); err != nil {
		return "", err
	}
	snapshot, err := manager.GetCertificateSnapshot(ctx, spec.Namespace, spec.Name)
	if err != nil {
		return "", err
	}
	return snapshot.Phase, nil
}

func (r *Runner) applyApplicationResources(ctx context.Context, release model.Release, project model.Project, application model.Application, environment model.Environment, namespace string) error {
	manager, err := r.kubernetesManager()
	if err != nil {
		return err
	}
	spec, err := applicationResourcesSpec(release, project, application, environment, namespace, r.deployRolloutTimeoutSeconds)
	if err != nil {
		return err
	}
	return manager.ApplyApplicationResources(ctx, spec)
}

func (r *Runner) waitForDeploymentRollout(ctx context.Context, release model.Release, application model.Application, environment model.Environment, namespace string) (string, error) {
	manager, err := r.kubernetesManager()
	if err != nil {
		return "", err
	}
	resourceName := applicationResourceName(application, environment)
	timeout := time.Duration(r.deployRolloutTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	rolloutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		snapshot, err := manager.GetDeploymentSnapshot(rolloutCtx, namespace, resourceName)
		if err != nil {
			return "", err
		}
		if snapshot.Message != "" {
			_ = r.db.Model(&model.Release{}).Where("id = ?", release.ID).Update("message", snapshot.Message).Error
		}

		switch snapshot.Phase {
		case kubeprovider.DeploymentSucceeded:
			return snapshot.Message, nil
		case kubeprovider.DeploymentFailed:
			return "", errors.New(firstNonEmpty(snapshot.Message, "Deployment rollout failed"))
		}

		select {
		case <-rolloutCtx.Done():
			return "", fmt.Errorf("Deployment rollout timed out after %s", timeout)
		case <-ticker.C:
		}
	}
}

func (r *Runner) finishDeployRelease(release model.Release, status string, message string) error {
	finishedAt := time.Now()
	return r.db.Model(&model.Release{}).Where("id = ?", release.ID).Updates(releaseFinishUpdates(status, message, finishedAt)).Error
}

func releaseFinishUpdates(status string, message string, finishedAt time.Time) map[string]any {
	return map[string]any{
		"status":      status,
		"message":     firstNonEmpty(message, "Deployment "+status),
		"finished_at": &finishedAt,
	}
}

func (r *Runner) kubernetesManager() (kubeprovider.NamespaceManager, error) {
	var cluster model.RuntimeCluster
	err := r.db.Where("scope = ? and is_default = ? and type in ?", "global", true, []string{"kubernetes", "k3s"}).First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = r.db.Where("scope = ? and type in ?", "global", []string{"kubernetes", "k3s"}).Order("created_at asc").First(&cluster).Error
	}
	if err != nil {
		return nil, fmt.Errorf("runtime cluster not found: %w", err)
	}

	kubeconfig := r.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		return nil, errors.New("runtime cluster kubeconfig is missing")
	}

	manager, err := r.namespaceFactory(kubeconfig)
	if err != nil {
		return nil, err
	}
	return manager, nil
}

func projectNamespace(project model.Project) string {
	return dnsLabel("project-" + project.Slug)
}

func deploymentNamespace(project model.Project, environment model.Environment) string {
	if namespace := strings.TrimSpace(environment.Namespace); namespace != "" {
		return dnsLabel(namespace)
	}
	return projectNamespace(project)
}

func applicationResourceName(application model.Application, environment model.Environment) string {
	return dnsLabel(application.Slug + "-" + environment.Slug)
}

func gatewayIngressName(route model.GatewayRoute) string {
	return buildResourceName(route.ID, "liteyuki-gateway-")
}

func gatewayTLSSecretName(route model.GatewayRoute) string {
	if strings.TrimSpace(route.TLSMode) == "http-only" {
		return ""
	}
	return dnsLabel("tls-" + route.Host)
}

func gatewayIngressSpec(route model.GatewayRoute, project model.Project, application model.Application, environment model.Environment, namespace string) kubeprovider.GatewayIngressSpec {
	servicePort := route.ServicePort
	if servicePort <= 0 {
		servicePort = application.ServicePort
	}
	if servicePort <= 0 {
		servicePort = 80
	}
	return kubeprovider.GatewayIngressSpec{
		Name:          gatewayIngressName(route),
		Namespace:     namespace,
		ProjectID:     project.ID,
		RouteID:       route.ID,
		Host:          strings.TrimSpace(route.Host),
		Path:          route.Path,
		ServiceName:   applicationResourceName(application, environment),
		ServicePort:   int32(servicePort),
		TLSSecretName: gatewayTLSSecretName(route),
	}
}

func gatewayCertificateSpec(route model.GatewayRoute, project model.Project, namespace string, clusterIssuer string) kubeprovider.CertificateSpec {
	return kubeprovider.CertificateSpec{
		Name:          gatewayIngressName(route),
		Namespace:     namespace,
		ProjectID:     project.ID,
		RouteID:       route.ID,
		Host:          strings.TrimSpace(route.Host),
		SecretName:    gatewayTLSSecretName(route),
		ClusterIssuer: strings.TrimSpace(clusterIssuer),
	}
}

func applicationResourcesSpec(release model.Release, project model.Project, application model.Application, environment model.Environment, namespace string, rolloutTimeoutSeconds int64) (kubeprovider.ApplicationResourcesSpec, error) {
	configData, err := mergeKeyValueMaps(environment.EnvVars, environment.ConfigRefs)
	if err != nil {
		return kubeprovider.ApplicationResourcesSpec{}, err
	}
	secretData, err := parseKeyValueMap(environment.SecretRefs)
	if err != nil {
		return kubeprovider.ApplicationResourcesSpec{}, err
	}
	servicePort := application.ServicePort
	if servicePort <= 0 {
		servicePort = 8080
	}
	replicas := environment.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	return kubeprovider.ApplicationResourcesSpec{
		Name:                  applicationResourceName(application, environment),
		Namespace:             namespace,
		ProjectID:             project.ID,
		ApplicationID:         application.ID,
		EnvironmentID:         environment.ID,
		Image:                 strings.TrimSpace(release.ImageRef),
		Replicas:              int32(replicas),
		ServicePort:           int32(servicePort),
		CPURequest:            strings.TrimSpace(environment.CPURequest),
		MemoryRequest:         strings.TrimSpace(environment.MemoryRequest),
		RolloutTimeoutSeconds: int32(rolloutTimeoutSeconds),
		ConfigData:            configData,
		SecretData:            secretData,
	}, nil
}

func mergeKeyValueMaps(values ...string) (map[string]string, error) {
	merged := map[string]string{}
	for _, value := range values {
		parsed, err := parseKeyValueMap(value)
		if err != nil {
			return nil, err
		}
		for key, item := range parsed {
			merged[key] = item
		}
	}
	return merged, nil
}

func parseKeyValueMap(value string) (map[string]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return map[string]string{}, nil
	}
	if strings.HasPrefix(value, "{") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(value), &raw); err != nil {
			return nil, err
		}
		parsed := make(map[string]string, len(raw))
		for key, item := range raw {
			parsed[strings.TrimSpace(key)] = fmt.Sprint(item)
		}
		return compactKeyValueMap(parsed), nil
	}
	parsed := map[string]string{}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, item, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid key-value line %q", line)
		}
		parsed[strings.TrimSpace(key)] = strings.TrimSpace(item)
	}
	return compactKeyValueMap(parsed), nil
}

func compactKeyValueMap(values map[string]string) map[string]string {
	compacted := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		compacted[key] = value
	}
	return compacted
}

func buildResourceName(buildRunID, prefix string) string {
	id := strings.ToLower(strings.TrimSpace(buildRunID))
	id = strings.TrimPrefix(id, "bldr_")
	var builder strings.Builder
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('-')
	}
	suffix := strings.Trim(builder.String(), "-")
	if suffix == "" {
		suffix = "run"
	}
	maxSuffix := 63 - len(prefix)
	if maxSuffix < 1 {
		maxSuffix = 1
	}
	if len(suffix) > maxSuffix {
		suffix = suffix[:maxSuffix]
	}
	return prefix + suffix
}

func dnsLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('-')
	}
	label := strings.Trim(builder.String(), "-")
	if label == "" {
		label = "liteyuki"
	}
	if len(label) > 63 {
		label = strings.TrimRight(label[:63], "-")
	}
	if label == "" {
		return "liteyuki"
	}
	return label
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func logTask(ctx context.Context, task *asynq.Task) error {
	log.Printf("received task type=%s payload=%s", task.Type(), string(task.Payload()))
	return nil
}
