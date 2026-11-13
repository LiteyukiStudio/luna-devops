package gitapi

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
)

// Stable codes recorded on build.webhook_rejected platform events.
const (
	webhookRejectProjectDeleting          = "project.delete_in_progress"
	webhookRejectActorForbidden           = "build.webhook_actor_forbidden"
	webhookRejectAuthorizationUnavailable = "build.webhook_authorization_unavailable"
	webhookRejectBillingUnavailable       = "build.webhook_billing_unavailable"
	webhookRejectRequestInvalid           = "build.webhook_request_invalid"
	webhookRejectDispatchFailed           = "build.webhook_dispatch_failed"
	webhookRejectStateUnavailable         = "build.webhook_state_unavailable"

	webhookRejectedEventType = "build.webhook_rejected"
)

var gitWebhookDeliveryIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

type webhookBuildTriggerResult struct {
	Matched       int
	Queued        int
	Failed        int
	Skipped       int
	Duplicate     int
	QueuedRunIDs  []string
	IgnoredReason string
}

// gitWebhookDeliveryID returns the provider delivery identifier used to drop
// redelivered events. Values outside the expected shape are ignored.
func gitWebhookDeliveryID(header http.Header) string {
	for _, key := range []string{"X-GitHub-Delivery", "X-Gitea-Delivery", "X-Gogs-Delivery"} {
		value := strings.TrimSpace(header.Get(key))
		if value != "" && gitWebhookDeliveryIDPattern.MatchString(value) {
			return value
		}
	}
	return ""
}

func webhookBuildIdempotencyKey(bindingID, deliveryID, targetID string) string {
	return "webhook:" + bindingID + ":" + deliveryID + ":" + targetID
}

// enqueueBuildRunsForWebhook applies the same admission guards as a manual
// trigger, evaluated for the Git account owner who acts as the build creator.
// Rejections are recorded as platform events with stable codes.
func (h *Handlers) enqueueBuildRunsForWebhook(ctx *gin.Context, binding model.RepositoryBinding, payload gitWebhookPushPayload, isPush bool, deliveryID string) webhookBuildTriggerResult {
	result := webhookBuildTriggerResult{QueuedRunIDs: []string{}}
	if !isPush {
		result.IgnoredReason = "unsupported_event"
		return result
	}
	if payload.Deleted {
		result.IgnoredReason = "deleted_ref"
		return result
	}
	var account model.GitAccount
	if err := h.dbFor(ctx).First(&account, "id = ?", binding.GitAccountID).Error; err != nil {
		result.IgnoredReason = "git_account_missing"
		return result
	}
	var actor model.User
	if err := h.dbFor(ctx).First(&actor, "id = ?", account.UserID).Error; err != nil {
		result.IgnoredReason = "actor_missing"
		return result
	}
	var targets []model.DeploymentTarget
	if err := h.dbFor(ctx).Where(
		"project_id = ? and application_id = ? and repository_binding_id = ? and enabled = ? and delete_status in ?",
		binding.ProjectID,
		binding.ApplicationID,
		binding.ID,
		true,
		model.ActiveDeleteStatuses,
	).Where("(source_type = ? or source_type = '')", "repository").
		Order("created_at asc").
		Find(&targets).Error; err != nil {
		result.IgnoredReason = "deployment_targets_unavailable"
		return result
	}
	admissionChecked := false
	admissionCode := ""
	for _, target := range targets {
		run := webhookBuildRunFromTarget(binding, target, actor, payload, h.host.BuildRunActorName(actor))
		if !h.host.DeploymentTargetMatchesBuildRun(target, run) {
			result.Skipped++
			continue
		}
		result.Matched++
		if deliveryID != "" {
			run.IdempotencyKey = webhookBuildIdempotencyKey(binding.ID, deliveryID, target.ID)
			run.IdempotencyRequestHash = transportapi.DeliveryRequestHash(map[string]string{
				"bindingId": binding.ID,
				"targetId":  target.ID,
				"commit":    payload.CommitSHA,
			})
			duplicate, err := h.webhookBuildRunExists(ctx, actor.ID, run.IdempotencyKey)
			if err != nil {
				// 事件只记录稳定原因码，具体错误写入关联日志以便排查。
				telemetry.RecordError(ctx.Request.Context(), "git.webhook_build_state_unavailable", err)
				h.rejectWebhookBuild(ctx, &result, binding, target, actor, deliveryID, webhookRejectStateUnavailable)
				continue
			}
			if duplicate {
				result.Duplicate++
				continue
			}
		}
		if !admissionChecked {
			admissionCode = h.webhookBuildAdmissionCode(ctx, binding, actor)
			admissionChecked = true
		}
		if admissionCode != "" {
			h.rejectWebhookBuild(ctx, &result, binding, target, actor, deliveryID, admissionCode)
			continue
		}
		if err := h.prepareBuildRunRequest(actor, &run, ctx.Request.Context()); err != nil {
			h.rejectWebhookBuild(ctx, &result, binding, target, actor, deliveryID, webhookBuildRequestErrorCode(err))
			continue
		}
		queuedRun, err := h.queueBuildRun(ctx.Request.Context(), actor, run)
		if err != nil {
			if run.IdempotencyKey != "" {
				if duplicate, lookupErr := h.webhookBuildRunExists(ctx, actor.ID, run.IdempotencyKey); lookupErr == nil && duplicate {
					// A concurrent redelivery won the unique key; the run it
					// created is either queued or reclaimed by the Worker.
					result.Duplicate++
					continue
				}
			}
			h.rejectWebhookBuild(ctx, &result, binding, target, actor, deliveryID, webhookRejectDispatchFailed)
			continue
		}
		result.Queued++
		result.QueuedRunIDs = append(result.QueuedRunIDs, queuedRun.ID)
	}
	switch {
	case result.Matched == 0 && result.IgnoredReason == "":
		result.IgnoredReason = "no_matching_deployment_target"
	case result.Matched > 0 && result.Duplicate == result.Matched:
		result.IgnoredReason = "duplicate_delivery"
	}
	return result
}

// webhookBuildAdmissionCode re-evaluates, at delivery time, whether the Git
// account owner may still trigger builds in the project.
func (h *Handlers) webhookBuildAdmissionCode(ctx *gin.Context, binding model.RepositoryBinding, actor model.User) string {
	requestCtx := ctx.Request.Context()
	var project model.Project
	if err := h.dbFor(ctx).Select("id", "delete_status").First(&project, "id = ?", binding.ProjectID).Error; err != nil {
		telemetry.RecordError(requestCtx, "git.webhook_build_state_unavailable", err)
		return webhookRejectStateUnavailable
	}
	if !model.DeleteStatusActive(project.DeleteStatus) {
		return webhookRejectProjectDeleting
	}
	allowed, err := h.host.ProjectRoleActionAllowed(requestCtx, actor, binding.ProjectID, authz.ActionBuildTrigger)
	if err != nil {
		return webhookRejectAuthorizationUnavailable
	}
	if !allowed {
		return webhookRejectActorForbidden
	}
	code, err := h.host.NewBuildBillingDenialCode(requestCtx, binding.ProjectID)
	if err != nil {
		return webhookRejectBillingUnavailable
	}
	return code
}

func (h *Handlers) webhookBuildRunExists(ctx *gin.Context, actorID, idempotencyKey string) (bool, error) {
	var count int64
	err := h.dbFor(ctx).Model(&model.BuildRun{}).
		Where("created_by = ? and idempotency_key = ?", actorID, idempotencyKey).
		Count(&count).Error
	return count > 0, err
}

func webhookBuildRequestErrorCode(err error) string {
	var coded interface{ Code() string }
	if errors.As(err, &coded) && strings.TrimSpace(coded.Code()) != "" {
		return coded.Code()
	}
	return webhookRejectRequestInvalid
}

func (h *Handlers) rejectWebhookBuild(ctx *gin.Context, result *webhookBuildTriggerResult, binding model.RepositoryBinding, target model.DeploymentTarget, actor model.User, deliveryID string, code string) {
	result.Failed++
	dedupKey := ""
	if deliveryID != "" {
		dedupKey = webhookBuildIdempotencyKey(binding.ID, deliveryID, target.ID) + ":rejected"
	}
	requestCtx := ctx.Request.Context()
	db := h.dbFor(ctx)
	var project model.Project
	_ = db.Select("id", "name", "identifier").First(&project, "id = ?", binding.ProjectID).Error
	var application model.Application
	_ = db.Select("id", "name", "identifier").First(&application, "id = ?", binding.ApplicationID).Error
	links := map[string]string{}
	if baseURL := strings.TrimRight(strings.TrimSpace(h.host.PublicBaseURL()), "/"); baseURL != "" && binding.ProjectID != "" && binding.ApplicationID != "" {
		links["primary"] = baseURL + "/projects/" + binding.ProjectID + "/applications/" + binding.ApplicationID + "?tab=builds"
	}
	ownerUserID := strings.TrimSpace(target.CreatedBy)
	if ownerUserID == "" {
		ownerUserID = actor.ID
	}
	if _, err := (notification.Service{DB: db, Enqueuer: h.host.DeliveryEnqueuer()}).Emit(requestCtx, notification.Event{
		Type:     webhookRejectedEventType,
		Severity: notification.SeverityWarning,
		Project: notification.EntityRef{
			ID:         binding.ProjectID,
			Name:       project.Name,
			Identifier: project.Identifier,
		},
		Application: notification.EntityRef{
			ID:         binding.ApplicationID,
			Name:       application.Name,
			Identifier: application.Identifier,
		},
		DeploymentTarget: notification.EntityRef{
			ID:         target.ID,
			Name:       target.Name,
			Identifier: target.KubernetesName,
		},
		Build: notification.BuildContext{
			ID:      binding.ID,
			Status:  "webhook_rejected",
			Message: code,
		},
		Actor: notification.ActorContext{
			ID:    actor.ID,
			Name:  h.host.BuildRunActorName(actor),
			Email: actor.Email,
		},
		ResourceOwnerUserID: ownerUserID,
		CorrelationID:       binding.ID,
		DedupKey:            dedupKey,
		Links:               links,
		Message:             code,
	}); err != nil {
		telemetry.RecordError(requestCtx, "git.webhook_rejection_event_failed", err)
	}
}
