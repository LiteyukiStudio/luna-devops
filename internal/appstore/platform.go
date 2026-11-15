package appstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const PlatformTemplateBundleSchemaVersion = "luna.app-template/v1"

var (
	platformSlugPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	valueKeyPattern     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)
	allowedCategories   = []string{
		"collaboration",
		"database",
		"developerTool",
		"middleware",
		"observability",
		"security",
		"storage",
	}
)

// PlatformTemplateValidationError 表示平台应用模板规格校验失败，可映射为稳定 400 错误响应。
type PlatformTemplateValidationError struct {
	Code    string
	Message string
}

func (err *PlatformTemplateValidationError) Error() string {
	return err.Message
}

func validationErrorf(code, format string, args ...any) error {
	return &PlatformTemplateValidationError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// AllowedCategories 返回应用商城允许的稳定分类枚举。
func AllowedCategories() []string {
	return slices.Clone(allowedCategories)
}

// TemplateSpec 是 platform_app_templates.spec_json 持久化的运行与参数规格，与内置模板完全同构。
type TemplateSpec struct {
	OfficialWebsite    string            `json:"officialWebsite,omitempty"`
	OfficialRepository string            `json:"officialRepository,omitempty"`
	Workload           Workload          `json:"workload,omitempty"`
	Components         []Component       `json:"components,omitempty"`
	Values             []ValueDefinition `json:"values,omitempty"`
}

// PlatformTemplateBundle 是平台模板导入/导出的可移植 JSON 文档，兼容内置 templates/*.json 结构。
type PlatformTemplateBundle struct {
	SchemaVersion      string `json:"schemaVersion,omitempty"`
	Slug               string `json:"slug"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Category           string `json:"category"`
	Icon               string `json:"icon,omitempty"`
	Version            string `json:"version"`
	Enabled            *bool  `json:"enabled,omitempty"`
	Featured           bool   `json:"featured"`
	PopularityWeight   int    `json:"popularityWeight"`
	OfficialWebsite    string `json:"officialWebsite,omitempty"`
	OfficialRepository string `json:"officialRepository,omitempty"`
	Workload
	Components []Component       `json:"components,omitempty"`
	Values     []ValueDefinition `json:"values,omitempty"`
}

func (bundle PlatformTemplateBundle) ToTemplate(id string) Template {
	return Template{
		ID:                 strings.TrimSpace(id),
		Slug:               bundle.Slug,
		Name:               bundle.Name,
		Description:        bundle.Description,
		Category:           bundle.Category,
		Kind:               "application",
		Source:             TemplateSourcePlatform,
		Icon:               bundle.Icon,
		OfficialWebsite:    bundle.OfficialWebsite,
		OfficialRepository: bundle.OfficialRepository,
		PopularityWeight:   bundle.PopularityWeight,
		Featured:           bundle.Featured,
		Version:            bundle.Version,
		Workload:           bundle.Workload,
		Components:         bundle.Components,
		Values:             bundle.Values,
	}
}

// BundleFromTemplate 将内置或平台模板转换为可安全导出的 JSON 文档，自动清空敏感参数的默认值。
func BundleFromTemplate(template Template, enabled bool) PlatformTemplateBundle {
	cleanValues := make([]ValueDefinition, 0, len(template.Values))
	for _, item := range template.Values {
		copied := item
		if copied.Secret {
			copied.Default = ""
		}
		cleanValues = append(cleanValues, copied)
	}
	enabledFlag := enabled
	return PlatformTemplateBundle{
		SchemaVersion:      PlatformTemplateBundleSchemaVersion,
		Slug:               template.Slug,
		Name:               template.Name,
		Description:        template.Description,
		Category:           template.Category,
		Icon:               template.Icon,
		Version:            template.Version,
		Enabled:            &enabledFlag,
		Featured:           template.Featured,
		PopularityWeight:   template.PopularityWeight,
		OfficialWebsite:    template.OfficialWebsite,
		OfficialRepository: template.OfficialRepository,
		Workload:           template.Workload,
		Components:         slices.Clone(template.Components),
		Values:             cleanValues,
	}
}

// IsBuiltinSlugOrID 检查标识是否已被内置模板占用。
func IsBuiltinSlugOrID(candidate string) (bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(candidate))
	if normalized == "" {
		return false, nil
	}
	builtins, err := Catalog()
	if err != nil {
		return false, err
	}
	for _, tpl := range builtins {
		if strings.EqualFold(tpl.ID, normalized) || strings.EqualFold(tpl.Slug, normalized) {
			return true, nil
		}
	}
	return false, nil
}

// ValidatePlatformTemplate 复用内置目录强校验并补充平台级模板的参数、链接和敏感字段安全约束。
func ValidatePlatformTemplate(template Template) (Template, error) {
	template.Slug = strings.ToLower(strings.TrimSpace(template.Slug))
	if len(template.Slug) < 2 || len(template.Slug) > 63 || !platformSlugPattern.MatchString(template.Slug) {
		return Template{}, validationErrorf("app_template.slug_invalid", "template slug %q must be 2-63 lowercase alphanumeric characters or hyphens", template.Slug)
	}

	template.Name = strings.TrimSpace(template.Name)
	if template.Name == "" || utf8.RuneCountInString(template.Name) > 120 {
		return Template{}, validationErrorf("app_template.name_invalid", "template name is required and must not exceed 120 characters")
	}

	template.Description = strings.TrimSpace(template.Description)
	if utf8.RuneCountInString(template.Description) > 2000 {
		return Template{}, validationErrorf("app_template.description_invalid", "template description must not exceed 2000 characters")
	}

	template.Category = strings.TrimSpace(template.Category)
	if !slices.Contains(allowedCategories, template.Category) {
		return Template{}, validationErrorf("app_template.category_invalid", "template category %q must be one of %s", template.Category, strings.Join(allowedCategories, ", "))
	}

	kind := strings.TrimSpace(template.Kind)
	if (kind != "" && kind != "application") || strings.TrimSpace(template.SystemComponent) != "" {
		return Template{}, validationErrorf("app_template.kind_invalid", "platform templates only support standard application kind")
	}
	template.Kind = "application"
	template.SystemComponent = ""
	template.Source = TemplateSourcePlatform

	template.Icon = strings.TrimSpace(template.Icon)
	if utf8.RuneCountInString(template.Icon) > 64 {
		return Template{}, validationErrorf("app_template.icon_invalid", "template icon must not exceed 64 characters")
	}

	template.Version = strings.TrimSpace(template.Version)
	if template.Version == "" {
		template.Version = "1.0.0"
	}
	if utf8.RuneCountInString(template.Version) > 64 {
		return Template{}, validationErrorf("app_template.version_invalid", "template version must not exceed 64 characters")
	}

	if template.PopularityWeight < 0 || template.PopularityWeight > 1000 {
		return Template{}, validationErrorf("app_template.popularity_invalid", "popularityWeight must be between 0 and 1000")
	}

	var err error
	if template.OfficialWebsite, err = normalizeOptionalHTTPURL(template.OfficialWebsite, "officialWebsite"); err != nil {
		return Template{}, err
	}
	if template.OfficialRepository, err = normalizeOptionalHTTPURL(template.OfficialRepository, "officialRepository"); err != nil {
		return Template{}, err
	}
	if template.OfficialWebsite == "" {
		template.OfficialWebsite = template.OfficialRepository
	}

	if !template.IsMultiComponent() {
		normalized, err := normalizeAndValidateWorkload(template.Workload, "workload")
		if err != nil {
			return Template{}, err
		}
		template.Workload = normalized
	} else {
		normalizedComponents := make([]Component, len(template.Components))
		for idx, comp := range template.Components {
			comp.Key = strings.TrimSpace(comp.Key)
			comp.Name = strings.TrimSpace(comp.Name)
			normalizedWorkload, err := normalizeAndValidateWorkload(comp.Workload, fmt.Sprintf("component %q", comp.Key))
			if err != nil {
				return Template{}, err
			}
			comp.Workload = normalizedWorkload
			normalizedComponents[idx] = comp
		}
		template.Components = normalizedComponents
	}

	if err := validateTemplateWorkloads(template); err != nil {
		return Template{}, validationErrorf("app_template.workload_invalid", "%s", err.Error())
	}

	valueMap := make(map[string]ValueDefinition, len(template.Values))
	normalizedValues := make([]ValueDefinition, 0, len(template.Values))
	for _, def := range template.Values {
		def.Key = strings.TrimSpace(def.Key)
		def.Label = strings.TrimSpace(def.Label)
		def.Description = strings.TrimSpace(def.Description)
		def.Default = strings.TrimSpace(def.Default)
		if !valueKeyPattern.MatchString(def.Key) {
			return Template{}, validationErrorf("app_template.value_invalid", "value key %q must match %s", def.Key, valueKeyPattern.String())
		}
		if _, exists := valueMap[def.Key]; exists {
			return Template{}, validationErrorf("app_template.value_duplicated", "value key %q is duplicated", def.Key)
		}
		if def.Label == "" {
			def.Label = def.Key
		}
		if def.Secret && def.Default != "" {
			return Template{}, validationErrorf("app_template.secret_default_forbidden", "secret value %q must not contain a plaintext default", def.Key)
		}
		valueMap[def.Key] = def
		normalizedValues = append(normalizedValues, def)
	}
	template.Values = normalizedValues

	componentKeys := make(map[string]struct{}, len(template.Components))
	for _, comp := range template.Components {
		componentKeys[comp.Key] = struct{}{}
	}
	if !template.IsMultiComponent() {
		if err := validateWorkloadPlaceholdersAndSecrets(template.Workload, valueMap, componentKeys, "workload"); err != nil {
			return Template{}, err
		}
	} else {
		for _, comp := range template.Components {
			if err := validateWorkloadPlaceholdersAndSecrets(comp.Workload, valueMap, componentKeys, fmt.Sprintf("component %q", comp.Key)); err != nil {
				return Template{}, err
			}
		}
	}

	return template, nil
}

func normalizeOptionalHTTPURL(raw, field string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.TrimSpace(parsed.Host) == "" {
		return "", validationErrorf("app_template.url_invalid", "%s must be a valid http or https URL", field)
	}
	return trimmed, nil
}

func normalizeAndValidateWorkload(workload Workload, scope string) (Workload, error) {
	workload.Image = strings.TrimSpace(workload.Image)
	if workload.Image == "" {
		return Workload{}, validationErrorf("app_template.workload_invalid", "%s image is required", scope)
	}
	if workload.ServicePort <= 0 || workload.ServicePort > 65535 {
		return Workload{}, validationErrorf("app_template.workload_invalid", "%s servicePort must be between 1 and 65535", scope)
	}
	if workload.DefaultReplicas <= 0 {
		workload.DefaultReplicas = 1
	}
	workload.DefaultCPU = strings.TrimSpace(workload.DefaultCPU)
	if workload.DefaultCPU == "" {
		workload.DefaultCPU = "250m"
	}
	workload.DefaultMemory = strings.TrimSpace(workload.DefaultMemory)
	if workload.DefaultMemory == "" {
		workload.DefaultMemory = "256Mi"
	}
	workload.FSGroup = strings.TrimSpace(workload.FSGroup)

	seenPaths := make(map[string]struct{}, len(workload.ConfigFiles)+len(workload.SecretFiles))
	normalizeFiles := func(files []ConfigFile, label string) ([]ConfigFile, error) {
		if len(files) == 0 {
			return nil, nil
		}
		out := make([]ConfigFile, 0, len(files))
		for _, file := range files {
			filePath := strings.TrimSpace(file.Path)
			if !strings.HasPrefix(filePath, "/") || path.Clean(filePath) == "/" {
				return nil, validationErrorf("app_template.workload_invalid", "%s %s path %q must be an absolute non-root path", scope, label, file.Path)
			}
			cleaned := path.Clean(filePath)
			if _, exists := seenPaths[cleaned]; exists {
				return nil, validationErrorf("app_template.workload_invalid", "%s file path %q is duplicated", scope, cleaned)
			}
			seenPaths[cleaned] = struct{}{}
			out = append(out, ConfigFile{Path: cleaned, Content: file.Content})
		}
		return out, nil
	}

	var err error
	if workload.ConfigFiles, err = normalizeFiles(workload.ConfigFiles, "configFiles"); err != nil {
		return Workload{}, err
	}
	if workload.SecretFiles, err = normalizeFiles(workload.SecretFiles, "secretFiles"); err != nil {
		return Workload{}, err
	}
	return workload, nil
}

func validateWorkloadPlaceholdersAndSecrets(
	workload Workload,
	values map[string]ValueDefinition,
	componentKeys map[string]struct{},
	scope string,
) error {
	checkPlaceholders := func(raw string, allowSecret bool, requirePlaceholder bool, targetLabel string) error {
		matches := placeholderPattern.FindAllStringSubmatch(raw, -1)
		if requirePlaceholder && len(matches) == 0 {
			return validationErrorf("app_template.secret_literal_forbidden", "%s %s must reference a template placeholder rather than storing plaintext secrets", scope, targetLabel)
		}
		for _, match := range matches {
			ref := match[1]
			if compRef, ok := strings.CutPrefix(ref, "components."); ok {
				compKey, hasHost := strings.CutSuffix(compRef, ".host")
				if !hasHost {
					return validationErrorf("app_template.placeholder_invalid", "%s %s references unsupported component field %q", scope, targetLabel, ref)
				}
				if _, exists := componentKeys[compKey]; !exists {
					return validationErrorf("app_template.placeholder_invalid", "%s %s references unknown component %q", scope, targetLabel, compKey)
				}
				continue
			}
			def, exists := values[ref]
			if !exists {
				return validationErrorf("app_template.placeholder_invalid", "%s %s references undeclared value %q", scope, targetLabel, ref)
			}
			if !allowSecret && def.Secret {
				return validationErrorf("app_template.secret_leak_forbidden", "%s %s cannot reference secret value %q in plaintext config", scope, targetLabel, ref)
			}
		}
		return nil
	}

	for key, val := range workload.Env {
		if strings.TrimSpace(key) == "" {
			return validationErrorf("app_template.workload_invalid", "%s env key cannot be empty", scope)
		}
		if err := checkPlaceholders(val, false, false, fmt.Sprintf("env[%s]", key)); err != nil {
			return err
		}
	}
	for key, val := range workload.SecretEnv {
		if strings.TrimSpace(key) == "" {
			return validationErrorf("app_template.workload_invalid", "%s secretEnv key cannot be empty", scope)
		}
		if err := checkPlaceholders(val, true, true, fmt.Sprintf("secretEnv[%s]", key)); err != nil {
			return err
		}
	}
	for _, file := range workload.ConfigFiles {
		if err := checkPlaceholders(file.Content, false, false, fmt.Sprintf("configFiles[%s]", file.Path)); err != nil {
			return err
		}
	}
	for _, file := range workload.SecretFiles {
		if err := checkPlaceholders(file.Content, true, true, fmt.Sprintf("secretFiles[%s]", file.Path)); err != nil {
			return err
		}
	}
	return nil
}

// ModelFromPlatformTemplate 校验并构建数据库模型。
func ModelFromPlatformTemplate(recordID, createdBy string, enabled bool, template Template) (model.PlatformAppTemplate, Template, error) {
	validated, err := ValidatePlatformTemplate(template)
	if err != nil {
		return model.PlatformAppTemplate{}, Template{}, err
	}
	validated.ID = strings.TrimSpace(recordID)
	spec := TemplateSpec{
		OfficialWebsite:    validated.OfficialWebsite,
		OfficialRepository: validated.OfficialRepository,
		Workload:           validated.Workload,
		Components:         validated.Components,
		Values:             validated.Values,
	}
	encodedSpec, err := json.Marshal(spec)
	if err != nil {
		return model.PlatformAppTemplate{}, Template{}, fmt.Errorf("marshal template spec: %w", err)
	}
	return model.PlatformAppTemplate{
		ID:               validated.ID,
		Slug:             validated.Slug,
		Name:             validated.Name,
		Description:      validated.Description,
		Category:         validated.Category,
		Icon:             validated.Icon,
		Version:          validated.Version,
		IsMultiComponent: validated.IsMultiComponent(),
		Enabled:          enabled,
		Featured:         validated.Featured,
		PopularityWeight: validated.PopularityWeight,
		SpecJSON:         string(encodedSpec),
		CreatedBy:        strings.TrimSpace(createdBy),
	}, validated, nil
}

// TemplateFromPlatformModel 将数据库记录还原为运行时 Template。
func TemplateFromPlatformModel(record model.PlatformAppTemplate) (Template, error) {
	var spec TemplateSpec
	rawSpec := strings.TrimSpace(record.SpecJSON)
	if rawSpec != "" {
		if err := json.Unmarshal([]byte(rawSpec), &spec); err != nil {
			return Template{}, fmt.Errorf("decode platform template %s spec_json: %w", record.ID, err)
		}
	}
	officialWebsite := strings.TrimSpace(spec.OfficialWebsite)
	officialRepository := strings.TrimSpace(spec.OfficialRepository)
	if officialWebsite == "" {
		officialWebsite = officialRepository
	}
	return Template{
		ID:                 record.ID,
		Slug:               record.Slug,
		Name:               record.Name,
		Description:        record.Description,
		Category:           record.Category,
		Kind:               "application",
		Source:             TemplateSourcePlatform,
		Icon:               record.Icon,
		OfficialWebsite:    officialWebsite,
		OfficialRepository: officialRepository,
		PopularityWeight:   record.PopularityWeight,
		Featured:           record.Featured,
		Version:            record.Version,
		Workload:           spec.Workload,
		Components:         spec.Components,
		Values:             spec.Values,
	}, nil
}

// CatalogWithPlatform 返回内置目录与已启用平台数据库模板的合并视图。db 为空时仅返回内置目录。
func CatalogWithPlatform(ctx context.Context, db *gorm.DB) ([]Template, error) {
	builtins, err := Catalog()
	if err != nil || db == nil {
		return builtins, err
	}
	var records []model.PlatformAppTemplate
	if err := db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("popularity_weight DESC, slug ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return builtins, nil
	}
	seenIDs := make(map[string]struct{}, len(builtins)+len(records))
	seenSlugs := make(map[string]struct{}, len(builtins)+len(records))
	merged := make([]Template, 0, len(builtins)+len(records))
	for _, tpl := range builtins {
		seenIDs[strings.ToLower(tpl.ID)] = struct{}{}
		seenSlugs[strings.ToLower(tpl.Slug)] = struct{}{}
		merged = append(merged, tpl)
	}
	for _, record := range records {
		if _, exists := seenIDs[strings.ToLower(record.ID)]; exists {
			continue
		}
		if _, exists := seenSlugs[strings.ToLower(record.Slug)]; exists {
			continue
		}
		tpl, err := TemplateFromPlatformModel(record)
		if err != nil {
			return nil, err
		}
		merged = append(merged, tpl)
	}
	slices.SortFunc(merged, compareCatalogTemplates)
	return merged, nil
}

// FindWithPlatform 按模板 ID 或 Slug 查找内置模板或已启用的平台数据库模板。
func FindWithPlatform(ctx context.Context, db *gorm.DB, idOrSlug string) (Template, bool, error) {
	key := strings.TrimSpace(idOrSlug)
	if key == "" {
		return Template{}, false, nil
	}
	builtins, err := Catalog()
	if err != nil {
		return Template{}, false, err
	}
	for _, tpl := range builtins {
		if tpl.ID == key || tpl.Slug == key {
			return tpl, true, nil
		}
	}
	if db == nil {
		return Template{}, false, nil
	}
	var record model.PlatformAppTemplate
	err = db.WithContext(ctx).
		Where("enabled = ? AND (id = ? OR slug = ?)", true, key, strings.ToLower(key)).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Template{}, false, nil
	}
	if err != nil {
		return Template{}, false, err
	}
	tpl, err := TemplateFromPlatformModel(record)
	if err != nil {
		return Template{}, false, err
	}
	return tpl, true, nil
}
