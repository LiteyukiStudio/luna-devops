package worker

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	dnsprovider "github.com/LiteyukiStudio/devops/internal/provider/dns"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/provider/networkpolicy"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
)

func (r *Runner) handleGatewayApply(ctx context.Context, task *asynq.Task) error {
	var payload tasks.GatewayApplyPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}

	var route model.GatewayRoute
	if err := r.db.First(&route, "id = ? and project_id = ?", payload.GatewayRouteID, payload.ProjectID).Error; err != nil {
		return err
	}
	var project model.Project
	if err := r.db.First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		return err
	}
	var application model.Application
	if err := r.db.First(&application, "id = ? and project_id = ?", route.ApplicationID, payload.ProjectID).Error; err != nil {
		return err
	}
	if !applicationRuntimeCanMutate(application) {
		return nil
	}
	var target model.DeploymentTarget
	if err := r.db.First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, payload.ProjectID).Error; err != nil {
		return err
	}
	environment := deploymentTargetEnvironment(target)
	if !route.Enabled {
		if err := r.cleanupGatewayRuntimeResources(ctx, route); err != nil {
			_ = r.db.Model(&route).Updates(map[string]any{"status": "failed"}).Error
			return err
		}
		return r.db.Model(&route).Updates(map[string]any{"status": "disabled"}).Error
	}

	namespace := deploymentNamespace(project, environment)
	if err := r.ensureProjectNamespace(ctx, namespace, project, environment); err != nil {
		_ = r.db.Model(&route).Updates(map[string]any{"status": "failed"}).Error
		return err
	}
	if err := r.applyGatewayIngress(ctx, route, project, application, environment, namespace); err != nil {
		_ = r.db.Model(&route).Updates(map[string]any{"status": "failed"}).Error
		return err
	}
	certificateStatus, err := r.applyGatewayCertificate(ctx, route, project, namespace)
	if err != nil {
		_ = r.db.Model(&route).Updates(map[string]any{"status": "failed", "certificate_status": "failed"}).Error
		return err
	}
	updates := map[string]any{"status": "active", "dns_status": r.gatewayDNSStatus(ctx, route)}
	if certificateStatus != "" {
		updates["certificate_status"] = certificateStatus
	}
	return r.db.Model(&route).Updates(updates).Error
}

func (r *Runner) ensureProjectNamespace(ctx context.Context, namespace string, project model.Project, environment model.Environment) error {
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return err
	}
	if err := manager.EnsureNamespace(ctx, namespace, kubeprovider.ProjectNamespaceLabels(project.ID)); err != nil {
		return err
	}
	if r.buildEgressMode != "restricted" {
		return manager.EnsureBuildPolicy(ctx, networkpolicy.PermissiveBuildPolicy(namespace))
	}
	return manager.EnsureBuildPolicy(ctx, networkpolicy.BuildPolicyWithEgressControlsAndPorts(namespace, r.buildPrivateEgressCIDRs, r.buildPrivateEgressPorts, r.buildBlockedEgressCIDRs))
}

func (r *Runner) applyGatewayIngress(ctx context.Context, route model.GatewayRoute, project model.Project, application model.Application, environment model.Environment, namespace string) error {
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return err
	}
	return manager.ApplyGatewayIngress(ctx, gatewayIngressSpec(route, project, application, environment, namespace, r.gatewayServiceName(route, application, environment)))
}

func (r *Runner) gatewayServiceName(route model.GatewayRoute, application model.Application, environment model.Environment) string {
	var target model.DeploymentTarget
	query := r.db.Where("project_id = ? and application_id = ? and enabled = ?", route.ProjectID, application.ID, true)
	if strings.TrimSpace(route.DeploymentTargetID) != "" {
		query = query.Where("id = ?", strings.TrimSpace(route.DeploymentTargetID))
	} else {
		query = query.Order("created_at asc")
	}
	err := query.First(&target).Error
	if err == nil {
		return applicationResourceName(target)
	}
	return dnsLabel(application.Slug)
}

func (r *Runner) gatewayDNSStatus(ctx context.Context, route model.GatewayRoute) string {
	if err := dnsprovider.CheckCNAME(ctx, r.dnsResolver, route.Host, route.CNAMETarget); err != nil {
		return "failed"
	}
	return "verified"
}

func (r *Runner) applyGatewayCertificate(ctx context.Context, route model.GatewayRoute, project model.Project, namespace string) (string, error) {
	if strings.TrimSpace(route.TLSMode) != "http-challenge" {
		return "", nil
	}
	manager, err := r.kubernetesManager(model.Environment{})
	if err != nil {
		return "", err
	}
	spec := gatewayCertificateSpec(route, project, namespace, r.certManagerClusterIssuer)
	if err := manager.ApplyCertificate(ctx, spec); err != nil {
		return "", err
	}
	snapshot, err := manager.GetCertificateSnapshot(ctx, spec.Namespace, spec.Name)
	if err != nil {
		return "", err
	}
	return snapshot.Phase, nil
}
