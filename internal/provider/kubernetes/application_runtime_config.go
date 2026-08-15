package kubernetes

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) ApplyApplicationRuntimeConfig(ctx context.Context, spec ApplicationResourcesSpec) error {
	if err := validateApplicationResourcesSpec(spec); err != nil {
		return err
	}
	if err := c.ensureApplicationRuntimeConfigOwnership(ctx, spec); err != nil {
		return err
	}
	if err := c.ensureApplicationStorageOwnership(ctx, spec); err != nil {
		return err
	}
	objectLabels := appObjectLabels(spec)
	if err := c.applyApplicationRuntimeConfig(ctx, spec, objectLabels); err != nil {
		return err
	}
	return nil
}

func (c *Client) applyApplicationRuntimeConfig(ctx context.Context, spec ApplicationResourcesSpec, objectLabels map[string]string) error {
	if err := c.applyConfigMap(ctx, spec, objectLabels); err != nil {
		return err
	}
	if err := c.applySecret(ctx, spec, objectLabels); err != nil {
		return err
	}
	if err := c.applyConfigFilesConfigMap(ctx, spec, objectLabels); err != nil {
		return err
	}
	if err := c.applySecretFilesSecret(ctx, spec, objectLabels); err != nil {
		return err
	}
	return nil
}

func (c *Client) applyConfigMap(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string) error {
	item := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: spec.Name + "-config", Namespace: spec.Namespace, Labels: labels}, Data: spec.ConfigData}
	existing, err := c.client.CoreV1().ConfigMaps(spec.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Create(ctx, item, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("ConfigMap", existing, labels); err != nil {
		return err
	}
	existing.Labels = item.Labels
	existing.Data = item.Data
	_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applySecret(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string) error {
	data := make(map[string][]byte, len(spec.SecretData))
	for key, value := range spec.SecretData {
		data[key] = []byte(value)
	}
	item := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: spec.Name + "-secret", Namespace: spec.Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, Data: data}
	existing, err := c.client.CoreV1().Secrets(spec.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().Secrets(spec.Namespace).Create(ctx, item, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("Secret", existing, labels); err != nil {
		return err
	}
	existing.Labels = item.Labels
	existing.Type = item.Type
	existing.Data = item.Data
	_, err = c.client.CoreV1().Secrets(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyConfigFilesConfigMap(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string) error {
	if len(spec.ConfigFiles) == 0 {
		return c.deleteConfigMapIfExists(ctx, spec.Namespace, spec.Name+"-config-files", labels)
	}
	data := make(map[string]string, len(spec.ConfigFiles))
	for _, file := range spec.ConfigFiles {
		data[file.Key] = file.Content
	}
	item := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: spec.Name + "-config-files", Namespace: spec.Namespace, Labels: labels}, Data: data}
	existing, err := c.client.CoreV1().ConfigMaps(spec.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Create(ctx, item, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("ConfigMap", existing, labels); err != nil {
		return err
	}
	existing.Labels = item.Labels
	existing.Data = item.Data
	_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applySecretFilesSecret(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string) error {
	if len(spec.SecretFiles) == 0 {
		return c.deleteSecretIfExists(ctx, spec.Namespace, spec.Name+"-secret-files", labels)
	}
	data := make(map[string][]byte, len(spec.SecretFiles))
	for _, file := range spec.SecretFiles {
		data[file.Key] = []byte(file.Content)
	}
	item := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: spec.Name + "-secret-files", Namespace: spec.Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, Data: data}
	existing, err := c.client.CoreV1().Secrets(spec.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().Secrets(spec.Namespace).Create(ctx, item, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("Secret", existing, labels); err != nil {
		return err
	}
	existing.Labels = item.Labels
	existing.Type = item.Type
	existing.Data = item.Data
	_, err = c.client.CoreV1().Secrets(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) deleteConfigMapIfExists(ctx context.Context, namespace string, name string, labels map[string]string) error {
	item, err := c.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("ConfigMap", item, labels); err != nil {
		return err
	}
	return c.client.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) deleteSecretIfExists(ctx context.Context, namespace string, name string, labels map[string]string) error {
	item, err := c.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("Secret", item, labels); err != nil {
		return err
	}
	return c.client.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}
