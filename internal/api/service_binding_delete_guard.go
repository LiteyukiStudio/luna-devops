package api

import (
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
)

type serviceBindingUsage struct {
	BindingID             string `json:"bindingId"`
	SourceApplicationID   string `json:"sourceApplicationId"`
	SourceApplicationName string `json:"sourceApplicationName"`
	SourceTargetID        string `json:"sourceDeploymentTargetId"`
	SourceTargetName      string `json:"sourceDeploymentTargetName"`
}

func (h *Handlers) ensureNoIncomingServiceBindings(ctx *gin.Context, projectID, targetApplicationID, targetDeploymentTargetID string) bool {
	query := h.dbFor(ctx).Table("service_bindings AS binding").
		Select(`binding.id AS binding_id,
                binding.source_application_id,
                source_application.name AS source_application_name,
                binding.source_deployment_target_id AS source_target_id,
                source_target.name AS source_target_name`).
		Joins("JOIN applications AS source_application ON source_application.id = binding.source_application_id").
		Joins("JOIN deployment_targets AS source_target ON source_target.id = binding.source_deployment_target_id").
		Where("binding.project_id = ? AND binding.enabled = ?", projectID, true)
	if targetDeploymentTargetID != "" {
		query = query.Where("binding.target_deployment_target_id = ?", targetDeploymentTargetID)
	} else {
		query = query.Where("binding.target_application_id = ?", targetApplicationID)
	}

	var usages []serviceBindingUsage
	if err := query.Order("source_application.name ASC, source_target.name ASC").Limit(100).Scan(&usages).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if len(usages) == 0 {
		return true
	}
	writeServiceBindingInUse(ctx, usages)
	return false
}

func writeServiceBindingInUse(ctx *gin.Context, usages []serviceBindingUsage) {
	const code = "service_binding_in_use"
	telemetry.SetHTTPError(ctx, code, code)
	response := errorEnvelope(ctx, http.StatusConflict, code)
	if isDevelopmentRequest(ctx) {
		response["affectedSources"] = usages
	}
	ctx.JSON(http.StatusConflict, response)
}
