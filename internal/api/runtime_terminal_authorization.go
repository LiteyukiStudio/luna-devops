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
	UserID                    string
	SessionID                 string
	AssertionID               string
	AssertionRequired         bool
	AssertionAbsoluteDeadline time.Time
	Deadline                  time.Time
}

type runtimeTerminalAuthorizationState struct {
	Session              model.UserSession
	User                 model.User
	Assertion            model.StepUpAssertion
	AuthorizationAllowed bool
}

func (state runtimeTerminalAuthorizationState) active(binding runtimeTerminalAuthorizationBinding, now time.Time) bool {
	return state.AuthorizationAllowed && state.identityActive(binding, now)
}

func (state runtimeTerminalAuthorizationState) identityActive(binding runtimeTerminalAuthorizationBinding, now time.Time) bool {
	if !binding.Deadline.After(now) {
		return false
	}
	if state.Session.ID != binding.SessionID || state.Session.UserID != binding.UserID || !state.Session.ExpiresAt.After(now) {
		return false
	}
	if state.User.ID != binding.UserID || state.User.Disabled {
		return false
	}
	if !binding.AssertionRequired {
		return true
	}
	return state.Assertion.ID == binding.AssertionID &&
		state.Assertion.UserID == binding.UserID &&
		state.Assertion.SessionID == binding.SessionID &&
		state.Assertion.Purpose == stepUpPurposeRuntimeTerminal &&
		stepUpAssertionActive(state.Assertion, now) &&
		!state.Assertion.AbsoluteExpiresAt.After(binding.AssertionAbsoluteDeadline)
}

func (h *Handlers) requireRuntimeTerminalAuthorization(ctx *gin.Context, user model.User) (runtimeTerminalAuthorizationBinding, bool) {
	if !requireInteractiveSession(ctx) {
		return runtimeTerminalAuthorizationBinding{}, false
	}
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok || session.UserID != user.ID {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return runtimeTerminalAuthorizationBinding{}, false
	}

	binding := runtimeTerminalAuthorizationBinding{
		UserID:    user.ID,
		SessionID: session.ID,
		Deadline:  session.ExpiresAt,
	}
	if !h.stepUpMFAEnabled() {
		return binding, true
	}
	if !h.requireMFAAssertion(ctx, user, stepUpPurposeRuntimeTerminal) {
		return runtimeTerminalAuthorizationBinding{}, false
	}

	now := time.Now()
	var assertion model.StepUpAssertion
	if err := h.db.First(
		&assertion,
		"user_id = ? and session_id = ? and purpose = ? and idle_expires_at > ? and absolute_expires_at > ?",
		user.ID,
		session.ID,
		stepUpPurposeRuntimeTerminal,
		now,
		now,
	).Error; err != nil || !stepUpAssertionActive(assertion, now) {
		writeMFARequired(ctx, stepUpPurposeRuntimeTerminal)
		return runtimeTerminalAuthorizationBinding{}, false
	}
	binding.AssertionID = assertion.ID
	binding.AssertionRequired = true
	binding.AssertionAbsoluteDeadline = assertion.AbsoluteExpiresAt
	if assertion.AbsoluteExpiresAt.Before(binding.Deadline) {
		binding.Deadline = assertion.AbsoluteExpiresAt
	}
	return binding, true
}

func (h *Handlers) monitorRuntimeTerminalAuthorization(
	ctx context.Context,
	binding runtimeTerminalAuthorizationBinding,
	authorizationAllowed func(context.Context, model.User) bool,
	cancel context.CancelFunc,
) <-chan struct{} {
	revoked := make(chan struct{})
	go func() {
		ticker := time.NewTicker(runtimeTerminalAuthorizationInterval)
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
	db := h.db.WithContext(ctx)
	_ = db.First(&state.Session, "id = ? and user_id = ?", binding.SessionID, binding.UserID).Error
	_ = db.First(&state.User, "id = ? and disabled = ?", binding.UserID, false).Error
	if binding.AssertionRequired {
		_ = db.First(&state.Assertion, "id = ? and user_id = ? and session_id = ? and purpose = ?", binding.AssertionID, binding.UserID, binding.SessionID, stepUpPurposeRuntimeTerminal).Error
	} else if h.stepUpMFAEnabled() {
		return false
	}
	if !state.identityActive(binding, now) {
		return false
	}
	state.AuthorizationAllowed = authorizationAllowed(ctx, state.User)
	now = time.Now()
	if !state.active(binding, now) {
		return false
	}
	if !binding.AssertionRequired {
		return true
	}

	idleTimeout, _ := h.stepUpTimeouts()
	idleExpiresAt := refreshedStepUpIdleExpiry(now, idleTimeout, binding.AssertionAbsoluteDeadline)
	result := db.Model(&model.StepUpAssertion{}).
		Where("id = ? and user_id = ? and session_id = ? and purpose = ? and idle_expires_at > ? and absolute_expires_at > ?", binding.AssertionID, binding.UserID, binding.SessionID, stepUpPurposeRuntimeTerminal, now, now).
		Updates(map[string]any{"last_activity_at": now, "idle_expires_at": idleExpiresAt, "updated_at": now})
	return result.Error == nil && result.RowsAffected == 1
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
	db := h.db.WithContext(ctx)
	var project model.Project
	if err := db.First(&project, "id = ?", reference.ProjectID).Error; err != nil || !resourceCanMutateDuringDelete(project.DeleteStatus) {
		return false
	}
	if !authz.IsPlatformAdmin(user.Role) {
		var member model.ProjectMember
		if err := db.First(&member, "project_id = ? and user_id = ?", reference.ProjectID, user.ID).Error; err != nil || !projectUserRoleAllowed(user, member.Role, []string{"owner", "admin", "developer"}) {
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
	if err := h.db.WithContext(ctx).First(&cluster, "id = ? and type in ?", reference.ClusterID, []string{"kubernetes", "k3s"}).Error; err != nil || cluster.KubeconfigRef != reference.ClusterKubeconfig {
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
	db := h.db.WithContext(ctx)
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
