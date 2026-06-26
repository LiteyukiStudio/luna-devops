package gitprovider

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const gitBranchPageSize = 100
const gitBranchMaxPages = 20

func (c Client) ListRepositories(ctx context.Context, search string, page, pageSize int) ([]Repository, error) {
	switch c.provider.Type {
	case "github":
		var repos []githubRepositoryResponse
		err := c.getJSON(ctx, c.apiURL("/user/repos", map[string]string{
			"affiliation": "owner,collaborator,organization_member",
			"sort":        "updated",
			"page":        strconv.Itoa(page),
			"per_page":    strconv.Itoa(pageSize),
		}), &repos)
		return FilterRepositories(withRepositorySource(githubRepositories(repos), "accessible"), search), err
	case "gitea":
		var repos []giteaRepositoryResponse
		err := c.getJSON(ctx, c.apiURL("/user/repos", map[string]string{
			"page":  strconv.Itoa(page),
			"limit": strconv.Itoa(pageSize),
		}), &repos)
		return FilterRepositories(withRepositorySource(giteaRepositories(repos), "accessible"), search), err
	default:
		return nil, fmt.Errorf("git provider type %q is not supported", c.provider.Type)
	}
}

func (c Client) SearchPublicRepositories(ctx context.Context, search string, page, pageSize int) ([]Repository, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil, nil
	}
	switch c.provider.Type {
	case "github":
		var response githubRepositorySearchResponse
		err := c.getJSON(ctx, c.apiURL("/search/repositories", map[string]string{
			"q":        search,
			"sort":     "updated",
			"order":    "desc",
			"page":     strconv.Itoa(page),
			"per_page": strconv.Itoa(pageSize),
		}), &response)
		return withRepositorySource(githubRepositories(response.Items), "public"), err
	case "gitea":
		var response giteaRepositorySearchResponse
		err := c.getJSON(ctx, c.apiURL("/repos/search", map[string]string{
			"q":     search,
			"page":  strconv.Itoa(page),
			"limit": strconv.Itoa(pageSize),
		}), &response)
		return withRepositorySource(giteaRepositories(response.Data), "public"), err
	default:
		return nil, fmt.Errorf("git provider type %q is not supported", c.provider.Type)
	}
}

func (c Client) ListBranches(ctx context.Context, owner, repo string) ([]Branch, error) {
	output := make([]Branch, 0)
	for page := 1; page <= gitBranchMaxPages; page++ {
		params := map[string]string{"page": strconv.Itoa(page)}
		switch c.provider.Type {
		case "github":
			params["per_page"] = strconv.Itoa(gitBranchPageSize)
		case "gitea":
			params["limit"] = strconv.Itoa(gitBranchPageSize)
		default:
			return nil, fmt.Errorf("git provider type %q is not supported", c.provider.Type)
		}

		var branches []gitBranchResponse
		err := c.getJSON(ctx, c.apiURL(fmt.Sprintf("/repos/%s/%s/branches", pathEscape(owner), pathEscape(repo)), params), &branches)
		if err != nil {
			return nil, err
		}
		for _, branch := range branches {
			output = append(output, Branch{Name: branch.Name, SHA: branch.Commit.SHA})
		}
		if len(branches) < gitBranchPageSize {
			break
		}
	}
	return output, nil
}

func (c Client) GetBranch(ctx context.Context, owner, repo, branchName string) (Branch, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return Branch{}, fmt.Errorf("branch name is required")
	}
	var branch gitBranchResponse
	if err := c.getJSON(ctx, c.apiURL(fmt.Sprintf("/repos/%s/%s/branches/%s", pathEscape(owner), pathEscape(repo), pathEscape(branchName)), nil), &branch); err != nil {
		return Branch{}, err
	}
	return Branch{Name: branch.Name, SHA: branch.Commit.SHA}, nil
}

func (c Client) ReadFile(ctx context.Context, owner, repo, filePath, ref string) (FileContent, error) {
	filePath = strings.TrimLeft(strings.TrimSpace(filePath), "/")
	if filePath == "" {
		return FileContent{}, fmt.Errorf("file path is required")
	}
	params := map[string]string{}
	if strings.TrimSpace(ref) != "" {
		params["ref"] = strings.TrimSpace(ref)
	}
	var file gitContentResponse
	err := c.getJSON(ctx, c.apiURL(fmt.Sprintf("/repos/%s/%s/contents/%s", pathEscape(owner), pathEscape(repo), PathEscapePath(filePath)), params), &file)
	if err != nil {
		return FileContent{}, err
	}
	content := strings.TrimSpace(file.Content)
	if strings.EqualFold(file.Encoding, "base64") {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
		if err != nil {
			return FileContent{}, err
		}
		content = string(decoded)
	}
	return FileContent{
		Path:     file.Path,
		Name:     file.Name,
		Ref:      strings.TrimSpace(ref),
		SHA:      file.SHA,
		Content:  content,
		Encoding: "utf-8",
	}, nil
}

func (c Client) ListContents(ctx context.Context, owner, repo, dirPath, ref string) ([]ContentItem, error) {
	dirPath = strings.Trim(strings.TrimSpace(dirPath), "/")
	params := map[string]string{}
	if strings.TrimSpace(ref) != "" {
		params["ref"] = strings.TrimSpace(ref)
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/contents", pathEscape(owner), pathEscape(repo))
	if dirPath != "" {
		apiPath += "/" + PathEscapePath(dirPath)
	}
	var items []gitContentResponse
	if err := c.getJSON(ctx, c.apiURL(apiPath, params), &items); err != nil {
		return nil, err
	}
	output := make([]ContentItem, 0, len(items))
	for _, item := range items {
		output = append(output, ContentItem{
			Path: item.Path,
			Name: item.Name,
			Type: normalizeContentType(item.Type),
			SHA:  item.SHA,
		})
	}
	return output, nil
}

func (c Client) CreateWebhook(ctx context.Context, owner, repo, callbackURL, secret string) (WebhookCreateResult, error) {
	switch c.provider.Type {
	case "github":
		payload := map[string]any{
			"name":   "web",
			"active": true,
			"events": []string{"push", "create"},
			"config": map[string]any{
				"url":          callbackURL,
				"content_type": "json",
				"secret":       secret,
				"insecure_ssl": "0",
			},
		}
		var response githubWebhookResponse
		if err := c.postJSON(ctx, c.apiURL(fmt.Sprintf("/repos/%s/%s/hooks", pathEscape(owner), pathEscape(repo)), nil), payload, &response); err != nil {
			return WebhookCreateResult{}, err
		}
		return WebhookCreateResult{ID: strconv.FormatInt(response.ID, 10), URL: response.Config.URL, Secret: secret}, nil
	case "gitea":
		payload := map[string]any{
			"type":   "gitea",
			"active": true,
			"events": []string{"push", "create"},
			"config": map[string]any{
				"url":          callbackURL,
				"content_type": "json",
				"secret":       secret,
			},
		}
		var response giteaWebhookResponse
		if err := c.postJSON(ctx, c.apiURL(fmt.Sprintf("/repos/%s/%s/hooks", pathEscape(owner), pathEscape(repo)), nil), payload, &response); err != nil {
			return WebhookCreateResult{}, err
		}
		return WebhookCreateResult{ID: strconv.FormatInt(response.ID, 10), URL: callbackURL, Secret: secret}, nil
	default:
		return WebhookCreateResult{}, fmt.Errorf("git provider type %q is not supported", c.provider.Type)
	}
}

func FilterRepositories(repos []Repository, search string) []Repository {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return repos
	}
	filtered := make([]Repository, 0, len(repos))
	for _, repo := range repos {
		if strings.Contains(strings.ToLower(repo.FullName), search) || strings.Contains(strings.ToLower(repo.Name), search) {
			filtered = append(filtered, repo)
		}
	}
	return filtered
}

func githubRepositories(repos []githubRepositoryResponse) []Repository {
	output := make([]Repository, 0, len(repos))
	for _, repo := range repos {
		output = append(output, Repository{
			Owner:         repo.Owner.Login,
			Name:          repo.Name,
			FullName:      repo.FullName,
			CloneURL:      repo.CloneURL,
			DefaultBranch: repo.DefaultBranch,
			Private:       repo.Private,
		})
	}
	return output
}

func giteaRepositories(repos []giteaRepositoryResponse) []Repository {
	output := make([]Repository, 0, len(repos))
	for _, repo := range repos {
		output = append(output, Repository{
			Owner:         repo.Owner.UserName,
			Name:          repo.Name,
			FullName:      repo.FullName,
			CloneURL:      repo.CloneURL,
			DefaultBranch: repo.DefaultBranch,
			Private:       repo.Private,
		})
	}
	return output
}

func withRepositorySource(repos []Repository, source string) []Repository {
	for index := range repos {
		repos[index].Source = source
	}
	return repos
}

func pathEscape(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}

func PathEscapePath(value string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	for index, part := range parts {
		parts[index] = pathEscape(part)
	}
	return strings.Join(parts, "/")
}
