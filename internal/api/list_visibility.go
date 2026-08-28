package api

import (
	"errors"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
)

func resolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	visibility, err := projectservice.ResolveListVisibility(ctx.Query("visibility"), authz.IsPlatformAdmin(user.Role))
	switch {
	case errors.Is(err, projectservice.ErrListVisibilityInvalid):
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", err.Error())
		return "", false
	case errors.Is(err, projectservice.ErrListVisibilityForbidden):
		writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", err.Error())
		return "", false
	case err != nil:
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return "", false
	default:
		return visibility, true
	}
}
