package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	dnsprovider "github.com/LiteyukiStudio/devops/internal/provider/dns"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/provider/networkpolicy"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
)

func (r *Runner) handleGatewayApply(ctx context.Context, task *asynq.Task) (err error) {
	startedAt := time.Now()
	operation := "apply"
	var route model.GatewayRoute
	defer func() {
		result := "succeeded"
		if err != nil {
			result = "failed"
			if route.ID != "" {
				r.emitGatewayApplyFailed(ctx, route, err.Error())
			}
		}
		r.recordGatewaySyncMetric(operation, result, startedAt)
		r.refreshGatewayRouteMetrics()
	}()

	var payload tasks.GatewayApplyPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}

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
		operation = "disable"
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
	if err := r.applyGatewayAPIResources(ctx, route, project, application, environment, namespace); err != nil {
		_ = r.db.Model(&route).Updates(map[string]any{"status": "failed"}).Error
		return err
	}
	certificateSnapshot, certificateConfigured, err := r.gatewayCertificateSnapshot(ctx, route, project, environment, namespace)
	if err != nil {
		failureUpdates := gatewayCertificateFailureUpdates(err)
		failureUpdates["status"] = "failed"
		_ = r.db.Model(&route).Updates(failureUpdates).Error
		return err
	}
	updates := map[string]any{"status": "active", "dns_status": r.gatewayDNSStatus(ctx, route)}
	if certificateConfigured {
		cluster, clusterErr := r.runtimeClusterForEnvironment(environment)
		if clusterErr != nil {
			return clusterErr
		}
		for key, value := range gatewayCertificateRuntimeUpdates(certificateSnapshot, cluster, r.certManagerClusterIssuer) {
			updates[key] = value
		}
	} else {
		updates["certificate_status"] = "disabled"
		updates["certificate_message"] = ""
		updates["certificate_not_after"] = nil
		updates["certificate_issuer_kind"] = ""
		updates["certificate_issuer_name"] = ""
	}
	return r.db.Model(&route).Updates(updates).Error
}

func gatewayCertificateFailureUpdates(err error) map[string]any {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return map[string]any{
		"certificate_status":    "failed",
		"certificate_message":   message,
		"certificate_not_after": nil,
	}
}

func gatewayCertificateRuntimeUpdates(snapshot kubeprovider.CertificateSnapshot, cluster model.RuntimeCluster, fallbackIssuer string) map[string]any {
	return map[string]any{
		"certificate_status":      snapshot.Phase,
		"certificate_message":     snapshot.Message,
		"certificate_not_after":   snapshot.NotAfter,
		"certificate_issuer_kind": gatewayCertificateIssuerKind(cluster),
		"certificate_issuer_name": gatewayCertificateIssuerName(cluster, fallbackIssuer),
	}
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

func (r *Runner) applyGatewayAPIResources(ctx context.Context, route model.GatewayRoute, project model.Project, application model.Application, environment model.Environment, namespace string) error {
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return err
	}
	cluster, err := r.runtimeClusterForEnvironment(environment)
	if err != nil {
		return err
	}
	spec, err := httpRouteSpec(route, project, application, environment, cluster, namespace, r.gatewayServiceName(route, application, environment))
	if err != nil {
		return err
	}
	if err := ensureGatewayBackendAvailable(ctx, manager, spec); err != nil {
		return err
	}
	gateway := gatewaySpec(cluster, project.ID)
	if certificate, ok, err := r.ensureGatewayWildcardCertificate(ctx, manager, project, cluster, namespace); err != nil {
		return err
	} else if ok {
		gateway.TLSSecretRefs = append(gateway.TLSSecretRefs, kubeprovider.GatewayTLSSecretRef{
			Name:      certificate.SecretName,
			Namespace: certificate.Namespace,
		})
	}
	if certificate, ok, err := r.ensureGatewayCertificate(ctx, manager, route, project, cluster, namespace); err != nil {
		return err
	} else if ok {
		gateway.TLSSecretRefs = append(gateway.TLSSecretRefs, kubeprovider.GatewayTLSSecretRef{
			Name:      certificate.SecretName,
			Namespace: certificate.Namespace,
		})
	}
	if err := manager.EnsureGateway(ctx, gateway); err != nil {
		return err
	}
	if err := manager.ApplyHTTPRoute(ctx, spec); err != nil {
		return err
	}
	return r.waitForHTTPRouteAccepted(ctx, manager, spec.Namespace, spec.Name)
}

func ensureGatewayBackendAvailable(ctx context.Context, manager kubeprovider.NamespaceManager, spec kubeprovider.HTTPRouteSpec) error {
	if strings.TrimSpace(spec.RequestRedirect) != "" {
		return nil
	}
	snapshot, err := manager.GetServiceBackendSnapshot(ctx, spec.Namespace, spec.ServiceName, spec.ServicePort)
	if err != nil {
		return fmt.Errorf("访问入口后端服务检查失败: %w", err)
	}
	if !snapshot.ServiceExists {
		return fmt.Errorf("后端 Service %s/%s 不存在，请先重新发布部署配置以恢复 Service 后再重新启用访问入口", spec.Namespace, spec.ServiceName)
	}
	if !snapshot.PortExists {
		return fmt.Errorf("后端 Service %s/%s 未暴露端口 %d，请调整部署配置并重新发布后再重新启用访问入口", spec.Namespace, spec.ServiceName, spec.ServicePort)
	}
	return nil
}

func (r *Runner) ensureGatewayWildcardCertificate(ctx context.Context, manager kubeprovider.NamespaceManager, project model.Project, cluster model.RuntimeCluster, namespace string) (kubeprovider.CertificateSpec, bool, error) {
	if strings.TrimSpace(cluster.GatewayExternalTLSMode) != "gateway" {
		return kubeprovider.CertificateSpec{}, false, nil
	}
	spec, ok := gatewayWildcardCertificateSpec(cluster, project, namespace, gatewayCertificateIssuerName(cluster, r.certManagerClusterIssuer))
	if !ok {
		return kubeprovider.CertificateSpec{}, false, nil
	}
	if err := manager.ApplyCertificate(ctx, spec); err != nil {
		return kubeprovider.CertificateSpec{}, false, err
	}
	return spec, true, nil
}

func (r *Runner) waitForHTTPRouteAccepted(ctx context.Context, manager kubeprovider.NamespaceManager, namespace string, name string) error {
	timeout := time.NewTimer(4 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := manager.GetHTTPRouteStatus(ctx, namespace, name)
		if err == nil {
			switch strings.TrimSpace(status.Summary) {
			case "accepted":
				return nil
			case "failed":
				return fmt.Errorf("HTTPRoute was applied but Gateway API reported failed status: %s", routeConditionSummary(status.Conditions))
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return nil
		case <-ticker.C:
		}
	}
}

func routeConditionSummary(conditions []kubeprovider.RouteConditionSnapshot) string {
	parts := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		if condition.Type == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(fmt.Sprintf("%s=%s reason=%s message=%s", condition.Type, condition.Status, condition.Reason, condition.Message)))
	}
	if len(parts) == 0 {
		return "no conditions"
	}
	return strings.Join(parts, "; ")
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

func (r *Runner) ensureGatewayCertificate(ctx context.Context, manager kubeprovider.NamespaceManager, route model.GatewayRoute, project model.Project, cluster model.RuntimeCluster, namespace string) (kubeprovider.CertificateSpec, bool, error) {
	if strings.TrimSpace(route.TLSMode) != "http-challenge" {
		return kubeprovider.CertificateSpec{}, false, nil
	}
	spec := gatewayCertificateSpec(
		route,
		project,
		gatewayCertificateNamespace(cluster, namespace),
		gatewayCertificateIssuerKind(cluster),
		gatewayCertificateIssuerName(cluster, r.certManagerClusterIssuer),
	)
	if err := manager.ApplyCertificate(ctx, spec); err != nil {
		return kubeprovider.CertificateSpec{}, false, err
	}
	return spec, true, nil
}

func (r *Runner) gatewayCertificateSnapshot(ctx context.Context, route model.GatewayRoute, project model.Project, environment model.Environment, namespace string) (kubeprovider.CertificateSnapshot, bool, error) {
	if strings.TrimSpace(route.TLSMode) != "http-challenge" {
		return kubeprovider.CertificateSnapshot{}, false, nil
	}
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return kubeprovider.CertificateSnapshot{}, true, err
	}
	cluster, err := r.runtimeClusterForEnvironment(environment)
	if err != nil {
		return kubeprovider.CertificateSnapshot{}, true, err
	}
	spec := gatewayCertificateSpec(
		route,
		project,
		gatewayCertificateNamespace(cluster, namespace),
		gatewayCertificateIssuerKind(cluster),
		gatewayCertificateIssuerName(cluster, r.certManagerClusterIssuer),
	)
	snapshot, err := manager.GetCertificateSnapshot(ctx, spec.Namespace, spec.Name)
	if err != nil {
		return kubeprovider.CertificateSnapshot{}, true, err
	}
	return snapshot, true, nil
}
