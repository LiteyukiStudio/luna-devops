package projectapi

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/dependency"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetProjectTopology(ctx *gin.Context) {
	if _, _, ok := h.authorizeProject(ctx, authz.ActionProjectRead); !ok {
		return
	}
	topology, err := h.dependencyService(ctx.Request.Context()).ProjectTopology(ctx.Request.Context(), ctx.Param("projectId"), dependency.TopologyFilter{
		Stage: strings.TrimSpace(ctx.Query("stage")), ApplicationID: strings.TrimSpace(ctx.Query("applicationId")), Origins: topologyOrigins(ctx.Query("origins")),
	})
	if err != nil {
		writeDependencyError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, topology)
}

func topologyOrigins(raw string) map[string]bool {
	origins := map[string]bool{}
	for _, value := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "service_binding":
			origins["service_binding"] = true
		case "manual":
			origins["manual"] = true
		}
	}
	return origins
}
