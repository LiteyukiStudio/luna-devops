package volumeapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const scopedResourceRuntimeCluster = "runtime_cluster"

// RegisterAITools registers the 8 AI Domain Facade Tools owned by volumeapi:
//   - runtime_list_volumes
//   - runtime_get_volume
//   - runtime_create_volume
//   - runtime_update_volume
//   - runtime_retry_volume_operation
//   - runtime_delete_volume
//   - runtime_create_volume_export
//   - runtime_cancel_volume_transfer
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	if h == nil || reg == nil {
		return
	}
	h.registerVolumeAITools(reg)
}

func newVolumeSubGin(tc *aitool.ToolContext) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	sub, _ := gin.CreateTestContext(rec)
	req, _ := http.NewRequestWithContext(tc.Context(), http.MethodPost, "/internal/v1/ai/tools", http.NoBody)
	if tc != nil && tc.Gin != nil && tc.Gin.Request != nil {
		for k, vals := range tc.Gin.Request.Header {
			req.Header[k] = append([]string(nil), vals...)
		}
		req.RemoteAddr = tc.Gin.Request.RemoteAddr
	}
	if tc != nil && strings.TrimSpace(tc.IdempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(tc.IdempotencyKey))
	}
	sub.Request = req
	if tc != nil && tc.Gin != nil && tc.Gin.Keys != nil {
		sub.Keys = maps.Clone(tc.Gin.Keys)
	}
	return sub, rec
}

func abortedVolumeToolError(rec *httptest.ResponseRecorder, fallbackCode, fallbackMessage string) *aitool.ToolError {
	status := http.StatusBadRequest
	if rec != nil && rec.Code >= 400 {
		status = rec.Code
	}
	if rec == nil {
		return aitool.NewError(status, fallbackCode, fallbackMessage)
	}
	raw := bytes.TrimSpace(rec.Body.Bytes())
	if len(raw) == 0 {
		return aitool.NewError(status, fallbackCode, fallbackMessage)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return aitool.NewError(status, fallbackCode, string(raw))
	}
	code, _ := parsed["code"].(string)
	if strings.TrimSpace(code) == "" {
		code = fallbackCode
	}
	msg, _ := parsed["message"].(string)
	if strings.TrimSpace(msg) == "" {
		msg = fallbackMessage
	}
	return &aitool.ToolError{
		Status:  status,
		Code:    code,
		Message: msg,
		RawBody: append(json.RawMessage(nil), raw...),
	}
}

func volumeDomainToToolError(err error) *aitool.ToolError {
	if err == nil {
		return nil
	}
	if te, ok := aitool.AsToolError(err); ok {
		return te
	}
	if errors.Is(err, authz.ErrProjectAuthorizationUnavailable) || errors.Is(err, authz.ErrProjectPolicyUndefined) {
		return aitool.NewError(http.StatusServiceUnavailable, "project.authorization_unavailable", "project authorization is temporarily unavailable")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return aitool.NotFound(volume.CodeNotFound, "project volume was not found")
	}
	code := volume.ErrorCode(err)
	status := http.StatusInternalServerError
	switch code {
	case volume.CodeInvalidInput, volume.CodePaginationSortByInvalid, volume.CodePaginationOrderInvalid:
		status = http.StatusBadRequest
	case volume.CodeNotFound, volume.CodeClaimNotFound, volume.CodeTransferNotFound, volume.CodeSnapshotNotFound:
		status = http.StatusNotFound
	case volume.CodeTaskEnqueueFailed, volume.CodeClusterUnavailable, volume.CodeQuotaUnavailable, volume.CodeTransferUnavailable:
		status = http.StatusServiceUnavailable
	case volume.CodeOwnershipConflict, volume.CodeIncompatibleCluster, volume.CodeBindingConflict,
		volume.CodeInUse, volume.CodeCapacityShrinkForbidden, volume.CodeRevisionConflict,
		volume.CodeStateConflict, volume.CodeIdempotencyConflict, volume.CodeNameConflict,
		volume.CodeClaimConflict, volume.CodeClaimSpecConflict, volume.CodeTransferStateConflict,
		volume.CodeTransferFormatMismatch, volume.CodeQuotaExceeded, volume.CodeDeletionPending:
		status = http.StatusConflict
	case volume.CodeExpansionUnsupported, volume.CodeSnapshotUnsupported, volume.CodeSnapshotRequired,
		volume.CodeTransferProgressInvalid, volume.CodeTransferChecksumInvalid,
		volume.CodeTransferChecksumMismatch, volume.CodeTransferArchiveUnsafe,
		volume.CodeTransferCapacityExceeded, volume.CodeTransferFormatUnsupported:
		status = http.StatusUnprocessableEntity
	case volume.CodeTransferExpired:
		status = http.StatusGone
	case volume.CodeTransferDownloadUnauthorized:
		status = http.StatusUnauthorized
	case "":
		code = "internal_error"
	}
	return aitool.NewError(status, code, err.Error())
}

func (h *Handler) authorizeAIVolumeProject(tc *aitool.ToolContext, projectRef string, action authz.Action) (model.User, model.Project, error) {
	projectRef = strings.TrimSpace(projectRef)
	if projectRef == "" {
		return model.User{}, model.Project{}, aitool.BadRequest("project.required", "project is required")
	}
	sub, rec := newVolumeSubGin(tc)
	user, project, ok := h.authorizeProjectRef(sub, projectRef, action)
	if !ok {
		return model.User{}, model.Project{}, abortedVolumeToolError(rec, "project.forbidden", "project authorization failed")
	}
	if strings.TrimSpace(user.ID) == "" {
		user = tc.User
	}
	return user, project, nil
}

func (h *Handler) resolveClusterForProjectVolumeAI(tc *aitool.ToolContext, user model.User, projectID, clusterRef string) (model.RuntimeCluster, error) {
	clusterRef = strings.TrimSpace(clusterRef)
	if clusterRef == "" {
		return model.RuntimeCluster{}, aitool.BadRequest(volume.CodeInvalidInput, "cluster is required")
	}
	db := h.dbWithContext(tc.Context())
	if db != nil {
		var byID model.RuntimeCluster
		if err := runtimecluster.ActiveScope(db).First(&byID, "id = ?", clusterRef).Error; err == nil {
			sub, rec := newVolumeSubGin(tc)
			cluster, usable := h.runtimeClusterForProjectUse(sub, user, projectID, byID.ID)
			if !usable {
				return model.RuntimeCluster{}, abortedVolumeToolError(rec, "runtime_cluster.forbidden", "runtime cluster is not usable for this project")
			}
			return cluster, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return model.RuntimeCluster{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}

		query := h.applyScopedResourceVisibilityForProject(
			runtimecluster.ActiveScope(db).Model(&model.RuntimeCluster{}),
			scopedResourceRuntimeCluster,
			user,
			projectID,
			tc.Context(),
		)
		var candidates []model.RuntimeCluster
		if err := query.Order("created_at asc").Find(&candidates).Error; err != nil {
			return model.RuntimeCluster{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		matched, err := aitool.ResolveUniqueByIDOrName(
			candidates,
			"runtime_cluster",
			"runtime_cluster.not_found",
			clusterRef,
			func(c model.RuntimeCluster) string { return c.ID },
			func(c model.RuntimeCluster) string { return c.Name },
		)
		if err != nil {
			return model.RuntimeCluster{}, err
		}
		sub, rec := newVolumeSubGin(tc)
		cluster, usable := h.runtimeClusterForProjectUse(sub, user, projectID, matched.ID)
		if !usable {
			return model.RuntimeCluster{}, abortedVolumeToolError(rec, "runtime_cluster.forbidden", "runtime cluster is not usable for this project")
		}
		return cluster, nil
	}

	sub, rec := newVolumeSubGin(tc)
	cluster, usable := h.runtimeClusterForProjectUse(sub, user, projectID, clusterRef)
	if !usable {
		return model.RuntimeCluster{}, abortedVolumeToolError(rec, "runtime_cluster.forbidden", "runtime cluster is not usable for this project")
	}
	return cluster, nil
}

func (h *Handler) resolveProjectVolumeForAI(tc *aitool.ToolContext, projectID, volumeRef string) (model.ProjectVolume, error) {
	volumeRef = strings.TrimSpace(volumeRef)
	if volumeRef == "" {
		return model.ProjectVolume{}, aitool.BadRequest(volume.CodeInvalidInput, "volume is required")
	}
	if h.volumes == nil {
		return model.ProjectVolume{}, aitool.NewError(http.StatusServiceUnavailable, volume.CodeClusterUnavailable, "volume service is unavailable")
	}
	if item, err := h.volumes.GetProjectVolume(tc.Context(), projectID, volumeRef); err == nil {
		return item, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) && volume.ErrorCode(err) != volume.CodeNotFound {
		return model.ProjectVolume{}, volumeDomainToToolError(err)
	}

	var candidates []model.ProjectVolume
	if db := h.dbWithContext(tc.Context()); db != nil {
		if err := db.Where("project_id = ?", projectID).Order("created_at asc").Find(&candidates).Error; err != nil {
			return model.ProjectVolume{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
	} else {
		listRes, err := h.volumes.ListProjectVolumes(tc.Context(), projectID, volume.ProjectVolumeListOptions{
			Page:     1,
			PageSize: volume.MaxPageSize,
			Search:   volumeRef,
		})
		if err != nil {
			return model.ProjectVolume{}, volumeDomainToToolError(err)
		}
		candidates = listRes.Items
	}

	matched, err := aitool.ResolveUniqueByIDOrName(
		candidates,
		"project_volume",
		volume.CodeNotFound,
		volumeRef,
		func(item model.ProjectVolume) string { return item.ID },
		func(item model.ProjectVolume) string { return item.DisplayName },
	)
	if err != nil {
		return model.ProjectVolume{}, err
	}
	full, err := h.volumes.GetProjectVolume(tc.Context(), projectID, matched.ID)
	if err != nil {
		return model.ProjectVolume{}, volumeDomainToToolError(err)
	}
	return full, nil
}

// effectiveVolumeIdempotencyKey prefers the deterministic Idempotency-Key header
// sent by luna-agent, falling back to a deterministic hash of (runId, toolCallId).
func effectiveVolumeIdempotencyKey(tc *aitool.ToolContext, prefix string) (string, error) {
	if tc != nil {
		if headerKey := strings.TrimSpace(tc.IdempotencyKey); headerKey != "" {
			if len(headerKey) < 8 || len(headerKey) > 160 {
				return "", aitool.BadRequest(volume.CodeInvalidInput, "Idempotency-Key must contain 8 to 160 characters")
			}
			return headerKey, nil
		}
		seed := strings.TrimSpace(tc.RunID) + ":" + strings.TrimSpace(tc.ToolCallID)
		if seed != ":" {
			sum := sha256.Sum256([]byte(prefix + ":" + seed))
			return "aiidem_" + prefix + "_" + hex.EncodeToString(sum[:16]), nil
		}
	}
	return "", aitool.BadRequest(volume.CodeInvalidInput, "Idempotency-Key is required")
}
