package api

import (
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
	if !h.allowSensitiveAuthAttempt(ctx, "bootstrap_admin", 5, time.Minute) {
		return
	}
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
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return createDefaultUserProject(tx, user)
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if !h.createSession(ctx, user.ID) {
		return
	}
	if !h.createRememberToken(ctx, user.ID) {
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{"user": currentUserResponse(user)})
}

func (h *Handlers) Login(ctx *gin.Context) {
	if !h.allowSensitiveAuthAttempt(ctx, "login", 10, time.Minute) {
		return
	}
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
	if !h.createRememberToken(ctx, user.ID) {
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"user": currentUserResponse(user)})
}

func (h *Handlers) ResumeLogin(ctx *gin.Context) {
	var input resumeLoginInput
	if !bindJSON(ctx, &input) {
		return
	}

	userID := strings.TrimSpace(input.UserID)
	plainToken, err := ctx.Cookie(rememberCookieNameForUser(userID))
	if err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return
	}

	var rememberToken model.UserRememberToken
	err = h.db.First(
		&rememberToken,
		"token_hash = ? and user_id = ? and expires_at > ?",
		hashToken(plainToken),
		userID,
		time.Now(),
	).Error
	if err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return
	}

	var user model.User
	if err := h.db.First(&user, "id = ? and disabled = ?", rememberToken.UserID, false).Error; err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
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

	if strings.TrimSpace(input.Name) != "" {
		user.Name = strings.TrimSpace(input.Name)
	}
	user.AvatarURL = strings.TrimSpace(input.AvatarURL)
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

	pagination := paginationFromQuery(ctx)
	var users []model.User
	query := h.db.Model(&model.User{})
	query = applySearch(ctx, query, "email", "name")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"createdAt": "created_at",
		"email":     "email",
		"name":      "name",
		"role":      "role",
		"authType":  "auth_type",
		"status":    "disabled",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&users).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(users, total, pagination))
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
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return createDefaultUserProject(tx, user)
	}); err != nil {
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

type loginInput struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type resumeLoginInput struct {
	UserID string `json:"userId" binding:"required"`
}

type updateCurrentUserInput struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
	Language  string `json:"language"`
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

func ensureDevelopmentAdmin(db *gorm.DB) {
	email := developmentAdminEmail()
	password := developmentAdminPassword()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}

	var user model.User
	err = db.First(&user, "email = ? and auth_type = ?", email, "local").Error
	if err != nil {
		user = model.User{
			ID:       "user_admin",
			Email:    email,
			Name:     env("LOCAL_ADMIN_NAME", "Platform Admin"),
			AuthType: "local",
			Role:     "platform_admin",
			Language: "zh-CN",
		}
		err = db.Create(&user).Error
		if err != nil && strings.Contains(err.Error(), "users_pkey") {
			user.ID = id.New("usr")
			err = db.Create(&user).Error
		}
		if err != nil {
			return
		}
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

func createDefaultUserProject(tx *gorm.DB, user model.User) error {
	project := model.Project{
		ID:                id.New("prj"),
		Slug:              defaultUserProjectSlug(tx, user),
		Name:              defaultUserProjectName(user),
		Description:       defaultUserProjectDescription(user),
		NamespaceStrategy: "project",
	}
	if err := tx.Create(&project).Error; err != nil {
		return err
	}
	member := model.ProjectMember{
		ID:        id.New("mem"),
		ProjectID: project.ID,
		UserID:    user.ID,
		Role:      "owner",
	}
	if err := tx.Create(&member).Error; err != nil {
		return err
	}
	return nil
}

func defaultUserProjectName(user model.User) string {
	name := fallback(strings.TrimSpace(user.Name), strings.TrimSpace(user.Email))
	if normalizeLanguage(user.Language) == "en-US" {
		return name + "'s Project Space"
	}
	return name + " 的项目空间"
}

func defaultUserProjectDescription(user model.User) string {
	if normalizeLanguage(user.Language) == "en-US" {
		return "Default project space created for the user."
	}
	return "为用户自动创建的默认项目空间。"
}

func defaultUserProjectSlug(tx *gorm.DB, user model.User) string {
	base := dnsSafeProjectSlug(user.Name)
	if base == "" {
		base = dnsSafeProjectSlug(strings.Split(strings.TrimSpace(user.Email), "@")[0])
	}
	if base == "" {
		base = "project"
	}
	for index := 0; ; index++ {
		candidate := slugWithNumericSuffix(base, index)
		var count int64
		if err := tx.Model(&model.Project{}).Where("slug = ?", candidate).Count(&count).Error; err != nil || count == 0 {
			return candidate
		}
	}
}

func dnsSafeProjectSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '-':
			builder.WriteRune(char)
		case char == '_' || char == '.' || char == ' ':
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func slugWithNumericSuffix(base string, index int) string {
	const maxSlugLength = 48
	suffix := ""
	if index > 0 {
		suffix = "-" + strconv.Itoa(index+1)
	}
	maxBaseLength := maxSlugLength - len(suffix)
	if maxBaseLength < 1 {
		maxBaseLength = 1
	}
	if len(base) > maxBaseLength {
		base = strings.TrimRight(base[:maxBaseLength], "-")
	}
	if base == "" {
		base = "project"
	}
	return base + suffix
}

func currentUserResponse(user model.User) gin.H {
	return gin.H{
		"id":          user.ID,
		"email":       user.Email,
		"name":        user.Name,
		"avatarUrl":   user.AvatarURL,
		"authType":    user.AuthType,
		"role":        user.Role,
		"language":    normalizeLanguage(user.Language),
		"permissions": permissionsFor(user),
	}
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
