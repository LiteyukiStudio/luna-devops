package identityapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/platformmail"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type opsGetMyAccountSecurityInput struct {
	IncludeTokens             *bool `json:"includeTokens,omitempty" default:"true" desc:"是否返回个人访问令牌（PAT）摘要列表（不回显明文 Token）"`
	IncludeExternalIdentities *bool `json:"includeExternalIdentities,omitempty" default:"true" desc:"是否返回已绑定的外部 OIDC 身份列表"`
	IncludeOAuthGrants        *bool `json:"includeOAuthGrants,omitempty" default:"true" desc:"是否返回本人已授权的第三方 OAuth 应用列表"`
	IncludeOwnedOAuthApps     *bool `json:"includeOwnedOAuthApps,omitempty" default:"true" desc:"是否返回本人创建的第三方 OAuth 应用列表（不回显密钥）"`
}

type opsUpdateMyProfileInput struct {
	Name             *string `json:"name,omitempty" desc:"新的用户显示名称；省略保持原值"`
	AvatarURL        *string `json:"avatarUrl,omitempty" desc:"新的头像 URL；省略保持原头像（如需清空请传 clearAvatar=true）"`
	ClearAvatar      *bool   `json:"clearAvatar,omitempty" desc:"显式清空当前个人头像"`
	Language         *string `json:"language,omitempty" enum:"zh-CN|en-US" desc:"界面语言偏好"`
	BrandColorPreset *string `json:"brandColorPreset,omitempty" enum:"inherit|blue|violet|teal|green|orange|pink" desc:"个人主题色预设（inherit 表示跟随平台默认主题色）"`
}

type opsRevokeMyCredentialOrGrantInput struct {
	TargetType string `json:"targetType" required:"true" enum:"access_token|oauth_application|oauth_grant|external_identity" desc:"待吊销/解绑的目标类型"`
	TargetID   string `json:"targetId" required:"true" min:"1" desc:"对应令牌 ID、OAuth 应用 ID、OAuth 授权 ID 或外部身份 ID"`
}

type adminListUsersAndAuthInput struct {
	User                *string `json:"user,omitempty" desc:"按用户 ID（usr_xxx）或唯一邮箱精确过滤单条用户"`
	Search              *string `json:"search,omitempty" desc:"按邮箱或显示名称模糊搜索"`
	SortBy              *string `json:"sortBy,omitempty" enum:"createdAt|email|name|role|passwordSet|status" default:"createdAt" desc:"排序字段"`
	SortOrder           *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	IncludeAuthSettings *bool   `json:"includeAuthSettings,omitempty" default:"false" desc:"是否同时附带登录准入策略、注册设置、OIDC Providers、回调地址及系统发信 SMTP 摘要"`
	Page                *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码（>=1）"`
	PageSize            *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（默认 20，上限 50）"`
}

type adminSaveUserInput struct {
	User     *string `json:"user,omitempty" desc:"省略为新建本地用户；传入用户 ID 或唯一邮箱为存量增量修改"`
	Email    *string `json:"email,omitempty" desc:"用户邮箱（新建必填）"`
	Name     *string `json:"name,omitempty" desc:"用户显示名称（新建必填）"`
	Role     *string `json:"role,omitempty" enum:"platform_admin|user" desc:"平台角色（新建默认 user；更新省略保持原角色）"`
	Disabled *bool   `json:"disabled,omitempty" desc:"是否禁用该用户账户"`
	Language *string `json:"language,omitempty" enum:"zh-CN|en-US" desc:"用户默认语言"`
	Password *string `json:"password,omitempty" min:"8" sensitive:"true" desc:"新建必填初始密码（>=8 位）；更新他人账户时传入即重置密码"`
}

type adminConfigureAuthPolicyInput struct {
	AllowLocalLogin               *bool    `json:"allowLocalLogin,omitempty" desc:"是否允许本地账号密码登录"`
	AllowOIDCLogin                *bool    `json:"allowOidcLogin,omitempty" desc:"是否允许 OIDC 单点登录"`
	RequireVerifiedOIDCEmail      *bool    `json:"requireVerifiedOidcEmail,omitempty" desc:"OIDC 登录是否强制要求已验证邮箱"`
	DefaultRole                   *string  `json:"defaultRole,omitempty" enum:"user|platform_admin" desc:"新自动创建用户的默认平台角色"`
	AddAllowedEmailDomains        []string `json:"addAllowedEmailDomains,omitempty" desc:"增量添加准入邮箱域名"`
	RemoveAllowedEmailDomains     []string `json:"removeAllowedEmailDomains,omitempty" desc:"增量移除准入邮箱域名"`
	ClearAllowedEmailDomains      *bool    `json:"clearAllowedEmailDomains,omitempty" desc:"显式清空准入邮箱域名白名单"`
	AddAllowedOIDCGroups          []string `json:"addAllowedOidcGroups,omitempty" desc:"增量添加准入 OIDC 用户组"`
	RemoveAllowedOIDCGroups       []string `json:"removeAllowedOidcGroups,omitempty" desc:"增量移除准入 OIDC 用户组"`
	ClearAllowedOIDCGroups        *bool    `json:"clearAllowedOidcGroups,omitempty" desc:"显式清空准入 OIDC 用户组白名单"`
	AddInvitedEmails              []string `json:"addInvitedEmails,omitempty" desc:"增量添加受邀邮箱白名单"`
	RemoveInvitedEmails           []string `json:"removeInvitedEmails,omitempty" desc:"增量移除受邀邮箱白名单"`
	ClearInvitedEmails            *bool    `json:"clearInvitedEmails,omitempty" desc:"显式清空受邀邮箱白名单"`
	AllowEmailRegistration        *bool    `json:"allowEmailRegistration,omitempty" desc:"是否开启邮箱验证码自助注册（需先配置平台 SMTP）"`
	AllowOIDCRegistration         *bool    `json:"allowOidcRegistration,omitempty" desc:"是否开启 OIDC 首次登录自动注册"`
	AllowExternalIdentityPassword *bool    `json:"allowExternalIdentityPassword,omitempty" desc:"是否允许外部身份用户自助设置本地密码"`
}

type adminSaveOIDCProviderInput struct {
	Provider      *string  `json:"provider,omitempty" desc:"省略为新建 OIDC Provider；传入 Provider ID 或唯一名称为存量增量修改"`
	Name          *string  `json:"name,omitempty" desc:"OIDC Provider 显示名称（新建必填）"`
	IssuerURL     *string  `json:"issuerUrl,omitempty" desc:"OIDC Issuer URL（新建必填）"`
	ClientID      *string  `json:"clientId,omitempty" desc:"OAuth2/OIDC Client ID（新建必填）"`
	ClientSecret  *string  `json:"clientSecret,omitempty" sensitive:"true" desc:"OIDC Client Secret（新建必填；更新时省略严格保留原密钥）"`
	AddScopes     []string `json:"addScopes,omitempty" desc:"增量添加 OIDC Scopes（新建未传默认 openid profile email）"`
	RemoveScopes  []string `json:"removeScopes,omitempty" desc:"增量移除 OIDC Scopes（禁止移除必选 openid）"`
	EmailClaim    *string  `json:"emailClaim,omitempty" desc:"邮箱 Claim 名（默认 email）"`
	UsernameClaim *string  `json:"usernameClaim,omitempty" desc:"用户名 Claim 名（默认 preferred_username）"`
	GroupClaim    *string  `json:"groupClaim,omitempty" desc:"用户组 Claim 名（默认 groups）"`
	Enabled       *bool    `json:"enabled,omitempty" desc:"是否启用该 OIDC Provider（新建默认 true）"`
	IsDefault     *bool    `json:"isDefault,omitempty" desc:"是否设为默认 OIDC Provider"`
}

type accessTokenSummary struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scope     string     `json:"scope"`
	Source    string     `json:"source"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

type identityTargetLock struct {
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
}

func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[opsGetMyAccountSecurityInput, map[string]any]{
		Name:           "ops_get_my_account_security",
		Domain:         aitool.DomainOps,
		Title:          "查看本人账户资料与安全凭据",
		Summary:        "查看当前用户个人资料（含 passwordSet）、外部 OIDC 绑定身份、个人访问令牌（PAT）摘要、已授权第三方 OAuth 应用及本人创建的 OAuth 应用列表",
		AliasesZH:      []string{"账户安全", "个人资料", "访问令牌列表", "OAuth 授权", "外部绑定身份"},
		AliasesEN:      []string{"my account security", "profile", "access tokens", "oauth grants"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionUserRead)},
		Execute:        h.executeOpsGetMyAccountSecurity,
	})

	aitool.Register(reg, aitool.Spec[opsUpdateMyProfileInput, map[string]any]{
		Name:           "ops_update_my_profile",
		Domain:         aitool.DomainOps,
		Title:          "增量修改本人个人资料与界面偏好",
		Summary:        "增量修改当前用户显示名称、头像、界面语言或主题色预设，省略字段严格保持现状",
		AliasesZH:      []string{"修改个人资料", "更新昵称", "设置语言", "设置主题色"},
		AliasesEN:      []string{"update my profile", "change display name", "update language"},
		ApprovalTier:   aitool.ApprovalTierL1,
		Effect:         aitool.EffectWrite,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionUserWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID: "ops_get_my_account_security",
			Family:      "resource",
			Singleton:   true,
		}},
		Execute: h.executeOpsUpdateMyProfile,
	})

	aitool.Register(reg, aitool.Spec[opsRevokeMyCredentialOrGrantInput, map[string]any]{
		Name:           "ops_revoke_my_credential_or_grant",
		Domain:         aitool.DomainOps,
		Title:          "吊销本人访问令牌、OAuth 应用/授权或解绑外部身份",
		Summary:        "吊销本人个人访问令牌（PAT）、吊销本人创建的 OAuth 应用、撤销 OAuth 授权或解绑外部 OIDC 身份",
		AliasesZH:      []string{"吊销令牌", "删除 PAT", "撤销 OAuth 授权", "解绑 OIDC"},
		AliasesEN:      []string{"revoke token", "revoke oauth grant", "unbind external identity"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionTokenManage), string(authz.ActionUserWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:        "ops_get_my_account_security",
				Family:             "resource",
				Expect:             "absent",
				WhenArgumentEquals: map[string]any{"targetType": "access_token"},
				IDFrom:             "result.id",
				Collection:         true,
				CollectionPath:     "tokens",
				StaticArguments:    map[string]any{"includeTokens": true},
			},
			{
				OperationID:        "ops_get_my_account_security",
				Family:             "resource",
				Expect:             "absent",
				WhenArgumentEquals: map[string]any{"targetType": "oauth_application"},
				IDFrom:             "result.id",
				Collection:         true,
				CollectionPath:     "ownedOAuthApps",
				StaticArguments:    map[string]any{"includeOwnedOAuthApps": true},
			},
			{
				OperationID:        "ops_get_my_account_security",
				Family:             "resource",
				Expect:             "absent",
				WhenArgumentEquals: map[string]any{"targetType": "oauth_grant"},
				IDFrom:             "result.id",
				Collection:         true,
				CollectionPath:     "oauthGrants",
				StaticArguments:    map[string]any{"includeOAuthGrants": true},
			},
			{
				OperationID:        "ops_get_my_account_security",
				Family:             "resource",
				Expect:             "absent",
				WhenArgumentEquals: map[string]any{"targetType": "external_identity"},
				IDFrom:             "result.id",
				Collection:         true,
				CollectionPath:     "externalIdentities",
				StaticArguments:    map[string]any{"includeExternalIdentities": true},
			},
		},
		Preview: h.previewOpsRevokeMyCredentialOrGrant,
		Execute: h.executeOpsRevokeMyCredentialOrGrant,
	})

	aitool.Register(reg, aitool.Spec[adminListUsersAndAuthInput, map[string]any]{
		Name:           "admin_list_users_and_auth",
		Domain:         aitool.DomainAdmin,
		Title:          "查看平台用户与认证治理配置",
		Summary:        "分页查看平台用户列表（含钱包余额与禁用状态），可选附带登录准入策略、自助注册设置、OIDC 身份源及系统 SMTP 摘要",
		AliasesZH:      []string{"用户列表", "认证策略", "OIDC 列表", "注册开关"},
		AliasesEN:      []string{"list users and auth", "oidc providers", "admission policy"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionUserRead)},
		Execute:        h.executeAdminListUsersAndAuth,
	})

	aitool.Register(reg, aitool.Spec[adminSaveUserInput, map[string]any]{
		Name:           "admin_save_user",
		Domain:         aitool.DomainAdmin,
		Title:          "创建或增量修改平台本地用户",
		Summary:        "创建本地用户（自动初始化默认项目空间），或增量修改现有用户的角色、禁用状态与管理员密码重置",
		AliasesZH:      []string{"创建用户", "禁用用户", "修改用户角色", "重置用户密码"},
		AliasesEN:      []string{"create user", "update user", "disable user", "reset user password"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionUserManage)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "admin_list_users_and_auth",
			Family:         "resource",
			IDFrom:         "result.id",
			ReadIDArgument: "user",
			Collection:     true,
			CollectionPath: "items",
		}},
		Preview: h.previewAdminSaveUser,
		Execute: h.executeAdminSaveUser,
	})

	aitool.Register(reg, aitool.Spec[adminConfigureAuthPolicyInput, map[string]any]{
		Name:           "admin_configure_auth_policy",
		Domain:         aitool.DomainAdmin,
		Title:          "单事务配置登录准入策略与注册设置",
		Summary:        "单事务纯增量更新平台登录准入策略（本地/OIDC 开关、白名单域名/用户组/受邀邮箱）与自助注册设置",
		AliasesZH:      []string{"登录准入策略", "注册设置", "邮箱白名单", "OIDC 用户组限制"},
		AliasesEN:      []string{"configure auth policy", "admission policy", "registration settings"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionAuthManage)},
		Verification: []aitool.OperationVerification{{
			OperationID:     "admin_list_users_and_auth",
			Family:          "resource",
			Singleton:       true,
			StaticArguments: map[string]any{"includeAuthSettings": true},
		}},
		Preview: h.previewAdminConfigureAuthPolicy,
		Execute: h.executeAdminConfigureAuthPolicy,
	})

	aitool.Register(reg, aitool.Spec[adminSaveOIDCProviderInput, map[string]any]{
		Name:           "admin_save_oidc_provider",
		Domain:         aitool.DomainAdmin,
		Title:          "创建或增量修改 OIDC 身份提供商",
		Summary:        "创建或纯增量更新平台 OIDC 身份提供商配置，省略 clientSecret 时严格保留存量密钥引用",
		AliasesZH:      []string{"配置 OIDC", "新增单点登录", "修改 OIDC Provider"},
		AliasesEN:      []string{"save oidc provider", "create oidc provider", "update oidc provider"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionAuthManage)},
		Verification: []aitool.OperationVerification{{
			OperationID:     "admin_list_users_and_auth",
			Family:          "resource",
			IDFrom:          "result.id",
			Collection:      true,
			CollectionPath:  "oidcProviders",
			StaticArguments: map[string]any{"includeAuthSettings": true},
		}},
		Preview: h.previewAdminSaveOIDCProvider,
		Execute: h.executeAdminSaveOIDCProvider,
	})
}

func (h *Handler) executeOpsGetMyAccountSecurity(tc *aitool.ToolContext, in opsGetMyAccountSecurityInput) (map[string]any, error) {
	tc.SetNoStore()
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	var user model.User
	if err := tc.DB.First(&user, "id = ?", tc.User.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("user.not_found", "current user was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	oauthRestricted := !h.isPrimaryNonImpersonatedSession(tc, user.ID)
	profile := currentUserResponse(user)

	out := map[string]any{
		"id":                      user.ID,
		"email":                   user.Email,
		"name":                    user.Name,
		"avatarUrl":               user.AvatarURL,
		"passwordSet":             strings.TrimSpace(user.Password) != "",
		"role":                    user.Role,
		"language":                normalizeLanguage(user.Language),
		"brandColorPreset":        user.BrandColorPreset,
		"profile":                 profile,
		"oauthSectionsRestricted": oauthRestricted,
	}

	if in.IncludeTokens == nil || *in.IncludeTokens {
		var rawTokens []model.AccessToken
		if err := tc.DB.Where("user_id = ? AND source = ? AND revoked_at IS NULL", user.ID, model.AccessTokenSourcePersonal).
			Order("created_at desc").
			Find(&rawTokens).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		summaries := make([]accessTokenSummary, 0, len(rawTokens))
		for _, tok := range rawTokens {
			summaries = append(summaries, accessTokenSummary{
				ID:        tok.ID,
				Name:      tok.Name,
				Scope:     tok.Scope,
				Source:    tok.Source,
				ExpiresAt: tok.ExpiresAt,
				CreatedAt: tok.CreatedAt,
				UpdatedAt: tok.UpdatedAt,
			})
		}
		out["tokens"] = summaries
	}

	if in.IncludeExternalIdentities == nil || *in.IncludeExternalIdentities {
		var identities []externalIdentityResponse
		if err := tc.DB.Table("external_identities").
			Select("external_identities.id, external_identities.user_id, external_identities.provider_id, auth_providers.name as provider_name, external_identities.subject, external_identities.email, external_identities.email_verified, external_identities.username, external_identities.last_login_at, external_identities.created_at").
			Joins("join auth_providers on auth_providers.id = external_identities.provider_id").
			Where("external_identities.user_id = ?", user.ID).
			Order("external_identities.created_at desc").
			Scan(&identities).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if identities == nil {
			identities = []externalIdentityResponse{}
		}
		out["externalIdentities"] = identities
	}

	if !oauthRestricted {
		if in.IncludeOAuthGrants == nil || *in.IncludeOAuthGrants {
			var grants []model.OAuthGrant
			if err := tc.DB.Where("user_id = ? AND revoked_at IS NULL", user.ID).
				Order("updated_at desc").
				Find(&grants).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			grantItems := make([]oauthGrantResponse, 0, len(grants))
			for _, g := range grants {
				var app model.OAuthApplication
				if err := tc.DB.First(&app, "id = ?", g.ApplicationID).Error; err != nil {
					continue
				}
				grantItems = append(grantItems, oauthGrantResponse{
					ID:          g.ID,
					Application: oauthApplicationToResponse(app),
					Scope:       g.Scope,
					CreatedAt:   g.CreatedAt,
					UpdatedAt:   g.UpdatedAt,
				})
			}
			out["oauthGrants"] = grantItems
		}

		if in.IncludeOwnedOAuthApps == nil || *in.IncludeOwnedOAuthApps {
			var apps []model.OAuthApplication
			if err := tc.DB.Where("owner_user_id = ? AND revoked_at IS NULL", user.ID).
				Order("created_at desc").
				Find(&apps).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			appItems := make([]oauthApplicationResponse, 0, len(apps))
			for _, app := range apps {
				appItems = append(appItems, oauthApplicationToResponse(app))
			}
			out["ownedOAuthApps"] = appItems
		}
	}

	return out, nil
}

func (h *Handler) executeOpsUpdateMyProfile(tc *aitool.ToolContext, in opsUpdateMyProfileInput) (map[string]any, error) {
	clearAvatar := in.ClearAvatar != nil && *in.ClearAvatar
	if in.Name == nil && in.AvatarURL == nil && !clearAvatar && in.Language == nil && in.BrandColorPreset == nil {
		return nil, aitool.BadRequest("user.update_noop", "at least one profile field must be provided")
	}
	if clearAvatar && in.AvatarURL != nil {
		return nil, aitool.BadRequest("user.avatar_conflict", "cannot set avatarUrl and clearAvatar=true at the same time")
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return nil, aitool.BadRequest("user.name_invalid", "name cannot be empty")
	}
	if in.AvatarURL != nil && strings.TrimSpace(*in.AvatarURL) == "" {
		return nil, aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"user.avatar_empty_forbidden",
			"avatarUrl cannot be an empty string",
			"Pass clearAvatar=true to clear the current avatar.",
		)
	}
	if in.Language != nil {
		lang := strings.TrimSpace(*in.Language)
		if lang != "zh-CN" && lang != "en-US" {
			return nil, aitool.BadRequest("user.language_invalid", "language must be zh-CN or en-US")
		}
	}

	var nextPreset *string
	if in.BrandColorPreset != nil {
		rawPreset := strings.ToLower(strings.TrimSpace(*in.BrandColorPreset))
		if rawPreset == "inherit" {
			empty := ""
			nextPreset = &empty
		} else {
			normalized, valid := h.host.NormalizeUserBrandColorPreset(rawPreset)
			if !valid {
				return nil, aitool.BadRequest("user.brand_color_invalid", "unsupported brandColorPreset")
			}
			nextPreset = &normalized
		}
	}

	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	var user model.User
	if err := tc.DB.First(&user, "id = ?", tc.User.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("user.not_found", "user was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	if in.Name != nil {
		user.Name = strings.TrimSpace(*in.Name)
	}
	if clearAvatar {
		user.AvatarURL = ""
	} else if in.AvatarURL != nil {
		user.AvatarURL = strings.TrimSpace(*in.AvatarURL)
	}
	if in.Language != nil {
		user.Language = normalizeLanguage(*in.Language)
	}
	if nextPreset != nil {
		user.BrandColorPreset = *nextPreset
	}

	if err := tc.DB.Save(&user).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "user.update_failed", err.Error())
	}
	h.auditWithContext(user.ID, "user.profile.update", user.ID, true, "user profile updated", tc.Context())
	return currentUserResponse(user), nil
}

func (h *Handler) previewOpsRevokeMyCredentialOrGrant(tc *aitool.ToolContext, in opsRevokeMyCredentialOrGrantInput) (*aitool.PreviewResult, error) {
	targetType := strings.TrimSpace(in.TargetType)
	targetID := strings.TrimSpace(in.TargetID)
	if targetID == "" {
		return nil, aitool.BadRequest("request.invalid", "targetId is required")
	}

	switch targetType {
	case "access_token", "oauth_application", "oauth_grant", "external_identity":
	default:
		return nil, aitool.BadRequest("request.invalid", "unsupported targetType")
	}

	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	if (targetType == "oauth_application" || targetType == "oauth_grant") && !h.isPrimaryNonImpersonatedSession(tc, tc.User.ID) {
		return nil, aitool.Forbidden("auth.impersonation_forbidden", "OAuth applications and grants require a primary non-impersonated session")
	}

	var summary string
	var label string

	switch targetType {
	case "access_token":
		var token model.AccessToken
		if err := tc.DB.First(&token, "id = ? AND user_id = ? AND source = ? AND revoked_at IS NULL", targetID, tc.User.ID, model.AccessTokenSourcePersonal).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("access_token.not_found", "personal access token was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		label = token.Name
		summary = fmt.Sprintf("Revoke personal access token %q (%s)", token.Name, token.ID)

	case "oauth_application":
		var app model.OAuthApplication
		if err := tc.DB.First(&app, "id = ? AND owner_user_id = ? AND revoked_at IS NULL", targetID, tc.User.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("oauth.application.not_found", "OAuth application was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		label = app.Name
		summary = fmt.Sprintf("Revoke owned OAuth application %q (%s) and all issued tokens", app.Name, app.ID)

	case "oauth_grant":
		var grant model.OAuthGrant
		if err := tc.DB.First(&grant, "id = ? AND user_id = ? AND revoked_at IS NULL", targetID, tc.User.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("oauth.grant.not_found", "OAuth grant was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		label = grant.ApplicationID
		summary = fmt.Sprintf("Revoke OAuth grant %s", grant.ID)

	case "external_identity":
		var user model.User
		if err := tc.DB.First(&user, "id = ?", tc.User.ID).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var identity model.ExternalIdentity
		if err := tc.DB.First(&identity, "id = ? AND user_id = ?", targetID, tc.User.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("external_identity.not_found", "external identity was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var count int64
		if err := tc.DB.Model(&model.ExternalIdentity{}).Where("user_id = ?", tc.User.ID).Count(&count).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if strings.TrimSpace(user.Password) == "" && count <= 1 {
			return nil, aitool.Conflict("auth.last_login_method_protected", "cannot unbind the last external identity when no local password is set")
		}
		label = identity.Email
		summary = fmt.Sprintf("Unbind external OIDC identity %s", identity.ID)
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: targetType,
			TargetID:       targetID,
			Changes: []aitool.PreviewChange{
				{Field: targetType, Before: label, After: nil, Action: "delete"},
			},
		},
		LockedPayload: identityTargetLock{TargetType: targetType, TargetID: targetID},
	}, nil
}

func (h *Handler) executeOpsRevokeMyCredentialOrGrant(tc *aitool.ToolContext, in opsRevokeMyCredentialOrGrantInput) (map[string]any, error) {
	targetType := strings.TrimSpace(in.TargetType)
	targetID := strings.TrimSpace(in.TargetID)
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}
	if len(tc.LockedPayload) > 0 {
		var locked identityTargetLock
		if err := tc.UnmarshalLockedPayload(&locked); err != nil {
			return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
		}
		if locked.TargetType != targetType || locked.TargetID != targetID {
			return nil, aitool.Conflict("ai.preview_lock_invalid", "target changed since preview")
		}
	}

	if (targetType == "oauth_application" || targetType == "oauth_grant") && !h.isPrimaryNonImpersonatedSession(tc, tc.User.ID) {
		return nil, aitool.Forbidden("auth.impersonation_forbidden", "OAuth applications and grants require a primary non-impersonated session")
	}

	now := time.Now()
	switch targetType {
	case "access_token":
		var token model.AccessToken
		if err := tc.DB.First(&token, "id = ? AND user_id = ? AND source = ? AND revoked_at IS NULL", targetID, tc.User.ID, model.AccessTokenSourcePersonal).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("access_token.not_found", "personal access token was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		token.RevokedAt = &now
		if err := tc.DB.Save(&token).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "access_token.revoke_failed", err.Error())
		}
		h.auditWithContext(tc.User.ID, "access_token.revoke", token.ID, true, "", tc.Context())

	case "oauth_application":
		app, err := deleteOwnedOAuthApplication(tc.DB, targetID, tc.User.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("oauth.application.not_found", "OAuth application was not found")
		}
		if err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "oauth.application.revoke_failed", err.Error())
		}
		h.auditWithContext(tc.User.ID, "oauth_application.revoke", app.ID, true, "", tc.Context())

	case "oauth_grant":
		var grant model.OAuthGrant
		if err := tc.DB.First(&grant, "id = ? AND user_id = ? AND revoked_at IS NULL", targetID, tc.User.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("oauth.grant.not_found", "OAuth grant was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if err := tc.DB.Transaction(func(tx *gorm.DB) error {
			return revokeOAuthGrant(tx, grant.ID, now)
		}); err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "oauth.grant.revoke_failed", err.Error())
		}
		h.auditWithContext(tc.User.ID, "oauth_grant.revoke", grant.ID, true, "", tc.Context())

	case "external_identity":
		err := tc.DB.Transaction(func(tx *gorm.DB) error {
			var lockedUser model.User
			if uErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedUser, "id = ?", tc.User.ID).Error; uErr != nil {
				return uErr
			}
			var identity model.ExternalIdentity
			if iErr := tx.First(&identity, "id = ? AND user_id = ?", targetID, lockedUser.ID).Error; iErr != nil {
				if errors.Is(iErr, gorm.ErrRecordNotFound) {
					return aitool.NotFound("external_identity.not_found", "external identity was not found")
				}
				return iErr
			}
			var count int64
			if cErr := tx.Model(&model.ExternalIdentity{}).Where("user_id = ?", lockedUser.ID).Count(&count).Error; cErr != nil {
				return cErr
			}
			if strings.TrimSpace(lockedUser.Password) == "" && count <= 1 {
				return aitool.Conflict("auth.last_login_method_protected", "cannot unbind the last external identity when no local password is set")
			}
			if dErr := tx.Delete(&identity).Error; dErr != nil {
				return dErr
			}
			return tx.Create(&model.AuditLog{
				ID:        id.New("aud"),
				UserID:    lockedUser.ID,
				Action:    "external_identity.unbind",
				Resource:  identity.ID,
				Success:   true,
				Message:   "external identity unbound",
				CreatedAt: now,
			}).Error
		})
		if err != nil {
			if _, ok := aitool.AsToolError(err); ok {
				return nil, err
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "external_identity.unbind_failed", err.Error())
		}

	default:
		return nil, aitool.BadRequest("request.invalid", "unsupported targetType")
	}

	return map[string]any{
		"id":         targetID,
		"targetType": targetType,
		"deleted":    true,
	}, nil
}

func (h *Handler) executeAdminListUsersAndAuth(tc *aitool.ToolContext, in adminListUsersAndAuthInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	sortBy := strings.TrimSpace(derefIdentityStr(in.SortBy))
	if sortBy == "" {
		sortBy = "createdAt"
	}
	allowedSort := map[string]string{
		"createdAt":   "created_at",
		"email":       "email",
		"name":        "name",
		"role":        "role",
		"passwordSet": "CASE WHEN password = '' THEN 0 ELSE 1 END",
		"status":      "disabled",
	}
	sortCol, ok := allowedSort[sortBy]
	if !ok {
		return nil, aitool.BadRequest("user.sort_invalid", "unsupported sortBy field")
	}
	sortOrder := strings.ToLower(strings.TrimSpace(derefIdentityStr(in.SortOrder)))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, aitool.BadRequest("user.sort_order_invalid", "sortOrder must be asc or desc")
	}

	query := tc.DB.Model(&model.User{})
	if uRef := strings.TrimSpace(derefIdentityStr(in.User)); uRef != "" {
		query = query.Where("id = ? OR LOWER(email) = LOWER(?)", uRef, uRef)
	}
	if search := strings.TrimSpace(derefIdentityStr(in.Search)); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(email) LIKE ? OR LOWER(name) LIKE ?", like, like)
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var users []model.User
	if err := query.Session(&gorm.Session{}).
		Order(fmt.Sprintf("%s %s, id desc", sortCol, sortOrder)).
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&users).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	balances, err := h.userWalletBalances(users, tc.Context())
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	items := make([]map[string]any, 0, len(users))
	for _, u := range users {
		items = append(items, userListResponse(u, balances[u.ID]))
	}

	env := aitool.PageEnvelope(items, page, pageSize, sortBy, sortOrder, total)

	if in.IncludeAuthSettings != nil && *in.IncludeAuthSettings {
		policy, pErr := h.ensureAdmissionPolicy(tc.Context())
		if pErr != nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.admission_policy_unavailable", pErr.Error())
		}
		regSettings := h.ensureAuthRegistrationSettings(tc.Context())

		var providers []model.AuthProvider
		if prErr := tc.DB.Order("is_default desc, created_at desc").Find(&providers).Error; prErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", prErr.Error())
		}

		publicBaseURL := h.externalBaseURL(tc.Gin)
		cbURL := ""
		if publicBaseURL != "" {
			cbURL = oidcCallbackURL(publicBaseURL)
		}

		mailSettings, mErr := platformmail.Get(tc.Context(), tc.DB)
		if mErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "mail.settings_load_failed", mErr.Error())
		}

		env["admissionPolicy"] = admissionPolicyResponse(policy)
		env["registrationSettings"] = authRegistrationSettingsResponse(regSettings)
		env["oidcProviders"] = authProviderResponses(providers)
		env["oidcCallbackUrl"] = map[string]any{
			"publicBaseUrl": publicBaseURL,
			"callbackUrl":   cbURL,
			"configured":    cbURL != "",
		}
		env["mailSettings"] = map[string]any{
			"host":                         mailSettings.Host,
			"port":                         mailSettings.Port,
			"security":                     mailSettings.Security,
			"username":                     mailSettings.Username,
			"passwordSet":                  secret.HasValue(mailSettings.PasswordRef),
			"fromAddress":                  mailSettings.FromAddress,
			"fromName":                     mailSettings.FromName,
			"personalEmailCooldownSeconds": mailSettings.PersonalEmailCooldownSeconds,
		}
	}

	return env, nil
}

func (h *Handler) previewAdminSaveUser(tc *aitool.ToolContext, in adminSaveUserInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	userRef := strings.TrimSpace(derefIdentityStr(in.User))
	if userRef == "" {
		email, name, role, lang, disabled, err := validateCreateUserParams(in)
		if err != nil {
			return nil, err
		}
		if tc.DB == nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
		}
		var count int64
		if err := tc.DB.Model(&model.User{}).Where("LOWER(email) = LOWER(?)", email).Count(&count).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if count > 0 {
			return nil, aitool.Conflict("user.email_conflict", "email is already in use")
		}

		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Create local user %s (%s) with role %s", name, email, role),
				TargetResource: "user",
				Changes: []aitool.PreviewChange{
					{Field: "email", After: email, Action: "create"},
					{Field: "name", After: name, Action: "create"},
					{Field: "role", After: role, Action: "create"},
					{Field: "language", After: lang, Action: "create"},
					{Field: "disabled", After: disabled, Action: "create"},
				},
				SensitiveKeys: []string{"password"},
			},
		}, nil
	}

	if in.Email == nil && in.Name == nil && in.Role == nil && in.Disabled == nil && in.Language == nil && in.Password == nil {
		return nil, aitool.BadRequest("user.update_noop", "at least one field must be provided when updating a user")
	}
	if in.Password != nil {
		if len(*in.Password) < 8 || len([]byte(*in.Password)) > 72 {
			return nil, aitool.BadRequest("password.invalid", "password must be between 8 and 72 bytes")
		}
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	target, err := resolveIdentityUser(tc.DB, userRef)
	if err != nil {
		return nil, err
	}

	changes, sensitiveKeys, warnings, err := h.validateAndDiffUserUpdate(tc.DB, tc.User, target, in)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Update user %s (%s)", target.Email, target.ID),
			TargetResource: "user",
			TargetID:       target.ID,
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
			Warnings:       warnings,
		},
		LockedPayload: identityTargetLock{TargetType: "user", TargetID: target.ID},
	}, nil
}

func (h *Handler) executeAdminSaveUser(tc *aitool.ToolContext, in adminSaveUserInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	userRef := strings.TrimSpace(derefIdentityStr(in.User))
	if userRef == "" {
		email, name, role, lang, disabled, err := validateCreateUserParams(in)
		if err != nil {
			return nil, err
		}
		if tc.DB == nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(*in.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "password.hash_failed", err.Error())
		}
		user := model.User{
			ID:       id.New("usr"),
			Email:    email,
			Name:     name,
			Role:     role,
			Language: lang,
			Password: string(passwordHash),
			Disabled: disabled,
		}
		err = tc.DB.Transaction(func(tx *gorm.DB) error {
			if _, lockErr := lockActiveUserRole(tx, tc.User.ID, authz.PlatformRoleAdmin); lockErr != nil {
				return aitool.Forbidden("config.admin.required", "platform administrator role is required")
			}
			var count int64
			if cErr := tx.Model(&model.User{}).Where("LOWER(email) = LOWER(?)", email).Count(&count).Error; cErr != nil {
				return cErr
			}
			if count > 0 {
				return aitool.Conflict("user.email_conflict", "email is already in use")
			}
			if cErr := tx.Create(&user).Error; cErr != nil {
				return cErr
			}
			if pErr := createDefaultUserProject(tx, user); pErr != nil {
				return pErr
			}
			return tx.Create(&model.AuditLog{
				ID:        id.New("aud"),
				UserID:    tc.User.ID,
				Action:    "user.create",
				Resource:  user.ID,
				Success:   true,
				Message:   "local user created",
				CreatedAt: time.Now(),
			}).Error
		})
		if err != nil {
			if _, ok := aitool.AsToolError(err); ok {
				return nil, err
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "user.create_failed", err.Error())
		}
		return userListResponse(user, decimal.Zero), nil
	}

	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}
	target, err := resolveIdentityUser(tc.DB, userRef)
	if err != nil {
		return nil, err
	}
	if len(tc.LockedPayload) > 0 {
		var locked identityTargetLock
		if lErr := tc.UnmarshalLockedPayload(&locked); lErr != nil {
			return nil, aitool.Conflict("ai.preview_lock_invalid", lErr.Error())
		}
		if locked.TargetID != target.ID {
			return nil, aitool.Conflict("ai.preview_lock_invalid", "target user changed since preview")
		}
	}

	var passwordHash string
	if in.Password != nil {
		if len(*in.Password) < 8 || len([]byte(*in.Password)) > 72 {
			return nil, aitool.BadRequest("password.invalid", "password must be between 8 and 72 bytes")
		}
		hashed, hErr := bcrypt.GenerateFromPassword([]byte(*in.Password), bcrypt.DefaultCost)
		if hErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "password.hash_failed", hErr.Error())
		}
		passwordHash = string(hashed)
	}

	var updated model.User
	err = tc.DB.Transaction(func(tx *gorm.DB) error {
		if _, lockErr := lockActiveUserRole(tx, tc.User.ID, authz.PlatformRoleAdmin); lockErr != nil {
			return aitool.Forbidden("config.admin.required", "platform administrator role is required")
		}
		var stored model.User
		if fErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&stored, "id = ?", target.ID).Error; fErr != nil {
			if errors.Is(fErr, gorm.ErrRecordNotFound) {
				return aitool.NotFound("user.not_found", "user was not found")
			}
			return fErr
		}
		if _, _, _, vErr := h.validateAndDiffUserUpdate(tx, tc.User, stored, in); vErr != nil {
			return vErr
		}

		nextEmail := stored.Email
		if in.Email != nil {
			em, eErr := normalizedRegistrationEmail(*in.Email)
			if eErr != nil {
				return aitool.BadRequest("user.email_invalid", eErr.Error())
			}
			nextEmail = em
		}
		nextName := stored.Name
		if in.Name != nil {
			nextName = strings.TrimSpace(*in.Name)
		}
		nextRole := stored.Role
		if in.Role != nil {
			nextRole = normalizeUserRole(*in.Role)
		}
		nextLang := stored.Language
		if in.Language != nil {
			nextLang = normalizeLanguage(*in.Language)
		}
		nextDisabled := stored.Disabled
		if in.Disabled != nil {
			nextDisabled = *in.Disabled
		}

		if stored.Role == authz.PlatformRoleAdmin && !stored.Disabled && (nextRole != authz.PlatformRoleAdmin || nextDisabled) {
			var otherAdmins []model.User
			if aErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("role = ? AND disabled = ? AND id <> ?", authz.PlatformRoleAdmin, false, stored.ID).
				Find(&otherAdmins).Error; aErr != nil {
				return aErr
			}
			if len(otherAdmins) == 0 {
				return aitool.Conflict("auth.last_platform_admin_protected", "cannot disable or demote the last active platform administrator")
			}
		}

		passwordChanged := passwordHash != ""
		revokeAuth := shouldRevokeUserAuthentication(stored.Role, nextRole, stored.Disabled, nextDisabled, passwordChanged)

		stored.Email = nextEmail
		stored.Name = nextName
		stored.Role = nextRole
		stored.Language = nextLang
		stored.Disabled = nextDisabled
		if passwordChanged {
			stored.Password = passwordHash
		}

		if sErr := tx.Save(&stored).Error; sErr != nil {
			return sErr
		}
		now := time.Now()
		if revokeAuth {
			if rErr := revokeUserAuthentication(tx, stored.ID); rErr != nil {
				return rErr
			}
			if tErr := tx.Model(&model.AccessToken{}).Where("user_id = ? AND revoked_at IS NULL", stored.ID).Update("revoked_at", now).Error; tErr != nil {
				return tErr
			}
			if rtErr := tx.Model(&model.OAuthRefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", stored.ID).Update("revoked_at", now).Error; rtErr != nil {
				return rtErr
			}
		}
		updated = stored
		return tx.Create(&model.AuditLog{
			ID:        id.New("aud"),
			UserID:    tc.User.ID,
			Action:    "user.update",
			Resource:  stored.ID,
			Success:   true,
			Message:   "user updated",
			CreatedAt: now,
		}).Error
	})
	if err != nil {
		if _, ok := aitool.AsToolError(err); ok {
			return nil, err
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "user.update_failed", err.Error())
	}

	balances, _ := h.userWalletBalances([]model.User{updated}, tc.Context())
	return userListResponse(updated, balances[updated.ID]), nil
}

func validateCreateUserParams(in adminSaveUserInput) (string, string, string, string, bool, error) {
	emailRaw := strings.TrimSpace(derefIdentityStr(in.Email))
	if emailRaw == "" {
		return "", "", "", "", false, aitool.BadRequest("user.email_required", "email is required when creating a user")
	}
	email, err := normalizedRegistrationEmail(emailRaw)
	if err != nil {
		return "", "", "", "", false, aitool.BadRequest("user.email_invalid", err.Error())
	}
	name := strings.TrimSpace(derefIdentityStr(in.Name))
	if name == "" {
		return "", "", "", "", false, aitool.BadRequest("user.name_required", "name is required when creating a user")
	}
	if in.Password == nil || len(*in.Password) < 8 || len([]byte(*in.Password)) > 72 {
		return "", "", "", "", false, aitool.BadRequest("password.invalid", "password of 8..72 bytes is required when creating a user")
	}
	role := authz.PlatformRoleUser
	if in.Role != nil {
		if *in.Role != authz.PlatformRoleAdmin && *in.Role != authz.PlatformRoleUser {
			return "", "", "", "", false, aitool.BadRequest("user.role_invalid", "role must be platform_admin or user")
		}
		role = normalizeUserRole(*in.Role)
	}
	lang := "zh-CN"
	if in.Language != nil {
		if *in.Language != "zh-CN" && *in.Language != "en-US" {
			return "", "", "", "", false, aitool.BadRequest("user.language_invalid", "language must be zh-CN or en-US")
		}
		lang = normalizeLanguage(*in.Language)
	}
	disabled := in.Disabled != nil && *in.Disabled
	return email, name, role, lang, disabled, nil
}

func (h *Handler) validateAndDiffUserUpdate(
	db *gorm.DB,
	actor model.User,
	stored model.User,
	in adminSaveUserInput,
) ([]aitool.PreviewChange, []string, []string, error) {
	if actor.ID == stored.ID {
		if (in.Disabled != nil && *in.Disabled) ||
			(in.Role != nil && normalizeUserRole(*in.Role) != authz.PlatformRoleAdmin) ||
			(in.Password != nil && *in.Password != "") {
			return nil, nil, nil, aitool.Conflict(
				"auth.self_credential_mutation_forbidden",
				"current administrator cannot disable, demote, or reset password on their own account through AI tools",
			)
		}
	}

	var changes []aitool.PreviewChange
	var sensitiveKeys []string
	var warnings []string

	if in.Email != nil {
		email, err := normalizedRegistrationEmail(*in.Email)
		if err != nil {
			return nil, nil, nil, aitool.BadRequest("user.email_invalid", err.Error())
		}
		if !strings.EqualFold(email, stored.Email) {
			var count int64
			if cErr := db.Model(&model.User{}).Where("LOWER(email) = LOWER(?) AND id <> ?", email, stored.ID).Count(&count).Error; cErr != nil {
				return nil, nil, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", cErr.Error())
			}
			if count > 0 {
				return nil, nil, nil, aitool.Conflict("user.email_conflict", "email is already in use")
			}
			changes = append(changes, aitool.PreviewChange{Field: "email", Before: stored.Email, After: email, Action: "update"})
		}
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, nil, nil, aitool.BadRequest("user.name_invalid", "name cannot be empty")
		}
		if name != stored.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: stored.Name, After: name, Action: "update"})
		}
	}
	nextRole := stored.Role
	if in.Role != nil {
		if *in.Role != authz.PlatformRoleAdmin && *in.Role != authz.PlatformRoleUser {
			return nil, nil, nil, aitool.BadRequest("user.role_invalid", "role must be platform_admin or user")
		}
		nextRole = normalizeUserRole(*in.Role)
		if nextRole != stored.Role {
			changes = append(changes, aitool.PreviewChange{Field: "role", Before: stored.Role, After: nextRole, Action: "update"})
		}
	}
	nextDisabled := stored.Disabled
	if in.Disabled != nil {
		nextDisabled = *in.Disabled
		if nextDisabled != stored.Disabled {
			changes = append(changes, aitool.PreviewChange{Field: "disabled", Before: stored.Disabled, After: nextDisabled, Action: "update"})
		}
	}
	if in.Language != nil {
		if *in.Language != "zh-CN" && *in.Language != "en-US" {
			return nil, nil, nil, aitool.BadRequest("user.language_invalid", "language must be zh-CN or en-US")
		}
		nextLang := normalizeLanguage(*in.Language)
		if nextLang != normalizeLanguage(stored.Language) {
			changes = append(changes, aitool.PreviewChange{Field: "language", Before: normalizeLanguage(stored.Language), After: nextLang, Action: "update"})
		}
	}
	passwordChanged := in.Password != nil && *in.Password != ""
	if passwordChanged {
		sensitiveKeys = append(sensitiveKeys, "password")
	}

	if stored.Role == authz.PlatformRoleAdmin && !stored.Disabled && (nextRole != authz.PlatformRoleAdmin || nextDisabled) {
		var otherAdminCount int64
		if err := db.Model(&model.User{}).
			Where("role = ? AND disabled = ? AND id <> ?", authz.PlatformRoleAdmin, false, stored.ID).
			Count(&otherAdminCount).Error; err != nil {
			return nil, nil, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if otherAdminCount == 0 {
			return nil, nil, nil, aitool.Conflict("auth.last_platform_admin_protected", "cannot disable or demote the last active platform administrator")
		}
	}

	if shouldRevokeUserAuthentication(stored.Role, nextRole, stored.Disabled, nextDisabled, passwordChanged) {
		warnings = append(warnings, "Changing role, disabling account, or resetting password will immediately revoke all active sessions, personal access tokens, and OAuth tokens for this user.")
	}

	return changes, sensitiveKeys, warnings, nil
}

func (h *Handler) previewAdminConfigureAuthPolicy(tc *aitool.ToolContext, in adminConfigureAuthPolicyInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if !hasAuthPolicyPatchField(in) {
		return nil, aitool.BadRequest("auth.policy_noop", "at least one auth policy or registration field must be provided")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	policy, err := h.ensureAdmissionPolicy(tc.Context())
	if err != nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.admission_policy_unavailable", err.Error())
	}
	regSettings := h.ensureAuthRegistrationSettings(tc.Context())

	nextPolicy, nextReg, changes, err := applyAuthPolicyPatch(tc.Context(), tc.DB, policy, regSettings, in)
	if err != nil {
		return nil, err
	}
	_ = nextPolicy
	_ = nextReg

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        "Update platform authentication admission policy and registration settings",
			TargetResource: "auth_policy",
			TargetID:       policy.ID,
			Changes:        changes,
		},
	}, nil
}

func (h *Handler) executeAdminConfigureAuthPolicy(tc *aitool.ToolContext, in adminConfigureAuthPolicyInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if !hasAuthPolicyPatchField(in) {
		return nil, aitool.BadRequest("auth.policy_noop", "at least one auth policy or registration field must be provided")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	if _, err := h.ensureAdmissionPolicy(tc.Context()); err != nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.admission_policy_unavailable", err.Error())
	}
	_ = h.ensureAuthRegistrationSettings(tc.Context())

	var savedPolicy model.AuthAdmissionPolicy
	var savedReg model.AuthRegistrationSettings

	err := tc.DB.Transaction(func(tx *gorm.DB) error {
		if _, lockErr := lockActiveUserRole(tx, tc.User.ID, authz.PlatformRoleAdmin); lockErr != nil {
			return aitool.Forbidden("config.admin.required", "platform administrator role is required")
		}
		var policy model.AuthAdmissionPolicy
		if pErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, "id = ?", defaultAdmissionPolicyID).Error; pErr != nil {
			return pErr
		}
		var reg model.AuthRegistrationSettings
		if rErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&reg, "id = ?", authRegistrationSettingsID).Error; rErr != nil {
			return rErr
		}

		nextPolicy, nextReg, _, patchErr := applyAuthPolicyPatch(tc.Context(), tx, policy, reg, in)
		if patchErr != nil {
			return patchErr
		}
		if sErr := tx.Save(&nextPolicy).Error; sErr != nil {
			return sErr
		}
		if rErr := tx.Save(&nextReg).Error; rErr != nil {
			return rErr
		}
		savedPolicy = nextPolicy
		savedReg = nextReg
		return tx.Create(&model.AuditLog{
			ID:        id.New("aud"),
			UserID:    tc.User.ID,
			Action:    "auth.policy.update",
			Resource:  savedPolicy.ID,
			Success:   true,
			Message:   "authentication admission policy and registration settings updated",
			CreatedAt: time.Now(),
		}).Error
	})
	if err != nil {
		if _, ok := aitool.AsToolError(err); ok {
			return nil, err
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "auth.policy_update_failed", err.Error())
	}

	return map[string]any{
		"admissionPolicy":      admissionPolicyResponse(savedPolicy),
		"registrationSettings": authRegistrationSettingsResponse(savedReg),
	}, nil
}

func applyAuthPolicyPatch(
	ctx anyContext,
	db *gorm.DB,
	policy model.AuthAdmissionPolicy,
	reg model.AuthRegistrationSettings,
	in adminConfigureAuthPolicyInput,
) (model.AuthAdmissionPolicy, model.AuthRegistrationSettings, []aitool.PreviewChange, error) {
	var changes []aitool.PreviewChange

	if in.AllowLocalLogin != nil && *in.AllowLocalLogin != policy.AllowLocalLogin {
		changes = append(changes, aitool.PreviewChange{Field: "allowLocalLogin", Before: policy.AllowLocalLogin, After: *in.AllowLocalLogin, Action: "update"})
		policy.AllowLocalLogin = *in.AllowLocalLogin
	}
	if in.AllowOIDCLogin != nil && *in.AllowOIDCLogin != policy.AllowOIDCLogin {
		changes = append(changes, aitool.PreviewChange{Field: "allowOidcLogin", Before: policy.AllowOIDCLogin, After: *in.AllowOIDCLogin, Action: "update"})
		policy.AllowOIDCLogin = *in.AllowOIDCLogin
	}
	if in.RequireVerifiedOIDCEmail != nil && *in.RequireVerifiedOIDCEmail != policy.RequireVerifiedOIDCEmail {
		changes = append(changes, aitool.PreviewChange{Field: "requireVerifiedOidcEmail", Before: policy.RequireVerifiedOIDCEmail, After: *in.RequireVerifiedOIDCEmail, Action: "update"})
		policy.RequireVerifiedOIDCEmail = *in.RequireVerifiedOIDCEmail
	}
	if in.DefaultRole != nil {
		if *in.DefaultRole != authz.PlatformRoleUser && *in.DefaultRole != authz.PlatformRoleAdmin {
			return policy, reg, nil, aitool.BadRequest("auth.default_role_invalid", "defaultRole must be user or platform_admin")
		}
		role := normalizeUserRole(*in.DefaultRole)
		if role != policy.DefaultRole {
			changes = append(changes, aitool.PreviewChange{Field: "defaultRole", Before: policy.DefaultRole, After: role, Action: "update"})
			policy.DefaultRole = role
		}
	}

	if in.ClearAllowedEmailDomains != nil || len(in.AddAllowedEmailDomains) > 0 || len(in.RemoveAllowedEmailDomains) > 0 {
		before := jsonList(splitCSV(policy.AllowedEmailDomains))
		next := patchNormalizedCSVList(before, in.ClearAllowedEmailDomains != nil && *in.ClearAllowedEmailDomains, in.AddAllowedEmailDomains, in.RemoveAllowedEmailDomains, false)
		policy.AllowedEmailDomains = strings.Join(next, ",")
		changes = append(changes, aitool.PreviewChange{Field: "allowedEmailDomains", Before: before, After: next, Action: "update"})
	}
	if in.ClearAllowedOIDCGroups != nil || len(in.AddAllowedOIDCGroups) > 0 || len(in.RemoveAllowedOIDCGroups) > 0 {
		before := jsonList(splitCSV(policy.AllowedOIDCGroups))
		next := patchNormalizedCSVList(before, in.ClearAllowedOIDCGroups != nil && *in.ClearAllowedOIDCGroups, in.AddAllowedOIDCGroups, in.RemoveAllowedOIDCGroups, true)
		policy.AllowedOIDCGroups = strings.Join(next, ",")
		changes = append(changes, aitool.PreviewChange{Field: "allowedOidcGroups", Before: before, After: next, Action: "update"})
	}
	if in.ClearInvitedEmails != nil || len(in.AddInvitedEmails) > 0 || len(in.RemoveInvitedEmails) > 0 {
		before := jsonList(splitCSV(policy.InvitedEmails))
		next := patchNormalizedCSVList(before, in.ClearInvitedEmails != nil && *in.ClearInvitedEmails, in.AddInvitedEmails, in.RemoveInvitedEmails, false)
		policy.InvitedEmails = strings.Join(next, ",")
		changes = append(changes, aitool.PreviewChange{Field: "invitedEmails", Before: before, After: next, Action: "update"})
	}

	if in.AllowEmailRegistration != nil && *in.AllowEmailRegistration != reg.AllowEmailRegistration {
		changes = append(changes, aitool.PreviewChange{Field: "allowEmailRegistration", Before: reg.AllowEmailRegistration, After: *in.AllowEmailRegistration, Action: "update"})
		reg.AllowEmailRegistration = *in.AllowEmailRegistration
	}
	if in.AllowOIDCRegistration != nil && *in.AllowOIDCRegistration != reg.AllowOIDCRegistration {
		changes = append(changes, aitool.PreviewChange{Field: "allowOidcRegistration", Before: reg.AllowOIDCRegistration, After: *in.AllowOIDCRegistration, Action: "update"})
		reg.AllowOIDCRegistration = *in.AllowOIDCRegistration
	}
	if in.AllowExternalIdentityPassword != nil && *in.AllowExternalIdentityPassword != reg.AllowExternalIdentityPassword {
		changes = append(changes, aitool.PreviewChange{Field: "allowExternalIdentityPassword", Before: reg.AllowExternalIdentityPassword, After: *in.AllowExternalIdentityPassword, Action: "update"})
		reg.AllowExternalIdentityPassword = *in.AllowExternalIdentityPassword
	}

	// Anti-lockout guard
	if !policy.AllowLocalLogin && !policy.AllowOIDCLogin {
		return policy, reg, nil, aitool.Conflict("auth.login_method_lockout_forbidden", "cannot disable both local login and OIDC login")
	}
	if !policy.AllowLocalLogin {
		var enabledOIDCCount int64
		if err := db.Model(&model.AuthProvider{}).Where("enabled = ?", true).Count(&enabledOIDCCount).Error; err != nil {
			return policy, reg, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if enabledOIDCCount == 0 {
			return policy, reg, nil, aitool.Conflict("auth.login_method_lockout_forbidden", "cannot disable local login when no enabled OIDC provider exists")
		}
	}

	if reg.AllowEmailRegistration {
		mailSettings, mErr := platformmail.Get(ctx, db)
		if mErr != nil {
			return policy, reg, nil, aitool.NewError(http.StatusInternalServerError, "registration.mail_settings_failed", mErr.Error())
		}
		if vErr := platformmail.Validate(mailSettings, false); vErr != nil {
			return policy, reg, nil, aitool.BadRequest("registration.mail_not_configured", vErr.Error())
		}
	}

	return policy, reg, changes, nil
}

func (h *Handler) previewAdminSaveOIDCProvider(tc *aitool.ToolContext, in adminSaveOIDCProviderInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	for _, rem := range in.RemoveScopes {
		if strings.EqualFold(strings.TrimSpace(rem), "openid") {
			return nil, aitool.BadRequest("argument_invalid", "openid scope is mandatory and cannot be removed")
		}
	}

	providerRef := strings.TrimSpace(derefIdentityStr(in.Provider))
	if providerRef == "" {
		next, err := validateCreateOIDCProviderParams(in)
		if err != nil {
			return nil, err
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Create OIDC provider %q (%s)", next.Name, next.IssuerURL),
				TargetResource: "oidc_provider",
				Changes: []aitool.PreviewChange{
					{Field: "name", After: next.Name, Action: "create"},
					{Field: "issuerUrl", After: next.IssuerURL, Action: "create"},
					{Field: "clientId", After: next.ClientID, Action: "create"},
					{Field: "scopes", After: next.Scopes, Action: "create"},
					{Field: "enabled", After: next.Enabled, Action: "create"},
					{Field: "isDefault", After: next.IsDefault, Action: "create"},
				},
				SensitiveKeys: []string{"clientSecret"},
			},
		}, nil
	}

	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}
	existing, err := resolveOIDCProvider(tc.DB, providerRef)
	if err != nil {
		return nil, err
	}
	next, changes, sensitiveKeys, err := h.mergeAndValidateOIDCProviderPatch(tc, tc.DB, existing, in)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Update OIDC provider %q (%s)", next.Name, existing.ID),
			TargetResource: "oidc_provider",
			TargetID:       existing.ID,
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
		},
		LockedPayload: identityTargetLock{TargetType: "oidc_provider", TargetID: existing.ID},
	}, nil
}

func (h *Handler) executeAdminSaveOIDCProvider(tc *aitool.ToolContext, in adminSaveOIDCProviderInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	for _, rem := range in.RemoveScopes {
		if strings.EqualFold(strings.TrimSpace(rem), "openid") {
			return nil, aitool.BadRequest("argument_invalid", "openid scope is mandatory and cannot be removed")
		}
	}

	providerRef := strings.TrimSpace(derefIdentityStr(in.Provider))
	if providerRef == "" {
		next, err := validateCreateOIDCProviderParams(in)
		if err != nil {
			return nil, err
		}
		if tc.DB == nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
		}
		providerID := id.New("ap")
		next.ID = providerID
		secretVal := strings.TrimSpace(*in.ClientSecret)

		err = tc.DB.Transaction(func(tx *gorm.DB) error {
			if _, lockErr := lockActiveUserRole(tx, tc.User.ID, authz.PlatformRoleAdmin); lockErr != nil {
				return aitool.Forbidden("config.admin.required", "platform administrator role is required")
			}
			ref, sErr := h.secrets.StoreContextWithDB(tc.Context(), tx, secretVal, tc.User.ID, "auth_provider:"+providerID+":client_secret")
			if sErr != nil || ref == "" {
				return aitool.NewError(http.StatusInternalServerError, "secret.store_failed", "failed to store OIDC clientSecret")
			}
			next.ClientSecretRef = ref
			if next.IsDefault {
				if uErr := tx.Model(&model.AuthProvider{}).Where("is_default = ?", true).Update("is_default", false).Error; uErr != nil {
					return uErr
				}
			}
			if cErr := tx.Select("*").Create(&next).Error; cErr != nil {
				return cErr
			}
			return tx.Create(&model.AuditLog{
				ID:        id.New("aud"),
				UserID:    tc.User.ID,
				Action:    "oidc.provider.create",
				Resource:  next.ID,
				Success:   true,
				Message:   "OIDC provider created",
				CreatedAt: time.Now(),
			}).Error
		})
		if err != nil {
			if _, ok := aitool.AsToolError(err); ok {
				return nil, err
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "oidc.provider_create_failed", err.Error())
		}
		return structToMap(authProviderResponse(next)), nil
	}

	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}
	existing, err := resolveOIDCProvider(tc.DB, providerRef)
	if err != nil {
		return nil, err
	}
	if len(tc.LockedPayload) > 0 {
		var locked identityTargetLock
		if lErr := tc.UnmarshalLockedPayload(&locked); lErr != nil {
			return nil, aitool.Conflict("ai.preview_lock_invalid", lErr.Error())
		}
		if locked.TargetID != existing.ID {
			return nil, aitool.Conflict("ai.preview_lock_invalid", "OIDC provider changed since preview")
		}
	}

	var saved model.AuthProvider
	err = tc.DB.Transaction(func(tx *gorm.DB) error {
		if _, lockErr := lockActiveUserRole(tx, tc.User.ID, authz.PlatformRoleAdmin); lockErr != nil {
			return aitool.Forbidden("config.admin.required", "platform administrator role is required")
		}
		var lockedProv model.AuthProvider
		if fErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProv, "id = ?", existing.ID).Error; fErr != nil {
			if errors.Is(fErr, gorm.ErrRecordNotFound) {
				return aitool.NotFound("auth_provider.not_found", "OIDC provider was not found")
			}
			return fErr
		}
		next, _, _, mErr := h.mergeAndValidateOIDCProviderPatch(tc, tx, lockedProv, in)
		if mErr != nil {
			return mErr
		}

		if in.ClientSecret != nil && strings.TrimSpace(*in.ClientSecret) != "" {
			ref, sErr := h.secrets.StoreContextWithDB(tc.Context(), tx, strings.TrimSpace(*in.ClientSecret), tc.User.ID, "auth_provider:"+lockedProv.ID+":client_secret")
			if sErr != nil || ref == "" {
				return aitool.NewError(http.StatusInternalServerError, "secret.store_failed", "failed to store OIDC clientSecret")
			}
			next.ClientSecretRef = ref
		} else {
			next.ClientSecretRef = lockedProv.ClientSecretRef
		}

		if next.IsDefault {
			if uErr := tx.Model(&model.AuthProvider{}).Where("id <> ? AND is_default = ?", next.ID, true).Update("is_default", false).Error; uErr != nil {
				return uErr
			}
		}
		if sErr := tx.Save(&next).Error; sErr != nil {
			return sErr
		}
		saved = next
		return tx.Create(&model.AuditLog{
			ID:        id.New("aud"),
			UserID:    tc.User.ID,
			Action:    "oidc.provider.update",
			Resource:  saved.ID,
			Success:   true,
			Message:   "OIDC provider updated",
			CreatedAt: time.Now(),
		}).Error
	})
	if err != nil {
		if _, ok := aitool.AsToolError(err); ok {
			return nil, err
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "oidc.provider_update_failed", err.Error())
	}

	return structToMap(authProviderResponse(saved)), nil
}

func validateCreateOIDCProviderParams(in adminSaveOIDCProviderInput) (model.AuthProvider, error) {
	name := strings.TrimSpace(derefIdentityStr(in.Name))
	issuer := strings.TrimRight(strings.TrimSpace(derefIdentityStr(in.IssuerURL)), "/")
	clientID := strings.TrimSpace(derefIdentityStr(in.ClientID))
	clientSecret := strings.TrimSpace(derefIdentityStr(in.ClientSecret))
	if name == "" || issuer == "" || clientID == "" || clientSecret == "" {
		return model.AuthProvider{}, aitool.BadRequest("argument_invalid", "name, issuerUrl, clientId, and clientSecret are required when creating an OIDC provider")
	}
	if !isValidIssuerHTTPURL(issuer) {
		return model.AuthProvider{}, aitool.BadRequest("argument_invalid", "issuerUrl must be a valid HTTP or HTTPS URL")
	}

	baseScopes := []string{"openid", "profile", "email"}
	if len(in.AddScopes) > 0 {
		baseScopes = append([]string{"openid"}, in.AddScopes...)
	}
	scopes := mergeOIDCScopes(baseScopes, nil, in.RemoveScopes)
	enabled := in.Enabled == nil || *in.Enabled
	isDefault := in.IsDefault != nil && *in.IsDefault

	return model.AuthProvider{
		Type:          "oidc",
		Name:          name,
		Enabled:       enabled,
		IssuerURL:     issuer,
		ClientID:      clientID,
		Scopes:        strings.Join(scopes, " "),
		EmailClaim:    fallback(strings.TrimSpace(derefIdentityStr(in.EmailClaim)), "email"),
		UsernameClaim: fallback(strings.TrimSpace(derefIdentityStr(in.UsernameClaim)), "preferred_username"),
		GroupClaim:    fallback(strings.TrimSpace(derefIdentityStr(in.GroupClaim)), "groups"),
		IsDefault:     isDefault,
	}, nil
}

func (h *Handler) mergeAndValidateOIDCProviderPatch(
	tc *aitool.ToolContext,
	db *gorm.DB,
	existing model.AuthProvider,
	in adminSaveOIDCProviderInput,
) (model.AuthProvider, []aitool.PreviewChange, []string, error) {
	if in.Name == nil && in.IssuerURL == nil && in.ClientID == nil && in.ClientSecret == nil &&
		len(in.AddScopes) == 0 && len(in.RemoveScopes) == 0 && in.EmailClaim == nil &&
		in.UsernameClaim == nil && in.GroupClaim == nil && in.Enabled == nil && in.IsDefault == nil {
		return existing, nil, nil, aitool.BadRequest("oidc.provider_noop", "at least one field must be provided when updating an OIDC provider")
	}

	next := existing
	var changes []aitool.PreviewChange
	var sensitiveKeys []string

	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			return existing, nil, nil, aitool.BadRequest("argument_invalid", "name cannot be empty")
		}
		if n != existing.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: n, Action: "update"})
			next.Name = n
		}
	}
	if in.IssuerURL != nil {
		iss := strings.TrimRight(strings.TrimSpace(*in.IssuerURL), "/")
		if !isValidIssuerHTTPURL(iss) {
			return existing, nil, nil, aitool.BadRequest("argument_invalid", "issuerUrl must be a valid HTTP or HTTPS URL")
		}
		if iss != existing.IssuerURL {
			changes = append(changes, aitool.PreviewChange{Field: "issuerUrl", Before: existing.IssuerURL, After: iss, Action: "update"})
			next.IssuerURL = iss
		}
	}
	if in.ClientID != nil {
		cid := strings.TrimSpace(*in.ClientID)
		if cid == "" {
			return existing, nil, nil, aitool.BadRequest("argument_invalid", "clientId cannot be empty")
		}
		if cid != existing.ClientID {
			changes = append(changes, aitool.PreviewChange{Field: "clientId", Before: existing.ClientID, After: cid, Action: "update"})
			next.ClientID = cid
		}
	}
	if in.ClientSecret != nil {
		if strings.TrimSpace(*in.ClientSecret) == "" {
			return existing, nil, nil, aitool.BadRequest("argument_invalid", "clientSecret cannot be an empty string")
		}
		sensitiveKeys = append(sensitiveKeys, "clientSecret")
	}
	if len(in.AddScopes) > 0 || len(in.RemoveScopes) > 0 {
		merged := strings.Join(mergeOIDCScopes(strings.Fields(existing.Scopes), in.AddScopes, in.RemoveScopes), " ")
		if merged != existing.Scopes {
			changes = append(changes, aitool.PreviewChange{Field: "scopes", Before: existing.Scopes, After: merged, Action: "update"})
			next.Scopes = merged
		}
	}
	if in.EmailClaim != nil {
		ec := strings.TrimSpace(*in.EmailClaim)
		if ec == "" {
			return existing, nil, nil, aitool.BadRequest("argument_invalid", "emailClaim cannot be empty")
		}
		if ec != existing.EmailClaim {
			changes = append(changes, aitool.PreviewChange{Field: "emailClaim", Before: existing.EmailClaim, After: ec, Action: "update"})
			next.EmailClaim = ec
		}
	}
	if in.UsernameClaim != nil {
		uc := strings.TrimSpace(*in.UsernameClaim)
		if uc == "" {
			return existing, nil, nil, aitool.BadRequest("argument_invalid", "usernameClaim cannot be empty")
		}
		if uc != existing.UsernameClaim {
			changes = append(changes, aitool.PreviewChange{Field: "usernameClaim", Before: existing.UsernameClaim, After: uc, Action: "update"})
			next.UsernameClaim = uc
		}
	}
	if in.GroupClaim != nil {
		gc := strings.TrimSpace(*in.GroupClaim)
		if gc == "" {
			return existing, nil, nil, aitool.BadRequest("argument_invalid", "groupClaim cannot be empty")
		}
		if gc != existing.GroupClaim {
			changes = append(changes, aitool.PreviewChange{Field: "groupClaim", Before: existing.GroupClaim, After: gc, Action: "update"})
			next.GroupClaim = gc
		}
	}
	if in.Enabled != nil && *in.Enabled != existing.Enabled {
		changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: *in.Enabled, Action: "update"})
		next.Enabled = *in.Enabled
	}
	if in.IsDefault != nil && *in.IsDefault != existing.IsDefault {
		changes = append(changes, aitool.PreviewChange{Field: "isDefault", Before: existing.IsDefault, After: *in.IsDefault, Action: "update"})
		next.IsDefault = *in.IsDefault
	}

	// Symmetric anti-lockout guard when disabling an OIDC provider while local login is disabled
	if existing.Enabled && !next.Enabled {
		var policy model.AuthAdmissionPolicy
		pErr := db.First(&policy, "id = ?", defaultAdmissionPolicyID).Error
		if pErr == nil && !policy.AllowLocalLogin {
			var otherEnabledCount int64
			if cErr := db.Model(&model.AuthProvider{}).Where("id <> ? AND enabled = ?", existing.ID, true).Count(&otherEnabledCount).Error; cErr != nil {
				return existing, nil, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", cErr.Error())
			}
			if otherEnabledCount == 0 {
				return existing, nil, nil, aitool.Conflict(
					"auth.login_method_lockout_forbidden",
					"cannot disable the last enabled OIDC provider while local login is disabled",
				)
			}
		}
	}
	_ = tc
	return next, changes, sensitiveKeys, nil
}

func (h *Handler) isPrimaryNonImpersonatedSession(tc *aitool.ToolContext, userID string) bool {
	if tc == nil || tc.DB == nil {
		return false
	}
	if sid := strings.TrimSpace(tc.SessionID); sid != "" {
		var session model.UserSession
		if err := tc.DB.First(&session, "id = ? AND user_id = ? AND expires_at > ?", sid, userID, time.Now()).Error; err != nil {
			return false
		}
		return strings.TrimSpace(session.ImpersonatorID) == ""
	}
	session, ok := h.currentSessionFromCookie(tc.Gin)
	return ok && session.UserID == userID && strings.TrimSpace(session.ImpersonatorID) == ""
}

func resolveIdentityUser(db *gorm.DB, ref string) (model.User, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.User{}, aitool.BadRequest("user.required", "user is required")
	}
	var user model.User
	if err := db.Where("id = ? OR LOWER(email) = LOWER(?)", ref, ref).
		Order(gorm.Expr("CASE WHEN id = ? THEN 0 ELSE 1 END", ref)).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, aitool.NotFound("user.not_found", "user was not found")
		}
		return model.User{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return user, nil
}

func resolveOIDCProvider(db *gorm.DB, ref string) (model.AuthProvider, error) {
	var providers []model.AuthProvider
	if err := db.Order("created_at asc").Find(&providers).Error; err != nil {
		return model.AuthProvider{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		providers,
		"oidc_provider",
		"auth_provider.not_found",
		ref,
		func(p model.AuthProvider) string { return p.ID },
		func(p model.AuthProvider) string { return p.Name },
	)
}

func mergeOIDCScopes(base, add, remove []string) []string {
	removed := make(map[string]bool, len(remove))
	for _, r := range remove {
		norm := strings.TrimSpace(r)
		if norm != "" && !strings.EqualFold(norm, "openid") {
			removed[norm] = true
		}
	}
	seen := map[string]bool{"openid": true}
	out := []string{"openid"}
	for _, list := range [][]string{base, add} {
		for _, item := range list {
			for _, token := range strings.Fields(item) {
				if token == "" || strings.EqualFold(token, "openid") || removed[token] || seen[token] {
					continue
				}
				seen[token] = true
				out = append(out, token)
			}
		}
	}
	return out
}

func patchNormalizedCSVList(base []string, clearAll bool, add, remove []string, preserveCase bool) []string {
	var seed []string
	if !clearAll {
		seed = append(seed, base...)
	}
	seed = append(seed, add...)
	normalizedSeed := normalizeList(seed, preserveCase)
	if len(remove) == 0 {
		return normalizedSeed
	}
	removeSet := make(map[string]bool, len(remove))
	for _, r := range normalizeList(remove, preserveCase) {
		removeSet[r] = true
	}
	filtered := make([]string, 0, len(normalizedSeed))
	for _, item := range normalizedSeed {
		if !removeSet[item] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func hasAuthPolicyPatchField(in adminConfigureAuthPolicyInput) bool {
	return in.AllowLocalLogin != nil ||
		in.AllowOIDCLogin != nil ||
		in.RequireVerifiedOIDCEmail != nil ||
		in.DefaultRole != nil ||
		in.ClearAllowedEmailDomains != nil ||
		len(in.AddAllowedEmailDomains) > 0 ||
		len(in.RemoveAllowedEmailDomains) > 0 ||
		in.ClearAllowedOIDCGroups != nil ||
		len(in.AddAllowedOIDCGroups) > 0 ||
		len(in.RemoveAllowedOIDCGroups) > 0 ||
		in.ClearInvitedEmails != nil ||
		len(in.AddInvitedEmails) > 0 ||
		len(in.RemoveInvitedEmails) > 0 ||
		in.AllowEmailRegistration != nil ||
		in.AllowOIDCRegistration != nil ||
		in.AllowExternalIdentityPassword != nil
}

func isValidIssuerHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.User == nil
}

func structToMap(out authProviderOutput) map[string]any {
	return map[string]any{
		"id":              out.ID,
		"type":            out.Type,
		"name":            out.Name,
		"enabled":         out.Enabled,
		"issuerUrl":       out.IssuerURL,
		"clientId":        out.ClientID,
		"clientSecretRef": out.ClientSecretRef,
		"clientSecretSet": out.ClientSecretSet,
		"scopes":          out.Scopes,
		"groupClaim":      out.GroupClaim,
		"emailClaim":      out.EmailClaim,
		"usernameClaim":   out.UsernameClaim,
		"isDefault":       out.IsDefault,
		"createdAt":       out.CreatedAt,
	}
}

type anyContext = interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}

func derefIdentityStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
