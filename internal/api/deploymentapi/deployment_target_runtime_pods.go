package deploymentapi

import (
	"net/http"
	"slices"
	"strings"
	"time"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
)

// recommended 为平台默认选择顺序：指定发布优先，其次就绪、运行中、创建时间从新到旧；desc 表示优先级高的在前。
var deploymentTargetRuntimePodSortFields = map[string]string{
	"recommended": "recommended",
	"name":        "name",
	"createdAt":   "createdAt",
}

type deploymentTargetRuntimePodsResponse struct {
	transportapi.PaginatedResponseBody[kubeprovider.RuntimePodSnapshot]
	observation.Result
}

func (h *Handlers) ListDeploymentTargetRuntimePods(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	_, project, ok := h.authorizeProject(ctx, authz.ActionDeploymentRead)
	if !ok {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	var target model.DeploymentTarget
	if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), project.ID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return
	}
	pagination := paginationFromQueryWithSort(ctx, deploymentTargetRuntimePodSortFields, "recommended")
	client, namespace, _, ok := h.runtimeClientForDeploymentTarget(ctx, project, target)
	if !ok {
		return
	}
	pods, err := client.RuntimePods(ctx.Request.Context(), namespace, target.ID, strings.TrimSpace(ctx.Query("releaseId")))
	if err != nil {
		ctx.JSON(http.StatusOK, deploymentTargetRuntimePodsResponse{
			PaginatedResponseBody: paginatedResponse([]kubeprovider.RuntimePodSnapshot{}, 0, pagination),
			Result:                observation.Result{Status: observation.StatusUnavailable, Code: "deployment_target.runtime_pods_unavailable", ObservedAt: time.Now()},
		})
		return
	}
	pods = filterRuntimePods(pods, ctx.Query("search"))
	sortRuntimePodSnapshots(pods, pagination)
	ctx.JSON(http.StatusOK, deploymentTargetRuntimePodsResponse{
		PaginatedResponseBody: paginatedResponse(paginateSlice(pods, pagination), int64(len(pods)), pagination),
		Result:                observation.Result{Status: observation.StatusReady, Code: "deployment_target.runtime_pods_ready", ObservedAt: time.Now()},
	})
}

func filterRuntimePods(pods []kubeprovider.RuntimePodSnapshot, search string) []kubeprovider.RuntimePodSnapshot {
	keyword := strings.ToLower(strings.TrimSpace(search))
	if keyword == "" {
		return pods
	}
	return slices.DeleteFunc(pods, func(pod kubeprovider.RuntimePodSnapshot) bool {
		return !strings.Contains(strings.ToLower(pod.Name), keyword)
	})
}

// sortRuntimePodSnapshots 在 Provider 推荐顺序之上应用白名单排序；推荐顺序升序即整体反转。
func sortRuntimePodSnapshots(pods []kubeprovider.RuntimePodSnapshot, pagination paginationParams) {
	switch pagination.SortBy {
	case "name":
		slices.SortStableFunc(pods, func(left, right kubeprovider.RuntimePodSnapshot) int {
			return strings.Compare(left.Name, right.Name)
		})
	case "createdAt":
		slices.SortStableFunc(pods, func(left, right kubeprovider.RuntimePodSnapshot) int {
			return left.CreatedAt.Compare(right.CreatedAt)
		})
	default:
		if pagination.SortOrder == "asc" {
			slices.Reverse(pods)
		}
		return
	}
	if pagination.SortOrder == "desc" {
		slices.Reverse(pods)
	}
}
