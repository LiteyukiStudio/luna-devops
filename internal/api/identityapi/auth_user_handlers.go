package identityapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handlers) Login(ctx *gin.Context) {
	if !h.allowSensitiveAuthAttempt(ctx, "login_ip", 10, time.Minute) {
		return
	}
	policy, err := h.ensureAdmissionPolicy(ctx.Request.Context())
	if err != nil {
		writeAdmissionPolicyUnavailable(ctx, err)
		return
	}
	if !policy.AllowLocalLogin {
		writeError(ctx, http.StatusForbidden, "本地账号登录已关闭")
		return
	}

	var input loginInput
	if !bindJSON(ctx, &input) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if !h.allowLoginAccountAttempt(ctx, email, 10, time.Minute) {
		return
	}

	var user model.User
	err = h.dbFor(ctx).First(&user, "email = ?", email).Error
	if err != nil || user.Disabled || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)) != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.login.invalid")
		return
	}

	if !h.createLoginCredentials(ctx, user.ID, input.RememberMe) {
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

	user, sessionToken, rememberToken, err := h.rotateRememberLogin(userID, plainToken, ctx.Request.Context())
	if errors.Is(err, errRememberTokenInvalid) || errors.Is(err, errRememberTokenReused) {
		clearRememberCookie(ctx, userID)
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return
	}
	if errors.Is(err, errRememberUserDisabled) {
		clearRememberCookie(ctx, userID)
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return
	}
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "auth.session.resume_failed", err.Error())
		return
	}
	setSessionCookie(ctx, sessionToken, h.mode == "production", true)
	setRememberCookie(ctx, user.ID, rememberToken, h.mode == "production")

	ctx.JSON(http.StatusOK, gin.H{"user": currentUserResponse(user)})
}

func (h *Handlers) Logout(ctx *gin.Context) {
	if plainToken, err := ctx.Cookie(sessionCookieName); err == nil {
		userID, revokeErr := h.revokeCurrentSessionAndRememberTokens(plainToken, ctx.Request.Context())
		clearRememberCookie(ctx, userID)
		if revokeErr != nil {
			clearSessionCookie(ctx)
			writeErrorCode(ctx, http.StatusInternalServerError, "auth.logout_failed", revokeErr.Error())
			return
		}
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
	if input.BrandColorPreset != nil {
		preset, valid := h.host.NormalizeUserBrandColorPreset(*input.BrandColorPreset)
		if !valid {
			writeErrorCode(ctx, http.StatusBadRequest, "user.brand_color_invalid", "unsupported brand color preset")
			return
		}
		user.BrandColorPreset = preset
	}
	if input.InterfaceStyle != nil {
		style, valid := h.host.NormalizeUserInterfaceStyle(*input.InterfaceStyle)
		if !valid {
			writeErrorCode(ctx, http.StatusBadRequest, "user.interface_style_invalid", "unsupported interface style")
			return
		}
		user.InterfaceStyle = style
	}

	if err := h.dbFor(ctx).Save(&user).Error; err != nil {
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
	query := h.dbFor(ctx).Model(&model.User{})
	query = applySearch(ctx, query, "email", "name")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"createdAt":   "created_at",
		"email":       "email",
		"name":        "name",
		"role":        "role",
		"passwordSet": "CASE WHEN password = '' THEN 0 ELSE 1 END",
		"status":      "disabled",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&users).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	balances, err := h.userWalletBalances(users, ctx.Request.Context())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	responses := make([]gin.H, 0, len(users))
	for _, user := range users {
		responses = append(responses, userListResponse(user, balances[user.ID]))
	}
	ctx.JSON(http.StatusOK, paginatedResponse(responses, total, pagination))
}

func (h *Handlers) userWalletBalances(users []model.User, ctx context.Context) (map[string]decimal.Decimal, error) {
	balances := make(map[string]decimal.Decimal, len(users))
	if len(users) == 0 {
		return balances, nil
	}
	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
		balances[user.ID] = decimal.Zero
	}
	var wallets []model.UserWallet
	if err := h.dbWithContext(ctx).Select("user_id", "balance_credits").Where("user_id in ?", userIDs).Find(&wallets).Error; err != nil {
		return nil, err
	}
	for _, wallet := range wallets {
		balances[wallet.UserID] = wallet.BalanceCredits
	}
	return balances, nil
}

func (h *Handlers) CreateUser(ctx *gin.Context) {
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
		Role:     normalizeUserRole(input.Role),
		Language: normalizeLanguage(input.Language),
		Password: string(passwordHash),
		Disabled: input.Disabled,
	}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
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
	var user model.User
	if err := h.dbFor(ctx).First(&user, "id = ?", ctx.Param("userId")).Error; err != nil {
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

	passwordChanged := input.Password != ""
	user.Email = email
	user.Name = name
	user.Role = normalizeUserRole(input.Role)
	user.Language = normalizeLanguage(input.Language)
	user.Disabled = input.Disabled
	if passwordChanged {
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

	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockActiveUserRole(tx, currentUser.ID, authz.PlatformRoleAdmin); err != nil {
			return err
		}
		var stored model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&stored, "id = ?", user.ID).Error; err != nil {
			return err
		}
		revokeAuthentication := shouldRevokeUserAuthentication(stored.Role, user.Role, stored.Disabled, user.Disabled, passwordChanged)
		stored.Email = user.Email
		stored.Name = user.Name
		stored.Role = user.Role
		stored.Language = user.Language
		stored.Disabled = user.Disabled
		if passwordChanged {
			stored.Password = user.Password
		}
		if err := tx.Save(&stored).Error; err != nil {
			return err
		}
		if revokeAuthentication {
			if err := revokeUserAuthentication(tx, stored.ID); err != nil {
				return err
			}
		}
		user = stored
		return nil
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, user)
}

type loginInput struct {
	Email      string `json:"email" binding:"required"`
	Password   string `json:"password" binding:"required"`
	RememberMe bool   `json:"rememberMe"`
}

type resumeLoginInput struct {
	UserID string `json:"userId" binding:"required"`
}

type updateCurrentUserInput struct {
	Name             string  `json:"name"`
	AvatarURL        string  `json:"avatarUrl"`
	Language         string  `json:"language"`
	BrandColorPreset *string `json:"brandColorPreset"`
	InterfaceStyle   *string `json:"interfaceStyle"`
}

type userInput struct {
	Email    string `json:"email" binding:"required"`
	Name     string `json:"name" binding:"required"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Language string `json:"language"`
	Disabled bool   `json:"disabled"`
}

func shouldRevokeUserAuthentication(originalRole, nextRole string, originallyDisabled, nextDisabled, passwordChanged bool) bool {
	return originalRole != nextRole || (!originallyDisabled && nextDisabled) || passwordChanged
}

func ensureDevelopmentAdminWallet(ctx context.Context, db *gorm.DB, user model.User, configuredCredits string) {
	credits, err := developmentAdminFreeQuotaCredits(configuredCredits)
	if err != nil {
		telemetry.LogError(ctx, "Development administrator wallet initialization failed",
			"billing.development_wallet_initialization.failed", "billing.development_wallet.initialize",
			"billing.development_free_quota_invalid", err)
		return
	}

	service := billing.Service{DB: db}
	if credits.IsZero() {
		_, err = service.EnsureWallet(user.ID)
	} else {
		_, err = service.ApplyWalletTransaction(billing.WalletTransactionInput{
			UserID:         user.ID,
			AmountCredits:  credits,
			Type:           "credit",
			Reason:         billing.ReasonDevelopmentQuota,
			Description:    "Development administrator free quota",
			IdempotencyKey: "development-admin-free-quota:" + user.ID,
			ActorID:        "system:initial-admin",
		})
	}
	if err != nil {
		telemetry.LogError(ctx, "Development administrator wallet initialization failed",
			"billing.development_wallet_initialization.failed", "billing.development_wallet.initialize",
			"billing.development_wallet_initialization_failed", err,
			slog.String("resource.type", "user"),
			slog.String("resource.id", user.ID))
	}
}

func developmentAdminFreeQuotaCredits(configured string) (decimal.Decimal, error) {
	credits, err := decimal.NewFromString(strings.TrimSpace(configured))
	if err != nil || credits.IsNegative() {
		return decimal.Zero, errors.New("development administrator free quota must be a non-negative decimal")
	}
	return credits, nil
}

func normalizeUserRole(role string) string {
	if role == authz.PlatformRoleAdmin {
		return authz.PlatformRoleAdmin
	}
	return authz.PlatformRoleUser
}

func createDefaultUserProject(tx *gorm.DB, user model.User) error {
	identifier := defaultUserProjectIdentifier(tx, user)
	project := model.Project{
		ID:                  id.New("prj"),
		Identifier:          identifier,
		KubernetesNamespace: resourceidentifier.ProjectNamespace(identifier),
		Name:                defaultUserProjectName(user),
		Description:         defaultUserProjectDescription(user),
		BillingOwnerUserID:  user.ID,
	}
	if err := tx.Create(&project).Error; err != nil {
		return err
	}
	member := model.ProjectMember{
		ID:        id.New("mem"),
		ProjectID: project.ID,
		UserID:    user.ID,
		Role:      authz.ProjectRoleOwner,
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

func defaultUserProjectIdentifier(tx *gorm.DB, user model.User) string {
	base := dnsSafeProjectIdentifier(user.Name)
	if base == "" {
		base = dnsSafeProjectIdentifier(strings.Split(strings.TrimSpace(user.Email), "@")[0])
	}
	if base == "" {
		base = "project"
	}
	for index := 0; ; index++ {
		candidate := slugWithNumericSuffix(base, index)
		var count int64
		if err := tx.Model(&model.Project{}).Where("identifier = ?", candidate).Count(&count).Error; err != nil || count == 0 {
			return candidate
		}
	}
}

func dnsSafeProjectIdentifier(value string) string {
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
	identifier := strings.Trim(builder.String(), "-")
	if len(identifier) < resourceidentifier.ProjectMinLength {
		identifier = "user"
	}
	if len(identifier) > resourceidentifier.ProjectMaxLength {
		identifier = strings.TrimRight(identifier[:resourceidentifier.ProjectMaxLength], "-")
	}
	return identifier
}

func slugWithNumericSuffix(base string, index int) string {
	const maxSlugLength = resourceidentifier.ProjectMaxLength
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
		"id":               user.ID,
		"email":            user.Email,
		"name":             user.Name,
		"avatarUrl":        user.AvatarURL,
		"passwordSet":      strings.TrimSpace(user.Password) != "",
		"role":             user.Role,
		"language":         normalizeLanguage(user.Language),
		"brandColorPreset": user.BrandColorPreset,
		"interfaceStyle":   user.InterfaceStyle,
		"permissions":      permissionsFor(user),
	}
}

func userListResponse(user model.User, balanceCredits decimal.Decimal) gin.H {
	return gin.H{
		"id":             user.ID,
		"email":          user.Email,
		"name":           user.Name,
		"avatarUrl":      user.AvatarURL,
		"passwordSet":    strings.TrimSpace(user.Password) != "",
		"role":           user.Role,
		"language":       normalizeLanguage(user.Language),
		"disabled":       user.Disabled,
		"balanceCredits": balanceCredits.String(),
		"createdAt":      user.CreatedAt,
	}
}

func permissionsFor(user model.User) []string {
	if user.Role == authz.PlatformRoleAdmin {
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
