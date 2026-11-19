package runtimeapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type aiListConfigSetsInput struct {
	Project   string  `json:"project" desc:"项目 ID 或唯一 identifier"`
	Search    *string `json:"search,omitempty" desc:"按配置集名称、变量或文件路径模糊搜索"`
	SortBy    *string `json:"sortBy,omitempty" enum:"createdAt|name" default:"createdAt" desc:"排序字段"`
	SortOrder *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page      *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize  *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type aiGetConfigSetInput struct {
	Project   string `json:"project" desc:"项目 ID 或唯一 identifier"`
	ConfigSet string `json:"configSet" desc:"运行配置集 ID（prcs_xxx）或同项目唯一名称"`
}

type aiConfigSetPublicEnvItem struct {
	Key   string `json:"key" pattern:"^[A-Za-z_][A-Za-z0-9_]*$" desc:"普通环境变量键名"`
	Value string `json:"value" desc:"普通环境变量值"`
}

type aiConfigSetCreateSecretItem struct {
	Key       string  `json:"key" pattern:"^[A-Za-z_][A-Za-z0-9_]*$" desc:"加密环境变量键名"`
	Operation *string `json:"operation,omitempty" enum:"set|generate" default:"set" desc:"密钥操作（默认 set）"`
	Value     *string `json:"value,omitempty" sensitive:"true" desc:"密钥明文（operation=set 时必填非空）"`
	Encoding  *string `json:"encoding,omitempty" enum:"base64|hex|alphanumeric|numeric" desc:"随机生成编码（仅 operation=generate）"`
	Length    *int    `json:"length,omitempty" min:"8" max:"256" default:"32" desc:"随机生成长度 8..256（仅 operation=generate）"`
}

type aiConfigSetSecretActionItem struct {
	Key       string  `json:"key" pattern:"^[A-Za-z_][A-Za-z0-9_]*$" desc:"加密环境变量键名"`
	Operation *string `json:"operation,omitempty" enum:"set|generate|clear" default:"set" desc:"密钥操作（默认 set）"`
	Value     *string `json:"value,omitempty" sensitive:"true" desc:"密钥明文（operation=set 时必填非空）"`
	Encoding  *string `json:"encoding,omitempty" enum:"base64|hex|alphanumeric|numeric" desc:"随机生成编码（仅 operation=generate）"`
	Length    *int    `json:"length,omitempty" min:"8" max:"256" default:"32" desc:"随机生成长度 8..256（仅 operation=generate）"`
}

type aiConfigSetPlainFileItem struct {
	Path    string `json:"path" desc:"容器内非根绝对路径"`
	Content string `json:"content" desc:"明文文件内容"`
}

type aiConfigSetCreateSecretFileItem struct {
	Path    string `json:"path" desc:"容器内非根绝对路径"`
	Content string `json:"content" sensitive:"true" desc:"加密文件内容（必填非空）"`
}

type aiConfigSetSecretFileActionItem struct {
	Path      string  `json:"path" desc:"容器内非根绝对路径"`
	Operation *string `json:"operation,omitempty" enum:"set|clear" default:"set" desc:"加密文件操作（默认 set）"`
	Content   *string `json:"content,omitempty" sensitive:"true" desc:"加密文件内容（operation=set 时必填非空）"`
}

type aiCreateConfigSetInput struct {
	Project     string                            `json:"project" desc:"项目 ID 或唯一 identifier"`
	Name        string                            `json:"name" min:"1" desc:"配置集名称"`
	Enabled     *bool                             `json:"enabled,omitempty" default:"true" desc:"是否启用配置集"`
	PublicEnv   []aiConfigSetPublicEnvItem        `json:"publicEnv,omitempty" desc:"初始普通环境变量列表"`
	Secrets     []aiConfigSetCreateSecretItem     `json:"secrets,omitempty" desc:"初始加密环境变量列表"`
	ConfigFiles []aiConfigSetPlainFileItem        `json:"configFiles,omitempty" desc:"初始明文挂载文件列表"`
	SecretFiles []aiConfigSetCreateSecretFileItem `json:"secretFiles,omitempty" desc:"初始加密挂载文件列表"`
}

type aiConfigureConfigSetInput struct {
	Project               string                            `json:"project" desc:"项目 ID 或唯一 identifier"`
	ConfigSet             string                            `json:"configSet" desc:"运行配置集 ID（prcs_xxx）或同项目唯一名称"`
	Name                  *string                           `json:"name,omitempty" desc:"重命名配置集"`
	Enabled               *bool                             `json:"enabled,omitempty" desc:"启用或停用配置集"`
	SetPublicEnv          []aiConfigSetPublicEnvItem        `json:"setPublicEnv,omitempty" desc:"增量新增或更新普通环境变量"`
	RemovePublicEnvKeys   []string                          `json:"removePublicEnvKeys,omitempty" desc:"按变量名移除普通环境变量"`
	SecretActions         []aiConfigSetSecretActionItem     `json:"secretActions,omitempty" desc:"增量设置、随机生成或清除加密环境变量"`
	UpsertConfigFiles     []aiConfigSetPlainFileItem        `json:"upsertConfigFiles,omitempty" desc:"按非根绝对路径增量新增或更新明文文件"`
	RemoveConfigFilePaths []string                          `json:"removeConfigFilePaths,omitempty" desc:"按绝对路径移除明文文件"`
	SecretFileActions     []aiConfigSetSecretFileActionItem `json:"secretFileActions,omitempty" desc:"按非根绝对路径增量写入或清除加密文件"`
}

type aiDeleteConfigSetInput struct {
	Project   string `json:"project" desc:"项目 ID 或唯一 identifier"`
	ConfigSet string `json:"configSet" desc:"运行配置集 ID（prcs_xxx）或同项目唯一名称"`
}

func (h *Handler) registerConfigSetAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[aiListConfigSetsInput, map[string]any]{
		Name:           "delivery_list_config_sets",
		Domain:         aitool.DomainDelivery,
		Category:       "config_set",
		Summary:        "分页列出项目共享运行配置集摘要",
		Purpose:        aitool.Text("分页浏览项目空间内的共享运行配置集及其变量、挂载文件和关联环境计数摘要。", "List paginated summaries of project runtime config sets."),
		AliasesZH:      []string{"运行配置集列表", "共享配置集", "查看配置集"},
		AliasesEN:      []string{"list config sets", "runtime config sets"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionSecretReadSummary)},
		Execute:        h.execListConfigSetsAI,
	})

	aitool.Register(reg, aitool.Spec[aiGetConfigSetInput, map[string]any]{
		Name:           "delivery_get_config_set",
		Domain:         aitool.DomainDelivery,
		Category:       "config_set",
		Summary:        "查看单个项目运行配置集的公有变量、明文文件及加密键名/加密文件路径列表",
		Purpose:        aitool.Text("读取单个项目共享运行配置集的普通环境变量、明文配置文件、加密变量键名与加密文件路径清单及受影响部署目标数量。", "Inspect a single project runtime config set without exposing secret values or internal secret refs."),
		AliasesZH:      []string{"配置集详情", "查看运行配置集", "读取配置集"},
		AliasesEN:      []string{"get config set", "inspect runtime config set"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionSecretReadSummary)},
		Execute:        h.execGetConfigSetAI,
	})

	aitool.Register(reg, aitool.Spec[aiCreateConfigSetInput, map[string]any]{
		Name:           "delivery_create_config_set",
		Domain:         aitool.DomainDelivery,
		Category:       "config_set",
		Summary:        "创建项目共享运行配置集",
		Purpose:        aitool.Text("在项目内新建共享运行配置集，支持在同一事务内初始化普通环境变量、加密环境变量、明文挂载文件与加密挂载文件。", "Create a project runtime config set with optional initial environment variables and mounted files."),
		AliasesZH:      []string{"新建配置集", "创建运行配置集"},
		AliasesEN:      []string{"create config set", "new runtime config set"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionSecretUpdate)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "delivery_get_config_set",
				Family:         "resource",
				Expect:         "present",
				IDFrom:         "result.id",
				ReadIDArgument: "configSet",
			},
		},
		Preview: h.previewCreateConfigSetAI,
		Execute: h.execCreateConfigSetAI,
	})

	aitool.Register(reg, aitool.Spec[aiConfigureConfigSetInput, map[string]any]{
		Name:           "delivery_configure_config_set",
		Domain:         aitool.DomainDelivery,
		Category:       "config_set",
		Summary:        "纯增量修改项目共享运行配置集",
		Purpose:        aitool.Text("对项目运行配置集执行局部增量修改（重命名、启停、增删普通变量、设置/生成/清除加密变量、增删明文或加密挂载文件），保留所有未提及项。", "Incrementally patch a project runtime config set without overwriting omitted variables or files."),
		AliasesZH:      []string{"修改配置集", "更新运行配置集", "配置集增删变量"},
		AliasesEN:      []string{"configure config set", "update runtime config set"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionSecretUpdate)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "delivery_get_config_set",
				Family:         "resource",
				Expect:         "present",
				IDFrom:         "result.id",
				ReadIDArgument: "configSet",
			},
		},
		Preview: h.previewConfigureConfigSetAI,
		Execute: h.execConfigureConfigSetAI,
	})

	aitool.Register(reg, aitool.Spec[aiDeleteConfigSetInput, map[string]any]{
		Name:           "delivery_delete_config_set",
		Domain:         aitool.DomainDelivery,
		Category:       "config_set",
		Summary:        "异步删除项目共享运行配置集",
		Purpose:        aitool.Text("异步删除项目运行配置集并自动解绑关联部署目标上的 live 引用。", "Asynchronously delete a project runtime config set."),
		AliasesZH:      []string{"删除配置集", "移除运行配置集"},
		AliasesEN:      []string{"delete config set", "remove runtime config set"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionSecretUpdate)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "delivery_get_config_set",
				Family:          "resource",
				Expect:          "absent",
				IDFrom:          "result.id",
				ReadIDArgument:  "configSet",
				StatusField:     "deleteStatus",
				PendingStatuses: []string{"active", "deleting"},
				SuccessStatuses: []string{"deleted"},
				FailureStatuses: []string{"delete_failed"},
			},
		},
		Preview: h.previewDeleteConfigSetAI,
		Execute: h.execDeleteConfigSetAI,
	})
}

func (h *Handler) resolveConfigSetForAI(db *gorm.DB, projectID, ref string) (model.ProjectRuntimeConfigSet, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.ProjectRuntimeConfigSet{}, aitool.BadRequest("runtime_config.required", "configSet is required")
	}
	var items []model.ProjectRuntimeConfigSet
	if err := db.Where("project_id = ?", projectID).Order("created_at asc").Find(&items).Error; err != nil {
		return model.ProjectRuntimeConfigSet{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		items,
		"config_set",
		"runtime_config.not_found",
		ref,
		func(item model.ProjectRuntimeConfigSet) string { return item.ID },
		func(item model.ProjectRuntimeConfigSet) string { return item.Name },
	)
}

func decodePlainConfigFiles(raw string) []runtimeConfigFileInput {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[]" {
		return []runtimeConfigFileInput{}
	}
	var files []runtimeConfigFileInput
	if err := json.Unmarshal([]byte(trimmed), &files); err != nil {
		return []runtimeConfigFileInput{}
	}
	out := make([]runtimeConfigFileInput, 0, len(files))
	for _, item := range files {
		cleaned, err := normalizeContainerAbsPath(item.Path)
		if err != nil {
			cleaned = strings.TrimSpace(item.Path)
		}
		out = append(out, runtimeConfigFileInput{Path: cleaned, Content: item.Content})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func encodePlainConfigFiles(files []runtimeConfigFileInput) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	sorted := make([]runtimeConfigFileInput, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	data, err := json.Marshal(sorted)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func sortedSecretFilePaths(raw string) []string {
	refs := decodeSecretRefs(raw)
	paths := make([]string, 0, len(refs))
	for rawPath, ref := range refs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		cleaned, err := normalizeContainerAbsPath(rawPath)
		if err != nil {
			cleaned = strings.TrimSpace(rawPath)
		}
		if cleaned != "" {
			paths = append(paths, cleaned)
		}
	}
	sort.Strings(paths)
	return paths
}

func (h *Handler) configSetSummaryMap(set model.ProjectRuntimeConfigSet, affectedCount int) map[string]any {
	pubEnv := runtimeConfigMap(set.EnvVars)
	secKeys := runtimeSecretKeys(set.SecretRefs)
	if secKeys == nil {
		secKeys = []string{}
	}
	plainFiles := decodePlainConfigFiles(set.ConfigFiles)
	plainPaths := make([]string, 0, len(plainFiles))
	for _, f := range plainFiles {
		plainPaths = append(plainPaths, f.Path)
	}
	secFilePaths := sortedSecretFilePaths(set.SecretFiles)
	return map[string]any{
		"id":                            set.ID,
		"projectId":                     set.ProjectID,
		"name":                          set.Name,
		"enabled":                       set.Enabled,
		"publicEnvCount":                len(pubEnv),
		"secretKeys":                    secKeys,
		"configFilePaths":               plainPaths,
		"secretFilePaths":               secFilePaths,
		"secretFilesSet":                len(secFilePaths) > 0,
		"affectedDeploymentTargetCount": affectedCount,
		"deleteStatus":                  set.DeleteStatus,
		"deleteMessage":                 set.DeleteMessage,
		"createdBy":                     set.CreatedBy,
		"createdAt":                     set.CreatedAt,
		"updatedAt":                     set.UpdatedAt,
	}
}

func (h *Handler) configSetDetailMap(set model.ProjectRuntimeConfigSet, affectedCount int) map[string]any {
	pubEnv := runtimeConfigMap(set.EnvVars)
	if pubEnv == nil {
		pubEnv = map[string]string{}
	}
	secKeys := runtimeSecretKeys(set.SecretRefs)
	if secKeys == nil {
		secKeys = []string{}
	}
	plainFiles := decodePlainConfigFiles(set.ConfigFiles)
	secFilePaths := sortedSecretFilePaths(set.SecretFiles)
	return map[string]any{
		"id":                            set.ID,
		"projectId":                     set.ProjectID,
		"name":                          set.Name,
		"enabled":                       set.Enabled,
		"environmentVariables":          pubEnv,
		"configFiles":                   plainFiles,
		"secretKeys":                    secKeys,
		"secretFilePaths":               secFilePaths,
		"secretFilesSet":                len(secFilePaths) > 0,
		"affectedDeploymentTargetCount": affectedCount,
		"deleteStatus":                  set.DeleteStatus,
		"deleteMessage":                 set.DeleteMessage,
		"createdBy":                     set.CreatedBy,
		"createdAt":                     set.CreatedAt,
		"updatedAt":                     set.UpdatedAt,
	}
}

func (h *Handler) execListConfigSetsAI(tc *aitool.ToolContext, in aiListConfigSetsInput) (map[string]any, error) {
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionSecretReadSummary)
	if err != nil {
		return nil, err
	}
	sortBy := "createdAt"
	if in.SortBy != nil && strings.TrimSpace(*in.SortBy) != "" {
		sortBy = strings.TrimSpace(*in.SortBy)
	}
	sortColMap := map[string]string{"createdAt": "created_at", "name": "name"}
	sortCol, ok := sortColMap[sortBy]
	if !ok {
		return nil, aitool.BadRequest("argument_invalid", "sortBy must be createdAt or name")
	}
	sortOrder := "desc"
	if in.SortOrder != nil && strings.TrimSpace(*in.SortOrder) != "" {
		sortOrder = strings.ToLower(strings.TrimSpace(*in.SortOrder))
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, aitool.BadRequest("argument_invalid", "sortOrder must be asc or desc")
	}
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	query := h.dbWithContext(tc.Context()).Model(&model.ProjectRuntimeConfigSet{}).Where("project_id = ?", project.ID)
	if in.Search != nil && strings.TrimSpace(*in.Search) != "" {
		pattern := escapeLikePattern(*in.Search)
		query = query.Where(
			"(LOWER(name) LIKE LOWER(?) ESCAPE '\\' OR LOWER(env_vars) LIKE LOWER(?) ESCAPE '\\' OR LOWER(config_files) LIKE LOWER(?) ESCAPE '\\')",
			pattern, pattern, pattern,
		)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var sets []model.ProjectRuntimeConfigSet
	if err := query.Order(sortCol + " " + sortOrder + ", id asc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&sets).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	items := make([]map[string]any, 0, len(sets))
	for _, set := range sets {
		count := h.countRuntimeConfigSetDeploymentTargets(project.ID, set.ID, tc.Context())
		items = append(items, h.configSetSummaryMap(set, count))
	}
	out := aitool.PageEnvelope(items, page, pageSize, sortBy, sortOrder, total)
	out["projectId"] = project.ID
	return out, nil
}

func (h *Handler) execGetConfigSetAI(tc *aitool.ToolContext, in aiGetConfigSetInput) (map[string]any, error) {
	tc.SetNoStore()
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionSecretReadSummary)
	if err != nil {
		return nil, err
	}
	set, err := h.resolveConfigSetForAI(h.dbWithContext(tc.Context()), project.ID, in.ConfigSet)
	if err != nil {
		return nil, err
	}
	affectedCount := h.countRuntimeConfigSetDeploymentTargets(project.ID, set.ID, tc.Context())
	return h.configSetDetailMap(set, affectedCount), nil
}

type preparedConfigSetPatch struct {
	name             string
	enabled          bool
	publicEnv        map[string]string
	encodedPublicEnv string
	secretInput      runtimeSecretMutationInput
	hasSecretChanges bool
	nextSecretKeys   map[string]struct{}
	plainFiles       []runtimeConfigFileInput
	encodedPlain     string
	setSecretFiles   map[string]string
	clearSecretFiles []string
	nextSecretPaths  []string
	sensitiveKeys    []string
}

func validateConfigSetFileIsolation(plainFiles []runtimeConfigFileInput, secretPaths []string) error {
	plainPaths := make([]string, 0, len(plainFiles))
	seenPlain := make(map[string]bool, len(plainFiles))
	for _, f := range plainFiles {
		if seenPlain[f.Path] {
			return aitool.BadRequest("deployment.runtime_config_files_invalid", fmt.Sprintf("duplicate config file path %q", f.Path))
		}
		seenPlain[f.Path] = true
		plainPaths = append(plainPaths, f.Path)
	}
	for i := 0; i < len(plainPaths); i++ {
		for j := i + 1; j < len(plainPaths); j++ {
			if pathsHavePrefixConflict(plainPaths[i], plainPaths[j]) {
				return aitool.Conflict("deployment.runtime_config_mount_conflict",
					fmt.Sprintf("configFiles paths %q and %q have a parent-child directory prefix conflict", plainPaths[i], plainPaths[j]))
			}
		}
	}
	seenSecret := make(map[string]bool, len(secretPaths))
	for _, p := range secretPaths {
		if seenSecret[p] {
			return aitool.BadRequest("deployment.runtime_config_files_invalid", fmt.Sprintf("duplicate secret file path %q", p))
		}
		seenSecret[p] = true
		if seenPlain[p] {
			return aitool.Conflict("deployment.runtime_config_mount_conflict",
				fmt.Sprintf("file path %q cannot exist in both configFiles and secretFiles", p))
		}
	}
	for i := 0; i < len(secretPaths); i++ {
		for j := i + 1; j < len(secretPaths); j++ {
			if pathsHavePrefixConflict(secretPaths[i], secretPaths[j]) {
				return aitool.Conflict("deployment.runtime_config_mount_conflict",
					fmt.Sprintf("secretFiles paths %q and %q have a parent-child directory prefix conflict", secretPaths[i], secretPaths[j]))
			}
		}
	}
	for _, p1 := range plainPaths {
		for _, p2 := range secretPaths {
			if pathsHavePrefixConflict(p1, p2) {
				return aitool.Conflict("deployment.runtime_config_mount_conflict",
					fmt.Sprintf("configFiles path %q and secretFiles path %q have a parent-child directory prefix conflict", p1, p2))
			}
		}
	}
	return nil
}

func validatePublicEnvItems(items []aiConfigSetPublicEnvItem) (map[string]string, error) {
	if len(items) > maxRuntimeEnvironmentVariables {
		return nil, aitool.BadRequest("deployment.runtime_environment_items_invalid", "too many environment variables")
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.Key)
		if !isBuildEnvKey(key) {
			return nil, aitool.BadRequest("deployment.runtime_environment_key_invalid", fmt.Sprintf("invalid environment variable key %q", key))
		}
		if _, dup := out[key]; dup {
			return nil, aitool.BadRequest("deployment.runtime_environment_key_duplicate", fmt.Sprintf("duplicate environment variable key %q", key))
		}
		if utf8.RuneCountInString(item.Value) > maxRuntimeEnvironmentValueLength {
			return nil, aitool.BadRequest("deployment.runtime_environment_value_too_long", fmt.Sprintf("environment variable %q exceeds max length", key))
		}
		out[key] = item.Value
	}
	return out, nil
}

func buildSecretMutationFromActions(actions []aiConfigSetSecretActionItem, allowClear bool) (runtimeSecretMutationInput, []string, error) {
	input := runtimeSecretMutationInput{
		Values:   map[string]string{},
		Generate: map[string]runtimeSecretGeneration{},
	}
	if len(actions) > maxRuntimeEnvironmentVariables {
		return input, nil, aitool.BadRequest("deployment.secret_items_invalid", "too many secret actions")
	}
	seen := make(map[string]struct{}, len(actions))
	var sensitiveKeys []string
	for _, item := range actions {
		key := strings.TrimSpace(item.Key)
		if !isBuildEnvKey(key) {
			return input, nil, aitool.BadRequest("deployment.secret_key_invalid", fmt.Sprintf("invalid secret key %q", key))
		}
		if _, dup := seen[key]; dup {
			return input, nil, aitool.BadRequest("deployment.secret_update_conflict", fmt.Sprintf("duplicate secret action on key %q", key))
		}
		seen[key] = struct{}{}

		op := "set"
		if item.Operation != nil && strings.TrimSpace(*item.Operation) != "" {
			op = strings.ToLower(strings.TrimSpace(*item.Operation))
		}
		switch op {
		case "set":
			if item.Value == nil || strings.TrimSpace(*item.Value) == "" {
				return input, nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("secret key %q with operation=set requires a non-empty value", key))
			}
			if utf8.RuneCountInString(*item.Value) > maxRuntimeEnvironmentValueLength {
				return input, nil, aitool.BadRequest("deployment.secret_value_too_long", fmt.Sprintf("secret value for %q is too long", key))
			}
			if item.Encoding != nil || item.Length != nil {
				return input, nil, aitool.BadRequest("deployment.secret_update_conflict", fmt.Sprintf("secret key %q with operation=set must not specify encoding or length", key))
			}
			input.Values[key] = *item.Value
			sensitiveKeys = append(sensitiveKeys, key)
		case "generate":
			if item.Value != nil && strings.TrimSpace(*item.Value) != "" {
				return input, nil, aitool.BadRequest("deployment.secret_update_conflict", fmt.Sprintf("secret key %q with operation=generate must not specify plaintext value", key))
			}
			gen := runtimeSecretGeneration{Length: 32, Encoding: "base64"}
			if item.Length != nil {
				gen.Length = *item.Length
			}
			if item.Encoding != nil && strings.TrimSpace(*item.Encoding) != "" {
				gen.Encoding = strings.TrimSpace(*item.Encoding)
			}
			if err := secret.ValidateGeneration(gen.Length, gen.Encoding); err != nil {
				return input, nil, aitool.BadRequest("deployment.secret_generation_invalid", err.Error())
			}
			input.Generate[key] = gen
			sensitiveKeys = append(sensitiveKeys, key)
		case "clear":
			if !allowClear {
				return input, nil, aitool.BadRequest("deployment.secret_operation_invalid", "operation=clear is not valid when creating a new config set")
			}
			if (item.Value != nil && strings.TrimSpace(*item.Value) != "") || item.Encoding != nil || item.Length != nil {
				return input, nil, aitool.BadRequest("deployment.secret_update_conflict", fmt.Sprintf("secret key %q with operation=clear must not specify value or generation options", key))
			}
			input.Clear = append(input.Clear, key)
			sensitiveKeys = append(sensitiveKeys, key)
		default:
			return input, nil, aitool.BadRequest("deployment.secret_operation_invalid", fmt.Sprintf("unsupported secret operation %q", op))
		}
	}
	sort.Strings(sensitiveKeys)
	return input, sensitiveKeys, nil
}

func prepareCreateConfigSetAI(in aiCreateConfigSetInput) (preparedConfigSetPatch, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return preparedConfigSetPatch{}, aitool.BadRequest("argument_invalid", "name is required")
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	pubEnv, err := validatePublicEnvItems(in.PublicEnv)
	if err != nil {
		return preparedConfigSetPatch{}, err
	}
	actions := make([]aiConfigSetSecretActionItem, len(in.Secrets))
	for i, s := range in.Secrets {
		actions[i] = aiConfigSetSecretActionItem{
			Key:       s.Key,
			Operation: s.Operation,
			Value:     s.Value,
			Encoding:  s.Encoding,
			Length:    s.Length,
		}
	}
	secInput, sensitiveKeys, err := buildSecretMutationFromActions(actions, false)
	if err != nil {
		return preparedConfigSetPatch{}, err
	}
	nextSecKeys := make(map[string]struct{}, len(secInput.Values)+len(secInput.Generate))
	for k := range secInput.Values {
		nextSecKeys[k] = struct{}{}
	}
	for k := range secInput.Generate {
		nextSecKeys[k] = struct{}{}
	}
	for k := range pubEnv {
		if _, conflict := nextSecKeys[k]; conflict {
			return preparedConfigSetPatch{}, aitool.Conflict("deployment.runtime_environment_key_conflict",
				fmt.Sprintf("environment variable key %q cannot exist as both publicEnv and secret", k))
		}
	}

	plainFiles := make([]runtimeConfigFileInput, 0, len(in.ConfigFiles))
	for _, f := range in.ConfigFiles {
		cleaned, pathErr := normalizeContainerAbsPath(f.Path)
		if pathErr != nil {
			return preparedConfigSetPatch{}, pathErr
		}
		plainFiles = append(plainFiles, runtimeConfigFileInput{Path: cleaned, Content: f.Content})
	}
	setSecretFiles := make(map[string]string, len(in.SecretFiles))
	var secPaths []string
	for _, sf := range in.SecretFiles {
		cleaned, pathErr := normalizeContainerAbsPath(sf.Path)
		if pathErr != nil {
			return preparedConfigSetPatch{}, pathErr
		}
		if strings.TrimSpace(sf.Content) == "" {
			return preparedConfigSetPatch{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("secretFile %q content must not be empty", cleaned))
		}
		if _, dup := setSecretFiles[cleaned]; dup {
			return preparedConfigSetPatch{}, aitool.BadRequest("deployment.runtime_config_files_invalid", fmt.Sprintf("duplicate secret file path %q", cleaned))
		}
		setSecretFiles[cleaned] = sf.Content
		secPaths = append(secPaths, cleaned)
		sensitiveKeys = append(sensitiveKeys, "file:"+cleaned)
	}
	sort.Strings(secPaths)
	sort.Strings(sensitiveKeys)
	if err := validateConfigSetFileIsolation(plainFiles, secPaths); err != nil {
		return preparedConfigSetPatch{}, err
	}
	encodedPub, err := runtimeconfig.EncodeKeyValue(pubEnv)
	if err != nil {
		return preparedConfigSetPatch{}, aitool.BadRequest("deployment.runtime_config_invalid", err.Error())
	}
	encodedPlain, err := encodePlainConfigFiles(plainFiles)
	if err != nil {
		return preparedConfigSetPatch{}, aitool.NewError(http.StatusInternalServerError, "runtime_config.encode_failed", err.Error())
	}
	return preparedConfigSetPatch{
		name:             name,
		enabled:          enabled,
		publicEnv:        pubEnv,
		encodedPublicEnv: encodedPub,
		secretInput:      secInput,
		hasSecretChanges: len(secInput.Values) > 0 || len(secInput.Generate) > 0,
		nextSecretKeys:   nextSecKeys,
		plainFiles:       plainFiles,
		encodedPlain:     encodedPlain,
		setSecretFiles:   setSecretFiles,
		nextSecretPaths:  secPaths,
		sensitiveKeys:    sensitiveKeys,
	}, nil
}

func (h *Handler) previewCreateConfigSetAI(tc *aitool.ToolContext, in aiCreateConfigSetInput) (*aitool.PreviewResult, error) {
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionSecretUpdate)
	if err != nil {
		return nil, err
	}
	if err := h.ensureAIProjectMutatable(tc, project); err != nil {
		return nil, err
	}
	patch, err := prepareCreateConfigSetAI(in)
	if err != nil {
		return nil, err
	}
	if (patch.hasSecretChanges || len(patch.setSecretFiles) > 0) && !h.secrets.Available() {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "deployment.secret_store_unavailable", "secret store is unavailable")
	}
	changes := []aitool.PreviewChange{
		{Field: "name", After: patch.name, Action: "create"},
		{Field: "enabled", After: patch.enabled, Action: "create"},
	}
	if len(patch.publicEnv) > 0 {
		changes = append(changes, aitool.PreviewChange{Field: "publicEnvKeys", After: sortedMapKeys(patch.publicEnv), Action: "set"})
	}
	if len(patch.plainFiles) > 0 {
		paths := make([]string, len(patch.plainFiles))
		for i, f := range patch.plainFiles {
			paths[i] = f.Path
		}
		changes = append(changes, aitool.PreviewChange{Field: "configFiles", After: paths, Action: "set"})
	}
	if len(patch.nextSecretPaths) > 0 {
		changes = append(changes, aitool.PreviewChange{Field: "secretFiles", After: patch.nextSecretPaths, Action: "set"})
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("在项目 %s 中创建运行配置集 %s", project.Name, patch.name),
			TargetResource: "runtime_config_set",
			ProjectID:      project.ID,
			Changes:        changes,
			SensitiveKeys:  patch.sensitiveKeys,
		},
	}, nil
}

func (h *Handler) execCreateConfigSetAI(tc *aitool.ToolContext, in aiCreateConfigSetInput) (map[string]any, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionSecretUpdate)
	if err != nil {
		return nil, err
	}
	if err := h.ensureAIProjectMutatable(tc, project); err != nil {
		return nil, err
	}
	patch, err := prepareCreateConfigSetAI(in)
	if err != nil {
		return nil, err
	}
	if (patch.hasSecretChanges || len(patch.setSecretFiles) > 0) && !h.secrets.Available() {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "deployment.secret_store_unavailable", "secret store is unavailable")
	}
	preparedSecrets, err := prepareRuntimeSecretMutation(patch.secretInput)
	if err != nil {
		return nil, aitool.BadRequest("deployment.secret_generation_invalid", err.Error())
	}

	setID := id.New("prcs")
	set := model.ProjectRuntimeConfigSet{
		ID:           setID,
		ProjectID:    project.ID,
		Name:         patch.name,
		EnvVars:      patch.encodedPublicEnv,
		ConfigFiles:  patch.encodedPlain,
		SecretFiles:  "{}",
		Enabled:      patch.enabled,
		DeleteStatus: "active",
		CreatedBy:    user.ID,
	}

	if err := h.dbWithContext(tc.Context()).Transaction(func(tx *gorm.DB) error {
		secretFileRefs := make(map[string]string, len(patch.setSecretFiles))
		for filePath, content := range patch.setSecretFiles {
			ref, storeErr := h.secrets.StoreContextWithDB(tc.Context(), tx, content, user.ID, "runtime_config:"+setID+":file:"+filePath)
			if storeErr != nil {
				return storeErr
			}
			secretFileRefs[filePath] = ref
		}
		secFilesJSON, marshalErr := json.Marshal(secretFileRefs)
		if marshalErr != nil {
			return marshalErr
		}
		set.SecretFiles = string(secFilesJSON)
		if createErr := tx.Create(&set).Error; createErr != nil {
			return createErr
		}
		if patch.hasSecretChanges {
			_, encodedRefs, secErr := h.mutateRuntimeSecretsTx(tc.Context(), tx, user, preparedSecrets, projectRuntimeConfigSetSecretMutationOwner(set.ID, project.ID))
			if secErr != nil {
				return secErr
			}
			set.SecretRefs = encodedRefs
		}
		return nil
	}); err != nil {
		if errors.Is(err, errRuntimeEnvironmentKeyConflict) {
			return nil, aitool.Conflict("deployment.runtime_environment_key_conflict", "public environment variable conflicts with secret key")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "runtime_config.create_failed", err.Error())
	}

	h.auditWithContext(user.ID, "runtime_config.create", set.ID, true, set.Name, tc.Context())
	return h.configSetDetailMap(set, 0), nil
}

func prepareConfigureConfigSetAI(existing model.ProjectRuntimeConfigSet, in aiConfigureConfigSetInput) (preparedConfigSetPatch, error) {
	hasAnyChange := in.Name != nil ||
		in.Enabled != nil ||
		len(in.SetPublicEnv) > 0 ||
		len(in.RemovePublicEnvKeys) > 0 ||
		len(in.SecretActions) > 0 ||
		len(in.UpsertConfigFiles) > 0 ||
		len(in.RemoveConfigFilePaths) > 0 ||
		len(in.SecretFileActions) > 0
	if !hasAnyChange {
		return preparedConfigSetPatch{}, aitool.BadRequest("argument_invalid", "at least one config set modification field must be specified")
	}

	nextName := existing.Name
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return preparedConfigSetPatch{}, aitool.BadRequest("argument_invalid", "name must not be empty")
		}
		nextName = trimmed
	}
	nextEnabled := existing.Enabled
	if in.Enabled != nil {
		nextEnabled = *in.Enabled
	}

	nextPubEnv := runtimeConfigMap(existing.EnvVars)
	setPubMap, err := validatePublicEnvItems(in.SetPublicEnv)
	if err != nil {
		return preparedConfigSetPatch{}, err
	}
	for _, rawKey := range in.RemovePublicEnvKeys {
		key := strings.TrimSpace(rawKey)
		if !isBuildEnvKey(key) {
			return preparedConfigSetPatch{}, aitool.BadRequest("deployment.runtime_environment_key_invalid", fmt.Sprintf("invalid removePublicEnvKeys entry %q", key))
		}
		if _, both := setPubMap[key]; both {
			return preparedConfigSetPatch{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("key %q cannot appear in both setPublicEnv and removePublicEnvKeys", key))
		}
		delete(nextPubEnv, key)
	}
	for k, v := range setPubMap {
		nextPubEnv[k] = v
	}
	if len(nextPubEnv) > maxRuntimeEnvironmentVariables {
		return preparedConfigSetPatch{}, aitool.BadRequest("deployment.runtime_environment_items_invalid", "total public environment variables exceed limit")
	}

	secInput, sensitiveKeys, err := buildSecretMutationFromActions(in.SecretActions, true)
	if err != nil {
		return preparedConfigSetPatch{}, err
	}
	existingSecRefs, err := decodeRuntimeSecretRefs(existing.SecretRefs)
	if err != nil {
		return preparedConfigSetPatch{}, aitool.NewError(http.StatusInternalServerError, "deployment.secret_store_unavailable", "failed to decode existing secret references")
	}
	nextSecKeys := make(map[string]struct{}, len(existingSecRefs)+len(secInput.Values)+len(secInput.Generate))
	for k, ref := range existingSecRefs {
		if strings.TrimSpace(ref) != "" {
			nextSecKeys[k] = struct{}{}
		}
	}
	for _, k := range secInput.Clear {
		delete(nextSecKeys, k)
	}
	for k := range secInput.Values {
		nextSecKeys[k] = struct{}{}
	}
	for k := range secInput.Generate {
		nextSecKeys[k] = struct{}{}
	}
	for k := range nextPubEnv {
		if _, conflict := nextSecKeys[k]; conflict {
			return preparedConfigSetPatch{}, aitool.Conflict("deployment.runtime_environment_key_conflict",
				fmt.Sprintf("environment variable key %q conflicts between public environment variables and secrets", k))
		}
	}

	existingPlain := decodePlainConfigFiles(existing.ConfigFiles)
	plainByPath := make(map[string]string, len(existingPlain))
	for _, f := range existingPlain {
		plainByPath[f.Path] = f.Content
	}
	upsertPlainByPath := make(map[string]string, len(in.UpsertConfigFiles))
	for _, f := range in.UpsertConfigFiles {
		cleaned, pathErr := normalizeContainerAbsPath(f.Path)
		if pathErr != nil {
			return preparedConfigSetPatch{}, pathErr
		}
		if _, dup := upsertPlainByPath[cleaned]; dup {
			return preparedConfigSetPatch{}, aitool.BadRequest("deployment.runtime_config_files_invalid", fmt.Sprintf("duplicate upsertConfigFiles path %q", cleaned))
		}
		upsertPlainByPath[cleaned] = f.Content
	}
	for _, rawPath := range in.RemoveConfigFilePaths {
		cleaned, pathErr := normalizeContainerAbsPath(rawPath)
		if pathErr != nil {
			return preparedConfigSetPatch{}, pathErr
		}
		if _, both := upsertPlainByPath[cleaned]; both {
			return preparedConfigSetPatch{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("path %q cannot appear in both upsertConfigFiles and removeConfigFilePaths", cleaned))
		}
		delete(plainByPath, cleaned)
	}
	for p, c := range upsertPlainByPath {
		plainByPath[p] = c
	}
	nextPlainFiles := make([]runtimeConfigFileInput, 0, len(plainByPath))
	for p, c := range plainByPath {
		nextPlainFiles = append(nextPlainFiles, runtimeConfigFileInput{Path: p, Content: c})
	}
	sort.Slice(nextPlainFiles, func(i, j int) bool { return nextPlainFiles[i].Path < nextPlainFiles[j].Path })

	existingSecFileRefs := decodeSecretRefs(existing.SecretFiles)
	nextSecFilePathSet := make(map[string]bool, len(existingSecFileRefs))
	for rawPath, ref := range existingSecFileRefs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		if cleaned, pathErr := normalizeContainerAbsPath(rawPath); pathErr == nil {
			nextSecFilePathSet[cleaned] = true
		}
	}
	setSecFiles := make(map[string]string)
	var clearSecFiles []string
	seenSecFileAction := make(map[string]bool, len(in.SecretFileActions))
	for _, act := range in.SecretFileActions {
		cleaned, pathErr := normalizeContainerAbsPath(act.Path)
		if pathErr != nil {
			return preparedConfigSetPatch{}, pathErr
		}
		if seenSecFileAction[cleaned] {
			return preparedConfigSetPatch{}, aitool.BadRequest("deployment.runtime_config_files_invalid", fmt.Sprintf("duplicate secretFileActions path %q", cleaned))
		}
		seenSecFileAction[cleaned] = true

		op := "set"
		if act.Operation != nil && strings.TrimSpace(*act.Operation) != "" {
			op = strings.ToLower(strings.TrimSpace(*act.Operation))
		}
		switch op {
		case "set":
			if act.Content == nil || strings.TrimSpace(*act.Content) == "" {
				return preparedConfigSetPatch{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("secretFile %q with operation=set requires non-empty content", cleaned))
			}
			setSecFiles[cleaned] = *act.Content
			nextSecFilePathSet[cleaned] = true
			sensitiveKeys = append(sensitiveKeys, "file:"+cleaned)
		case "clear":
			if act.Content != nil && strings.TrimSpace(*act.Content) != "" {
				return preparedConfigSetPatch{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("secretFile %q with operation=clear must not include content", cleaned))
			}
			clearSecFiles = append(clearSecFiles, cleaned)
			delete(nextSecFilePathSet, cleaned)
			sensitiveKeys = append(sensitiveKeys, "file:"+cleaned)
		default:
			return preparedConfigSetPatch{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("unsupported secretFile operation %q", op))
		}
	}
	nextSecPaths := make([]string, 0, len(nextSecFilePathSet))
	for p := range nextSecFilePathSet {
		nextSecPaths = append(nextSecPaths, p)
	}
	sort.Strings(nextSecPaths)
	sort.Strings(clearSecFiles)
	sort.Strings(sensitiveKeys)

	if err := validateConfigSetFileIsolation(nextPlainFiles, nextSecPaths); err != nil {
		return preparedConfigSetPatch{}, err
	}
	encodedPub, err := runtimeconfig.EncodeKeyValue(nextPubEnv)
	if err != nil {
		return preparedConfigSetPatch{}, aitool.BadRequest("deployment.runtime_config_invalid", err.Error())
	}
	encodedPlain, err := encodePlainConfigFiles(nextPlainFiles)
	if err != nil {
		return preparedConfigSetPatch{}, aitool.NewError(http.StatusInternalServerError, "runtime_config.encode_failed", err.Error())
	}

	return preparedConfigSetPatch{
		name:             nextName,
		enabled:          nextEnabled,
		publicEnv:        nextPubEnv,
		encodedPublicEnv: encodedPub,
		secretInput:      secInput,
		hasSecretChanges: len(secInput.Values) > 0 || len(secInput.Generate) > 0 || len(secInput.Clear) > 0,
		nextSecretKeys:   nextSecKeys,
		plainFiles:       nextPlainFiles,
		encodedPlain:     encodedPlain,
		setSecretFiles:   setSecFiles,
		clearSecretFiles: clearSecFiles,
		nextSecretPaths:  nextSecPaths,
		sensitiveKeys:    sensitiveKeys,
	}, nil
}

// validateLiveBoundTargetsForConfigSet enforces §1.3 Tool 15:
// When the resulting config set is enabled, verify every active DeploymentTarget
// referencing it via mode=="live" against outbound ServiceBinding env keys and
// heterogeneous/prefix/volume mount path conflicts.
func (h *Handler) validateLiveBoundTargetsForConfigSet(
	db *gorm.DB,
	projectID string,
	setID string,
	patch preparedConfigSetPatch,
) (int, error) {
	var targets []model.DeploymentTarget
	if err := db.Where("project_id = ? AND delete_status IN ?", projectID, []string{"", "active", "delete_failed"}).
		Find(&targets).Error; err != nil {
		return 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var liveBoundTargets []model.DeploymentTarget
	otherLiveSetIDs := make(map[string]bool)
	for _, t := range targets {
		refs := model.DecodeDeploymentRuntimeConfigRefs(t.RuntimeConfigRefs)
		hasLiveRef := false
		for _, r := range refs {
			if r.Mode == model.RuntimeConfigRefModeLive && r.SetID == setID {
				hasLiveRef = true
			} else if r.Mode == model.RuntimeConfigRefModeLive && r.SetID != "" {
				otherLiveSetIDs[r.SetID] = true
			}
		}
		if hasLiveRef {
			liveBoundTargets = append(liveBoundTargets, t)
		}
	}
	if !patch.enabled || len(liveBoundTargets) == 0 {
		return len(liveBoundTargets), nil
	}

	otherSetsByID := make(map[string]model.ProjectRuntimeConfigSet, len(otherLiveSetIDs))
	if len(otherLiveSetIDs) > 0 {
		ids := make([]string, 0, len(otherLiveSetIDs))
		for idVal := range otherLiveSetIDs {
			ids = append(ids, idVal)
		}
		var otherSets []model.ProjectRuntimeConfigSet
		if err := db.Where("project_id = ? AND id IN ?", projectID, ids).Find(&otherSets).Error; err != nil {
			return 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		for _, s := range otherSets {
			if s.Enabled && resourceCanMutateDuringDelete(s.DeleteStatus) {
				otherSetsByID[s.ID] = s
			}
		}
	}

	targetIDs := make([]string, len(liveBoundTargets))
	for i, t := range liveBoundTargets {
		targetIDs[i] = t.ID
	}

	var bindings []model.ServiceBinding
	if err := db.Where("project_id = ? AND source_deployment_target_id IN ? AND enabled = ?", projectID, targetIDs, true).
		Find(&bindings).Error; err != nil {
		return 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	bindingsByTarget := make(map[string][]model.ServiceBinding)
	for _, b := range bindings {
		bindingsByTarget[b.SourceDeploymentTargetID] = append(bindingsByTarget[b.SourceDeploymentTargetID], b)
	}

	var mounts []model.DeploymentVolumeMount
	if err := db.Where("project_id = ? AND deployment_target_id IN ? AND activation_state <> ?",
		projectID, targetIDs, model.DeploymentVolumeActivationReleasePending).
		Find(&mounts).Error; err != nil {
		return 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	mountsByTarget := make(map[string][]model.DeploymentVolumeMount)
	for _, m := range mounts {
		mountsByTarget[m.DeploymentTargetID] = append(mountsByTarget[m.DeploymentTargetID], m)
	}

	for _, target := range liveBoundTargets {
		effectiveEnvKeys := make(map[string]bool)
		effectiveConfigPaths := make(map[string]bool)
		effectiveSecretPaths := make(map[string]bool)

		addLayer := func(pubEnv map[string]string, secKeys []string, plainFiles []runtimeConfigFileInput, secPaths []string) {
			for k := range pubEnv {
				effectiveEnvKeys[k] = true
			}
			for _, k := range secKeys {
				effectiveEnvKeys[k] = true
			}
			for _, f := range plainFiles {
				if f.Path != "" {
					effectiveConfigPaths[f.Path] = true
				}
			}
			for _, p := range secPaths {
				if p != "" {
					effectiveSecretPaths[p] = true
				}
			}
		}

		for _, ref := range model.DecodeDeploymentRuntimeConfigRefs(target.RuntimeConfigRefs) {
			switch {
			case ref.Mode == model.RuntimeConfigRefModeLive && ref.SetID == setID:
				secKeys := make([]string, 0, len(patch.nextSecretKeys))
				for k := range patch.nextSecretKeys {
					secKeys = append(secKeys, k)
				}
				addLayer(patch.publicEnv, secKeys, patch.plainFiles, patch.nextSecretPaths)
			case ref.Mode == model.RuntimeConfigRefModeLive:
				if other, ok := otherSetsByID[ref.SetID]; ok {
					addLayer(
						runtimeConfigMap(other.EnvVars),
						runtimeSecretKeys(other.SecretRefs),
						decodePlainConfigFiles(other.ConfigFiles),
						sortedSecretFilePaths(other.SecretFiles),
					)
				}
			case ref.Mode == model.RuntimeConfigRefModeSnapshot && ref.Snapshot != nil && ref.Snapshot.Enabled:
				addLayer(
					runtimeConfigMap(ref.Snapshot.EnvVars),
					runtimeSecretKeys(ref.Snapshot.SecretRefs),
					decodePlainConfigFiles(ref.Snapshot.ConfigFiles),
					sortedSecretFilePaths(ref.Snapshot.SecretFiles),
				)
			}
		}

		addLayer(
			runtimeConfigMap(target.EnvVars),
			runtimeSecretKeys(target.SecretRefs),
			decodePlainConfigFiles(target.ConfigFiles),
			sortedSecretFilePaths(target.SecretFiles),
		)

		// (1) Outbound active ServiceBinding env key collision -> 409 service_binding.env_conflict
		for _, sb := range bindingsByTarget[target.ID] {
			injected := []string{
				strings.TrimSpace(sb.URLEnvVar),
				strings.TrimSpace(sb.HostEnvVar),
				strings.TrimSpace(sb.PortEnvVar),
			}
			for _, sm := range model.DecodeSecretMap(sb.SecretMap) {
				injected = append(injected, strings.TrimSpace(sm.SourceEnvVar))
			}
			for _, envKey := range injected {
				if envKey != "" && effectiveEnvKeys[envKey] {
					return 0, aitool.Conflict("service_binding.env_conflict",
						fmt.Sprintf("environment variable %q conflicts with active outbound ServiceBinding %s on deployment target %s (%s)",
							envKey, sb.ID, target.Name, target.Stage))
				}
			}
		}

		// (2a) Heterogeneous configFiles <-> secretFiles same-path conflict -> 409
		allMountedFilePaths := make([]string, 0, len(effectiveConfigPaths)+len(effectiveSecretPaths))
		for p := range effectiveConfigPaths {
			if effectiveSecretPaths[p] {
				return 0, aitool.Conflict("deployment.runtime_config_mount_conflict",
					fmt.Sprintf("path %q conflicts between configFiles and secretFiles on deployment target %s (%s)",
						p, target.Name, target.Stage))
			}
			allMountedFilePaths = append(allMountedFilePaths, p)
		}
		for p := range effectiveSecretPaths {
			allMountedFilePaths = append(allMountedFilePaths, p)
		}
		sort.Strings(allMountedFilePaths)

		// (2b) Any two mounted files parent-child prefix conflict -> 409
		for i := 0; i < len(allMountedFilePaths); i++ {
			for j := i + 1; j < len(allMountedFilePaths); j++ {
				if pathsHavePrefixConflict(allMountedFilePaths[i], allMountedFilePaths[j]) {
					return 0, aitool.Conflict("deployment.runtime_config_mount_conflict",
						fmt.Sprintf("mounted file paths %q and %q have a parent-child prefix conflict on deployment target %s (%s)",
							allMountedFilePaths[i], allMountedFilePaths[j], target.Name, target.Stage))
				}
			}
		}

		// (2c) Mounted file vs target dataVolumes.mountPath same-path or prefix conflict -> 409
		for _, m := range mountsByTarget[target.ID] {
			if m.MountPath == nil || strings.TrimSpace(*m.MountPath) == "" {
				continue
			}
			volPath, pathErr := normalizeContainerAbsPath(*m.MountPath)
			if pathErr != nil {
				volPath = strings.TrimSpace(*m.MountPath)
			}
			for _, fp := range allMountedFilePaths {
				if fp == volPath || pathsHavePrefixConflict(fp, volPath) {
					return 0, aitool.Conflict("deployment.runtime_config_mount_conflict",
						fmt.Sprintf("mounted file path %q conflicts with volume mount %q (%s) on deployment target %s (%s)",
							fp, volPath, m.LogicalName, target.Name, target.Stage))
				}
			}
		}
	}

	return len(liveBoundTargets), nil
}

func (h *Handler) previewConfigureConfigSetAI(tc *aitool.ToolContext, in aiConfigureConfigSetInput) (*aitool.PreviewResult, error) {
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionSecretUpdate)
	if err != nil {
		return nil, err
	}
	if err := h.ensureAIProjectMutatable(tc, project); err != nil {
		return nil, err
	}
	db := h.dbWithContext(tc.Context())
	existing, err := h.resolveConfigSetForAI(db, project.ID, in.ConfigSet)
	if err != nil {
		return nil, err
	}
	if err := h.ensureAIConfigSetMutatable(tc, existing); err != nil {
		return nil, err
	}
	patch, err := prepareConfigureConfigSetAI(existing, in)
	if err != nil {
		return nil, err
	}
	if (patch.hasSecretChanges || len(patch.setSecretFiles) > 0 || len(patch.clearSecretFiles) > 0) && !h.secrets.Available() {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "deployment.secret_store_unavailable", "secret store is unavailable")
	}
	affectedCount, err := h.validateLiveBoundTargetsForConfigSet(db, project.ID, existing.ID, patch)
	if err != nil {
		return nil, err
	}

	var changes []aitool.PreviewChange
	if patch.name != existing.Name {
		changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: patch.name, Action: "update"})
	}
	if patch.enabled != existing.Enabled {
		changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: patch.enabled, Action: "update"})
	}
	if len(in.SetPublicEnv) > 0 || len(in.RemovePublicEnvKeys) > 0 {
		changes = append(changes, aitool.PreviewChange{
			Field:  "publicEnvKeys",
			Before: sortedMapKeys(runtimeConfigMap(existing.EnvVars)),
			After:  sortedMapKeys(patch.publicEnv),
			Action: "patch",
		})
	}
	if len(in.UpsertConfigFiles) > 0 || len(in.RemoveConfigFilePaths) > 0 {
		beforeFiles := decodePlainConfigFiles(existing.ConfigFiles)
		beforePaths := make([]string, len(beforeFiles))
		for i, f := range beforeFiles {
			beforePaths[i] = f.Path
		}
		afterPaths := make([]string, len(patch.plainFiles))
		for i, f := range patch.plainFiles {
			afterPaths[i] = f.Path
		}
		changes = append(changes, aitool.PreviewChange{Field: "configFiles", Before: beforePaths, After: afterPaths, Action: "patch"})
	}
	if len(in.SecretFileActions) > 0 {
		changes = append(changes, aitool.PreviewChange{
			Field:  "secretFilePaths",
			Before: sortedSecretFilePaths(existing.SecretFiles),
			After:  patch.nextSecretPaths,
			Action: "patch",
		})
	}

	var impact []string
	if affectedCount > 0 {
		impact = append(impact, fmt.Sprintf("%d 个 live 绑定部署目标将在下次发布（Release）时生效", affectedCount))
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("增量修改运行配置集 %s（%s）", existing.Name, existing.ID),
			TargetResource: "runtime_config_set",
			TargetID:       existing.ID,
			ProjectID:      project.ID,
			Changes:        changes,
			SensitiveKeys:  patch.sensitiveKeys,
			Impact:         impact,
		},
	}, nil
}

func (h *Handler) execConfigureConfigSetAI(tc *aitool.ToolContext, in aiConfigureConfigSetInput) (map[string]any, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionSecretUpdate)
	if err != nil {
		return nil, err
	}
	if err := h.ensureAIProjectMutatable(tc, project); err != nil {
		return nil, err
	}
	db := h.dbWithContext(tc.Context())
	resolved, err := h.resolveConfigSetForAI(db, project.ID, in.ConfigSet)
	if err != nil {
		return nil, err
	}
	if err := h.ensureAIConfigSetMutatable(tc, resolved); err != nil {
		return nil, err
	}

	var updated model.ProjectRuntimeConfigSet
	var affectedCount int
	if err := db.Transaction(func(tx *gorm.DB) error {
		var locked model.ProjectRuntimeConfigSet
		if lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&locked, "id = ? AND project_id = ?", resolved.ID, project.ID).Error; lockErr != nil {
			if errors.Is(lockErr, gorm.ErrRecordNotFound) {
				return aitool.NotFound("runtime_config.not_found", "runtime config set not found")
			}
			return lockErr
		}
		if !resourceCanMutateDuringDelete(locked.DeleteStatus) {
			return aitool.Conflict("runtime_config.delete_in_progress", "runtime config set is being deleted")
		}

		patch, patchErr := prepareConfigureConfigSetAI(locked, in)
		if patchErr != nil {
			return patchErr
		}
		if (patch.hasSecretChanges || len(patch.setSecretFiles) > 0 || len(patch.clearSecretFiles) > 0) && !h.secrets.Available() {
			return aitool.NewError(http.StatusServiceUnavailable, "deployment.secret_store_unavailable", "secret store is unavailable")
		}
		var valErr error
		affectedCount, valErr = h.validateLiveBoundTargetsForConfigSet(tx, project.ID, locked.ID, patch)
		if valErr != nil {
			return valErr
		}

		// Update secret files incrementally while cleaning up superseded/cleared secret_values.
		prevSecFileRefs := decodeSecretRefs(locked.SecretFiles)
		nextSecFileRefs := make(map[string]string, len(prevSecFileRefs))
		for rawPath, ref := range prevSecFileRefs {
			if strings.TrimSpace(ref) == "" {
				continue
			}
			cleaned, pathErr := normalizeContainerAbsPath(rawPath)
			if pathErr != nil {
				cleaned = strings.TrimSpace(rawPath)
			}
			nextSecFileRefs[cleaned] = ref
		}
		for _, clearPath := range patch.clearSecretFiles {
			if oldRef := strings.TrimSpace(nextSecFileRefs[clearPath]); oldRef != "" {
				if delErr := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, oldRef, "runtime_config:"+locked.ID+":file:"+clearPath); delErr != nil {
					return delErr
				}
			}
			delete(nextSecFileRefs, clearPath)
		}
		for filePath, content := range patch.setSecretFiles {
			oldRef := strings.TrimSpace(nextSecFileRefs[filePath])
			newRef, storeErr := h.secrets.StoreContextWithDB(tc.Context(), tx, content, user.ID, "runtime_config:"+locked.ID+":file:"+filePath)
			if storeErr != nil {
				return storeErr
			}
			if oldRef != "" && oldRef != newRef {
				if delErr := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, oldRef, "runtime_config:"+locked.ID+":file:"+filePath); delErr != nil {
					return delErr
				}
			}
			nextSecFileRefs[filePath] = newRef
		}
		encodedSecFiles, marshalErr := json.Marshal(nextSecFileRefs)
		if marshalErr != nil {
			return marshalErr
		}

		locked.Name = patch.name
		locked.Enabled = patch.enabled
		locked.EnvVars = patch.encodedPublicEnv
		locked.ConfigFiles = patch.encodedPlain
		locked.SecretFiles = string(encodedSecFiles)
		if saveErr := tx.Save(&locked).Error; saveErr != nil {
			return saveErr
		}

		if patch.hasSecretChanges {
			preparedSecrets, genErr := prepareRuntimeSecretMutation(patch.secretInput)
			if genErr != nil {
				return aitool.BadRequest("deployment.secret_generation_invalid", genErr.Error())
			}
			_, encodedRefs, secErr := h.mutateRuntimeSecretsTx(tc.Context(), tx, user, preparedSecrets, projectRuntimeConfigSetSecretMutationOwner(locked.ID, project.ID))
			if secErr != nil {
				return secErr
			}
			locked.SecretRefs = encodedRefs
		}
		updated = locked
		return nil
	}); err != nil {
		if te, ok := aitool.AsToolError(err); ok {
			return nil, te
		}
		if errors.Is(err, errRuntimeEnvironmentKeyConflict) {
			return nil, aitool.Conflict("deployment.runtime_environment_key_conflict", "public environment variable conflicts with secret key")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "runtime_config.update_failed", err.Error())
	}

	h.auditWithContext(user.ID, "runtime_config.update", updated.ID, true, updated.Name, tc.Context())
	resp := h.configSetDetailMap(updated, affectedCount)
	resp["requiresRelease"] = true
	return resp, nil
}

func (h *Handler) listLiveAffectedTargetsForConfigSet(db *gorm.DB, projectID, setID string) ([]model.DeploymentTarget, error) {
	var targets []model.DeploymentTarget
	if err := db.Where("project_id = ?", projectID).Order("created_at asc").Find(&targets).Error; err != nil {
		return nil, err
	}
	var affected []model.DeploymentTarget
	for _, t := range targets {
		for _, liveID := range model.DeploymentRuntimeConfigLiveSetIDs(model.DecodeDeploymentRuntimeConfigRefs(t.RuntimeConfigRefs)) {
			if liveID == setID {
				affected = append(affected, t)
				break
			}
		}
	}
	return affected, nil
}

func (h *Handler) previewDeleteConfigSetAI(tc *aitool.ToolContext, in aiDeleteConfigSetInput) (*aitool.PreviewResult, error) {
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionSecretUpdate)
	if err != nil {
		return nil, err
	}
	if err := h.ensureAIProjectMutatable(tc, project); err != nil {
		return nil, err
	}
	db := h.dbWithContext(tc.Context())
	set, err := h.resolveConfigSetForAI(db, project.ID, in.ConfigSet)
	if err != nil {
		return nil, err
	}
	if !h.deleteStatusCanStart(set.DeleteStatus) {
		return nil, aitool.Conflict("runtime_config.delete_in_progress", "runtime config set deletion is already in progress")
	}
	affected, err := h.listLiveAffectedTargetsForConfigSet(db, project.ID, set.ID)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	targetLabels := make([]string, 0, len(affected))
	for _, t := range affected {
		targetLabels = append(targetLabels, fmt.Sprintf("%s (%s, %s)", t.Name, t.Stage, t.ID))
	}
	changes := []aitool.PreviewChange{
		{Field: "deleteStatus", Before: set.DeleteStatus, After: "deleting", Action: "delete"},
		{Field: "affectedDeploymentTargetCount", After: len(affected), Action: "detach_live_refs"},
	}
	var impact []string
	if len(targetLabels) > 0 {
		impact = append(impact, fmt.Sprintf("将自动解绑 %d 个部署目标的 live 引用：%s", len(targetLabels), strings.Join(targetLabels, ", ")))
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("异步删除运行配置集 %s（%s）", set.Name, set.ID),
			TargetResource: "runtime_config_set",
			TargetID:       set.ID,
			ProjectID:      project.ID,
			Changes:        changes,
			Impact:         impact,
		},
	}, nil
}

func (h *Handler) execDeleteConfigSetAI(tc *aitool.ToolContext, in aiDeleteConfigSetInput) (map[string]any, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionSecretUpdate)
	if err != nil {
		return nil, err
	}
	if err := h.ensureAIProjectMutatable(tc, project); err != nil {
		return nil, err
	}
	db := h.dbWithContext(tc.Context())
	set, err := h.resolveConfigSetForAI(db, project.ID, in.ConfigSet)
	if err != nil {
		return nil, err
	}
	if !h.deleteStatusCanStart(set.DeleteStatus) {
		return nil, aitool.Conflict("runtime_config.delete_in_progress", "runtime config set deletion is already in progress")
	}
	affectedCount := h.countRuntimeConfigSetDeploymentTargets(project.ID, set.ID, tc.Context())
	if err := h.markResourceDeleting(db, &model.ProjectRuntimeConfigSet{}, set.ID); err != nil {
		if h.host.ResourceDeleteAlreadyStarted(err) {
			return nil, aitool.Conflict("runtime_config.delete_in_progress", "runtime config set deletion is already in progress")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "runtime_config.delete_failed", err.Error())
	}
	if !h.enqueueResourceCleanup(tc.Context(), "runtime_config", set.ID, set.ProjectID, user.ID) {
		_ = h.markResourceDeleteFailed(db, &model.ProjectRuntimeConfigSet{}, set.ID, "资源清理任务投递失败，请稍后重试")
		h.auditWithContext(user.ID, "runtime_config.delete", set.ID, false, "cleanup_enqueue_failed", tc.Context())
		return nil, aitool.NewError(http.StatusServiceUnavailable, "runtime_config.cleanup_enqueue_failed", "failed to enqueue runtime config cleanup")
	}
	h.auditWithContext(user.ID, "runtime_config.delete", set.ID, true, "cleanup_queued", tc.Context())
	return map[string]any{
		"id":                            set.ID,
		"projectId":                     project.ID,
		"deleteStatus":                  "deleting",
		"affectedDeploymentTargetCount": affectedCount,
	}, nil
}
