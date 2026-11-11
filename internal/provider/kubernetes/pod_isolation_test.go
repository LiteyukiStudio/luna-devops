package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestValidatePodIsolationRejectsHostAccess(t *testing.T) {
	cases := map[string]corev1.PodSpec{
		"hostNetwork": {HostNetwork: true},
		"hostPID":     {HostPID: true},
		"hostIPC":     {HostIPC: true},
		"hostPort":    {Containers: []corev1.Container{{Name: "app", Ports: []corev1.ContainerPort{{ContainerPort: 80, HostPort: 80}}}}},
		"hostIP":      {InitContainers: []corev1.Container{{Name: "init", Ports: []corev1.ContainerPort{{ContainerPort: 80, HostIP: "0.0.0.0"}}}}},
		"hostPath":    {Volumes: []corev1.Volume{{Name: "root", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}}}},
	}
	for name, spec := range cases {
		if err := ValidatePodIsolation(spec); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

// 平台生成的每一类 Pod 都必须满足隔离底线，否则会绕过项目空间 NetworkPolicy。
func TestGeneratedPodsStayIsolated(t *testing.T) {
	spec := ApplicationResourcesSpec{
		Name: "api", Namespace: "project-demo", ProjectID: "prj_demo", ApplicationID: "app_api", DeploymentTargetID: "dplt_api",
		Image: "registry.example.com/acme/api:prod", ServicePort: 8080,
		InitContainers:    `[{"name":"migrate","image":"busybox","ports":[{"containerPort":9000}]}]`,
		SidecarContainers: `[{"name":"proxy","image":"envoy","ports":[{"containerPort":9901}]}]`,
		DataVolumes: []ApplicationDataVolume{
			{Name: "cache", SourceType: "emptyDir", MountPath: "/cache"},
			{Name: "data", SourceType: "projectVolume", ProjectVolumeID: "pvol_data", ClaimName: "data", MountPath: "/data"},
		},
	}
	template := applicationPodTemplate(spec, map[string]string{}, map[string]string{})
	if len(template.Spec.InitContainers) == 0 || len(template.Spec.Containers) < 2 {
		t.Fatalf("fixture must exercise auxiliary containers: %#v", template.Spec)
	}
	pods := map[string]corev1.PodSpec{
		"application":    template.Spec,
		"volumeTransfer": volumeTransferPod(directTransferSpec("export"), "transfer", map[string]string{}).Spec,
		"probe":          networkPolicyProbePod("probe-ns", "server", "busybox", map[string]string{}, []string{"sh"}).Spec,
	}
	for name, pod := range pods {
		if err := ValidatePodIsolation(pod); err != nil {
			t.Errorf("%s pod: %v", name, err)
		}
	}
	if _, err := applicationAuxContainers(`[{"name":"x","image":"busybox","ports":[{"containerPort":80,"hostPort":80}]}]`, "sidecar", spec, map[string]bool{}); err == nil {
		t.Fatal("auxiliary containers must not request host ports")
	}
}
