package kubernetes

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// ValidatePodIsolation 是平台创建的所有 Pod 的硬底线：不得共享宿主机网络、PID 或 IPC 命名空间，
// 不得占用宿主机端口，也不得挂载宿主机路径。hostNetwork Pod 不受 NetworkPolicy 约束，
// 这些能力会绕过项目空间网络隔离或直接触达节点。
func ValidatePodIsolation(spec corev1.PodSpec) error {
	if spec.HostNetwork || spec.HostPID || spec.HostIPC {
		return fmt.Errorf("pods cannot share host network, PID or IPC namespaces")
	}
	for _, containers := range [][]corev1.Container{spec.InitContainers, spec.Containers} {
		for _, container := range containers {
			for _, port := range container.Ports {
				if port.HostPort != 0 || port.HostIP != "" {
					return fmt.Errorf("container %s cannot bind host ports", container.Name)
				}
			}
		}
	}
	for _, volume := range spec.Volumes {
		if volume.HostPath != nil {
			return fmt.Errorf("volume %s cannot mount a host path", volume.Name)
		}
	}
	return nil
}
