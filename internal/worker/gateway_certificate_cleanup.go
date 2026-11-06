package worker

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

const gatewayCertificateReleaseAuditAction = "gateway.certificate_release"

// releaseGatewayCertificates 回收已删除访问入口在网关证书命名空间留下的 Certificate、Secret 与 Gateway 引用。
// 同一 Secret 仍被其他证书写入时只删除本入口的 Certificate；集群仍有平台 HTTPRoute 时保留共享泛域名证书。
// 调用方必须先删除对应 HTTPRoute（或确认项目命名空间已删除），否则泛域名证书的引用计数不会归零。
func (r *Runner) releaseGatewayCertificates(ctx context.Context, manager kubeprovider.NamespaceManager, target model.DeploymentTarget, project model.Project, routes []model.GatewayRoute, actorID string) error {
	if len(routes) == 0 {
		return nil
	}
	cluster, err := r.runtimeClusterForDeploymentTarget(ctx, target)
	if err != nil {
		return err
	}
	certificateNamespace := gatewayCertificateNamespace(cluster, deploymentNamespace(project))
	gateway := gatewaySpec(cluster, project.ID)
	for _, route := range routes {
		release := kubeprovider.GatewayCertificateRelease{
			GatewayName:      gateway.Name,
			GatewayNamespace: gateway.Namespace,
			Namespace:        certificateNamespace,
			Name:             gatewayRuntimeName(route),
		}
		if strings.TrimSpace(route.TLSMode) == "http-challenge" {
			release.SecretName = gatewayTLSSecretName(route)
		}
		if err := r.releaseGatewayCertificate(ctx, manager, release, route.ID, actorID); err != nil {
			return err
		}
	}
	wildcardSecretName := gatewayWildcardCertificateSecretName(cluster)
	if !cluster.GatewayWildcardCertEnabled || wildcardSecretName == "" {
		return nil
	}
	inUse, err := manager.HasManagedHTTPRoutes(ctx)
	if err != nil || inUse {
		return err
	}
	release := kubeprovider.GatewayCertificateRelease{
		GatewayName:      gateway.Name,
		GatewayNamespace: gateway.Namespace,
		Namespace:        certificateNamespace,
		Name:             wildcardSecretName,
		SecretName:       wildcardSecretName,
	}
	return r.releaseGatewayCertificate(ctx, manager, release, cluster.ID, actorID)
}

func (r *Runner) releaseGatewayCertificate(ctx context.Context, manager kubeprovider.NamespaceManager, release kubeprovider.GatewayCertificateRelease, resource string, actorID string) error {
	result, err := manager.ReleaseGatewayCertificate(ctx, release)
	if err != nil {
		return err
	}
	if !result.Changed() {
		return nil
	}
	return r.auditGatewayCertificateRelease(ctx, actorID, resource, release, result)
}

func (r *Runner) auditGatewayCertificateRelease(ctx context.Context, actorID string, resource string, release kubeprovider.GatewayCertificateRelease, result kubeprovider.GatewayCertificateReleaseResult) error {
	raw, err := json.Marshal(map[string]any{
		"certificateNamespace": release.Namespace,
		"certificateName":      release.Name,
		"secretName":           result.SecretName,
		"certificateDeleted":   result.CertificateDeleted,
		"secretDeleted":        result.SecretDeleted,
		"gatewayRefRemoved":    result.GatewayRefRemoved,
	})
	if err != nil {
		return err
	}
	metadata := string(raw)
	return r.db.WithContext(ctx).Create(&model.AuditLog{
		ID:       id.New("aud"),
		UserID:   strings.TrimSpace(actorID),
		Action:   gatewayCertificateReleaseAuditAction,
		Resource: strings.TrimSpace(resource),
		Success:  true,
		Message:  "released",
		Metadata: &metadata,
	}).Error
}
