package kubernetes

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ResourceOwnershipConflictCode = "kubernetes.resource_ownership_conflict"

var immutableOwnershipLabels = []string{
	DeploymentTargetIDLabel,
	GatewayRouteIDLabel,
	ProjectIDLabel,
	RuntimeClusterIDLabel,
	SystemComponentLabel,
}

// ResourceOwnershipConflictError prevents a newly-created platform resource
// from adopting a same-name Kubernetes object left by another lifecycle.
type ResourceOwnershipConflictError struct {
	Kind      string
	Namespace string
	Name      string
	OwnerKey  string
	Expected  string
	Actual    string
}

type ownershipCheck struct {
	kind   string
	object metav1.Object
	err    error
}

func (e *ResourceOwnershipConflictError) Error() string {
	resource := strings.Trim(strings.TrimSpace(e.Namespace)+"/"+strings.TrimSpace(e.Name), "/")
	return fmt.Sprintf("%s: %s %s ownership label %s is %q, expected %q", ResourceOwnershipConflictCode, e.Kind, resource, e.OwnerKey, e.Actual, e.Expected)
}

func ensureResourceOwnership(kind string, object metav1.Object, desiredLabels map[string]string) error {
	ownerKey, expected := immutableOwnershipIdentity(desiredLabels)
	if ownerKey == "" || expected == "" {
		return &ResourceOwnershipConflictError{
			Kind: kind, Namespace: object.GetNamespace(), Name: object.GetName(),
			OwnerKey: "immutable-owner", Expected: "configured", Actual: "missing",
		}
	}
	labels := object.GetLabels()
	annotations := object.GetAnnotations()
	actual := strings.TrimSpace(labels[ownerKey])
	if actual == "" {
		actual = strings.TrimSpace(annotations[ownerKey])
	}
	if labels[ManagedByLabel] != ManagedByValue || actual != expected {
		return &ResourceOwnershipConflictError{
			Kind: kind, Namespace: object.GetNamespace(), Name: object.GetName(),
			OwnerKey: ownerKey, Expected: expected, Actual: actual,
		}
	}
	return nil
}

func immutableOwnershipIdentity(labels map[string]string) (string, string) {
	for _, key := range immutableOwnershipLabels {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return key, value
		}
	}
	return "", ""
}

func (c *Client) ensureApplicationRuntimeConfigOwnership(ctx context.Context, spec ApplicationResourcesSpec) error {
	labels := appObjectLabels(spec)
	checks := []ownershipCheck{
		configMapOwnershipCheck(c, ctx, spec.Namespace, spec.Name+"-config"),
		secretOwnershipCheck(c, ctx, spec.Namespace, spec.Name+"-secret"),
		configMapOwnershipCheck(c, ctx, spec.Namespace, spec.Name+"-config-files"),
		secretOwnershipCheck(c, ctx, spec.Namespace, spec.Name+"-secret-files"),
	}
	for _, check := range checks {
		if err := checkExistingOwnership(check.kind, check.object, check.err, labels); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) ensureApplicationWorkloadOwnership(ctx context.Context, spec ApplicationResourcesSpec) error {
	labels := appObjectLabels(spec)
	deployment, deploymentErr := c.client.AppsV1().Deployments(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	statefulSet, statefulSetErr := c.client.AppsV1().StatefulSets(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	service, serviceErr := c.client.CoreV1().Services(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	hpa, hpaErr := c.client.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	checks := []ownershipCheck{
		{"Deployment", deployment, deploymentErr},
		{"StatefulSet", statefulSet, statefulSetErr},
		{"Service", service, serviceErr},
		{"HorizontalPodAutoscaler", hpa, hpaErr},
	}
	for _, check := range checks {
		if err := checkExistingOwnership(check.kind, check.object, check.err, labels); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) ensureApplicationStorageOwnership(ctx context.Context, spec ApplicationResourcesSpec) error {
	labels := appObjectLabels(spec)
	for _, volume := range persistentDataVolumes(spec) {
		if !dataVolumeNeedsPVC(volume) {
			continue
		}
		claim, err := c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(ctx, persistentDataPVCName(spec, volume), metav1.GetOptions{})
		if err := checkExistingOwnership("PersistentVolumeClaim", claim, err, labels); err != nil {
			return err
		}
	}
	return nil
}

func configMapOwnershipCheck(c *Client, ctx context.Context, namespace string, name string) ownershipCheck {
	item, err := c.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	return ownershipCheck{"ConfigMap", item, err}
}

func secretOwnershipCheck(c *Client, ctx context.Context, namespace string, name string) ownershipCheck {
	item, err := c.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	return ownershipCheck{"Secret", item, err}
}

func checkExistingOwnership(kind string, object metav1.Object, err error, desiredLabels map[string]string) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return ensureResourceOwnership(kind, object, desiredLabels)
}
