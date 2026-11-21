package registryprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/security"
)

const registrySearchTimeout = 8 * time.Second

type RepositorySearchResult struct {
	Items   []RepositoryItem `json:"items"`
	Total   int              `json:"total"`
	Limited bool             `json:"limited"`
}

type RepositoryItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
}

type TagSearchResult struct {
	Items   []TagItem `json:"items"`
	Total   int       `json:"total"`
	Limited bool      `json:"limited"`
}

type TagItem struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

func SearchRepositories(parent context.Context, provider, endpointText, namespace, query string, page, pageSize int, policy security.EgressPolicy, credential Credential) (RepositorySearchResult, error) {
	page, pageSize = normalizeSearchPage(page, pageSize)
	switch strings.TrimSpace(provider) {
	case "dockerhub":
		return searchDockerHubRepositories(parent, namespace, query, page, pageSize, policy)
	case "harbor":
		result, err := searchHarborRepositories(parent, endpointText, query, page, pageSize, policy, credential)
		if err == nil {
			return result, nil
		}
		return searchCatalogRepositories(parent, endpointText, namespace, query, page, pageSize, policy, credential)
	default:
		return searchCatalogRepositories(parent, endpointText, namespace, query, page, pageSize, policy, credential)
	}
}

func ListTags(parent context.Context, provider, endpointText, repository string, limit int, policy security.EgressPolicy, credential Credential) (TagSearchResult, error) {
	cleanedRepository, err := validateSafeRegistryRepository(repository)
	if err != nil {
		return TagSearchResult{}, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	result, err := listProviderTags(parent, provider, endpointText, cleanedRepository, limit, policy, credential)
	if err != nil {
		return result, err
	}
	return withoutBuildCacheTag(result), nil
}

func listProviderTags(parent context.Context, provider, endpointText, repository string, limit int, policy security.EgressPolicy, credential Credential) (TagSearchResult, error) {
	switch strings.TrimSpace(provider) {
	case "dockerhub":
		return listDockerHubTags(parent, repository, limit, policy, credential)
	case "harbor":
		result, err := listHarborTags(parent, endpointText, repository, limit, policy, credential)
		if err == nil {
			return result, nil
		}
		return listRegistryTags(parent, endpointText, repository, limit, policy, credential)
	default:
		return listRegistryTags(parent, endpointText, repository, limit, policy, credential)
	}
}

// withoutBuildCacheTag 去掉平台 BuildKit 缓存占用的保留 Tag：它指向缓存清单而不是可运行镜像，不能被选为部署镜像。
func withoutBuildCacheTag(result TagSearchResult) TagSearchResult {
	items := make([]TagItem, 0, len(result.Items))
	for _, item := range result.Items {
		if strings.TrimSpace(item.Name) == builder.RegistryCacheTag {
			if result.Total > 0 {
				result.Total--
			}
			continue
		}
		items = append(items, item)
	}
	result.Items = items
	return result
}

func searchDockerHubRepositories(parent context.Context, namespace, query string, page, pageSize int, policy security.EgressPolicy) (RepositorySearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		query = strings.TrimSpace(namespace)
	}
	values := url.Values{}
	values.Set("page", strconv.Itoa(page))
	values.Set("page_size", strconv.Itoa(pageSize))
	if query != "" {
		values.Set("query", query)
	}
	var response dockerHubSearchResponse
	if err := getRegistryJSON(parent, "https://hub.docker.com/v2/search/repositories/?"+values.Encode(), policy, Credential{}, &response); err != nil {
		return RepositorySearchResult{}, err
	}
	items := make([]RepositoryItem, 0, len(response.Results))
	for _, item := range response.Results {
		name := strings.TrimSpace(item.RepoName)
		if name == "" {
			name = strings.Trim(strings.TrimSpace(item.Namespace)+"/"+strings.TrimSpace(item.Name), "/")
		}
		items = append(items, RepositoryItem{Name: name, Description: item.Description, Private: item.IsPrivate})
	}
	return RepositorySearchResult{Items: items, Total: response.Count, Limited: len(items) >= pageSize && (response.Count == 0 || page*pageSize < response.Count)}, nil
}

func searchHarborRepositories(parent context.Context, endpointText, query string, page, pageSize int, policy security.EgressPolicy, credential Credential) (RepositorySearchResult, error) {
	endpoint, err := normalizeOCIEndpoint(endpointText)
	if err != nil {
		return RepositorySearchResult{}, err
	}
	endpoint.Path = path.Join(strings.TrimRight(endpoint.Path, "/"), "/api/v2.0/search")
	values := endpoint.Query()
	values.Set("q", strings.TrimSpace(query))
	endpoint.RawQuery = values.Encode()
	var response harborSearchResponse
	if err := getRegistryJSON(parent, endpoint.String(), policy, credential, &response); err != nil {
		return RepositorySearchResult{}, err
	}
	items := make([]RepositoryItem, 0, pageSize)
	for _, item := range response.Repository {
		if len(items) >= pageSize {
			break
		}
		name := strings.TrimSpace(item.RepositoryName)
		if name == "" {
			name = strings.TrimSpace(item.ProjectName)
		}
		if name == "" || !strings.Contains(strings.ToLower(name), strings.ToLower(strings.TrimSpace(query))) {
			continue
		}
		items = append(items, RepositoryItem{Name: name, Private: item.Private})
	}
	return RepositorySearchResult{Items: items, Total: len(response.Repository), Limited: len(response.Repository) > len(items)}, nil
}

func searchCatalogRepositories(parent context.Context, endpointText, namespace, query string, page, pageSize int, policy security.EgressPolicy, credential Credential) (RepositorySearchResult, error) {
	endpoint, err := normalizeOCIEndpoint(endpointText)
	if err != nil {
		return RepositorySearchResult{}, err
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v2/_catalog"
	values := endpoint.Query()
	values.Set("n", strconv.Itoa(pageSize*page))
	endpoint.RawQuery = values.Encode()
	var response catalogResponse
	if err := getRegistryJSON(parent, endpoint.String(), policy, credential, &response); err != nil {
		return RepositorySearchResult{}, err
	}
	search := strings.ToLower(strings.TrimSpace(query))
	prefix := strings.Trim(strings.TrimSpace(namespace), "/")
	filtered := make([]RepositoryItem, 0, pageSize)
	matched := 0
	start := (page - 1) * pageSize
	for _, repository := range response.Repositories {
		if prefix != "" && !strings.HasPrefix(repository, prefix+"/") && repository != prefix {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(repository), search) {
			continue
		}
		if matched >= start && len(filtered) < pageSize {
			filtered = append(filtered, RepositoryItem{Name: repository})
		}
		matched++
	}
	return RepositorySearchResult{Items: filtered, Total: matched, Limited: matched > start+len(filtered)}, nil
}

func listDockerHubTags(parent context.Context, repository string, limit int, policy security.EgressPolicy, credential Credential) (TagSearchResult, error) {
	repository = normalizeDockerHubRepository(repository)
	requestURL := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags?page_size=%d", PathEscapePath(repository), limit)
	var response dockerHubTagsResponse
	if err := getRegistryJSON(parent, requestURL, policy, Credential{}, &response); err != nil {
		if credential.Secret != "" {
			return listRegistryTags(parent, "https://registry-1.docker.io", repository, limit, policy, credential)
		}
		return TagSearchResult{}, err
	}
	items := make([]TagItem, 0, len(response.Results))
	for _, tag := range response.Results {
		digest := ""
		if len(tag.Images) > 0 {
			digest = tag.Images[0].Digest
		}
		items = append(items, TagItem{Name: tag.Name, Digest: digest})
	}
	return TagSearchResult{Items: items, Total: response.Count, Limited: len(items) >= limit && response.Count > len(items)}, nil
}

func listHarborTags(parent context.Context, endpointText, repository string, limit int, policy security.EgressPolicy, credential Credential) (TagSearchResult, error) {
	cleaned, err := validateSafeRegistryRepository(repository)
	if err != nil {
		return TagSearchResult{}, err
	}
	project, repoPath, ok := strings.Cut(cleaned, "/")
	if !ok || project == "" || repoPath == "" {
		return TagSearchResult{}, fmt.Errorf("harbor repository must include project/name")
	}
	endpoint, err := normalizeOCIEndpoint(endpointText)
	if err != nil {
		return TagSearchResult{}, err
	}
	basePath := strings.TrimRight(endpoint.EscapedPath(), "/")
	encodedRepo := strings.ReplaceAll(url.PathEscape(repoPath), "%2F", "%252F")
	values := endpoint.Query()
	values.Set("page_size", strconv.Itoa(limit))
	values.Set("with_tag", "true")
	requestURL := fmt.Sprintf("%s://%s%s/api/v2.0/projects/%s/repositories/%s/artifacts?%s",
		endpoint.Scheme, endpoint.Host, basePath, url.PathEscape(project), encodedRepo, values.Encode())
	var artifacts []harborArtifactResponse
	if err := getRegistryJSON(parent, requestURL, policy, credential, &artifacts); err != nil {
		return TagSearchResult{}, err
	}
	items := make([]TagItem, 0, limit)
	for _, artifact := range artifacts {
		for _, tag := range artifact.Tags {
			if len(items) >= limit {
				break
			}
			items = append(items, TagItem{Name: tag.Name, Digest: artifact.Digest})
		}
	}
	return TagSearchResult{Items: items, Total: len(items), Limited: len(items) >= limit}, nil
}

func listRegistryTags(parent context.Context, endpointText, repository string, limit int, policy security.EgressPolicy, credential Credential) (TagSearchResult, error) {
	cleaned, err := validateSafeRegistryRepository(repository)
	if err != nil {
		return TagSearchResult{}, err
	}
	endpoint, err := normalizeOCIEndpoint(endpointText)
	if err != nil {
		return TagSearchResult{}, err
	}
	basePath := strings.TrimRight(endpoint.EscapedPath(), "/")
	values := endpoint.Query()
	values.Set("n", strconv.Itoa(limit))
	requestURL := fmt.Sprintf("%s://%s%s/v2/%s/tags/list?%s",
		endpoint.Scheme, endpoint.Host, basePath, PathEscapePath(cleaned), values.Encode())
	var response tagsResponse
	if err := getRegistryJSON(parent, requestURL, policy, credential, &response); err != nil {
		return TagSearchResult{}, err
	}
	items := make([]TagItem, 0, len(response.Tags))
	for _, tag := range response.Tags {
		items = append(items, TagItem{Name: tag})
	}
	return TagSearchResult{Items: items, Total: len(response.Tags), Limited: len(response.Tags) >= limit}, nil
}

func getRegistryJSON(parent context.Context, requestURL string, policy security.EgressPolicy, credential Credential, output any) error {
	resp, err := doRegistryGET(parent, requestURL, "application/json", policy, credential, registrySearchTimeout)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &UpstreamError{
			StatusCode: resp.StatusCode,
			Message:    http.StatusText(resp.StatusCode),
		}
	}
	if output == nil || len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, output)
}

func normalizeSearchPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 20 {
		pageSize = 20
	}
	return page, pageSize
}

func normalizeDockerHubRepository(repository string) string {
	repository = strings.Trim(repository, "/")
	if !strings.Contains(repository, "/") {
		return "library/" + repository
	}
	return repository
}

type dockerHubSearchResponse struct {
	Count   int `json:"count"`
	Results []struct {
		RepoName    string `json:"repo_name"`
		Name        string `json:"name"`
		Namespace   string `json:"namespace"`
		Description string `json:"short_description"`
		IsPrivate   bool   `json:"is_private"`
	} `json:"results"`
}

type dockerHubTagsResponse struct {
	Count   int `json:"count"`
	Results []struct {
		Name   string `json:"name"`
		Images []struct {
			Digest string `json:"digest"`
		} `json:"images"`
	} `json:"results"`
}

type harborSearchResponse struct {
	Repository []struct {
		ProjectName    string `json:"project_name"`
		RepositoryName string `json:"repository_name"`
		Private        bool   `json:"private"`
	} `json:"repository"`
}

type harborArtifactResponse struct {
	Digest string `json:"digest"`
	Tags   []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

type catalogResponse struct {
	Repositories []string `json:"repositories"`
}

type tagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func validateSafeRegistryRepository(value string) (string, error) {
	trimmed := strings.Trim(strings.TrimSpace(value), "/")
	if trimmed == "" {
		return "", fmt.Errorf("registry repository is required")
	}
	if strings.Contains(trimmed, "\\") || strings.ContainsAny(trimmed, " \t\r\n?#") {
		return "", fmt.Errorf("invalid registry repository %q", value)
	}
	parts := strings.Split(trimmed, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid registry repository %q", value)
		}
	}
	return trimmed, nil
}

func PathEscapePath(value string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
