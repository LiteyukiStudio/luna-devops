package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) ListContainerImages(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	pagination := paginationFromQuery(ctx)
	query := h.db
	if projectID := strings.TrimSpace(ctx.Query("projectId")); projectID != "" {
		if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); !ok {
			return
		}
		query = query.Where("project_id = ?", projectID)
	} else if user.Role != authz.PlatformRoleAdmin {
		query = query.Where("created_by = ? or project_id in ?", user.ID, h.projectIDsForUser(user.ID))
	}
	query = applySearch(ctx, query, "image_ref", "repository", "tag", "digest", "source_commit", "build_run_id", "source_type", "scan_status")

	var images []model.ContainerImage
	if ctx.Query("page") == "" && ctx.Query("pageSize") == "" {
		if err := query.Order("created_at desc").Find(&images).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, images)
		return
	}
	var total int64
	if err := query.Model(&model.ContainerImage{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	orderBy := orderByClause(pagination, map[string]string{
		"createdAt":    "created_at",
		"repository":   "repository",
		"tag":          "tag",
		"sourceType":   "source_type",
		"scanStatus":   "scan_status",
		"sourceCommit": "source_commit",
	}, "created_at")
	if err := query.Order(orderBy).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&images).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(images, total, pagination))
}

func (h *Handlers) CreateContainerImage(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input containerImageInput
	if !bindJSON(ctx, &input) {
		return
	}
	registry, ok := h.findAccessibleRegistry(ctx, user, input.RegistryID)
	if !ok {
		return
	}
	if input.ProjectID != "" {
		if _, ok := h.findProjectForCurrentUserWithRolesByID(ctx, input.ProjectID, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper); !ok {
			return
		}
	} else if user.Role != authz.PlatformRoleAdmin {
		writeError(ctx, http.StatusForbidden, "只有平台管理员可以创建未归属项目的镜像记录")
		return
	}

	image := model.ContainerImage{
		ID:            id.New("img"),
		ProjectID:     strings.TrimSpace(input.ProjectID),
		ApplicationID: strings.TrimSpace(input.ApplicationID),
		RegistryID:    registry.ID,
		Repository:    strings.Trim(strings.TrimSpace(input.Repository), "/"),
		Tag:           fallback(strings.TrimSpace(input.Tag), "latest"),
		Digest:        strings.TrimSpace(input.Digest),
		SourceCommit:  strings.TrimSpace(input.SourceCommit),
		BuildRunID:    strings.TrimSpace(input.BuildRunID),
		SourceType:    normalizeImageSourceType(input.SourceType),
		ScanStatus:    normalizeScanStatus(input.ScanStatus),
		CreatedBy:     user.ID,
	}
	if image.Repository == "" {
		writeError(ctx, http.StatusBadRequest, "请输入镜像仓库路径")
		return
	}
	image.ImageRef = imageReference(registry, image.Repository, image.Tag, image.Digest)

	if err := h.db.Create(&image).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, image)
}
