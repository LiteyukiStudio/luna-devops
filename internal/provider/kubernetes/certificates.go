package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var certificateResource = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "certificates",
}

type CertificateSpec struct {
	Name          string
	Namespace     string
	ProjectID     string
	RouteID       string
	Host          string
	DNSNames      []string
	SecretName    string
	IssuerKind    string
	ClusterIssuer string
}

const (
	CertificatePending = "pending"
	CertificateIssued  = "issued"
	CertificateFailed  = "failed"
	CertificateExpired = "expired"
)

type CertificateSnapshot struct {
	Phase    string
	Message  string
	NotAfter *time.Time
}

func (c *Client) ApplyCertificate(ctx context.Context, spec CertificateSpec) error {
	if err := validateCertificateSpec(spec); err != nil {
		return err
	}
	if c.dynamic == nil {
		return fmt.Errorf("dynamic kubernetes client is required")
	}
	if err := c.DetectCertManagerSupport(ctx); err != nil {
		return err
	}
	certificate := certificateObject(spec)
	resource := c.dynamic.Resource(certificateResource).Namespace(spec.Namespace)
	existing, err := resource.Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, certificate, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.SetLabels(certificate.GetLabels())
	if err := unstructured.SetNestedMap(existing.Object, certificate.Object["spec"].(map[string]any), "spec"); err != nil {
		return err
	}
	_, err = resource.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) DetectCertManagerSupport(ctx context.Context) error {
	if c.dynamic == nil {
		return fmt.Errorf("dynamic kubernetes client is required")
	}
	if _, err := c.dynamic.Resource(certificateResource).List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("cert-manager CRDs are not installed: install cert-manager.io/v1 Certificate before enabling automatic gateway certificates")
		}
		return err
	}
	return nil
}

func (c *Client) GetCertificateSnapshot(ctx context.Context, namespace, name string) (CertificateSnapshot, error) {
	if c.dynamic == nil {
		return CertificateSnapshot{}, fmt.Errorf("dynamic kubernetes client is required")
	}
	certificate, err := c.dynamic.Resource(certificateResource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return CertificateSnapshot{}, err
	}
	return certificateSnapshot(certificate, time.Now()), nil
}

func certificateSnapshot(certificate *unstructured.Unstructured, now time.Time) CertificateSnapshot {
	snapshot := CertificateSnapshot{Phase: CertificatePending, Message: "Certificate is pending"}
	if notAfterText, _, _ := unstructured.NestedString(certificate.Object, "status", "notAfter"); notAfterText != "" {
		if notAfter, err := time.Parse(time.RFC3339, notAfterText); err == nil {
			snapshot.NotAfter = &notAfter
			if now.After(notAfter) {
				snapshot.Phase = CertificateExpired
				snapshot.Message = "Certificate is expired"
				return snapshot
			}
		}
	}
	conditions, _, _ := unstructured.NestedSlice(certificate.Object, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok || condition["type"] != "Ready" {
			continue
		}
		message, _ := condition["message"].(string)
		switch condition["status"] {
		case "True":
			snapshot.Phase = CertificateIssued
			snapshot.Message = firstNonEmpty(message, "Certificate is ready")
		case "False":
			snapshot.Phase = CertificateFailed
			snapshot.Message = firstNonEmpty(message, "Certificate is not ready")
		}
		return snapshot
	}
	return snapshot
}

func validateCertificateSpec(spec CertificateSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" {
		return fmt.Errorf("certificate name and namespace are required")
	}
	if len(certificateDNSNames(spec)) == 0 || strings.TrimSpace(spec.SecretName) == "" {
		return fmt.Errorf("certificate host and secret name are required")
	}
	if strings.TrimSpace(spec.ClusterIssuer) == "" {
		return fmt.Errorf("certificate issuer is required")
	}
	return nil
}

func certificateObject(spec CertificateSpec) *unstructured.Unstructured {
	dnsNames := make([]any, 0, len(certificateDNSNames(spec)))
	for _, name := range certificateDNSNames(spec) {
		dnsNames = append(dnsNames, name)
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Certificate",
		"metadata": map[string]any{
			"name":      spec.Name,
			"namespace": spec.Namespace,
			"labels": map[string]any{
				"app.kubernetes.io/managed-by": "liteyuki-devops",
				"liteyuki.devops/project-id":   spec.ProjectID,
				"liteyuki.devops/route-id":     spec.RouteID,
			},
		},
		"spec": map[string]any{
			"secretName": spec.SecretName,
			"dnsNames":   dnsNames,
			"issuerRef": map[string]any{
				"name": spec.ClusterIssuer,
				"kind": certificateIssuerKind(spec.IssuerKind),
			},
		},
	}}
}

func certificateDNSNames(spec CertificateSpec) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(spec.DNSNames)+1)
	appendName := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		names = append(names, value)
	}
	appendName(spec.Host)
	for _, name := range spec.DNSNames {
		appendName(name)
	}
	return names
}

func certificateIssuerKind(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "Issuer") {
		return "Issuer"
	}
	return "ClusterIssuer"
}
