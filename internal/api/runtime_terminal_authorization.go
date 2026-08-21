package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	runtimeTerminalAuthorizationInterval = 3 * time.Second
	runtimeTerminalResourceCheckTimeout  = 2 * time.Second
)

type runtimeTerminalAuthorizationBinding struct {
	UserID    string
	SubjectID string
	Deadline  time.Time
}

type runtimeTerminalAuthorizationState struct {
	Session              model.UserSession
	OAuthGrant           model.OAuthGrant
	OAuthApplication     model.OAuthApplication
	User                 model.User
	AuthorizationAllowed bool
}

func (state runtimeTerminalAuthorizationState) active(binding runtimeTerminalAuthorizationBinding, now time.Time) bool {
	return state.AuthorizationAllowed && state.identityActive(binding, now)
}

func (state runtimeTerminalAuthorizationState) identityActive(binding runtimeTerminalAuthorizationBinding, now time.Time) bool {
	if !binding.Deadline.After(now) {
		return false
	}
	if grantID, oauth := runtimeTerminalOAuthGrantID(binding.SubjectID); oauth {
		if state.OAuthGrant.ID != grantID ||
			state.OAuthGrant.UserID != binding.UserID ||
			state.OAuthGrant.ApplicationID != lunaCLIApplicationID ||
			state.OAuthGrant.RevokedAt != nil ||
			state.OAuthApplication.ID != lunaCLIApplicationID ||
			state.OAuthApplication.RevokedAt != nil {
			return false
		}
	} else if state.Session.ID != binding.SubjectID || state.Session.UserID != binding.UserID || !state.Session.ExpiresAt.After(now) {
		return false
	}
	if state.User.ID != binding.UserID || state.User.Disabled {
		return false
	}
	return true
}

func (h *Handlers) requireRuntimeTerminalAuthorization(ctx *gin.Context, user model.User) (runtimeTerminalAuthorizationBinding, bool) {
	subject, ok := h.currentInteractiveSubject(ctx, user)
	if !ok {
		if requestUsesBearerToken(ctx) {
			h.auditWithContext(user.ID, "runtime_terminal.session_required", "runtime_terminal", false, "personal access tokens cannot authorize a terminal", ctx.Request.Context())
			writeErrorCode(ctx, http.StatusForbidden, "runtime.terminal_session_required", "个人令牌不能用于终端，请使用浏览器会话或 Luna CLI OAuth 登录")
		} else {
			writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		}
		return runtimeTerminalAuthorizationBinding{}, false
	}

	binding := runtimeTerminalAuthorizationBinding{UserID: user.ID, SubjectID: subject}
	if _, oauth := runtimeTerminalOAuthGrantID(subject); oauth {
		token, tokenOK := currentAccessTokenFromContext(ctx)
		if !tokenOK || token.UserID != user.ID || token.OAuthGrantID == "" {
			writeErrorCode(ctx, http.StatusForbidden, "runtime.terminal_session_required", "Luna CLI OAuth 登录状态无效")
			return runtimeTerminalAuthorizationBinding{}, false
		}
		binding.Deadline = time.Now().Add(time.Hour)
		if token.ExpiresAt != nil && token.ExpiresAt.Before(binding.Deadline) {
			binding.Deadline = *token.ExpiresAt
		}
	} else {
		session, sessionOK := h.currentSessionFromCookie(ctx)
		if !sessionOK || session.UserID != user.ID || session.ID != subject {
			writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
			return runtimeTerminalAuthorizationBinding{}, false
		}
		binding.Deadline = session.ExpiresAt
	}
	return binding, true
}

func (h *Handlers) currentInteractiveSubject(ctx *gin.Context, user model.User) (string, bool) {
	if requestUsesBearerToken(ctx) {
		token, ok := currentAccessTokenFromContext(ctx)
		if !ok || token.UserID != user.ID || token.Source != "oauth" || token.OAuthApplicationID != lunaCLIApplicationID || token.OAuthGrantID == "" {
			return "", false
		}
		return oauthGrantSubject(token.OAuthGrantID), true
	}
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok || session.UserID != user.ID {
		return "", false
	}
	return session.ID, true
}

func oauthGrantSubject(grantID string) string {
	return "oauth:" + strings.TrimSpace(grantID)
}

func runtimeTerminalOAuthGrantID(subject string) (string, bool) {
	grantID := strings.TrimSpace(strings.TrimPrefix(subject, "oauth:"))
	return grantID, strings.HasPrefix(subject, "oauth:") && grantID != ""
}

func (h *Handlers) monitorRuntimeTerminalAuthorization(
	ctx context.Context,
	binding runtimeTerminalAuthorizationBinding,
	authorizationAllowed func(context.Context, model.User) bool,
	cancel context.CancelFunc,
) <-chan struct{} {
	return h.monitorRuntimeTerminalAuthorizationAtInterval(ctx, binding, authorizationAllowed, cancel, runtimeTerminalAuthorizationInterval)
}

func (h *Handlers) monitorRuntimeTerminalAuthorizationAtInterval(
	ctx context.Context,
	binding runtimeTerminalAuthorizationBinding,
	authorizationAllowed func(context.Context, model.User) bool,
	cancel context.CancelFunc,
	interval time.Duration,
) <-chan struct{} {
	revoked := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if h.runtimeTerminalAuthorizationActive(ctx, binding, authorizationAllowed) {
					continue
				}
				close(revoked)
				cancel()
				return
			}
		}
	}()
	return revoked
}

func (h *Handlers) runtimeTerminalAuthorizationActive(
	ctx context.Context,
	binding runtimeTerminalAuthorizationBinding,
	authorizationAllowed func(context.Context, model.User) bool,
) bool {
	now := time.Now()
	state := runtimeTerminalAuthorizationState{}
	db := h.dbWithContext(ctx).WithContext(ctx)
	if grantID, oauth := runtimeTerminalOAuthGrantID(binding.SubjectID); oauth {
		_ = db.First(&state.OAuthGrant, "id = ? and user_id = ? and application_id = ? and revoked_at is null", grantID, binding.UserID, lunaCLIApplicationID).Error
		_ = db.First(&state.OAuthApplication, "id = ? and revoked_at is null", lunaCLIApplicationID).Error
	} else {
		_ = db.First(&state.Session, "id = ? and user_id = ?", binding.SubjectID, binding.UserID).Error
	}
	_ = db.First(&state.User, "id = ? and disabled = ?", binding.UserID, false).Error
	if !state.identityActive(binding, now) {
		return false
	}
	state.AuthorizationAllowed = authorizationAllowed(ctx, state.User)
	now = time.Now()
	if !state.active(binding, now) {
		return false
	}
	return true
}

type runtimeTerminalActivityTracker struct {
	binding runtimeTerminalAuthorizationBinding
}

func (h *Handlers) newRuntimeTerminalActivityTracker(binding runtimeTerminalAuthorizationBinding) *runtimeTerminalActivityTracker {
	return &runtimeTerminalActivityTracker{binding: binding}
}

func (tracker *runtimeTerminalActivityTracker) Record(_ context.Context, now time.Time) bool {
	return tracker == nil || tracker.binding.Deadline.After(now)
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
	if !authz.IsPlatformAdmin(user.Role) {
		var member model.ProjectMember
		if err := db.First(&member, "project_id = ? and user_id = ?", reference.ProjectID, user.ID).Error; err != nil || !projectUserRoleAllowed(user, member.Role, []string{authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper}) {
			return false
		}
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
	if err := h.dbWithContext(ctx).WithContext(ctx).First(&cluster, "id = ? and type in ?", reference.ClusterID, []string{"kubernetes", "k3s"}).Error; err != nil || cluster.KubeconfigRef != reference.ClusterKubeconfig {
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
	if clusterID := strings.TrimSpace(target.ClusterID); clusterID != "" {
		err := db.First(&cluster, "id = ? and type in ?", clusterID, []string{"kubernetes", "k3s"}).Error
		return cluster, err
	}
	err := db.Where("scope = ? and is_default = ? and type in ?", "global", true, []string{"kubernetes", "k3s"}).First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = db.Where("scope = ? and type in ?", "global", []string{"kubernetes", "k3s"}).Order("created_at asc").First(&cluster).Error
	}
	return cluster, err
}
