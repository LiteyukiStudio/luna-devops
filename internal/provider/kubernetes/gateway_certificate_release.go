package kubernetes

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// certManagerCertificateNameAnnotation 由 cert-manager 写入其签发的 TLS Secret，用于识别可回收的签发产物。
const certManagerCertificateNameAnnotation = "cert-manager.io/certificate-name"

// GatewayCertificateRelease 描述一张需要回收的平台网关 Certificate。
// SecretName 用于 Certificate 已在前一次尝试中删除时继续回收 Secret 与 Gateway 引用，保证重试幂等。
type GatewayCertificateRelease struct {
	GatewayName      string
	GatewayNamespace string
	Namespace        string
	Name             string
	SecretName       string
}

// GatewayCertificateReleaseResult 记录本次回收实际发生的变更；全部为 false 表示已是终态。
type GatewayCertificateReleaseResult struct {
	SecretName         string
	CertificateDeleted bool
	SecretDeleted      bool
	GatewayRefRemoved  bool
}

func (result GatewayCertificateReleaseResult) Changed() bool {
	return result.CertificateDeleted || result.SecretDeleted || result.GatewayRefRemoved
}

// ReleaseGatewayCertificate 删除平台创建的 Certificate；当同命名空间不再有任何 Certificate 写入同一 Secret 时，
// 再从 Gateway listener 移除该 Secret 引用并删除 cert-manager 签发的 Secret。非平台创建的同名 Certificate 不处理。
func (c *Client) ReleaseGatewayCertificate(ctx context.Context, release GatewayCertificateRelease) (GatewayCertificateReleaseResult, error) {
	result := GatewayCertificateReleaseResult{}
	if c.dynamic == nil {
		return result, fmt.Errorf("dynamic kubernetes client is required")
	}
	namespace := strings.TrimSpace(release.Namespace)
	name := strings.TrimSpace(release.Name)
	if namespace == "" || name == "" {
		return result, fmt.Errorf("certificate name and namespace are required")
	}
	secretName := strings.TrimSpace(release.SecretName)
	certificates := c.dynamic.Resource(certificateResource).Namespace(namespace)
	certificate, err := certificates.Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
	case err != nil:
		return result, err
	case !isManagedResource(certificate.GetLabels()):
		return result, nil
	default:
		if value, _, _ := unstructured.NestedString(certificate.Object, "spec", "secretName"); strings.TrimSpace(value) != "" {
			secretName = strings.TrimSpace(value)
		}
		if err := certificates.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return result, err
		}
		result.CertificateDeleted = true
	}
	result.SecretName = secretName
	if secretName == "" {
		return result, nil
	}
	inUse, err := c.certificateSecretInUse(ctx, namespace, name, secretName)
	if err != nil || inUse {
		return result, err
	}
	removed, err := c.removeGatewayCertificateRef(ctx, release.GatewayNamespace, release.GatewayName, namespace, secretName)
	if err != nil {
		return result, err
	}
	result.GatewayRefRemoved = removed
	deleted, err := c.deleteIssuedCertificateSecret(ctx, namespace, secretName)
	if err != nil {
		return result, err
	}
	result.SecretDeleted = deleted
	return result, nil
}

// HasManagedHTTPRoutes 判断集群内是否仍有平台管理且未在删除中的 HTTPRoute，用于泛域名证书的引用计数。
func (c *Client) HasManagedHTTPRoutes(ctx context.Context) (bool, error) {
	if c.dynamic == nil {
		return false, fmt.Errorf("Gateway API dynamic client is not configured")
	}
	items, err := c.dynamic.Resource(httpRouteGVR).List(ctx, metav1.ListOptions{LabelSelector: ManagedByLabel + "=" + ManagedByValue})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, item := range items.Items {
		if item.GetDeletionTimestamp() == nil {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) certificateSecretInUse(ctx context.Context, namespace, releasedName, secretName string) (bool, error) {
	items, err := c.dynamic.Resource(certificateResource).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, item := range items.Items {
		if item.GetName() == releasedName || item.GetDeletionTimestamp() != nil {
			continue
		}
		if value, _, _ := unstructured.NestedString(item.Object, "spec", "secretName"); strings.TrimSpace(value) == secretName {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) deleteIssuedCertificateSecret(ctx context.Context, namespace, name string) (bool, error) {
	secrets := c.client.CoreV1().Secrets(namespace)
	secret, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(secret.Annotations[certManagerCertificateNameAnnotation]) == "" {
		return false, nil
	}
	if err := secrets.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
