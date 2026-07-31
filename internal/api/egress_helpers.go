package api

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

func (h *Handlers) egressPolicyForUser(user model.User, contexts ...context.Context) security.EgressPolicy {
	policy := security.PublicEgressPolicy()
	if user.Role == authz.PlatformRoleAdmin {
		policy.AllowPrivateNetwork = true
	}
	if h.dbWithContext(firstContext(contexts)) != nil {
		h.configs.reload(h.dbWithContext(firstContext(contexts)))
	}

	values := h.configs.get([]string{
		"security.egress.domainAllowList",
		"security.egress.domainBlockList",
		"security.egress.ipAllowList",
		"security.egress.ipBlockList",
		"security.egress.allowedPorts",
	})
	policy.DomainAllowList = splitConfigList(values["security.egress.domainAllowList"])
	policy.DomainBlockList = splitConfigList(values["security.egress.domainBlockList"])
	policy.IPAllowList = splitConfigList(values["security.egress.ipAllowList"])
	policy.IPBlockList = splitConfigList(values["security.egress.ipBlockList"])
	policy.AllowedPorts = splitPortList(values["security.egress.allowedPorts"])
	return policy
}

func (h *Handlers) egressContextForUser(ctx context.Context, user model.User, timeout time.Duration) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, security.NewHTTPClient(h.egressPolicyForUser(user, ctx), timeout))
}

func (h *Handlers) adminConfiguredEgressContext(ctx context.Context, timeout time.Duration) context.Context {
	admin := model.User{Role: authz.PlatformRoleAdmin}
	return context.WithValue(ctx, oauth2.HTTPClient, security.NewHTTPClient(h.egressPolicyForUser(admin, ctx), timeout))
}

func (h *Handlers) aiWebEgressPolicyForUser(ctx context.Context, userID string) (security.EgressPolicy, error) {
	var user model.User
	if err := h.dbWithContext(ctx).WithContext(ctx).First(&user, "id = ? and disabled = ?", userID, false).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return security.EgressPolicy{}, aitool.ErrForbidden
		}
		return security.EgressPolicy{}, err
	}
	if h.dbWithContext(ctx) != nil {
		h.configs.reload(h.dbWithContext(ctx))
	}
	values := h.configs.get([]string{
		"security.egress.domainBlockList",
		"security.egress.ipBlockList",
	})
	policy := security.AdminEgressPolicy()
	policy.DomainBlockList = splitConfigList(values["security.egress.domainBlockList"])
	policy.IPBlockList = splitConfigList(values["security.egress.ipBlockList"])
	return policy, nil
}

func (h *Handlers) aiWebProxyPoolForUser(ctx context.Context, userID string) ([]string, error) {
	var user model.User
	if err := h.dbWithContext(ctx).WithContext(ctx).Select("id").First(&user, "id = ? and disabled = ?", userID, false).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.ErrForbidden
		}
		return nil, err
	}
	if h.dbWithContext(ctx) != nil {
		h.configs.reload(h.dbWithContext(ctx))
	}
	if !configBool(h.configs.get([]string{"ai.web.proxy_enabled"})["ai.web.proxy_enabled"]) {
		return nil, nil
	}
	var secretConfig model.AppConfig
	if err := h.dbWithContext(ctx).WithContext(ctx).First(&secretConfig, "key = ?", "ai.web.proxy_pool").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.ErrWebRequestFailed
		}
		return nil, err
	}
	pool := splitProxyPool(h.secrets.ResolveContext(ctx, secretConfig.Value))
	if len(pool) == 0 {
		return nil, aitool.ErrWebRequestFailed
	}
	return pool, nil
}

func splitProxyPool(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			items = append(items, line)
		}
	}
	return items
}

func splitConfigList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			items = append(items, field)
		}
	}
	return items
}

func splitPortList(value string) []int {
	items := splitConfigList(value)
	ports := make([]int, 0, len(items))
	for _, item := range items {
		port, err := strconv.Atoi(item)
		if err == nil && port >= 1 && port <= 65535 {
			ports = append(ports, port)
		}
	}
	return ports
}
