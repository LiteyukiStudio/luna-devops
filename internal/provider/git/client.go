package gitprovider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
	"golang.org/x/oauth2"
)

const gitHTTPTimeout = 15 * time.Second
const gitBranchPageSize = 100
const gitBranchMaxPages = 20
const gitBuildOptionsMaxDirectories = 80
const gitBuildOptionsMaxDepth = 3

type Client struct {
	httpClient *http.Client
	provider   model.GitProvider
	token      string
	policy     security.EgressPolicy
}

type Repository struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	FullName      string `json:"fullName"`
	CloneURL      string `json:"cloneUrl"`
	DefaultBranch string `json:"defaultBranch"`
	Private       bool   `json:"private"`
	Source        string `json:"source"`
}

type Branch struct {
	Name string `json:"name"`
	SHA  string `json:"sha"`
}

type FileContent struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Ref      string `json:"ref"`
	SHA      string `json:"sha"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type ContentItem struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

type BuildOptions struct {
	Dockerfiles  []string         `json:"dockerfiles"`
	Directories  []string         `json:"directories"`
	ExposedPorts map[string][]int `json:"exposedPorts"`
	Strategy     string           `json:"strategy"`
	Truncated    bool             `json:"truncated"`
}

type WebhookCreateResult struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Secret string `json:"-"`
}

type UpstreamError struct {
	StatusCode       int
	Message          string
	DocumentationURL string
	Details          []UpstreamErrorDetail
}

type UpstreamErrorDetail struct {
	Resource string `json:"resource"`
	Code     string `json:"code"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "git api returned an upstream error"
	}
	parts := []string{fmt.Sprintf("git api returned %d", e.StatusCode)}
	if strings.TrimSpace(e.Message) != "" {
		parts = append(parts, strings.TrimSpace(e.Message))
	}
	for _, detail := range e.Details {
		detailParts := []string{}
		if strings.TrimSpace(detail.Resource) != "" {
			detailParts = append(detailParts, strings.TrimSpace(detail.Resource))
		}
		if strings.TrimSpace(detail.Field) != "" {
			detailParts = append(detailParts, strings.TrimSpace(detail.Field))
		}
		if strings.TrimSpace(detail.Code) != "" {
			detailParts = append(detailParts, strings.TrimSpace(detail.Code))
		}
		if len(detailParts) > 0 {
			parts = append(parts, strings.Join(detailParts, "."))
		}
	}
	return strings.Join(parts, ": ")
}

func AsUpstreamError(err error) (*UpstreamError, bool) {
	var upstreamErr *UpstreamError
	if errors.As(err, &upstreamErr) {
		return upstreamErr, true
	}
	return nil, false
}

func NewClient(provider model.GitProvider, token string) Client {
	return NewClientWithPolicy(provider, token, security.PublicEgressPolicy())
}

func NewClientWithPolicy(provider model.GitProvider, token string, policy security.EgressPolicy) Client {
	return Client{
		httpClient: security.NewHTTPClient(policy, gitHTTPTimeout),
		provider:   provider,
		token:      strings.TrimSpace(token),
		policy:     policy,
	}
}

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

func (c Client) DiscoverBuildOptions(ctx context.Context, owner, repo, ref string) (BuildOptions, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "main"
	}
	options, err := c.discoverBuildOptionsByTree(ctx, owner, repo, ref)
	if err == nil && !options.Truncated {
		c.populateDockerfileExposedPorts(ctx, owner, repo, ref, &options)
		return options, nil
	}
	fallbackOptions, fallbackErr := c.discoverBuildOptionsByContents(ctx, owner, repo, ref)
	if fallbackErr != nil {
		if err != nil {
			return BuildOptions{}, err
		}
		return BuildOptions{}, fallbackErr
	}
	if err == nil && options.Truncated {
		fallbackOptions.Truncated = true
	}
	c.populateDockerfileExposedPorts(ctx, owner, repo, ref, &fallbackOptions)
	return fallbackOptions, nil
}

func (c Client) populateDockerfileExposedPorts(ctx context.Context, owner, repo, ref string, options *BuildOptions) {
	if options == nil || len(options.Dockerfiles) == 0 {
		return
	}
	exposedPorts := make(map[string][]int)
	for _, dockerfile := range options.Dockerfiles {
		content, err := c.ReadFile(ctx, owner, repo, dockerfile, ref)
		if err != nil {
			continue
		}
		ports := parseDockerfileExposedPorts(content.Content)
		if len(ports) > 0 {
			exposedPorts[dockerfile] = ports
		}
	}
	if len(exposedPorts) > 0 {
		options.ExposedPorts = exposedPorts
	}
}

func (c Client) discoverBuildOptionsByTree(ctx context.Context, owner, repo, ref string) (BuildOptions, error) {
	branch, err := c.GetBranch(ctx, owner, repo, ref)
	if err != nil {
		return BuildOptions{}, err
	}
	params := map[string]string{}
	switch c.provider.Type {
	case "github":
		params["recursive"] = "1"
	case "gitea":
		params["recursive"] = "true"
	default:
		return BuildOptions{}, fmt.Errorf("git provider type %q is not supported", c.provider.Type)
	}
	var tree gitTreeResponse
	if err := c.getJSON(ctx, c.apiURL(fmt.Sprintf("/repos/%s/%s/git/trees/%s", pathEscape(owner), pathEscape(repo), pathEscape(branch.SHA)), params), &tree); err != nil {
		return BuildOptions{}, err
	}
	dockerfiles := map[string]struct{}{}
	directories := map[string]struct{}{".": {}}
	for _, item := range tree.Tree {
		path := strings.Trim(strings.TrimSpace(item.Path), "/")
		if path == "" {
			continue
		}
		switch normalizeContentType(item.Type) {
		case "dir":
			directories[path] = struct{}{}
		case "file":
			parts := strings.Split(path, "/")
			if isDockerfileName(parts[len(parts)-1]) {
				dockerfiles[path] = struct{}{}
			}
		}
	}
	return BuildOptions{
		Directories: sortedPathSet(directories),
		Dockerfiles: sortedPathSet(dockerfiles),
		Strategy:    "recursive-tree",
		Truncated:   tree.Truncated,
	}, nil
}

func (c Client) discoverBuildOptionsByContents(ctx context.Context, owner, repo, ref string) (BuildOptions, error) {
	dockerfiles := map[string]struct{}{}
	directories := map[string]struct{}{".": {}}
	queue := []struct {
		path  string
		depth int
	}{{path: "", depth: 0}}

	for index := 0; index < len(queue) && index < gitBuildOptionsMaxDirectories; index++ {
		current := queue[index]
		items, err := c.ListContents(ctx, owner, repo, current.path, ref)
		if err != nil {
			return BuildOptions{}, err
		}
		for _, item := range items {
			switch item.Type {
			case "dir":
				directories[item.Path] = struct{}{}
				if current.depth < gitBuildOptionsMaxDepth {
					queue = append(queue, struct {
						path  string
						depth int
					}{path: item.Path, depth: current.depth + 1})
				}
			case "file":
				if isDockerfileName(item.Name) {
					dockerfiles[item.Path] = struct{}{}
				}
			}
		}
	}
	return BuildOptions{
		Directories: sortedPathSet(directories),
		Dockerfiles: sortedPathSet(dockerfiles),
		Strategy:    "contents-bfs",
		Truncated:   len(queue) > gitBuildOptionsMaxDirectories,
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

func (c Client) getJSON(ctx context.Context, requestURL string, output any) error {
	if _, err := c.policy.ValidateURL(requestURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeGitResponse(resp, output)
}

func (c Client) postJSON(ctx context.Context, requestURL string, input, output any) error {
	if _, err := c.policy.ValidateURL(requestURL); err != nil {
		return err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeGitResponse(resp, output)
}

func (c Client) authorize(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	if c.provider.Type == "github" {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (c Client) apiURL(apiPath string, params map[string]string) string {
	base := strings.TrimRight(c.provider.BaseURL, "/")
	switch c.provider.Type {
	case "github":
		if base == "" || base == "https://github.com" {
			base = "https://api.github.com"
		} else if !strings.Contains(base, "/api/") {
			base += "/api/v3"
		}
	case "gitea":
		base += "/api/v1"
	}
	parsed, _ := url.Parse(base + apiPath)
	query := parsed.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func OAuthConfig(provider model.GitProvider, redirectURL string, clientSecret string) (*oauth2.Config, error) {
	endpoint, err := OAuthEndpoint(provider)
	if err != nil {
		return nil, err
	}
	scopes := []string{"repo", "read:user"}
	if provider.Type == "gitea" {
		scopes = []string{"read:repository", "write:repository", "read:user"}
	}
	return &oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: strings.TrimSpace(clientSecret),
		Endpoint:     endpoint,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
	}, nil
}

func OAuthEndpoint(provider model.GitProvider) (oauth2.Endpoint, error) {
	base := strings.TrimRight(provider.BaseURL, "/")
	switch provider.Type {
	case "github":
		if base == "" {
			base = "https://github.com"
		}
		return oauth2.Endpoint{
			AuthURL:  base + "/login/oauth/authorize",
			TokenURL: base + "/login/oauth/access_token",
		}, nil
	case "gitea":
		return oauth2.Endpoint{
			AuthURL:  base + "/login/oauth/authorize",
			TokenURL: base + "/login/oauth/access_token",
		}, nil
	default:
		return oauth2.Endpoint{}, fmt.Errorf("git provider type %q is not supported", provider.Type)
	}
}

func (c Client) CurrentUser(ctx context.Context) (UserResponse, error) {
	switch c.provider.Type {
	case "github", "gitea":
		var user UserResponse
		err := c.getJSON(ctx, c.apiURL("/user", nil), &user)
		return user, err
	default:
		return UserResponse{}, fmt.Errorf("git provider type %q is not supported", c.provider.Type)
	}
}

func decodeGitResponse(resp *http.Response, output any) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeUpstreamError(resp.StatusCode, body)
	}
	if output == nil || len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, output)
}

func decodeUpstreamError(statusCode int, body []byte) error {
	var response struct {
		Message          string                `json:"message"`
		DocumentationURL string                `json:"documentation_url"`
		Errors           []UpstreamErrorDetail `json:"errors"`
	}
	if err := json.Unmarshal(body, &response); err == nil && (strings.TrimSpace(response.Message) != "" || len(response.Errors) > 0) {
		return &UpstreamError{
			StatusCode:       statusCode,
			Message:          strings.TrimSpace(response.Message),
			DocumentationURL: strings.TrimSpace(response.DocumentationURL),
			Details:          response.Errors,
		}
	}
	return &UpstreamError{
		StatusCode: statusCode,
		Message:    http.StatusText(statusCode),
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

func isDockerfileName(name string) bool {
	return name == "Dockerfile" || strings.HasPrefix(name, "Dockerfile.") || strings.HasSuffix(name, ".Dockerfile")
}

func parseDockerfileExposedPorts(content string) []int {
	seen := map[int]bool{}
	ports := []int{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "EXPOSE") {
			continue
		}
		for _, field := range fields[1:] {
			value := strings.TrimSpace(strings.SplitN(field, "/", 2)[0])
			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 || seen[port] {
				continue
			}
			seen[port] = true
			ports = append(ports, port)
		}
	}
	return ports
}

func sortedPathSet(paths map[string]struct{}) []string {
	output := make([]string, 0, len(paths))
	for path := range paths {
		output = append(output, path)
	}
	sortBuildPaths(output)
	return output
}

func sortBuildPaths(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		if paths[i] == "." {
			return true
		}
		if paths[j] == "." {
			return false
		}
		return paths[i] < paths[j]
	})
}

type UserResponse struct {
	ID        any    `json:"id"`
	Login     string `json:"login"`
	UserName  string `json:"username"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

func (u UserResponse) ExternalID() string {
	switch value := u.ID.(type) {
	case json.Number:
		return strings.TrimSpace(value.String())
	case float64:
		if math.Trunc(value) == value {
			return strconv.FormatInt(int64(value), 10)
		}
		return strings.TrimSpace(strconv.FormatFloat(value, 'f', -1, 64))
	case float32:
		floatValue := float64(value)
		if math.Trunc(floatValue) == floatValue {
			return strconv.FormatInt(int64(floatValue), 10)
		}
		return strings.TrimSpace(strconv.FormatFloat(floatValue, 'f', -1, 32))
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func (u UserResponse) Username() string {
	if strings.TrimSpace(u.Login) != "" {
		return strings.TrimSpace(u.Login)
	}
	if strings.TrimSpace(u.UserName) != "" {
		return strings.TrimSpace(u.UserName)
	}
	return strings.TrimSpace(u.Name)
}

type githubRepositoryResponse struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type githubRepositorySearchResponse struct {
	Items []githubRepositoryResponse `json:"items"`
}

type giteaRepositoryResponse struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Owner         struct {
		UserName string `json:"username"`
	} `json:"owner"`
}

type giteaRepositorySearchResponse struct {
	Data []giteaRepositoryResponse `json:"data"`
}

type gitBranchResponse struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

type gitContentResponse struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type gitTreeResponse struct {
	SHA       string `json:"sha"`
	Truncated bool   `json:"truncated"`
	Tree      []struct {
		Path string `json:"path"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"tree"`
}

func normalizeContentType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "dir", "directory", "tree":
		return "dir"
	case "file", "blob":
		return "file"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

type githubWebhookResponse struct {
	ID     int64 `json:"id"`
	Config struct {
		URL string `json:"url"`
	} `json:"config"`
}

type giteaWebhookResponse struct {
	ID int64 `json:"id"`
}
