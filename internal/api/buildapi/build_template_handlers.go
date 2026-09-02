package buildapi

import (
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/gin-gonic/gin"
)

type buildTemplatePreviewInput struct {
	Version string            `json:"version"`
	Values  map[string]string `json:"values"`
}

func (h *Handlers) ListBuildTemplates(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}
	ctx.JSON(http.StatusOK, buildtemplate.List())
}

func (h *Handlers) PreviewBuildTemplate(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}
	var input buildTemplatePreviewInput
	if !bindJSON(ctx, &input) {
		return
	}
	preview, err := buildtemplate.Render(ctx.Param("templateId"), input.Version, input.Values)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "build_template.invalid", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, preview)
}
