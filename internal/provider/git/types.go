package gitprovider

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

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
