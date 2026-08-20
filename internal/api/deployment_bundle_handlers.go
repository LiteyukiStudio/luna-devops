package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ExportDeploymentTargetBundle(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper, authz.ProjectRoleViewer)
	if !ok {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	operationCtx, endOperation := telemetry.StartOperation(ctx.Request.Context(), "deployment", "bundle_export")
	ctx.Request = ctx.Request.WithContext(operationCtx)
	var operationErr error
	defer func() { endOperation(operationErr) }()
	var target model.DeploymentTarget
	if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), project.ID, app.ID).Error; err != nil {
		operationErr = err
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErrorCode(ctx, http.StatusNotFound, "deployment_target.not_found", "deployment target not found")
			return
		}
		h.auditWithContext(user.ID, "deployment_bundle.export", ctx.Param("targetId"), false, "deployment_bundle.export_failed", ctx.Request.Context())
		writeDeploymentBundleCode(ctx, "deployment_bundle.export_failed")
		return
	}
	bundle, err := h.buildDeploymentTargetBundle(ctx.Request.Context(), project, app, target)
	if err != nil {
		operationErr = err
		h.auditWithContext(user.ID, "deployment_bundle.export", target.ID, false, "deployment_bundle.export_failed", ctx.Request.Context())
		writeDeploymentBundleCode(ctx, "deployment_bundle.export_failed")
		return
	}
	filename := fmt.Sprintf("luna-deployment-%s-%s.json", deploymentBundleFilenamePart(app.Identifier), deploymentBundleFilenamePart(target.Stage))
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	ctx.Header("X-Content-Type-Options", "nosniff")
	h.auditWithContext(user.ID, "deployment_bundle.export", target.ID, true, "deployment_bundle.export_succeeded", ctx.Request.Context())
	ctx.JSON(http.StatusOK, bundle)
}

func (h *Handlers) PreviewDeploymentTargetBundleImport(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	user, project, app, request, ok := h.prepareDeploymentTargetBundleImport(ctx, false)
	if !ok {
		return
	}
	operationCtx, endOperation := telemetry.StartOperation(ctx.Request.Context(), "deployment", "bundle_preview")
	ctx.Request = ctx.Request.WithContext(operationCtx)
	var operationErr error
	defer func() { endOperation(operationErr) }()
	plan, err := h.buildDeploymentTargetImportPlan(ctx, user, project, app, request, false)
	if err != nil {
		operationErr = err
		h.auditWithContext(user.ID, "deployment_bundle.preview", app.ID, false, deploymentBundleErrorCode(err), ctx.Request.Context())
		writeDeploymentBundleError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "deployment_bundle.preview", app.ID, true, "deployment_bundle.preview_"+plan.Preview.Status, ctx.Request.Context())
	ctx.JSON(http.StatusOK, plan.Preview)
}

func (h *Handlers) ListDeploymentTargetBundleReferenceCandidates(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "application deletion is in progress")
		return
	}
	var request deploymentBundleReferenceCandidatesRequest
	if !bindDeploymentBundleCandidateJSON(ctx, &request) {
		return
	}
	if strings.TrimSpace(request.Reference.Key) == "" || !deploymentBundleReferenceKindAllowed(request.Reference.Kind) {
		writeDeploymentBundleError(ctx, &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle reference is invalid"})
		return
	}
	if order := strings.ToLower(strings.TrimSpace(ctx.Query("sortOrder"))); order != "" && order != "asc" && order != "desc" {
		writeDeploymentBundleCode(ctx, "deployment_bundle.candidate_query_invalid")
		return
	}
	if sortBy := strings.TrimSpace(ctx.Query("sortBy")); sortBy != "" && sortBy != "name" && sortBy != "createdAt" {
		writeDeploymentBundleCode(ctx, "deployment_bundle.candidate_query_invalid")
		return
	}
	search := strings.TrimSpace(ctx.Query("search"))
	if len([]rune(search)) > 120 {
		writeDeploymentBundleCode(ctx, "deployment_bundle.candidate_query_invalid")
		return
	}
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"name": "name", "createdAt": "created_at"}, "name")
	if strings.TrimSpace(ctx.Query("sortOrder")) == "" {
		pagination.SortOrder = "asc"
	}
	operationCtx, endOperation := telemetry.StartOperation(ctx.Request.Context(), "deployment", "bundle_reference_candidates")
	ctx.Request = ctx.Request.WithContext(operationCtx)
	var operationErr error
	defer func() { endOperation(operationErr) }()
	page, _, err := h.deploymentBundleCandidates(ctx.Request.Context(), user, project, app, request.Reference, deploymentBundleCandidateQuery{
		Pagination: pagination,
		Search:     search,
	})
	if err != nil {
		operationErr = err
		writeDeploymentBundleError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, page)
}

func (h *Handlers) ImportDeploymentTargetBundle(ctx *gin.Context) {
	user, project, app, request, ok := h.prepareDeploymentTargetBundleImport(ctx, true)
	if !ok {
		return
	}
	if !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
		return
	}
	operationCtx, endOperation := telemetry.StartOperation(ctx.Request.Context(), "deployment", "bundle_import")
	ctx.Request = ctx.Request.WithContext(operationCtx)
	var operationErr error
	defer func() { endOperation(operationErr) }()
	plan, err := h.buildDeploymentTargetImportPlan(ctx, user, project, app, request, true)
	if err != nil {
		operationErr = err
		h.auditWithContext(user.ID, "deployment_bundle.import", app.ID, false, deploymentBundleErrorCode(err), ctx.Request.Context())
		writeDeploymentBundleError(ctx, err)
		return
	}
	if plan.Preview.Status != deploymentBundleStatusReady {
		err = &deploymentBundleError{Code: "deployment_bundle.not_ready", Message: "deployment bundle references or target stage are not ready"}
		operationErr = err
		h.auditWithContext(user.ID, "deployment_bundle.import", app.ID, false, deploymentBundleErrorCode(err), ctx.Request.Context())
		writeDeploymentBundleError(ctx, err)
		return
	}
	if (plan.Input.BuildVariables != nil || len(plan.SecretValues) > 0) && !h.canManageBuildEnvironmentProject(ctx, user, project.ID) {
		operationErr = errors.New("deployment bundle secret permission denied")
		return
	}
	if (plan.Input.BuildVariables != nil || len(plan.SecretValues) > 0) && !h.requireStepUp(ctx, user, stepUpPurposeSecretUpdate) {
		operationErr = errors.New("deployment bundle secret step-up required")
		return
	}

	targetID := id.New("dplt")
	secretEntries, buildEnvironment, runtimeSecretRefs, runtimeSecretFiles, ok := h.materializeDeploymentBundleSecrets(ctx, user, targetID, plan)
	if !ok {
		operationErr = errors.New("deployment bundle secret materialization failed")
		return
	}
	input := plan.Input
	// Secret files have already been encrypted into transaction-local entries.
	// Keep the regular input parser from storing them before the target transaction.
	input.SecretFiles = "[]"
	input.BuildSecrets = nil
	target, dataVolumes, ok := h.deploymentTargetFromInput(
		ctx, user, app, input, targetID,
		resourceidentifier.DeploymentTargetName(app.Identifier, normalizeStage(input.Stage)), nil, "",
	)
	if !ok {
		operationErr = errors.New("deployment bundle target validation failed")
		return
	}
	target.SecretRefs = encodeStringMap(runtimeSecretRefs)
	target.SecretFiles = encodeStringMap(runtimeSecretFiles)
	for _, volumeInput := range dataVolumes {
		if runtimeDataPathConflicts(volumeInput.MountPath, target.ConfigFiles, target.SecretFiles) {
			operationErr = errors.New("deployment bundle runtime path conflict")
			h.auditWithContext(user.ID, "deployment_bundle.import", app.ID, false, "deployment_bundle.runtime_path_conflict", ctx.Request.Context())
			writeDeploymentBundleCode(ctx, "deployment_bundle.runtime_path_conflict")
			return
		}
	}
	changes, err := h.persistDeploymentTarget(target, dataVolumes, input.BuildHookBindings, buildEnvironment, secretEntries, true, ctx.Request.Context())
	if errors.Is(err, errDeploymentStageExists) {
		operationErr = err
		h.auditWithContext(user.ID, "deployment_bundle.import", app.ID, false, "deployment_bundle.stage_conflict", ctx.Request.Context())
		writeDeploymentBundleError(ctx, &deploymentBundleError{Code: "deployment_bundle.stage_conflict", Message: "deployment stage already exists in the destination application"})
		return
	}
	if err != nil {
		operationErr = err
		h.auditDeploymentVolumeMountFailure(ctx.Request.Context(), user.ID, changes, err)
		h.auditWithContext(user.ID, "deployment_bundle.import", app.ID, false, deploymentBundleErrorCode(err), ctx.Request.Context())
		writeDeploymentBundleError(ctx, err)
		return
	}
	for _, entry := range secretEntries {
		h.auditWithContext(user.ID, "secret.write", entry.ID, true, "deployment_bundle.secret_stored", ctx.Request.Context())
	}
	h.auditDeploymentVolumeMountChanges(ctx.Request.Context(), user.ID, target, changes)
	h.auditWithContext(user.ID, "deployment_bundle.import", target.ID, true, "deployment_bundle.import_succeeded", ctx.Request.Context())
	// The transaction has committed at this point. Build the response from the
	// values returned by that transaction instead of performing a fallible read
	// and misreporting an already-created deployment as a failed import.
	target.BuildHookBindings = changes.HookBindings
	ctx.JSON(http.StatusCreated, deploymentTargetResponseFromModel(target, changes.Bound))
}

func (h *Handlers) prepareDeploymentTargetBundleImport(ctx *gin.Context, commit bool) (model.User, model.Project, model.Application, deploymentTargetBundleImportRequest, bool) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
	if !ok {
		return model.User{}, model.Project{}, model.Application{}, deploymentTargetBundleImportRequest{}, false
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return model.User{}, model.Project{}, model.Application{}, deploymentTargetBundleImportRequest{}, false
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return model.User{}, model.Project{}, model.Application{}, deploymentTargetBundleImportRequest{}, false
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "application deletion is in progress")
		return model.User{}, model.Project{}, model.Application{}, deploymentTargetBundleImportRequest{}, false
	}
	var request deploymentTargetBundleImportRequest
	if !bindDeploymentBundleJSON(ctx, &request) {
		return model.User{}, model.Project{}, model.Application{}, deploymentTargetBundleImportRequest{}, false
	}
	if !commit {
		request.SecretValues = nil
	}
	return user, project, app, request, true
}

func (h *Handlers) materializeDeploymentBundleSecrets(ctx *gin.Context, user model.User, targetID string, plan deploymentTargetBundleImportPlan) ([]model.SecretValue, *model.BuildEnvironmentConfig, map[string]string, map[string]string, bool) {
	entries := make([]model.SecretValue, 0, len(plan.SecretValues))
	buildSecretRefs := map[string]string{}
	runtimeSecretRefs := map[string]string{}
	runtimeSecretFiles := map[string]string{}
	for _, item := range plan.SecretValues {
		requirement := item.Requirement
		if requirement.Target != deploymentBundleSecretRuntimeFile && !isBuildEnvKey(requirement.Name) {
			writeDeploymentBundleError(ctx, &deploymentBundleError{Code: "deployment_bundle.secret_requirement_invalid", Message: "secret requirement name is invalid"})
			return nil, nil, nil, nil, false
		}
		path := strings.TrimSpace(requirement.Path)
		if requirement.Target == deploymentBundleSecretRuntimeFile {
			var pathOK bool
			path, pathOK = normalizeRuntimeConfigFilePathInput(ctx, path)
			if !pathOK {
				return nil, nil, nil, nil, false
			}
		}
		cipherRef := secret.Encrypt(item.Value)
		if cipherRef == "" {
			writeDeploymentBundleCode(ctx, "deployment_bundle.secret_encrypt_failed")
			return nil, nil, nil, nil, false
		}
		entry := model.SecretValue{
			ID: id.New("sec"), CipherRef: cipherRef, CreatedBy: user.ID,
			Resource: "deployment_bundle:" + targetID + ":" + requirement.Target,
		}
		ref := "secret-id:" + entry.ID
		switch requirement.Target {
		case deploymentBundleSecretBuild:
			buildSecretRefs[requirement.Name] = ref
		case deploymentBundleSecretRuntimeEnv:
			runtimeSecretRefs[requirement.Name] = ref
		case deploymentBundleSecretRuntimeFile:
			runtimeSecretFiles[path] = ref
		default:
			writeDeploymentBundleError(ctx, &deploymentBundleError{Code: "deployment_bundle.secret_requirement_invalid", Message: "secret requirement target is invalid"})
			return nil, nil, nil, nil, false
		}
		entries = append(entries, entry)
	}
	var buildEnvironment *model.BuildEnvironmentConfig
	if plan.Input.BuildVariables != nil || len(buildSecretRefs) > 0 {
		variables := map[string]string{}
		if plan.Input.BuildVariables != nil {
			var ok bool
			variables, ok = normalizeBuildVariables(ctx, *plan.Input.BuildVariables)
			if !ok {
				return nil, nil, nil, nil, false
			}
		}
		buildEnvironment = &model.BuildEnvironmentConfig{
			ID: id.New("bec"), Scope: model.BuildEnvironmentScopeDeployment, ScopeRef: targetID,
			Variables: buildenv.Encode(variables), SecretRefs: buildenv.Encode(buildSecretRefs), UpdatedBy: user.ID,
		}
	}
	return entries, buildEnvironment, runtimeSecretRefs, runtimeSecretFiles, true
}

func encodeStringMap(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(payload)
}

func deploymentBundleFilenamePart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			builder.WriteRune(char)
		}
	}
	if builder.Len() == 0 {
		return "deployment"
	}
	return builder.String()
}
