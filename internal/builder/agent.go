package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Options struct {
	APIURL            string
	RedisAddr         string
	Token             string
	Transport         string
	AgentID           string
	Name              string
	Executor          string
	ExecutorImage     string
	Labels            []string
	MaxConcurrency    int
	Scopes            []string
	PollInterval      time.Duration
	WorkspaceRoot     string
	WorkspaceHostRoot string
	NPMRegistry       string
	DockerBinary      string
}

type Agent struct {
	options   Options
	transport Transport
}

func New(options Options) *Agent {
	if options.MaxConcurrency <= 0 {
		options.MaxConcurrency = 1
	}
	if options.PollInterval <= 0 {
		options.PollInterval = 5 * time.Second
	}
	if strings.TrimSpace(options.Executor) == "" {
		options.Executor = "docker"
	}
	if strings.TrimSpace(options.Transport) == "" {
		options.Transport = TransportRedis
	}
	if strings.TrimSpace(options.ExecutorImage) == "" {
		options.ExecutorImage = "moby/buildkit:v0.24.0-rootless"
	}
	if strings.TrimSpace(options.WorkspaceRoot) == "" {
		options.WorkspaceRoot = "/builder-workspace"
	}
	if strings.TrimSpace(options.DockerBinary) == "" {
		options.DockerBinary = "docker"
	}
	return &Agent{options: options}
}

func (a *Agent) Run(ctx context.Context) error {
	agentName := strings.TrimSpace(a.options.Name)
	if agentName == "" {
		agentName = "local-builder"
	}
	a.options.Name = agentName
	a.options.AgentID = agentName
	a.options.Transport = TransportRedis
	transport, err := NewTransport(a.options)
	if err != nil {
		return err
	}
	a.transport = transport
	defer func() {
		if err := a.transport.Close(); err != nil {
			log.Printf("builder transport close failed: %v", err)
		}
	}()
	if err := os.MkdirAll(a.options.WorkspaceRoot, 0o700); err != nil {
		return err
	}
	ticker := time.NewTicker(a.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := a.heartbeat(ctx, 0); err != nil {
			log.Printf("builder heartbeat failed: %v", err)
		}
		task, ok, err := a.claim(ctx)
		if err != nil {
			log.Printf("builder claim failed: %v", err)
		}
		if ok {
			a.runTask(ctx, task)
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *Agent) runTask(ctx context.Context, task Task) {
	log.Printf("builder task claimed: job=%s run=%s image=%s", task.JobID, task.BuildRunID, task.Registry.ImageRef)
	if err := a.heartbeat(ctx, 1); err != nil {
		log.Printf("builder heartbeat failed: %v", err)
	}
	result, logs, err := a.executeDockerTask(ctx, task, func(content string) error {
		return a.appendLogs(ctx, task.JobID, content)
	})
	if err != nil {
		message := err.Error()
		if logs != "" {
			message = firstLine(logs, message)
		}
		if failErr := a.fail(ctx, task.JobID, message); failErr != nil {
			log.Printf("builder fail report failed: %v", failErr)
		}
		return
	}
	if err := a.complete(ctx, task.JobID, result); err != nil {
		log.Printf("builder complete report failed: %v", err)
	}
}

func (a *Agent) heartbeat(ctx context.Context, current int) error {
	return a.transport.Heartbeat(ctx, Heartbeat{
		AgentID:            a.options.AgentID,
		Name:               a.options.Name,
		Labels:             a.options.Labels,
		Scopes:             a.options.Scopes,
		Executor:           a.options.Executor,
		MaxConcurrency:     a.options.MaxConcurrency,
		CurrentConcurrency: current,
	})
}

func (a *Agent) claim(ctx context.Context) (Task, bool, error) {
	task, err := a.transport.Claim(ctx)
	if errors.Is(err, errNoTask) {
		return Task{}, false, nil
	}
	return task, task.JobID != "", err
}

func (a *Agent) appendLogs(ctx context.Context, jobID string, content string) error {
	return a.transport.AppendLogs(ctx, jobID, content)
}

func (a *Agent) complete(ctx context.Context, jobID string, result Result) error {
	return a.transport.Complete(ctx, jobID, result)
}

func (a *Agent) fail(ctx context.Context, jobID string, message string) error {
	return a.transport.Fail(ctx, jobID, message)
}

func (a *Agent) executeDockerTask(ctx context.Context, task Task, onLog func(string) error) (Result, string, error) {
	if a.options.Executor != "docker" && a.options.Executor != "docker-dind" {
		return Result{}, "", fmt.Errorf("unsupported builder executor: %s", a.options.Executor)
	}
	workspace, err := os.MkdirTemp(a.options.WorkspaceRoot, task.JobID+"-")
	if err != nil {
		return Result{}, "", err
	}
	defer os.RemoveAll(workspace)
	volumeWorkspace := a.executorVolumeWorkspace(workspace)
	scriptPath := filepath.Join(workspace, "run.sh")
	resultPath := filepath.Join(workspace, "result.json")
	if err := os.WriteFile(scriptPath, []byte(executorScript()), 0o700); err != nil {
		return Result{}, "", err
	}
	args := []string{
		"run", "--rm",
		"--privileged",
		"--security-opt", "seccomp=unconfined",
		"--entrypoint", "/bin/sh",
		"-v", volumeWorkspace + ":/workspace",
		"-e", "GIT_CLONE_URL=" + task.Repository.CloneURL,
		"-e", "GIT_ACCESS_TOKEN=" + task.Repository.AccessToken,
		"-e", "SOURCE_BRANCH=" + task.Repository.SourceBranch,
		"-e", "SOURCE_TAG=" + task.Repository.SourceTag,
		"-e", "SOURCE_COMMIT=" + task.Repository.SourceCommit,
		"-e", "DOCKERFILE_PATH=" + task.Build.DockerfilePath,
		"-e", "BUILD_CONTEXT=" + task.Build.BuildContext,
		"-e", "BUILD_DIRECTORY=" + task.Build.BuildDirectory,
		"-e", "NPM_REGISTRY=" + a.options.NPMRegistry,
		"-e", "REGISTRY_ENDPOINT=" + task.Registry.Endpoint,
		"-e", "REGISTRY_USERNAME=" + task.Registry.Username,
		"-e", "REGISTRY_PASSWORD=" + task.Registry.Password,
		"-e", "IMAGE_REF=" + task.Registry.ImageRef,
		"-e", "IMAGE_NAME_PREFIX=" + task.Registry.ImageNamePrefix,
		"-e", "IMAGE_TAG_TEMPLATE=" + task.Registry.ImageTagTemplate,
	}
	buildEnv := normalizedBuildEnv(task.Build.Env)
	if strings.TrimSpace(a.options.NPMRegistry) != "" {
		if _, ok := buildEnv["NPM_REGISTRY"]; !ok {
			buildEnv["NPM_REGISTRY"] = strings.TrimSpace(a.options.NPMRegistry)
		}
		if _, ok := buildEnv["npm_config_registry"]; !ok {
			buildEnv["npm_config_registry"] = strings.TrimSpace(a.options.NPMRegistry)
		}
	}
	buildEnvKeys := make([]string, 0, len(buildEnv))
	for key := range buildEnv {
		buildEnvKeys = append(buildEnvKeys, key)
	}
	sort.Strings(buildEnvKeys)
	args = append(args, "-e", "BUILD_ENV_KEYS="+strings.Join(buildEnvKeys, ","))
	for _, key := range buildEnvKeys {
		args = append(args, "-e", key+"="+buildEnv[key])
	}
	args = append(args, a.options.ExecutorImage, "/workspace/run.sh")
	cmd := exec.CommandContext(ctx, a.options.DockerBinary, args...)
	output := newConcurrentBuffer()
	streamer := newLogStreamer(ctx, output, onLog)
	defer streamer.Close()
	cmd.Stdout = streamer
	cmd.Stderr = streamer
	err = cmd.Run()
	streamer.Close()
	result := Result{ImageRef: task.Registry.ImageRef}
	if data, readErr := os.ReadFile(resultPath); readErr == nil {
		_ = json.Unmarshal(data, &result)
	}
	return result, output.String(), err
}

func (a *Agent) executorVolumeWorkspace(workspace string) string {
	hostRoot := strings.TrimSpace(a.options.WorkspaceHostRoot)
	if hostRoot == "" {
		return workspace
	}
	rel, err := filepath.Rel(a.options.WorkspaceRoot, workspace)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return workspace
	}
	return filepath.Join(hostRoot, rel)
}

func executorScript() string {
	return `set -eu
cd /workspace
AUTH_CLONE_URL="$GIT_CLONE_URL"
if [ -n "${GIT_ACCESS_TOKEN:-}" ]; then
  case "$AUTH_CLONE_URL" in
    https://*) AUTH_CLONE_URL="$(printf "%s" "$AUTH_CLONE_URL" | sed "s#https://#https://x-access-token:${GIT_ACCESS_TOKEN}@#")" ;;
  esac
fi
clone_with_retry() {
  attempt=1
  while [ "$attempt" -le 3 ]; do
    rm -rf source
    if [ -n "${SOURCE_BRANCH:-}" ]; then
      if git clone --depth 1 --single-branch --branch "$SOURCE_BRANCH" "$AUTH_CLONE_URL" source; then
        return 0
      fi
    else
      if git clone --depth 1 "$AUTH_CLONE_URL" source; then
        return 0
      fi
    fi
    if [ "$attempt" -eq 3 ]; then
      return 1
    fi
    sleep $((attempt * 2))
    attempt=$((attempt + 1))
  done
}
clone_with_retry
cd source
if [ -n "${SOURCE_TAG:-}" ]; then git checkout "$SOURCE_TAG"; fi
if [ -n "${SOURCE_BRANCH:-}" ]; then git checkout "$SOURCE_BRANCH"; fi
if [ -n "${SOURCE_COMMIT:-}" ]; then git checkout "$SOURCE_COMMIT"; fi
CHECKED_OUT_COMMIT="$(git rev-parse HEAD)"
short_commit() {
  value="$1"
  printf "%s" "$(printf "%.12s" "$value")"
}
sanitize_tag() {
  printf "%s" "$1" \
    | sed -E 's/[^A-Za-z0-9_.-]+/-/g; s/^[.-]+//; s/[.-]+$//' \
    | cut -c 1-128
}
escape_sed_pattern() {
  printf "%s" "$1" | sed 's/[][\/.^$*+?{}()|]/\\&/g'
}
escape_sed_replacement() {
  printf "%s" "$1" | sed 's/[\/&]/\\&/g'
}
replace_token() {
  pattern="$(escape_sed_pattern "$1")"
  replacement="$(escape_sed_replacement "$2")"
  printf "%s" "$3" | sed -E "s/$pattern/$replacement/g"
}
render_image_tag() {
  template="${IMAGE_TAG_TEMPLATE:-latest}"
  ref_name="${SOURCE_TAG:-$SOURCE_BRANCH}"
  ref_type="branch"
  ref_value=""
  if [ -n "${SOURCE_TAG:-}" ]; then
    ref_type="tag"
    ref_value="refs/tags/$SOURCE_TAG"
  elif [ -n "${SOURCE_BRANCH:-}" ]; then
    ref_value="refs/heads/$SOURCE_BRANCH"
  fi
  rendered="$template"
  short_sha="$(short_commit "$CHECKED_OUT_COMMIT")"
  rendered="$(replace_token '${{ github.sha }}' "$CHECKED_OUT_COMMIT" "$rendered")"
  rendered="$(replace_token '${{ github.ref_name }}' "$ref_name" "$rendered")"
  rendered="$(replace_token '${{ github.ref_type }}' "$ref_type" "$rendered")"
  rendered="$(replace_token '${{ github.ref }}' "$ref_value" "$rendered")"
  rendered="$(replace_token '${{ github.head_ref }}' "$SOURCE_BRANCH" "$rendered")"
  rendered="$(replace_token '${{ github.base_ref }}' "" "$rendered")"
  rendered="$(replace_token '{sha}' "$CHECKED_OUT_COMMIT" "$rendered")"
  rendered="$(replace_token '{commit}' "$CHECKED_OUT_COMMIT" "$rendered")"
  rendered="$(replace_token '{short_sha}' "$short_sha" "$rendered")"
  rendered="$(replace_token '{commit_short}' "$short_sha" "$rendered")"
  rendered="$(replace_token '{branch}' "$SOURCE_BRANCH" "$rendered")"
  rendered="$(replace_token '{tag}' "$SOURCE_TAG" "$rendered")"
  rendered="$(replace_token '{ref_name}' "$ref_name" "$rendered")"
  sanitized="$(sanitize_tag "$rendered")"
  if [ -z "$sanitized" ]; then
    sanitized="latest"
  fi
  printf "%s" "$sanitized"
}
if [ -n "${IMAGE_NAME_PREFIX:-}" ]; then
  IMAGE_REF="${IMAGE_NAME_PREFIX}:$(render_image_tag)"
fi
if [ -n "${NPM_REGISTRY:-}" ]; then
  mkdir -p "$PWD/$BUILD_CONTEXT"
  printf "registry=%s\n" "$NPM_REGISTRY" > "$PWD/$BUILD_CONTEXT/.npmrc"
fi
set --
OLDIFS="$IFS"
IFS=","
for key in ${BUILD_ENV_KEYS:-}; do
  if [ -z "$key" ]; then
    continue
  fi
  eval "value=\${$key:-}"
  set -- "$@" --opt "build-arg:${key}=${value}"
done
IFS="$OLDIFS"
mkdir -p "$HOME/.docker"
AUTH="$(printf "%s:%s" "$REGISTRY_USERNAME" "$REGISTRY_PASSWORD" | base64 | tr -d "\n")"
printf '{"auths":{"%s":{"auth":"%s"}}}' "$REGISTRY_ENDPOINT" "$AUTH" > "$HOME/.docker/config.json"
build_with_retry() {
  attempt=1
  while [ "$attempt" -le 3 ]; do
    if buildctl-daemonless.sh build \
      --frontend dockerfile.v0 \
      --local context="$PWD/$BUILD_CONTEXT" \
      --local dockerfile="$PWD/$(dirname "$DOCKERFILE_PATH")" \
      --opt filename="$(basename "$DOCKERFILE_PATH")" \
      "$@" \
      --output type=image,name="$IMAGE_REF",push=true; then
      return 0
    fi
    if [ "$attempt" -eq 3 ]; then
      return 1
    fi
    sleep $((attempt * 3))
    attempt=$((attempt + 1))
  done
}
build_with_retry "$@"
printf '{"imageRef":"%s","sourceCommit":"%s","message":"builder task succeeded"}' "$IMAGE_REF" "$CHECKED_OUT_COMMIT" > /workspace/result.json
`
}

func defaultAgentID() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "builder-local"
	}
	return "builder-" + strings.ToLower(strings.ReplaceAll(hostname, "_", "-"))
}

func firstLine(content string, fallback string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return fallback
}

func normalizedBuildEnv(values map[string]string) map[string]string {
	output := make(map[string]string)
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if isBuildEnvKey(key) {
			output[key] = value
		}
	}
	return output
}

func isBuildEnvKey(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, char := range value {
		if index == 0 {
			if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' {
				continue
			}
			return false
		}
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

type concurrentBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func newConcurrentBuffer() *concurrentBuffer {
	return &concurrentBuffer{}
}

func (b *concurrentBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *concurrentBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type logStreamer struct {
	ctx       context.Context
	output    io.Writer
	onLog     func(string) error
	mu        sync.Mutex
	pending   bytes.Buffer
	done      chan struct{}
	closeOnce sync.Once
}

func newLogStreamer(ctx context.Context, output io.Writer, onLog func(string) error) *logStreamer {
	streamer := &logStreamer{
		ctx:     ctx,
		output:  output,
		onLog:   onLog,
		done:    make(chan struct{}),
		pending: bytes.Buffer{},
	}
	go streamer.flushLoop()
	return streamer
}

func (s *logStreamer) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if _, err := s.output.Write(data); err != nil {
		return 0, err
	}
	s.mu.Lock()
	_, _ = s.pending.Write(data)
	shouldFlush := s.pending.Len() >= 8192
	s.mu.Unlock()
	if shouldFlush {
		s.flush()
	}
	return len(data), nil
}

func (s *logStreamer) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.flush()
	})
}

func (s *logStreamer) flushLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.flush()
		case <-s.done:
			return
		case <-s.ctx.Done():
			s.flush()
			return
		}
	}
}

func (s *logStreamer) flush() {
	if s.onLog == nil {
		return
	}
	s.mu.Lock()
	content := s.pending.String()
	s.pending.Reset()
	s.mu.Unlock()
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return
	}
	if err := s.onLog(content); err != nil {
		log.Printf("builder realtime log upload failed: %v", err)
	}
}

type Task struct {
	StreamID      string            `json:"-"`
	JobID         string            `json:"jobId"`
	TargetBuilder string            `json:"targetBuilder"`
	BuildRunID    string            `json:"buildRunId"`
	ProjectID     string            `json:"projectId"`
	ApplicationID string            `json:"applicationId"`
	Repository    RepositoryPayload `json:"repository"`
	Build         BuildPayload      `json:"build"`
	Registry      RegistryPayload   `json:"registry"`
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
	DockerfilePath string            `json:"dockerfilePath"`
	BuildContext   string            `json:"buildContext"`
	BuildDirectory string            `json:"buildDirectory"`
	Env            map[string]string `json:"env"`
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
	ImageRef     string `json:"imageRef"`
	ImageDigest  string `json:"imageDigest"`
	SourceCommit string `json:"sourceCommit"`
	Message      string `json:"message"`
}
