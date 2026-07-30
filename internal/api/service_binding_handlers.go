package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/dependency"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) dependencyService() *dependency.Service {
	return dependency.NewService(dependency.NewGormRepository(h.db))
}

func (h *Handlers) ListServiceBindings(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	pagination := dependencyPagination(ctx, map[string]bool{
		"createdAt": true, "updatedAt": true, "protocol": true, "enabled": true,
	}, "createdAt")
	bindings, total, err := h.dependencyService().ListServiceBindings(ctx.Request.Context(), ctx.Param("projectId"), dependencyListOptions(pagination))
	if err != nil {
		writeDependencyError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(bindings, total, pagination))
}

func (h *Handlers) CreateServiceBinding(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	var input dependency.ServiceBindingInput
	if !bindJSON(ctx, &input) {
		return
	}
	binding, err := h.dependencyService().CreateServiceBinding(ctx.Request.Context(), project.ID, user.ID, input)
	if err != nil {
		h.audit(user.ID, "service_binding.create", project.ID, false, dependencyAuditMessage(err))
		writeDependencyError(ctx, err)
		return
	}
	h.audit(user.ID, "service_binding.create", binding.ID, true, binding.SourceDeploymentTargetID)
	h.emitServiceBindingEvent(ctx.Request.Context(), user, project, binding, "created", notification.SeverityInfo)
	ctx.JSON(http.StatusCreated, serviceBindingMutationResponseFor(binding))
}

func (h *Handlers) UpdateServiceBinding(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	var input dependency.ServiceBindingInput
	if !bindJSON(ctx, &input) {
		return
	}
	bindingID := ctx.Param("bindingId")
	binding, err := h.dependencyService().UpdateServiceBinding(ctx.Request.Context(), project.ID, bindingID, input)
	if err != nil {
		h.audit(user.ID, "service_binding.update", bindingID, false, dependencyAuditMessage(err))
		writeDependencyError(ctx, err)
		return
	}
	h.audit(user.ID, "service_binding.update", binding.ID, true, binding.SourceDeploymentTargetID)
	h.emitServiceBindingEvent(ctx.Request.Context(), user, project, binding, "updated", notification.SeverityInfo)
	ctx.JSON(http.StatusOK, serviceBindingMutationResponseFor(binding))
}

func (h *Handlers) DeleteServiceBinding(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	bindingID := ctx.Param("bindingId")
	binding, err := h.dependencyService().DeleteServiceBinding(ctx.Request.Context(), project.ID, bindingID)
	if err != nil {
		h.audit(user.ID, "service_binding.delete", bindingID, false, dependencyAuditMessage(err))
		writeDependencyError(ctx, err)
		return
	}
	h.audit(user.ID, "service_binding.delete", binding.ID, true, binding.SourceDeploymentTargetID)
	h.emitServiceBindingEvent(ctx.Request.Context(), user, project, binding, "deleted", notification.SeverityInfo)
	ctx.JSON(http.StatusOK, deletedServiceBindingMutationResponse(binding))
}

func (h *Handlers) CheckServiceBinding(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	_, project, ok := h.projectAndCurrentUser(ctx)
	if !ok {
		return
	}
	service := h.dependencyService()
	binding, err := service.ServiceBinding(ctx.Request.Context(), project.ID, ctx.Param("bindingId"))
	if err != nil {
		writeDependencyError(ctx, err)
		return
	}
	result, err := service.CheckServiceBinding(ctx.Request.Context(), project.ID, binding.ID)
	if err != nil {
		writeDependencyError(ctx, err)
		return
	}
	if result.Status == observation.StatusDeclared {
		var targetTarget model.DeploymentTarget
		if err := h.db.WithContext(ctx).First(&targetTarget, "id = ? and project_id = ?", binding.TargetDeploymentTargetID, project.ID).Error; err != nil {
			writeErrorCode(ctx, http.StatusNotFound, dependency.CodeNotFound, "target deployment target not found")
			return
		}
		client, _, unavailableCode := h.kubernetesClientForDeploymentTargetObservation(project, targetTarget)
		if client == nil {
			result.Status = observation.StatusUnavailable
			result.ObservationCode = "service_binding." + unavailableCode
			result.Checks = append(result.Checks, dependency.BindingCheckItem{
				Code: "kubernetes_check", Status: observation.StatusUnavailable,
			})
			ctx.JSON(http.StatusOK, result)
			return
		}
		readContext, cancel := context.WithTimeout(ctx.Request.Context(), 12*time.Second)
		diagnostic, diagnosticErr := client.CheckServiceDependency(readContext, kubeprovider.ServiceDependencyCheckOptions{
			SourceNamespace: runtimeProjectNamespace(project),
			TargetNamespace: runtimeProjectNamespace(project),
			ServiceName:     deploymentTargetResourceName(targetTarget),
			PortName:        binding.TargetPortName,
			PortNumber:      int32(binding.TargetPort),
		})
		cancel()
		if diagnosticErr != nil {
			result.Status = observation.StatusUnavailable
			result.ObservationCode = "service_binding.kubernetes_unavailable"
			result.Checks = append(result.Checks, dependency.BindingCheckItem{
				Code: "kubernetes_check", Status: observation.StatusUnavailable,
			})
		} else {
			result.Status = observation.StatusReady
			result.ObservationCode = "service_binding.ready"
			for _, check := range diagnostic.Checks {
				result.Checks = append(result.Checks, dependency.BindingCheckItem{
					Code: check.Code, Status: string(check.Status), Resource: fmt.Sprintf("%s/%s", diagnostic.TargetNamespace, diagnostic.ServiceName),
				})
				switch {
				case check.Code == kubeprovider.ServiceDependencyCheckServicePortResolved && check.Status == kubeprovider.ServiceDependencyCheckFailed:
					result.Status = "invalid"
					result.ObservationCode = "service_binding.port_unresolved"
				case (check.Code == kubeprovider.ServiceDependencyCheckServiceExists || check.Code == kubeprovider.ServiceDependencyCheckEndpointReady) && check.Status == kubeprovider.ServiceDependencyCheckFailed && result.Status != "invalid":
					result.Status = observation.StatusUnavailable
					if check.Code == kubeprovider.ServiceDependencyCheckServiceExists {
						result.ObservationCode = "service_binding.service_not_found"
					} else {
						result.ObservationCode = "service_binding.endpoint_unavailable"
					}
				}
			}
		}
	} else if result.Status == "disabled" {
		result.ObservationCode = "service_binding.disabled"
	} else if result.Status == "invalid" {
		result.ObservationCode = "service_binding.invalid"
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) ListProjectTopologyEdges(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	pagination := dependencyPagination(ctx, map[string]bool{
		"createdAt": true, "updatedAt": true, "relationType": true, "protocol": true,
	}, "createdAt")
	edges, total, err := h.dependencyService().ListTopologyEdges(ctx.Request.Context(), ctx.Param("projectId"), dependencyListOptions(pagination))
	if err != nil {
		writeDependencyError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(edges, total, pagination))
}

func (h *Handlers) CreateProjectTopologyEdge(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	var input dependency.TopologyEdgeInput
	if !bindJSON(ctx, &input) {
		return
	}
	edge, err := h.dependencyService().CreateTopologyEdge(ctx.Request.Context(), project.ID, user.ID, input)
	if err != nil {
		h.audit(user.ID, "project_topology_edge.create", project.ID, false, dependencyAuditMessage(err))
		writeDependencyError(ctx, err)
		return
	}
	h.audit(user.ID, "project_topology_edge.create", edge.ID, true, edge.RelationType)
	ctx.JSON(http.StatusCreated, edge)
}

func (h *Handlers) UpdateProjectTopologyEdge(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	var input dependency.TopologyEdgeInput
	if !bindJSON(ctx, &input) {
		return
	}
	edgeID := ctx.Param("edgeId")
	edge, err := h.dependencyService().UpdateTopologyEdge(ctx.Request.Context(), project.ID, edgeID, input)
	if err != nil {
		h.audit(user.ID, "project_topology_edge.update", edgeID, false, dependencyAuditMessage(err))
		writeDependencyError(ctx, err)
		return
	}
	h.audit(user.ID, "project_topology_edge.update", edge.ID, true, edge.RelationType)
	ctx.JSON(http.StatusOK, edge)
}

func (h *Handlers) DeleteProjectTopologyEdge(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	edgeID := ctx.Param("edgeId")
	edge, err := h.dependencyService().DeleteTopologyEdge(ctx.Request.Context(), project.ID, edgeID)
	if err != nil {
		h.audit(user.ID, "project_topology_edge.delete", edgeID, false, dependencyAuditMessage(err))
		writeDependencyError(ctx, err)
		return
	}
	h.audit(user.ID, "project_topology_edge.delete", edge.ID, true, edge.RelationType)
	ctx.Status(http.StatusNoContent)
}

func dependencyListOptions(pagination paginationParams) dependency.ListOptions {
	return dependency.ListOptions{
		Page: pagination.Page, PageSize: pagination.PageSize, SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
	}
}

func serviceBindingMutationResponseFor(binding model.ServiceBinding) gin.H {
	return gin.H{
		"item":             binding,
		"requiresRedeploy": true,
		"affectedDeploymentTargets": []gin.H{{
			"applicationId": binding.SourceApplicationID, "deploymentTargetId": binding.SourceDeploymentTargetID,
		}},
	}
}

func deletedServiceBindingMutationResponse(binding model.ServiceBinding) gin.H {
	return gin.H{
		"requiresRedeploy": true,
		"affectedDeploymentTargets": []gin.H{{
			"applicationId": binding.SourceApplicationID, "deploymentTargetId": binding.SourceDeploymentTargetID,
		}},
	}
}

func dependencyPagination(ctx *gin.Context, allowed map[string]bool, fallback string) paginationParams {
	pagination := paginationFromQuery(ctx)
	if !allowed[pagination.SortBy] {
		pagination.SortBy = fallback
	}
	return pagination
}

func dependencyAuditMessage(err error) string {
	if code := dependency.ErrorCode(err); code != "" {
		return code
	}
	return "dependency operation failed"
}

func writeDependencyError(ctx *gin.Context, err error) {
	code := dependency.ErrorCode(err)
	status := http.StatusInternalServerError
	switch code {
	case dependency.CodeNotFound:
		status = http.StatusNotFound
	case dependency.CodeEnvConflict, dependency.CodeTopologyDuplicate:
		status = http.StatusConflict
	case dependency.CodeInvalidInput, dependency.CodeCrossProject, dependency.CodeCrossCluster,
		dependency.CodeSourceTargetSame, dependency.CodePortNotFound, dependency.CodeReservedEnv:
		status = http.StatusBadRequest
	}
	if code == "" {
		code = "dependency_operation_failed"
	}
	writeErrorCode(ctx, status, code, err.Error())
}
