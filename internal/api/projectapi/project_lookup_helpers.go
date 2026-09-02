package projectapi

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handler) projectIDsForUser(ctx context.Context, userID string) []string {
	return h.host.ProjectIDsForUser(ctx, userID)
}

func (h *Handler) findProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	_, project, ok := h.authorizeProjectByID(ctx, projectID, authz.ActionProjectRead)
	return project, ok
}

func replaceParam(params gin.Params, key, value string) gin.Params {
	result := gin.Params{}
	for _, param := range params {
		if param.Key == key {
			continue
		}
		result = append(result, param)
	}
	if value != "" {
		result = append(result, gin.Param{Key: key, Value: value})
	}
	return result
}
