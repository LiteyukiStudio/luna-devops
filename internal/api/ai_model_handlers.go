package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type aiModelInput struct {
	Name                          string `json:"name"`
	InputCreditsPerMillion        string `json:"inputCreditsPerMillion"`
	OutputCreditsPerMillion       string `json:"outputCreditsPerMillion"`
	CachedInputCreditsPerMillion  string `json:"cachedInputCreditsPerMillion"`
	CachedOutputCreditsPerMillion string `json:"cachedOutputCreditsPerMillion"`
	Enabled                       *bool  `json:"enabled"`
}

type aiModelPublic struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (h *Handlers) ListAIModels(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}
	var models []model.AIModel
	if err := h.dbFor(ctx).Where("enabled = ?", true).Order("name asc").Find(&models).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.models_unavailable", "AI models are unavailable")
		return
	}
	items := make([]aiModelPublic, 0, len(models))
	for _, item := range models {
		items = append(items, aiModelPublic{ID: item.ID, Name: item.Name})
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, items)
}

func (h *Handlers) ListAIModelConfigs(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var models []model.AIModel
	if err := h.dbFor(ctx).Order("name asc").Find(&models).Error; err != nil {
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
	var input aiModelInput
	if !bindJSON(ctx, &input) {
		h.auditWithContext(user.ID, "ai.model.create", "ai.models", false, "invalid request", ctx.Request.Context())
		return
	}
	item, err := parseAIModelInput(input, true)
	if err != nil {
		h.auditWithContext(user.ID, "ai.model.create", strings.TrimSpace(input.Name), false, err.Error(), ctx.Request.Context())
		writeAIModelInputError(ctx, err)
		return
	}
	item.ID = id.New("aimod")
	if err := h.dbFor(ctx).Create(&item).Error; err != nil {
		h.auditWithContext(user.ID, "ai.model.create", item.ID, false, "model create failed", ctx.Request.Context())
		if isAIModelNameConflict(err) {
			writeErrorCode(ctx, http.StatusConflict, "ai.model_name_conflict", "AI model name is already in use")
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
	var input aiModelInput
	if !bindJSON(ctx, &input) {
		h.auditWithContext(user.ID, "ai.model.update", modelID, false, "invalid request", ctx.Request.Context())
		return
	}
	item, err := parseAIModelInput(input, false)
	if err != nil {
		h.auditWithContext(user.ID, "ai.model.update", modelID, false, err.Error(), ctx.Request.Context())
		writeAIModelInputError(ctx, err)
		return
	}
	var updated model.AIModel
	err = h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.AIModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", modelID).Error; err != nil {
			return err
		}
		if item.Name != "" {
			current.Name = item.Name
		}
		if item.InputCreditsPerMillion.IsPositive() || input.InputCreditsPerMillion != "" {
			current.InputCreditsPerMillion = item.InputCreditsPerMillion
		}
		if item.OutputCreditsPerMillion.IsPositive() || input.OutputCreditsPerMillion != "" {
			current.OutputCreditsPerMillion = item.OutputCreditsPerMillion
		}
		if item.CachedInputCreditsPerMillion.IsPositive() || input.CachedInputCreditsPerMillion != "" {
			current.CachedInputCreditsPerMillion = item.CachedInputCreditsPerMillion
		}
		if item.CachedOutputCreditsPerMillion.IsPositive() || input.CachedOutputCreditsPerMillion != "" {
			current.CachedOutputCreditsPerMillion = item.CachedOutputCreditsPerMillion
		}
		if input.Enabled != nil && current.Enabled && !*input.Enabled {
			var enabledCount int64
			if err := tx.Model(&model.AIModel{}).Where("enabled = ? AND id <> ?", true, current.ID).Count(&enabledCount).Error; err != nil {
				return err
			}
			if enabledCount == 0 {
				return errAIModelLastEnabled
			}
		}
		if input.Enabled != nil {
			current.Enabled = *input.Enabled
		}
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		updated = current
		return nil
	})
	if err != nil {
		h.auditWithContext(user.ID, "ai.model.update", modelID, false, err.Error(), ctx.Request.Context())
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			writeErrorCode(ctx, http.StatusNotFound, "ai.model_not_found", "AI model was not found")
		case errors.Is(err, errAIModelLastEnabled):
			writeErrorCode(ctx, http.StatusConflict, "ai.last_model_cannot_be_disabled", "at least one AI model must remain enabled")
		case isAIModelNameConflict(err):
			writeErrorCode(ctx, http.StatusConflict, "ai.model_name_conflict", "AI model name is already in use")
		default:
			writeErrorCode(ctx, http.StatusInternalServerError, "ai.model_write_failed", "AI model could not be saved")
		}
		return
	}
	h.auditWithContext(user.ID, "ai.model.update", modelID, true, "AI model updated", ctx.Request.Context())
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, updated)
}

var errAIModelLastEnabled = errors.New("ai.last_model_cannot_be_disabled")

func parseAIModelInput(input aiModelInput, create bool) (model.AIModel, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return model.AIModel{}, errors.New("ai.model_name_required")
	}
	result := model.AIModel{Name: name, Enabled: true}
	for _, field := range []struct {
		raw  string
		out  *decimal.Decimal
		code string
	}{
		{input.InputCreditsPerMillion, &result.InputCreditsPerMillion, "ai.model_input_price_invalid"},
		{input.OutputCreditsPerMillion, &result.OutputCreditsPerMillion, "ai.model_output_price_invalid"},
		{input.CachedInputCreditsPerMillion, &result.CachedInputCreditsPerMillion, "ai.model_cached_input_price_invalid"},
		{input.CachedOutputCreditsPerMillion, &result.CachedOutputCreditsPerMillion, "ai.model_cached_output_price_invalid"},
	} {
		if strings.TrimSpace(field.raw) == "" {
			if create {
				return model.AIModel{}, errors.New(field.code)
			}
			continue
		}
		value, err := decimal.NewFromString(strings.TrimSpace(field.raw))
		if err != nil || value.IsNegative() {
			return model.AIModel{}, errors.New(field.code)
		}
		*field.out = value
	}
	if input.Enabled != nil {
		result.Enabled = *input.Enabled
	}
	return result, nil
}

func writeAIModelInputError(ctx *gin.Context, err error) {
	code := err.Error()
	status := http.StatusBadRequest
	writeErrorCode(ctx, status, code, "invalid AI model configuration")
}

func isAIModelNameConflict(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "ai_models_name_key") ||
		(strings.Contains(message, "duplicate key") && strings.Contains(message, "ai_models"))
}
