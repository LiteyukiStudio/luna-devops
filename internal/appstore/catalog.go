package appstore

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed templates.json
var templateFS embed.FS

type Template struct {
	ID                   string            `json:"id"`
	Slug                 string            `json:"slug"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	Category             string            `json:"category"`
	Icon                 string            `json:"icon"`
	OfficialWebsite      string            `json:"officialWebsite"`
	OfficialRepository   string            `json:"officialRepository"`
	PopularityWeight     int               `json:"popularityWeight"`
	Image                string            `json:"image"`
	Version              string            `json:"version"`
	ServicePort          int               `json:"servicePort"`
	DefaultReplicas      int               `json:"defaultReplicas"`
	DefaultCPU           string            `json:"defaultCPU"`
	DefaultMemory        string            `json:"defaultMemory"`
	DataRetentionEnabled bool              `json:"dataRetentionEnabled"`
	DataMountPath        string            `json:"dataMountPath"`
	DataCapacity         string            `json:"dataCapacity"`
	Env                  map[string]string `json:"env"`
	SecretEnv            map[string]string `json:"secretEnv"`
	ConfigFiles          []ConfigFile      `json:"configFiles"`
	SecretFiles          []ConfigFile      `json:"secretFiles"`
	Values               []ValueDefinition `json:"values"`
}

type ConfigFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ValueDefinition struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	Default      string `json:"default"`
	Required     bool   `json:"required"`
	Secret       bool   `json:"secret"`
	AutoGenerate bool   `json:"autoGenerate"`
}

type RenderedTemplate struct {
	Values      map[string]string
	Env         map[string]string
	SecretEnv   map[string]string
	ConfigFiles []ConfigFile
	SecretFiles []ConfigFile
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

func Catalog() ([]Template, error) {
	content, err := templateFS.ReadFile("templates.json")
	if err != nil {
		return nil, err
	}
	var templates []Template
	if err := json.Unmarshal(content, &templates); err != nil {
		return nil, err
	}
	for index := range templates {
		templates[index].OfficialWebsite = strings.TrimSpace(templates[index].OfficialWebsite)
		templates[index].OfficialRepository = strings.TrimSpace(templates[index].OfficialRepository)
		if templates[index].OfficialWebsite == "" {
			templates[index].OfficialWebsite = templates[index].OfficialRepository
		}
	}
	return templates, nil
}

func Find(id string) (Template, bool, error) {
	templates, err := Catalog()
	if err != nil {
		return Template{}, false, err
	}
	id = strings.TrimSpace(id)
	for _, template := range templates {
		if template.ID == id {
			return template, true, nil
		}
	}
	return Template{}, false, nil
}

func Render(template Template, input map[string]string) (RenderedTemplate, error) {
	values := map[string]string{}
	for key, value := range input {
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	for _, definition := range template.Values {
		key := strings.TrimSpace(definition.Key)
		if key == "" {
			continue
		}
		if values[key] == "" {
			values[key] = strings.TrimSpace(definition.Default)
		}
		if values[key] == "" && definition.AutoGenerate {
			values[key] = randomSecret()
		}
		if definition.Required && values[key] == "" {
			return RenderedTemplate{}, fmt.Errorf("template value %s is required", key)
		}
	}
	rendered := RenderedTemplate{
		Values:      values,
		Env:         renderStringMap(template.Env, values),
		SecretEnv:   renderStringMap(template.SecretEnv, values),
		ConfigFiles: renderConfigFiles(template.ConfigFiles, values),
		SecretFiles: renderConfigFiles(template.SecretFiles, values),
	}
	return rendered, nil
}

func renderStringMap(source map[string]string, values map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range source {
		output[key] = renderTemplateString(value, values)
	}
	return output
}

func renderConfigFiles(source []ConfigFile, values map[string]string) []ConfigFile {
	output := make([]ConfigFile, 0, len(source))
	for _, file := range source {
		output = append(output, ConfigFile{
			Path:    strings.TrimSpace(file.Path),
			Content: renderTemplateString(file.Content, values),
		})
	}
	return output
}

func renderTemplateString(value string, values map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := placeholderPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return values[parts[1]]
	})
}

func randomSecret() string {
	data := make([]byte, 18)
	if _, err := rand.Read(data); err != nil {
		return "change-me"
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
