package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	resourcename "github.com/LiteyukiStudio/devops/internal/resourcename"
)

type resolvedServiceBindingConfig struct {
	Values       map[string]string
	SecretValues map[string]string
	Digest       string
	Count        int
}

type serviceBindingDigestEntry struct {
	ID     string            `json:"id"`
	Values map[string]string `json:"values"`
}

func (r *Runner) resolveServiceBindingConfig(project model.Project, source model.DeploymentTarget) (resolvedServiceBindingConfig, error) {
	result := resolvedServiceBindingConfig{Values: map[string]string{}}
	var bindings []model.ServiceBinding
	if err := r.db.
		Where("project_id = ? and source_deployment_target_id = ? and enabled = ?", project.ID, source.ID, true).
		Order("id asc").
		Find(&bindings).Error; err != nil {
		return result, err
	}
	if len(bindings) == 0 {
		return result, nil
	}

	targetIDs := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		targetIDs = append(targetIDs, binding.TargetDeploymentTargetID)
	}
	var targets []model.DeploymentTarget
	if err := r.db.Where("project_id = ? and id in ?", project.ID, targetIDs).Find(&targets).Error; err != nil {
		return result, err
	}
	targetsByID := make(map[string]model.DeploymentTarget, len(targets))
	for _, target := range targets {
		targetsByID[target.ID] = target
	}

	digestEntries := make([]serviceBindingDigestEntry, 0, len(bindings))
	for _, binding := range bindings {
		target, ok := targetsByID[binding.TargetDeploymentTargetID]
		if !ok || target.ApplicationID != binding.TargetApplicationID {
			return result, fmt.Errorf("service binding %s target deployment is unavailable", binding.ID)
		}
		if strings.TrimSpace(source.ClusterID) != strings.TrimSpace(target.ClusterID) {
			return result, fmt.Errorf("service binding %s requires source and target deployments on the same runtime cluster", binding.ID)
		}
		port, ok := serviceBindingTargetPort(binding, target)
		if !ok {
			return result, fmt.Errorf("service binding %s target service port %q:%d is unavailable or changed", binding.ID, binding.TargetPortName, binding.TargetPort)
		}
		values, err := serviceBindingValues(project, binding, target, port)
		if err != nil {
			return result, err
		}
		for key, value := range values {
			if _, exists := result.Values[key]; exists {
				return result, fmt.Errorf("service binding environment variable %s is used more than once", key)
			}
			result.Values[key] = value
		}
		digestEntries = append(digestEntries, serviceBindingDigestEntry{ID: binding.ID, Values: values})

		// 解析跨服务 Secret 引用
		secretMappings := model.DecodeSecretMap(binding.SecretMap)
		for _, m := range secretMappings {
			sourceEnvVar := strings.TrimSpace(m.SourceEnvVar)
			targetKey := strings.TrimSpace(m.TargetSecretKey)
			if sourceEnvVar == "" || targetKey == "" {
				continue
			}
			resolved := r.resolveTargetSecretKey(target, targetKey)
			if resolved == "" {
				return result, fmt.Errorf("service binding %s credentialMap references target secret key %q which is not available", binding.ID, targetKey)
			}
			if result.SecretValues == nil {
				result.SecretValues = map[string]string{}
			}
			if _, exists := result.SecretValues[sourceEnvVar]; exists {
				return result, fmt.Errorf("service binding credentialMap environment variable %s is used more than once", sourceEnvVar)
			}
			result.SecretValues[sourceEnvVar] = resolved
		}
	}

	digest, err := serviceBindingConfigDigest(digestEntries)
	if err != nil {
		return result, err
	}
	result.Digest = digest
	result.Count = len(bindings)
	return result, nil
}

func (r *Runner) resolveTargetSecretKey(target model.DeploymentTarget, key string) string {
	refs := map[string]string{}
	trimmed := strings.TrimSpace(target.SecretRefs)
	if trimmed == "" {
		return ""
	}
	if err := json.Unmarshal([]byte(trimmed), &refs); err != nil {
		return ""
	}
	ref, ok := refs[key]
	if !ok {
		return ""
	}
	return r.secrets.Resolve(ref)
}

func serviceBindingTargetPort(binding model.ServiceBinding, target model.DeploymentTarget) (model.DeploymentServicePort, bool) {
	for _, port := range model.DeploymentTargetServicePorts(target) {
		if port.Name == strings.TrimSpace(binding.TargetPortName) && port.Port == binding.TargetPort {
			return port, true
		}
	}
	return model.DeploymentServicePort{}, false
}

func serviceBindingValues(project model.Project, binding model.ServiceBinding, target model.DeploymentTarget, port model.DeploymentServicePort) (map[string]string, error) {
	host := resourcename.PersistedOrLegacy(target.KubernetesName, "dplt", target.ID) + "." +
		resourcename.PersistedOrLegacy(project.KubernetesNamespace, "ns", project.ID) + ".svc.cluster.local"
	switch strings.TrimSpace(binding.InjectionMode) {
	case "url":
		protocol := strings.ToLower(strings.TrimSpace(binding.Protocol))
		if protocol != "http" && protocol != "https" {
			return nil, fmt.Errorf("service binding %s URL injection requires http or https", binding.ID)
		}
		key := strings.TrimSpace(binding.URLEnvVar)
		if key == "" {
			return nil, fmt.Errorf("service binding %s URL environment variable is required", binding.ID)
		}
		path := strings.TrimSpace(binding.Path)
		if path != "" && !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("service binding %s URL path must start with /", binding.ID)
		}
		return map[string]string{key: protocol + "://" + host + ":" + strconv.Itoa(port.Port) + path}, nil
	case "host_port":
		hostKey := strings.TrimSpace(binding.HostEnvVar)
		portKey := strings.TrimSpace(binding.PortEnvVar)
		if hostKey == "" || portKey == "" || hostKey == portKey {
			return nil, fmt.Errorf("service binding %s host and port environment variables must be distinct", binding.ID)
		}
		return map[string]string{hostKey: host, portKey: strconv.Itoa(port.Port)}, nil
	default:
		return nil, fmt.Errorf("service binding %s has unsupported injection mode", binding.ID)
	}
}

func serviceBindingConfigDigest(entries []serviceBindingDigestEntry) (string, error) {
	for index := range entries {
		entries[index].Values = sortedStringMap(entries[index].Values)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	payload, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func sortedStringMap(values map[string]string) map[string]string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]string, len(values))
	for _, key := range keys {
		result[key] = values[key]
	}
	return result
}

func applyServiceBindingConfig(spec *kubeprovider.ApplicationResourcesSpec, bindings resolvedServiceBindingConfig) error {
	for key, value := range bindings.Values {
		if _, exists := spec.ConfigData[key]; exists {
			return fmt.Errorf("service binding environment variable %s conflicts with runtime configuration", key)
		}
		if _, exists := spec.SecretData[key]; exists {
			return fmt.Errorf("service binding environment variable %s conflicts with runtime Secret", key)
		}
		spec.ConfigData[key] = value
	}
	for key, value := range bindings.SecretValues {
		if _, exists := spec.ConfigData[key]; exists {
			return fmt.Errorf("service binding secret variable %s conflicts with runtime configuration", key)
		}
		if existing, ok := spec.SecretData[key]; ok && existing != value {
			return fmt.Errorf("service binding secret variable %s conflicts with runtime Secret", key)
		}
		spec.SecretData[key] = value
	}
	spec.ServiceBindingsDigest = bindings.Digest
	return nil
}
