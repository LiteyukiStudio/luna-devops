package builder

const (
	ResultMarkerPrefix = "::luna-devops-build-result::"

	// RegistryCacheTag 是 BuildKit Registry 缓存在目标镜像仓库中占用的保留 Tag。
	// 执行器拒绝推送同名镜像 Tag，发布候选列表也据此过滤缓存清单。
	RegistryCacheTag = "buildcache"

	hookPhasePrePull   = "prePull"
	hookPhasePostPull  = "postPull"
	hookPhasePreBuild  = "preBuild"
	hookPhasePostBuild = "postBuild"
	hookPhasePrePush   = "prePush"
	hookPhasePostPush  = "postPush"
)

var buildHookPhases = []string{
	hookPhasePrePull,
	hookPhasePostPull,
	hookPhasePreBuild,
	hookPhasePostBuild,
	hookPhasePrePush,
	hookPhasePostPush,
}

type Progress struct {
	Key       string `json:"key"`
	Name      string `json:"name,omitempty"`
	Vertex    string `json:"vertex,omitempty"`
	Cached    bool   `json:"cached,omitempty"`
	Started   bool   `json:"started,omitempty"`
	Completed bool   `json:"completed,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Task struct {
	JobID              string            `json:"jobId"`
	BuildRunID         string            `json:"buildRunId"`
	ProjectID          string            `json:"projectId"`
	ApplicationID      string            `json:"applicationId"`
	DeploymentTargetID string            `json:"deploymentTargetId"`
	Repository         RepositoryPayload `json:"repository"`
	Build              BuildPayload      `json:"build"`
	Registry           RegistryPayload   `json:"registry"`
}

type RepositoryPayload struct {
	CloneURL     string `json:"cloneUrl"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	SourceBranch string `json:"sourceBranch"`
	SourceTag    string `json:"sourceTag"`
	SourceCommit string `json:"sourceCommit"`
	AccessToken  string `json:"accessToken"`
}

type BuildPayload struct {
	DefinitionMode     string            `json:"definitionMode"`
	TemplateDockerfile string            `json:"templateDockerfile"`
	DockerfilePath     string            `json:"dockerfilePath"`
	BuildContext       string            `json:"buildContext"`
	BuildDirectory     string            `json:"buildDirectory"`
	BuildArgs          map[string]string `json:"buildArgs"`
	Env                map[string]string `json:"env"`
	// ContainsSecrets 表示 Env 中含有解析出的构建密钥；此时 Registry 缓存只导出最终镜像已有的层。
	ContainsSecrets bool          `json:"containsSecrets"`
	Hooks           []HookPayload `json:"hooks"`
}

type HookPayload struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Phase          string `json:"phase"`
	Script         string `json:"script"`
	Shell          string `json:"shell"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	FailurePolicy  string `json:"failurePolicy"`
}

type HookResult struct {
	Succeeded bool   `json:"succeeded"`
	ExitCode  int    `json:"exitCode"`
	Message   string `json:"message"`
}

type RegistryPayload struct {
	Endpoint         string `json:"endpoint"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	ImageRef         string `json:"imageRef"`
	ImageNamePrefix  string `json:"imageNamePrefix"`
	ImageTagTemplate string `json:"imageTagTemplate"`
}

type Result struct {
	ImageRef          string `json:"imageRef"`
	ImageDigest       string `json:"imageDigest"`
	SourceCommit      string `json:"sourceCommit"`
	SourceAuthorName  string `json:"sourceAuthorName"`
	SourceAuthorEmail string `json:"sourceAuthorEmail"`
	// SourceCommitMessageBase64 是检出提交的完整消息；执行器的 JSON 转义不处理换行和控制字符，因此以 base64 传递。
	SourceCommitMessageBase64 string `json:"sourceCommitMessageBase64"`
	Message                   string `json:"message"`
}
