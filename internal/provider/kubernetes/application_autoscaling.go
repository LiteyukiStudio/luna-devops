package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) applyApplicationAutoScaling(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string) error {
	if !spec.AutoScalingEnabled {
		existing, err := c.client.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ensureResourceOwnership("HorizontalPodAutoscaler", existing, labels); err != nil {
			return err
		}
		return c.client.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Delete(ctx, spec.Name, metav1.DeleteOptions{})
	}
	behavior, err := applicationAutoScalingBehavior(spec)
	if err != nil {
		return err
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: labels},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       applicationWorkloadType(spec),
				Name:       spec.Name,
			},
			MinReplicas: int32Ptr(autoScalingMinReplicas(spec)),
			MaxReplicas: autoScalingMaxReplicas(spec),
			Metrics:     autoScalingMetrics(spec),
			Behavior:    behavior,
		},
	}
	existing, err := c.client.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Create(ctx, hpa, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("HorizontalPodAutoscaler", existing, labels); err != nil {
		return err
	}
	existing.Labels = labels
	existing.Spec = hpa.Spec
	_, err = c.client.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func validateApplicationAutoScaling(spec ApplicationResourcesSpec) error {
	if !spec.AutoScalingEnabled {
		return nil
	}
	if autoScalingMaxReplicas(spec) < autoScalingMinReplicas(spec) {
		return fmt.Errorf("hpa max replicas must be greater than or equal to min replicas")
	}
	if len(autoScalingMetrics(spec)) == 0 {
		return fmt.Errorf("hpa requires at least one cpu or memory metric")
	}
	return nil
}

func autoScalingMinReplicas(spec ApplicationResourcesSpec) int32 {
	if spec.AutoScalingMinReplicas >= 0 {
		return spec.AutoScalingMinReplicas
	}
	if spec.Replicas > 0 {
		return spec.Replicas
	}
	return 1
}

func autoScalingMaxReplicas(spec ApplicationResourcesSpec) int32 {
	if spec.AutoScalingMaxReplicas > 0 {
		return spec.AutoScalingMaxReplicas
	}
	return maxInt32(autoScalingMinReplicas(spec), 1)
}

func maxInt32(left, right int32) int32 {
	if left > right {
		return left
	}
	return right
}

func autoScalingMetrics(spec ApplicationResourcesSpec) []autoscalingv2.MetricSpec {
	metrics := []autoscalingv2.MetricSpec{}
	if spec.AutoScalingCPUPercent > 0 {
		metrics = append(metrics, autoScalingResourceMetric(corev1.ResourceCPU, spec.AutoScalingCPUPercent))
	}
	if spec.AutoScalingMemoryPercent > 0 {
		metrics = append(metrics, autoScalingResourceMetric(corev1.ResourceMemory, spec.AutoScalingMemoryPercent))
	}
	return metrics
}

func applicationAutoScalingBehavior(spec ApplicationResourcesSpec) (*autoscalingv2.HorizontalPodAutoscalerBehavior, error) {
	raw := strings.TrimSpace(spec.AutoScalingBehavior)
	if raw == "" {
		return nil, nil
	}
	var behavior autoscalingv2.HorizontalPodAutoscalerBehavior
	if err := json.Unmarshal([]byte(raw), &behavior); err != nil {
		return nil, fmt.Errorf("invalid hpa behavior: %w", err)
	}
	return &behavior, nil
}

func autoScalingResourceMetric(name corev1.ResourceName, utilization int32) autoscalingv2.MetricSpec {
	return autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name: name,
			Target: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: int32Ptr(utilization),
			},
		},
	}
}
