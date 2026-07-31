package builder

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

func HandleHookControlLine(line string, hookLabels map[string]string, onHookLog func(string, string) error, onHookComplete func(string, HookResult) error) (string, bool) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, ResultMarkerPrefix) {
		return "", true
	}
	if strings.HasPrefix(line, "::luna-devops-hook-log::") && onHookLog != nil {
		parts := strings.SplitN(strings.TrimPrefix(line, "::luna-devops-hook-log::"), "::", 2)
		if len(parts) != 2 {
			return "", true
		}
		content, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return "", true
		}
		hookLog := string(content)
		if err := onHookLog(parts[0], hookLog); err != nil {
			telemetry.Logger().Error("builder hook log upload failed",
				slog.String("event.name", "builder.hook_log.upload_failed"),
				slog.String("error.type", telemetry.ErrorType(err)),
			)
		}
		return formatHookLog(parts[0], hookLog, hookLabels), true
	}
	if strings.HasPrefix(line, "::luna-devops-hook-complete::") && onHookComplete != nil {
		parts := strings.SplitN(strings.TrimPrefix(line, "::luna-devops-hook-complete::"), "::", 4)
		if len(parts) != 4 {
			return "", true
		}
		exitCode, _ := strconv.Atoi(parts[2])
		message, _ := base64.StdEncoding.DecodeString(parts[3])
		result := HookResult{
			Succeeded: parts[1] == "true",
			ExitCode:  exitCode,
			Message:   string(message),
		}
		if err := onHookComplete(parts[0], result); err != nil {
			telemetry.Logger().Error("builder hook status upload failed",
				slog.String("event.name", "builder.hook_status.upload_failed"),
				slog.String("error.type", telemetry.ErrorType(err)),
			)
		}
		return formatHookLog(parts[0], result.Message, hookLabels), true
	}
	return "", false
}

func formatHookLog(hookRunID string, content string, hookLabels map[string]string) string {
	content = strings.TrimRight(content, "\n")
	if strings.TrimSpace(content) == "" {
		return ""
	}
	label := hookLabels[hookRunID]
	if label == "" {
		label = hookRunID
	}
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		lines[index] = fmt.Sprintf("[%s] %s", label, strings.TrimRight(line, "\r"))
	}
	return strings.Join(lines, "\n")
}

func hookLabelsByRunID(hooks []HookPayload) map[string]string {
	labels := make(map[string]string, len(hooks))
	for _, hook := range hooks {
		hookID := strings.TrimSpace(hook.ID)
		if hookID == "" {
			continue
		}
		phase := strings.TrimSpace(hook.Phase)
		name := strings.TrimSpace(hook.Name)
		switch {
		case phase != "" && name != "":
			labels[hookID] = phase + ": " + name
		case phase != "":
			labels[hookID] = phase
		case name != "":
			labels[hookID] = name
		default:
			labels[hookID] = hookID
		}
	}
	return labels
}

func HookLabelsByRunID(hooks []HookPayload) map[string]string {
	return hookLabelsByRunID(hooks)
}

type buildkitRawProgress struct {
	Vertexes []buildkitRawVertex  `json:"vertexes"`
	Statuses []buildkitRawStatus  `json:"statuses"`
	Logs     []buildkitRawLog     `json:"logs"`
	Warnings []buildkitRawWarning `json:"warnings"`
}

type buildkitRawVertex struct {
	Digest    string `json:"digest"`
	Name      string `json:"name"`
	Cached    bool   `json:"cached"`
	Error     string `json:"error"`
	Started   any    `json:"started"`
	Completed any    `json:"completed"`
}

type buildkitRawStatus struct {
	ID        string `json:"id"`
	Vertex    string `json:"vertex"`
	Name      string `json:"name"`
	Started   any    `json:"started"`
	Completed any    `json:"completed"`
}

type buildkitRawLog struct {
	Vertex string `json:"vertex"`
	Stream int    `json:"stream"`
	Data   string `json:"data"`
}

type buildkitRawWarning struct {
	Vertex string   `json:"vertex"`
	Short  string   `json:"short"`
	Detail []string `json:"detail"`
	URL    string   `json:"url"`
}

func buildkitProgressFromRawJSONLine(line string) Progress {
	line = strings.TrimSpace(strings.TrimRight(line, "\r"))
	if line == "" {
		return Progress{}
	}
	if !strings.HasPrefix(line, "{") {
		return plainProgressFromLogLine(line)
	}
	var raw buildkitRawProgress
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return Progress{}
	}
	for i := len(raw.Statuses) - 1; i >= 0; i-- {
		status := raw.Statuses[i]
		if key := buildkitProgressKey(status.Name); key != "" {
			return Progress{
				Key:       key,
				Name:      status.Name,
				Vertex:    status.Vertex,
				Started:   status.Started != nil,
				Completed: status.Completed != nil,
			}
		}
	}
	for i := len(raw.Vertexes) - 1; i >= 0; i-- {
		vertex := raw.Vertexes[i]
		if key := buildkitProgressKey(vertex.Name); key != "" {
			return Progress{
				Key:       key,
				Name:      vertex.Name,
				Vertex:    vertex.Digest,
				Cached:    vertex.Cached,
				Started:   vertex.Started != nil,
				Completed: vertex.Completed != nil,
				Error:     vertex.Error,
			}
		}
	}
	return Progress{}
}

func ProgressFromLogLine(line string) Progress {
	return buildkitProgressFromRawJSONLine(line)
}

func ParseResultMarkerLine(line string) (Result, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, ResultMarkerPrefix) {
		return Result{}, false
	}
	content, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, ResultMarkerPrefix))
	if err != nil {
		return Result{}, true
	}
	var result Result
	_ = json.Unmarshal(content, &result)
	return result, true
}

func plainProgressFromLogLine(line string) Progress {
	value := strings.TrimSpace(strings.TrimRight(line, "\r"))
	if value == "" {
		return Progress{}
	}
	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "cloning into"):
		return Progress{Key: "clone_repository", Name: value, Started: true}
	case strings.HasPrefix(lower, "already on ") || strings.HasPrefix(lower, "your branch is up to date"):
		return Progress{Key: "clone_repository", Name: value, Completed: true}
	}
	if !strings.HasPrefix(value, "#") {
		return Progress{}
	}
	name := plainBuildkitStepName(value)
	if name == "" {
		return Progress{}
	}
	key := buildkitProgressKey(name)
	if key == "" {
		return Progress{}
	}
	return Progress{
		Key:       key,
		Name:      name,
		Started:   !strings.Contains(lower, " done") && !strings.Contains(lower, "done "),
		Completed: strings.Contains(lower, " done") || strings.Contains(lower, "done "),
	}
}

func plainBuildkitStepName(line string) string {
	trimmed := strings.TrimSpace(line)
	firstSpace := strings.IndexByte(trimmed, ' ')
	if firstSpace < 0 || firstSpace+1 >= len(trimmed) {
		return ""
	}
	rest := strings.TrimSpace(trimmed[firstSpace+1:])
	if rest == "" || strings.HasPrefix(rest, "...") || strings.HasPrefix(strings.ToUpper(rest), "DONE ") {
		return ""
	}
	if fields := strings.Fields(rest); len(fields) > 0 {
		first := strings.ToUpper(fields[0])
		if strings.HasPrefix(first, "CACHED") || first == "DONE" || first == "ERROR" {
			return ""
		}
	}
	return rest
}

func buildkitProgressKey(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	if value == "" {
		return ""
	}
	switch {
	case strings.Contains(value, "[auth]"):
		return "registry_auth"
	case strings.Contains(value, "load build definition"):
		return "load_dockerfile"
	case strings.Contains(value, "load metadata for"):
		return "pull_image_metadata"
	case strings.Contains(value, "load build context") || strings.Contains(value, "transferring context"):
		return "upload_build_context"
	case strings.Contains(value, "resolve image config") || strings.HasPrefix(value, "from "):
		return "pull_base_image"
	case strings.HasPrefix(value, "run ") || strings.Contains(value, " run "):
		return "run_command"
	case strings.Contains(value, "exporting to image") || strings.Contains(value, "exporting manifest") || strings.Contains(value, "exporting config"):
		return "export_image"
	case strings.Contains(value, "pushing layers"):
		return "push_image_layers"
	case strings.Contains(value, "pushing manifest"):
		return "push_image_manifest"
	default:
		return ""
	}
}
