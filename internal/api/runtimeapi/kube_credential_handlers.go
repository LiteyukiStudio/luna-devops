package runtimeapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/kubeaccess"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) CreateKubeCredential(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input kubeaccess.CreateInput
	if !bindJSON(ctx, &input) {
		return
	}
	result, err := h.kubeAccess.Create(ctx.Request.Context(), user, input)
	if err != nil {
		writeKubeAccessError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	auditWithSafeMetadata(h, user.ID, "kube_credential.create", result.Credential.ID, true, "", kubeCredentialAuditMetadata{
		BindingCount: len(result.Bindings), Scopes: result.Credential.Scopes, ExpiresInDays: normalizedKubeCredentialDays(input.ExpiresInDays),
	}, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, result)
}

func (h *Handlers) ListKubeCredentials(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	pagination := paginationFromQueryWithSort(ctx, map[string]string{
		"name": "name", "createdAt": "created_at", "expiresAt": "expires_at", "status": "status",
	}, "createdAt")
	result, err := h.kubeAccess.List(ctx.Request.Context(), user.ID, kubeaccess.PageOptions{
		Page: pagination.Page, PageSize: pagination.PageSize, SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
		Search: ctx.Query("search"), Status: strings.TrimSpace(ctx.Query("status")),
	})
	if err != nil {
		writeKubeAccessError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) ListKubeCredentialBindings(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	pagination := paginationFromQueryWithSort(ctx, map[string]string{
		"createdAt": "created_at", "projectId": "project_id", "runtimeClusterId": "runtime_cluster_id",
	}, "createdAt")
	result, err := h.kubeAccess.ListBindings(ctx.Request.Context(), user.ID, ctx.Param("credentialId"), kubeaccess.PageOptions{
		Page: pagination.Page, PageSize: pagination.PageSize, SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
	})
	if err != nil {
		writeKubeAccessError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) RevokeKubeCredential(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	credentialID := strings.TrimSpace(ctx.Param("credentialId"))
	if err := h.kubeAccess.Revoke(ctx.Request.Context(), user.ID, credentialID); err != nil {
		writeKubeAccessError(ctx, err)
		return
	}
	auditWithSafeMetadata(h, user.ID, "kube_credential.revoke", credentialID, true, "", kubeCredentialAuditMetadata{}, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func normalizedKubeCredentialDays(value int) int {
	if value == 0 {
		return 7
	}
	return value
}

func writeKubeAccessError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, kubeaccess.ErrScopeInvalid):
		writeErrorCode(ctx, http.StatusBadRequest, "kube_credential.scope_invalid", "kubernetes credential scope is invalid")
	case errors.Is(err, kubeaccess.ErrContextInvalid), errors.Is(err, kubeaccess.ErrInputInvalid):
		writeErrorCode(ctx, http.StatusBadRequest, "kube_credential.context_invalid", "kubernetes credential input is invalid")
	case errors.Is(err, kubeaccess.ErrCredentialNotFound):
		writeErrorCode(ctx, http.StatusNotFound, "kube_credential.not_found", "kubernetes credential was not found")
	case errors.Is(err, kubeaccess.ErrCredentialInvalid):
		writeErrorCode(ctx, http.StatusUnauthorized, "kube_credential.invalid", "kubernetes credential is invalid")
	case errors.Is(err, kubeaccess.ErrPermissionDenied):
		writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "permission denied")
	case errors.Is(err, kubeaccess.ErrGatewayDisabled):
		writeErrorCode(ctx, http.StatusConflict, "kube_gateway.disabled", "kubernetes gateway is disabled")
	case errors.Is(err, kubeaccess.ErrGatewayReconciling):
		writeErrorCode(ctx, http.StatusConflict, "kube_gateway.reconciling", "kubernetes gateway is reconciling")
	case errors.Is(err, kubeaccess.ErrGatewayUnavailable), errors.Is(err, kubeaccess.ErrPublicBaseURLRequired):
		writeErrorCode(ctx, http.StatusServiceUnavailable, "kube_gateway.unavailable", "kubernetes gateway is unavailable")
	default:
		writeErrorCode(ctx, http.StatusInternalServerError, "kube_credential.internal_error", "kubernetes credential operation failed")
	}
}
