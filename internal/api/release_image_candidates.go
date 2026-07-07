package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/gin-gonic/gin"
)

const releaseImageCandidateLimit = 30

type releaseImageCandidateOutput struct {
	Key          string `json:"key"`
	Source       string `json:"source"`
	Label        string `json:"label"`
	ImageRef     string `json:"imageRef"`
	BuildRunID   string `json:"buildRunId"`
	Tag          string `json:"tag"`
	Digest       string `json:"digest"`
	SourceCommit string `json:"sourceCommit"`
	CreatedAt    string `json:"createdAt"`
}

type releaseImageCandidatesOutput struct {
	Items             []releaseImageCandidateOutput `json:"items"`
	RegistryAvailable bool                          `json:"registryAvailable"`
	RegistryError     string                        `json:"registryError"`
	FallbackUsed      bool                          `json:"fallbackUsed"`
}

func (h *Handlers) ListReleaseImageCandidates(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUser(ctx)
	if !ok {
		return
	}
	applicationID := strings.TrimSpace(ctx.Param("applicationId"))
	targetID := strings.TrimSpace(ctx.Param("targetId"))
	var app model.Application
	if err := h.db.First(&app, "id = ? and project_id = ?", applicationID, project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "应用不存在或不属于当前项目空间")
		return
	}
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ? and application_id = ?", targetID, project.ID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "部署配置不存在或不属于当前应用")
		return
	}

	response := releaseImageCandidatesOutput{Items: []releaseImageCandidateOutput{}}
	seen := map[string]bool{}
	if target.SourceType == "repository" {
		h.appendRegistryReleaseImageCandidates(ctx, user, target, &response, seen)
	}
	h.appendBuildRunReleaseImageCandidates(target, &response, seen)
	if target.SourceType == "image" && strings.TrimSpace(target.ImageRef) != "" {
		appendReleaseImageCandidate(&response.Items, seen, releaseImageCandidateOutput{
			Key:      "image:" + strings.TrimSpace(target.ImageRef),
			Source:   "target",
			Label:    strings.TrimSpace(target.ImageRef),
			ImageRef: strings.TrimSpace(target.ImageRef),
		})
	}
	response.FallbackUsed = !response.RegistryAvailable && len(response.Items) > 0
	ctx.JSON(http.StatusOK, response)
}

func (h *Handlers) appendRegistryReleaseImageCandidates(ctx *gin.Context, user model.User, target model.DeploymentTarget, response *releaseImageCandidatesOutput, seen map[string]bool) {
	registryID := strings.TrimSpace(target.TargetRegistryID)
	repository := strings.Trim(strings.TrimSpace(target.TargetRepository), "/")
	if registryID == "" || repository == "" {
		response.RegistryError = "registry_or_repository_missing"
		return
	}
	var registry model.ArtifactRegistry
	if err := h.db.First(&registry, "id = ?", registryID).Error; err != nil {
		response.RegistryError = "registry_not_found"
		return
	}
	if !h.canUseScopedResourceByID(user, registry.Scope, registry.OwnerRef, scopedResourceArtifactRegistry, registry.ID) {
		response.RegistryError = "registry_forbidden"
		return
	}
	repository = repositoryWithoutRegistryHost(registry, repository)
	credential := h.registryCredentialInput(user, registry)
	result, err := registryprovider.ListTags(ctx.Request.Context(), registry.Provider, registry.Endpoint, repository, releaseImageCandidateLimit, h.egressPolicyForUser(user), credential)
	if err != nil {
		response.RegistryError = "registry_unavailable"
		return
	}
	prefix := buildImageNamePrefix(registry, repository)
	if prefix == "" {
		response.RegistryError = "repository_invalid"
		return
	}
	response.RegistryAvailable = true
	for _, tag := range result.Items {
		tagName := strings.TrimSpace(tag.Name)
		if tagName == "" {
			continue
		}
		imageRef := prefix + ":" + tagName
		label := imageRef
		digest := strings.TrimSpace(tag.Digest)
		if digest != "" {
			label = fmt.Sprintf("%s · %s", imageRef, shortDigestLabel(digest))
		}
		appendReleaseImageCandidate(&response.Items, seen, releaseImageCandidateOutput{
			Key:      "registry:" + imageRef,
			Source:   "registry",
			Label:    label,
			ImageRef: imageRef,
			Tag:      tagName,
			Digest:   digest,
		})
	}
}

func (h *Handlers) appendBuildRunReleaseImageCandidates(target model.DeploymentTarget, response *releaseImageCandidatesOutput, seen map[string]bool) {
	var runs []model.BuildRun
	if err := h.db.
		Where("project_id = ? and application_id = ? and deployment_target_id = ? and status = ?", target.ProjectID, target.ApplicationID, target.ID, "succeeded").
		Order("finished_at desc nulls last, created_at desc").
		Limit(releaseImageCandidateLimit).
		Find(&runs).Error; err != nil {
		return
	}
	for _, run := range runs {
		imageRef := strings.TrimSpace(run.ImageRef)
		if imageRef == "" {
			imageRef = strings.TrimSpace(run.TargetRepository)
			if imageRef != "" {
				imageRef = imageRef + ":" + fallback(strings.TrimSpace(run.TargetTag), "latest")
			}
		}
		if imageRef == "" {
			continue
		}
		label := buildRunReleaseCandidateLabel(run, imageRef)
		appendReleaseImageCandidate(&response.Items, seen, releaseImageCandidateOutput{
			Key:          "build:" + run.ID,
			Source:       "build",
			Label:        label,
			ImageRef:     imageRef,
			BuildRunID:   run.ID,
			Tag:          strings.TrimSpace(run.TargetTag),
			Digest:       strings.TrimSpace(run.ImageDigest),
			SourceCommit: strings.TrimSpace(run.SourceCommit),
			CreatedAt:    releaseCandidateTime(run).Format(time.RFC3339),
		})
	}
}

func appendReleaseImageCandidate(items *[]releaseImageCandidateOutput, seen map[string]bool, item releaseImageCandidateOutput) {
	imageRef := strings.TrimSpace(item.ImageRef)
	if imageRef == "" || seen[imageRef] {
		return
	}
	seen[imageRef] = true
	if strings.TrimSpace(item.Label) == "" {
		item.Label = imageRef
	}
	*items = append(*items, item)
}

func buildRunReleaseCandidateLabel(run model.BuildRun, imageRef string) string {
	parts := []string{}
	if branch := strings.TrimSpace(firstNonEmpty(run.SourceBranch, run.SourceTag)); branch != "" {
		parts = append(parts, branch)
	}
	parts = append(parts, imageRef)
	if digest := shortDigestLabel(run.ImageDigest); digest != "" {
		parts = append(parts, digest)
	}
	return strings.Join(parts, " · ")
}

func releaseCandidateTime(run model.BuildRun) time.Time {
	if run.FinishedAt != nil && !run.FinishedAt.IsZero() {
		return *run.FinishedAt
	}
	return run.CreatedAt
}

func shortDigestLabel(digest string) string {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return ""
	}
	if strings.HasPrefix(digest, "sha256:") && len(digest) > len("sha256:")+12 {
		return digest[:len("sha256:")+12]
	}
	if len(digest) > 18 {
		return digest[:18] + "..."
	}
	return digest
}
