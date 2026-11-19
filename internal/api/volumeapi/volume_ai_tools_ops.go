package volumeapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

type aiListVolumesInput struct {
	Project        string  `json:"project" desc:"项目 ID 或唯一 identifier"`
	Cluster        *string `json:"cluster,omitempty" desc:"运行集群 ID（clu_xxx）或名称（传入时附带查询该集群可用 storageClasses）"`
	Availability   *string `json:"availability,omitempty" enum:"available|reserved|in_use|unavailable" desc:"按数据卷可用状态过滤"`
	LifecycleState *string `json:"lifecycleState,omitempty" enum:"provisioning|ready|deleting|error" desc:"按生命周期状态过滤"`
	SourceKind     *string `json:"sourceKind,omitempty" enum:"blank|managed|retained|archive_import|snapshot_restore|existing_claim" desc:"按数据卷创建来源过滤"`
	OwnershipMode  *string `json:"ownershipMode,omitempty" enum:"managed|referenced" desc:"按所有权模式过滤"`
	VolumeMode     *string `json:"volumeMode,omitempty" enum:"Filesystem|Block" desc:"按卷模式过滤"`
	Search         *string `json:"search,omitempty" desc:"按数据卷显示名称或底层 PVC Claim 名搜索"`
	SortBy         *string `json:"sortBy,omitempty" enum:"createdAt|updatedAt|displayName|capacity" default:"createdAt" desc:"排序字段"`
	SortOrder      *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page           *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize       *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type aiGetVolumeInput struct {
	Project    string  `json:"project" desc:"项目 ID 或唯一 identifier"`
	Volume     *string `json:"volume,omitempty" desc:"数据卷 ID（pvol_xxx）或同项目唯一 displayName（与 transferId 至少传其一）"`
	TransferID *string `json:"transferId,omitempty" desc:"传输任务 ID（vtx_xxx；单独传入时自动反解所属数据卷并一并返回该传输进度）"`
}

type aiCreateVolumeInput struct {
	Project          string  `json:"project" desc:"项目 ID 或唯一 identifier"`
	DisplayName      string  `json:"displayName" min:"1" desc:"数据卷显示名称"`
	Cluster          string  `json:"cluster" desc:"目标运行集群 ID（clu_xxx）或唯一名称"`
	SourceType       string  `json:"sourceType" enum:"blank|volumeSnapshot|existingClaim" desc:"数据卷创建来源"`
	Capacity         *string `json:"capacity,omitempty" desc:"目标存储容量（如 10Gi；blank / volumeSnapshot 时必填，existingClaim 忽略）"`
	StorageClassName *string `json:"storageClassName,omitempty" desc:"Kubernetes StorageClass 名称（省略时自动选取集群唯一默认 StorageClass）"`
	AccessMode       *string `json:"accessMode,omitempty" enum:"ReadWriteOnce|ReadWriteMany|ReadOnlyMany|ReadWriteOncePod" default:"ReadWriteOnce" desc:"PVC 访问模式（默认 ReadWriteOnce）"`
	VolumeMode       *string `json:"volumeMode,omitempty" enum:"Filesystem|Block" default:"Filesystem" desc:"数据卷模式（默认 Filesystem）"`
	ClaimName        *string `json:"claimName,omitempty" desc:"已有 PVC Claim 名称（sourceType=existingClaim 时必填）"`
	OwnershipMode    *string `json:"ownershipMode,omitempty" enum:"managed|referenced" default:"managed" desc:"纳管所有权模式（仅 sourceType=existingClaim 时可选，默认 managed）"`
	SnapshotName     *string `json:"snapshotName,omitempty" desc:"CSI VolumeSnapshot 名称（sourceType=volumeSnapshot 时必填）"`
}

type aiUpdateVolumeInput struct {
	Project          string  `json:"project" desc:"项目 ID 或唯一 identifier"`
	Volume           string  `json:"volume" desc:"数据卷 ID（pvol_xxx）或同项目唯一 displayName"`
	DisplayName      *string `json:"displayName,omitempty" desc:"新数据卷显示名称"`
	Capacity         *string `json:"capacity,omitempty" desc:"在线扩容目标容量（须大于当前容量，如 20Gi）"`
	ExpectedRevision *int64  `json:"expectedRevision,omitempty" min:"1" desc:"乐观锁版本号（省略时在审批快照阶段自动锁定当前 revision）"`
}

type aiRetryVolumeOperationInput struct {
	Project          string `json:"project" desc:"项目 ID 或唯一 identifier"`
	Volume           string `json:"volume" desc:"数据卷 ID（pvol_xxx）或同项目唯一 displayName"`
	ExpectedRevision *int64 `json:"expectedRevision,omitempty" min:"1" desc:"乐观锁版本号（省略时在审批快照阶段自动锁定当前 revision）"`
}

type aiDeleteVolumeInput struct {
	Project          string `json:"project" desc:"项目 ID 或唯一 identifier"`
	Volume           string `json:"volume" desc:"数据卷 ID（pvol_xxx）或同项目唯一 displayName"`
	ExpectedRevision *int64 `json:"expectedRevision,omitempty" min:"1" desc:"乐观锁版本号（省略时在审批快照阶段自动锁定当前 revision）"`
}

type aiCreateVolumeExportInput struct {
	Project     string  `json:"project" desc:"项目 ID 或唯一 identifier"`
	Volume      string  `json:"volume" desc:"数据卷 ID（pvol_xxx）或同项目唯一 displayName"`
	Format      *string `json:"format,omitempty" enum:"tar_gz|raw_zst" desc:"导出归档格式（省略时按卷模式自动推导：Filesystem->tar_gz，Block->raw_zst）"`
	Consistency *string `json:"consistency,omitempty" enum:"auto|snapshot|live|unmounted" default:"auto" desc:"导出一致性模式（默认 auto；Block 卷禁止 live）"`
}

type aiCancelVolumeTransferInput struct {
	Project    string `json:"project" desc:"项目 ID 或唯一 identifier"`
	TransferID string `json:"transferId" desc:"进行中的数据卷传输任务 ID（vtx_xxx）"`
}

type aiCreateVolumeLock struct {
	ClusterID        string `json:"clusterId"`
	StorageClassName string `json:"storageClassName,omitempty"`
}

type aiVolumeRevisionLock struct {
	VolumeID         string `json:"volumeId"`
	ExpectedRevision int64  `json:"expectedRevision"`
}

type aiVolumeDeleteLock struct {
	VolumeID         string `json:"volumeId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	DataAction       string `json:"dataAction"`
}

func (h *Handler) registerVolumeAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[aiListVolumesInput, map[string]any]{
		Name:           "runtime_list_volumes",
		Domain:         aitool.DomainRuntime,
		Category:       "volume",
		Summary:        "分页列出项目持久化数据卷、实时 PVC 观测状态及集群可用 StorageClass",
		Purpose:        aitool.Text("分页查看项目内的持久化数据卷（ProjectVolume）、Kubernetes PVC 实时观测状态，并在传入 cluster 时一并返回可用 StorageClass 列表。", "List project persistent volumes with live PVC observations and optional cluster StorageClasses."),
		AliasesZH:      []string{"数据卷列表", "查看 PVC", "存储类列表"},
		AliasesEN:      []string{"list volumes", "project volumes", "storage classes"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionVolumeRead)},
		Execute:        h.execListVolumesAI,
	})

	aitool.Register(reg, aitool.Spec[aiGetVolumeInput, map[string]any]{
		Name:           "runtime_get_volume",
		Domain:         aitool.DomainRuntime,
		Category:       "volume",
		Summary:        "查看单个项目数据卷详情、当前 revision、挂载绑定列表与导入导出传输记录",
		Purpose:        aitool.Text("按数据卷 ID/displayName 或传输任务 transferId 查看数据卷详情、乐观锁 revision、实时 PVC 状态、工作负载挂载关系与导入导出传输进度。", "Inspect a single project volume, its revision, mount bindings, and transfer history."),
		AliasesZH:      []string{"数据卷详情", "查看 PVC 状态", "查看卷传输进度"},
		AliasesEN:      []string{"get volume", "inspect pvc", "volume transfer status"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionVolumeRead)},
		Execute:        h.execGetVolumeAI,
	})

	aitool.Register(reg, aitool.Spec[aiCreateVolumeInput, map[string]any]{
		Name:           "runtime_create_volume",
		Domain:         aitool.DomainRuntime,
		Category:       "volume",
		Summary:        "异步创建空白托管数据卷、从快照恢复数据卷或纳管已有 PVC",
		Purpose:        aitool.Text("异步创建项目数据卷（支持 blank 空白卷、volumeSnapshot 快照恢复、existingClaim 纳管已有 PVC）；对 WaitForFirstConsumer 存储类处于 provisioning 即可绑定挂载发版触发首次消费。", "Asynchronously create a blank volume, restore from snapshot, or adopt an existing PVC."),
		AliasesZH:      []string{"创建数据卷", "新建 PVC", "纳管已有 PVC", "快照恢复数据卷"},
		AliasesEN:      []string{"create volume", "provision pvc", "adopt pvc"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RetryOnUnknown: true,
		RequiredScopes: []string{string(authz.ActionVolumeWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_get_volume",
				Family:          "resource",
				Expect:          "present",
				IDFrom:          "result.id",
				ReadIDArgument:  "volume",
				StatusField:     "lifecycleState",
				PendingStatuses: []string{"provisioning"},
				SuccessStatuses: []string{"ready"},
				FailureStatuses: []string{"error"},
			},
		},
		Preview: h.previewCreateVolumeAI,
		Execute: h.execCreateVolumeAI,
	})

	aitool.Register(reg, aitool.Spec[aiUpdateVolumeInput, map[string]any]{
		Name:           "runtime_update_volume",
		Domain:         aitool.DomainRuntime,
		Category:       "volume",
		Summary:        "重命名项目数据卷或异步在线扩容 PVC 容量",
		Purpose:        aitool.Text("重命名项目数据卷或在线扩大托管 PVC 容量（审批预检阶段自动锁定 expectedRevision 防止并发覆盖）。", "Rename a project volume or expand its PVC capacity online."),
		AliasesZH:      []string{"扩容数据卷", "重命名数据卷", "扩大 PVC 容量"},
		AliasesEN:      []string{"update volume", "expand pvc", "rename volume"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionVolumeWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_get_volume",
				Family:          "resource",
				Expect:          "present",
				IDFrom:          "result.id",
				ReadIDArgument:  "volume",
				StatusField:     "lifecycleState",
				PendingStatuses: []string{"provisioning"},
				SuccessStatuses: []string{"ready"},
				FailureStatuses: []string{"error"},
			},
		},
		Preview: h.previewUpdateVolumeAI,
		Execute: h.execUpdateVolumeAI,
	})

	aitool.Register(reg, aitool.Spec[aiRetryVolumeOperationInput, map[string]any]{
		Name:           "runtime_retry_volume_operation",
		Domain:         aitool.DomainRuntime,
		Category:       "volume",
		Summary:        "重试失败的数据卷创建（provision）、扩容（expand）或删除（delete）操作",
		Purpose:        aitool.Text("对处于 error 状态的数据卷重试上次失败的 provision、expand 或 delete 异步操作（不支持传输任务重试）。", "Retry a failed volume provision, expand, or delete operation."),
		AliasesZH:      []string{"重试数据卷操作", "重试失败 PVC 任务"},
		AliasesEN:      []string{"retry volume operation", "retry failed pvc"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionVolumeRead)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:      "runtime_get_volume",
				Family:           "resource",
				Expect:           "absent",
				WhenResultEquals: map[string]any{"pendingOperation": volume.OperationDelete},
				IDFrom:           "result.id",
				ReadIDArgument:   "volume",
				StatusField:      "lifecycleState",
				PendingStatuses:  []string{"deleting"},
				FailureStatuses:  []string{"error"},
			},
			{
				OperationID:     "runtime_get_volume",
				Family:          "resource",
				Expect:          "present",
				IDFrom:          "result.id",
				ReadIDArgument:  "volume",
				StatusField:     "lifecycleState",
				PendingStatuses: []string{"provisioning"},
				SuccessStatuses: []string{"ready"},
				FailureStatuses: []string{"error"},
			},
		},
		Preview: h.previewRetryVolumeOperationAI,
		Execute: h.execRetryVolumeOperationAI,
	})

	aitool.Register(reg, aitool.Spec[aiDeleteVolumeInput, map[string]any]{
		Name:           "runtime_delete_volume",
		Domain:         aitool.DomainRuntime,
		Category:       "volume",
		Summary:        "异步删除托管数据卷或解管引用数据卷",
		Purpose:        aitool.Text("预检无活跃挂载与运行中传输任务后，按纳管模式自动锁定物理删除（managed->delete）或仅解管保留 PVC（referenced->detach）并异步执行。", "Asynchronously delete a managed volume or detach a referenced PVC."),
		AliasesZH:      []string{"删除数据卷", "销毁 PVC", "解管数据卷"},
		AliasesEN:      []string{"delete volume", "detach pvc", "remove project volume"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionVolumeDelete)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_get_volume",
				Family:          "resource",
				Expect:          "absent",
				IDFrom:          "result.id",
				ReadIDArgument:  "volume",
				StatusField:     "lifecycleState",
				PendingStatuses: []string{"ready", "provisioning", "deleting"},
				FailureStatuses: []string{"error"},
			},
		},
		Preview: h.previewDeleteVolumeAI,
		Execute: h.execDeleteVolumeAI,
	})

	aitool.Register(reg, aitool.Spec[aiCreateVolumeExportInput, map[string]any]{
		Name:           "runtime_create_volume_export",
		Domain:         aitool.DomainRuntime,
		Category:       "volume",
		Summary:        "异步创建项目数据卷归档导出任务，完成后供用户在控制台下载",
		Purpose:        aitool.Text("为项目数据卷创建异步归档导出传输任务（支持自动推导 tar_gz / raw_zst 格式与快照/离线一致性策略），可通过 runtime_get_volume(project, transferId) 查看进度。", "Create an asynchronous archive export transfer for a project volume."),
		AliasesZH:      []string{"导出数据卷", "备份 PVC 数据", "创建卷归档导出"},
		AliasesEN:      []string{"export volume", "create volume backup", "archive pvc"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RetryOnUnknown: true,
		RequiredScopes: []string{string(authz.ActionVolumeExport)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_get_volume",
				Family:          "resource",
				Expect:          "present",
				IDFrom:          "result.transferId",
				ReadIDArgument:  "transferId",
				Collection:      true,
				CollectionPath:  "recentTransfers",
				TargetField:     "id",
				StatusField:     "state",
				PendingStatuses: []string{"created", "preparing", "ready", "streaming"},
				SuccessStatuses: []string{"succeeded"},
				FailureStatuses: []string{"failed", "expired", "cancelled"},
			},
		},
		Preview: h.previewCreateVolumeExportAI,
		Execute: h.execCreateVolumeExportAI,
	})

	aitool.Register(reg, aitool.Spec[aiCancelVolumeTransferInput, map[string]any]{
		Name:           "runtime_cancel_volume_transfer",
		Domain:         aitool.DomainRuntime,
		Category:       "volume",
		Summary:        "取消进行中的数据卷导入或导出传输任务",
		Purpose:        aitool.Text("取消尚未进入终态的数据卷导入或导出传输任务并触发临时传输资源清理。", "Cancel an in-progress volume import or export transfer."),
		AliasesZH:      []string{"取消数据卷传输", "终止卷导出任务", "取消卷导入"},
		AliasesEN:      []string{"cancel volume transfer", "abort volume export"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionVolumeRead)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_get_volume",
				Family:          "resource",
				Expect:          "canceled",
				IDFrom:          "argument.transferId",
				ReadIDArgument:  "transferId",
				Collection:      true,
				CollectionPath:  "recentTransfers",
				TargetField:     "id",
				StatusField:     "state",
				PendingStatuses: []string{"created", "preparing", "ready", "streaming"},
				SuccessStatuses: []string{"cancelled"},
				FailureStatuses: []string{"failed", "expired"},
			},
		},
		Preview: h.previewCancelVolumeTransferAI,
		Execute: h.execCancelVolumeTransferAI,
	})
}

func optionalTrimmed(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func structToMap(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "internal_error", err.Error())
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "internal_error", err.Error())
	}
	return out, nil
}

func (h *Handler) execListVolumesAI(tc *aitool.ToolContext, in aiListVolumesInput) (map[string]any, error) {
	tc.SetNoStore()
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeRead)
	if err != nil {
		return nil, err
	}
	if h.volumes == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, volume.CodeClusterUnavailable, "volume service is unavailable")
	}

	sortBy := "createdAt"
	if s := optionalTrimmed(in.SortBy); s != "" {
		sortBy = s
	}
	allowedSort := map[string]bool{"createdAt": true, "updatedAt": true, "displayName": true, "capacity": true}
	if !allowedSort[sortBy] {
		return nil, aitool.BadRequest(volume.CodePaginationSortByInvalid, "unsupported volume sort field")
	}
	sortOrder := "desc"
	if o := strings.ToLower(optionalTrimmed(in.SortOrder)); o != "" {
		sortOrder = o
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, aitool.BadRequest(volume.CodePaginationOrderInvalid, "unsupported volume sort order")
	}
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	var resolvedClusterID string
	var storageClasses []projectVolumeStorageClass
	var storageClassesStatus string
	if clusterRef := optionalTrimmed(in.Cluster); clusterRef != "" {
		cluster, cErr := h.resolveClusterForProjectVolumeAI(tc, user, project.ID, clusterRef)
		if cErr != nil {
			return nil, cErr
		}
		resolvedClusterID = cluster.ID
		storageClasses = []projectVolumeStorageClass{}
		storageClassesStatus = "unavailable"
		if h.volumeClusters != nil {
			if classes, scErr := h.volumeClusters.ListStorageClasses(tc.Context(), cluster.ID); scErr == nil {
				sort.SliceStable(classes, func(i, j int) bool { return classes[i].Name < classes[j].Name })
				if classes != nil {
					storageClasses = classes
				}
				storageClassesStatus = "ready"
			}
		}
	}

	result, listErr := h.volumes.ListProjectVolumes(tc.Context(), project.ID, volume.ProjectVolumeListOptions{
		Page:           page,
		PageSize:       pageSize,
		SortBy:         sortBy,
		SortOrder:      sortOrder,
		Search:         optionalTrimmed(in.Search),
		Availability:   optionalTrimmed(in.Availability),
		LifecycleState: optionalTrimmed(in.LifecycleState),
		ClusterID:      resolvedClusterID,
		SourceKind:     optionalTrimmed(in.SourceKind),
		OwnershipMode:  optionalTrimmed(in.OwnershipMode),
		VolumeMode:     optionalTrimmed(in.VolumeMode),
	})
	if listErr != nil {
		return nil, volumeDomainToToolError(listErr)
	}

	observations := h.observeProjectVolumeResponses(tc.Context(), result.Items)
	items := make([]projectVolumeResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, projectVolumeResponseForObservation(item, observations[item.ID]))
	}

	out := aitool.PageEnvelope(items, result.Page, result.PageSize, result.SortBy, result.SortOrder, result.Total)
	out["projectId"] = project.ID
	if resolvedClusterID != "" {
		out["clusterId"] = resolvedClusterID
		out["storageClasses"] = storageClasses
		out["storageClassesStatus"] = storageClassesStatus
	}
	return out, nil
}

func (h *Handler) userPrivilegedForVolumeTransfers(tc *aitool.ToolContext, user model.User, projectID string) (bool, error) {
	if authz.IsPlatformAdmin(user.Role) {
		return true, nil
	}
	sub, rec := newVolumeSubGin(tc)
	allowed, available := h.projectMemberActionAllowed(sub, projectID, user.ID, authz.ActionVolumeExport)
	if !available {
		return false, abortedVolumeToolError(rec, "project.authorization_unavailable", "project authorization is temporarily unavailable")
	}
	return allowed, nil
}

func (h *Handler) execGetVolumeAI(tc *aitool.ToolContext, in aiGetVolumeInput) (map[string]any, error) {
	tc.SetNoStore()
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeRead)
	if err != nil {
		return nil, err
	}
	if h.volumes == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, volume.CodeClusterUnavailable, "volume service is unavailable")
	}
	volRef := optionalTrimmed(in.Volume)
	txRef := optionalTrimmed(in.TransferID)
	if volRef == "" && txRef == "" {
		return nil, aitool.BadRequest("argument_invalid", "either volume or transferId must be specified")
	}

	var targetVolumeID string
	var matchedTransfer *model.VolumeTransfer
	if txRef != "" {
		txItem, txErr := h.volumes.GetVolumeTransfer(tc.Context(), project.ID, txRef)
		if txErr != nil {
			return nil, volumeDomainToToolError(txErr)
		}
		matchedTransfer = &txItem
		targetVolumeID = txItem.ProjectVolumeID
		if volRef != "" {
			resolvedVol, vErr := h.resolveProjectVolumeForAI(tc, project.ID, volRef)
			if vErr != nil {
				return nil, vErr
			}
			if resolvedVol.ID != targetVolumeID {
				return nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("transferId %s belongs to volume %s, not %s", txRef, targetVolumeID, resolvedVol.ID))
			}
		}
	} else {
		resolvedVol, vErr := h.resolveProjectVolumeForAI(tc, project.ID, volRef)
		if vErr != nil {
			return nil, vErr
		}
		targetVolumeID = resolvedVol.ID
	}

	detail, dErr := h.volumes.GetProjectVolumeDetailPage(tc.Context(), project.ID, targetVolumeID, 1, 50, 1, 50)
	if dErr != nil {
		return nil, volumeDomainToToolError(dErr)
	}
	privileged, pErr := h.userPrivilegedForVolumeTransfers(tc, user, project.ID)
	if pErr != nil {
		return nil, pErr
	}
	observations := h.observeProjectVolumeResponses(tc.Context(), []model.ProjectVolume{detail.Volume})
	resp := projectVolumeDetailResponseFor(detail, observations[detail.Volume.ID], privileged, user.ID)

	if matchedTransfer != nil {
		txResp := volumeTransferResponseFor(*matchedTransfer, privileged || matchedTransfer.ActorID == user.ID, h.volumeTransferMaxBytes)
		foundInRecent := false
		for i := range resp.RecentTransfers {
			if resp.RecentTransfers[i].ID == txResp.ID {
				resp.RecentTransfers[i] = txResp
				foundInRecent = true
				break
			}
		}
		if !foundInRecent {
			resp.RecentTransfers = append([]volumeTransferResponse{txResp}, resp.RecentTransfers...)
		}
		out, mErr := structToMap(resp)
		if mErr != nil {
			return nil, mErr
		}
		out["projectId"] = project.ID
		out["transfer"] = txResp
		return out, nil
	}

	out, mErr := structToMap(resp)
	if mErr != nil {
		return nil, mErr
	}
	out["projectId"] = project.ID
	return out, nil
}

type preparedVolumeCreate struct {
	user            model.User
	project         model.Project
	cluster         model.RuntimeCluster
	createInput     projectVolumeCreateInput
	resolvedStorage string
	domainPreview   volume.CreateProjectVolumeInput
}

func (h *Handler) prepareCreateVolumeForAI(
	tc *aitool.ToolContext,
	in aiCreateVolumeInput,
	lockedStorageClass string,
) (preparedVolumeCreate, error) {
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeWrite)
	if err != nil {
		return preparedVolumeCreate{}, err
	}
	if h.volumes == nil {
		return preparedVolumeCreate{}, aitool.NewError(http.StatusServiceUnavailable, volume.CodeClusterUnavailable, "volume service is unavailable")
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		return preparedVolumeCreate{}, aitool.BadRequest(volume.CodeInvalidInput, "displayName is required")
	}
	cluster, err := h.resolveClusterForProjectVolumeAI(tc, user, project.ID, in.Cluster)
	if err != nil {
		return preparedVolumeCreate{}, err
	}

	sourceType := strings.TrimSpace(in.SourceType)
	accessMode := optionalTrimmed(in.AccessMode)
	if accessMode == "" {
		accessMode = model.ProjectVolumeAccessReadWriteOnce
	}
	volumeMode := optionalTrimmed(in.VolumeMode)
	if volumeMode == "" {
		volumeMode = model.ProjectVolumeModeFilesystem
	}
	ownershipMode := optionalTrimmed(in.OwnershipMode)
	if ownershipMode == "" {
		ownershipMode = model.ProjectVolumeOwnershipManaged
	}
	storageClassName := optionalTrimmed(in.StorageClassName)
	if storageClassName == "" && lockedStorageClass != "" {
		storageClassName = lockedStorageClass
	}
	capacity := optionalTrimmed(in.Capacity)
	claimName := optionalTrimmed(in.ClaimName)
	snapshotName := optionalTrimmed(in.SnapshotName)

	switch sourceType {
	case "existingClaim":
		if claimName == "" {
			return preparedVolumeCreate{}, aitool.BadRequest(volume.CodeInvalidInput, "claimName is required when sourceType=existingClaim")
		}
		if ownershipMode != model.ProjectVolumeOwnershipManaged && ownershipMode != model.ProjectVolumeOwnershipReferenced {
			return preparedVolumeCreate{}, aitool.BadRequest(volume.CodeInvalidInput, "ownershipMode must be managed or referenced")
		}
	case "blank", "volumeSnapshot":
		if ownershipMode != model.ProjectVolumeOwnershipManaged {
			return preparedVolumeCreate{}, aitool.BadRequest(volume.CodeInvalidInput, "ownershipMode=referenced is only supported when sourceType=existingClaim")
		}
		if capacity == "" {
			return preparedVolumeCreate{}, aitool.BadRequest(volume.CodeInvalidInput, fmt.Sprintf("capacity is required when sourceType=%s", sourceType))
		}
		if _, _, valid := parseVolumeCapacity(capacity); !valid {
			return preparedVolumeCreate{}, aitool.BadRequest(volume.CodeInvalidInput, "capacity must be a positive Kubernetes resource quantity such as 10Gi")
		}
		if sourceType == "volumeSnapshot" && snapshotName == "" {
			return preparedVolumeCreate{}, aitool.BadRequest(volume.CodeInvalidInput, "snapshotName is required when sourceType=volumeSnapshot")
		}
		if storageClassName == "" {
			if h.volumeClusters == nil {
				return preparedVolumeCreate{}, aitool.NewError(http.StatusServiceUnavailable, volume.CodeClusterUnavailable, "cluster storage classes are unavailable")
			}
			classes, scErr := h.volumeClusters.ListStorageClasses(tc.Context(), cluster.ID)
			if scErr != nil {
				return preparedVolumeCreate{}, aitool.NewError(http.StatusServiceUnavailable, volume.CodeClusterUnavailable, "failed to inspect cluster storage classes")
			}
			var defaults []projectVolumeStorageClass
			candidates := make([]string, 0, len(classes))
			for _, sc := range classes {
				candidates = append(candidates, sc.Name)
				if sc.IsDefault {
					defaults = append(defaults, sc)
				}
			}
			sort.Strings(candidates)
			if len(defaults) != 1 {
				te := aitool.NewErrorWithHint(
					http.StatusBadRequest,
					"volume.storage_class_required",
					"cluster does not have a single default StorageClass; specify storageClassName explicitly",
					"Choose one StorageClass name from details.storageClasses and pass it in storageClassName.",
				)
				te.Details = map[string]any{"storageClasses": candidates}
				return preparedVolumeCreate{}, te
			}
			storageClassName = defaults[0].Name
		}
	default:
		return preparedVolumeCreate{}, aitool.BadRequest(volume.CodeInvalidInput, "sourceType must be blank, volumeSnapshot, or existingClaim")
	}

	createInput := projectVolumeCreateInput{
		DisplayName:      displayName,
		ClusterID:        cluster.ID,
		Capacity:         capacity,
		StorageClassName: storageClassName,
		AccessMode:       accessMode,
		VolumeMode:       volumeMode,
		Source: projectVolumeSourceInput{
			Type:          sourceType,
			ClaimName:     claimName,
			OwnershipMode: ownershipMode,
			SnapshotName:  snapshotName,
		},
	}
	sub, rec := newVolumeSubGin(tc)
	domainInput, ok := projectVolumeCreateDomainInput(sub, project, user, createInput, "preview-idem-placeholder")
	if !ok {
		return preparedVolumeCreate{}, abortedVolumeToolError(rec, volume.CodeInvalidInput, "volume creation parameters are invalid")
	}
	return preparedVolumeCreate{
		user:            user,
		project:         project,
		cluster:         cluster,
		createInput:     createInput,
		resolvedStorage: storageClassName,
		domainPreview:   domainInput,
	}, nil
}

func (h *Handler) previewCreateVolumeAI(tc *aitool.ToolContext, in aiCreateVolumeInput) (*aitool.PreviewResult, error) {
	prep, err := h.prepareCreateVolumeForAI(tc, in, "")
	if err != nil {
		return nil, err
	}
	if prep.domainPreview.OwnershipMode == model.ProjectVolumeOwnershipManaged {
		sub, rec := newVolumeSubGin(tc)
		if !h.ensureBillingAllowsManagedVolumeChange(sub, prep.project.ID) {
			return nil, abortedVolumeToolError(rec, "billing.insufficient_balance", "billing check blocked managed volume creation")
		}
	}
	changes := []aitool.PreviewChange{
		{Field: "displayName", After: prep.createInput.DisplayName, Action: "create"},
		{Field: "cluster", After: fmt.Sprintf("%s (%s)", prep.cluster.Name, prep.cluster.ID), Action: "create"},
		{Field: "sourceType", After: prep.createInput.Source.Type, Action: "create"},
		{Field: "ownershipMode", After: prep.domainPreview.OwnershipMode, Action: "create"},
	}
	if prep.domainPreview.CapacityRequest != "" {
		changes = append(changes, aitool.PreviewChange{Field: "capacity", After: prep.domainPreview.CapacityRequest, Action: "create"})
	}
	if prep.resolvedStorage != "" {
		changes = append(changes, aitool.PreviewChange{Field: "storageClassName", After: prep.resolvedStorage, Action: "create"})
	}
	if prep.createInput.Source.ClaimName != "" {
		changes = append(changes, aitool.PreviewChange{Field: "claimName", After: prep.createInput.Source.ClaimName, Action: "adopt"})
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("在项目 %s（集群 %s）中创建数据卷 %s", prep.project.Name, prep.cluster.Name, prep.createInput.DisplayName),
			TargetResource: "project_volume",
			ProjectID:      prep.project.ID,
			Changes:        changes,
		},
		LockedPayload: aiCreateVolumeLock{
			ClusterID:        prep.cluster.ID,
			StorageClassName: prep.resolvedStorage,
		},
	}, nil
}

func (h *Handler) execCreateVolumeAI(tc *aitool.ToolContext, in aiCreateVolumeInput) (map[string]any, error) {
	var lock aiCreateVolumeLock
	if len(tc.LockedPayload) > 0 {
		if err := tc.UnmarshalLockedPayload(&lock); err != nil {
			return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
		}
	}
	prep, err := h.prepareCreateVolumeForAI(tc, in, lock.StorageClassName)
	if err != nil {
		return nil, err
	}
	if lock.ClusterID != "" && lock.ClusterID != prep.cluster.ID {
		return nil, aitool.Conflict("ai.preview_lock_mismatch", "resolved cluster drifted since approval preview")
	}
	if prep.domainPreview.OwnershipMode == model.ProjectVolumeOwnershipManaged {
		sub, rec := newVolumeSubGin(tc)
		if !h.ensureBillingAllowsManagedVolumeChange(sub, prep.project.ID) {
			return nil, abortedVolumeToolError(rec, "billing.insufficient_balance", "billing check blocked managed volume creation")
		}
	}
	idemKey, err := effectiveVolumeIdempotencyKey(tc, "volcreate")
	if err != nil {
		return nil, err
	}
	domainInput := prep.domainPreview
	domainInput.IdempotencyKey = idemKey

	result, createErr := h.volumes.CreateProjectVolume(tc.Context(), domainInput)
	if createErr != nil {
		h.auditWithContext(prep.user.ID, projectVolumeCreateAuditAction(domainInput), prep.project.ID, false, volumeAuditErrorCode(createErr), tc.Context())
		return nil, volumeDomainToToolError(createErr)
	}
	h.auditWithContext(prep.user.ID, projectVolumeCreateAuditAction(domainInput), result.Volume.ID, true, domainInput.SourceKind, tc.Context())

	out, mErr := structToMap(projectVolumeResponseFor(result.Volume))
	if mErr != nil {
		return nil, mErr
	}
	out["projectId"] = prep.project.ID
	return out, nil
}

func (h *Handler) previewUpdateVolumeAI(tc *aitool.ToolContext, in aiUpdateVolumeInput) (*aitool.PreviewResult, error) {
	_, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeWrite)
	if err != nil {
		return nil, err
	}
	if in.DisplayName == nil && in.Capacity == nil {
		return nil, aitool.BadRequest(volume.CodeInvalidInput, "at least one of displayName or capacity must be specified")
	}
	if in.DisplayName != nil && strings.TrimSpace(*in.DisplayName) == "" {
		return nil, aitool.BadRequest(volume.CodeInvalidInput, "displayName must not be empty")
	}
	vol, err := h.resolveProjectVolumeForAI(tc, project.ID, in.Volume)
	if err != nil {
		return nil, err
	}
	if in.ExpectedRevision != nil && *in.ExpectedRevision != vol.Revision {
		return nil, aitool.Conflict(volume.CodeRevisionConflict, fmt.Sprintf("volume revision mismatch: expected %d, current %d", *in.ExpectedRevision, vol.Revision))
	}

	var changes []aitool.PreviewChange
	if in.DisplayName != nil && strings.TrimSpace(*in.DisplayName) != vol.DisplayName {
		changes = append(changes, aitool.PreviewChange{
			Field:  "displayName",
			Before: vol.DisplayName,
			After:  strings.TrimSpace(*in.DisplayName),
			Action: "rename",
		})
	}
	if in.Capacity != nil {
		sub, rec := newVolumeSubGin(tc)
		if !h.ensureBillingAllowsManagedVolumeChange(sub, project.ID) {
			return nil, abortedVolumeToolError(rec, "billing.insufficient_balance", "billing check blocked volume expansion")
		}
		normCap, capBytes, valid := parseVolumeCapacity(*in.Capacity)
		if !valid {
			return nil, aitool.BadRequest(volume.CodeInvalidInput, "volume capacity is invalid")
		}
		if capBytes <= vol.CapacityBytes {
			return nil, aitool.Conflict(volume.CodeCapacityShrinkForbidden,
				fmt.Sprintf("target capacity %s must be strictly greater than current capacity %s", normCap, vol.CapacityRequest))
		}
		if vol.OwnershipMode != model.ProjectVolumeOwnershipManaged {
			return nil, aitool.NewError(http.StatusUnprocessableEntity, volume.CodeExpansionUnsupported, "only managed project volumes can be expanded via platform API")
		}
		changes = append(changes, aitool.PreviewChange{
			Field:  "capacity",
			Before: vol.CapacityRequest,
			After:  normCap,
			Action: "expand",
		})
	}

	lockedRev := vol.Revision
	if in.ExpectedRevision != nil {
		lockedRev = *in.ExpectedRevision
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("更新项目数据卷 %s（%s，锁定 revision=%d）", vol.DisplayName, vol.ID, lockedRev),
			TargetResource: "project_volume",
			TargetID:       vol.ID,
			ProjectID:      project.ID,
			Changes:        changes,
		},
		LockedPayload: aiVolumeRevisionLock{
			VolumeID:         vol.ID,
			ExpectedRevision: lockedRev,
		},
	}, nil
}

func (h *Handler) execUpdateVolumeAI(tc *aitool.ToolContext, in aiUpdateVolumeInput) (map[string]any, error) {
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeWrite)
	if err != nil {
		return nil, err
	}
	if in.DisplayName == nil && in.Capacity == nil {
		return nil, aitool.BadRequest(volume.CodeInvalidInput, "at least one of displayName or capacity must be specified")
	}
	var lock aiVolumeRevisionLock
	if err := tc.UnmarshalLockedPayload(&lock); err != nil {
		if errors.Is(err, aitool.ErrPreviewLockMissing) {
			return nil, aitool.NewError(http.StatusPreconditionRequired, "ai.approval_required", "L2 volume update requires a signed preview lock")
		}
		return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
	}

	domainInput := volume.UpdateProjectVolumeInput{ActorID: user.ID}
	if in.DisplayName != nil {
		trimmed := strings.TrimSpace(*in.DisplayName)
		if trimmed == "" {
			return nil, aitool.BadRequest(volume.CodeInvalidInput, "displayName must not be empty")
		}
		domainInput.DisplayName = &trimmed
	}
	if in.Capacity != nil {
		sub, rec := newVolumeSubGin(tc)
		if !h.ensureBillingAllowsManagedVolumeChange(sub, project.ID) {
			return nil, abortedVolumeToolError(rec, "billing.insufficient_balance", "billing check blocked volume expansion")
		}
		normCap, capBytes, valid := parseVolumeCapacity(*in.Capacity)
		if !valid {
			return nil, aitool.BadRequest(volume.CodeInvalidInput, "volume capacity is invalid")
		}
		domainInput.CapacityRequest = &normCap
		domainInput.CapacityBytes = &capBytes
	}

	item, uErr := h.volumes.UpdateProjectVolume(tc.Context(), project.ID, lock.VolumeID, lock.ExpectedRevision, domainInput)
	if uErr != nil {
		h.auditWithContext(user.ID, "project_volume.update", lock.VolumeID, false, volumeAuditErrorCode(uErr), tc.Context())
		return nil, volumeDomainToToolError(uErr)
	}
	h.auditWithContext(user.ID, "project_volume.update", item.ID, true, "", tc.Context())
	out, mErr := structToMap(projectVolumeResponseFor(item))
	if mErr != nil {
		return nil, mErr
	}
	out["projectId"] = project.ID
	return out, nil
}

func (h *Handler) validateRetryVolumeForAI(
	tc *aitool.ToolContext,
	project model.Project,
	user model.User,
	current model.ProjectVolume,
	expectedRev *int64,
) (int64, error) {
	if current.LifecycleState != model.ProjectVolumeLifecycleError {
		return 0, aitool.Conflict(volume.CodeStateConflict, "only volumes in error lifecycleState can be retried")
	}
	retryAction, authorized := projectVolumeRetryAuthorization(current)
	if !authorized {
		return 0, aitool.Conflict(volume.CodeStateConflict, "project volume operation cannot be retried via runtime_retry_volume_operation")
	}
	if !authz.IsPlatformAdmin(user.Role) {
		sub, rec := newVolumeSubGin(tc)
		allowed, available := h.projectMemberActionAllowed(sub, project.ID, user.ID, retryAction)
		if !available {
			return 0, abortedVolumeToolError(rec, "project.authorization_unavailable", "project authorization is temporarily unavailable")
		}
		if !allowed {
			return 0, aitool.Forbidden("auth.forbidden", fmt.Sprintf("project role does not allow %s", retryAction))
		}
	}
	sub, rec := newVolumeSubGin(tc)
	if token, bearer := h.currentAccessTokenFromContext(sub); bearer && !h.accessTokenAllows(token.Scope, string(retryAction)) {
		return 0, aitool.Forbidden("auth.token.scope_insufficient", fmt.Sprintf("%s scope is required", retryAction))
	}
	if expectedRev != nil && *expectedRev != current.Revision {
		return 0, aitool.Conflict(volume.CodeRevisionConflict, fmt.Sprintf("volume revision mismatch: expected %d, current %d", *expectedRev, current.Revision))
	}
	if current.OwnershipMode == model.ProjectVolumeOwnershipManaged &&
		(current.PendingOperation == volume.OperationProvision || current.PendingOperation == volume.OperationExpand) {
		if !h.ensureBillingAllowsManagedVolumeChange(sub, project.ID) {
			return 0, abortedVolumeToolError(rec, "billing.insufficient_balance", "billing check blocked volume retry")
		}
	}
	lockedRev := current.Revision
	if expectedRev != nil {
		lockedRev = *expectedRev
	}
	return lockedRev, nil
}

func (h *Handler) previewRetryVolumeOperationAI(tc *aitool.ToolContext, in aiRetryVolumeOperationInput) (*aitool.PreviewResult, error) {
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeRead)
	if err != nil {
		return nil, err
	}
	current, err := h.resolveProjectVolumeForAI(tc, project.ID, in.Volume)
	if err != nil {
		return nil, err
	}
	lockedRev, err := h.validateRetryVolumeForAI(tc, project, user, current, in.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("重试数据卷 %s（%s）失败的 %s 操作（锁定 revision=%d）", current.DisplayName, current.ID, current.PendingOperation, lockedRev),
			TargetResource: "project_volume",
			TargetID:       current.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "pendingOperation", Before: current.PendingOperation, After: current.PendingOperation, Action: "retry"},
				{Field: "lifecycleState", Before: current.LifecycleState, Action: "retry"},
			},
		},
		LockedPayload: aiVolumeRevisionLock{
			VolumeID:         current.ID,
			ExpectedRevision: lockedRev,
		},
	}, nil
}

func (h *Handler) execRetryVolumeOperationAI(tc *aitool.ToolContext, in aiRetryVolumeOperationInput) (map[string]any, error) {
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeRead)
	if err != nil {
		return nil, err
	}
	var lock aiVolumeRevisionLock
	if err := tc.UnmarshalLockedPayload(&lock); err != nil {
		if errors.Is(err, aitool.ErrPreviewLockMissing) {
			return nil, aitool.NewError(http.StatusPreconditionRequired, "ai.approval_required", "L2 volume retry requires a signed preview lock")
		}
		return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
	}
	current, err := h.volumes.GetProjectVolume(tc.Context(), project.ID, lock.VolumeID)
	if err != nil {
		h.auditWithContext(user.ID, "project_volume.retry", lock.VolumeID, false, volumeAuditErrorCode(err), tc.Context())
		return nil, volumeDomainToToolError(err)
	}
	if _, err := h.validateRetryVolumeForAI(tc, project, user, current, &lock.ExpectedRevision); err != nil {
		return nil, err
	}
	item, rErr := h.volumes.RetryProjectVolumeOperation(tc.Context(), project.ID, lock.VolumeID, user.ID, lock.ExpectedRevision)
	if rErr != nil {
		h.auditWithContext(user.ID, "project_volume.retry", current.ID, false, volumeAuditErrorCode(rErr), tc.Context())
		return nil, volumeDomainToToolError(rErr)
	}
	h.auditWithContext(user.ID, "project_volume.retry", item.ID, true, item.PendingOperation, tc.Context())
	out, mErr := structToMap(projectVolumeResponseFor(item))
	if mErr != nil {
		return nil, mErr
	}
	out["projectId"] = project.ID
	return out, nil
}

func (h *Handler) previewDeleteVolumeAI(tc *aitool.ToolContext, in aiDeleteVolumeInput) (*aitool.PreviewResult, error) {
	_, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeDelete)
	if err != nil {
		return nil, err
	}
	vol, err := h.resolveProjectVolumeForAI(tc, project.ID, in.Volume)
	if err != nil {
		return nil, err
	}
	preview, pErr := h.volumes.PreviewProjectVolumeDeletion(tc.Context(), project.ID, vol.ID)
	if pErr != nil {
		return nil, volumeDomainToToolError(pErr)
	}
	if preview.BlockingBindingCount > 0 {
		bindings := make([]projectVolumeBindingResponse, 0, len(preview.BlockingBindings))
		for _, b := range preview.BlockingBindings {
			bindings = append(bindings, projectVolumeBindingResponseFor(b))
		}
		te := aitool.Conflict(volume.CodeInUse, "project volume still has active or reserved workload mounts")
		te.Hint = "Detach the volume from deployment targets via delivery_configure_target_volumes (and deploy if active) before deleting it."
		te.Details = map[string]any{"blockingBindings": bindings}
		return nil, te
	}
	if preview.BlockingTransferCount > 0 {
		transfers := make([]volumeTransferResponse, 0, len(preview.BlockingTransfers))
		for _, tx := range preview.BlockingTransfers {
			transfers = append(transfers, volumeTransferResponseFor(tx, true))
		}
		te := aitool.Conflict(volume.CodeTransferStateConflict, "project volume has in-progress import or export transfers")
		te.Hint = "Wait for or cancel running transfers via runtime_cancel_volume_transfer before deleting the volume."
		te.Details = map[string]any{"blockingTransfers": transfers}
		return nil, te
	}
	if !preview.CanDelete {
		return nil, aitool.Conflict(volume.CodeStateConflict, "project volume cannot be deleted in its current lifecycle state")
	}
	if in.ExpectedRevision != nil && *in.ExpectedRevision != preview.Volume.Revision {
		return nil, aitool.Conflict(volume.CodeRevisionConflict, fmt.Sprintf("volume revision mismatch: expected %d, current %d", *in.ExpectedRevision, preview.Volume.Revision))
	}

	dataAction := strings.TrimSpace(preview.RequiredDataAction)
	if dataAction == "" {
		if preview.Volume.OwnershipMode == model.ProjectVolumeOwnershipManaged {
			dataAction = "delete"
		} else {
			dataAction = "detach"
		}
	}
	willDeletePVC := preview.Volume.OwnershipMode == model.ProjectVolumeOwnershipManaged
	var warnings, impact []string
	if willDeletePVC {
		warnings = append(warnings, fmt.Sprintf("底层 Kubernetes PVC %s/%s 将被永久物理删除且不可恢复！", preview.Volume.Namespace, preview.Volume.ClaimName))
		impact = append(impact, "underlyingClaimWillBeDeleted=true (dataAction=delete)")
	} else {
		impact = append(impact, fmt.Sprintf("仅解除平台纳管引用（dataAction=detach），底层 PVC %s/%s 的数据将保留", preview.Volume.Namespace, preview.Volume.ClaimName))
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("删除/解管项目数据卷 %s（%s，dataAction=%s，underlyingClaimWillBeDeleted=%t）", preview.Volume.DisplayName, preview.Volume.ID, dataAction, willDeletePVC),
			TargetResource: "project_volume",
			TargetID:       preview.Volume.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "dataAction", After: dataAction, Action: dataAction},
				{Field: "underlyingClaimWillBeDeleted", After: willDeletePVC, Action: dataAction},
				{Field: "claimName", Before: preview.Volume.ClaimName, Action: dataAction},
			},
			Impact:   impact,
			Warnings: warnings,
		},
		LockedPayload: aiVolumeDeleteLock{
			VolumeID:         preview.Volume.ID,
			ExpectedRevision: preview.Volume.Revision,
			DataAction:       dataAction,
		},
	}, nil
}

func (h *Handler) execDeleteVolumeAI(tc *aitool.ToolContext, in aiDeleteVolumeInput) (map[string]any, error) {
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeDelete)
	if err != nil {
		return nil, err
	}
	var lock aiVolumeDeleteLock
	if err := tc.UnmarshalLockedPayload(&lock); err != nil {
		if errors.Is(err, aitool.ErrPreviewLockMissing) {
			return nil, aitool.NewError(http.StatusPreconditionRequired, "ai.approval_required", "L2 volume deletion requires a signed preview lock")
		}
		return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
	}

	item, detached, delErr := h.volumes.RequestDeleteProjectVolume(tc.Context(), volume.DeleteProjectVolumeInput{
		ProjectID:        project.ID,
		VolumeID:         lock.VolumeID,
		ActorID:          user.ID,
		ExpectedRevision: lock.ExpectedRevision,
		DataAction:       lock.DataAction,
	})
	if delErr != nil {
		h.auditWithContext(user.ID, "project_volume.delete", lock.VolumeID, false, volumeAuditErrorCode(delErr), tc.Context())
		return nil, volumeDomainToToolError(delErr)
	}
	auditAction := "project_volume.delete"
	if detached {
		auditAction = "project_volume.detach"
	}
	h.auditWithContext(user.ID, auditAction, item.ID, true, "", tc.Context())

	out, mErr := structToMap(projectVolumeResponseFor(item))
	if mErr != nil {
		return nil, mErr
	}
	out["projectId"] = project.ID
	out["dataAction"] = lock.DataAction
	out["detached"] = detached
	return out, nil
}

func resolveExportFormatAndConsistency(vol model.ProjectVolume, rawFormat, rawConsistency *string) (string, string, error) {
	defaultFormat := model.VolumeTransferFormatTarGZ
	if vol.VolumeMode == model.ProjectVolumeModeBlock {
		defaultFormat = model.VolumeTransferFormatRawZST
	}
	format := optionalTrimmed(rawFormat)
	if format == "" {
		format = defaultFormat
	}
	if format != model.VolumeTransferFormatTarGZ && format != model.VolumeTransferFormatRawZST {
		return "", "", aitool.BadRequest(volume.CodeInvalidInput, "format must be tar_gz or raw_zst")
	}
	if format != defaultFormat {
		return "", "", aitool.Conflict(volume.CodeTransferFormatMismatch,
			fmt.Sprintf("volumeMode %s requires export format %s", vol.VolumeMode, defaultFormat))
	}

	consistency := optionalTrimmed(rawConsistency)
	if consistency == "" {
		consistency = "auto"
	}
	switch consistency {
	case "auto", "snapshot", "live", "unmounted":
	default:
		return "", "", aitool.BadRequest(volume.CodeInvalidInput, "consistency must be auto, snapshot, live, or unmounted")
	}
	if vol.VolumeMode == model.ProjectVolumeModeBlock && consistency == "live" {
		return "", "", aitool.BadRequest(volume.CodeInvalidInput, "Block volumes do not support consistency=live")
	}
	return format, consistency, nil
}

func (h *Handler) previewCreateVolumeExportAI(tc *aitool.ToolContext, in aiCreateVolumeExportInput) (*aitool.PreviewResult, error) {
	_, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeExport)
	if err != nil {
		return nil, err
	}
	sub, rec := newVolumeSubGin(tc)
	if !h.ensureBillingAllowsDeployChange(sub, project.ID) {
		return nil, abortedVolumeToolError(rec, "billing.insufficient_balance", "billing check blocked volume export")
	}
	if !h.volumeTransferEnabled || h.volumeContent == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, volume.CodeTransferUnavailable, "direct volume transfer is not configured")
	}
	vol, err := h.resolveProjectVolumeForAI(tc, project.ID, in.Volume)
	if err != nil {
		return nil, err
	}
	format, consistency, err := resolveExportFormatAndConsistency(vol, in.Format, in.Consistency)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("异步创建数据卷 %s（%s）归档导出任务（format=%s, consistency=%s）", vol.DisplayName, vol.ID, format, consistency),
			TargetResource: "volume_transfer",
			TargetID:       vol.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "volume", After: fmt.Sprintf("%s (%s)", vol.DisplayName, vol.ID), Action: "export"},
				{Field: "format", After: format, Action: "export"},
				{Field: "consistency", After: consistency, Action: "export"},
			},
		},
	}, nil
}

func (h *Handler) execCreateVolumeExportAI(tc *aitool.ToolContext, in aiCreateVolumeExportInput) (map[string]any, error) {
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeExport)
	if err != nil {
		return nil, err
	}
	sub, rec := newVolumeSubGin(tc)
	if !h.ensureBillingAllowsDeployChange(sub, project.ID) {
		return nil, abortedVolumeToolError(rec, "billing.insufficient_balance", "billing check blocked volume export")
	}
	if !h.volumeTransferEnabled || h.volumeContent == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, volume.CodeTransferUnavailable, "direct volume transfer is not configured")
	}
	vol, err := h.resolveProjectVolumeForAI(tc, project.ID, in.Volume)
	if err != nil {
		return nil, err
	}
	format, consistency, err := resolveExportFormatAndConsistency(vol, in.Format, in.Consistency)
	if err != nil {
		return nil, err
	}
	idemKey, err := effectiveVolumeIdempotencyKey(tc, "volexport")
	if err != nil {
		return nil, err
	}
	transfer, expErr := h.volumeContent.CreateExport(tc.Context(), user, project, vol.ID, volumeExportCreateInput{
		Format:      format,
		Consistency: consistency,
	}, idemKey)
	if expErr != nil {
		h.auditWithContext(user.ID, "volume_transfer.export_start", vol.ID, false, volumeAuditErrorCode(expErr), tc.Context())
		return nil, volumeDomainToToolError(expErr)
	}
	h.auditWithContext(user.ID, "volume_transfer.export_start", transfer.ID, true, transfer.Format, tc.Context())

	resp := volumeTransferResponseFor(transfer, true, h.volumeTransferMaxBytes)
	out, mErr := structToMap(resp)
	if mErr != nil {
		return nil, mErr
	}
	out["projectId"] = project.ID
	out["transferId"] = transfer.ID
	return out, nil
}

func (h *Handler) authorizeCancelTransferAI(tc *aitool.ToolContext, user model.User, project model.Project, transfer model.VolumeTransfer) error {
	if transfer.ActorID == user.ID {
		return nil
	}
	if !authz.IsPlatformAdmin(user.Role) {
		sub, rec := newVolumeSubGin(tc)
		allowed, available := h.projectMemberActionAllowed(sub, project.ID, user.ID, authz.ActionVolumeExport)
		if !available {
			return abortedVolumeToolError(rec, "project.authorization_unavailable", "project authorization is temporarily unavailable")
		}
		if !allowed {
			return aitool.Forbidden("auth.forbidden", "only the creator or a project Owner/Admin can cancel this transfer")
		}
	}
	sub, _ := newVolumeSubGin(tc)
	if token, bearer := h.currentAccessTokenFromContext(sub); bearer && !h.accessTokenAllows(token.Scope, string(authz.ActionVolumeDelete)) {
		return aitool.Forbidden("auth.token.scope_insufficient", "volume:delete scope is required to cancel another user's transfer")
	}
	return nil
}

func (h *Handler) previewCancelVolumeTransferAI(tc *aitool.ToolContext, in aiCancelVolumeTransferInput) (*aitool.PreviewResult, error) {
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeRead)
	if err != nil {
		return nil, err
	}
	if h.volumes == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, volume.CodeClusterUnavailable, "volume service is unavailable")
	}
	txID := strings.TrimSpace(in.TransferID)
	if txID == "" {
		return nil, aitool.BadRequest(volume.CodeInvalidInput, "transferId is required")
	}
	transfer, tErr := h.volumes.GetVolumeTransfer(tc.Context(), project.ID, txID)
	if tErr != nil {
		return nil, volumeDomainToToolError(tErr)
	}
	if err := h.authorizeCancelTransferAI(tc, user, project, transfer); err != nil {
		return nil, err
	}
	if transfer.State == model.VolumeTransferStateSucceeded ||
		transfer.State == model.VolumeTransferStateFailed ||
		transfer.State == model.VolumeTransferStateExpired {
		return nil, aitool.Conflict(volume.CodeTransferStateConflict, fmt.Sprintf("transfer %s is already in terminal state %s", transfer.ID, transfer.State))
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("取消数据卷传输任务 %s（direction=%s, currentState=%s）", transfer.ID, transfer.Direction, transfer.State),
			TargetResource: "volume_transfer",
			TargetID:       transfer.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "state", Before: transfer.State, After: model.VolumeTransferStateCancelled, Action: "cancel"},
			},
		},
	}, nil
}

func (h *Handler) execCancelVolumeTransferAI(tc *aitool.ToolContext, in aiCancelVolumeTransferInput) (map[string]any, error) {
	user, project, err := h.authorizeAIVolumeProject(tc, in.Project, authz.ActionVolumeRead)
	if err != nil {
		return nil, err
	}
	if h.volumes == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, volume.CodeClusterUnavailable, "volume service is unavailable")
	}
	txID := strings.TrimSpace(in.TransferID)
	if txID == "" {
		return nil, aitool.BadRequest(volume.CodeInvalidInput, "transferId is required")
	}
	transfer, tErr := h.volumes.GetVolumeTransfer(tc.Context(), project.ID, txID)
	if tErr != nil {
		return nil, volumeDomainToToolError(tErr)
	}
	if err := h.authorizeCancelTransferAI(tc, user, project, transfer); err != nil {
		return nil, err
	}
	cancelled := transfer
	if transfer.State != model.VolumeTransferStateCancelled {
		var cErr error
		cancelled, cErr = h.volumes.TransitionVolumeTransfer(tc.Context(), project.ID, transfer.ID, model.VolumeTransferStateCancelled, "", "")
		if cErr != nil {
			h.auditWithContext(user.ID, "volume_transfer.cancel", transfer.ID, false, volumeAuditErrorCode(cErr), tc.Context())
			return nil, volumeDomainToToolError(cErr)
		}
		h.auditWithContext(user.ID, "volume_transfer.cancel", transfer.ID, true, transfer.Direction, tc.Context())
	}
	resp := volumeTransferResponseFor(cancelled, true, h.volumeTransferMaxBytes)
	out, mErr := structToMap(resp)
	if mErr != nil {
		return nil, mErr
	}
	out["projectId"] = project.ID
	out["transferId"] = cancelled.ID
	return out, nil
}
