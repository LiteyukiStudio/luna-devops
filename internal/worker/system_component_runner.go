package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
)

const (
	systemComponentGatewayTrafficProbe = model.GatewayTrafficProbeApplicationSlug
	systemComponentNamespaceDefault    = "liteyuki-system"
)

type systemComponentConfig struct {
	APIBaseURL        string `json:"apiBaseUrl"`
	Image             string `json:"image"`
	TraefikMetricsURL string `json:"traefikMetricsUrl"`
}

func (r *Runner) handleSystemComponentApply(ctx context.Context, task *asynq.Task) error {
	var payload tasks.SystemComponentApplyPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if strings.TrimSpace(payload.InstallationID) == "" {
		return errors.New("system component installation id is required")
	}

	var installation model.SystemComponentInstallation
	if err := r.db.First(&installation, "id = ? and component_id = ? and runtime_cluster_id = ?", payload.InstallationID, payload.ComponentID, payload.ClusterID).Error; err != nil {
		return err
	}

	var cluster model.RuntimeCluster
	if err := r.db.First(&cluster, "id = ? and type in ?", installation.RuntimeClusterID, []string{"kubernetes", "k3s"}).Error; err != nil {
		_ = r.markSystemComponentApplyFailed(installation.ID, err)
		return err
	}
	kubeconfig := r.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		err := errors.New("runtime cluster kubeconfig is missing")
		_ = r.markSystemComponentApplyFailed(installation.ID, err)
		return err
	}
	manager, err := r.namespaceFactory(kubeconfig)
	if err != nil {
		err = runtimeClusterKubeconfigError(err)
		_ = r.markSystemComponentApplyFailed(installation.ID, err)
		return err
	}

	var cfg systemComponentConfig
	if strings.TrimSpace(installation.Config) != "" {
		if err := json.Unmarshal([]byte(installation.Config), &cfg); err != nil {
			_ = r.markSystemComponentApplyFailed(installation.ID, err)
			return err
		}
	}

	switch installation.ComponentID {
	case systemComponentGatewayTrafficProbe:
		err = manager.ApplyGatewayTrafficProbe(ctx, kubeprovider.GatewayTrafficProbeSpec{
			Name:              model.GatewayTrafficProbeServiceAccountName,
			Namespace:         firstNonEmpty(installation.Namespace, systemComponentNamespaceDefault),
			RuntimeClusterID:  installation.RuntimeClusterID,
			Image:             cfg.Image,
			APIBaseURL:        cfg.APIBaseURL,
			ReportToken:       payload.ReportToken,
			ControllerType:    firstNonEmpty(installation.ControllerType, cluster.GatewayControllerType, "traefik"),
			Mode:              firstNonEmpty(installation.Mode, "traefik-metrics"),
			GatewayNamespace:  firstNonEmpty(cluster.GatewayNamespace, "kube-system"),
			TraefikMetricsURL: cfg.TraefikMetricsURL,
		})
	default:
		err = fmt.Errorf("unsupported system component %s", installation.ComponentID)
	}
	if err != nil {
		_ = r.markSystemComponentApplyFailed(installation.ID, err)
		return err
	}

	now := time.Now()
	return r.db.Model(&model.SystemComponentInstallation{}).
		Where("id = ?", installation.ID).
		Updates(map[string]any{
			"status":     "deployed",
			"message":    "system component resources applied",
			"last_error": "",
			"updated_at": now,
		}).Error
}

func (r *Runner) markSystemComponentApplyFailed(installationID string, applyErr error) error {
	if strings.TrimSpace(installationID) == "" || applyErr == nil {
		return nil
	}
	return r.db.Model(&model.SystemComponentInstallation{}).
		Where("id = ?", installationID).
		Updates(map[string]any{
			"status":     "failed",
			"message":    applyErr.Error(),
			"last_error": applyErr.Error(),
			"updated_at": time.Now(),
		}).Error
}
