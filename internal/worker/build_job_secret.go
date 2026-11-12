package worker

import (
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/builder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const buildUserVarSecretPrefix = "var-"

// buildProxyEnvKeys 是允许进入执行器进程环境的用户变量。BuildKit 守护进程拉取基础镜像与访问镜像站时
// 只读取进程环境，build-arg 与 /executor/vars 文件对这部分流量无效；其余用户变量仍以文件交付，
// 避免覆盖 IMAGE_REF、GIT_CLONE_URL 等平台变量。
var buildProxyEnvKeys = map[string]bool{
	"HTTP_PROXY":  true,
	"HTTPS_PROXY": true,
	"NO_PROXY":    true,
	"http_proxy":  true,
	"https_proxy": true,
	"no_proxy":    true,
}

func buildJobSecret(name string, task builder.Task, cacheEnabled bool) *corev1.Secret {
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
		"BUILD_DEFINITION_MODE":         builder.StringDefault(task.Build.DefinitionMode, "repository_dockerfile"),
		"BUILD_CONTEXT":                 builder.StringDefault(task.Build.BuildContext, "."),
		"BUILD_DIRECTORY":               task.Build.BuildDirectory,
		"CACHE_ENABLED":                 builder.BoolEnvValue(cacheEnabled),
		"CACHE_TAG":                     builder.RegistryCacheTag,
		"BUILD_CONTAINS_SECRETS":        builder.BoolEnvValue(task.Build.ContainsSecrets),
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
	// 用户变量以文件交付，不进入执行器进程环境，避免覆盖 IMAGE_REF、GIT_CLONE_URL 等平台变量；
	// 代理类变量例外，见下。
	userVars := map[string]string{}
	buildArgs := builder.NormalizedBuildEnv(task.Build.BuildArgs)
	buildEnvKeys := make([]string, 0, len(task.Build.Env))
	for key, value := range builder.NormalizedBuildEnv(task.Build.Env) {
		if _, overridden := buildArgs[key]; overridden {
			continue
		}
		userVars[key] = value
		buildEnvKeys = append(buildEnvKeys, key)
	}
	buildArgKeys := make([]string, 0, len(buildArgs))
	for key, value := range buildArgs {
		userVars[key] = value
		buildArgKeys = append(buildArgKeys, key)
	}
	sort.Strings(buildEnvKeys)
	sort.Strings(buildArgKeys)
	// 代理变量同时进入执行器进程环境，让 BuildKit 拉取、推送镜像与访问 registry 时走代理。
	for key, value := range userVars {
		if buildProxyEnvKeys[key] {
			env[key] = value
		}
	}
	env["BUILD_ENV_KEYS"] = strings.Join(buildEnvKeys, ",")
	env["BUILD_ARG_KEYS"] = strings.Join(buildArgKeys, ",")

	data := map[string]string{"run.sh": builder.ExecutorScript()}
	if strings.TrimSpace(task.Build.TemplateDockerfile) != "" {
		data["template.Dockerfile"] = task.Build.TemplateDockerfile
	}
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
	for key, value := range userVars {
		data[buildUserVarSecretPrefix+key] = value
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Type:       corev1.SecretTypeOpaque,
		StringData: data,
	}
}
