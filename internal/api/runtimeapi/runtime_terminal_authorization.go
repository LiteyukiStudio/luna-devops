package runtimeapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	runtimeTerminalResourceCheckTimeout = 2 * time.Second
)

func (h *Handlers) requireRuntimeTerminalAuthorization(ctx *gin.Context, user model.User) (runtimeTerminalAuthorizationBinding, bool) {
	binding, ok := h.currentInteractiveAuthorizationBinding(ctx, user)
	if !ok {
		if requestUsesBearerToken(ctx) {
			h.auditWithContext(user.ID, "runtime_terminal.session_required", "runtime_terminal", false, "personal access tokens cannot authorize a terminal", ctx.Request.Context())
			writeErrorCode(ctx, http.StatusForbidden, "runtime.terminal_session_required", "个人令牌不能用于终端，请使用浏览器会话或 Luna CLI OAuth 登录")
		} else {
			writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		}
		return runtimeTerminalAuthorizationBinding{}, false
	}
	return binding, true
}

func requireRuntimeTerminalTicketForBearer(ctx *gin.Context, ticket string) bool {
	if strings.TrimSpace(ticket) != "" || !requestUsesBearerToken(ctx) {
		return true
	}
	writeErrorCode(
		ctx,
		http.StatusForbidden,
		"runtime_terminal.ticket_required",
		"bearer clients must authorize a one-time terminal ticket before opening the WebSocket",
	)
	return false
}

func (h *Handlers) currentInteractiveAuthorizationBinding(ctx *gin.Context, user model.User) (runtimeTerminalAuthorizationBinding, bool) {
	subject, ok := h.currentInteractiveSubject(ctx, user)
	if !ok {
		return runtimeTerminalAuthorizationBinding{}, false
	}
	binding := runtimeTerminalAuthorizationBinding{UserID: user.ID, SubjectID: subject}
	if requestUsesBearerToken(ctx) {
		token, tokenOK := currentAccessTokenFromContext(ctx)
		if !tokenOK || token.UserID != user.ID || token.OAuthGrantID == "" || token.OAuthFamilyID == "" {
			return runtimeTerminalAuthorizationBinding{}, false
		}
		binding = continuousAuthorizationBindingForAccessToken(user.ID, token)
		binding.Deadline = time.Now().Add(time.Hour)
		if token.ExpiresAt != nil && token.ExpiresAt.Before(binding.Deadline) {
			binding.Deadline = *token.ExpiresAt
		}
	} else {
		session, sessionOK := h.currentSessionFromCookie(ctx)
		if !sessionOK || session.UserID != user.ID || session.ID != subject {
			return runtimeTerminalAuthorizationBinding{}, false
		}
		binding.Deadline = session.ExpiresAt
	}
	return binding, true
}

func (h *Handlers) currentInteractiveSubject(ctx *gin.Context, user model.User) (string, bool) {
	if requestUsesBearerToken(ctx) {
		token, ok := currentAccessTokenFromContext(ctx)
		if !ok || token.UserID != user.ID || token.Source != "oauth" || token.OAuthApplicationID != lunaCLIApplicationID || token.OAuthGrantID == "" || token.OAuthFamilyID == "" {
			return "", false
		}
		return oauthAccessTokenSubject(token.ID), true
	}
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok || session.UserID != user.ID {
		return "", false
	}
	return session.ID, true
}

func oauthAccessTokenSubject(tokenID string) string {
	return continuousAccessTokenSubject(tokenID)
}

type releaseRuntimeTerminalAuthorizationReference struct {
	ProjectID          string
	ApplicationID      string
	ReleaseID          string
	DeploymentTargetID string
	ClusterID          string
	ClusterKubeconfig  string
	Namespace          string
}

func (h *Handlers) releaseRuntimeTerminalAuthorizationAllowed(ctx context.Context, user model.User, reference releaseRuntimeTerminalAuthorizationReference) bool {
	db := h.dbWithContext(ctx).WithContext(ctx)
	var project model.Project
	if err := db.First(&project, "id = ?", reference.ProjectID).Error; err != nil || !resourceCanMutateDuringDelete(project.DeleteStatus) {
		return false
	}
	if _, err := h.projectAuthorizer(ctx).AuthorizeProject(ctx, authz.ProjectSubject{
		UserID: user.ID, PlatformRole: user.Role,
	}, reference.ProjectID, authz.ActionDeploymentExec); err != nil {
		return false
	}

	var release model.Release
	if err := db.First(&release, "id = ? and project_id = ? and application_id = ? and deployment_target_id = ?", reference.ReleaseID, reference.ProjectID, reference.ApplicationID, reference.DeploymentTargetID).Error; err != nil {
		return false
	}
	var target model.DeploymentTarget
	if err := db.First(&target, "id = ? and project_id = ? and application_id = ?", reference.DeploymentTargetID, reference.ProjectID, reference.ApplicationID).Error; err != nil {
		return false
	}
	if !resourceCanMutateDuringDelete(target.DeleteStatus) || !runtimeWebConsoleEnabled(project, target) || deploymentTargetNamespace(project, target) != reference.Namespace {
		return false
	}
	cluster, err := runtimeClusterForDeploymentTargetDB(db, target)
	return err == nil && cluster.ID == reference.ClusterID && cluster.KubeconfigRef == reference.ClusterKubeconfig
}

type runtimeClusterPodTerminalAuthorizationReference struct {
	ClusterID          string
	ClusterKubeconfig  string
	Namespace          string
	Name               string
	ProjectID          string
	ApplicationID      string
	DeploymentTargetID string
	ReleaseID          string
}

func runtimeClusterPodTerminalReference(cluster model.RuntimeCluster, snapshot kubeprovider.ResourceSnapshot) runtimeClusterPodTerminalAuthorizationReference {
	return runtimeClusterPodTerminalAuthorizationReference{
		ClusterID:          cluster.ID,
		ClusterKubeconfig:  cluster.KubeconfigRef,
		Namespace:          snapshot.Namespace,
		Name:               snapshot.Name,
		ProjectID:          snapshot.ProjectID,
		ApplicationID:      snapshot.ApplicationID,
		DeploymentTargetID: snapshot.DeploymentTargetID,
		ReleaseID:          snapshot.ReleaseID,
	}
}

func (h *Handlers) runtimeClusterPodTerminalAuthorizationAllowed(ctx context.Context, user model.User, client *kubeprovider.Client, reference runtimeClusterPodTerminalAuthorizationReference) bool {
	if !authz.IsPlatformAdmin(user.Role) {
		return false
	}
	var cluster model.RuntimeCluster
	if err := runtimecluster.ActiveScope(h.dbWithContext(ctx)).First(&cluster, "id = ? and type in ?", reference.ClusterID, []string{"kubernetes", "k3s"}).Error; err != nil || cluster.KubeconfigRef != reference.ClusterKubeconfig {
		return false
	}
	resourceCtx, cancel := context.WithTimeout(ctx, runtimeTerminalResourceCheckTimeout)
	defer cancel()
	snapshot, err := client.GetManagedResource(resourceCtx, "pod", reference.Namespace, reference.Name)
	if err != nil || !sameRuntimeClusterPodTerminalResource(reference, snapshot) {
		return false
	}
	return h.runtimeClusterPodWebConsoleAllowed(resourceCtx, snapshot)
}

func sameRuntimeClusterPodTerminalResource(reference runtimeClusterPodTerminalAuthorizationReference, snapshot kubeprovider.ResourceSnapshot) bool {
	return snapshot.Namespace == reference.Namespace &&
		snapshot.Name == reference.Name &&
		snapshot.ProjectID == reference.ProjectID &&
		snapshot.ApplicationID == reference.ApplicationID &&
		snapshot.DeploymentTargetID == reference.DeploymentTargetID &&
		snapshot.ReleaseID == reference.ReleaseID
}

func (h *Handlers) runtimeClusterPodWebConsoleAllowed(ctx context.Context, snapshot kubeprovider.ResourceSnapshot) bool {
	projectID := strings.TrimSpace(snapshot.ProjectID)
	targetID := strings.TrimSpace(snapshot.DeploymentTargetID)
	if projectID == "" {
		return targetID == ""
	}
	db := h.dbWithContext(ctx).WithContext(ctx)
	var project model.Project
	if err := db.First(&project, "id = ?", projectID).Error; err != nil || !resourceCanMutateDuringDelete(project.DeleteStatus) || !project.WebConsoleEnabled {
		return false
	}
	if targetID == "" {
		return true
	}
	query := db.Where("id = ? and project_id = ?", targetID, projectID)
	if applicationID := strings.TrimSpace(snapshot.ApplicationID); applicationID != "" {
		query = query.Where("application_id = ?", applicationID)
	}
	var target model.DeploymentTarget
	if err := query.First(&target).Error; err != nil || !resourceCanMutateDuringDelete(target.DeleteStatus) {
		return false
	}
	return runtimeWebConsoleEnabled(project, target)
}

func (h *Handlers) ensureRuntimeClusterPodWebConsoleEnabled(ctx *gin.Context, snapshot kubeprovider.ResourceSnapshot) bool {
	if h.runtimeClusterPodWebConsoleAllowed(ctx.Request.Context(), snapshot) {
		return true
	}
	writeErrorCode(ctx, http.StatusForbidden, "runtime.web_console_disabled", "web console is disabled for this cluster resource")
	return false
}

func runtimeClusterForDeploymentTargetDB(db *gorm.DB, target model.DeploymentTarget) (model.RuntimeCluster, error) {
	var cluster model.RuntimeCluster
	query := runtimecluster.ActiveScope(db)
	if clusterID := strings.TrimSpace(target.ClusterID); clusterID != "" {
		err := query.First(&cluster, "id = ? and type in ?", clusterID, []string{"kubernetes", "k3s"}).Error
		return cluster, err
	}
	err := query.Where("scope = ? and is_default = ? and type in ?", "global", true, []string{"kubernetes", "k3s"}).First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = query.Where("scope = ? and type in ?", "global", []string{"kubernetes", "k3s"}).Order("created_at asc").First(&cluster).Error
	}
	return cluster, err
}
