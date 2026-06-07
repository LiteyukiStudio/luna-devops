package api

import (
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
)

func registryResponses(registries []model.ArtifactRegistry) []artifactRegistryOutput {
	result := make([]artifactRegistryOutput, 0, len(registries))
	for _, registry := range registries {
		result = append(result, registryResponse(registry))
	}
	return result
}

func (h *Handlers) registryResponsesForUser(user model.User, registries []model.ArtifactRegistry) []artifactRegistryOutput {
	result := make([]artifactRegistryOutput, 0, len(registries))
	for _, registry := range registries {
		response := registryResponse(registry)
		if !h.canInspectScopedResourceConfig(user, registry.Scope, registry.OwnerRef) {
			response.Endpoint = ""
			response.Namespace = ""
			response.Capabilities = []string{}
		}
		result = append(result, response)
	}
	return result
}

func registryResponse(registry model.ArtifactRegistry) artifactRegistryOutput {
	return artifactRegistryOutput{
		ID:            registry.ID,
		Name:          registry.Name,
		Provider:      registry.Provider,
		Endpoint:      registry.Endpoint,
		Namespace:     registry.Namespace,
		Scope:         registry.Scope,
		OwnerRef:      registry.OwnerRef,
		CredentialSet: registry.CredentialRef != "",
		IsDefault:     registry.IsDefault,
		Capabilities:  jsonList(splitCSV(registry.Capabilities)),
		CreatedBy:     registry.CreatedBy,
		CreatedAt:     registry.CreatedAt,
	}
}

func credentialResponses(credentials []model.RegistryCredential) []registryCredentialOutput {
	result := make([]registryCredentialOutput, 0, len(credentials))
	for _, credential := range credentials {
		result = append(result, credentialResponse(credential))
	}
	return result
}

func credentialResponse(credential model.RegistryCredential) registryCredentialOutput {
	return registryCredentialOutput{
		ID:          credential.ID,
		RegistryID:  credential.RegistryID,
		Name:        credential.Name,
		Username:    credential.Username,
		Scope:       credential.Scope,
		AccessScope: credential.AccessScope,
		PasswordSet: secret.HasValue(credential.PasswordRef),
		TokenSet:    secret.HasValue(credential.TokenRef),
		CreatedAt:   credential.CreatedAt,
	}
}

func normalizeRegistryProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "dockerhub", "gitea-registry":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "harbor"
	}
}

func normalizeRegistryScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "project", "user":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "global"
	}
}

func normalizeCredentialScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "push", "pull":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "push-pull"
	}
}

func normalizeCredentialAccessScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "registry":
		return "registry"
	default:
		return "personal"
	}
}

func normalizeImageSourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "build":
		return "build"
	default:
		return "manual-image"
	}
}

func normalizeScanStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending", "scanning", "passed", "failed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func imageReference(registry model.ArtifactRegistry, repository, tag, digest string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(strings.TrimRight(registry.Endpoint, "/"), "https://"), "http://")
	base := strings.Join([]string{host, strings.Trim(repository, "/")}, "/")
	if digest != "" {
		return base + "@" + digest
	}
	return base + ":" + fallback(tag, "latest")
}

type artifactRegistryInput struct {
	Name         string   `json:"name" binding:"required"`
	Provider     string   `json:"provider"`
	Endpoint     string   `json:"endpoint" binding:"required"`
	Namespace    string   `json:"namespace"`
	Scope        string   `json:"scope"`
	OwnerRef     string   `json:"ownerRef"`
	IsDefault    bool     `json:"isDefault"`
	Capabilities []string `json:"capabilities"`
}

type artifactRegistryOutput struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Provider      string    `json:"provider"`
	Endpoint      string    `json:"endpoint"`
	Namespace     string    `json:"namespace"`
	Scope         string    `json:"scope"`
	OwnerRef      string    `json:"ownerRef"`
	CredentialSet bool      `json:"credentialSet"`
	IsDefault     bool      `json:"isDefault"`
	Capabilities  []string  `json:"capabilities"`
	CreatedBy     string    `json:"createdBy"`
	CreatedAt     time.Time `json:"createdAt"`
}

type registryCredentialInput struct {
	Name        string `json:"name"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Token       string `json:"token"`
	Scope       string `json:"scope"`
	AccessScope string `json:"accessScope"`
}

type registryCredentialOutput struct {
	ID          string    `json:"id"`
	RegistryID  string    `json:"registryId"`
	Name        string    `json:"name"`
	Username    string    `json:"username"`
	Scope       string    `json:"scope"`
	AccessScope string    `json:"accessScope"`
	PasswordSet bool      `json:"passwordSet"`
	TokenSet    bool      `json:"tokenSet"`
	CreatedAt   time.Time `json:"createdAt"`
}

type containerImageInput struct {
	ProjectID     string `json:"projectId"`
	ApplicationID string `json:"applicationId"`
	RegistryID    string `json:"registryId" binding:"required"`
	Repository    string `json:"repository" binding:"required"`
	Tag           string `json:"tag"`
	Digest        string `json:"digest"`
	SourceCommit  string `json:"sourceCommit"`
	BuildRunID    string `json:"buildRunId"`
	SourceType    string `json:"sourceType"`
	ScanStatus    string `json:"scanStatus"`
}

type registryTestResult struct {
	Success    bool   `json:"success"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	Endpoint   string `json:"endpoint"`
}
