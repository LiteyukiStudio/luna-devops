package appstore

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"strings"
)

//go:embed templates.json
var templateFS embed.FS

type Template struct {
	ID                 string `json:"id"`
	Slug               string `json:"slug"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Category           string `json:"category"`
	Kind               string `json:"kind"`
	SystemComponent    string `json:"systemComponent"`
	Icon               string `json:"icon"`
	OfficialWebsite    string `json:"officialWebsite"`
	OfficialRepository string `json:"officialRepository"`
	PopularityWeight   int    `json:"popularityWeight"`
	Version            string `json:"version"`
	Workload
	// Components 声明多组件模板；第一个组件是入口组件。声明后模板自身不再携带单容器 Workload。
	Components []Component       `json:"components"`
	Values     []ValueDefinition `json:"values"`
}

// Workload 是模板中一个容器化应用的运行定义，单组件模板和多组件模板的每个组件共用。
type Workload struct {
	Image            string            `json:"image"`
	ServicePort      int               `json:"servicePort"`
	DefaultReplicas  int               `json:"defaultReplicas"`
	DefaultCPU       string            `json:"defaultCPU"`
	DefaultMemory    string            `json:"defaultMemory"`
	ContainerCommand string            `json:"containerCommand"`
	ContainerArgs    string            `json:"containerArgs"`
	FSGroup          string            `json:"fsGroup"`
	DataVolumes      []DataVolume      `json:"dataVolumes"`
	Env              map[string]string `json:"env"`
	SecretEnv        map[string]string `json:"secretEnv"`
	ConfigFiles      []ConfigFile      `json:"configFiles"`
	SecretFiles      []ConfigFile      `json:"secretFiles"`
}

// Component 是多组件模板中的一个独立应用，例如主应用前后端或它依赖的数据库。
type Component struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Workload
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

type RenderedWorkload struct {
	Env         map[string]string
	SecretEnv   map[string]string
	ConfigFiles []ConfigFile
	SecretFiles []ConfigFile
}

var (
	// placeholderPattern 支持 {{key}} 与 {{key | urlUserinfo}}；后者把值转义为 URL userinfo，供连接串中的密码使用。
	placeholderPattern  = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*(?:\|\s*(urlUserinfo)\s*)?\}\}`)
	componentKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
)

// ComponentHostValueKey 是多组件模板引用同一次安装中其他组件服务地址的占位符键。
func ComponentHostValueKey(componentKey string) string {
	return "components." + componentKey + ".host"
}

// IsMultiComponent 判断模板是否按组件安装多个应用。
func (template Template) IsMultiComponent() bool {
	return len(template.Components) > 0
}

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
		if err := validateTemplateWorkloads(templates[index]); err != nil {
			return nil, fmt.Errorf("template %s: %w", templates[index].ID, err)
		}
	}
	return templates, nil
}

func validateTemplateWorkloads(template Template) error {
	if !template.IsMultiComponent() {
		return validateWorkloadDataVolumes(template.Workload)
	}
	if !reflect.DeepEqual(template.Workload, Workload{}) {
		return fmt.Errorf("multi-component template cannot declare a template-level workload")
	}
	keys := make(map[string]struct{}, len(template.Components))
	for _, component := range template.Components {
		if !componentKeyPattern.MatchString(component.Key) {
			return fmt.Errorf("component key %q must match %s", component.Key, componentKeyPattern.String())
		}
		if _, exists := keys[component.Key]; exists {
			return fmt.Errorf("component key %q is duplicated", component.Key)
		}
		keys[component.Key] = struct{}{}
		if strings.TrimSpace(component.Name) == "" || strings.TrimSpace(component.Image) == "" || component.ServicePort <= 0 {
			return fmt.Errorf("component %q requires name, image, and servicePort", component.Key)
		}
		if err := validateWorkloadDataVolumes(component.Workload); err != nil {
			return fmt.Errorf("component %s: %w", component.Key, err)
		}
	}
	for _, component := range template.Components {
		for _, reference := range workloadComponentReferences(component.Workload) {
			if _, exists := keys[reference]; !exists {
				return fmt.Errorf("component %s references unknown component %q", component.Key, reference)
			}
		}
	}
	return nil
}

func workloadComponentReferences(workload Workload) []string {
	sources := make([]string, 0, len(workload.Env)+len(workload.SecretEnv)+len(workload.ConfigFiles)+len(workload.SecretFiles))
	for _, value := range workload.Env {
		sources = append(sources, value)
	}
	for _, value := range workload.SecretEnv {
		sources = append(sources, value)
	}
	for _, file := range append(append([]ConfigFile{}, workload.ConfigFiles...), workload.SecretFiles...) {
		sources = append(sources, file.Content)
	}
	references := []string{}
	for _, source := range sources {
		for _, match := range placeholderPattern.FindAllStringSubmatch(source, -1) {
			if key, ok := strings.CutPrefix(match[1], "components."); ok {
				references = append(references, strings.TrimSuffix(key, ".host"))
			}
		}
	}
	return references
}

func validateWorkloadDataVolumes(template Workload) error {
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

// ResolveValues 按模板参数定义补默认值、生成随机密钥并校验必填项；同一次安装的所有组件共用这份结果。
func ResolveValues(template Template, input map[string]string) (map[string]string, error) {
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
			return nil, fmt.Errorf("template value %s is required", key)
		}
		if definition.Secret && !definition.Required && values[key] == "" {
			delete(values, key)
		}
	}
	return values, nil
}

// RenderWorkload 用已解析的参数渲染一个运行定义中的环境变量与文件。
func RenderWorkload(workload Workload, values map[string]string) RenderedWorkload {
	return RenderedWorkload{
		Env:         renderStringMap(workload.Env, values),
		SecretEnv:   renderNonEmptyStringMap(workload.SecretEnv, values),
		ConfigFiles: renderConfigFiles(workload.ConfigFiles, values),
		SecretFiles: renderConfigFiles(workload.SecretFiles, values),
	}
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
		if len(parts) != 3 {
			return match
		}
		if parts[2] == "urlUserinfo" {
			return strings.TrimPrefix(url.UserPassword("", values[parts[1]]).String(), ":")
		}
		return values[parts[1]]
	})
}

func randomSecret() string {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "change-me"
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
