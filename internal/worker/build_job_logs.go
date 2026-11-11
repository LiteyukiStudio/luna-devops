package worker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (r *Runner) followBuildJob(ctx context.Context, client kubernetes.Interface, namespace string, jobName string, job model.BuildJob, run model.BuildRun, hooks []builder.HookPayload, sensitiveValues []string) (builder.Result, error) {
	timeoutSeconds := effectiveBuildTimeoutSeconds(run.BuildTimeoutSeconds, r.buildJobTimeoutSeconds)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	hookLabels := builder.HookLabelsByRunID(hooks)
	resultCh := make(chan builder.Result, 1)
	logErrCh := make(chan error, 1)
	go func() {
		result, err := r.streamBuildPodLogs(ctx, client, namespace, jobName, job, hookLabels, sensitiveValues)
		if err != nil {
			logErrCh <- err
			return
		}
		resultCh <- result
	}()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var result builder.Result
	for {
		select {
		case parsed := <-resultCh:
			result = parsed
		case err := <-logErrCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				return result, err
			}
		case <-ctx.Done():
			_ = r.deleteKubernetesBuildJob(context.WithoutCancel(ctx), client, namespace, jobName)
			return result, fmt.Errorf("%w after %ds", errBuildJobTimedOut, timeoutSeconds)
		case <-ticker.C:
			canceled, err := r.buildRunCanceled(job)
			if err != nil {
				return result, err
			}
			if canceled {
				_ = r.deleteKubernetesBuildJob(context.WithoutCancel(ctx), client, namespace, jobName)
				return result, errBuildRunCanceled
			}
			kubeJob, err := client.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
			if err != nil {
				return result, err
			}
			if kubeJob.Status.Succeeded > 0 {
				if strings.TrimSpace(result.ImageRef) == "" {
					select {
					case parsed := <-resultCh:
						result = parsed
					case <-time.After(2 * time.Second):
					}
				}
				return result, nil
			}
			if kubeJob.Status.Failed > 0 {
				message := r.buildKubernetesJobFailureMessage(ctx, client, namespace, jobName, firstNonEmpty(result.Message, "kubernetes build job failed"))
				return result, fmt.Errorf("%w: %s", errBuildJobFailed, message)
			}
		}
	}
}
func (r *Runner) streamBuildPodLogs(ctx context.Context, client kubernetes.Interface, namespace string, jobName string, job model.BuildJob, hookLabels map[string]string, sensitiveValues []string) (builder.Result, error) {
	podName, err := waitForBuildPod(ctx, client, namespace, jobName)
	if err != nil {
		return builder.Result{}, err
	}
	stream, err := client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{Follow: true, Container: "executor"}).Stream(ctx)
	if err != nil {
		return builder.Result{}, err
	}
	defer stream.Close()
	reader := bufio.NewReader(stream)
	var result builder.Result
	var lastProgressKey string
	steps := buildStepProgress{counter: builder.NewStepCounter()}
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimRight(line, "\n")
		if parsed, ok := builder.ParseResultMarkerLine(line); ok {
			if strings.TrimSpace(parsed.ImageRef) != "" {
				result = parsed
			}
			if err == nil {
				continue
			}
		}
		if rendered, control := builder.HandleHookControlLine(line, hookLabels, func(hookRunID string, content string) error {
			return r.appendBuildHookRunLog(hookRunID, job.ProjectID, content, sensitiveValues)
		}, func(hookRunID string, hookResult builder.HookResult) error {
			return r.completeBuildHookRun(hookRunID, job.ProjectID, hookResult)
		}); control {
			if strings.TrimSpace(rendered) != "" {
				r.appendBuildLog(job, rendered, sensitiveValues)
			}
		} else if strings.TrimSpace(line) != "" {
			r.appendBuildLog(job, line, sensitiveValues)
			progress := builder.ProgressFromLogLine(line)
			if progress.Key != "" && progress.Key != lastProgressKey {
				lastProgressKey = progress.Key
				_ = r.db.Model(&model.BuildJob{}).Where("id = ? and status = ?", job.ID, "running").Update("message", progress.Key).Error
			}
			if steps.counter.Observe(line) {
				steps.sensitiveValues = sensitiveValues
				steps.dirty = true
			}
			if steps.dirty && time.Since(steps.writtenAt) >= buildStepProgressInterval {
				r.recordBuildStepProgress(ctx, job, &steps)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if steps.dirty {
					r.recordBuildStepProgress(ctx, job, &steps)
				}
				return result, nil
			}
			return result, err
		}
	}
}

func waitForBuildPod(ctx context.Context, client kubernetes.Interface, namespace string, jobName string) (string, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
		if err != nil {
			return "", err
		}
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil {
				continue
			}
			if buildPodLogsAvailable(pod) {
				return pod.Name, nil
			}
			if err := buildPodStartupError(pod); err != nil {
				return "", err
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func buildPodLogsAvailable(pod corev1.Pod) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != "executor" {
			continue
		}
		return status.State.Running != nil || status.State.Terminated != nil
	}
	return false
}

func buildPodStartupError(pod corev1.Pod) error {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != "executor" || status.State.Waiting == nil {
			continue
		}
		waiting := status.State.Waiting
		switch waiting.Reason {
		case "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "CreateContainerConfigError", "CreateContainerError":
			return fmt.Errorf("build pod %s executor failed to start: %s: %s", pod.Name, waiting.Reason, waiting.Message)
		}
	}
	return nil
}

// buildStepProgressInterval 限制步骤进度的写库频率；缓存命中时步骤会在一秒内连续完成。
const buildStepProgressInterval = time.Second

type buildStepProgress struct {
	counter         *builder.StepCounter
	sensitiveValues []string
	dirty           bool
	writtenAt       time.Time
}

// maxBuildCurrentStepBytes 限制保存的当前指令长度；超长指令截断并以省略号结尾。
const maxBuildCurrentStepBytes = 512

// buildCurrentStepText 与构建日志使用相同的脱敏规则，再按 UTF-8 字符边界截断。
func buildCurrentStepText(instruction string, sensitiveValues []string) string {
	value := strings.TrimSpace(redactSensitiveLogContent(instruction, sensitiveValues))
	if len(value) <= maxBuildCurrentStepBytes {
		return value
	}
	cut := maxBuildCurrentStepBytes - len("…")
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return strings.TrimSpace(value[:cut]) + "…"
}

func (r *Runner) recordBuildStepProgress(ctx context.Context, job model.BuildJob, progress *buildStepProgress) {
	completed, total := progress.counter.Counts()
	err := r.db.WithContext(ctx).Model(&model.BuildJob{}).
		Where("id = ? and status = ?", job.ID, "running").
		Updates(map[string]any{
			"steps_completed": completed,
			"steps_total":     total,
			"current_step":    buildCurrentStepText(progress.counter.Current(), progress.sensitiveValues),
		}).Error
	if err != nil {
		return
	}
	progress.dirty = false
	progress.writtenAt = time.Now()
}

func (r *Runner) appendBuildLog(job model.BuildJob, content string, sensitiveValues []string) {
	content = trimBuildLogContent(redactSensitiveLogContent(content, sensitiveValues))
	if strings.TrimSpace(content) == "" {
		return
	}
	var existing model.BuildLog
	err := r.db.First(&existing, "build_job_id = ? and project_id = ?", job.ID, job.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = r.db.Create(&model.BuildLog{
			ID:         id.New("blog"),
			ProjectID:  job.ProjectID,
			BuildRunID: job.BuildRunID,
			BuildJobID: job.ID,
			Content:    content,
		}).Error
		return
	}
	if err != nil {
		return
	}
	existing.Content = trimBuildLogContent(existing.Content + "\n" + content)
	_ = r.db.Save(&existing).Error
}

func (r *Runner) appendBuildHookRunLog(hookRunID string, projectID string, content string, sensitiveValues []string) error {
	content = trimBuildLogContent(redactSensitiveLogContent(content, sensitiveValues))
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var hookRun model.HookRun
	if err := r.db.First(&hookRun, "id = ? and project_id = ?", hookRunID, projectID).Error; err != nil {
		return err
	}
	if hookRun.Status == "queued" {
		now := time.Now()
		_ = r.db.Model(&hookRun).Updates(map[string]any{"status": "running", "started_at": &now}).Error
	}
	var existing model.HookRunLog
	err := r.db.First(&existing, "hook_run_id = ? and project_id = ?", hookRun.ID, hookRun.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(&model.HookRunLog{
			ID:        id.New("hlog"),
			ProjectID: hookRun.ProjectID,
			HookRunID: hookRun.ID,
			Content:   content,
		}).Error
	}
	if err != nil {
		return err
	}
	existing.Content = trimBuildLogContent(existing.Content + "\n" + content)
	return r.db.Save(&existing).Error
}

func (r *Runner) completeBuildHookRun(hookRunID string, projectID string, result builder.HookResult) error {
	finishedAt := time.Now()
	status := "failed"
	if result.Succeeded {
		status = "succeeded"
	}
	return r.db.Model(&model.HookRun{}).Where("id = ? and project_id = ?", hookRunID, projectID).Updates(map[string]any{
		"status":      status,
		"exit_code":   result.ExitCode,
		"message":     result.Message,
		"finished_at": &finishedAt,
	}).Error
}

func trimBuildLogContent(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 262144 {
		return content
	}
	return content[len(content)-262144:]
}

// 与 GitHub Actions 一致，日志中的密钥值统一显示为 ***。
const redactedLogValue = "***"

type sensitiveLogPattern struct {
	regex       *regexp.Regexp
	replacement string
}

var sensitiveLogPatterns = []sensitiveLogPattern{
	{regex: regexp.MustCompile(`(?i)(authorization:\s*(?:bearer|basic)\s+)[^\s]+`), replacement: "${1}" + redactedLogValue},
	{regex: regexp.MustCompile(`(?i)(x-access-token:)[^@\s]+(@)`), replacement: "${1}" + redactedLogValue + "${2}"},
	{regex: regexp.MustCompile(`(?i)\b((?:password|token|secret|access_token|refresh_token)=)[^\s&]+`), replacement: "${1}" + redactedLogValue},
}

func redactSensitiveLogContent(content string, sensitiveValues []string) string {
	output := content
	for _, pattern := range sensitiveLogPatterns {
		output = pattern.regex.ReplaceAllString(output, pattern.replacement)
	}
	for _, value := range normalizedSensitiveLogValues(sensitiveValues) {
		output = strings.ReplaceAll(output, value, redactedLogValue)
	}
	return output
}

func normalizedSensitiveLogValues(values []string) []string {
	seen := map[string]bool{}
	output := make([]string, 0, len(values)*3)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if len(value) < 4 {
			return
		}
		for _, candidate := range []string{value, url.QueryEscape(value), url.PathEscape(value)} {
			if candidate == "" || seen[candidate] {
				continue
			}
			seen[candidate] = true
			output = append(output, candidate)
		}
	}
	for _, value := range values {
		add(value)
		// 日志按行写入，多行密钥（如私钥）需要逐行遮盖。
		for _, line := range strings.Split(value, "\n") {
			add(line)
		}
	}
	// 先替换更长的值，避免较短的密钥先命中后残留长密钥的其余部分。
	sort.SliceStable(output, func(i, j int) bool { return len(output[i]) > len(output[j]) })
	return output
}
