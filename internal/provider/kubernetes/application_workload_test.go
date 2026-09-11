package kubernetes

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestApplyApplicationResourcesPreservesZeroReplicas(t *testing.T) {
	tests := []struct {
		name         string
		workloadType string
	}{
		{name: "Deployment", workloadType: "Deployment"},
		{name: "StatefulSet", workloadType: "StatefulSet"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewClientForInterface(fake.NewSimpleClientset())
			spec := applicationWorkloadTestSpec(test.workloadType)

			if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
				t.Fatalf("ApplyApplicationResources returned error: %v", err)
			}

			switch applicationWorkloadType(spec) {
			case "StatefulSet":
				statefulSet, err := client.client.AppsV1().StatefulSets(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("get StatefulSet: %v", err)
				}
				if statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas != 0 {
					t.Fatalf("StatefulSet replicas = %#v, want 0", statefulSet.Spec.Replicas)
				}
			default:
				deployment, err := client.client.AppsV1().Deployments(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("get Deployment: %v", err)
				}
				if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 0 {
					t.Fatalf("Deployment replicas = %#v, want 0", deployment.Spec.Replicas)
				}
			}
		})
	}
}

func TestValidateApplicationResourcesSpecRejectsNegativeReplicas(t *testing.T) {
	spec := applicationWorkloadTestSpec("Deployment")
	spec.Replicas = -1

	if err := validateApplicationResourcesSpec(spec); err == nil {
		t.Fatal("expected negative replicas to be rejected")
	}
}

func TestScaleApplicationWorkloadUsesSelectedScaleSubresource(t *testing.T) {
	tests := []struct {
		name         string
		workloadType string
		resource     string
		workload     func(ApplicationResourcesSpec) runtime.Object
	}{
		{
			name:         "Deployment",
			workloadType: "Deployment",
			resource:     "deployments",
			workload: func(spec ApplicationResourcesSpec) runtime.Object {
				return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Name: spec.Name, Namespace: spec.Namespace, ResourceVersion: "17", Labels: appObjectLabels(spec),
				}}
			},
		},
		{
			name:         "StatefulSet",
			workloadType: "StatefulSet",
			resource:     "statefulsets",
			workload: func(spec ApplicationResourcesSpec) runtime.Object {
				return &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
					Name: spec.Name, Namespace: spec.Namespace, ResourceVersion: "23", Labels: appObjectLabels(spec),
				}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := applicationWorkloadTestSpec(test.workloadType)
			clientset := fake.NewSimpleClientset(test.workload(spec))
			scaledReplicas := make([]int32, 0, 2)
			clientset.PrependReactor("update", test.resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
				if action.GetSubresource() != "scale" {
					return false, nil, nil
				}
				update, ok := action.(clienttesting.UpdateAction)
				if !ok {
					t.Fatalf("scale action type = %T", action)
				}
				scale, ok := update.GetObject().(*autoscalingv1.Scale)
				if !ok {
					t.Fatalf("scale object type = %T", update.GetObject())
				}
				if scale.Name != spec.Name || scale.Namespace != spec.Namespace {
					t.Fatalf("scale object metadata = %s/%s", scale.Namespace, scale.Name)
				}
				scaledReplicas = append(scaledReplicas, scale.Spec.Replicas)
				return true, scale.DeepCopy(), nil
			})
			client := NewClientForInterface(clientset)

			if err := client.ScaleApplicationWorkload(context.Background(), spec, 0); err != nil {
				t.Fatalf("scale to zero returned error: %v", err)
			}
			if err := client.ScaleApplicationWorkload(context.Background(), spec, 3); err != nil {
				t.Fatalf("restore replicas returned error: %v", err)
			}
			if want := []int32{0, 3}; !reflect.DeepEqual(scaledReplicas, want) {
				t.Fatalf("scaled replicas = %v, want %v", scaledReplicas, want)
			}
		})
	}
}

func TestScaleApplicationWorkloadRejectsForeignOwnership(t *testing.T) {
	spec := applicationWorkloadTestSpec("Deployment")
	foreign := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: spec.Name, Namespace: spec.Namespace,
		Labels: map[string]string{ManagedByLabel: ManagedByValue, DeploymentTargetIDLabel: "dplt_foreign"},
	}}
	clientset := fake.NewSimpleClientset(foreign)
	client := NewClientForInterface(clientset)

	err := client.ScaleApplicationWorkload(context.Background(), spec, 0)
	var conflict *ResourceOwnershipConflictError
	if !errors.As(err, &conflict) || conflict.Kind != "Deployment" {
		t.Fatalf("expected Deployment ownership conflict, got %v", err)
	}
	for _, action := range clientset.Actions() {
		if action.GetSubresource() == "scale" {
			t.Fatalf("foreign workload must not be scaled: %#v", action)
		}
	}
}

func TestScaleApplicationWorkloadRejectsNegativeReplicasBeforeKubernetesCall(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientForInterface(clientset)

	if err := client.ScaleApplicationWorkload(context.Background(), applicationWorkloadTestSpec("Deployment"), -1); err == nil {
		t.Fatal("expected negative replicas to be rejected")
	}
	if len(clientset.Actions()) != 0 {
		t.Fatalf("negative replicas must not call Kubernetes: %#v", clientset.Actions())
	}
}

func applicationWorkloadTestSpec(workloadType string) ApplicationResourcesSpec {
	return ApplicationResourcesSpec{
		Name:               "api-dev",
		Namespace:          "project-demo",
		WorkloadType:       workloadType,
		ProjectID:          "prj_demo",
		ApplicationID:      "app_api",
		DeploymentTargetID: "dplt_backend",
		ReleaseID:          "rel_1",
		Image:              "registry.example.com/acme/api:v1",
		Replicas:           0,
		ServicePort:        8080,
	}
}
