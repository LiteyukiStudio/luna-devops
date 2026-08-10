package api

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const redactedLogValue = "[REDACTED]"

type sensitiveLogPattern struct {
	regex       *regexp.Regexp
	replacement string
}

var sensitiveLogPatterns = []sensitiveLogPattern{
	{regex: regexp.MustCompile(`(?i)(authorization:\s*(?:bearer|basic)\s+)[^\s]+`), replacement: "${1}" + redactedLogValue},
	{regex: regexp.MustCompile(`(?i)(x-access-token:)[^@\s]+(@)`), replacement: "${1}" + redactedLogValue + "${2}"},
	{regex: regexp.MustCompile(`(?i)\b((?:password|token|secret|access_token|refresh_token)=)[^\s&]+`), replacement: "${1}" + redactedLogValue},
}

func (h *Handlers) redactBuildJobLogContent(job model.BuildJob, content string, ctx context.Context) string {
	return redactSensitiveLogContent(content, h.sensitiveValuesForBuildJob(job, ctx))
}

func (h *Handlers) redactHookRunLogContent(run model.HookRun, content string, ctx context.Context) string {
	if strings.TrimSpace(run.BuildJobID) == "" {
		return redactSensitiveLogContent(content, nil)
	}
	var job model.BuildJob
	if err := h.dbWithContext(ctx).First(&job, "id = ? and project_id = ?", run.BuildJobID, run.ProjectID).Error; err != nil {
		return redactSensitiveLogContent(content, nil)
	}
	return h.redactBuildJobLogContent(job, content, ctx)
}

func (h *Handlers) sensitiveValuesForBuildJob(job model.BuildJob, ctx context.Context) []string {
	var run model.BuildRun
	if err := h.dbWithContext(ctx).First(&run, "id = ? and project_id = ?", job.BuildRunID, job.ProjectID).Error; err != nil {
		return nil
	}
	return h.sensitiveValuesForBuildRun(h.dbWithContext(ctx), run, ctx)
}

func (h *Handlers) sensitiveValuesForBuildRun(db *gorm.DB, run model.BuildRun, ctx context.Context) []string {
	values := make([]string, 0, 8)
	values = append(values, h.gitSensitiveValuesForBuildRun(db, run)...)
	values = append(values, h.registrySensitiveValuesForBuildRun(run, ctx)...)
	values = append(values, h.buildVariableSensitiveValuesForRun(db, run, ctx)...)
	return values
}

func (h *Handlers) gitSensitiveValuesForBuildRun(db *gorm.DB, run model.BuildRun) []string {
	var binding model.RepositoryBinding
	bindingQuery := db.Where("project_id = ? and application_id = ?", run.ProjectID, run.ApplicationID)
	if strings.TrimSpace(run.DeploymentTargetID) != "" {
		var target model.DeploymentTarget
		if err := db.First(&target, "id = ? and project_id = ? and application_id = ?", run.DeploymentTargetID, run.ProjectID, run.ApplicationID).Error; err == nil && strings.TrimSpace(target.RepositoryBindingID) != "" {
			bindingQuery = bindingQuery.Where("id = ?", target.RepositoryBindingID)
		}
	}
	if err := bindingQuery.First(&binding).Error; err != nil {
		return nil
	}

	var values []string
	if value := h.secrets.ResolveContext(db.Statement.Context, binding.CredentialRef); strings.TrimSpace(value) != "" {
		values = append(values, value)
	}
	var account model.GitAccount
	if err := db.First(&account, "id = ?", binding.GitAccountID).Error; err != nil {
		return values
	}
	if value := h.secrets.ResolveContext(db.Statement.Context, account.AccessTokenRef); strings.TrimSpace(value) != "" {
		values = append(values, value)
	}
	if value := h.secrets.ResolveContext(db.Statement.Context, account.RefreshTokenRef); strings.TrimSpace(value) != "" {
		values = append(values, value)
	}
	return values
}

func (h *Handlers) registrySensitiveValuesForBuildRun(run model.BuildRun, ctx context.Context) []string {
	if strings.TrimSpace(run.TargetRegistryID) == "" {
		return nil
	}
	var registry model.ArtifactRegistry
	if err := h.dbWithContext(ctx).First(&registry, "id = ?", run.TargetRegistryID).Error; err != nil {
		return nil
	}
	var actor model.User
	if err := h.dbWithContext(ctx).First(&actor, "id = ?", run.CreatedBy).Error; err != nil {
		return nil
	}
	credential, ok := h.registryCredentialFor(actor, registry, ctx)
	if !ok {
		return nil
	}
	return []string{
		h.secrets.ResolveContext(ctx, credential.TokenRef),
		h.secrets.ResolveContext(ctx, credential.PasswordRef),
	}
}

func (h *Handlers) buildVariableSensitiveValuesForRun(db *gorm.DB, run model.BuildRun, ctx context.Context) []string {
	var actor model.User
	if err := db.First(&actor, "id = ?", run.CreatedBy).Error; err != nil {
		return nil
	}
	sets, err := h.buildVariableSetsForRun(db, actor, run.ProjectID, buildVariableSetIDs(run.BuildVariableSetIDs), ctx)
	if err != nil {
		return nil
	}
	values := make([]string, 0, len(sets))
	for _, set := range sets {
		for _, ref := range decodeSecretRefs(set.SecretRefs) {
			if value := h.secrets.ResolveContext(db.Statement.Context, ref); strings.TrimSpace(value) != "" {
				values = append(values, value)
			}
		}
	}
	return values
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
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len(value) < 4 {
			continue
		}
		for _, candidate := range []string{value, url.QueryEscape(value), url.PathEscape(value)} {
			if candidate == "" || seen[candidate] {
				continue
			}
			seen[candidate] = true
			output = append(output, candidate)
		}
	}
	return output
}
