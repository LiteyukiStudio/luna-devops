package appstore

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

//go:embed templates.json
var templateFS embed.FS

type Template struct {
	ID                 string            `json:"id"`
	Slug               string            `json:"slug"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Category           string            `json:"category"`
	Kind               string            `json:"kind"`
	SystemComponent    string            `json:"systemComponent"`
	Icon               string            `json:"icon"`
	OfficialWebsite    string            `json:"officialWebsite"`
	OfficialRepository string            `json:"officialRepository"`
	PopularityWeight   int               `json:"popularityWeight"`
	Image              string            `json:"image"`
	Version            string            `json:"version"`
	ServicePort        int               `json:"servicePort"`
	DefaultReplicas    int               `json:"defaultReplicas"`
	DefaultCPU         string            `json:"defaultCPU"`
	DefaultMemory      string            `json:"defaultMemory"`
	ContainerCommand   string            `json:"containerCommand"`
	ContainerArgs      string            `json:"containerArgs"`
	DataVolumes        []DataVolume      `json:"dataVolumes"`
	Env                map[string]string `json:"env"`
	SecretEnv          map[string]string `json:"secretEnv"`
	ConfigFiles        []ConfigFile      `json:"configFiles"`
	SecretFiles        []ConfigFile      `json:"secretFiles"`
	Values             []ValueDefinition `json:"values"`
}

type DataVolume struct {
	LogicalName string        `json:"logicalName"`
	SourceType  string        `json:"sourceType"`
	MountPath   string        `json:"mountPath,omitempty"`
	DevicePath  string        `json:"devicePath,omitempty"`
	ReadOnly    bool          `json:"readOnly,omitempty"`
	EmptyDir    *EmptyDirSpec `json:"emptyDir,omitempty"`
}

type EmptyDirSpec struct {
	Medium    string `json:"medium,omitempty"`
	SizeLimit string `json:"sizeLimit,omitempty"`
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
		if err := validateTemplateDataVolumes(templates[index]); err != nil {
			return nil, fmt.Errorf("template %s: %w", templates[index].ID, err)
		}
	}
	return templates, nil
}

func validateTemplateDataVolumes(template Template) error {
	projectVolumeCount := 0
	logicalNames := make(map[string]struct{}, len(template.DataVolumes))
	paths := make(map[string]struct{}, len(template.DataVolumes))
	for _, dataVolume := range template.DataVolumes {
		logicalName := strings.TrimSpace(dataVolume.LogicalName)
		if logicalName == "" {
			return fmt.Errorf("data volume logicalName is required")
		}
		if _, exists := logicalNames[logicalName]; exists {
			return fmt.Errorf("data volume logicalName %q is duplicated", logicalName)
		}
		logicalNames[logicalName] = struct{}{}

		mountPath := strings.TrimSpace(dataVolume.MountPath)
		devicePath := strings.TrimSpace(dataVolume.DevicePath)
		switch strings.TrimSpace(dataVolume.SourceType) {
		case "projectVolume":
			projectVolumeCount++
			if projectVolumeCount > 1 {
				return fmt.Errorf("only one projectVolume declaration is supported")
			}
			if (mountPath == "") == (devicePath == "") || dataVolume.EmptyDir != nil {
				return fmt.Errorf("projectVolume %q requires exactly one mountPath or devicePath", logicalName)
			}
		case "emptyDir":
			if mountPath == "" || devicePath != "" || dataVolume.ReadOnly {
				return fmt.Errorf("emptyDir %q requires mountPath and cannot use devicePath or readOnly", logicalName)
			}
		default:
			return fmt.Errorf("data volume %q sourceType must be projectVolume or emptyDir", logicalName)
		}
		volumePath := mountPath
		if volumePath == "" {
			volumePath = devicePath
		}
		if !strings.HasPrefix(volumePath, "/") || path.Clean(volumePath) == "/" {
			return fmt.Errorf("data volume %q path must be absolute and cannot be root", logicalName)
		}
		cleaned := path.Clean(volumePath)
		if _, exists := paths[cleaned]; exists {
			return fmt.Errorf("data volume path %q is duplicated", cleaned)
		}
		paths[cleaned] = struct{}{}
	}
	return nil
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
		if definition.Secret && !definition.Required && values[key] == "" {
			delete(values, key)
		}
	}
	rendered := RenderedTemplate{
		Values:      values,
		Env:         renderStringMap(template.Env, values),
		SecretEnv:   renderNonEmptyStringMap(template.SecretEnv, values),
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

func renderNonEmptyStringMap(source map[string]string, values map[string]string) map[string]string {
	output := renderStringMap(source, values)
	for key, value := range output {
		if strings.TrimSpace(value) == "" {
			delete(output, key)
		}
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
