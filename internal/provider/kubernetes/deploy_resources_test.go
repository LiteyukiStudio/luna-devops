package kubernetes

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestApplyApplicationResourcesCreatesWorkloadResources(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := ApplicationResourcesSpec{
		Name:                  "api-dev",
		Namespace:             "project-demo",
		ProjectID:             "prj_demo",
		ApplicationID:         "app_api",
		EnvironmentID:         "env_dev",
		DeploymentTargetID:    "dplt_backend",
		ReleaseID:             "rel_1",
		ServiceBindingsDigest: "binding-digest",
		Image:                 "registry.example.com/acme/api:v1",
		Replicas:              2,
		ServicePort:           8080,
		CPURequest:            "100m",
		MemoryRequest:         "128Mi",
		RolloutTimeoutSeconds: 120,
		ConfigData:            map[string]string{"APP_ENV": "dev"},
		SecretData:            map[string]string{"TOKEN": "secret"},
		DataRetentionEnabled:  true,
		DataCapacity:          "2Gi",
		DataMountPath:         "/data",
	}

	if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
		t.Fatalf("ApplyApplicationResources returned error: %v", err)
	}

	deployment, err := client.client.AppsV1().Deployments(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if *deployment.Spec.Replicas != 2 {
		t.Fatalf("replicas = %d", *deployment.Spec.Replicas)
	}
	if deployment.Spec.Template.Spec.Containers[0].Image != spec.Image {
		t.Fatalf("image = %q", deployment.Spec.Template.Spec.Containers[0].Image)
	}
	if deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy != corev1.PullIfNotPresent {
		t.Fatalf("image pull policy = %q", deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	}
	if deployment.Spec.ProgressDeadlineSeconds == nil || *deployment.Spec.ProgressDeadlineSeconds != 120 {
		t.Fatalf("progress deadline = %#v", deployment.Spec.ProgressDeadlineSeconds)
	}
	pvcName := persistentDataPVCName(spec, persistentDataVolumes(spec)[0])
	if len(deployment.Spec.Template.Spec.Volumes) != 1 || deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim == nil || deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != pvcName {
		t.Fatalf("deployment data volume = %#v", deployment.Spec.Template.Spec.Volumes)
	}
	if len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts) != 1 || deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath != "/data" {
		t.Fatalf("deployment data mount = %#v", deployment.Spec.Template.Spec.Containers[0].VolumeMounts)
	}
	assertManagedLabels(t, deployment.Labels, spec.Name, spec.ProjectID, spec.ApplicationID, spec.EnvironmentID, spec.DeploymentTargetID, spec.ReleaseID)
	assertSelectorLabels(t, deployment.Spec.Selector.MatchLabels, spec.Name, spec.DeploymentTargetID)
	assertManagedLabels(t, deployment.Spec.Template.Labels, spec.Name, spec.ProjectID, spec.ApplicationID, spec.EnvironmentID, spec.DeploymentTargetID, spec.ReleaseID)
	if deployment.Spec.Template.Annotations[ReleaseIDLabel] != spec.ReleaseID {
		t.Fatalf("template release annotation = %q", deployment.Spec.Template.Annotations[ReleaseIDLabel])
	}
	if deployment.Spec.Template.Annotations["luna.devops/service-bindings-digest"] != spec.ServiceBindingsDigest {
		t.Fatalf("template service binding annotation = %q", deployment.Spec.Template.Annotations["luna.devops/service-bindings-digest"])
	}

	service, err := client.client.CoreV1().Services(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if service.Spec.Ports[0].Port != 8080 {
		t.Fatalf("service port = %d", service.Spec.Ports[0].Port)
	}
	assertManagedLabels(t, service.Labels, spec.Name, spec.ProjectID, spec.ApplicationID, spec.EnvironmentID, spec.DeploymentTargetID, spec.ReleaseID)

	configMap, err := client.client.CoreV1().ConfigMaps(spec.Namespace).Get(context.Background(), spec.Name+"-config", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if configMap.Data["APP_ENV"] != "dev" {
		t.Fatalf("config data = %#v", configMap.Data)
	}
	assertManagedLabels(t, configMap.Labels, spec.Name, spec.ProjectID, spec.ApplicationID, spec.EnvironmentID, spec.DeploymentTargetID, spec.ReleaseID)

	secret, err := client.client.CoreV1().Secrets(spec.Namespace).Get(context.Background(), spec.Name+"-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if string(secret.Data["TOKEN"]) != "secret" {
		t.Fatalf("secret data = %#v", secret.Data)
	}
	assertManagedLabels(t, secret.Labels, spec.Name, spec.ProjectID, spec.ApplicationID, spec.EnvironmentID, spec.DeploymentTargetID, spec.ReleaseID)

	claim, err := client.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(context.Background(), pvcName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get persistent data claim: %v", err)
	}
	storage := claim.Spec.Resources.Requests[corev1.ResourceStorage]
	if storage.String() != "2Gi" {
		t.Fatalf("data capacity = %s", storage.String())
	}
	assertManagedLabels(t, claim.Labels, spec.Name, spec.ProjectID, spec.ApplicationID, spec.EnvironmentID, spec.DeploymentTargetID, spec.ReleaseID)
}

func TestApplyApplicationResourcesKeepsDeploymentSelectorStableAcrossReleases(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := ApplicationResourcesSpec{
		Name:               "api-backend-dev",
		Namespace:          "project-demo",
		ProjectID:          "prj_demo",
		ApplicationID:      "app_api",
		EnvironmentID:      "env_dev",
		DeploymentTargetID: "dplt_backend",
		ReleaseID:          "rel_1",
		Image:              "registry.example.com/acme/api:v1",
		ServicePort:        8080,
	}
	if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
		t.Fatalf("first apply returned error: %v", err)
	}

	spec.ReleaseID = "rel_2"
	spec.Image = "registry.example.com/acme/api:v1"
	if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
		t.Fatalf("second apply returned error: %v", err)
	}

	deployment, err := client.client.AppsV1().Deployments(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if deployment.Spec.Selector.MatchLabels[ReleaseIDLabel] != "" {
		t.Fatalf("selector should not include release label: %#v", deployment.Spec.Selector.MatchLabels)
	}
	assertSelectorLabels(t, deployment.Spec.Selector.MatchLabels, spec.Name, spec.DeploymentTargetID)
	if deployment.Labels[ReleaseIDLabel] != "rel_2" {
		t.Fatalf("deployment release label = %q", deployment.Labels[ReleaseIDLabel])
	}
	if deployment.Spec.Template.Annotations[ReleaseIDLabel] != "rel_2" {
		t.Fatalf("template release annotation = %q", deployment.Spec.Template.Annotations[ReleaseIDLabel])
	}
	if deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy != corev1.PullIfNotPresent {
		t.Fatalf("image pull policy = %q", deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	}
}

func TestApplyApplicationResourcesRejectsForeignOwnerBeforeMutation(t *testing.T) {
	existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      "api-backend-dev",
		Namespace: "project-demo",
		Labels: map[string]string{
			ManagedByLabel: ManagedByValue, DeploymentTargetIDLabel: "dplt_old",
		},
	}}
	client := NewClientForInterface(fake.NewSimpleClientset(existing))
	spec := ApplicationResourcesSpec{
		Name:               existing.Name,
		Namespace:          existing.Namespace,
		ProjectID:          "prj_demo",
		ApplicationID:      "app_new",
		EnvironmentID:      "env_dev",
		DeploymentTargetID: "dplt_new",
		ReleaseID:          "rel_new",
		Image:              "registry.example.com/acme/api:new",
		ServicePort:        8080,
		ConfigData:         map[string]string{"MUST_NOT_BE_CREATED": "true"},
	}

	err := client.ApplyApplicationResources(context.Background(), spec)
	var conflict *ResourceOwnershipConflictError
	if !errors.As(err, &conflict) || conflict.Kind != "Deployment" {
		t.Fatalf("expected deployment ownership conflict, got %v", err)
	}
	if _, err := client.client.CoreV1().ConfigMaps(spec.Namespace).Get(context.Background(), spec.Name+"-config", metav1.GetOptions{}); err == nil {
		t.Fatal("runtime config must not be created before ownership preflight succeeds")
	}
}

func TestApplyApplicationResourcesRejectsUnmanagedService(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api-dev", Namespace: "project-demo"}}
	client := NewClientForInterface(fake.NewSimpleClientset(service))
	spec := ApplicationResourcesSpec{
		Name:               service.Name,
		Namespace:          service.Namespace,
		ProjectID:          "prj_demo",
		ApplicationID:      "app_api",
		EnvironmentID:      "env_dev",
		DeploymentTargetID: "dplt_backend",
		ReleaseID:          "rel_1",
		Image:              "registry.example.com/acme/api:v1",
		ServicePort:        8080,
	}

	err := client.ApplyApplicationResources(context.Background(), spec)
	var conflict *ResourceOwnershipConflictError
	if !errors.As(err, &conflict) || conflict.Kind != "Service" {
		t.Fatalf("expected service ownership conflict, got %v", err)
	}
}

func TestManagedPVCRejectsDifferentDeploymentTargetLifecycle(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	base := ApplicationResourcesSpec{
		Name:                 "postgres-dev",
		Namespace:            "project-demo",
		ProjectID:            "prj_demo",
		ApplicationID:        "app_postgres",
		DeploymentTargetID:   "dplt_first_lifecycle",
		DataRetentionEnabled: true,
		DataCapacity:         "1Gi",
	}
	if err := client.ApplyPersistentDataVolume(context.Background(), base); err != nil {
		t.Fatalf("apply first lifecycle pvc: %v", err)
	}

	next := base
	next.ApplicationID = "app_postgres_recreated"
	next.DeploymentTargetID = "dplt_second_lifecycle"
	err := client.ApplyPersistentDataVolume(context.Background(), next)
	var conflict *ResourceOwnershipConflictError
	if !errors.As(err, &conflict) || conflict.Kind != "PersistentVolumeClaim" {
		t.Fatalf("expected retained pvc ownership conflict, got %v", err)
	}

	claimName := persistentDataPVCName(base, persistentDataVolumes(base)[0])
	claim, getErr := client.client.CoreV1().PersistentVolumeClaims(base.Namespace).Get(context.Background(), claimName, metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("get retained pvc: %v", getErr)
	}
	if claim.Labels[DeploymentTargetIDLabel] != base.DeploymentTargetID {
		t.Fatalf("retained pvc owner changed: %#v", claim.Labels)
	}
}

func TestApplyApplicationResourcesPreservesExistingDeploymentSelector(t *testing.T) {
	oldSelector := map[string]string{
		ManagedByLabel:     ManagedByValue,
		ApplicationNameKey: "api-backend-dev",
		ProjectIDLabel:     "prj_old",
	}
	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api-backend-dev", Namespace: "project-demo", Labels: map[string]string{
			ManagedByLabel: ManagedByValue, DeploymentTargetIDLabel: "dplt_backend",
		}},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: oldSelector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: oldSelector},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "registry.example.com/acme/api:old"}}},
			},
		},
	}
	client := NewClientForInterface(fake.NewSimpleClientset(existing))
	spec := ApplicationResourcesSpec{
		Name:               "api-backend-dev",
		Namespace:          "project-demo",
		ProjectID:          "prj_new",
		ApplicationID:      "app_api",
		EnvironmentID:      "env_dev",
		DeploymentTargetID: "dplt_backend",
		ReleaseID:          "rel_2",
		BuildRunID:         "bldr_2",
		Image:              "registry.example.com/acme/api:latest",
		ServicePort:        8080,
	}

	if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}

	deployment, err := client.client.AppsV1().Deployments(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if deployment.Spec.Selector.MatchLabels[ProjectIDLabel] != "prj_old" {
		t.Fatalf("selector was changed: %#v", deployment.Spec.Selector.MatchLabels)
	}
	if deployment.Labels[ProjectIDLabel] != "prj_new" {
		t.Fatalf("object project label = %q", deployment.Labels[ProjectIDLabel])
	}
	if deployment.Spec.Template.Labels[ProjectIDLabel] != "prj_old" {
		t.Fatalf("template selector label should be preserved: %#v", deployment.Spec.Template.Labels)
	}
	if deployment.Spec.Template.Labels[ReleaseIDLabel] != "rel_2" {
		t.Fatalf("template release label = %q", deployment.Spec.Template.Labels[ReleaseIDLabel])
	}
	if deployment.Spec.Template.Annotations[BuildRunIDLabel] != "bldr_2" {
		t.Fatalf("template build run annotation = %q", deployment.Spec.Template.Annotations[BuildRunIDLabel])
	}
	service, err := client.client.CoreV1().Services(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if service.Spec.Selector[ProjectIDLabel] != "prj_old" {
		t.Fatalf("service selector should follow deployment selector: %#v", service.Spec.Selector)
	}
}

func TestApplyApplicationResourcesCanForceImagePull(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := ApplicationResourcesSpec{
		Name:               "api-backend-dev",
		Namespace:          "project-demo",
		ProjectID:          "prj_demo",
		ApplicationID:      "app_api",
		EnvironmentID:      "env_dev",
		DeploymentTargetID: "dplt_backend",
		ReleaseID:          "rel_1",
		Image:              "registry.example.com/acme/api:prod",
		ServicePort:        8080,
		ForceImagePull:     true,
	}
	if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}

	deployment, err := client.client.AppsV1().Deployments(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy != corev1.PullAlways {
		t.Fatalf("image pull policy = %q", deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	}
}

func TestApplyApplicationResourcesAppliesAdvancedKubernetesOptions(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := ApplicationResourcesSpec{
		Name:                         "api-advanced",
		Namespace:                    "project-demo",
		ProjectID:                    "prj_demo",
		ApplicationID:                "app_api",
		EnvironmentID:                "env_dev",
		DeploymentTargetID:           "dplt_backend",
		ReleaseID:                    "rel_1",
		Image:                        "registry.example.com/acme/api:prod",
		Replicas:                     2,
		ServicePort:                  8080,
		CPURequest:                   "100m",
		MemoryRequest:                "128Mi",
		CPULimit:                     "500m",
		MemoryLimit:                  "512Mi",
		ImagePullPolicy:              "Never",
		ContainerCommand:             `["/bin/sh","-ec"]`,
		ContainerArgs:                "npm run start",
		Lifecycle:                    `{"preStop":{"exec":{"command":["/bin/sh","-c","sleep 5"]}}}`,
		InitContainers:               `[{"name":"init-permissions","image":"busybox:1.36","command":["sh","-c","echo init"],"env":[{"name":"MODE","value":"init"},{"name":"FROM_SECRET","valueFrom":{"secretKeyRef":{"name":"other","key":"token"}}}],"envFrom":[{"secretRef":{"name":"other"}}],"volumeMounts":[{"name":"data","mountPath":"/data"}]}]`,
		SidecarContainers:            `[{"name":"log-agent","image":"busybox:1.36","args":["sleep","3600"],"ports":[{"containerPort":9000,"hostPort":39000}],"securityContext":{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}}}]`,
		ReadinessProbe:               `{"httpGet":{"path":"/ready","port":8080},"periodSeconds":5}`,
		RunAsUser:                    "1001",
		RunAsGroup:                   "1001",
		FSGroup:                      "1001",
		FSGroupChangePolicy:          "OnRootMismatch",
		ReadOnlyRootFilesystem:       true,
		AllowPrivilegeEscalation:     "false",
		CapabilityDrop:               "ALL",
		NodeSelector:                 "kubernetes.io/os=linux",
		Tolerations:                  `[{"key":"dedicated","operator":"Equal","value":"apps","effect":"NoSchedule"}]`,
		PriorityClassName:            "high-priority",
		ServiceType:                  "NodePort",
		ServicePorts:                 []ApplicationServicePort{{Name: "http", Port: 8080, AppProtocol: "http"}},
		ServiceAnnotations:           "example.com/service=true",
		ServiceExternalTrafficPolicy: "Local",
		ServiceSessionAffinity:       "ClientIP",
		AutoScalingEnabled:           true,
		AutoScalingMinReplicas:       2,
		AutoScalingMaxReplicas:       5,
		AutoScalingCPUPercent:        70,
		DataRetentionEnabled:         true,
		DataCapacity:                 "2Gi",
		DataMountPath:                "/data",
		DataStorageClassName:         "local-path",
		DataAccessMode:               "ReadWriteMany",
		DataVolumeMode:               "Filesystem",
	}
	if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}

	deployment, err := client.client.AppsV1().Deployments(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	podSpec := deployment.Spec.Template.Spec
	container := podSpec.Containers[0]
	if container.ImagePullPolicy != corev1.PullNever {
		t.Fatalf("image pull policy = %q", container.ImagePullPolicy)
	}
	if container.Resources.Limits.Cpu().String() != "500m" || container.Resources.Limits.Memory().String() != "512Mi" {
		t.Fatalf("limits = %#v", container.Resources.Limits)
	}
	if len(container.Command) != 2 || container.Command[0] != "/bin/sh" || len(container.Args) != 1 || container.Args[0] != "npm run start" {
		t.Fatalf("command/args = %#v %#v", container.Command, container.Args)
	}
	if container.ReadinessProbe == nil || container.ReadinessProbe.HTTPGet == nil || container.ReadinessProbe.HTTPGet.Path != "/ready" {
		t.Fatalf("readiness probe = %#v", container.ReadinessProbe)
	}
	if container.Lifecycle == nil || container.Lifecycle.PreStop == nil {
		t.Fatalf("lifecycle = %#v", container.Lifecycle)
	}
	if len(podSpec.InitContainers) != 1 || podSpec.InitContainers[0].Name != "init-permissions" || len(podSpec.InitContainers[0].VolumeMounts) != 1 {
		t.Fatalf("init containers = %#v", podSpec.InitContainers)
	}
	if len(podSpec.InitContainers[0].Env) != 1 || podSpec.InitContainers[0].Env[0].Name != "MODE" || len(podSpec.InitContainers[0].EnvFrom) != 2 {
		t.Fatalf("init container env = %#v envFrom = %#v", podSpec.InitContainers[0].Env, podSpec.InitContainers[0].EnvFrom)
	}
	if len(podSpec.Containers) != 2 || podSpec.Containers[1].Name != "log-agent" {
		t.Fatalf("containers = %#v", podSpec.Containers)
	}
	if len(podSpec.Containers[1].Ports) != 1 || podSpec.Containers[1].Ports[0].HostPort != 0 {
		t.Fatalf("sidecar ports = %#v", podSpec.Containers[1].Ports)
	}
	if podSpec.Containers[1].SecurityContext == nil || podSpec.Containers[1].SecurityContext.AllowPrivilegeEscalation == nil || *podSpec.Containers[1].SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("sidecar security context = %#v", podSpec.Containers[1].SecurityContext)
	}
	if podSpec.SecurityContext == nil || podSpec.SecurityContext.RunAsUser == nil || *podSpec.SecurityContext.RunAsUser != 1001 {
		t.Fatalf("pod security context = %#v", podSpec.SecurityContext)
	}
	if podSpec.SecurityContext.FSGroupChangePolicy == nil || *podSpec.SecurityContext.FSGroupChangePolicy != corev1.FSGroupChangeOnRootMismatch {
		t.Fatalf("fsGroup change policy = %#v", podSpec.SecurityContext.FSGroupChangePolicy)
	}
	if container.SecurityContext == nil || container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem {
		t.Fatalf("container security context = %#v", container.SecurityContext)
	}
	if container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("allow privilege escalation = %#v", container.SecurityContext.AllowPrivilegeEscalation)
	}
	if len(container.SecurityContext.Capabilities.Drop) != 1 || container.SecurityContext.Capabilities.Drop[0] != "ALL" {
		t.Fatalf("capability drop = %#v", container.SecurityContext.Capabilities)
	}
	if podSpec.NodeSelector["kubernetes.io/os"] != "linux" || len(podSpec.Tolerations) != 1 || podSpec.PriorityClassName != "high-priority" {
		t.Fatalf("scheduling = %#v %#v %q", podSpec.NodeSelector, podSpec.Tolerations, podSpec.PriorityClassName)
	}

	service, err := client.client.CoreV1().Services(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if service.Spec.Type != corev1.ServiceTypeNodePort || service.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal || service.Spec.SessionAffinity != corev1.ServiceAffinityClientIP {
		t.Fatalf("service spec = %#v", service.Spec)
	}
	if service.Annotations["example.com/service"] != "true" {
		t.Fatalf("service annotations = %#v", service.Annotations)
	}
	if service.Spec.Ports[0].AppProtocol == nil || *service.Spec.Ports[0].AppProtocol != "http" {
		t.Fatalf("app protocol = %#v", service.Spec.Ports[0].AppProtocol)
	}

	hpa, err := client.client.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get hpa: %v", err)
	}
	if hpa.Spec.MaxReplicas != 5 || hpa.Spec.MinReplicas == nil || *hpa.Spec.MinReplicas != 2 || len(hpa.Spec.Metrics) != 1 {
		t.Fatalf("hpa spec = %#v", hpa.Spec)
	}

	claim, err := client.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(context.Background(), persistentDataPVCName(spec, persistentDataVolumes(spec)[0]), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pvc: %v", err)
	}
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName != "local-path" {
		t.Fatalf("storage class = %#v", claim.Spec.StorageClassName)
	}
	if len(claim.Spec.AccessModes) != 1 || claim.Spec.AccessModes[0] != corev1.ReadWriteMany {
		t.Fatalf("access modes = %#v", claim.Spec.AccessModes)
	}
	if claim.Spec.VolumeMode == nil || *claim.Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		t.Fatalf("volume mode = %#v", claim.Spec.VolumeMode)
	}
}

func TestApplyApplicationResourcesSupportsStatefulSetAndHPABehavior(t *testing.T) {
	labels := baseManagedLabels("api-stateful")
	labels[DeploymentTargetIDLabel] = "dplt_backend"
	client := NewClientForInterface(fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api-stateful", Namespace: "project-demo", Labels: labels},
	}))
	spec := ApplicationResourcesSpec{
		Name:                   "api-stateful",
		Namespace:              "project-demo",
		ProjectID:              "prj_demo",
		ApplicationID:          "app_api",
		EnvironmentID:          "env_dev",
		DeploymentTargetID:     "dplt_backend",
		ReleaseID:              "rel_1",
		Image:                  "registry.example.com/acme/api:prod",
		WorkloadType:           "StatefulSet",
		Replicas:               3,
		ServicePort:            8080,
		CPURequest:             "100m",
		MemoryRequest:          "128Mi",
		AutoScalingEnabled:     true,
		AutoScalingMinReplicas: 2,
		AutoScalingMaxReplicas: 6,
		AutoScalingCPUPercent:  75,
		AutoScalingBehavior:    `{"scaleDown":{"stabilizationWindowSeconds":300}}`,
		DataRetentionEnabled:   true,
		DataCapacity:           "2Gi",
		DataMountPath:          "/data",
		DataStorageClassName:   "local-path",
	}
	if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}
	if _, err := client.client.AppsV1().Deployments(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{}); err == nil {
		t.Fatal("expected stale deployment to be deleted")
	}
	statefulSet, err := client.client.AppsV1().StatefulSets(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if statefulSet.Spec.ServiceName != spec.Name || statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas != 3 {
		t.Fatalf("statefulset spec = %#v", statefulSet.Spec)
	}
	hpa, err := client.client.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get hpa: %v", err)
	}
	if hpa.Spec.ScaleTargetRef.Kind != "StatefulSet" || hpa.Spec.Behavior == nil || hpa.Spec.Behavior.ScaleDown == nil || hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds == nil {
		t.Fatalf("hpa spec = %#v", hpa.Spec)
	}
}

func TestApplyApplicationResourcesSupportsExistingClaimAndEmptyDirDataVolumes(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := ApplicationResourcesSpec{
		Name:                 "api-data-sources",
		Namespace:            "project-demo",
		ProjectID:            "prj_demo",
		ApplicationID:        "app_api",
		EnvironmentID:        "env_dev",
		DeploymentTargetID:   "dplt_backend",
		ReleaseID:            "rel_1",
		Image:                "registry.example.com/acme/api:prod",
		ServicePort:          8080,
		DataRetentionEnabled: true,
		DataVolumes: []ApplicationDataVolume{
			{Name: "shared", MountPath: "/shared", SourceType: "existingClaim", ExistingClaimName: "shared-pvc"},
			{Name: "cache", MountPath: "/cache", SourceType: "emptyDir", EmptyDirMedium: "Memory", EmptyDirSizeLimit: "512Mi"},
		},
	}
	if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}
	deployment, err := client.client.AppsV1().Deployments(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	volumes := deployment.Spec.Template.Spec.Volumes
	if len(volumes) != 2 {
		t.Fatalf("volumes = %#v", volumes)
	}
	if volumes[0].PersistentVolumeClaim == nil || volumes[0].PersistentVolumeClaim.ClaimName != "shared-pvc" {
		t.Fatalf("existing claim volume = %#v", volumes[0])
	}
	if volumes[1].EmptyDir == nil || volumes[1].EmptyDir.Medium != corev1.StorageMediumMemory {
		t.Fatalf("empty dir volume = %#v", volumes[1])
	}
	claims, err := client.client.CoreV1().PersistentVolumeClaims(spec.Namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list pvc: %v", err)
	}
	if len(claims.Items) != 0 {
		t.Fatalf("unexpected managed pvcs = %#v", claims.Items)
	}
}

func TestApplyApplicationResourcesReclaimsRetainedClaim(t *testing.T) {
	const retainedVolumeID = "rvol_demo"
	existing := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "retained-data",
			Namespace: "project-demo",
			Labels: map[string]string{
				ManagedByLabel:        ManagedByValue,
				RetainedVolumeIDLabel: retainedVolumeID,
			},
		},
	}
	client := NewClientForInterface(fake.NewSimpleClientset(existing))
	spec := ApplicationResourcesSpec{
		Name: "api-reclaim", Namespace: "project-demo", ProjectID: "prj_demo",
		ApplicationID: "app_api", EnvironmentID: "env_dev", DeploymentTargetID: "dplt_backend",
		ReleaseID: "rel_1", Image: "registry.example.com/acme/api:prod", ServicePort: 8080,
		DataRetentionEnabled: true,
		DataVolumes: []ApplicationDataVolume{{
			Name: "data", MountPath: "/data", SourceType: "retainedClaim",
			ExistingClaimName: "retained-data", RetainedVolumeID: retainedVolumeID,
		}},
	}
	if err := client.ApplyApplicationResources(context.Background(), spec); err != nil {
		t.Fatalf("apply retained claim: %v", err)
	}
	claim, err := client.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(context.Background(), "retained-data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get reclaimed claim: %v", err)
	}
	if claim.Labels[ApplicationIDLabel] != spec.ApplicationID || claim.Labels[DeploymentTargetIDLabel] != spec.DeploymentTargetID {
		t.Fatalf("reclaimed labels = %#v", claim.Labels)
	}
	if _, ok := claim.Labels[RetainedVolumeIDLabel]; ok {
		t.Fatalf("retained ownership label was not removed: %#v", claim.Labels)
	}
	deployment, err := client.client.AppsV1().Deployments(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "retained-data" {
		t.Fatalf("deployment did not mount retained claim: %#v", deployment.Spec.Template.Spec.Volumes)
	}
}

func TestApplyApplicationResourcesRejectsRiskyAuxContainerSecurityContext(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := ApplicationResourcesSpec{
		Name:               "api-risky-sidecar",
		Namespace:          "project-demo",
		Image:              "registry.example.com/acme/api:prod",
		ServicePort:        8080,
		SidecarContainers:  `[{"name":"debug","image":"busybox:1.36","securityContext":{"capabilities":{"add":["NET_ADMIN"]}}}]`,
		DeploymentTargetID: "dplt_backend",
	}
	if err := client.ApplyApplicationResources(context.Background(), spec); err == nil {
		t.Fatal("expected risky sidecar security context to be rejected")
	}
}

func TestSetHookJobTTL(t *testing.T) {
	namespace := "project-demo"
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "hook-demo", Namespace: namespace}}
	client := NewClientForInterface(fake.NewSimpleClientset(job))

	if err := client.setHookJobTTL(context.Background(), namespace, job.Name, hookJobSuccessTTLSeconds); err != nil {
		t.Fatalf("setHookJobTTL returned error: %v", err)
	}

	stored, err := client.client.BatchV1().Jobs(namespace).Get(context.Background(), job.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get hook job: %v", err)
	}
	if stored.Spec.TTLSecondsAfterFinished == nil || *stored.Spec.TTLSecondsAfterFinished != hookJobSuccessTTLSeconds {
		t.Fatalf("ttlSecondsAfterFinished = %#v", stored.Spec.TTLSecondsAfterFinished)
	}
}

func TestAttachHookScriptOwner(t *testing.T) {
	namespace := "project-demo"
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hook-demo",
			Namespace: namespace,
			UID:       types.UID("job-uid"),
		},
	}
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "hook-demo-script", Namespace: namespace}}
	client := NewClientForInterface(fake.NewSimpleClientset(job, configMap))

	if err := client.attachHookScriptOwner(context.Background(), namespace, configMap.Name, job); err != nil {
		t.Fatalf("attachHookScriptOwner returned error: %v", err)
	}

	stored, err := client.client.CoreV1().ConfigMaps(namespace).Get(context.Background(), configMap.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get hook script configmap: %v", err)
	}
	if len(stored.OwnerReferences) != 1 {
		t.Fatalf("ownerReferences = %#v", stored.OwnerReferences)
	}
	owner := stored.OwnerReferences[0]
	if owner.Kind != "Job" || owner.Name != job.Name || owner.UID != job.UID {
		t.Fatalf("ownerReference = %#v", owner)
	}
	if owner.Controller == nil || !*owner.Controller {
		t.Fatalf("owner controller flag = %#v", owner.Controller)
	}
}

func assertManagedLabels(t *testing.T, labels map[string]string, name string, projectID string, applicationID string, environmentID string, deploymentTargetID string, releaseID string) {
	t.Helper()
	expected := map[string]string{
		ManagedByLabel:          ManagedByValue,
		ApplicationNameKey:      name,
		ProjectIDLabel:          projectID,
		ApplicationIDLabel:      applicationID,
		EnvironmentIDLabel:      environmentID,
		DeploymentTargetIDLabel: deploymentTargetID,
		ReleaseIDLabel:          releaseID,
	}
	for key, value := range expected {
		if labels[key] != value {
			t.Fatalf("label %s = %q, want %q in %#v", key, labels[key], value, labels)
		}
	}
}

func assertSelectorLabels(t *testing.T, labels map[string]string, name string, deploymentTargetID string) {
	t.Helper()
	expected := map[string]string{
		ManagedByLabel:          ManagedByValue,
		ApplicationNameKey:      name,
		DeploymentTargetIDLabel: deploymentTargetID,
	}
	for key, value := range expected {
		if labels[key] != value {
			t.Fatalf("selector label %s = %q, want %q in %#v", key, labels[key], value, labels)
		}
	}
	if labels[ReleaseIDLabel] != "" {
		t.Fatalf("selector labels must not include release id: %#v", labels)
	}
	for _, key := range []string{ProjectIDLabel, ApplicationIDLabel, EnvironmentIDLabel} {
		if labels[key] != "" {
			t.Fatalf("selector labels must not include ownership label %s: %#v", key, labels)
		}
	}
}
