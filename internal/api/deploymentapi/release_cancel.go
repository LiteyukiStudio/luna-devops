package deploymentapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const releaseNotCancelableCode = "release.not_cancelable"

var errReleaseNotCancelable = errors.New("release is not cancelable")

// CancelRelease stops a pending or running release workflow. Resources that
// were already applied to the cluster are left as they are; the Worker stops
// before applying the workload or while waiting for the rollout. Repeating the
// request for an already canceled release returns the same canceled state.
func (h *Handlers) CancelRelease(ctx *gin.Context) {
	user, _, ok := h.authorizeProject(ctx, authz.ActionDeploymentRelease)
	if !ok {
		return
	}
	release, ok := h.findRelease(ctx)
	if !ok {
		return
	}
	canceled := false
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.Release
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ? and project_id = ?", release.ID, release.ProjectID).Error; err != nil {
			return err
		}
		if locked.Status == "canceled" {
			release = locked
			return nil
		}
		if locked.Status != "pending" && locked.Status != "running" {
			return errReleaseNotCancelable
		}
		finishedAt := time.Now()
		if err := tx.Model(&model.Release{}).
			Where("id = ? and status in ?", locked.ID, model.ReleaseActiveStatuses).
			Updates(map[string]any{
				"status":       "canceled",
				"failure_code": model.ReleaseFailureCanceledByUser,
				"finished_at":  &finishedAt,
			}).Error; err != nil {
			return err
		}
		canceled = true
		return tx.First(&release, "id = ?", locked.ID).Error
	})
	if err != nil {
		if errors.Is(err, errReleaseNotCancelable) {
			writeErrorCode(ctx, http.StatusConflict, releaseNotCancelableCode, "only pending or running releases can be canceled")
			return
		}
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if canceled {
		h.auditWithContext(user.ID, "release.cancel", release.ID, true, "", ctx.Request.Context())
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, release)
}
