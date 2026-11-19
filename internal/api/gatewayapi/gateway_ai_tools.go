package gatewayapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterAITools registers the 5 gateway domain facade AI tools owned by gatewayapi:
//   - network_list_gateway_routes (L0, read)
//   - network_check_gateway_domain (L0, read)
//   - network_create_gateway_route (L2, async)
//   - network_update_gateway_route (L2, async)
//   - network_delete_gateway_route (L2, async)
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	h.registerGatewayAITools(reg)
}

func newGatewayAIGin(tc *aitool.ToolContext) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	subGin, _ := gin.CreateTestContext(rec)
	reqCtx := tc.Context()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, "/internal/v1/ai/tools", http.NoBody)
	if tc != nil && tc.Gin != nil && tc.Gin.Request != nil {
		req.Header = tc.Gin.Request.Header.Clone()
		req.RemoteAddr = tc.Gin.Request.RemoteAddr
	}
	subGin.Request = req
	if tc != nil && tc.Gin != nil && tc.Gin.Keys != nil {
		subGin.Keys = maps.Clone(tc.Gin.Keys)
	}
	return subGin, rec
}

func gatewayAIErrorFromRecorder(rec *httptest.ResponseRecorder, fallbackCode, fallbackMessage string) *aitool.ToolError {
	status := http.StatusBadRequest
	if rec != nil && rec.Code >= 400 {
		status = rec.Code
	}
	if rec == nil {
		return aitool.NewError(status, fallbackCode, fallbackMessage)
	}
	raw := bytes.TrimSpace(rec.Body.Bytes())
	if len(raw) == 0 {
		return aitool.NewError(status, fallbackCode, fallbackMessage)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return aitool.NewError(status, fallbackCode, string(raw))
	}
	code, _ := parsed["code"].(string)
	if strings.TrimSpace(code) == "" {
		code = fallbackCode
	}
	msg, _ := parsed["message"].(string)
	if strings.TrimSpace(msg) == "" {
		msg = fallbackMessage
	}
	te := &aitool.ToolError{
		Status:  status,
		Code:    code,
		Message: msg,
		RawBody: append(json.RawMessage(nil), raw...),
	}
	if code == "gateway_route.backend_service_missing" {
		te.Hint = "Deploy the target first via delivery_deploy_target so its Kubernetes Service exists before creating a gateway route."
	}
	return te
}

func (h *Handler) authorizeGatewayAIProject(tc *aitool.ToolContext, projectRef string, action authz.Action) (model.User, model.Project, error) {
	projectRef = strings.TrimSpace(projectRef)
	if projectRef == "" {
		return model.User{}, model.Project{}, aitool.BadRequest("project.required", "project is required")
	}
	c, rec := newGatewayAIGin(tc)
	user, project, ok := h.host.AuthorizeProjectRef(c, projectRef, action)
	if !ok {
		return model.User{}, model.Project{}, gatewayAIErrorFromRecorder(rec, "project.access_denied", "project authorization failed")
	}
	return user, project, nil
}

func (h *Handler) ensureGatewayAIProjectMutable(tc *aitool.ToolContext, project model.Project) error {
	c, rec := newGatewayAIGin(tc)
	if !h.ensureProjectCanMutate(c, project) {
		return gatewayAIErrorFromRecorder(rec, "project.immutable", "project cannot be mutated")
	}
	return nil
}

// --- Typed Filter & Route DTOs (catalog §3B.1 #26 & §3B.2 #30/#31: typed filters, no hostnameAliases) ---

type aiGatewayURLRewrite struct {
	Hostname           *string `json:"hostname,omitempty" desc:"重写目标 Hostname"`
	ReplacePrefixMatch *string `json:"replacePrefixMatch,omitempty" desc:"路径前缀替换值（与 replaceFullPath 互斥）"`
	ReplaceFullPath    *string `json:"replaceFullPath,omitempty" desc:"完整路径替换值（与 replacePrefixMatch 互斥）"`
}

type aiGatewayRequestRedirect struct {
	Scheme             *string `json:"scheme,omitempty" enum:"http|https" desc:"重定向协议（http 或 https）"`
	Hostname           *string `json:"hostname,omitempty" desc:"重定向目标 Hostname"`
	StatusCode         *int    `json:"statusCode,omitempty" min:"301" max:"302" desc:"HTTP 重定向状态码（301 或 302）"`
	ReplacePrefixMatch *string `json:"replacePrefixMatch,omitempty" desc:"路径前缀替换值（与 replaceFullPath 互斥）"`
	ReplaceFullPath    *string `json:"replaceFullPath,omitempty" desc:"完整路径替换值（与 replacePrefixMatch 互斥）"`
}

type aiGatewayRouteDTO struct {
	ID                     string                    `json:"id"`
	RouteID                string                    `json:"routeId"`
	ProjectID              string                    `json:"projectId"`
	ApplicationID          string                    `json:"applicationId"`
	DeploymentTargetID     string                    `json:"deploymentTargetId"`
	Host                   string                    `json:"host"`
	DomainSuffix           string                    `json:"domainSuffix"`
	Path                   string                    `json:"path"`
	PathMatchType          string                    `json:"pathMatchType"`
	ServicePort            int                       `json:"servicePort"`
	TLSMode                string                    `json:"tlsMode"`
	SectionName            string                    `json:"sectionName"`
	ParentGatewayName      string                    `json:"parentGatewayName"`
	ParentGatewayNamespace string                    `json:"parentGatewayNamespace"`
	BackendWeight          int                       `json:"backendWeight"`
	RequestHeaders         map[string]string         `json:"requestHeaders"`
	ResponseHeaders        map[string]string         `json:"responseHeaders"`
	URLRewrite             *aiGatewayURLRewrite      `json:"urlRewrite,omitempty"`
	RequestRedirect        *aiGatewayRequestRedirect `json:"requestRedirect,omitempty"`
	CertificateStatus      string                    `json:"certificateStatus"`
	CertificateMessage     string                    `json:"certificateMessage"`
	CertificateNotAfter    *time.Time                `json:"certificateNotAfter"`
	CertificateIssuerKind  string                    `json:"certificateIssuerKind"`
	CertificateIssuerName  string                    `json:"certificateIssuerName"`
	CNAMEName              string                    `json:"cnameName"`
	CNAMETarget            string                    `json:"cnameTarget"`
	DNSStatus              string                    `json:"dnsStatus"`
	Status                 string                    `json:"status"`
	ObservationCode        string                    `json:"observationCode,omitempty"`
	ObservedAt             *time.Time                `json:"observedAt,omitempty"`
	Enabled                bool                      `json:"enabled"`
	DeleteStatus           string                    `json:"deleteStatus"`
	DeleteMessage          string                    `json:"deleteMessage"`
	DeleteStartedAt        *time.Time                `json:"deleteStartedAt"`
	DeleteFinishedAt       *time.Time                `json:"deleteFinishedAt"`
	IsDefault              bool                      `json:"isDefault"`
	AccessURL              string                    `json:"accessUrl"`
	RouteSummary           string                    `json:"routeSummary"`
	Conditions             []model.RouteCondition    `json:"conditions"`
	CreatedBy              string                    `json:"createdBy"`
	CreatedAt              time.Time                 `json:"createdAt"`
	UpdatedAt              time.Time                 `json:"updatedAt"`
}

func decodeGatewayURLRewrite(raw string) *aiGatewayURLRewrite {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || len(m) == 0 {
		return nil
	}
	out := &aiGatewayURLRewrite{}
	hasAny := false
	for _, k := range []string{"hostname", "host"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			s := strings.TrimSpace(v)
			out.Hostname = &s
			hasAny = true
			break
		}
	}
	if v, ok := m["replacePrefixMatch"].(string); ok && strings.TrimSpace(v) != "" {
		s := strings.TrimSpace(v)
		out.ReplacePrefixMatch = &s
		hasAny = true
	}
	if v, ok := m["replaceFullPath"].(string); ok && strings.TrimSpace(v) != "" {
		s := strings.TrimSpace(v)
		out.ReplaceFullPath = &s
		hasAny = true
	}
	if !hasAny {
		return nil
	}
	return out
}

func decodeGatewayRequestRedirect(raw string) *aiGatewayRequestRedirect {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || len(m) == 0 {
		return nil
	}
	out := &aiGatewayRequestRedirect{}
	hasAny := false
	if v, ok := m["scheme"].(string); ok && strings.TrimSpace(v) != "" {
		s := strings.TrimSpace(v)
		out.Scheme = &s
		hasAny = true
	}
	for _, k := range []string{"hostname", "host"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			s := strings.TrimSpace(v)
			out.Hostname = &s
			hasAny = true
			break
		}
	}
	if v, ok := m["statusCode"].(float64); ok && int(v) > 0 {
		code := int(v)
		out.StatusCode = &code
		hasAny = true
	}
	if v, ok := m["replacePrefixMatch"].(string); ok && strings.TrimSpace(v) != "" {
		s := strings.TrimSpace(v)
		out.ReplacePrefixMatch = &s
		hasAny = true
	}
	if v, ok := m["replaceFullPath"].(string); ok && strings.TrimSpace(v) != "" {
		s := strings.TrimSpace(v)
		out.ReplaceFullPath = &s
		hasAny = true
	}
	if !hasAny {
		return nil
	}
	return out
}

func toAIGatewayRouteDTO(route model.GatewayRoute) aiGatewayRouteDTO {
	reqHeaders, err := parseGatewayKeyValueMap(route.RequestHeaders)
	if err != nil || reqHeaders == nil {
		reqHeaders = map[string]string{}
	}
	respHeaders, err := parseGatewayKeyValueMap(route.ResponseHeaders)
	if err != nil || respHeaders == nil {
		respHeaders = map[string]string{}
	}
	conditions := route.Conditions
	if conditions == nil {
		conditions = []model.RouteCondition{}
	}
	pathMatchType := normalizeHTTPRoutePathMatchType(route.PathMatchType)
	backendWeight := normalizeBackendWeight(route.BackendWeight)
	return aiGatewayRouteDTO{
		ID:                     route.ID,
		RouteID:                route.ID,
		ProjectID:              route.ProjectID,
		ApplicationID:          route.ApplicationID,
		DeploymentTargetID:     route.DeploymentTargetID,
		Host:                   route.Host,
		DomainSuffix:           route.DomainSuffix,
		Path:                   route.Path,
		PathMatchType:          pathMatchType,
		ServicePort:            route.ServicePort,
		TLSMode:                route.TLSMode,
		SectionName:            route.SectionName,
		ParentGatewayName:      route.ParentGatewayName,
		ParentGatewayNamespace: route.ParentGatewayNamespace,
		BackendWeight:          backendWeight,
		RequestHeaders:         reqHeaders,
		ResponseHeaders:        respHeaders,
		URLRewrite:             decodeGatewayURLRewrite(route.URLRewrite),
		RequestRedirect:        decodeGatewayRequestRedirect(route.RequestRedirect),
		CertificateStatus:      route.CertificateStatus,
		CertificateMessage:     route.CertificateMessage,
		CertificateNotAfter:    route.CertificateNotAfter,
		CertificateIssuerKind:  route.CertificateIssuerKind,
		CertificateIssuerName:  route.CertificateIssuerName,
		CNAMEName:              route.CNAMEName,
		CNAMETarget:            route.CNAMETarget,
		DNSStatus:              route.DNSStatus,
		Status:                 route.Status,
		ObservationCode:        route.ObservationCode,
		ObservedAt:             route.ObservedAt,
		Enabled:                route.Enabled,
		DeleteStatus:           route.DeleteStatus,
		DeleteMessage:          route.DeleteMessage,
		DeleteStartedAt:        route.DeleteStartedAt,
		DeleteFinishedAt:       route.DeleteFinishedAt,
		IsDefault:              route.IsDefault,
		AccessURL:              route.AccessURL,
		RouteSummary:           route.RouteSummary,
		Conditions:             conditions,
		CreatedBy:              route.CreatedBy,
		CreatedAt:              route.CreatedAt,
		UpdatedAt:              route.UpdatedAt,
	}
}

func resolveAIGatewayRoute(db *gorm.DB, projectID, applicationID, routeRef string, allowAbsent bool) (model.GatewayRoute, bool, error) {
	ref := strings.TrimSpace(routeRef)
	if ref == "" {
		return model.GatewayRoute{}, false, aitool.BadRequest("gateway_route.required", "route is required")
	}

	baseQuery := func() *gorm.DB {
		q := db.Where("project_id = ?", projectID)
		if applicationID != "" {
			q = q.Where("application_id = ?", applicationID)
		}
		return q
	}

	var byID model.GatewayRoute
	if err := baseQuery().First(&byID, "id = ?", ref).Error; err == nil {
		return byID, true, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.GatewayRoute{}, false, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	normHost := strings.Trim(strings.ToLower(ref), ".")
	var byHost []model.GatewayRoute
	if err := baseQuery().Where("LOWER(host) = ?", normHost).Order("created_at asc, id asc").Find(&byHost).Error; err != nil {
		return model.GatewayRoute{}, false, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if len(byHost) == 1 {
		return byHost[0], true, nil
	}
	if len(byHost) > 1 {
		candidates := make([]aitool.AmbiguousCandidate, len(byHost))
		routeDetails := make([]map[string]any, len(byHost))
		for i, r := range byHost {
			candidates[i] = aitool.AmbiguousCandidate{
				ID:         r.ID,
				Name:       r.Host + r.Path,
				Identifier: r.Host,
			}
			routeDetails[i] = map[string]any{
				"id":   r.ID,
				"host": r.Host,
				"path": r.Path,
			}
		}
		ambErr := aitool.AmbiguousResource("gateway_route", ref, candidates)
		ambErr.Details = map[string]any{"routes": routeDetails}
		return model.GatewayRoute{}, false, ambErr
	}

	if allowAbsent {
		return model.GatewayRoute{}, false, nil
	}
	return model.GatewayRoute{}, false, aitool.NotFound("gateway_route.not_found", "gateway route was not found")
}

func validateStrictTLSMode(raw *string, fallbackDefault string) (string, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return fallbackDefault, nil
	}
	val := strings.TrimSpace(*raw)
	switch val {
	case "http-only", "http-challenge", "manual-cert":
		return val, nil
	default:
		return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid tlsMode %q; allowed values: http-only, http-challenge, manual-cert", val))
	}
}

func validateStrictPathMatchType(raw *string, fallbackDefault string) (string, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return fallbackDefault, nil
	}
	val := strings.TrimSpace(*raw)
	if val != "PathPrefix" && val != "Exact" {
		return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid pathMatchType %q; allowed values: PathPrefix, Exact", val))
	}
	return val, nil
}

func mergeGatewayHeaderPatch(existingRaw string, upserts map[string]string, removeKeys []string) (string, error) {
	existingMap, err := parseGatewayKeyValueMap(existingRaw)
	if err != nil {
		existingMap = map[string]string{}
	}
	if len(removeKeys) > 0 {
		removeLower := make(map[string]bool, len(removeKeys))
		for _, k := range removeKeys {
			norm := strings.ToLower(strings.TrimSpace(k))
			if norm != "" {
				removeLower[norm] = true
			}
		}
		for existingKey := range existingMap {
			if removeLower[strings.ToLower(strings.TrimSpace(existingKey))] {
				delete(existingMap, existingKey)
			}
		}
	}
	for k, v := range upserts {
		trimmedKey := strings.TrimSpace(k)
		if trimmedKey == "" {
			return "", aitool.BadRequest("argument_invalid", "header key cannot be empty")
		}
		// Replace any existing case-variant of the same header key
		for existingKey := range existingMap {
			if !strings.EqualFold(existingKey, trimmedKey) || existingKey == trimmedKey {
				continue
			}
			delete(existingMap, existingKey)
		}
		existingMap[trimmedKey] = strings.TrimSpace(v)
	}
	return encodeGatewayHeaderMap(existingMap), nil
}

func mergeAndEncodeURLRewrite(existingRaw string, patch *aiGatewayURLRewrite, clear bool) (string, error) {
	if clear && patch != nil {
		return "", aitool.BadRequest("argument_invalid", "cannot specify both urlRewrite and clearUrlRewrite=true")
	}
	if clear {
		return "", nil
	}
	if patch == nil {
		return strings.TrimSpace(existingRaw), nil
	}

	hasHostname := patch.Hostname != nil && strings.TrimSpace(*patch.Hostname) != ""
	hasPrefix := patch.ReplacePrefixMatch != nil && strings.TrimSpace(*patch.ReplacePrefixMatch) != ""
	hasFull := patch.ReplaceFullPath != nil && strings.TrimSpace(*patch.ReplaceFullPath) != ""
	if !hasHostname && !hasPrefix && !hasFull {
		return "", aitool.BadRequest("argument_invalid", "urlRewrite must specify at least one non-empty field (hostname, replacePrefixMatch, or replaceFullPath)")
	}
	if hasPrefix && hasFull {
		return "", aitool.BadRequest("argument_invalid", "urlRewrite replacePrefixMatch and replaceFullPath are mutually exclusive")
	}

	merged := decodeGatewayURLRewrite(existingRaw)
	if merged == nil {
		merged = &aiGatewayURLRewrite{}
	}
	if hasHostname {
		s := strings.TrimSpace(*patch.Hostname)
		merged.Hostname = &s
	}
	if hasPrefix {
		s := strings.TrimSpace(*patch.ReplacePrefixMatch)
		merged.ReplacePrefixMatch = &s
		merged.ReplaceFullPath = nil
	}
	if hasFull {
		s := strings.TrimSpace(*patch.ReplaceFullPath)
		merged.ReplaceFullPath = &s
		merged.ReplacePrefixMatch = nil
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return "", aitool.BadRequest("argument_invalid", err.Error())
	}
	return string(encoded), nil
}

func mergeAndEncodeRequestRedirect(existingRaw string, patch *aiGatewayRequestRedirect, clear bool) (string, error) {
	if clear && patch != nil {
		return "", aitool.BadRequest("argument_invalid", "cannot specify both requestRedirect and clearRequestRedirect=true")
	}
	if clear {
		return "", nil
	}
	if patch == nil {
		return strings.TrimSpace(existingRaw), nil
	}

	hasScheme := patch.Scheme != nil && strings.TrimSpace(*patch.Scheme) != ""
	hasHostname := patch.Hostname != nil && strings.TrimSpace(*patch.Hostname) != ""
	hasStatus := patch.StatusCode != nil
	hasPrefix := patch.ReplacePrefixMatch != nil && strings.TrimSpace(*patch.ReplacePrefixMatch) != ""
	hasFull := patch.ReplaceFullPath != nil && strings.TrimSpace(*patch.ReplaceFullPath) != ""
	if !hasScheme && !hasHostname && !hasStatus && !hasPrefix && !hasFull {
		return "", aitool.BadRequest("argument_invalid", "requestRedirect must specify at least one field (scheme, hostname, statusCode, replacePrefixMatch, or replaceFullPath)")
	}
	if hasPrefix && hasFull {
		return "", aitool.BadRequest("argument_invalid", "requestRedirect replacePrefixMatch and replaceFullPath are mutually exclusive")
	}
	if hasScheme {
		sc := strings.ToLower(strings.TrimSpace(*patch.Scheme))
		if sc != "http" && sc != "https" {
			return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid requestRedirect.scheme %q; allowed values: http, https", *patch.Scheme))
		}
	}
	if hasStatus && *patch.StatusCode != 301 && *patch.StatusCode != 302 {
		return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid requestRedirect.statusCode %d; allowed values: 301, 302", *patch.StatusCode))
	}

	merged := decodeGatewayRequestRedirect(existingRaw)
	if merged == nil {
		merged = &aiGatewayRequestRedirect{}
	}
	if hasScheme {
		sc := strings.ToLower(strings.TrimSpace(*patch.Scheme))
		merged.Scheme = &sc
	}
	if hasHostname {
		hn := strings.TrimSpace(*patch.Hostname)
		merged.Hostname = &hn
	}
	if hasStatus {
		code := *patch.StatusCode
		merged.StatusCode = &code
	}
	if hasPrefix {
		p := strings.TrimSpace(*patch.ReplacePrefixMatch)
		merged.ReplacePrefixMatch = &p
		merged.ReplaceFullPath = nil
	}
	if hasFull {
		f := strings.TrimSpace(*patch.ReplaceFullPath)
		merged.ReplaceFullPath = &f
		merged.ReplacePrefixMatch = nil
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return "", aitool.BadRequest("argument_invalid", err.Error())
	}
	return string(encoded), nil
}

// --- Input Structs for the 5 Gateway AI Tools ---

type networkListGatewayRoutesInput struct {
	Project     string  `json:"project" desc:"项目 ID 或 identifier"`
	Application *string `json:"application,omitempty" desc:"可选应用 ID、identifier 或名称"`
	Route       *string `json:"route,omitempty" desc:"可选路由 ID（gwr_xxx）或唯一 host 精确查看单条路由完整配置与实时状态"`
	Search      *string `json:"search,omitempty" desc:"按 host 或 path 模糊搜索"`
	SortBy      *string `json:"sortBy,omitempty" enum:"createdAt|host|enabled" default:"createdAt" desc:"排序字段"`
	SortOrder   *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page        *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize    *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type networkCheckGatewayDomainInput struct {
	Project      string  `json:"project" desc:"项目 ID 或 identifier"`
	Host         string  `json:"host" desc:"待检查完整域名或子域名前缀"`
	Application  *string `json:"application,omitempty" desc:"配合 target 定位运行集群"`
	Target       *string `json:"target,omitempty" desc:"支持 targetId、stage 或唯一 name（application+target 与 route 至少传一组）"`
	Route        *string `json:"route,omitempty" desc:"更新既有路由域名时传入 routeId 或唯一 host，将自身当前域名识别为 current"`
	DomainSuffix *string `json:"domainSuffix,omitempty" desc:"指定集群允许的网关域名后缀"`
}

type networkCreateGatewayRouteInput struct {
	Project                string  `json:"project" desc:"项目 ID 或 identifier"`
	Application            string  `json:"application" desc:"应用 ID、identifier 或名称"`
	Target                 string  `json:"target" desc:"支持 targetId、stage 或唯一 name（须已发布生成 K8s Service 且非 internalOnly）"`
	Host                   *string `json:"host,omitempty" desc:"自定义域名或子域名前缀（省略自动生成默认域名）"`
	DomainSuffix           *string `json:"domainSuffix,omitempty" desc:"集群允许的网关域名后缀"`
	Path                   *string `json:"path,omitempty" default:"/" desc:"路由匹配路径"`
	PathMatchType          *string `json:"pathMatchType,omitempty" enum:"PathPrefix|Exact" default:"PathPrefix" desc:"路径匹配类型（Exact 需项目 Owner/Admin）"`
	ServicePort            *int    `json:"servicePort,omitempty" min:"1" max:"65535" desc:"目标服务端口（省略默认取首个服务端口）"`
	TLSMode                *string `json:"tlsMode,omitempty" enum:"http-only|http-challenge|manual-cert" default:"http-only" desc:"TLS 证书模式（http-only: 不启用 TLS/纯 HTTP 即 none；http-challenge: 自动签发 HTTP-01 证书；manual-cert: 手动上传证书）"`
	SectionName            *string `json:"sectionName,omitempty" desc:"父 Gateway 监听器名称（如 web 或 websecure）"`
	ParentGatewayName      *string `json:"parentGatewayName,omitempty" desc:"自定义入口 Gateway 名称（需项目 Owner/Admin）"`
	ParentGatewayNamespace *string `json:"parentGatewayNamespace,omitempty" desc:"自定义入口 Gateway 所在命名空间（需项目 Owner/Admin）"`
	Enabled                *bool   `json:"enabled,omitempty" default:"true" desc:"是否启用路由"`
	IsDefault              *bool   `json:"isDefault,omitempty" default:"false" desc:"是否设为应用默认主访问入口"`
}

type createGatewayRouteLockedPayload struct {
	ProjectID          string `json:"projectId"`
	ApplicationID      string `json:"applicationId"`
	DeploymentTargetID string `json:"deploymentTargetId"`
	Host               string `json:"host"`
	DomainSuffix       string `json:"domainSuffix"`
	ServicePort        int    `json:"servicePort"`
}

type networkUpdateGatewayRouteInput struct {
	Project                     string                    `json:"project" desc:"项目 ID 或 identifier"`
	Route                       string                    `json:"route" desc:"路由 ID（gwr_xxx）或项目内唯一 host"`
	Application                 *string                   `json:"application,omitempty" desc:"切换后端所属应用（切换 application 时必须同步传入新应用下的 target）"`
	Target                      *string                   `json:"target,omitempty" desc:"切换指向目标部署配置（targetId、stage 或唯一 name）"`
	Host                        *string                   `json:"host,omitempty" desc:"修改路由域名（单路由单域名；如需额外备用域名请用 network_create_gateway_route 新建独立路由）"`
	DomainSuffix                *string                   `json:"domainSuffix,omitempty" desc:"修改网关域名后缀"`
	Path                        *string                   `json:"path,omitempty" desc:"修改路由路径"`
	PathMatchType               *string                   `json:"pathMatchType,omitempty" enum:"PathPrefix|Exact" desc:"修改路径匹配类型（Exact 需项目 Owner/Admin）"`
	ServicePort                 *int                      `json:"servicePort,omitempty" min:"1" max:"65535" desc:"修改后端服务端口"`
	TLSMode                     *string                   `json:"tlsMode,omitempty" enum:"http-only|http-challenge|manual-cert" desc:"修改 TLS 证书模式（http-only: 不启用 TLS/纯 HTTP 即 none；http-challenge: 自动签发 HTTP-01 证书；manual-cert: 手动证书）"`
	SectionName                 *string                   `json:"sectionName,omitempty" desc:"指定父 Gateway 监听器名称"`
	ClearSectionName            *bool                     `json:"clearSectionName,omitempty" desc:"清空自定义监听器名称"`
	ParentGatewayName           *string                   `json:"parentGatewayName,omitempty" desc:"自定义入口 Gateway 名称（需项目 Owner/Admin）"`
	ClearParentGatewayName      *bool                     `json:"clearParentGatewayName,omitempty" desc:"恢复集群默认入口 Gateway 名称"`
	ParentGatewayNamespace      *string                   `json:"parentGatewayNamespace,omitempty" desc:"自定义入口 Gateway 命名空间（需项目 Owner/Admin）"`
	ClearParentGatewayNamespace *bool                     `json:"clearParentGatewayNamespace,omitempty" desc:"恢复集群默认入口 Gateway 命名空间"`
	Enabled                     *bool                     `json:"enabled,omitempty" desc:"启用或停用网关路由"`
	IsDefault                   *bool                     `json:"isDefault,omitempty" desc:"是否设为默认主访问入口"`
	BackendWeight               *int                      `json:"backendWeight,omitempty" min:"1" desc:"后端流量权重（!= 1 需项目 Owner/Admin）"`
	UpsertRequestHeaders        map[string]string         `json:"upsertRequestHeaders,omitempty" desc:"增量设置注入请求头（需项目 Owner/Admin；特权头仅平台管理员可设）"`
	RemoveRequestHeaderKeys     []string                  `json:"removeRequestHeaderKeys,omitempty" desc:"按 Header 名移除注入请求头"`
	UpsertResponseHeaders       map[string]string         `json:"upsertResponseHeaders,omitempty" desc:"增量设置注入响应头（需项目 Owner/Admin；特权头仅平台管理员可设）"`
	RemoveResponseHeaderKeys    []string                  `json:"removeResponseHeaderKeys,omitempty" desc:"按 Header 名移除注入响应头"`
	URLRewrite                  *aiGatewayURLRewrite      `json:"urlRewrite,omitempty" desc:"强类型 URL 重写规则（与 requestRedirect 互斥，需项目 Owner/Admin）"`
	ClearURLRewrite             *bool                     `json:"clearUrlRewrite,omitempty" desc:"清除 URL 重写规则"`
	RequestRedirect             *aiGatewayRequestRedirect `json:"requestRedirect,omitempty" desc:"强类型请求重定向规则（与 urlRewrite 互斥，需项目 Owner/Admin）"`
	ClearRequestRedirect        *bool                     `json:"clearRequestRedirect,omitempty" desc:"清除请求重定向规则"`
}

type gatewayRouteIDLockedPayload struct {
	ProjectID string `json:"projectId"`
	RouteID   string `json:"routeId"`
}

type networkDeleteGatewayRouteInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	Route   string `json:"route" desc:"待删除路由 ID（gwr_xxx）或项目内唯一 host"`
}

func applyGatewaySearchFilter(query *gorm.DB, keyword string) *gorm.DB {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return query
	}
	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(keyword)
	pattern := "%" + escaped + "%"
	return query.Where("LOWER(host) LIKE LOWER(?) ESCAPE '\\' OR LOWER(path) LIKE LOWER(?) ESCAPE '\\'", pattern, pattern)
}

func normalizeGatewaySort(sortByRaw, sortOrderRaw *string) (string, string, string, error) {
	allowed := map[string]string{
		"createdAt": "created_at",
		"host":      "host",
		"enabled":   "enabled",
	}
	sortBy := "createdAt"
	if sortByRaw != nil && strings.TrimSpace(*sortByRaw) != "" {
		cand := strings.TrimSpace(*sortByRaw)
		if _, ok := allowed[cand]; !ok {
			keys := []string{"createdAt", "enabled", "host"}
			return "", "", "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid sortBy %q; allowed values: %s", cand, strings.Join(keys, ", ")))
		}
		sortBy = cand
	}
	sortOrder := "desc"
	if sortOrderRaw != nil && strings.TrimSpace(*sortOrderRaw) != "" {
		cand := strings.ToLower(strings.TrimSpace(*sortOrderRaw))
		if cand != "asc" && cand != "desc" {
			return "", "", "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid sortOrder %q; allowed values: asc, desc", cand))
		}
		sortOrder = cand
	}
	return sortBy, sortOrder, allowed[sortBy] + " " + sortOrder, nil
}

func (h *Handler) prepareAIGatewayRouteCreate(
	tc *aitool.ToolContext,
	in networkCreateGatewayRouteInput,
	locked *createGatewayRouteLockedPayload,
) (model.User, model.Project, model.Application, model.DeploymentTarget, model.GatewayRoute, createGatewayRouteLockedPayload, error) {
	user, project, err := h.authorizeGatewayAIProject(tc, in.Project, authz.ActionGatewayManage)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{}, err
	}
	if err := h.ensureGatewayAIProjectMutable(tc, project); err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{}, err
	}

	app, err := aitool.ResolveApplication(tc.DB, project.ID, in.Application)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{}, err
	}
	target, err := aitool.ResolveDeploymentTarget(tc.DB, project.ID, app.ID, in.Target)
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{}, err
	}
	if locked != nil && (locked.ProjectID != project.ID || locked.ApplicationID != app.ID || locked.DeploymentTargetID != target.ID) {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{},
			aitool.Conflict("ai.preview_lock_mismatch", "preview lock does not match target application or deployment target")
	}
	if !target.Enabled {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{},
			aitool.Conflict("deployment_target.disabled", "deployment target is disabled and cannot expose gateway routes")
	}
	if target.InternalOnly {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{},
			aitool.Conflict("gateway_route.target_internal", "the deployment target is internalOnly and cannot have gateway routes")
	}

	tlsMode, err := validateStrictTLSMode(in.TLSMode, "http-only")
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{}, err
	}
	pathMatchType, err := validateStrictPathMatchType(in.PathMatchType, "PathPrefix")
	if err != nil {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{}, err
	}

	hostVal := ""
	if locked != nil && strings.TrimSpace(locked.Host) != "" {
		hostVal = locked.Host
	} else if in.Host != nil {
		hostVal = strings.TrimSpace(*in.Host)
	}
	domainSuffix := ""
	if locked != nil && strings.TrimSpace(locked.DomainSuffix) != "" {
		domainSuffix = locked.DomainSuffix
	} else if in.DomainSuffix != nil {
		domainSuffix = strings.TrimSpace(*in.DomainSuffix)
	}
	pathVal := "/"
	if in.Path != nil && strings.TrimSpace(*in.Path) != "" {
		pathVal = strings.TrimSpace(*in.Path)
	}
	servicePort := 0
	if locked != nil && locked.ServicePort > 0 {
		servicePort = locked.ServicePort
	} else if in.ServicePort != nil {
		servicePort = *in.ServicePort
	}
	sectionName := ""
	if in.SectionName != nil {
		sectionName = strings.TrimSpace(*in.SectionName)
	}
	parentGwName := ""
	if in.ParentGatewayName != nil {
		parentGwName = strings.TrimSpace(*in.ParentGatewayName)
	}
	parentGwNs := ""
	if in.ParentGatewayNamespace != nil {
		parentGwNs = strings.TrimSpace(*in.ParentGatewayNamespace)
	}
	isDefault := in.IsDefault != nil && *in.IsDefault

	gwInput := gatewayRouteInput{
		ApplicationID:          app.ID,
		DeploymentTargetID:     target.ID,
		Host:                   hostVal,
		DomainSuffix:           domainSuffix,
		Path:                   pathVal,
		PathMatchType:          pathMatchType,
		ServicePort:            servicePort,
		TLSMode:                tlsMode,
		Enabled:                in.Enabled,
		IsDefault:              isDefault,
		SectionName:            sectionName,
		ParentGatewayName:      parentGwName,
		ParentGatewayNamespace: parentGwNs,
	}

	c, rec := newGatewayAIGin(tc)
	route, ok := h.gatewayRouteFromInput(c, project, user, user.ID, gwInput, "")
	if !ok {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{},
			gatewayAIErrorFromRecorder(rec, "gateway_route.invalid", "gateway route validation failed")
	}

	cBackend, recBackend := newGatewayAIGin(tc)
	if !h.ensureGatewayRouteBackendAvailable(cBackend, route) {
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{},
			gatewayAIErrorFromRecorder(recBackend, "gateway_route.backend_unavailable", "gateway route backend service check failed")
	}

	route, urlErr := h.gatewayRouteWithAccessURL(route, tc.Context())
	if urlErr != nil {
		cURL, recURL := newGatewayAIGin(tc)
		h.writeGatewayRuntimeClusterError(cURL, urlErr)
		return model.User{}, model.Project{}, model.Application{}, model.DeploymentTarget{}, model.GatewayRoute{}, createGatewayRouteLockedPayload{},
			gatewayAIErrorFromRecorder(recURL, "gateway_route.runtime_cluster_unavailable", urlErr.Error())
	}

	nextLock := createGatewayRouteLockedPayload{
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Host:               route.Host,
		DomainSuffix:       route.DomainSuffix,
		ServicePort:        route.ServicePort,
	}
	return user, project, app, target, route, nextLock, nil
}

func (h *Handler) prepareAIGatewayRouteUpdate(
	tc *aitool.ToolContext,
	in networkUpdateGatewayRouteInput,
	locked *gatewayRouteIDLockedPayload,
) (model.User, model.Project, model.GatewayRoute, model.GatewayRoute, error) {
	user, project, err := h.authorizeGatewayAIProject(tc, in.Project, authz.ActionGatewayManage)
	if err != nil {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
	}
	if err := h.ensureGatewayAIProjectMutable(tc, project); err != nil {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
	}

	var existing model.GatewayRoute
	if locked != nil && strings.TrimSpace(locked.RouteID) != "" {
		if locked.ProjectID != project.ID {
			return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
				aitool.Conflict("ai.preview_lock_mismatch", "preview lock project mismatch")
		}
		if err := tc.DB.First(&existing, "id = ? AND project_id = ?", locked.RouteID, project.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
					aitool.NotFound("gateway_route.not_found", "gateway route was not found")
			}
			return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
				aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
	} else {
		var found bool
		existing, found, err = resolveAIGatewayRoute(tc.DB, project.ID, "", in.Route, false)
		if err != nil || !found {
			return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
		}
	}

	cMut, recMut := newGatewayAIGin(tc)
	if !h.ensureGatewayRouteCanMutate(cMut, existing) {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			gatewayAIErrorFromRecorder(recMut, "gateway_route.immutable", "gateway route cannot be mutated")
	}

	// Clear vs Set conflict checks
	if in.ClearSectionName != nil && *in.ClearSectionName && in.SectionName != nil && strings.TrimSpace(*in.SectionName) != "" {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			aitool.BadRequest("argument_invalid", "cannot specify both sectionName and clearSectionName=true")
	}
	if in.ClearParentGatewayName != nil && *in.ClearParentGatewayName && in.ParentGatewayName != nil && strings.TrimSpace(*in.ParentGatewayName) != "" {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			aitool.BadRequest("argument_invalid", "cannot specify both parentGatewayName and clearParentGatewayName=true")
	}
	if in.ClearParentGatewayNamespace != nil && *in.ClearParentGatewayNamespace && in.ParentGatewayNamespace != nil && strings.TrimSpace(*in.ParentGatewayNamespace) != "" {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			aitool.BadRequest("argument_invalid", "cannot specify both parentGatewayNamespace and clearParentGatewayNamespace=true")
	}
	if in.URLRewrite != nil && in.RequestRedirect != nil {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			aitool.BadRequest("argument_invalid", "urlRewrite and requestRedirect are mutually exclusive")
	}

	// Cross-side filter conflict protection (catalog §3B.2 #31 rule 2)
	clearRewrite := in.ClearURLRewrite != nil && *in.ClearURLRewrite
	clearRedirect := in.ClearRequestRedirect != nil && *in.ClearRequestRedirect
	if in.URLRewrite != nil && strings.TrimSpace(existing.RequestRedirect) != "" && !clearRedirect {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			aitool.NewErrorWithHint(
				http.StatusConflict,
				"gateway_route.conflicting_filter",
				"route already has requestRedirect configured; urlRewrite and requestRedirect cannot coexist",
				"Pass clearRequestRedirect=true along with urlRewrite to switch filter types.",
			)
	}
	if in.RequestRedirect != nil && strings.TrimSpace(existing.URLRewrite) != "" && !clearRewrite {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			aitool.NewErrorWithHint(
				http.StatusConflict,
				"gateway_route.conflicting_filter",
				"route already has urlRewrite configured; requestRedirect and urlRewrite cannot coexist",
				"Pass clearUrlRewrite=true along with requestRedirect to switch filter types.",
			)
	}

	// Resolve application & target switching (catalog §3B.2 #31)
	appID := existing.ApplicationID
	if in.Application != nil && strings.TrimSpace(*in.Application) != "" {
		newApp, err := aitool.ResolveApplication(tc.DB, project.ID, *in.Application)
		if err != nil {
			return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
		}
		if newApp.ID != existing.ApplicationID && (in.Target == nil || strings.TrimSpace(*in.Target) == "") {
			return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
				aitool.NewErrorWithHint(
					http.StatusBadRequest,
					"argument_invalid",
					"switching application requires specifying target under the new application",
					"Specify target (stage, name, or ID) belonging to the new application.",
				)
		}
		appID = newApp.ID
	}

	targetID := existing.DeploymentTargetID
	var resolvedTarget *model.DeploymentTarget
	if in.Target != nil && strings.TrimSpace(*in.Target) != "" {
		newTarget, err := aitool.ResolveDeploymentTarget(tc.DB, project.ID, appID, *in.Target)
		if err != nil {
			return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
		}
		targetID = newTarget.ID
		resolvedTarget = &newTarget
	}

	servicePort := existing.ServicePort
	if in.ServicePort != nil {
		servicePort = *in.ServicePort
	} else if resolvedTarget != nil && resolvedTarget.ID != existing.DeploymentTargetID {
		if !deploymentTargetHasServicePort(*resolvedTarget, existing.ServicePort) {
			servicePort = deploymentTargetServicePort(*resolvedTarget)
		}
	}

	tlsMode, err := validateStrictTLSMode(in.TLSMode, existing.TLSMode)
	if err != nil {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
	}
	pathMatchType, err := validateStrictPathMatchType(in.PathMatchType, normalizeHTTPRoutePathMatchType(existing.PathMatchType))
	if err != nil {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
	}

	hostVal := existing.Host
	if in.Host != nil && strings.TrimSpace(*in.Host) != "" {
		hostVal = strings.TrimSpace(*in.Host)
	}
	domainSuffix := existing.DomainSuffix
	if in.DomainSuffix != nil && strings.TrimSpace(*in.DomainSuffix) != "" {
		domainSuffix = strings.TrimSpace(*in.DomainSuffix)
	}
	pathVal := existing.Path
	if in.Path != nil && strings.TrimSpace(*in.Path) != "" {
		pathVal = strings.TrimSpace(*in.Path)
	}

	sectionName := existing.SectionName
	if in.ClearSectionName != nil && *in.ClearSectionName {
		sectionName = ""
	} else if in.SectionName != nil {
		sectionName = strings.TrimSpace(*in.SectionName)
	}
	parentGwName := existing.ParentGatewayName
	if in.ClearParentGatewayName != nil && *in.ClearParentGatewayName {
		parentGwName = ""
	} else if in.ParentGatewayName != nil {
		parentGwName = strings.TrimSpace(*in.ParentGatewayName)
	}
	parentGwNs := existing.ParentGatewayNamespace
	if in.ClearParentGatewayNamespace != nil && *in.ClearParentGatewayNamespace {
		parentGwNs = ""
	} else if in.ParentGatewayNamespace != nil {
		parentGwNs = strings.TrimSpace(*in.ParentGatewayNamespace)
	}

	enabled := existing.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	isDefault := existing.IsDefault
	if in.IsDefault != nil {
		isDefault = *in.IsDefault
	}
	backendWeight := normalizeBackendWeight(existing.BackendWeight)
	if in.BackendWeight != nil {
		if *in.BackendWeight < 1 {
			return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
				aitool.BadRequest("argument_invalid", "backendWeight must be >= 1")
		}
		backendWeight = *in.BackendWeight
	}

	reqHeadersRaw, err := mergeGatewayHeaderPatch(existing.RequestHeaders, in.UpsertRequestHeaders, in.RemoveRequestHeaderKeys)
	if err != nil {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
	}
	respHeadersRaw, err := mergeGatewayHeaderPatch(existing.ResponseHeaders, in.UpsertResponseHeaders, in.RemoveResponseHeaderKeys)
	if err != nil {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
	}
	urlRewriteRaw, err := mergeAndEncodeURLRewrite(existing.URLRewrite, in.URLRewrite, clearRewrite)
	if err != nil {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
	}
	reqRedirectRaw, err := mergeAndEncodeRequestRedirect(existing.RequestRedirect, in.RequestRedirect, clearRedirect)
	if err != nil {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{}, err
	}

	gwInput := gatewayRouteInput{
		ApplicationID:          appID,
		DeploymentTargetID:     targetID,
		Host:                   hostVal,
		DomainSuffix:           domainSuffix,
		Path:                   pathVal,
		ServicePort:            servicePort,
		TLSMode:                tlsMode,
		Enabled:                &enabled,
		IsDefault:              isDefault,
		ParentGatewayName:      parentGwName,
		ParentGatewayNamespace: parentGwNs,
		SectionName:            sectionName,
		PathMatchType:          pathMatchType,
		RequestHeaders:         reqHeadersRaw,
		ResponseHeaders:        respHeadersRaw,
		URLRewrite:             urlRewriteRaw,
		RequestRedirect:        reqRedirectRaw,
		BackendWeight:          backendWeight,
		HostnameAliases:        existing.HostnameAliases, // Preserve existing DB column untouched without exposing it
	}

	c, rec := newGatewayAIGin(tc)
	next, ok := h.gatewayRouteFromInput(c, project, user, existing.CreatedBy, gwInput, existing.ID)
	if !ok {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			gatewayAIErrorFromRecorder(rec, "gateway_route.invalid", "gateway route validation failed")
	}

	cBackend, recBackend := newGatewayAIGin(tc)
	if !h.ensureGatewayRouteBackendAvailable(cBackend, next) {
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			gatewayAIErrorFromRecorder(recBackend, "gateway_route.backend_unavailable", "gateway route backend service check failed")
	}

	next, urlErr := h.gatewayRouteWithAccessURL(next, tc.Context())
	if urlErr != nil {
		cURL, recURL := newGatewayAIGin(tc)
		h.writeGatewayRuntimeClusterError(cURL, urlErr)
		return model.User{}, model.Project{}, model.GatewayRoute{}, model.GatewayRoute{},
			gatewayAIErrorFromRecorder(recURL, "gateway_route.runtime_cluster_unavailable", urlErr.Error())
	}

	return user, project, existing, next, nil
}

func (h *Handler) registerGatewayAITools(reg *aitool.Registry) {
	// 18. network_list_gateway_routes
	aitool.Register(reg, aitool.Spec[networkListGatewayRoutesInput, map[string]any]{
		Name:           "network_list_gateway_routes",
		Domain:         aitool.DomainNetwork,
		Title:          "分页列出网关访问入口",
		Summary:        "分页列出项目的网关访问入口及实时 HTTPRoute、TLS 证书与可访问 URL 状态",
		AliasesZH:      []string{"网关路由列表", "查看域名访问入口", "查询网关路由"},
		AliasesEN:      []string{"list gateway routes", "inspect gateway route"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionGatewayRead)},
		Execute: func(tc *aitool.ToolContext, in networkListGatewayRoutesInput) (map[string]any, error) {
			_, project, err := h.authorizeGatewayAIProject(tc, in.Project, authz.ActionGatewayRead)
			if err != nil {
				return nil, err
			}
			tc.SetNoStore()

			sortBy, sortOrder, orderClause, err := normalizeGatewaySort(in.SortBy, in.SortOrder)
			if err != nil {
				return nil, err
			}
			page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

			appID := ""
			if in.Application != nil && strings.TrimSpace(*in.Application) != "" {
				app, err := aitool.ResolveApplication(tc.DB, project.ID, *in.Application)
				if err != nil {
					return nil, err
				}
				appID = app.ID
			}

			var routes []model.GatewayRoute
			var total int64

			if in.Route != nil && strings.TrimSpace(*in.Route) != "" {
				single, found, err := resolveAIGatewayRoute(tc.DB, project.ID, appID, *in.Route, true)
				if err != nil {
					return nil, err
				}
				if found {
					routes = []model.GatewayRoute{single}
					total = 1
				}
			} else {
				query := tc.DB.Model(&model.GatewayRoute{}).Where("project_id = ?", project.ID)
				if appID != "" {
					query = query.Where("application_id = ?", appID)
				}
				if in.Search != nil {
					query = applyGatewaySearchFilter(query, *in.Search)
				}
				if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
					return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
				}
				offset := (page - 1) * pageSize
				if err := query.Order(orderClause).Limit(pageSize).Offset(offset).Find(&routes).Error; err != nil {
					return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
				}
			}

			if len(routes) > 0 {
				routes = h.observeGatewayRoutes(tc.Context(), routes)
				routes = h.gatewayRoutesWithAccessURL(routes, tc.Context())
			}

			dtos := make([]aiGatewayRouteDTO, 0, len(routes))
			for _, r := range routes {
				dtos = append(dtos, toAIGatewayRouteDTO(r))
			}

			out := aitool.PageEnvelope(dtos, page, pageSize, sortBy, sortOrder, total)
			out["projectId"] = project.ID
			return out, nil
		},
	})

	// 19. network_check_gateway_domain
	aitool.Register(reg, aitool.Spec[networkCheckGatewayDomainInput, map[string]any]{
		Name:           "network_check_gateway_domain",
		Domain:         aitool.DomainNetwork,
		Title:          "预检网关候选域名与 DNS 状态",
		Summary:        "检查候选域名在目标运行集群下是否可用、所需 CNAME 目标及实时 DNS 解析状态",
		AliasesZH:      []string{"检查域名可用性", "域名预检", "检查 CNAME 解析"},
		AliasesEN:      []string{"check gateway domain", "verify cname"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionGatewayRead)},
		Execute: func(tc *aitool.ToolContext, in networkCheckGatewayDomainInput) (map[string]any, error) {
			_, project, err := h.authorizeGatewayAIProject(tc, in.Project, authz.ActionGatewayRead)
			if err != nil {
				return nil, err
			}
			tc.SetNoStore()

			hasApp := in.Application != nil && strings.TrimSpace(*in.Application) != ""
			hasTarget := in.Target != nil && strings.TrimSpace(*in.Target) != ""
			hasRoute := in.Route != nil && strings.TrimSpace(*in.Route) != ""

			if hasApp != hasTarget {
				return nil, aitool.NewErrorWithHint(
					http.StatusBadRequest,
					"argument_invalid",
					"application and target must be provided together",
					"Specify both application and target, or specify route to reuse an existing gateway route's cluster.",
				)
			}
			if !hasApp && !hasRoute {
				return nil, aitool.NewErrorWithHint(
					http.StatusBadRequest,
					"gateway_route.runtime_cluster_reference_required",
					"either (application + target) or route is required to resolve the runtime cluster",
					"Pass application + target when planning a new route, or route when checking a replacement host for an existing route.",
				)
			}

			var routeID string
			var cluster model.RuntimeCluster
			if hasRoute {
				existingRoute, _, err := resolveAIGatewayRoute(tc.DB, project.ID, "", *in.Route, false)
				if err != nil {
					return nil, err
				}
				routeID = existingRoute.ID
				if !hasApp {
					cluster, err = h.runtimeClusterForGatewayRoute(existingRoute, tc.Context())
					if err != nil {
						c, rec := newGatewayAIGin(tc)
						h.writeGatewayRuntimeClusterError(c, err)
						return nil, gatewayAIErrorFromRecorder(rec, "gateway_route.runtime_cluster_missing", err.Error())
					}
				}
			}
			if hasApp {
				app, err := aitool.ResolveApplication(tc.DB, project.ID, *in.Application)
				if err != nil {
					return nil, err
				}
				target, err := aitool.ResolveDeploymentTarget(tc.DB, project.ID, app.ID, *in.Target)
				if err != nil {
					return nil, err
				}
				cluster, err = h.runtimeClusterForDeploymentTargetValue(target, tc.Context())
				if err != nil {
					c, rec := newGatewayAIGin(tc)
					h.writeGatewayRuntimeClusterError(c, err)
					return nil, gatewayAIErrorFromRecorder(rec, "gateway_route.runtime_cluster_missing", err.Error())
				}
			}

			rawSuffix := ""
			if in.DomainSuffix != nil {
				rawSuffix = *in.DomainSuffix
			}
			cSuf, recSuf := newGatewayAIGin(tc)
			domainSuffix, ok := h.gatewayRouteDomainSuffix(cSuf, rawSuffix, cluster)
			if !ok {
				return nil, gatewayAIErrorFromRecorder(recSuf, "gateway_route.domain_suffix_invalid", "invalid domain suffix")
			}

			normalizedHost := h.normalizeGatewayHost(strings.TrimSpace(in.Host), cluster, domainSuffix)
			if normalizedHost == "" {
				return nil, aitool.BadRequest("argument_invalid", "host is required and must be a valid domain or subdomain label")
			}

			var existingRoutes []model.GatewayRoute
			if err := tc.DB.Select("id").Where("host = ?", normalizedHost).Find(&existingRoutes).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			status := "available"
			available := true
			for _, r := range existingRoutes {
				if routeID != "" && r.ID == routeID {
					status = "current"
					continue
				}
				status = "conflict"
				available = false
				break
			}

			cnameName := normalizedHost
			cnameTarget := h.gatewayCNAMETarget(cluster, domainSuffix)
			dnsStatus := observeGatewayRouteDNS(tc.Context(), model.GatewayRoute{
				CNAMEName:   cnameName,
				CNAMETarget: cnameTarget,
			})

			return map[string]any{
				"projectId":    project.ID,
				"available":    available,
				"host":         normalizedHost,
				"domainSuffix": domainSuffix,
				"status":       status,
				"cnameName":    cnameName,
				"cnameTarget":  cnameTarget,
				"dnsStatus":    dnsStatus,
			}, nil
		},
	})

	// 20. network_create_gateway_route
	aitool.Register(reg, aitool.Spec[networkCreateGatewayRouteInput, aiGatewayRouteDTO]{
		Name:           "network_create_gateway_route",
		Domain:         aitool.DomainNetwork,
		Title:          "创建网关访问入口",
		Summary:        "为已部署且非 internalOnly 的部署环境创建网关访问入口，异步投递网关路由下发",
		AliasesZH:      []string{"创建网关路由", "绑定域名", "开放公网访问"},
		AliasesEN:      []string{"create gateway route", "expose domain"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionGatewayManage)},
		Verification: []aitool.OperationVerification{
			{OperationID: "network_list_gateway_routes", Family: "gateway_route", Expect: "present"},
		},
		Preview: func(tc *aitool.ToolContext, in networkCreateGatewayRouteInput) (*aitool.PreviewResult, error) {
			_, project, app, target, route, lockPayload, err := h.prepareAIGatewayRouteCreate(tc, in, nil)
			if err != nil {
				return nil, err
			}

			changes := []aitool.PreviewChange{
				{Field: "host", After: route.Host, Action: "create"},
				{Field: "path", After: fmt.Sprintf("%s (%s)", route.Path, route.PathMatchType), Action: "create"},
				{Field: "backend", After: fmt.Sprintf("%s/%s:%d", app.Identifier, target.Stage, route.ServicePort), Action: "create"},
				{Field: "tlsMode", After: route.TLSMode, Action: "create"},
				{Field: "enabled", After: route.Enabled, Action: "create"},
			}
			impact := []string{
				fmt.Sprintf("Exposes %s (%s) externally at %s", app.Identifier, target.Stage, firstNonEmpty(route.AccessURL, route.Host+route.Path)),
			}
			if route.CNAMETarget != "" {
				impact = append(impact, fmt.Sprintf("DNS CNAME required: %s -> %s", route.CNAMEName, route.CNAMETarget))
			}

			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("Create gateway route %s%s -> %s (%s:%d)", route.Host, route.Path, app.Identifier, target.Stage, route.ServicePort),
					TargetResource: "gateway_route",
					ProjectID:      project.ID,
					Changes:        changes,
					Impact:         impact,
				},
				LockedPayload: lockPayload,
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in networkCreateGatewayRouteInput) (aiGatewayRouteDTO, error) {
			var lockPtr *createGatewayRouteLockedPayload
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked createGatewayRouteLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return aiGatewayRouteDTO{}, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				lockPtr = &locked
			}

			user, project, _, _, route, _, err := h.prepareAIGatewayRouteCreate(tc, in, lockPtr)
			if err != nil {
				return aiGatewayRouteDTO{}, err
			}

			route.ID = id.New("gwr")
			if err := persistGatewayRouteForTarget(tc.DB, func(tx *gorm.DB) error {
				desiredEnabled := route.Enabled
				if err := tx.Create(&route).Error; err != nil {
					return err
				}
				if !desiredEnabled && route.Enabled {
					if err := tx.Model(&model.GatewayRoute{}).Where("id = ?", route.ID).UpdateColumn("enabled", false).Error; err != nil {
						return err
					}
					route.Enabled = false
				}
				return nil
			}, route.DeploymentTargetID); err != nil {
				if errors.Is(err, errGatewayRouteTargetInternal) {
					return aiGatewayRouteDTO{}, aitool.Conflict("gateway_route.target_internal", "the deployment target is internal and cannot have gateway routes")
				}
				return aiGatewayRouteDTO{}, aitool.BadRequest("gateway_route.create_failed", err.Error())
			}
			if !h.enqueueGatewayApply(tc.Context(), route, user.ID) {
				return aiGatewayRouteDTO{}, aitool.NewError(http.StatusServiceUnavailable, "gateway_route.enqueue_failed", "failed to enqueue gateway apply task")
			}

			h.auditWithContext(user.ID, "gateway_route.create", route.ID, true, runtimecluster.EgressExceptionAuditMarker(tc.DB, project.ID), tc.Context())
			route.Status = "progressing"
			route.ObservationCode = "gateway_route.apply_queued"
			return toAIGatewayRouteDTO(route), nil
		},
	})

	// 21. network_update_gateway_route
	aitool.Register(reg, aitool.Spec[networkUpdateGatewayRouteInput, aiGatewayRouteDTO]{
		Name:           "network_update_gateway_route",
		Domain:         aitool.DomainNetwork,
		Title:          "修改网关访问入口配置",
		Summary:        "基于存量记录纯增量修改网关访问入口配置与高级转发规则（绝不清空未传字段）",
		AliasesZH:      []string{"更新网关路由", "修改域名配置", "配置路由转发"},
		AliasesEN:      []string{"update gateway route", "patch gateway route"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionGatewayManage)},
		Verification: []aitool.OperationVerification{
			{OperationID: "network_list_gateway_routes", Family: "gateway_route", Expect: "present"},
		},
		Preview: func(tc *aitool.ToolContext, in networkUpdateGatewayRouteInput) (*aitool.PreviewResult, error) {
			_, project, existing, next, err := h.prepareAIGatewayRouteUpdate(tc, in, nil)
			if err != nil {
				return nil, err
			}

			changes := make([]aitool.PreviewChange, 0, 8)
			addDiff := func(field string, before, after any) {
				if fmt.Sprint(before) != fmt.Sprint(after) {
					changes = append(changes, aitool.PreviewChange{Field: field, Before: before, After: after, Action: "update"})
				}
			}
			addDiff("host", existing.Host, next.Host)
			addDiff("path", existing.Path, next.Path)
			addDiff("pathMatchType", normalizeHTTPRoutePathMatchType(existing.PathMatchType), next.PathMatchType)
			addDiff("deploymentTargetId", existing.DeploymentTargetID, next.DeploymentTargetID)
			addDiff("servicePort", existing.ServicePort, next.ServicePort)
			addDiff("tlsMode", existing.TLSMode, next.TLSMode)
			addDiff("enabled", existing.Enabled, next.Enabled)
			addDiff("isDefault", existing.IsDefault, next.IsDefault)
			addDiff("sectionName", existing.SectionName, next.SectionName)
			addDiff("parentGatewayName", existing.ParentGatewayName, next.ParentGatewayName)
			addDiff("parentGatewayNamespace", existing.ParentGatewayNamespace, next.ParentGatewayNamespace)
			addDiff("backendWeight", normalizeBackendWeight(existing.BackendWeight), next.BackendWeight)
			addDiff("requestHeaders", existing.RequestHeaders, next.RequestHeaders)
			addDiff("responseHeaders", existing.ResponseHeaders, next.ResponseHeaders)
			addDiff("urlRewrite", existing.URLRewrite, next.URLRewrite)
			addDiff("requestRedirect", existing.RequestRedirect, next.RequestRedirect)

			sort.SliceStable(changes, func(i, j int) bool { return changes[i].Field < changes[j].Field })

			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("Update gateway route %s (%s%s)", existing.ID, next.Host, next.Path),
					TargetResource: "gateway_route",
					TargetID:       existing.ID,
					ProjectID:      project.ID,
					Changes:        changes,
					Impact: []string{
						fmt.Sprintf("Asynchronously reconciles HTTPRoute %s (%s)", existing.ID, firstNonEmpty(next.AccessURL, next.Host+next.Path)),
					},
				},
				LockedPayload: gatewayRouteIDLockedPayload{
					ProjectID: project.ID,
					RouteID:   existing.ID,
				},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in networkUpdateGatewayRouteInput) (aiGatewayRouteDTO, error) {
			var lockPtr *gatewayRouteIDLockedPayload
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked gatewayRouteIDLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return aiGatewayRouteDTO{}, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				lockPtr = &locked
			}

			user, project, route, next, err := h.prepareAIGatewayRouteUpdate(tc, in, lockPtr)
			if err != nil {
				return aiGatewayRouteDTO{}, err
			}

			route.ApplicationID = next.ApplicationID
			route.DeploymentTargetID = next.DeploymentTargetID
			route.Host = next.Host
			route.DomainSuffix = next.DomainSuffix
			route.Path = next.Path
			route.ServicePort = next.ServicePort
			route.TLSMode = next.TLSMode
			route.CNAMEName = next.CNAMEName
			route.CNAMETarget = next.CNAMETarget
			route.Enabled = next.Enabled
			route.IsDefault = next.IsDefault
			route.ParentGatewayName = next.ParentGatewayName
			route.ParentGatewayNamespace = next.ParentGatewayNamespace
			route.SectionName = next.SectionName
			route.PathMatchType = next.PathMatchType
			route.RequestHeaders = next.RequestHeaders
			route.ResponseHeaders = next.ResponseHeaders
			route.URLRewrite = next.URLRewrite
			route.RequestRedirect = next.RequestRedirect
			route.BackendWeight = next.BackendWeight
			route.HostnameAliases = next.HostnameAliases
			route.AccessURL = next.AccessURL

			if err := persistGatewayRouteForTarget(tc.DB, func(tx *gorm.DB) error { return tx.Save(&route).Error }, route.DeploymentTargetID); err != nil {
				if errors.Is(err, errGatewayRouteTargetInternal) {
					return aiGatewayRouteDTO{}, aitool.Conflict("gateway_route.target_internal", "the deployment target is internal and cannot have gateway routes")
				}
				return aiGatewayRouteDTO{}, aitool.BadRequest("gateway_route.update_failed", err.Error())
			}
			if !h.enqueueGatewayApply(tc.Context(), route, user.ID) {
				return aiGatewayRouteDTO{}, aitool.NewError(http.StatusServiceUnavailable, "gateway_route.enqueue_failed", "failed to enqueue gateway apply task")
			}

			h.auditWithContext(user.ID, "gateway_route.update", route.ID, true, runtimecluster.EgressExceptionAuditMarker(tc.DB, project.ID), tc.Context())
			route.Status = "progressing"
			route.ObservationCode = "gateway_route.apply_queued"
			return toAIGatewayRouteDTO(route), nil
		},
	})

	// 22. network_delete_gateway_route
	aitool.Register(reg, aitool.Spec[networkDeleteGatewayRouteInput, map[string]any]{
		Name:           "network_delete_gateway_route",
		Domain:         aitool.DomainNetwork,
		Title:          "删除网关访问入口",
		Summary:        "异步删除网关访问入口并回收集群 HTTPRoute 与证书资源",
		AliasesZH:      []string{"删除网关路由", "移除域名访问"},
		AliasesEN:      []string{"delete gateway route", "remove gateway route"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionGatewayDelete)},
		Verification: []aitool.OperationVerification{
			{OperationID: "network_list_gateway_routes", Family: "gateway_route", Expect: "absent"},
		},
		Preview: func(tc *aitool.ToolContext, in networkDeleteGatewayRouteInput) (*aitool.PreviewResult, error) {
			_, project, err := h.authorizeGatewayAIProject(tc, in.Project, authz.ActionGatewayDelete)
			if err != nil {
				return nil, err
			}
			if err := h.ensureGatewayAIProjectMutable(tc, project); err != nil {
				return nil, err
			}
			existing, _, err := resolveAIGatewayRoute(tc.DB, project.ID, "", in.Route, false)
			if err != nil {
				return nil, err
			}
			if !h.deleteStatusCanStart(existing.DeleteStatus) {
				return nil, aitool.Conflict("gateway_route.delete_in_progress", "gateway route is already being deleted")
			}

			accessDesc := existing.Host + existing.Path
			if withURL, urlErr := h.gatewayRouteWithAccessURL(existing, tc.Context()); urlErr == nil && withURL.AccessURL != "" {
				accessDesc = withURL.AccessURL
			}

			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("Delete gateway route %s (%s)", existing.ID, accessDesc),
					TargetResource: "gateway_route",
					TargetID:       existing.ID,
					ProjectID:      project.ID,
					Changes: []aitool.PreviewChange{
						{Field: "deleteStatus", Before: existing.DeleteStatus, After: "deleting", Action: "delete"},
					},
					Impact: []string{
						fmt.Sprintf("External HTTP(S) traffic to %s will be terminated once cluster cleanup completes", accessDesc),
					},
				},
				LockedPayload: gatewayRouteIDLockedPayload{
					ProjectID: project.ID,
					RouteID:   existing.ID,
				},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in networkDeleteGatewayRouteInput) (map[string]any, error) {
			user, project, err := h.authorizeGatewayAIProject(tc, in.Project, authz.ActionGatewayDelete)
			if err != nil {
				return nil, err
			}
			if err := h.ensureGatewayAIProjectMutable(tc, project); err != nil {
				return nil, err
			}

			var route model.GatewayRoute
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked gatewayRouteIDLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				if locked.ProjectID != project.ID || strings.TrimSpace(locked.RouteID) == "" {
					return nil, aitool.Conflict("ai.preview_lock_mismatch", "preview lock does not match target gateway route")
				}
				if err := tc.DB.First(&route, "id = ? AND project_id = ?", locked.RouteID, project.ID).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return nil, aitool.NotFound("gateway_route.not_found", "gateway route was not found")
					}
					return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
				}
			} else {
				route, _, err = resolveAIGatewayRoute(tc.DB, project.ID, "", in.Route, false)
				if err != nil {
					return nil, err
				}
			}

			if !h.deleteStatusCanStart(route.DeleteStatus) {
				return nil, aitool.Conflict("gateway_route.delete_in_progress", "gateway route is already being deleted")
			}
			if err := h.markResourceDeleting(tc.DB, &model.GatewayRoute{}, route.ID); err != nil {
				if h.host.ResourceDeleteAlreadyStarted(err) {
					return nil, aitool.Conflict("gateway_route.delete_in_progress", "gateway route is already being deleted")
				}
				return nil, aitool.NewError(http.StatusInternalServerError, "gateway_route.delete_failed", err.Error())
			}
			if !h.enqueueResourceCleanup(tc.Context(), "gateway_route", route.ID, route.ProjectID, user.ID) {
				_ = h.markResourceDeleteFailed(tc.DB, &model.GatewayRoute{}, route.ID, "failed to enqueue gateway route cleanup")
				h.auditWithContext(user.ID, "gateway.delete", route.ID, false, "cleanup_enqueue_failed", tc.Context())
				return nil, aitool.NewError(http.StatusServiceUnavailable, "gateway_route.cleanup_enqueue_failed", "failed to enqueue gateway route cleanup")
			}
			h.auditWithContext(user.ID, "gateway.delete", route.ID, true, "cleanup_queued", tc.Context())
			return map[string]any{
				"id":           route.ID,
				"routeId":      route.ID,
				"projectId":    project.ID,
				"host":         route.Host,
				"path":         route.Path,
				"deleteStatus": "deleting",
			}, nil
		},
	})
}
