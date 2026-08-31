package kubernetes

import (
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListKubectlRuntimeWorkloadsUsesTopLevelOwnersWithoutDoubleCounting(t *testing.T) {
	replicas := int32(2)
	labels := kubectlRuntimeTestLabels("prj_demo", "app_demo")
	controller := true
	client := NewClientForInterface(fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "project-demo", UID: "uid-deployment", Labels: labels},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas, Template: kubectlRuntimeTestTemplate()},
			Status:     appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "web-rs", Namespace: "project-demo", UID: "uid-deployment-rs", Labels: labels, OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", UID: "uid-deployment", Controller: &controller}}},
			Spec:       appsv1.ReplicaSetSpec{Replicas: &replicas, Template: kubectlRuntimeTestTemplate()},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "project-demo", UID: "uid-web-pod", Labels: labels, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", UID: "uid-deployment-rs", Controller: &controller}}},
			Spec:       kubectlRuntimeTestTemplate().Spec,
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "project-demo", UID: "uid-statefulset", Labels: labels},
			Spec:       appsv1.StatefulSetSpec{Replicas: &replicas, Template: kubectlRuntimeTestTemplate()},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "db-0", Namespace: "project-demo", UID: "uid-db-pod", Labels: labels, OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", UID: "uid-statefulset", Controller: &controller}}},
			Spec:       kubectlRuntimeTestTemplate().Spec,
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "standalone-rs", Namespace: "project-demo", UID: "uid-standalone-rs", Labels: labels},
			Spec:       appsv1.ReplicaSetSpec{Replicas: &replicas, Template: kubectlRuntimeTestTemplate()},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "standalone-rs-1", Namespace: "project-demo", UID: "uid-rs-pod", Labels: labels, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", UID: "uid-standalone-rs", Controller: &controller}}},
			Spec:       kubectlRuntimeTestTemplate().Spec,
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "manual-job", Namespace: "project-demo", UID: "uid-job", Labels: labels},
			Spec:       batchv1.JobSpec{Template: kubectlRuntimeTestTemplate()},
			Status:     batchv1.JobStatus{Active: 1},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "manual-job-1", Namespace: "project-demo", UID: "uid-job-pod", Labels: labels, OwnerReferences: []metav1.OwnerReference{{Kind: "Job", UID: "uid-job", Controller: &controller}}},
			Spec:       kubectlRuntimeTestTemplate().Spec,
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: "scheduled", Namespace: "project-demo", UID: "uid-cronjob", Labels: labels},
			Spec:       batchv1.CronJobSpec{JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{Template: kubectlRuntimeTestTemplate()}}},
			Status:     batchv1.CronJobStatus{Active: []corev1.ObjectReference{{UID: "uid-cronjob-run"}}},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "scheduled-1", Namespace: "project-demo", UID: "uid-cronjob-run", Labels: labels, OwnerReferences: []metav1.OwnerReference{{Kind: "CronJob", UID: "uid-cronjob", Controller: &controller}}},
			Spec:       batchv1.JobSpec{Template: kubectlRuntimeTestTemplate()},
			Status:     batchv1.JobStatus{Active: 1},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "scheduled-1-pod", Namespace: "project-demo", UID: "uid-cron-pod", Labels: labels, OwnerReferences: []metav1.OwnerReference{{Kind: "Job", UID: "uid-cronjob-run", Controller: &controller}}},
			Spec:       kubectlRuntimeTestTemplate().Spec,
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "project-demo", UID: "uid-standalone-pod", Labels: labels},
			Spec:       kubectlRuntimeTestTemplate().Spec,
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "platform-app", Namespace: "project-demo", UID: "uid-platform", Labels: map[string]string{
			ManagedByLabel: ManagedByValue, KubectlGatewayManagementSourceLabel: PlatformManagementSourceValue, ProjectIDLabel: "prj_demo", ApplicationIDLabel: "app_demo",
		}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "project-demo", UID: "uid-foreign", Labels: kubectlRuntimeTestLabels("prj_foreign", "app_demo")}},
	))

	items, err := client.ListKubectlRuntimeWorkloads(t.Context(), KubectlRuntimeObservationOptions{
		RuntimeClusterID: "rcl_demo",
		ProjectID:        "prj_demo",
		Namespace:        "project-demo",
	})
	if err != nil {
		t.Fatalf("ListKubectlRuntimeWorkloads() error = %v", err)
	}
	if len(items) != 6 {
		t.Fatalf("workloads = %#v", items)
	}
	byKind := make(map[string]KubectlRuntimeWorkload, len(items))
	for _, item := range items {
		byKind[item.Kind] = item
	}
	for _, kind := range []string{"Deployment", "StatefulSet", "ReplicaSet", "Job", "CronJob", "Pod"} {
		if _, exists := byKind[kind]; !exists {
			t.Fatalf("missing %s in %#v", kind, items)
		}
	}
	deployment := byKind["Deployment"]
	if deployment.DesiredReplicas != 2 || !slices.Equal(deployment.PodNames, []string{"web-1"}) {
		t.Fatalf("deployment = %#v", deployment)
	}
	cronJob := byKind["CronJob"]
	if !slices.Equal(cronJob.PodNames, []string{"scheduled-1-pod"}) || cronJob.DesiredReplicas != 1 {
		t.Fatalf("cronjob = %#v", cronJob)
	}
	if options := deployment.RuntimeMetricsOptions(); options.DeploymentTargetID != "kubectl:rcl_demo:prj_demo:app_demo:uid-deployment" || !slices.Equal(options.ExactPodNames, []string{"web-1"}) {
		t.Fatalf("metrics options = %#v", options)
	}
	if deployment.EffectiveCPURequest != "250m" || deployment.EffectiveMemoryRequest != "536870912" {
		t.Fatalf("deployment requests = %#v", deployment)
	}
}

func TestListKubectlRuntimeWorkloadsRequiresOwnershipBoundary(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	if _, err := client.ListKubectlRuntimeWorkloads(t.Context(), KubectlRuntimeObservationOptions{RuntimeClusterID: "rcl_demo", Namespace: "project-demo"}); err == nil {
		t.Fatal("expected missing project ownership to fail")
	}
}

func kubectlRuntimeTestLabels(projectID, applicationID string) map[string]string {
	return map[string]string{
		ManagedByLabel:                      ManagedByValue,
		KubectlGatewayManagementSourceLabel: KubectlGatewayManagementSourceValue,
		ProjectIDLabel:                      projectID,
		ApplicationIDLabel:                  applicationID,
	}
}

func kubectlRuntimeTestTemplate() corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "app",
		Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("512Mi"),
		}},
	}}}}
}
