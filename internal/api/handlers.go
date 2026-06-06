package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const sessionCookieName = "lyd_session"

type Handlers struct {
	db      *gorm.DB
	configs *configCache
	mode    string
}

func NewHandlers(db *gorm.DB) *Handlers {
	mode := runtimeMode()
	if mode == "development" {
		ensureDevelopmentAdmin(db)
	}
	ensureCasdoorAuthProvider(db)
	return &Handlers{db: db, configs: newConfigCache(db), mode: mode}
}

func (h *Handlers) GetBootstrapStatus(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, bootstrapStatusResponse(h.mode, h.hasPlatformAdmin()))
}

func bootstrapStatusResponse(mode string, initialized bool) gin.H {
	status := gin.H{
		"mode":            mode,
		"initialized":     initialized,
		"devLoginEnabled": mode == "development",
	}
	if mode == "development" {
		status["devLoginHint"] = gin.H{
			"email":    developmentAdminEmail(),
			"password": developmentAdminPassword(),
		}
	}
	return status
}

func (h *Handlers) InitializeAdmin(ctx *gin.Context) {
	if h.hasPlatformAdmin() {
		writeError(ctx, http.StatusConflict, "平台管理员已经初始化")
		return
	}

	var input initializeAdminInput
	if !bindJSON(ctx, &input) {
		return
	}

	email := strings.ToLower(strings.TrimSpace(input.Email))
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = email
	}
	if email == "" || len(input.Password) < 8 {
		writeError(ctx, http.StatusBadRequest, "请输入有效邮箱和至少 8 位密码")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	user := model.User{
		ID:       id.New("usr"),
		Email:    email,
		Name:     name,
		AuthType: "local",
		Role:     "platform_admin",
		Language: normalizeLanguage(input.Language),
		Password: string(passwordHash),
	}
	if err := h.db.Create(&user).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if !h.createSession(ctx, user.ID) {
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{"user": currentUserResponse(user)})
}

func (h *Handlers) Login(ctx *gin.Context) {
	if !h.ensureAdmissionPolicy().AllowLocalLogin {
		writeError(ctx, http.StatusForbidden, "本地账号登录已关闭")
		return
	}

	var input loginInput
	if !bindJSON(ctx, &input) {
		return
	}

	var user model.User
	err := h.db.First(&user, "email = ? and auth_type = ?", strings.ToLower(input.Email), "local").Error
	if err != nil || user.Disabled || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)) != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.login.invalid")
		return
	}

	if !h.createSession(ctx, user.ID) {
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"user": currentUserResponse(user)})
}

func (h *Handlers) Logout(ctx *gin.Context) {
	if plainToken, err := ctx.Cookie(sessionCookieName); err == nil {
		_ = h.db.Where("token_hash = ?", hashToken(plainToken)).Delete(&model.UserSession{}).Error
	}
	clearSessionCookie(ctx)
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) GetCurrentUser(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, currentUserResponse(user))
}

func (h *Handlers) UpdateCurrentUser(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input updateCurrentUserInput
	if !bindJSON(ctx, &input) {
		return
	}

	if input.Language != "" {
		user.Language = normalizeLanguage(input.Language)
	}

	if err := h.db.Save(&user).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, currentUserResponse(user))
}

func (h *Handlers) ListUsers(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}

	var users []model.User
	if err := h.db.Order("created_at desc").Find(&users).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, users)
}

func (h *Handlers) CreateUser(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}

	var input userInput
	if !bindJSON(ctx, &input) {
		return
	}

	email := strings.ToLower(strings.TrimSpace(input.Email))
	name := fallback(strings.TrimSpace(input.Name), email)
	if email == "" || len(input.Password) < 8 {
		writeError(ctx, http.StatusBadRequest, "请输入有效邮箱和至少 8 位密码")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	user := model.User{
		ID:       id.New("usr"),
		Email:    email,
		Name:     name,
		AuthType: "local",
		Role:     normalizeUserRole(input.Role),
		Language: normalizeLanguage(input.Language),
		Password: string(passwordHash),
		Disabled: input.Disabled,
	}
	if err := h.db.Create(&user).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	ctx.JSON(http.StatusCreated, user)
}

func (h *Handlers) UpdateUser(ctx *gin.Context) {
	currentUser, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if currentUser.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, currentUser.Language, "config.admin.required")
		return
	}

	var user model.User
	if err := h.db.First(&user, "id = ?", ctx.Param("userId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "user not found")
		return
	}

	var input userInput
	if !bindJSON(ctx, &input) {
		return
	}

	email := strings.ToLower(strings.TrimSpace(input.Email))
	name := strings.TrimSpace(input.Name)
	if email == "" || name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入有效邮箱和名称")
		return
	}

	user.Email = email
	user.Name = name
	user.Role = normalizeUserRole(input.Role)
	user.Language = normalizeLanguage(input.Language)
	user.Disabled = input.Disabled
	if input.Password != "" {
		if len(input.Password) < 8 {
			writeError(ctx, http.StatusBadRequest, "密码至少 8 位")
			return
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		user.Password = string(passwordHash)
	}

	if currentUser.ID == user.ID && user.Disabled {
		writeError(ctx, http.StatusBadRequest, "不能禁用当前登录账号")
		return
	}

	if err := h.db.Save(&user).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, user)
}

func (h *Handlers) ListProjects(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var projects []model.Project
	err := h.db.
		Joins("join project_members on project_members.project_id = projects.id").
		Where("project_members.user_id = ?", user.ID).
		Order("projects.created_at desc").
		Find(&projects).Error
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, projects)
}

func (h *Handlers) CreateProject(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input projectInput
	if !bindJSON(ctx, &input) {
		return
	}

	project := model.Project{
		ID:                id.New("prj"),
		Slug:              input.Slug,
		Name:              input.Name,
		Description:       input.Description,
		NamespaceStrategy: fallback(input.NamespaceStrategy, "project"),
	}

	if err := h.db.Create(&project).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	member := model.ProjectMember{
		ID:        id.New("mem"),
		ProjectID: project.ID,
		UserID:    user.ID,
		Role:      "owner",
	}
	_ = h.db.Create(&member).Error

	ctx.JSON(http.StatusCreated, project)
}

func (h *Handlers) GetProject(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUser(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, project)
}

func (h *Handlers) UpdateProject(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return
	}

	var input projectInput
	if !bindJSON(ctx, &input) {
		return
	}

	project.Slug = input.Slug
	project.Name = input.Name
	project.Description = input.Description
	project.NamespaceStrategy = fallback(input.NamespaceStrategy, "project")

	if err := h.db.Save(&project).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, project)
}

func (h *Handlers) DeleteProject(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner")
	if !ok {
		return
	}
	if err := h.db.Delete(&project).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListProjectMembers(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	var members []projectMemberResponse
	err := h.db.Table("project_members").
		Select("project_members.id, project_members.project_id, project_members.user_id, project_members.role, users.email, users.name").
		Joins("join users on users.id = project_members.user_id").
		Where("project_members.project_id = ?", ctx.Param("projectId")).
		Order("project_members.created_at asc").
		Scan(&members).Error
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, members)
}

func (h *Handlers) CreateProjectMember(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin"); !ok {
		return
	}

	var input projectMemberInput
	if !bindJSON(ctx, &input) {
		return
	}

	var user model.User
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if err := h.db.First(&user, "email = ? and disabled = ?", email, false).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "user not found")
		return
	}

	member := model.ProjectMember{
		ID:        id.New("mem"),
		ProjectID: ctx.Param("projectId"),
		UserID:    user.ID,
		Role:      normalizeProjectRole(input.Role),
	}
	if err := h.db.First(&model.ProjectMember{}, "project_id = ? and user_id = ?", member.ProjectID, member.UserID).Error; err == nil {
		writeError(ctx, http.StatusConflict, "user is already a project member")
		return
	}
	if err := h.db.Create(&member).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	ctx.JSON(http.StatusCreated, projectMemberResponse{
		ID:        member.ID,
		ProjectID: member.ProjectID,
		UserID:    member.UserID,
		Role:      member.Role,
		Email:     user.Email,
		Name:      user.Name,
	})
}

func (h *Handlers) UpdateProjectMember(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin"); !ok {
		return
	}

	var member model.ProjectMember
	if err := h.db.First(&member, "id = ? and project_id = ?", ctx.Param("memberId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "member not found")
		return
	}

	var input projectMemberInput
	if !bindJSON(ctx, &input) {
		return
	}
	member.Role = normalizeProjectRole(input.Role)
	if err := h.db.Save(&member).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, member)
}

func (h *Handlers) DeleteProjectMember(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin"); !ok {
		return
	}

	var member model.ProjectMember
	if err := h.db.First(&member, "id = ? and project_id = ?", ctx.Param("memberId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "member not found")
		return
	}
	if member.UserID == user.ID {
		writeError(ctx, http.StatusBadRequest, "不能移除当前登录账号")
		return
	}
	if err := h.db.Delete(&member).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListApplications(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	var applications []model.Application
	if err := h.db.Where("project_id = ?", ctx.Param("projectId")).Order("created_at desc").Find(&applications).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, applications)
}

func (h *Handlers) CreateApplication(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer"); !ok {
		return
	}

	var input applicationInput
	if !bindJSON(ctx, &input) {
		return
	}

	app := model.Application{
		ID:             id.New("app"),
		ProjectID:      ctx.Param("projectId"),
		Slug:           input.Slug,
		Name:           input.Name,
		SourceType:     fallback(input.SourceType, "repository"),
		RepositoryURL:  input.RepositoryURL,
		ImageReference: input.ImageReference,
		DockerfilePath: fallback(input.DockerfilePath, "Dockerfile"),
		BuildContext:   fallback(input.BuildContext, "."),
		ServicePort:    fallbackInt(input.ServicePort, 8080),
	}

	if err := h.db.Create(&app).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, app)
}

func (h *Handlers) GetApplication(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, app)
}

func (h *Handlers) UpdateApplication(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer"); !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}

	var input applicationInput
	if !bindJSON(ctx, &input) {
		return
	}

	app.Slug = input.Slug
	app.Name = input.Name
	app.SourceType = fallback(input.SourceType, "repository")
	app.RepositoryURL = input.RepositoryURL
	app.ImageReference = input.ImageReference
	app.DockerfilePath = fallback(input.DockerfilePath, "Dockerfile")
	app.BuildContext = fallback(input.BuildContext, ".")
	app.ServicePort = fallbackInt(input.ServicePort, 8080)

	if err := h.db.Save(&app).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, app)
}

func (h *Handlers) DeleteApplication(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer"); !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if err := h.db.Delete(&app).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListAccessTokens(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var tokens []model.AccessToken
	if err := h.db.Where("user_id = ?", user.ID).Order("created_at desc").Find(&tokens).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, tokens)
}

func (h *Handlers) CreateAccessToken(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input accessTokenInput
	if !bindJSON(ctx, &input) {
		return
	}

	plainToken := "lyd_" + randomHex(24)
	token := model.AccessToken{
		ID:        id.New("tok"),
		UserID:    user.ID,
		Name:      input.Name,
		Scope:     fallback(input.Scope, "project:read"),
		TokenHash: hashToken(plainToken),
	}

	if input.ExpiresInDays > 0 {
		expiresAt := time.Now().Add(time.Duration(input.ExpiresInDays) * 24 * time.Hour)
		token.ExpiresAt = &expiresAt
	}

	if err := h.db.Create(&token).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"token":       token,
		"accessToken": plainToken,
	})
}

func (h *Handlers) RevokeAccessToken(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var token model.AccessToken
	if err := h.db.First(&token, "id = ? and user_id = ?", ctx.Param("tokenId"), user.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "token not found")
		return
	}
	now := time.Now()
	token.RevokedAt = &now
	if err := h.db.Save(&token).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, token)
}

func (h *Handlers) findProject(ctx *gin.Context) (model.Project, bool) {
	var project model.Project
	if err := h.db.First(&project, "id = ?", ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project not found")
		return project, false
	}
	return project, true
}

func (h *Handlers) findProjectForCurrentUser(ctx *gin.Context) (model.Project, bool) {
	return h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer", "viewer")
}

func (h *Handlers) findProjectForCurrentUserWithRoles(ctx *gin.Context, allowedRoles ...string) (model.Project, bool) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return model.Project{}, false
	}

	project, ok := h.findProject(ctx)
	if !ok {
		return project, false
	}

	var member model.ProjectMember
	err := h.db.First(&member, "project_id = ? and user_id = ?", project.ID, user.ID).Error
	if err != nil {
		writeError(ctx, http.StatusForbidden, "你没有访问该项目的权限")
		return model.Project{}, false
	}

	if !projectRoleAllowed(member.Role, allowedRoles) {
		writeError(ctx, http.StatusForbidden, "你没有执行该项目操作的权限")
		return model.Project{}, false
	}

	return project, true
}

func projectRoleAllowed(role string, allowedRoles []string) bool {
	for _, allowedRole := range allowedRoles {
		if role == allowedRole {
			return true
		}
	}
	return false
}

func (h *Handlers) findApplication(ctx *gin.Context) (model.Application, bool) {
	var app model.Application
	err := h.db.First(
		&app,
		"id = ? and project_id = ?",
		ctx.Param("applicationId"),
		ctx.Param("projectId"),
	).Error
	if err != nil {
		writeError(ctx, http.StatusNotFound, "application not found")
		return app, false
	}
	return app, true
}

type projectInput struct {
	Slug              string `json:"slug" binding:"required"`
	Name              string `json:"name" binding:"required"`
	Description       string `json:"description"`
	NamespaceStrategy string `json:"namespaceStrategy"`
}

type projectMemberInput struct {
	Email string `json:"email"`
	Role  string `json:"role" binding:"required"`
}

type projectMemberResponse struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	UserID    string `json:"userId"`
	Role      string `json:"role"`
	Email     string `json:"email"`
	Name      string `json:"name"`
}

type loginInput struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type updateCurrentUserInput struct {
	Language string `json:"language"`
}

type userInput struct {
	Email    string `json:"email" binding:"required"`
	Name     string `json:"name" binding:"required"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Language string `json:"language"`
	Disabled bool   `json:"disabled"`
}

type initializeAdminInput struct {
	Email    string `json:"email" binding:"required"`
	Name     string `json:"name"`
	Password string `json:"password" binding:"required"`
	Language string `json:"language"`
}

type applicationInput struct {
	Slug           string `json:"slug" binding:"required"`
	Name           string `json:"name" binding:"required"`
	SourceType     string `json:"sourceType"`
	RepositoryURL  string `json:"repositoryUrl"`
	ImageReference string `json:"imageReference"`
	DockerfilePath string `json:"dockerfilePath"`
	BuildContext   string `json:"buildContext"`
	ServicePort    int    `json:"servicePort"`
}

type accessTokenInput struct {
	Name          string `json:"name" binding:"required"`
	Scope         string `json:"scope"`
	ExpiresInDays int    `json:"expiresInDays"`
}

func bindJSON(ctx *gin.Context, value any) bool {
	if err := ctx.ShouldBindJSON(value); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func writeError(ctx *gin.Context, status int, message string) {
	ctx.JSON(status, gin.H{"error": message})
}

func writeErrorKey(ctx *gin.Context, status int, language, key string) {
	ctx.JSON(status, gin.H{"error": messageFor(language, key)})
}

func messageFor(language, key string) string {
	messages := localizedMessages[normalizeLanguage(language)]
	if message, ok := messages[key]; ok {
		return message
	}
	return localizedMessages["zh-CN"][key]
}

func requestLanguage(ctx *gin.Context) string {
	if strings.Contains(strings.ToLower(ctx.GetHeader("Accept-Language")), "en") {
		return "en-US"
	}
	return "zh-CN"
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func fallbackInt(value, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}

func randomHex(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func ensureDevelopmentAdmin(db *gorm.DB) {
	email := developmentAdminEmail()
	password := developmentAdminPassword()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}

	user := model.User{
		ID:       "user_admin",
		Email:    email,
		Name:     env("LOCAL_ADMIN_NAME", "Platform Admin"),
		AuthType: "local",
		Role:     "platform_admin",
		Language: "zh-CN",
	}
	err = db.FirstOrCreate(&user, "email = ?", user.Email).Error
	if err != nil {
		return
	}

	needsSave := false
	if user.Password == "" {
		user.Password = string(passwordHash)
		needsSave = true
	}
	if user.Language == "" {
		user.Language = "zh-CN"
		needsSave = true
	}
	if needsSave {
		_ = db.Save(&user).Error
	}
}

func developmentAdminEmail() string {
	return strings.ToLower(env("LOCAL_ADMIN_EMAIL", "admin@liteyuki.dev"))
}

func developmentAdminPassword() string {
	return env("LOCAL_ADMIN_PASSWORD", "devops")
}

func ensureCasdoorAuthProvider(db *gorm.DB) {
	issuerURL := strings.TrimRight(env("CASDOOR_ISSUER_URL", env("OIDC_CASDOOR_ISSUER_URL", "")), "/")
	clientID := env("CASDOOR_CLIENT_ID", env("OIDC_CASDOOR_CLIENT_ID", ""))
	clientSecretRef := env("CASDOOR_CLIENT_SECRET_REF", env("OIDC_CASDOOR_CLIENT_SECRET_REF", ""))
	if issuerURL == "" || clientID == "" {
		return
	}

	provider := model.AuthProvider{
		ID:              "auth_provider_casdoor",
		Type:            "oidc",
		Name:            env("CASDOOR_PROVIDER_NAME", "Casdoor"),
		Enabled:         true,
		IssuerURL:       issuerURL,
		ClientID:        clientID,
		ClientSecretRef: clientSecretRef,
		Scopes:          env("CASDOOR_SCOPES", "openid profile email"),
		GroupClaim:      env("CASDOOR_GROUP_CLAIM", "groups"),
		EmailClaim:      env("CASDOOR_EMAIL_CLAIM", "email"),
		UsernameClaim:   env("CASDOOR_USERNAME_CLAIM", "preferred_username"),
		IsDefault:       true,
	}
	_ = db.FirstOrCreate(&provider, "id = ?", provider.ID).Error
}

func runtimeMode() string {
	switch strings.ToLower(os.Getenv("APP_ENV")) {
	case "production", "prod":
		return "production"
	case "development", "dev", "local":
		return "development"
	}

	if strings.Contains(os.Args[0], "go-build") {
		return "development"
	}
	return "production"
}

func normalizeLanguage(language string) string {
	switch language {
	case "en-US":
		return "en-US"
	default:
		return "zh-CN"
	}
}

func normalizeUserRole(role string) string {
	if role == "platform_admin" {
		return "platform_admin"
	}
	return "user"
}

func normalizeProjectRole(role string) string {
	switch role {
	case "owner", "admin", "developer", "viewer":
		return role
	default:
		return "viewer"
	}
}

var localizedMessages = map[string]map[string]string{
	"zh-CN": {
		"auth.login.invalid":    "邮箱或密码不正确",
		"auth.session.missing":  "请先登录",
		"auth.session.expired":  "登录会话已过期，请重新登录",
		"auth.account.disabled": "账号不可用，请联系平台管理员",
		"config.admin.required": "只有平台管理员可以修改站点配置",
	},
	"en-US": {
		"auth.login.invalid":    "Email or password is incorrect",
		"auth.session.missing":  "Please sign in first",
		"auth.session.expired":  "Your session has expired. Please sign in again",
		"auth.account.disabled": "This account is unavailable. Contact a platform administrator",
		"config.admin.required": "Only platform administrators can update site settings",
	},
}

func (h *Handlers) currentUser(ctx *gin.Context) (model.User, bool) {
	if strings.HasPrefix(strings.ToLower(ctx.GetHeader("Authorization")), "bearer ") {
		return h.currentUserFromAccessToken(ctx)
	}

	plainToken, err := ctx.Cookie(sessionCookieName)
	if err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.missing")
		return model.User{}, false
	}

	var session model.UserSession
	err = h.db.First(&session, "token_hash = ? and expires_at > ?", hashToken(plainToken), time.Now()).Error
	if err != nil {
		clearSessionCookie(ctx)
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return model.User{}, false
	}

	var user model.User
	if err := h.db.First(&user, "id = ? and disabled = ?", session.UserID, false).Error; err != nil {
		clearSessionCookie(ctx)
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, false
	}

	return user, true
}

func (h *Handlers) currentUserFromAccessToken(ctx *gin.Context) (model.User, bool) {
	header := ctx.GetHeader("Authorization")
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return model.User{}, false
	}

	plainToken := strings.TrimSpace(header[len("Bearer "):])
	var token model.AccessToken
	err := h.db.First(
		&token,
		"token_hash = ? and revoked_at is null and (expires_at is null or expires_at > ?)",
		hashToken(plainToken),
		time.Now(),
	).Error
	if err != nil || !accessTokenAllows(token.Scope, requiredScopeForRequest(ctx)) {
		writeError(ctx, http.StatusForbidden, "Access Token scope 不足或已失效")
		return model.User{}, false
	}

	var user model.User
	if err := h.db.First(&user, "id = ? and disabled = ?", token.UserID, false).Error; err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, false
	}

	return user, true
}

func requiredScopeForRequest(ctx *gin.Context) string {
	switch {
	case strings.HasPrefix(ctx.FullPath(), "/api/v1/projects") && ctx.Request.Method == http.MethodGet:
		return "project:read"
	case strings.HasPrefix(ctx.FullPath(), "/api/v1/projects") && ctx.Request.Method != http.MethodGet:
		return "project:write"
	case strings.HasPrefix(ctx.FullPath(), "/api/v1/access-tokens"):
		return "token:manage"
	default:
		return ""
	}
}

func accessTokenAllows(scopeText, required string) bool {
	if required == "" {
		return true
	}
	scopes := splitCSV(strings.ReplaceAll(scopeText, " ", ","))
	if containsString(scopes, "*") || containsString(scopes, required) {
		return true
	}
	requiredPrefix, _, _ := strings.Cut(required, ":")
	return containsString(scopes, requiredPrefix+":*")
}

func (h *Handlers) hasPlatformAdmin() bool {
	var count int64
	_ = h.db.Model(&model.User{}).Where("role = ? and disabled = ?", "platform_admin", false).Count(&count).Error
	return count > 0
}

func (h *Handlers) requirePlatformAdmin(ctx *gin.Context) bool {
	user, ok := h.currentUser(ctx)
	if !ok {
		return false
	}
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return false
	}
	return true
}

func (h *Handlers) createSession(ctx *gin.Context, userID string) bool {
	plainToken := "sess_" + randomHex(32)
	session := model.UserSession{
		ID:        id.New("ses"),
		UserID:    userID,
		TokenHash: hashToken(plainToken),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := h.db.Create(&session).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}

	setSessionCookie(ctx, plainToken)
	return true
}

func currentUserResponse(user model.User) gin.H {
	return gin.H{
		"id":          user.ID,
		"email":       user.Email,
		"name":        user.Name,
		"authType":    user.AuthType,
		"role":        user.Role,
		"language":    normalizeLanguage(user.Language),
		"permissions": permissionsFor(user),
	}
}

func setSessionCookie(ctx *gin.Context, token string) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(sessionCookieName, token, 86400, "/", "", false, true)
}

func clearSessionCookie(ctx *gin.Context) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}

func permissionsFor(user model.User) []string {
	if user.Role == "platform_admin" {
		return []string{
			"project.create",
			"project.read",
			"project.update",
			"project.delete",
			"application.create",
			"application.read",
			"application.update",
			"application.delete",
			"token.create",
			"token.revoke",
			"user.manage",
		}
	}

	return []string{"project.read", "application.read"}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
