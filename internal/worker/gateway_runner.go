package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	dnsprovider "github.com/LiteyukiStudio/devops/internal/provider/dns"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/provider/networkpolicy"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
)

var errGatewayApplyTaskStale = errors.New("gateway apply task generation is stale")

func (r *Runner) handleGatewayApply(ctx context.Context, task *asynq.Task) (err error) {
	startedAt := time.Now()
	operation := "apply"
	var route model.GatewayRoute
	var payload tasks.GatewayApplyPayload
	defer func() {
		result := "succeeded"
		if errors.Is(err, errGatewayApplyTaskStale) {
			result = "skipped"
		} else if err != nil {
			result = "failed"
			if route.ID != "" {
				r.emitGatewayApplyFailed(ctx, route, payload.ActorID, err.Error())
			}
		}
		r.recordGatewaySyncMetric(ctx, operation, result, startedAt)
	}()

	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}

	if err := r.db.First(&route, "id = ? and project_id = ?", payload.GatewayRouteID, payload.ProjectID).Error; err != nil {
		return err
	}
	if !gatewayApplyPayloadMatchesRoute(payload, route) {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, errGatewayApplyTaskStale)
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
			return err
		}
		route.Status = "disabled"
		r.emitGatewayEvent(ctx, route, payload.ActorID, "applied", "Gateway route disabled")
		return nil
	}

	namespace := deploymentNamespace(project, environment)
	if err := workerStage(ctx, "gateway.ensure_namespace", func(stageCtx context.Context) error {
		return r.ensureProjectNamespace(stageCtx, namespace, project, environment)
	}); err != nil {
		return err
	}
	if err := workerStage(ctx, "gateway.apply_resources", func(stageCtx context.Context) error {
		return r.applyGatewayAPIResources(stageCtx, route, project, application, environment, namespace)
	}); err != nil {
		return err
	}
	type certificateResult struct {
		snapshot   kubeprovider.CertificateSnapshot
		configured bool
	}
	certificate, err := workerStageValue(ctx, "gateway.observe_certificate", func(stageCtx context.Context) (certificateResult, error) {
		snapshot, configured, observeErr := r.gatewayCertificateSnapshot(stageCtx, route, project, environment, namespace)
		return certificateResult{snapshot: snapshot, configured: configured}, observeErr
	})
	if err != nil {
		failedRoute := route
		failedRoute.CertificateStatus = kubeprovider.CertificateFailed
		failedRoute.CertificateMessage = err.Error()
		r.emitCertificateEvent(ctx, failedRoute, payload.ActorID, kubeprovider.CertificateFailed, err.Error())
		return err
	}
	certificateSnapshot := certificate.snapshot
	certificateConfigured := certificate.configured
	route.DNSStatus = r.gatewayDNSStatus(ctx, route)
	if certificateConfigured {
		cluster, clusterErr := r.runtimeClusterForEnvironment(ctx, environment)
		if clusterErr != nil {
			return clusterErr
		}
		route.CertificateStatus = certificateSnapshot.Phase
		route.CertificateMessage = certificateSnapshot.Message
		route.CertificateNotAfter = certificateSnapshot.NotAfter
		route.CertificateIssuerKind = gatewayCertificateIssuerKind(cluster)
		route.CertificateIssuerName = gatewayCertificateIssuerName(cluster, r.certManagerClusterIssuer)
		r.emitCertificateEvent(ctx, route, payload.ActorID, certificateSnapshot.Phase, certificateSnapshot.Message)
	} else {
		route.CertificateStatus = "disabled"
	}
	route.Status = "active"
	r.emitGatewayEvent(ctx, route, payload.ActorID, "applied", "Gateway route applied")
	return nil
}

func gatewayApplyPayloadMatchesRoute(payload tasks.GatewayApplyPayload, route model.GatewayRoute) bool {
	return payload.RouteUpdatedAtUnixMicro > 0 &&
		!route.UpdatedAt.IsZero() &&
		payload.RouteUpdatedAtUnixMicro == route.UpdatedAt.UTC().UnixMicro()
}

func (r *Runner) ensureProjectNamespace(ctx context.Context, namespace string, project model.Project, environment model.Environment) error {
	manager, err := r.kubernetesManager(ctx, environment)
	if err != nil {
		return err
	}
	if err := manager.EnsureNamespace(ctx, namespace, kubeprovider.ProjectNamespaceLabels(project.ID)); err != nil {
		return err
	}
	if r.buildEgressMode == "permissive" {
		return manager.EnsureBuildPolicy(ctx, networkpolicy.PermissiveBuildPolicy(namespace))
	}
	return manager.EnsureBuildPolicy(ctx, networkpolicy.BuildPolicyWithEgressControlsAndPorts(namespace, r.buildPrivateEgressCIDRs, r.buildPrivateEgressPorts, r.buildBlockedEgressCIDRs))
}

func (r *Runner) applyGatewayAPIResources(ctx context.Context, route model.GatewayRoute, project model.Project, application model.Application, environment model.Environment, namespace string) error {
	manager, err := r.kubernetesManager(ctx, environment)
	if err != nil {
		return err
	}
	cluster, err := r.runtimeClusterForEnvironment(ctx, environment)
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
	return dnsLabel(application.Identifier)
}

func (r *Runner) gatewayDNSStatus(ctx context.Context, route model.GatewayRoute) string {
	ctx, end := telemetry.StartOperation(ctx, "worker", "gateway.check_dns")
	err := dnsprovider.CheckCNAME(ctx, r.dnsResolver, route.Host, route.CNAMETarget)
	end(err)
	if err != nil {
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
	manager, err := r.kubernetesManager(ctx, environment)
	if err != nil {
		return kubeprovider.CertificateSnapshot{}, true, err
	}
	cluster, err := r.runtimeClusterForEnvironment(ctx, environment)
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
