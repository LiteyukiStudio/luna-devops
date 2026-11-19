package deploymentapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func releaseAsyncVerificationRule() []aitool.OperationVerification {
	return []aitool.OperationVerification{{
		OperationID:     "delivery_get_release",
		Family:          "release",
		IDFrom:          "result.releaseId",
		ReadIDArgument:  "releaseId",
		StatusField:     "status",
		PendingStatuses: []string{"pending", "running"},
		SuccessStatuses: []string{"succeeded"},
		FailureStatuses: []string{"failed", "canceled"},
	}}
}

func releaseCancelVerificationRule() []aitool.OperationVerification {
	return []aitool.OperationVerification{{
		OperationID:    "delivery_get_release",
		Family:         "release",
		IDFrom:         "result.releaseId",
		ReadIDArgument: "releaseId",
	}}
}

// --- Tool 9: delivery_scale_target ---

type deliveryScaleTargetInput struct {
	Project     string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application string `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target      string `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
	Replicas    *int   `json:"replicas" required:"true" min:"0" desc:"目标副本数（0 表示停机缩容至零释放算力）"`
}

type deliveryScaleTargetOutput struct {
	ID                 string `json:"id"`
	DeploymentTargetID string `json:"deploymentTargetId"`
	ProjectID          string `json:"projectId"`
	ApplicationID      string `json:"applicationId"`
	Stage              string `json:"stage"`
	PreviousReplicas   int    `json:"previousReplicas"`
	Replicas           int    `json:"replicas"`
	AppliedLive        bool   `json:"appliedLive"`
	RequiresRelease    bool   `json:"requiresRelease"`
}

// --- Tool 10: delivery_deploy_target ---

type deliveryDeployTargetInput struct {
	Project        string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application    string `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target         string `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
	ImageRef       string `json:"imageRef,omitempty" desc:"可选单次发布覆盖镜像地址（不改写 target 基准 imageRef）"`
	BuildRunID     string `json:"buildRunId,omitempty" desc:"可选指定同一部署目标下 succeeded 状态的构建记录 ID"`
	ForceImagePull *bool  `json:"forceImagePull,omitempty" default:"false" desc:"同 Tag 重推镜像后是否强制 K8s 重新拉取镜像"`
	Message        string `json:"message,omitempty" desc:"可选发布说明备注"`
}

type aiDeployPreviewLock struct {
	TargetID           string `json:"targetId"`
	ResolvedImageRef   string `json:"resolvedImageRef"`
	ResolvedBuildRunID string `json:"resolvedBuildRunId,omitempty"`
}

type deliveryReleaseMutationOutput struct {
	ID                 string    `json:"id"`
	ReleaseID          string    `json:"releaseId"`
	ProjectID          string    `json:"projectId"`
	ApplicationID      string    `json:"applicationId"`
	DeploymentTargetID string    `json:"deploymentTargetId"`
	Stage              string    `json:"stage,omitempty"`
	Revision           int       `json:"revision"`
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	ImageRef           string    `json:"imageRef"`
	BuildRunID         string    `json:"buildRunId,omitempty"`
	RollbackFromID     string    `json:"rollbackFromId,omitempty"`
	ForceImagePull     bool      `json:"forceImagePull"`
	Message            string    `json:"message,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
}

// --- Tool 11: delivery_rollback_target ---

type deliveryRollbackTargetInput struct {
	Project     string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application string `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target      string `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
	ToReleaseID string `json:"toReleaseId,omitempty" desc:"可选指定回滚到的同一目标历史 succeeded 发布记录 ID（省略按规则自动选取上一成功版本）"`
	Message     string `json:"message,omitempty" desc:"可选回滚原因备注"`
}

type aiRollbackPreviewLock struct {
	TargetID            string `json:"targetId"`
	ResolvedToReleaseID string `json:"resolvedToReleaseId"`
	ResolvedImageRef    string `json:"resolvedImageRef"`
	ResolvedBuildRunID  string `json:"resolvedBuildRunId,omitempty"`
	TargetRevision      int    `json:"targetRevision"`
}

// --- Tool 12: delivery_restart_target ---

type deliveryRestartTargetInput struct {
	Project     string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application string `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target      string `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
}

type deliveryRestartTargetOutput struct {
	ID                 string    `json:"id"`
	DeploymentTargetID string    `json:"deploymentTargetId"`
	ProjectID          string    `json:"projectId"`
	ApplicationID      string    `json:"applicationId"`
	Stage              string    `json:"stage"`
	KubernetesName     string    `json:"kubernetesName"`
	Restarted          bool      `json:"restarted"`
	RestartedAt        time.Time `json:"restartedAt"`
}

// --- Tool 13: delivery_cancel_release ---

type deliveryCancelReleaseInput struct {
	Project   string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	ReleaseID string `json:"releaseId" required:"true" desc:"待取消的发布记录 ID（rel_xxx）"`
}

type deliveryCancelReleaseOutput struct {
	ID                 string     `json:"id"`
	ReleaseID          string     `json:"releaseId"`
	ProjectID          string     `json:"projectId"`
	ApplicationID      string     `json:"applicationId"`
	DeploymentTargetID string     `json:"deploymentTargetId"`
	Status             string     `json:"status"`
	FailureCode        string     `json:"failureCode,omitempty"`
	FinishedAt         *time.Time `json:"finishedAt,omitempty"`
	Note               string     `json:"note"`
}

func (h *Handler) registerDeliveryReleaseExecAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[deliveryScaleTargetInput, deliveryScaleTargetOutput]{
		Name:           "delivery_scale_target",
		Domain:         aitool.DomainDelivery,
		Title:          "扩缩部署目标副本数",
		Summary:        "同步扩缩部署目标期望与线上副本数（0 表示停机缩容至零），自动拦截数据库或集群残留 HPA 冲突。",
		AliasesZH:      []string{"扩容", "缩容", "停机缩容到0", "修改副本数"},
		AliasesEN:      []string{"scale target", "scale replicas", "stop workload"},
		Purpose:        aitool.Text("调整部署目标的副本数量（传 0 可停机释放算力），对已部署工作负载同步即时生效。", "Scale the replica count of a deployment target (0 stops running Pods), applying immediately when deployed."),
		ApprovalTier:   aitool.ApprovalTierL2,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentUpdate)},
		Effect:         aitool.EffectWrite,
		Verification:   targetVerificationRule(),
		Preview:        h.previewDeliveryScaleTarget,
		Execute:        h.executeDeliveryScaleTarget,
	})

	aitool.Register(reg, aitool.Spec[deliveryDeployTargetInput, deliveryReleaseMutationOutput]{
		Name:           "delivery_deploy_target",
		Domain:         aitool.DomainDelivery,
		Title:          "发布部署目标至集群",
		Summary:        "异步创建发布任务（Release）将期望配置与锁定镜像下发至 Kubernetes 集群。",
		AliasesZH:      []string{"发布应用", "部署上线", "下发配置", "创建Release"},
		AliasesEN:      []string{"deploy target", "create release", "rollout deployment"},
		Purpose:        aitool.Text("创建异步 Release 将部署目标的当前期望规格、环境变量、密钥、文件与解析锁定的镜像下发到运行集群。", "Create an asynchronous Release to deploy the target's desired configuration and locked container image to Kubernetes."),
		ApprovalTier:   aitool.ApprovalTierL2,
		RequiredScopes: []string{string(authz.ActionDeploymentRelease)},
		Effect:         aitool.EffectAsync,
		Verification:   releaseAsyncVerificationRule(),
		Preview:        h.previewDeliveryDeployTarget,
		Execute:        h.executeDeliveryDeployTarget,
	})

	aitool.Register(reg, aitool.Spec[deliveryRollbackTargetInput, deliveryReleaseMutationOutput]{
		Name:           "delivery_rollback_target",
		Domain:         aitool.DomainDelivery,
		Title:          "回滚部署目标镜像版本",
		Summary:        "异步创建镜像回滚发布任务（仅恢复历史成功版本的容器镜像与构建引用，不回退环境变量或规格）。",
		AliasesZH:      []string{"回滚发布", "版本回退", "回滚镜像"},
		AliasesEN:      []string{"rollback target", "rollback release"},
		Purpose:        aitool.Text("将部署目标异步回滚到历史成功 Release 的容器镜像版本（仅回滚镜像与构建引用，不回退环境变量/密钥/规格）。", "Create a rollback Release restoring a previous succeeded Release's image and build reference without reverting env/secrets/specs."),
		ApprovalTier:   aitool.ApprovalTierL2,
		RequiredScopes: []string{string(authz.ActionDeploymentRollback)},
		Effect:         aitool.EffectAsync,
		Verification:   releaseAsyncVerificationRule(),
		Preview:        h.previewDeliveryRollbackTarget,
		Execute:        h.executeDeliveryRollbackTarget,
	})

	aitool.Register(reg, aitool.Spec[deliveryRestartTargetInput, deliveryRestartTargetOutput]{
		Name:           "delivery_restart_target",
		Domain:         aitool.DomainDelivery,
		Title:          "滚动重启线上 Pod",
		Summary:        "同步原地触发部署目标现有 K8s Deployment/StatefulSet 的 Pod 滚动重建（不下发未发布的配置改动）。",
		AliasesZH:      []string{"重启应用", "滚动重启Pod", "重启容器"},
		AliasesEN:      []string{"restart target", "rolling restart pods"},
		Purpose:        aitool.Text("向线上已部署的 Deployment/StatefulSet 写入 restartedAt 注解触发滚动重建（不创建 Release，也不下发未发布的配置变更）。", "Trigger an in-place rolling restart of live Kubernetes Pods without creating a Release or applying unpublished config changes."),
		ApprovalTier:   aitool.ApprovalTierL2,
		RequiredScopes: []string{string(authz.ActionDeploymentRestart)},
		Effect:         aitool.EffectExecute,
		Preview:        h.previewDeliveryRestartTarget,
		Execute:        h.executeDeliveryRestartTarget,
	})

	aitool.Register(reg, aitool.Spec[deliveryCancelReleaseInput, deliveryCancelReleaseOutput]{
		Name:           "delivery_cancel_release",
		Domain:         aitool.DomainDelivery,
		Title:          "取消排队或进行中的发布任务",
		Summary:        "取消 pending 或 running 状态的发布任务（若资源已下发集群则仅停止等待 Rollout，不自动回滚 Pod）。",
		AliasesZH:      []string{"取消发布", "终止部署任务"},
		AliasesEN:      []string{"cancel release", "abort deployment"},
		Purpose:        aitool.Text("终止排队中或运行中的 Release 工作流；若需恢复旧镜像请再调用 delivery_rollback_target。", "Cancel a pending or running Release workflow; call delivery_rollback_target afterward if live Pods already updated."),
		ApprovalTier:   aitool.ApprovalTierL2,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentRelease)},
		Effect:         aitool.EffectWrite,
		Verification:   releaseCancelVerificationRule(),
		Preview:        h.previewDeliveryCancelRelease,
		Execute:        h.executeDeliveryCancelRelease,
	})
}

// ============================================================================
// Tool 9 Implementation: delivery_scale_target
// ============================================================================

func (h *Handler) validateScaleTargetPreconditions(
	tc *aitool.ToolContext,
	in deliveryScaleTargetInput,
) (model.User, model.Project, model.Application, model.DeploymentTarget, int, int64, *kubeprovider.Client, string, error) {
	if in.Replicas == nil || *in.Replicas < 0 {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", aitool.BadRequest("deployment_target.replicas_invalid", "replicas must be an integer >= 0")
	}
	desired := *in.Replicas
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentUpdate)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", err
	}
	if err := h.ensureAIDeploymentTargetCanMutate(tc, target); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", err
	}
	if desired > 0 {
		if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", err
		}
	}

	if target.AutoScalingEnabled {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", aitool.NewErrorWithHint(
			http.StatusConflict,
			"deployment_target.autoscaling_enabled",
			"manual replica scaling is blocked while HorizontalPodAutoscaler (autoScaling) is enabled",
			"Disable `autoScaling.enabled` via `delivery_update_target_runtime` and run `delivery_deploy_target` to remove the HPA first.",
		)
	}

	var releaseCount int64
	if err := db.Model(&model.Release{}).
		Where("project_id = ? AND application_id = ? AND deployment_target_id = ?", project.ID, app.ID, target.ID).
		Count(&releaseCount).Error; err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	client, namespace, code := h.kubernetesClientForDeploymentTargetObservation(project, target, tc.Context())
	if client != nil {
		probeCtx, cancel := context.WithTimeout(tc.Context(), deploymentTargetObservationTimeout)
		hpaSnap, hpaErr := client.GetManagedResource(probeCtx, "HorizontalPodAutoscaler", namespace, deploymentTargetResourceName(target))
		cancel()
		if hpaErr == nil && hpaSnap.DeploymentTargetID == target.ID {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", aitool.NewErrorWithHint(
				http.StatusConflict,
				"deployment_target.autoscaling_enabled",
				"a live HorizontalPodAutoscaler still exists in Kubernetes for this deployment target",
				"Run `delivery_deploy_target` first so the Worker removes the residual Kubernetes HorizontalPodAutoscaler before manual scaling.",
			)
		}
	} else if releaseCount > 0 && code != "" {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, 0, 0, nil, "", aitool.NewError(
			http.StatusBadGateway,
			"deployment_target.scale_failed",
			fmt.Sprintf("runtime cluster is unavailable (%s); cannot scale live workload", code),
		)
	}

	return user, project, app, target, desired, releaseCount, client, namespace, nil
}

func (h *Handler) previewDeliveryScaleTarget(tc *aitool.ToolContext, in deliveryScaleTargetInput) (*aitool.PreviewResult, error) {
	_, project, app, target, desired, releaseCount, _, _, err := h.validateScaleTargetPreconditions(tc, in)
	if err != nil {
		return nil, err
	}
	var warnings []string
	if desired == 0 {
		warnings = append(warnings, "Scaling replicas to 0 stops all running Pods for this environment until scaled back up.")
	}
	if releaseCount == 0 {
		warnings = append(warnings, "Target has not been deployed yet; desired replicas will be saved and applied on first release.")
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("将部署目标 %s/%s 副本数从 %d 调整为 %d", app.Identifier, target.Stage, target.Replicas, desired),
			TargetResource: fmt.Sprintf("%s/%s", app.Identifier, target.Stage),
			TargetID:       target.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{{
				Field:  "replicas",
				Before: target.Replicas,
				After:  desired,
				Action: "scale",
			}},
			Warnings: warnings,
		},
	}, nil
}

func (h *Handler) executeDeliveryScaleTarget(tc *aitool.ToolContext, in deliveryScaleTargetInput) (deliveryScaleTargetOutput, error) {
	user, project, app, target, desired, _, client, namespace, err := h.validateScaleTargetPreconditions(tc, in)
	if err != nil {
		return deliveryScaleTargetOutput{}, err
	}
	ctx := tc.Context()
	db := h.aiDB(tc)

	prevReplicas := target.Replicas
	appliedLive := false
	requiresRelease := true

	txErr := db.Transaction(func(tx *gorm.DB) error {
		var locked model.DeploymentTarget
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&locked, "id = ? AND project_id = ?", target.ID, project.ID).Error; err != nil {
			return err
		}
		if !h.host.ResourceCanMutateDuringDelete(locked.DeleteStatus) {
			return aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is in progress")
		}
		if locked.AutoScalingEnabled {
			return aitool.Conflict("deployment_target.autoscaling_enabled", "autoScaling is enabled on this deployment target")
		}
		prevReplicas = locked.Replicas
		if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", locked.ID).UpdateColumn("replicas", desired).Error; err != nil {
			return err
		}

		if client == nil {
			appliedLive = false
			requiresRelease = true
			return nil
		}

		scaleCtx, cancel := context.WithTimeout(ctx, deploymentTargetScaleTimeout)
		defer cancel()
		primaryFound, scaleErr := scaleDeploymentTargetWorkload(scaleCtx, client, namespace, locked, desired)
		if scaleErr != nil {
			return &deploymentTargetScaleError{cause: scaleErr}
		}
		if primaryFound {
			appliedLive = true
			requiresRelease = false
			return nil
		}

		// Fallback to alternate workload controller (Deployment <-> StatefulSet) if workloadType was switched before release
		altTarget := locked
		if normalizeWorkloadType(locked.WorkloadType) == "StatefulSet" {
			altTarget.WorkloadType = "Deployment"
		} else {
			altTarget.WorkloadType = "StatefulSet"
		}
		altFound, altErr := scaleDeploymentTargetWorkload(scaleCtx, client, namespace, altTarget, desired)
		if altErr != nil {
			return &deploymentTargetScaleError{cause: altErr}
		}
		if altFound {
			appliedLive = true
			requiresRelease = true // Controller type switch still pending release
			return nil
		}

		appliedLive = false
		requiresRelease = true
		return nil
	})
	if txErr != nil {
		if te, ok := aitool.AsToolError(txErr); ok {
			return deliveryScaleTargetOutput{}, te
		}
		var scaleErr *deploymentTargetScaleError
		if errors.As(txErr, &scaleErr) {
			h.auditWithContext(user.ID, "deployment_target.scale", target.ID, false, scaleErr.Error(), ctx)
			scratchCtx, rec := newScratchGin(tc)
			writeDeploymentTargetScaleError(scratchCtx, scaleErr)
			return deliveryScaleTargetOutput{}, toolErrorFromRecorder(rec, http.StatusBadGateway, "deployment_target.scale_failed", scaleErr.Error())
		}
		return deliveryScaleTargetOutput{}, aitool.NewError(http.StatusBadRequest, "deployment_target.scale_failed", txErr.Error())
	}

	auditMsg := "workload_not_found"
	if appliedLive {
		if desired == 0 {
			auditMsg = "scaled_to_zero"
		} else {
			auditMsg = "scaled_to_positive"
		}
	}
	h.auditWithContext(user.ID, "deployment_target.scale", target.ID, true, auditMsg, ctx)

	return deliveryScaleTargetOutput{
		ID:                 target.ID,
		DeploymentTargetID: target.ID,
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		Stage:              target.Stage,
		PreviousReplicas:   prevReplicas,
		Replicas:           desired,
		AppliedLive:        appliedLive,
		RequiresRelease:    requiresRelease,
	}, nil
}

// ============================================================================
// Tool 10 Implementation: delivery_deploy_target
// ============================================================================

func (h *Handler) resolveDeployTargetPlan(
	tc *aitool.ToolContext,
	in deliveryDeployTargetInput,
) (model.User, model.Project, model.Application, model.DeploymentTarget, string, string, string, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRelease)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", err
	}
	if err := h.ensureAIDeploymentTargetCanMutate(tc, target); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", err
	}
	if !target.Enabled {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aitool.NewErrorWithHint(
			http.StatusConflict,
			"deployment_target.disabled",
			"deployment target is currently disabled",
			"Enable `governance.enabled` via `delivery_update_target_runtime` before deploying.",
		)
	}
	if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", err
	}

	if activeRel, hasActive, aErr := findActiveReleaseTx(db, project.ID, app.ID, target.ID); aErr != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aErr
	} else if hasActive {
		tErr := aitool.NewErrorWithHint(
			http.StatusConflict,
			"release.in_progress",
			fmt.Sprintf("another release (%s, status=%s) is currently active for this deployment target", activeRel.ID, activeRel.Status),
			"Wait for the active release using `wait_for_task` / `delivery_get_release`, or cancel it via `delivery_cancel_release`.",
		)
		tErr.Details = map[string]any{"activeReleaseId": activeRel.ID, "activeStatus": activeRel.Status}
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", tErr
	}

	var lastSucceeded model.Release
	hasLastSucceeded := db.Where("project_id = ? AND application_id = ? AND deployment_target_id = ? AND status = ?",
		project.ID, app.ID, target.ID, "succeeded").
		Order("revision desc, created_at desc").
		First(&lastSucceeded).Error == nil

	currentImage := strings.TrimSpace(lastSucceeded.ImageRef)

	var (
		resolvedImageRef   string
		resolvedBuildRunID string
	)
	explicitBuildRunID := strings.TrimSpace(in.BuildRunID)
	explicitImageRef := strings.TrimSpace(in.ImageRef)

	if explicitBuildRunID != "" {
		var run model.BuildRun
		if rErr := db.First(&run, "id = ? AND project_id = ? AND application_id = ?", explicitBuildRunID, project.ID, app.ID).Error; rErr != nil {
			if errors.Is(rErr, gorm.ErrRecordNotFound) {
				return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aitool.NotFound("build_run.not_found", "build run was not found in this application")
			}
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aitool.NewError(http.StatusInternalServerError, "database.query_failed", rErr.Error())
		}
		if run.DeploymentTargetID != target.ID {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aitool.BadRequest("argument_invalid", "buildRunId does not belong to this deployment target")
		}
		if run.Status != "succeeded" {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aitool.Conflict("build_run.not_succeeded", "only succeeded build runs can be deployed")
		}
		runImg := resolveBuildRunImageRef(db, run)
		if runImg == "" {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aitool.BadRequest("build_run.image_missing", "selected build run has no image reference")
		}
		if explicitImageRef != "" && explicitImageRef != runImg {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aitool.BadRequest("argument_invalid", "imageRef conflicts with the selected buildRunId's image")
		}
		resolvedImageRef = runImg
		resolvedBuildRunID = run.ID
	} else if explicitImageRef != "" {
		resolvedImageRef = explicitImageRef
	} else if normalizeDeploymentSourceType(target.SourceType) == "repository" {
		if hasLastSucceeded && strings.TrimSpace(lastSucceeded.ImageRef) != "" {
			resolvedImageRef = strings.TrimSpace(lastSucceeded.ImageRef)
			resolvedBuildRunID = strings.TrimSpace(lastSucceeded.BuildRunID)
		} else {
			var latestRun model.BuildRun
			if rErr := db.Where("project_id = ? AND application_id = ? AND deployment_target_id = ? AND status = ?",
				project.ID, app.ID, target.ID, "succeeded").
				Order("finished_at desc nulls last, created_at desc").
				First(&latestRun).Error; rErr == nil {
				resolvedImageRef = resolveBuildRunImageRef(db, latestRun)
				resolvedBuildRunID = latestRun.ID
			} else if strings.TrimSpace(target.ImageRef) != "" {
				resolvedImageRef = strings.TrimSpace(target.ImageRef)
			}
		}
		if resolvedImageRef == "" {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aitool.NewErrorWithHint(
				http.StatusConflict,
				"release.image_required",
				"no succeeded build run or previously deployed image is available for this repository-sourced target",
				"Trigger a build first via `build_trigger_run` and wait for it to succeed, or pass an explicit `imageRef`.",
			)
		}
	} else {
		// sourceType == "image"
		if hasLastSucceeded && !target.UpdatedAt.After(lastSucceeded.CreatedAt) && strings.TrimSpace(lastSucceeded.ImageRef) != "" {
			resolvedImageRef = strings.TrimSpace(lastSucceeded.ImageRef)
		} else {
			resolvedImageRef = firstNonEmpty(strings.TrimSpace(target.ImageRef), strings.TrimSpace(lastSucceeded.ImageRef))
		}
		if resolvedImageRef == "" {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", "", "", aitool.Conflict("release.image_required", "deployment target has no configured imageRef")
		}
	}

	return user, project, app, target, currentImage, resolvedImageRef, resolvedBuildRunID, nil
}

func resolveBuildRunImageRef(db *gorm.DB, run model.BuildRun) string {
	img := strings.TrimSpace(run.ImageRef)
	if img == "" && strings.TrimSpace(run.TargetRegistryID) != "" {
		var reg model.ArtifactRegistry
		if err := db.First(&reg, "id = ?", run.TargetRegistryID).Error; err == nil {
			img = buildImageRef(reg, run)
		}
	}
	return strings.TrimSpace(img)
}

func findActiveReleaseTx(db *gorm.DB, projectID, appID, targetID string) (model.Release, bool, error) {
	var active model.Release
	err := db.Where("project_id = ? AND application_id = ? AND deployment_target_id = ? AND status IN ?",
		projectID, appID, targetID, model.ReleaseActiveStatuses).
		Order("created_at desc").
		First(&active).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Release{}, false, nil
	}
	if err != nil {
		return model.Release{}, false, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return active, true, nil
}

func (h *Handler) previewDeliveryDeployTarget(tc *aitool.ToolContext, in deliveryDeployTargetInput) (*aitool.PreviewResult, error) {
	_, project, app, target, currentImage, resolvedImageRef, resolvedBuildRunID, err := h.resolveDeployTargetPlan(tc, in)
	if err != nil {
		return nil, err
	}
	beforeImg := any(currentImage)
	if currentImage == "" {
		beforeImg = "(not deployed)"
	}
	changes := []aitool.PreviewChange{
		{Field: "imageRef", Before: beforeImg, After: resolvedImageRef, Action: "deploy"},
	}
	if resolvedBuildRunID != "" {
		changes = append(changes, aitool.PreviewChange{Field: "buildRunId", After: resolvedBuildRunID, Action: "deploy"})
	}
	if boolVal(in.ForceImagePull, false) {
		changes = append(changes, aitool.PreviewChange{Field: "forceImagePull", After: true, Action: "deploy"})
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("发布部署目标 %s/%s（镜像 %s）", app.Identifier, target.Stage, resolvedImageRef),
			TargetResource: fmt.Sprintf("%s/%s", app.Identifier, target.Stage),
			TargetID:       target.ID,
			ProjectID:      project.ID,
			Changes:        changes,
		},
		LockedPayload: aiDeployPreviewLock{
			TargetID:           target.ID,
			ResolvedImageRef:   resolvedImageRef,
			ResolvedBuildRunID: resolvedBuildRunID,
		},
	}, nil
}

func (h *Handler) executeDeliveryDeployTarget(tc *aitool.ToolContext, in deliveryDeployTargetInput) (deliveryReleaseMutationOutput, error) {
	var lock aiDeployPreviewLock
	if err := tc.UnmarshalLockedPayload(&lock); err != nil {
		return deliveryReleaseMutationOutput{}, aitool.Conflict("ai.preview_lock_invalid", err.Error())
	}
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRelease)
	if err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	db := h.aiDB(tc)
	ctx := tc.Context()
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	if lock.TargetID != target.ID || strings.TrimSpace(lock.ResolvedImageRef) == "" {
		return deliveryReleaseMutationOutput{}, aitool.Conflict("ai.preview_lock_invalid", "preview lock does not match resolved deployment target")
	}
	if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
		return deliveryReleaseMutationOutput{}, err
	}

	scratchCtx, _ := newScratchGin(tc)
	idempotencyKey, validKey := transportapi.DeliveryIdempotencyKey(scratchCtx, "release.create")
	if !validKey {
		return deliveryReleaseMutationOutput{}, aitool.BadRequest("request.idempotency_key_invalid", "Idempotency-Key must contain 8 to 128 characters")
	}
	reqHash := transportapi.DeliveryRequestHash(map[string]any{
		"projectId":          project.ID,
		"applicationId":      app.ID,
		"deploymentTargetId": target.ID,
		"imageRef":           lock.ResolvedImageRef,
		"buildRunId":         lock.ResolvedBuildRunID,
		"forceImagePull":     boolVal(in.ForceImagePull, false),
		"message":            strings.TrimSpace(in.Message),
	})
	if idempotencyKey != "" {
		replayed, found, rErr := h.replayAIReleaseByIdempotency(tc, user.ID, project.ID, idempotencyKey, reqHash)
		if rErr != nil {
			return deliveryReleaseMutationOutput{}, rErr
		}
		if found {
			return releaseMutationOutputFromModel(replayed, target.Stage), nil
		}
	}

	var release model.Release
	txErr := db.Transaction(func(tx *gorm.DB) error {
		lockedTarget, lockErr := volume.NewGormRepository(tx).LockDeploymentTarget(ctx, project.ID, target.ID)
		if lockErr != nil {
			return lockErr
		}
		if !h.host.ResourceCanMutateDuringDelete(lockedTarget.DeleteStatus) {
			return aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is in progress")
		}
		if !lockedTarget.Enabled {
			return aitool.Conflict("deployment_target.disabled", "deployment target is disabled")
		}
		if activeRel, hasActive, aErr := findActiveReleaseTx(tx, project.ID, app.ID, target.ID); aErr != nil {
			return aErr
		} else if hasActive {
			tErr := aitool.Conflict("release.in_progress", fmt.Sprintf("release %s is already %s", activeRel.ID, activeRel.Status))
			tErr.Details = map[string]any{"activeReleaseId": activeRel.ID}
			return tErr
		}
		rev, revErr := nextReleaseRevisionFor(tx, project.ID, app.ID, target.ID)
		if revErr != nil {
			return revErr
		}
		release = model.Release{
			ID:                     id.New("rel"),
			ProjectID:              project.ID,
			ApplicationID:          app.ID,
			DeploymentTargetID:     target.ID,
			BuildRunID:             lock.ResolvedBuildRunID,
			ImageRef:               lock.ResolvedImageRef,
			ForceImagePull:         boolVal(in.ForceImagePull, false),
			Type:                   "deploy",
			Status:                 "pending",
			Revision:               rev,
			Message:                strings.TrimSpace(in.Message),
			CreatedBy:              user.ID,
			IdempotencyKey:         idempotencyKey,
			IdempotencyRequestHash: reqHash,
		}
		return tx.Create(&release).Error
	})
	if txErr != nil {
		if idempotencyKey != "" {
			if replayed, found, rErr := h.replayAIReleaseByIdempotency(tc, user.ID, project.ID, idempotencyKey, reqHash); rErr == nil && found {
				return releaseMutationOutputFromModel(replayed, target.Stage), nil
			}
		}
		if te, ok := aitool.AsToolError(txErr); ok {
			return deliveryReleaseMutationOutput{}, te
		}
		return deliveryReleaseMutationOutput{}, aitool.NewError(http.StatusBadRequest, "release.create_failed", txErr.Error())
	}

	if !h.enqueueDeployRun(ctx, release) {
		if release.IdempotencyKey == "" {
			_ = markReleaseDispatchFailed(db, release)
		}
		return deliveryReleaseMutationOutput{}, aitool.NewError(http.StatusServiceUnavailable, "release.dispatch_unavailable", "deployment task queue is temporarily unavailable")
	}

	h.auditWithContext(user.ID, "release.create", release.ID, true, release.ImageRef, ctx)
	return releaseMutationOutputFromModel(release, target.Stage), nil
}

func (h *Handler) replayAIReleaseByIdempotency(tc *aitool.ToolContext, userID, projectID, key, requestHash string) (model.Release, bool, error) {
	db := h.aiDB(tc)
	var existing model.Release
	err := db.First(&existing, "created_by = ? AND idempotency_key = ?", userID, key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Release{}, false, nil
	}
	if err != nil {
		return model.Release{}, false, aitool.NewError(http.StatusInternalServerError, "deployment.idempotency_unavailable", "release idempotency state is unavailable")
	}
	if existing.ProjectID != projectID || existing.IdempotencyRequestHash != requestHash {
		return model.Release{}, false, aitool.Conflict("request.idempotency_conflict", "idempotency key belongs to another request")
	}
	if existing.Status == "pending" && !h.enqueueDeployRun(tc.Context(), existing) {
		return model.Release{}, false, aitool.NewError(http.StatusServiceUnavailable, "release.dispatch_unavailable", "deployment task queue is temporarily unavailable")
	}
	return existing, true, nil
}

func releaseMutationOutputFromModel(rel model.Release, stage string) deliveryReleaseMutationOutput {
	return deliveryReleaseMutationOutput{
		ID:                 rel.ID,
		ReleaseID:          rel.ID,
		ProjectID:          rel.ProjectID,
		ApplicationID:      rel.ApplicationID,
		DeploymentTargetID: rel.DeploymentTargetID,
		Stage:              stage,
		Revision:           rel.Revision,
		Type:               rel.Type,
		Status:             rel.Status,
		ImageRef:           rel.ImageRef,
		BuildRunID:         rel.BuildRunID,
		RollbackFromID:     rel.RollbackFromID,
		ForceImagePull:     rel.ForceImagePull,
		Message:            rel.Message,
		CreatedAt:          rel.CreatedAt,
	}
}

// ============================================================================
// Tool 11 Implementation: delivery_rollback_target
// ============================================================================

func (h *Handler) resolveRollbackTargetPlan(
	tc *aitool.ToolContext,
	in deliveryRollbackTargetInput,
) (model.User, model.Project, model.Application, model.DeploymentTarget, string, model.Release, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRollback)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, err
	}
	if err := h.ensureAIDeploymentTargetCanMutate(tc, target); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, err
	}
	if !target.Enabled {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, aitool.Conflict("deployment_target.disabled", "deployment target is disabled")
	}
	if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, err
	}

	if activeRel, hasActive, aErr := findActiveReleaseTx(db, project.ID, app.ID, target.ID); aErr != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, aErr
	} else if hasActive {
		tErr := aitool.NewErrorWithHint(
			http.StatusConflict,
			"release.in_progress",
			fmt.Sprintf("release %s is currently %s; cannot start rollback while a release is active", activeRel.ID, activeRel.Status),
			"Cancel the active release via `delivery_cancel_release` first or wait for it to finish.",
		)
		tErr.Details = map[string]any{"activeReleaseId": activeRel.ID}
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, tErr
	}

	var latestRel model.Release
	hasLatest := db.Where("project_id = ? AND application_id = ? AND deployment_target_id = ?", project.ID, app.ID, target.ID).
		Order("revision desc, created_at desc").
		First(&latestRel).Error == nil
	if !hasLatest {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, aitool.Conflict("release.rollback_unavailable", "no release history exists for this deployment target")
	}

	var currentSucceeded model.Release
	currentImg := latestRel.ImageRef
	if db.Where("project_id = ? AND application_id = ? AND deployment_target_id = ? AND status = ?", project.ID, app.ID, target.ID, "succeeded").
		Order("revision desc, created_at desc").
		First(&currentSucceeded).Error == nil {
		currentImg = currentSucceeded.ImageRef
	}

	var targetRel model.Release
	if explicitTo := strings.TrimSpace(in.ToReleaseID); explicitTo != "" {
		if err := db.First(&targetRel, "id = ? AND project_id = ?", explicitTo, project.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, aitool.NotFound("release.not_found", "toReleaseId was not found")
			}
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if targetRel.ApplicationID != app.ID || targetRel.DeploymentTargetID != target.ID {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, aitool.BadRequest("argument_invalid", "toReleaseId must belong to the same deployment target")
		}
		if targetRel.Status != "succeeded" {
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, aitool.Conflict("release.rollback_target_not_succeeded", "toReleaseId must be a succeeded release")
		}
	} else {
		// Catalog §1.5 #24 exact default rule:
		// If latest Release is failed/canceled -> restore to most recent succeeded release;
		// Only when latest Release itself is succeeded -> restore to the succeeded release before it!
		q := db.Where("project_id = ? AND application_id = ? AND deployment_target_id = ? AND status = ?",
			project.ID, app.ID, target.ID, "succeeded")
		if latestRel.Status == "succeeded" {
			q = q.Where("revision < ?", latestRel.Revision)
		}
		if err := q.Order("revision desc, created_at desc").First(&targetRel).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, aitool.Conflict("release.rollback_unavailable", "no previous succeeded release is available to roll back to")
			}
			return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, "", model.Release{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
	}

	return user, project, app, target, currentImg, targetRel, nil
}

func (h *Handler) previewDeliveryRollbackTarget(tc *aitool.ToolContext, in deliveryRollbackTargetInput) (*aitool.PreviewResult, error) {
	_, project, app, target, currentImg, targetRel, err := h.resolveRollbackTargetPlan(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("将部署目标 %s/%s 回滚至发布 %s（revision %d）", app.Identifier, target.Stage, targetRel.ID, targetRel.Revision),
			TargetResource: fmt.Sprintf("%s/%s", app.Identifier, target.Stage),
			TargetID:       target.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{
					Field:  "imageRef",
					Before: currentImg,
					After:  fmt.Sprintf("%s (from %s, revision %d)", targetRel.ImageRef, targetRel.ID, targetRel.Revision),
					Action: "rollback",
				},
			},
			Warnings: []string{
				"Rollback restores ONLY the container image and build reference; it does NOT revert environment variables, secrets, config files, or resource specifications.",
			},
		},
		LockedPayload: aiRollbackPreviewLock{
			TargetID:            target.ID,
			ResolvedToReleaseID: targetRel.ID,
			ResolvedImageRef:    targetRel.ImageRef,
			ResolvedBuildRunID:  targetRel.BuildRunID,
			TargetRevision:      targetRel.Revision,
		},
	}, nil
}

func (h *Handler) executeDeliveryRollbackTarget(tc *aitool.ToolContext, in deliveryRollbackTargetInput) (deliveryReleaseMutationOutput, error) {
	var lock aiRollbackPreviewLock
	if err := tc.UnmarshalLockedPayload(&lock); err != nil {
		return deliveryReleaseMutationOutput{}, aitool.Conflict("ai.preview_lock_invalid", err.Error())
	}
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRollback)
	if err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	db := h.aiDB(tc)
	ctx := tc.Context()
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return deliveryReleaseMutationOutput{}, err
	}
	if lock.TargetID != target.ID || strings.TrimSpace(lock.ResolvedToReleaseID) == "" || strings.TrimSpace(lock.ResolvedImageRef) == "" {
		return deliveryReleaseMutationOutput{}, aitool.Conflict("ai.preview_lock_invalid", "preview lock does not match resolved rollback target")
	}
	if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
		return deliveryReleaseMutationOutput{}, err
	}

	scratchCtx, _ := newScratchGin(tc)
	idempotencyKey, validKey := transportapi.DeliveryIdempotencyKey(scratchCtx, "release.rollback")
	if !validKey {
		return deliveryReleaseMutationOutput{}, aitool.BadRequest("request.idempotency_key_invalid", "Idempotency-Key must contain 8 to 128 characters")
	}
	reqHash := transportapi.DeliveryRequestHash(map[string]any{
		"projectId":          project.ID,
		"applicationId":      app.ID,
		"deploymentTargetId": target.ID,
		"toReleaseId":        lock.ResolvedToReleaseID,
		"message":            strings.TrimSpace(in.Message),
	})
	if idempotencyKey != "" {
		replayed, found, rErr := h.replayAIReleaseByIdempotency(tc, user.ID, project.ID, idempotencyKey, reqHash)
		if rErr != nil {
			return deliveryReleaseMutationOutput{}, rErr
		}
		if found {
			return releaseMutationOutputFromModel(replayed, target.Stage), nil
		}
	}

	var release model.Release
	txErr := db.Transaction(func(tx *gorm.DB) error {
		lockedTarget, lockErr := volume.NewGormRepository(tx).LockDeploymentTarget(ctx, project.ID, target.ID)
		if lockErr != nil {
			return lockErr
		}
		if !h.host.ResourceCanMutateDuringDelete(lockedTarget.DeleteStatus) {
			return aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is in progress")
		}
		if !lockedTarget.Enabled {
			return aitool.Conflict("deployment_target.disabled", "deployment target is disabled")
		}
		if activeRel, hasActive, aErr := findActiveReleaseTx(tx, project.ID, app.ID, target.ID); aErr != nil {
			return aErr
		} else if hasActive {
			tErr := aitool.Conflict("release.in_progress", fmt.Sprintf("release %s is currently %s", activeRel.ID, activeRel.Status))
			tErr.Details = map[string]any{"activeReleaseId": activeRel.ID}
			return tErr
		}
		rev, revErr := nextReleaseRevisionFor(tx, project.ID, app.ID, target.ID)
		if revErr != nil {
			return revErr
		}
		release = model.Release{
			ID:                     id.New("rel"),
			ProjectID:              project.ID,
			ApplicationID:          app.ID,
			DeploymentTargetID:     target.ID,
			BuildRunID:             lock.ResolvedBuildRunID,
			ImageRef:               lock.ResolvedImageRef,
			Type:                   "rollback",
			Status:                 "pending",
			Revision:               rev,
			RollbackFromID:         lock.ResolvedToReleaseID,
			Message:                strings.TrimSpace(in.Message),
			CreatedBy:              user.ID,
			IdempotencyKey:         idempotencyKey,
			IdempotencyRequestHash: reqHash,
		}
		return tx.Create(&release).Error
	})
	if txErr != nil {
		if idempotencyKey != "" {
			if replayed, found, rErr := h.replayAIReleaseByIdempotency(tc, user.ID, project.ID, idempotencyKey, reqHash); rErr == nil && found {
				return releaseMutationOutputFromModel(replayed, target.Stage), nil
			}
		}
		if te, ok := aitool.AsToolError(txErr); ok {
			return deliveryReleaseMutationOutput{}, te
		}
		return deliveryReleaseMutationOutput{}, aitool.NewError(http.StatusBadRequest, "release.rollback_failed", txErr.Error())
	}

	if !h.enqueueDeployRun(ctx, release) {
		if release.IdempotencyKey == "" {
			_ = markReleaseDispatchFailed(db, release)
		}
		return deliveryReleaseMutationOutput{}, aitool.NewError(http.StatusServiceUnavailable, "release.dispatch_unavailable", "deployment task queue is temporarily unavailable")
	}

	h.auditWithContext(user.ID, "release.rollback", release.ID, true, lock.ResolvedToReleaseID, ctx)
	return releaseMutationOutputFromModel(release, target.Stage), nil
}

// ============================================================================
// Tool 12 Implementation: delivery_restart_target
// ============================================================================

func (h *Handler) resolveRestartTargetClient(
	tc *aitool.ToolContext,
	in deliveryRestartTargetInput,
) (model.User, model.Project, model.Application, model.DeploymentTarget, *kubeprovider.Client, string, string, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRestart)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, nil, "", "", err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, nil, "", "", err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, nil, "", "", err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, nil, "", "", err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, nil, "", "", err
	}
	if err := h.ensureAIDeploymentTargetCanMutate(tc, target); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, nil, "", "", err
	}
	scratchCtx, rec := newScratchGin(tc)
	client, namespace, ok := h.kubernetesClientForDeploymentTarget(scratchCtx, project, target, "runtime cluster is unavailable")
	if !ok {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, nil, "", "", toolErrorFromRecorder(rec, http.StatusBadRequest, "runtime_cluster.unavailable", "runtime cluster is unavailable")
	}
	return user, project, app, target, client, namespace, deploymentTargetResourceName(target), nil
}

func notDeployedRestartError() *aitool.ToolError {
	return aitool.NewErrorWithHint(
		http.StatusConflict,
		"deployment_target.not_deployed",
		"live Kubernetes workload does not exist yet for this deployment target",
		"Deploy the target first via `delivery_deploy_target`.",
	)
}

func (h *Handler) previewDeliveryRestartTarget(tc *aitool.ToolContext, in deliveryRestartTargetInput) (*aitool.PreviewResult, error) {
	_, project, app, target, client, namespace, resourceName, err := h.resolveRestartTargetClient(tc, in)
	if err != nil {
		return nil, err
	}
	probeCtx, cancel := context.WithTimeout(tc.Context(), 10*time.Second)
	_, snapErr := client.GetDeploymentSnapshot(probeCtx, namespace, resourceName)
	cancel()
	if apierrors.IsNotFound(snapErr) {
		return nil, notDeployedRestartError()
	}
	if snapErr != nil {
		return nil, aitool.NewError(http.StatusBadGateway, "deployment_target.restart_failed", "failed to inspect live Kubernetes workload: "+snapErr.Error())
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("原地滚动重启部署目标 %s/%s 的运行 Pod", app.Identifier, target.Stage),
			TargetResource: fmt.Sprintf("%s/%s", app.Identifier, target.Stage),
			TargetID:       target.ID,
			ProjectID:      project.ID,
			Impact:         []string{fmt.Sprintf("Annotate %s/%s with restartedAt to trigger rolling Pod recreation.", namespace, resourceName)},
			Warnings:       []string{"Restarting Pods does NOT apply unpublished env, secret, config file, or spec changes; use `delivery_deploy_target` to publish desired config changes."},
		},
	}, nil
}

func (h *Handler) executeDeliveryRestartTarget(tc *aitool.ToolContext, in deliveryRestartTargetInput) (deliveryRestartTargetOutput, error) {
	user, project, app, target, client, namespace, resourceName, err := h.resolveRestartTargetClient(tc, in)
	if err != nil {
		return deliveryRestartTargetOutput{}, err
	}
	reqCtx, cancel := context.WithTimeout(tc.Context(), 10*time.Second)
	defer cancel()
	if rErr := client.RestartDeployment(reqCtx, namespace, resourceName); rErr != nil {
		h.auditWithContext(user.ID, "deployment_target.restart", target.ID, false, rErr.Error(), tc.Context())
		if apierrors.IsNotFound(rErr) {
			return deliveryRestartTargetOutput{}, notDeployedRestartError()
		}
		return deliveryRestartTargetOutput{}, aitool.NewError(http.StatusBadGateway, "deployment_target.restart_failed", "failed to restart Kubernetes workload")
	}
	now := time.Now().UTC()
	h.auditWithContext(user.ID, "deployment_target.restart", target.ID, true, resourceName, tc.Context())
	return deliveryRestartTargetOutput{
		ID:                 target.ID,
		DeploymentTargetID: target.ID,
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		Stage:              target.Stage,
		KubernetesName:     resourceName,
		Restarted:          true,
		RestartedAt:        now,
	}, nil
}

// ============================================================================
// Tool 13 Implementation: delivery_cancel_release
// ============================================================================

const cancelReleaseRolloutWarning = "If the Worker has already applied resources to Kubernetes and is waiting for rollout, canceling terminates the wait task without automatically rolling back live Pods; call `delivery_rollback_target` if you need to restore the previous image."

func (h *Handler) previewDeliveryCancelRelease(tc *aitool.ToolContext, in deliveryCancelReleaseInput) (*aitool.PreviewResult, error) {
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRelease)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	releaseID := strings.TrimSpace(in.ReleaseID)
	if releaseID == "" {
		return nil, aitool.BadRequest("release.required", "releaseId is required")
	}
	var release model.Release
	if err := db.First(&release, "id = ? AND project_id = ?", releaseID, project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("release.not_found", "release was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if release.Status != "pending" && release.Status != "running" && release.Status != "canceled" {
		return nil, aitool.Conflict(releaseNotCancelableCode, "only pending or running releases can be canceled")
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("取消发布任务 %s（当前状态 %s）", release.ID, release.Status),
			TargetResource: release.ID,
			TargetID:       release.ID,
			ProjectID:      project.ID,
			Changes:        []aitool.PreviewChange{{Field: "status", Before: release.Status, After: "canceled", Action: "cancel"}},
			Warnings:       []string{cancelReleaseRolloutWarning},
		},
	}, nil
}

func (h *Handler) executeDeliveryCancelRelease(tc *aitool.ToolContext, in deliveryCancelReleaseInput) (deliveryCancelReleaseOutput, error) {
	tc.SetNoStore()
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRelease)
	if err != nil {
		return deliveryCancelReleaseOutput{}, err
	}
	db := h.aiDB(tc)
	releaseID := strings.TrimSpace(in.ReleaseID)
	if releaseID == "" {
		return deliveryCancelReleaseOutput{}, aitool.BadRequest("release.required", "releaseId is required")
	}
	var release model.Release
	if err := db.First(&release, "id = ? AND project_id = ?", releaseID, project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return deliveryCancelReleaseOutput{}, aitool.NotFound("release.not_found", "release was not found")
		}
		return deliveryCancelReleaseOutput{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	canceled := false
	txErr := db.Transaction(func(tx *gorm.DB) error {
		var locked model.Release
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ? AND project_id = ?", release.ID, project.ID).Error; err != nil {
			return err
		}
		if locked.Status == "canceled" {
			release = locked
			return nil
		}
		if locked.Status != "pending" && locked.Status != "running" {
			return errReleaseNotCancelable
		}
		finishedAt := time.Now()
		if err := tx.Model(&model.Release{}).
			Where("id = ? AND status IN ?", locked.ID, model.ReleaseActiveStatuses).
			Updates(map[string]any{
				"status":       "canceled",
				"failure_code": model.ReleaseFailureCanceledByUser,
				"finished_at":  &finishedAt,
			}).Error; err != nil {
			return err
		}
		canceled = true
		return tx.First(&release, "id = ?", locked.ID).Error
	})
	if txErr != nil {
		if errors.Is(txErr, errReleaseNotCancelable) {
			return deliveryCancelReleaseOutput{}, aitool.Conflict(releaseNotCancelableCode, "only pending or running releases can be canceled")
		}
		return deliveryCancelReleaseOutput{}, aitool.NewError(http.StatusInternalServerError, "release.cancel_failed", txErr.Error())
	}
	if canceled {
		h.auditWithContext(user.ID, "release.cancel", release.ID, true, "", tc.Context())
	}
	return deliveryCancelReleaseOutput{
		ID:                 release.ID,
		ReleaseID:          release.ID,
		ProjectID:          project.ID,
		ApplicationID:      release.ApplicationID,
		DeploymentTargetID: release.DeploymentTargetID,
		Status:             release.Status,
		FailureCode:        release.FailureCode,
		FinishedAt:         release.FinishedAt,
		Note:               cancelReleaseRolloutWarning,
	}, nil
}
