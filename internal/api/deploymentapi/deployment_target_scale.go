package deploymentapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const deploymentTargetScaleTimeout = 10 * time.Second

type applicationWorkloadScaler interface {
	ScaleApplicationWorkload(ctx context.Context, spec kubeprovider.ApplicationResourcesSpec, replicas int32) error
}

type deploymentTargetScaleError struct {
	cause error
}

func (e *deploymentTargetScaleError) Error() string { return e.cause.Error() }
func (e *deploymentTargetScaleError) Unwrap() error { return e.cause }

type deploymentTargetScaleOutcome struct {
	attempted     bool
	workloadFound bool
}

func (h *Handlers) scaleDeploymentTargetReplicas(ctx context.Context, project model.Project, current model.DeploymentTarget, desired int) (bool, error) {
	cluster, err := h.runtimeClusterForDeploymentTargetValue(current, ctx)
	if err != nil {
		return false, &deploymentTargetScaleError{cause: fmt.Errorf("resolve runtime cluster: %w", err)}
	}
	kubeconfig := h.secrets.ResolveContext(ctx, cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		return false, &deploymentTargetScaleError{cause: errors.New("runtime cluster kubeconfig is not configured")}
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return false, &deploymentTargetScaleError{cause: fmt.Errorf("create runtime cluster client: %w", err)}
	}
	scaleCtx, cancel := context.WithTimeout(ctx, deploymentTargetScaleTimeout)
	defer cancel()
	found, err := scaleDeploymentTargetWorkload(scaleCtx, client, deploymentTargetNamespace(project, current), current, desired)
	if err != nil {
		return false, &deploymentTargetScaleError{cause: err}
	}
	return found, nil
}

func scaleDeploymentTargetWorkload(ctx context.Context, scaler applicationWorkloadScaler, namespace string, target model.DeploymentTarget, desired int) (bool, error) {
	if desired < 0 {
		return false, errors.New("deployment replicas cannot be negative")
	}
	err := scaler.ScaleApplicationWorkload(ctx, kubeprovider.ApplicationResourcesSpec{
		Name:               deploymentTargetResourceName(target),
		Namespace:          namespace,
		ProjectID:          target.ProjectID,
		ApplicationID:      target.ApplicationID,
		DeploymentTargetID: target.ID,
		WorkloadType:       normalizeWorkloadType(target.WorkloadType),
	}, int32(desired))
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func writeDeploymentTargetScaleError(ctx *gin.Context, err error) {
	var ownershipError *kubeprovider.ResourceOwnershipConflictError
	if errors.As(err, &ownershipError) {
		writeErrorCode(ctx, http.StatusConflict, kubeprovider.ResourceOwnershipConflictCode, "The runtime workload belongs to another resource lifecycle.")
		return
	}
	status := http.StatusBadGateway
	if apierrors.IsConflict(err) {
		status = http.StatusConflict
	} else if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	writeArgumentErrorCode(ctx, status, "deployment_target.scale_failed", "The deployment replica count could not be applied to the runtime cluster.", "replicas", nil, true)
}
