package runtimeapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// RegisterAITools registers the 16 AI Domain Facade Tools owned by runtimeapi:
//   - delivery config-set tools (5)
//   - runtime cluster & K8s governance tools (9)
//   - admin system component tools (2)
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	if h == nil || reg == nil {
		return
	}
	h.registerConfigSetAITools(reg)
	h.registerClusterAITools(reg)
	h.registerClusterGovernanceAITools(reg)
	h.registerSystemComponentAITools(reg)
}

func newSubGin(tc *aitool.ToolContext) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	sub, _ := gin.CreateTestContext(rec)
	req, _ := http.NewRequestWithContext(tc.Context(), http.MethodPost, "/internal/v1/ai/tools", http.NoBody)
	if tc != nil && tc.Gin != nil && tc.Gin.Request != nil {
		for k, vals := range tc.Gin.Request.Header {
			req.Header[k] = append([]string(nil), vals...)
		}
		req.RemoteAddr = tc.Gin.Request.RemoteAddr
	}
	if tc != nil && strings.TrimSpace(tc.IdempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(tc.IdempotencyKey))
	}
	sub.Request = req
	if tc != nil && tc.Gin != nil && tc.Gin.Keys != nil {
		sub.Keys = maps.Clone(tc.Gin.Keys)
	}
	return sub, rec
}

func abortedToolError(rec *httptest.ResponseRecorder, fallbackCode, fallbackMessage string) *aitool.ToolError {
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
	return &aitool.ToolError{
		Status:  status,
		Code:    code,
		Message: msg,
		RawBody: append(json.RawMessage(nil), raw...),
	}
}

func (h *Handler) authorizeAIProject(tc *aitool.ToolContext, projectRef string, action authz.Action) (model.User, model.Project, error) {
	projectRef = strings.TrimSpace(projectRef)
	if projectRef == "" {
		return model.User{}, model.Project{}, aitool.BadRequest("project.required", "project is required")
	}
	sub, rec := newSubGin(tc)
	user, project, ok := h.authorizeProjectRef(sub, projectRef, action)
	if !ok {
		return model.User{}, model.Project{}, abortedToolError(rec, "project.forbidden", "project authorization failed")
	}
	if strings.TrimSpace(user.ID) == "" {
		user = tc.User
	}
	return user, project, nil
}

func (h *Handler) ensureAIProjectMutatable(tc *aitool.ToolContext, project model.Project) error {
	sub, rec := newSubGin(tc)
	if !h.ensureProjectCanMutate(sub, project) {
		return abortedToolError(rec, "project.not_mutable", "project cannot be mutated right now")
	}
	return nil
}

func (h *Handler) ensureAIConfigSetMutatable(tc *aitool.ToolContext, set model.ProjectRuntimeConfigSet) error {
	sub, rec := newSubGin(tc)
	if !h.ensureRuntimeConfigSetCanMutate(sub, set) {
		return abortedToolError(rec, "runtime_config.not_mutable", "runtime config set cannot be mutated right now")
	}
	return nil
}

func (h *Handler) requireAIPlatformAdmin(tc *aitool.ToolContext) (model.User, error) {
	if tc == nil || !authz.IsPlatformAdmin(tc.User.Role) {
		return model.User{}, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		err := aitool.Forbidden("ai.project_scope_mismatch", "platform administration tool is not allowed in a single-project conversation")
		err.Hint = "Switch to an unbound global conversation when operating platform-wide resources."
		return model.User{}, err
	}
	return tc.User, nil
}

func projectScopeMismatchError(detail string) *aitool.ToolError {
	err := aitool.Forbidden("ai.project_scope_mismatch", detail)
	err.Hint = "Switch to an unbound global conversation when operating across projects or global platform resources."
	return err
}

func equalSingleProject(projectIDs []string, expectedProjectID string) bool {
	normalized := normalizeStringList(projectIDs)
	return len(normalized) == 1 && normalized[0] == strings.TrimSpace(expectedProjectID)
}

func enforceScopedClusterSessionInvariant(tc *aitool.ToolContext, scope, ownerRef string, projectIDs []string) error {
	convProjectID := strings.TrimSpace(tc.ConversationProjectID)
	if convProjectID == "" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "user":
		if strings.TrimSpace(ownerRef) == strings.TrimSpace(tc.User.ID) {
			return nil
		}
	case "project":
		if equalSingleProject(projectIDs, convProjectID) {
			return nil
		}
	}
	return projectScopeMismatchError("runtime cluster scope does not match the bound conversation project")
}

// resolveActiveClusterForAI resolves an active RuntimeCluster by ID (`clu_xxx`) or
// unique visible name. When allowDeleting is true (for delete retry/conflict handling),
// non-active clusters are also matched by ID/name before deleteStatus checks run.
func (h *Handler) resolveClusterForAI(tc *aitool.ToolContext, ref string, projectID string, allowNonActive bool) (model.RuntimeCluster, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.RuntimeCluster{}, aitool.BadRequest("runtime_cluster.required", "cluster is required")
	}
	db := h.dbWithContext(tc.Context())
	baseQuery := db.Model(&model.RuntimeCluster{})
	if !allowNonActive {
		baseQuery = runtimecluster.ActiveScope(db).Model(&model.RuntimeCluster{})
	}

	// Exact ID lookup first so explicit IDs yield 403 (rather than 404/ambiguous) when unauthorized.
	var byID model.RuntimeCluster
	if err := baseQuery.Session(&gorm.Session{}).Where("id = ?", ref).First(&byID).Error; err == nil {
		return byID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.RuntimeCluster{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	visibility := projectservice.ListVisibilityRelated
	if strings.TrimSpace(projectID) == "" && strings.TrimSpace(tc.ConversationProjectID) == "" && authz.IsPlatformAdmin(tc.User.Role) {
		visibility = projectservice.ListVisibilityAll
	}
	sub, rec := newSubGin(tc)
	visibleQuery, visible := h.applyScopedResourceListVisibility(sub, baseQuery.Session(&gorm.Session{}), scopedResourceRuntimeCluster, tc.User, strings.TrimSpace(projectID), visibility)
	if !visible {
		return model.RuntimeCluster{}, abortedToolError(rec, "runtime_cluster.forbidden", "cluster visibility check failed")
	}
	var candidates []model.RuntimeCluster
	if err := visibleQuery.Order("created_at asc").Find(&candidates).Error; err != nil {
		return model.RuntimeCluster{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		candidates,
		"runtime_cluster",
		"runtime_cluster.not_found",
		ref,
		func(item model.RuntimeCluster) string { return item.ID },
		func(item model.RuntimeCluster) string { return item.Name },
	)
}

func (h *Handler) requireClusterManageForAI(tc *aitool.ToolContext, cluster model.RuntimeCluster, message string) error {
	sub, rec := newSubGin(tc)
	if !h.canManageScopedResourceByID(sub, tc.User, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, message) {
		return abortedToolError(rec, "runtime_cluster.forbidden", message)
	}
	return nil
}

func classifyClusterK8sResourceError(err error) *aitool.ToolError {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case apierrors.IsNotFound(err) || strings.Contains(msg, "does not exist"):
		return aitool.NotFound("cluster.resource_not_found", "cluster resource was not found")
	case strings.Contains(msg, "resource is not managed by Luna DevOps"):
		return aitool.Forbidden("cluster.resource_unmanaged", "resource is not managed by Luna DevOps")
	case strings.Contains(msg, "resource namespace is required"):
		return aitool.BadRequest("cluster.resource_namespace_required", "namespace is required for namespaced Kubernetes resources")
	case strings.Contains(msg, "resource name is required"):
		return aitool.BadRequest("cluster.resource_name_required", "resourceName is required")
	case strings.Contains(msg, "unsupported resource kind"):
		return aitool.BadRequest("cluster.resource_kind_invalid", msg)
	case apierrors.IsForbidden(err):
		return aitool.Forbidden("cluster.resource_forbidden", "cluster credentials do not permit accessing this resource")
	default:
		return aitool.NewError(http.StatusBadGateway, "cluster.upstream_unavailable", "failed to inspect or mutate cluster resource")
	}
}

func truncateUTF8Bytes(content string, maxBytes int) (string, bool, int) {
	if maxBytes <= 0 || len(content) <= maxBytes {
		return content, false, 0
	}
	origLen := len(content)
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(content[cut]) {
		cut--
	}
	// If cut lands inside an incomplete multi-byte rune at the end, step back if invalid.
	if cut < origLen {
		r, width := utf8.DecodeRuneInString(content[cut:])
		if r == utf8.RuneError && width == 1 {
			for cut > 0 {
				cut--
				if utf8.ValidString(content[:cut]) {
					break
				}
			}
		} else if cut+width > maxBytes {
			// Do not include a rune that crosses maxBytes.
		} else {
			cut += width
		}
	}
	for cut > maxBytes {
		_, size := utf8.DecodeLastRuneInString(content[:cut])
		if size <= 0 {
			cut--
		} else {
			cut -= size
		}
	}
	for cut > 0 && !utf8.ValidString(content[:cut]) {
		cut--
	}
	return content[:cut], true, origLen - cut
}

func normalizeContainerAbsPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return "", aitool.BadRequest("deployment.runtime_config_path_invalid", "file path must be a non-root absolute path starting with '/'")
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == ".." {
			return "", aitool.BadRequest("deployment.runtime_config_path_invalid", "file path must not contain '..' segments")
		}
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "/" || !strings.HasPrefix(cleaned, "/") {
		return "", aitool.BadRequest("deployment.runtime_config_path_invalid", "file path must not be root '/'")
	}
	return cleaned, nil
}

func pathsHavePrefixConflict(left, right string) bool {
	if left == "" || right == "" || left == right {
		return false
	}
	return strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func escapeLikePattern(keyword string) string {
	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.TrimSpace(keyword))
	return "%" + escaped + "%"
}
