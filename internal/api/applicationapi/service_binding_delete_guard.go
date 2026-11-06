package applicationapi

import (
	"errors"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type serviceBindingUsage struct {
	BindingID             string `json:"bindingId"`
	SourceApplicationID   string `json:"sourceApplicationId"`
	SourceApplicationName string `json:"sourceApplicationName"`
	SourceTargetID        string `json:"sourceDeploymentTargetId"`
	SourceTargetName      string `json:"sourceDeploymentTargetName"`
}

// serviceBindingInUseError 携带删除事务内发现的入站服务关系，事务外据此写 service_binding_in_use 响应。
type serviceBindingInUseError struct {
	usages []serviceBindingUsage
}

func (err *serviceBindingInUseError) Error() string {
	return "target is referenced by active service bindings"
}

// incomingServiceBindingUsages 列出指向目标的启用服务关系。按应用查询时排除同一应用内部部署配置之间的关系，它们随应用一起删除。
func incomingServiceBindingUsages(db *gorm.DB, projectID, targetApplicationID, targetDeploymentTargetID string) ([]serviceBindingUsage, error) {
	query := db.Table("service_bindings AS binding").
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
		query = query.Where("binding.target_application_id = ? AND binding.source_application_id <> ?", targetApplicationID, targetApplicationID)
	}

	var usages []serviceBindingUsage
	if err := query.Order("source_application.name ASC, source_target.name ASC").Limit(100).Scan(&usages).Error; err != nil {
		return nil, err
	}
	return usages, nil
}

// ensureNoIncomingServiceBindingsTx 在调用方已锁定目标行的删除事务内检查入站服务关系，冲突时返回 serviceBindingInUseError。
func ensureNoIncomingServiceBindingsTx(tx *gorm.DB, projectID, targetApplicationID, targetDeploymentTargetID string) error {
	usages, err := incomingServiceBindingUsages(tx, projectID, targetApplicationID, targetDeploymentTargetID)
	if err != nil {
		return err
	}
	if len(usages) > 0 {
		return &serviceBindingInUseError{usages: usages}
	}
	return nil
}

// writeServiceBindingInUseError 识别删除事务返回的入站关系冲突并写 409 service_binding_in_use；其他错误返回 false 交由调用方处理。
func writeServiceBindingInUseError(ctx *gin.Context, err error) bool {
	var inUse *serviceBindingInUseError
	if !errors.As(err, &inUse) {
		return false
	}
	writeServiceBindingInUse(ctx, inUse.usages)
	return true
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
