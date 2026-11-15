package applicationapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type platformAppTemplateWriteInput struct {
	Slug               string `json:"slug"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Category           string `json:"category"`
	Icon               string `json:"icon"`
	Version            string `json:"version"`
	Enabled            *bool  `json:"enabled"`
	Featured           bool   `json:"featured"`
	PopularityWeight   int    `json:"popularityWeight"`
	OfficialWebsite    string `json:"officialWebsite"`
	OfficialRepository string `json:"officialRepository"`
	appstore.Workload
	Components []appstore.Component       `json:"components"`
	Values     []appstore.ValueDefinition `json:"values"`
}

func (input platformAppTemplateWriteInput) toTemplate(recordID string) appstore.Template {
	return appstore.Template{
		ID:                 recordID,
		Slug:               input.Slug,
		Name:               input.Name,
		Description:        input.Description,
		Category:           input.Category,
		Kind:               "application",
		Source:             appstore.TemplateSourcePlatform,
		Icon:               input.Icon,
		OfficialWebsite:    input.OfficialWebsite,
		OfficialRepository: input.OfficialRepository,
		PopularityWeight:   input.PopularityWeight,
		Featured:           input.Featured,
		Version:            input.Version,
		Workload:           input.Workload,
		Components:         input.Components,
		Values:             input.Values,
	}
}

type platformAppTemplateImportInput struct {
	Overwrite bool                            `json:"overwrite"`
	Bundle    appstore.PlatformTemplateBundle `json:"bundle"`
}

type platformAppTemplateComponentDetail struct {
	Key              string                `json:"key"`
	Name             string                `json:"name"`
	Image            string                `json:"image"`
	ServicePort      int                   `json:"servicePort"`
	DefaultReplicas  int                   `json:"defaultReplicas"`
	DefaultCPU       string                `json:"defaultCPU"`
	DefaultMemory    string                `json:"defaultMemory"`
	ContainerCommand string                `json:"containerCommand"`
	ContainerArgs    string                `json:"containerArgs"`
	FSGroup          string                `json:"fsGroup"`
	DataVolumes      []appstore.DataVolume `json:"dataVolumes"`
	Env              map[string]string     `json:"env"`
	SecretEnv        map[string]string     `json:"secretEnv"`
	ConfigFiles      []appstore.ConfigFile `json:"configFiles"`
	SecretFiles      []appstore.ConfigFile `json:"secretFiles"`
	Dependencies     []string              `json:"dependencies"`
}

type platformAppTemplateResponse struct {
	ID                 string                               `json:"id"`
	Slug               string                               `json:"slug"`
	Name               string                               `json:"name"`
	Description        string                               `json:"description"`
	Category           string                               `json:"category"`
	Icon               string                               `json:"icon"`
	Version            string                               `json:"version"`
	IsMultiComponent   bool                                 `json:"isMultiComponent"`
	Enabled            bool                                 `json:"enabled"`
	Featured           bool                                 `json:"featured"`
	PopularityWeight   int                                  `json:"popularityWeight"`
	OfficialWebsite    string                               `json:"officialWebsite"`
	OfficialRepository string                               `json:"officialRepository"`
	Image              string                               `json:"image"`
	ServicePort        int                                  `json:"servicePort"`
	DefaultReplicas    int                                  `json:"defaultReplicas"`
	DefaultCPU         string                               `json:"defaultCPU"`
	DefaultMemory      string                               `json:"defaultMemory"`
	ContainerCommand   string                               `json:"containerCommand"`
	ContainerArgs      string                               `json:"containerArgs"`
	FSGroup            string                               `json:"fsGroup"`
	DataVolumes        []appstore.DataVolume                `json:"dataVolumes"`
	Env                map[string]string                    `json:"env"`
	SecretEnv          map[string]string                    `json:"secretEnv"`
	ConfigFiles        []appstore.ConfigFile                `json:"configFiles"`
	SecretFiles        []appstore.ConfigFile                `json:"secretFiles"`
	Components         []platformAppTemplateComponentDetail `json:"components"`
	Values             []appTemplateValueResponse           `json:"values"`
	CreatedBy          string                               `json:"createdBy"`
	CreatedAt          time.Time                            `json:"createdAt"`
	UpdatedAt          time.Time                            `json:"updatedAt"`
}

func (h *Handlers) requirePlatformTemplateAdmin(ctx *gin.Context) (model.User, bool) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return model.User{}, false
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorCode(ctx, http.StatusForbidden, "config.admin.required", "需要平台管理员权限")
		return model.User{}, false
	}
	return user, true
}

func writePlatformTemplateError(ctx *gin.Context, err error) {
	var validationErr *appstore.PlatformTemplateValidationError
	if errors.As(err, &validationErr) {
		code := strings.TrimSpace(validationErr.Code)
		if code == "" {
			code = "app_template.invalid"
		}
		writeErrorCode(ctx, http.StatusBadRequest, code, validationErr.Message)
		return
	}
	writeError(ctx, http.StatusInternalServerError, err.Error())
}

func nonNilStringMap(input map[string]string) map[string]string {
	if input == nil {
		return map[string]string{}
	}
	return input
}

func nonNilConfigFiles(files []appstore.ConfigFile) []appstore.ConfigFile {
	if files == nil {
		return []appstore.ConfigFile{}
	}
	return files
}

func platformAppTemplateResponseFrom(record model.PlatformAppTemplate) (platformAppTemplateResponse, error) {
	tpl, err := appstore.TemplateFromPlatformModel(record)
	if err != nil {
		return platformAppTemplateResponse{}, err
	}
	components := make([]platformAppTemplateComponentDetail, 0, len(tpl.Components))
	for _, comp := range tpl.Components {
		deps := appstore.ComponentDependencies(comp.Workload)
		if deps == nil {
			deps = []string{}
		}
		components = append(components, platformAppTemplateComponentDetail{
			Key:              comp.Key,
			Name:             comp.Name,
			Image:            comp.Image,
			ServicePort:      comp.ServicePort,
			DefaultReplicas:  comp.DefaultReplicas,
			DefaultCPU:       comp.DefaultCPU,
			DefaultMemory:    comp.DefaultMemory,
			ContainerCommand: comp.ContainerCommand,
			ContainerArgs:    comp.ContainerArgs,
			FSGroup:          comp.FSGroup,
			DataVolumes:      nonNilDataVolumes(comp.DataVolumes),
			Env:              nonNilStringMap(comp.Env),
			SecretEnv:        nonNilStringMap(comp.SecretEnv),
			ConfigFiles:      nonNilConfigFiles(comp.ConfigFiles),
			SecretFiles:      nonNilConfigFiles(comp.SecretFiles),
			Dependencies:     deps,
		})
	}
	values := make([]appTemplateValueResponse, 0, len(tpl.Values))
	for _, def := range tpl.Values {
		defaultVal := def.Default
		if def.Secret {
			defaultVal = ""
		}
		values = append(values, appTemplateValueResponse{
			Key:          def.Key,
			Label:        def.Label,
			Description:  def.Description,
			Default:      defaultVal,
			Required:     def.Required,
			Secret:       def.Secret,
			AutoGenerate: def.AutoGenerate,
		})
	}
	return platformAppTemplateResponse{
		ID:                 record.ID,
		Slug:               record.Slug,
		Name:               record.Name,
		Description:        record.Description,
		Category:           record.Category,
		Icon:               record.Icon,
		Version:            record.Version,
		IsMultiComponent:   record.IsMultiComponent,
		Enabled:            record.Enabled,
		Featured:           record.Featured,
		PopularityWeight:   record.PopularityWeight,
		OfficialWebsite:    tpl.OfficialWebsite,
		OfficialRepository: tpl.OfficialRepository,
		Image:              tpl.Image,
		ServicePort:        tpl.ServicePort,
		DefaultReplicas:    tpl.DefaultReplicas,
		DefaultCPU:         tpl.DefaultCPU,
		DefaultMemory:      tpl.DefaultMemory,
		ContainerCommand:   tpl.ContainerCommand,
		ContainerArgs:      tpl.ContainerArgs,
		FSGroup:            tpl.FSGroup,
		DataVolumes:        nonNilDataVolumes(tpl.DataVolumes),
		Env:                nonNilStringMap(tpl.Env),
		SecretEnv:          nonNilStringMap(tpl.SecretEnv),
		ConfigFiles:        nonNilConfigFiles(tpl.ConfigFiles),
		SecretFiles:        nonNilConfigFiles(tpl.SecretFiles),
		Components:         components,
		Values:             values,
		CreatedBy:          record.CreatedBy,
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
	}, nil
}

func (h *Handlers) ListPlatformAppTemplates(ctx *gin.Context) {
	if _, ok := h.requirePlatformTemplateAdmin(ctx); !ok {
		return
	}
	db := h.dbFor(ctx)
	if db == nil {
		ctx.JSON(http.StatusOK, []platformAppTemplateResponse{})
		return
	}
	var records []model.PlatformAppTemplate
	query := db.Order("popularity_weight DESC, slug ASC")
	if category := strings.TrimSpace(ctx.Query("category")); category != "" {
		query = query.Where("LOWER(category) = ?", strings.ToLower(category))
	}
	if enabledParam := strings.TrimSpace(ctx.Query("enabled")); enabledParam != "" {
		switch strings.ToLower(enabledParam) {
		case "true", "1":
			query = query.Where("enabled = ?", true)
		case "false", "0":
			query = query.Where("enabled = ?", false)
		}
	}
	if err := query.Find(&records).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	keyword := strings.ToLower(strings.TrimSpace(ctx.Query("query")))
	items := make([]platformAppTemplateResponse, 0, len(records))
	for _, record := range records {
		item, err := platformAppTemplateResponseFrom(record)
		if err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		if keyword != "" {
			tpl, _ := appstore.TemplateFromPlatformModel(record)
			if !templateMatchesQuery(tpl, keyword) {
				continue
			}
		}
		items = append(items, item)
	}
	ctx.JSON(http.StatusOK, items)
}

func (h *Handlers) CreatePlatformAppTemplate(ctx *gin.Context) {
	user, ok := h.requirePlatformTemplateAdmin(ctx)
	if !ok {
		return
	}
	var input platformAppTemplateWriteInput
	if !bindJSON(ctx, &input) {
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	recordID := id.New("ptpl")
	record, _, err := appstore.ModelFromPlatformTemplate(recordID, user.ID, enabled, input.toTemplate(recordID))
	if err != nil {
		writePlatformTemplateError(ctx, err)
		return
	}
	reserved, err := appstore.IsBuiltinSlugOrID(record.Slug)
	if err != nil {
		writeTemplateCatalogUnavailable(ctx, err)
		return
	}
	if reserved {
		writeErrorCode(ctx, http.StatusConflict, "app_template.slug_reserved", fmt.Sprintf("模板标识 %s 已被内置模板占用", record.Slug))
		return
	}

	db := h.dbFor(ctx)
	var existingCount int64
	if err := db.Model(&model.PlatformAppTemplate{}).Where("slug = ?", record.Slug).Count(&existingCount).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if existingCount > 0 {
		writeErrorCode(ctx, http.StatusConflict, "app_template.slug_exists", fmt.Sprintf("模板标识 %s 已存在", record.Slug))
		return
	}
	if err := db.Create(&record).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "platform_app_template.create", record.ID, true, fmt.Sprintf("slug=%s enabled=%t", record.Slug, record.Enabled), ctx.Request.Context())

	resp, err := platformAppTemplateResponseFrom(record)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, resp)
}

func findPlatformTemplateRecord(db *gorm.DB, key string) (model.PlatformAppTemplate, bool, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return model.PlatformAppTemplate{}, false, nil
	}
	var record model.PlatformAppTemplate
	err := db.Where("id = ? OR slug = ?", trimmed, strings.ToLower(trimmed)).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.PlatformAppTemplate{}, false, nil
	}
	if err != nil {
		return model.PlatformAppTemplate{}, false, err
	}
	return record, true, nil
}

func (h *Handlers) UpdatePlatformAppTemplate(ctx *gin.Context) {
	user, ok := h.requirePlatformTemplateAdmin(ctx)
	if !ok {
		return
	}
	db := h.dbFor(ctx)
	existing, found, err := findPlatformTemplateRecord(db, ctx.Param("templateId"))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeErrorCode(ctx, http.StatusNotFound, "app_template.not_found", "平台应用模板不存在")
		return
	}

	var input platformAppTemplateWriteInput
	if !bindJSON(ctx, &input) {
		return
	}
	enabled := existing.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	updated, _, err := appstore.ModelFromPlatformTemplate(existing.ID, existing.CreatedBy, enabled, input.toTemplate(existing.ID))
	if err != nil {
		writePlatformTemplateError(ctx, err)
		return
	}
	reserved, err := appstore.IsBuiltinSlugOrID(updated.Slug)
	if err != nil {
		writeTemplateCatalogUnavailable(ctx, err)
		return
	}
	if reserved {
		writeErrorCode(ctx, http.StatusConflict, "app_template.slug_reserved", fmt.Sprintf("模板标识 %s 已被内置模板占用", updated.Slug))
		return
	}

	var conflictCount int64
	if err := db.Model(&model.PlatformAppTemplate{}).
		Where("slug = ? AND id <> ?", updated.Slug, existing.ID).
		Count(&conflictCount).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if conflictCount > 0 {
		writeErrorCode(ctx, http.StatusConflict, "app_template.slug_exists", fmt.Sprintf("模板标识 %s 已存在", updated.Slug))
		return
	}

	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()
	if err := db.Save(&updated).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	auditAction := "platform_app_template.update"
	if existing.Enabled != updated.Enabled {
		if updated.Enabled {
			auditAction = "platform_app_template.enable"
		} else {
			auditAction = "platform_app_template.disable"
		}
	}
	h.auditWithContext(user.ID, auditAction, updated.ID, true, fmt.Sprintf("slug=%s enabled=%t", updated.Slug, updated.Enabled), ctx.Request.Context())

	resp, err := platformAppTemplateResponseFrom(updated)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (h *Handlers) DeletePlatformAppTemplate(ctx *gin.Context) {
	user, ok := h.requirePlatformTemplateAdmin(ctx)
	if !ok {
		return
	}
	db := h.dbFor(ctx)
	existing, found, err := findPlatformTemplateRecord(db, ctx.Param("templateId"))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeErrorCode(ctx, http.StatusNotFound, "app_template.not_found", "平台应用模板不存在")
		return
	}
	if err := db.Delete(&model.PlatformAppTemplate{}, "id = ?", existing.ID).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "platform_app_template.delete", existing.ID, true, fmt.Sprintf("slug=%s", existing.Slug), ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ExportPlatformAppTemplate(ctx *gin.Context) {
	user, ok := h.requirePlatformTemplateAdmin(ctx)
	if !ok {
		return
	}
	key := strings.TrimSpace(ctx.Param("templateId"))
	db := h.dbFor(ctx)
	if db != nil {
		if record, found, err := findPlatformTemplateRecord(db, key); err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		} else if found {
			tpl, err := appstore.TemplateFromPlatformModel(record)
			if err != nil {
				writeError(ctx, http.StatusInternalServerError, err.Error())
				return
			}
			h.auditWithContext(user.ID, "platform_app_template.export", record.ID, true, fmt.Sprintf("slug=%s source=platform", record.Slug), ctx.Request.Context())
			ctx.JSON(http.StatusOK, appstore.BundleFromTemplate(tpl, record.Enabled))
			return
		}
	}

	builtin, found, err := appstore.Find(key)
	if err != nil {
		writeTemplateCatalogUnavailable(ctx, err)
		return
	}
	if !found || builtin.Kind == "system_component" {
		writeErrorCode(ctx, http.StatusNotFound, "app_template.not_found", "应用模板不存在")
		return
	}
	h.auditWithContext(user.ID, "platform_app_template.export", builtin.ID, true, fmt.Sprintf("slug=%s source=builtin", builtin.Slug), ctx.Request.Context())
	ctx.JSON(http.StatusOK, appstore.BundleFromTemplate(builtin, true))
}

func (h *Handlers) ImportPlatformAppTemplate(ctx *gin.Context) {
	user, ok := h.requirePlatformTemplateAdmin(ctx)
	if !ok {
		return
	}
	var input platformAppTemplateImportInput
	if !bindJSON(ctx, &input) {
		return
	}
	if version := strings.TrimSpace(input.Bundle.SchemaVersion); version != "" && version != appstore.PlatformTemplateBundleSchemaVersion {
		writeErrorCode(ctx, http.StatusBadRequest, "app_template.schema_version_unsupported", fmt.Sprintf("不支持的模板导出版本 %s", version))
		return
	}
	enabled := true
	if input.Bundle.Enabled != nil {
		enabled = *input.Bundle.Enabled
	}

	candidateSlug := strings.ToLower(strings.TrimSpace(input.Bundle.Slug))
	reserved, err := appstore.IsBuiltinSlugOrID(candidateSlug)
	if err != nil {
		writeTemplateCatalogUnavailable(ctx, err)
		return
	}
	if reserved {
		writeErrorCode(ctx, http.StatusConflict, "app_template.slug_reserved", fmt.Sprintf("模板标识 %s 已被内置模板占用，导入为平台模板前请修改 slug", candidateSlug))
		return
	}

	db := h.dbFor(ctx)
	existing, found, err := findPlatformTemplateRecord(db, candidateSlug)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if found && !input.Overwrite {
		writeErrorCode(ctx, http.StatusConflict, "app_template.slug_exists", fmt.Sprintf("模板标识 %s 已存在，如需覆盖请开启 overwrite", candidateSlug))
		return
	}

	targetID := id.New("ptpl")
	createdBy := user.ID
	if found {
		targetID = existing.ID
		createdBy = existing.CreatedBy
	}
	record, _, err := appstore.ModelFromPlatformTemplate(targetID, createdBy, enabled, input.Bundle.ToTemplate(targetID))
	if err != nil {
		writePlatformTemplateError(ctx, err)
		return
	}

	statusCode := http.StatusCreated
	if found {
		statusCode = http.StatusOK
		record.CreatedAt = existing.CreatedAt
		record.UpdatedAt = time.Now().UTC()
		if err := db.Save(&record).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	} else if err := db.Create(&record).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	h.auditWithContext(user.ID, "platform_app_template.import", record.ID, true, fmt.Sprintf("slug=%s overwrite=%t enabled=%t", record.Slug, found && input.Overwrite, record.Enabled), ctx.Request.Context())
	resp, err := platformAppTemplateResponseFrom(record)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(statusCode, resp)
}
