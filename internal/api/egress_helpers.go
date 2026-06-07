package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
	"golang.org/x/oauth2"
)

func (h *Handlers) egressPolicyForUser(user model.User) security.EgressPolicy {
	policy := security.PublicEgressPolicy()
	if user.Role == "platform_admin" {
		policy.AllowPrivateNetwork = true
	}
	if h.db != nil {
		h.configs.reload(h.db)
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
	return context.WithValue(ctx, oauth2.HTTPClient, security.NewHTTPClient(h.egressPolicyForUser(user), timeout))
}

func (h *Handlers) adminConfiguredEgressContext(ctx context.Context, timeout time.Duration) context.Context {
	admin := model.User{Role: "platform_admin"}
	return context.WithValue(ctx, oauth2.HTTPClient, security.NewHTTPClient(h.egressPolicyForUser(admin), timeout))
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
