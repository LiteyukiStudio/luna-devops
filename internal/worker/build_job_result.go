package worker

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func effectiveBuildTimeoutSeconds(runTimeoutSeconds int, fallbackSeconds int64) int64 {
	if runTimeoutSeconds > 0 {
		return int64(runTimeoutSeconds)
	}
	if fallbackSeconds > 0 {
		return fallbackSeconds
	}
	return defaultBuildTimeoutSeconds
}
func (r *Runner) buildKubernetesJobFailureMessage(ctx context.Context, client kubernetes.Interface, namespace string, jobName string, fallback string) string {
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
	if err != nil || len(pods.Items) == 0 {
		return fallback
	}
	messages := make([]string, 0, 4)
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		if message := buildPodFailureMessage(pod); message != "" {
			messages = append(messages, message)
		}
		if eventMessage := buildPodEventFailureMessage(ctx, client, namespace, pod.Name); eventMessage != "" {
			messages = append(messages, eventMessage)
		}
	}
	if len(messages) == 0 {
		return fallback
	}
	return fallback + ": " + strings.Join(messages, "; ")
}

func buildPodFailureMessage(pod corev1.Pod) string {
	parts := make([]string, 0, 4)
	if pod.Status.Phase != "" {
		parts = append(parts, "pod="+string(pod.Status.Phase))
	}
	if pod.Status.Reason != "" {
		parts = append(parts, "reason="+pod.Status.Reason)
	}
	if pod.Status.Message != "" {
		parts = append(parts, "message="+strings.TrimSpace(pod.Status.Message))
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != "executor" {
			continue
		}
		if status.State.Terminated != nil {
			terminated := status.State.Terminated
			containerParts := []string{fmt.Sprintf("executor terminated: reason=%s", firstNonEmpty(terminated.Reason, "Error")), fmt.Sprintf("exitCode=%d", terminated.ExitCode)}
			if terminated.Message != "" {
				containerParts = append(containerParts, "message="+strings.TrimSpace(terminated.Message))
			}
			parts = append(parts, strings.Join(containerParts, " "))
		}
		if status.State.Waiting != nil {
			waiting := status.State.Waiting
			containerParts := []string{"executor waiting: reason=" + firstNonEmpty(waiting.Reason, "Waiting")}
			if waiting.Message != "" {
				containerParts = append(containerParts, "message="+strings.TrimSpace(waiting.Message))
			}
			parts = append(parts, strings.Join(containerParts, " "))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "pod " + pod.Name + " " + strings.Join(parts, ", ")
}

func buildPodEventFailureMessage(ctx context.Context, client kubernetes.Interface, namespace string, podName string) string {
	events, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.name=" + podName})
	if err != nil || len(events.Items) == 0 {
		return ""
	}
	items := make([]corev1.Event, 0, len(events.Items))
	for _, event := range events.Items {
		if event.Type != corev1.EventTypeWarning && event.Reason != "Failed" && event.Reason != "BackOff" && event.Reason != "Evicted" {
			continue
		}
		items = append(items, event)
	}
	if len(items) == 0 {
		return ""
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := eventTime(items[i])
		right := eventTime(items[j])
		return left.Before(right)
	})
	latest := items[len(items)-1]
	message := strings.TrimSpace(latest.Message)
	if message == "" {
		message = latest.Reason
	}
	return "event " + firstNonEmpty(latest.Reason, "Warning") + ": " + message
}

func eventTime(event corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if !event.FirstTimestamp.IsZero() {
		return event.FirstTimestamp.Time
	}
	return event.CreationTimestamp.Time
}

var errBuildRunCanceled = errors.New("build run canceled")

func (r *Runner) buildRunCanceled(job model.BuildJob) (bool, error) {
	var run model.BuildRun
	if err := r.db.First(&run, "id = ? and project_id = ?", job.BuildRunID, job.ProjectID).Error; err != nil {
		return false, err
	}
	return run.Status == "canceled", nil
}

func (r *Runner) completeBuildJob(job model.BuildJob, run model.BuildRun, result builder.Result) (model.BuildRun, error) {
	finishedAt := time.Now()
	var completedRun model.BuildRun
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var lockedJob model.BuildJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedJob, "id = ? and project_id = ?", job.ID, job.ProjectID).Error; err != nil {
			return err
		}
		var lockedRun model.BuildRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRun, "id = ? and project_id = ?", run.ID, run.ProjectID).Error; err != nil {
			return err
		}
		if lockedJob.Status != "running" || lockedRun.Status == "canceled" {
			return nil
		}
		imageRef := firstNonEmpty(result.ImageRef, lockedRun.ImageRef)
		sourceCommit := firstNonEmpty(result.SourceCommit, lockedRun.SourceCommit)
		sourceAuthorName := firstNonEmpty(result.SourceAuthorName, lockedRun.SourceAuthorName)
		sourceAuthorEmail := firstNonEmpty(result.SourceAuthorEmail, lockedRun.SourceAuthorEmail)
		if err := tx.Model(&model.BuildJob{}).Where("id = ?", lockedJob.ID).Updates(map[string]any{
			"status":            "succeeded",
			"message":           firstNonEmpty(result.Message, "builder task succeeded"),
			"lease_token":       "",
			"lease_until":       nil,
			"last_heartbeat_at": &finishedAt,
			"finished_at":       &finishedAt,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.BuildRun{}).Where("id = ?", lockedRun.ID).Updates(map[string]any{
			"status":              "succeeded",
			"image_ref":           imageRef,
			"image_digest":        result.ImageDigest,
			"source_commit":       sourceCommit,
			"source_author_name":  sourceAuthorName,
			"source_author_email": sourceAuthorEmail,
			"finished_at":         &finishedAt,
		}).Error; err != nil {
			return err
		}
		lockedRun.Status = "succeeded"
		lockedRun.ImageRef = imageRef
		lockedRun.ImageDigest = result.ImageDigest
		lockedRun.SourceCommit = sourceCommit
		lockedRun.SourceAuthorName = sourceAuthorName
		lockedRun.SourceAuthorEmail = sourceAuthorEmail
		lockedRun.FinishedAt = &finishedAt
		if imageRef != "" {
			image := containerImageFromBuildRun(lockedRun, imageRef, result.ImageDigest, sourceCommit)
			if image.ID != "" {
				if err := tx.Create(&image).Error; err != nil {
					return err
				}
			}
		}
		completedRun = lockedRun
		return nil
	})
	if err == nil && completedRun.ID != "" {
		r.recordBuildRunMetrics(completedRun)
	}
	return completedRun, err
}

func (r *Runner) failBuildJob(job model.BuildJob, run model.BuildRun, message string) error {
	finishedAt := time.Now()
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.BuildJob{}).Where("id = ? and project_id = ? and status in ?", job.ID, job.ProjectID, []string{"queued", "running"}).Updates(map[string]any{
			"status":      "failed",
			"message":     firstNonEmpty(message, "builder task failed"),
			"lease_token": "",
			"lease_until": nil,
			"finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.BuildRun{}).Where("id = ? and project_id = ? and status in ?", run.ID, run.ProjectID, []string{"queued", "running"}).Updates(map[string]any{
			"status":      "failed",
			"finished_at": &finishedAt,
		}).Error
	})
	if err == nil {
		run.Status = "failed"
		run.FinishedAt = &finishedAt
		r.recordBuildRunMetrics(run)
	}
	return err
}
func (r *Runner) cleanupBuildJobSecrets(ctx context.Context, client kubernetes.Interface, namespace string, secretName string) error {
	err := client.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *Runner) deleteKubernetesBuildJob(ctx context.Context, client kubernetes.Interface, namespace string, jobName string) error {
	propagation := metav1.DeletePropagationBackground
	return client.BatchV1().Jobs(namespace).Delete(ctx, jobName, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

func buildKubernetesJobName(jobID string) string {
	return "build-" + strings.Trim(sanitizeKubernetesName(jobID), "-")
}

func sanitizeKubernetesName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	previousDash := false
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			out.WriteRune(char)
			previousDash = false
			continue
		}
		if !previousDash {
			out.WriteByte('-')
			previousDash = true
		}
	}
	result := strings.Trim(out.String(), "-")
	if result == "" {
		result = "job"
	}
	if len(result) > 57 {
		result = strings.Trim(result[:57], "-")
	}
	return result
}
func containerImageFromBuildRun(run model.BuildRun, imageRef string, digest string, sourceCommit string) model.ContainerImage {
	if strings.TrimSpace(run.TargetRegistryID) == "" || strings.TrimSpace(run.TargetRepository) == "" {
		return model.ContainerImage{}
	}
	return model.ContainerImage{
		ID:            id.New("img"),
		ProjectID:     run.ProjectID,
		ApplicationID: run.ApplicationID,
		RegistryID:    run.TargetRegistryID,
		Repository:    strings.Trim(strings.TrimSpace(run.TargetRepository), "/"),
		Tag:           firstNonEmpty(strings.TrimSpace(run.TargetTag), "latest"),
		Digest:        strings.TrimSpace(digest),
		ImageRef:      strings.TrimSpace(imageRef),
		SourceType:    "build",
		BuildRunID:    run.ID,
		SourceCommit:  strings.TrimSpace(sourceCommit),
		ScanStatus:    "unknown",
		CreatedBy:     run.CreatedBy,
	}
}
