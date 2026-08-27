package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aimodel"
	"github.com/gin-gonic/gin"
)

type aiModelPublic struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	MaxContextTokens int64  `json:"maxContextTokens"`
	MaxOutputTokens  int64  `json:"maxOutputTokens"`
}

func (h *Handlers) ListAIModels(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}
	models, err := aimodel.NewService(h.dbFor(ctx)).ListEnabled(requestContext(ctx))
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.models_unavailable", "AI models are unavailable")
		return
	}
	items := make([]aiModelPublic, 0, len(models))
	for _, item := range models {
		items = append(items, aiModelPublic{ID: item.ID, Name: item.Name, MaxContextTokens: item.MaxContextTokens, MaxOutputTokens: item.MaxOutputTokens})
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, items)
}

func (h *Handlers) ListAIModelConfigs(ctx *gin.Context) {
	models, err := aimodel.NewService(h.dbFor(ctx)).ListAll(requestContext(ctx))
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.models_unavailable", "AI models are unavailable")
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, models)
}

func (h *Handlers) CreateAIModel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input aimodel.WriteInput
	if !bindJSON(ctx, &input) {
		h.auditWithContext(user.ID, "ai.model.create", "ai.models", false, "invalid request", ctx.Request.Context())
		return
	}
	item, err := aimodel.NewService(h.dbFor(ctx)).Create(requestContext(ctx), input)
	if err != nil {
		auditCode := aimodel.ErrorCode(err)
		if auditCode == "" {
			auditCode = "ai.model_write_failed"
		}
		h.auditWithContext(user.ID, "ai.model.create", strings.TrimSpace(input.Name), false, auditCode, ctx.Request.Context())
		if errors.Is(err, aimodel.ErrNameConflict) {
			writeErrorCode(ctx, http.StatusConflict, "ai.model_name_conflict", "AI model name is already in use")
			return
		}
		if code := aimodel.ErrorCode(err); code != "" {
			writeErrorCode(ctx, http.StatusBadRequest, code, "invalid AI model configuration")
			return
		}
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.model_write_failed", "AI model could not be saved")
		return
	}
	h.auditWithContext(user.ID, "ai.model.create", item.ID, true, "AI model created", ctx.Request.Context())
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusCreated, item)
}

func (h *Handlers) UpdateAIModel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	modelID := strings.TrimSpace(ctx.Param("id"))
	var input aimodel.WriteInput
	if !bindJSON(ctx, &input) {
		h.auditWithContext(user.ID, "ai.model.update", modelID, false, "invalid request", ctx.Request.Context())
		return
	}
	updated, err := aimodel.NewService(h.dbFor(ctx)).Update(requestContext(ctx), modelID, input)
	if err != nil {
		auditCode := aimodel.ErrorCode(err)
		if auditCode == "" {
			auditCode = "ai.model_write_failed"
		}
		h.auditWithContext(user.ID, "ai.model.update", modelID, false, auditCode, ctx.Request.Context())
		switch {
		case errors.Is(err, aimodel.ErrNotFound):
			writeErrorCode(ctx, http.StatusNotFound, "ai.model_not_found", "AI model was not found")
		case errors.Is(err, aimodel.ErrLastEnabled):
			writeErrorCode(ctx, http.StatusConflict, "ai.last_model_cannot_be_disabled", "at least one AI model must remain enabled")
		case errors.Is(err, aimodel.ErrNameConflict):
			writeErrorCode(ctx, http.StatusConflict, "ai.model_name_conflict", "AI model name is already in use")
		case aimodel.ErrorCode(err) != "":
			writeErrorCode(ctx, http.StatusBadRequest, aimodel.ErrorCode(err), "invalid AI model configuration")
		default:
			writeErrorCode(ctx, http.StatusInternalServerError, "ai.model_write_failed", "AI model could not be saved")
		}
		return
	}
	h.auditWithContext(user.ID, "ai.model.update", modelID, true, "AI model updated", ctx.Request.Context())
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, updated)
}

func (h *Handlers) DeleteAIModel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	modelID := strings.TrimSpace(ctx.Param("id"))
	deleted, err := aimodel.NewService(h.dbFor(ctx)).Delete(requestContext(ctx), modelID)
	if err != nil {
		auditCode := aimodel.ErrorCode(err)
		if auditCode == "" {
			auditCode = "ai.model_delete_failed"
		}
		h.auditWithContext(user.ID, "ai.model.delete", modelID, false, auditCode, ctx.Request.Context())
		switch {
		case errors.Is(err, aimodel.ErrNotFound):
			writeErrorCode(ctx, http.StatusNotFound, "ai.model_not_found", "AI model was not found")
		case errors.Is(err, aimodel.ErrLastEnabled):
			writeErrorCode(ctx, http.StatusConflict, "ai.last_model_cannot_be_deleted", "at least one enabled AI model must remain")
		default:
			writeErrorCode(ctx, http.StatusInternalServerError, "ai.model_delete_failed", "AI model could not be deleted")
		}
		return
	}
	h.auditWithContext(user.ID, "ai.model.delete", deleted.ID, true, "AI model deleted", ctx.Request.Context())
	ctx.Header("Cache-Control", "no-store")
	ctx.Status(http.StatusNoContent)
}
