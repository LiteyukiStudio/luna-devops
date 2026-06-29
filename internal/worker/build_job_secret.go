package worker

import (
	"strings"

	"github.com/LiteyukiStudio/devops/internal/builder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildJobSecret(name string, task builder.Task, npmRegistry string, cacheEnabled bool, cacheTag string) *corev1.Secret {
	env := map[string]string{
		"GIT_CLONE_URL":                 task.Repository.CloneURL,
		"GIT_ACCESS_TOKEN":              task.Repository.AccessToken,
		"SOURCE_BRANCH":                 task.Repository.SourceBranch,
		"SOURCE_TAG":                    task.Repository.SourceTag,
		"SOURCE_COMMIT":                 task.Repository.SourceCommit,
		"LITEYUKI_PROJECT_ID":           task.ProjectID,
		"LITEYUKI_APPLICATION_ID":       task.ApplicationID,
		"LITEYUKI_DEPLOYMENT_TARGET_ID": task.DeploymentTargetID,
		"LITEYUKI_BUILD_RUN_ID":         task.BuildRunID,
		"LITEYUKI_BUILD_JOB_ID":         task.JobID,
		"DOCKERFILE_PATH":               builder.StringDefault(task.Build.DockerfilePath, "Dockerfile"),
		"BUILD_CONTEXT":                 builder.StringDefault(task.Build.BuildContext, "."),
		"BUILD_DIRECTORY":               task.Build.BuildDirectory,
		"CACHE_ENABLED":                 builder.BoolEnvValue(cacheEnabled),
		"CACHE_TAG":                     builder.StringDefault(strings.TrimSpace(cacheTag), "buildcache"),
		"NPM_REGISTRY":                  strings.TrimSpace(npmRegistry),
		"REGISTRY_ENDPOINT":             task.Registry.Endpoint,
		"REGISTRY_USERNAME":             task.Registry.Username,
		"REGISTRY_PASSWORD":             task.Registry.Password,
		"IMAGE_REF":                     task.Registry.ImageRef,
		"IMAGE_NAME_PREFIX":             task.Registry.ImageNamePrefix,
		"IMAGE_TAG_TEMPLATE":            task.Registry.ImageTagTemplate,
	}
	hookIDsByPhase := builder.HookIDsByPhase(task.Build.Hooks)
	env["PRE_PULL_HOOK_IDS"] = strings.Join(hookIDsByPhase["prePull"], ",")
	env["POST_PULL_HOOK_IDS"] = strings.Join(hookIDsByPhase["postPull"], ",")
	env["PRE_BUILD_HOOK_IDS"] = strings.Join(hookIDsByPhase["preBuild"], ",")
	env["POST_BUILD_HOOK_IDS"] = strings.Join(hookIDsByPhase["postBuild"], ",")
	env["PRE_PUSH_HOOK_IDS"] = strings.Join(hookIDsByPhase["prePush"], ",")
	env["POST_PUSH_HOOK_IDS"] = strings.Join(hookIDsByPhase["postPush"], ",")
	buildEnv := builder.NormalizedBuildEnv(task.Build.Env)
	if strings.TrimSpace(npmRegistry) != "" {
		if _, ok := buildEnv["NPM_REGISTRY"]; !ok {
			buildEnv["NPM_REGISTRY"] = strings.TrimSpace(npmRegistry)
		}
		if _, ok := buildEnv["npm_config_registry"]; !ok {
			buildEnv["npm_config_registry"] = strings.TrimSpace(npmRegistry)
		}
	}
	buildEnvKeys := make([]string, 0, len(buildEnv))
	for key, value := range buildEnv {
		env[key] = value
		buildEnvKeys = append(buildEnvKeys, key)
	}
	env["BUILD_ENV_KEYS"] = strings.Join(buildEnvKeys, ",")

	data := map[string]string{"run.sh": builder.ExecutorScript()}
	for _, hook := range task.Build.Hooks {
		if strings.TrimSpace(hook.ID) == "" || strings.TrimSpace(hook.Script) == "" {
			continue
		}
		data[hook.ID+".sh"] = hook.Script
		data[hook.ID+".meta"] = builder.HookMetadataEnv(hook)
	}
	for key, value := range env {
		data["env-"+key] = value
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Type:       corev1.SecretTypeOpaque,
		StringData: data,
	}
}
