package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/kubeaccess"
	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type runtimeClusterKubeGatewayRule struct {
	APIGroup     string   `json:"apiGroup"`
	APIVersion   string   `json:"apiVersion"`
	Resource     string   `json:"resource"`
	Subresources []string `json:"subresources,omitempty"`
	Verbs        []string `json:"verbs"`
	Action       string   `json:"action"`
}

type runtimeClusterKubeGatewayInput struct {
	Enabled            bool                            `json:"enabled"`
	ExtraResourceRules []runtimeClusterKubeGatewayRule `json:"extraResourceRules"`
}

type runtimeClusterKubeGatewayResponse struct {
	Enabled            bool                            `json:"enabled"`
	ExtraResourceRules []runtimeClusterKubeGatewayRule `json:"extraResourceRules"`
	Status             string                          `json:"status"`
	ObservationCode    string                          `json:"observationCode,omitempty"`
	LastCheckedAt      *time.Time                      `json:"lastCheckedAt,omitempty"`
}

type apiKubeGatewayReadiness struct {
	handlers *Handlers
}

type apiKubectlGatewayTaskEnqueuer interface {
	EnqueueKubectlGateway(context.Context, tasks.KubectlGatewayPayload) (*asynq.TaskInfo, error)
}

var errKubeGatewayEnqueue = errors.New("kubernetes gateway reconciliation enqueue failed")

func (h *Handlers) enqueueKubectlGateway(ctx context.Context, clusterID string) error {
	taskClient, ok := h.taskClient.(apiKubectlGatewayTaskEnqueuer)
	if !ok || taskClient == nil {
		return errKubeGatewayEnqueue
	}
	if _, err := taskClient.EnqueueKubectlGateway(ctx, tasks.KubectlGatewayPayload{ClusterID: strings.TrimSpace(clusterID)}); err != nil {
		return errors.Join(errKubeGatewayEnqueue, err)
	}
	return nil
}

func writeKubeGatewayEnqueueError(ctx *gin.Context) {
	writeErrorCode(ctx, http.StatusServiceUnavailable, "kube_gateway.enqueue_failed", "kubernetes gateway reconciliation could not be queued")
}

// persistRuntimeClusterKubeGatewayDesired commits the desired configuration
// before queueing reconciliation so a fast worker can never observe the old
// state. Queue failures leave the committed state pending for the periodic
// sweep and are surfaced to the caller as a retryable 503.
func (h *Handlers) persistRuntimeClusterKubeGatewayDesired(ctx context.Context, clusterID string, enabled bool, encodedRules string) error {
	if err := h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.RuntimeCluster{}).Where("id = ?", strings.TrimSpace(clusterID)).Updates(map[string]any{
			"kube_gateway_enabled": enabled, "kube_gateway_extra_resource_rules": encodedRules,
		}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return h.enqueueKubectlGateway(ctx, clusterID)
}

// enqueueEnabledProjectAccessKubeGateways runs after the membership or project
// transaction commits. Reconciliation therefore observes the new authoritative
// state; an enqueue failure is surfaced as 503 and the periodic sweep retries
// the still-committed desired state.
func (h *Handlers) enqueueEnabledProjectAccessKubeGateways(ctx context.Context, userID string, includeGlobal bool) error {
	userID = strings.TrimSpace(userID)
	if userID == "" && !includeGlobal {
		return nil
	}
	var clusterIDs []string
	query := runtimecluster.ActiveScope(h.dbWithContext(ctx).Model(&model.RuntimeCluster{})).
		Where("kube_gateway_enabled = ? and type in ?", true, []string{"kubernetes", "k3s"})
	if includeGlobal {
		query = query.Where("scope = ? or (scope = ? and owner_ref = ?)", "global", "user", userID)
	} else {
		query = query.Where("scope = ? and owner_ref = ?", "user", userID)
	}
	if err := query.Order("id asc").Pluck("id", &clusterIDs).Error; err != nil {
		return errors.Join(errKubeGatewayEnqueue, err)
	}
	for _, clusterID := range clusterIDs {
		if err := h.enqueueKubectlGateway(ctx, clusterID); err != nil {
			return err
		}
	}
	return nil
}

func (r apiKubeGatewayReadiness) RequireReady(ctx context.Context, cluster model.RuntimeCluster, project model.Project) error {
	if r.handlers == nil {
		return kubeaccess.ErrGatewayUnavailable
	}
	response := r.handlers.observeRuntimeClusterKubeGateway(ctx, cluster)
	switch response.Status {
	case "ready":
		return nil
	case "reconciling":
		return kubeaccess.ErrGatewayReconciling
	case "disabled":
		return kubeaccess.ErrGatewayDisabled
	default:
		return kubeaccess.ErrGatewayUnavailable
	}
}

func (h *Handlers) GetRuntimeClusterKubeGateway(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	cluster, ok := h.findManagedRuntimeCluster(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, h.observeRuntimeClusterKubeGateway(ctx.Request.Context(), cluster))
}

func (h *Handlers) UpdateRuntimeClusterKubeGateway(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if !authz.IsPlatformAdmin(user.Role) {
		writeErrorCode(ctx, http.StatusForbidden, "config.admin.required", "platform administrator is required")
		return
	}
	cluster, ok := h.findManagedRuntimeCluster(ctx)
	if !ok {
		return
	}
	var input runtimeClusterKubeGatewayInput
	if !bindJSON(ctx, &input) {
		return
	}
	if len(input.ExtraResourceRules) > 50 {
		writeErrorCode(ctx, http.StatusBadRequest, "kube_gateway.rule_invalid", "too many kubernetes gateway rules")
		return
	}
	normalized, rbacRules, err := h.validateRuntimeClusterKubeGatewayRules(ctx.Request.Context(), cluster, input.ExtraResourceRules)
	if err != nil {
		writeRuntimeClusterKubeGatewayError(ctx, err)
		return
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		writeRuntimeClusterKubeGatewayError(ctx, err)
		return
	}
	if err := h.persistRuntimeClusterKubeGatewayDesired(ctx.Request.Context(), cluster.ID, input.Enabled, string(encoded)); err != nil {
		if errors.Is(err, errKubeGatewayEnqueue) {
			auditWithSafeMetadata(h, user.ID, "runtime_cluster.kube_gateway.update", cluster.ID, false, "enqueue_failed", kubeGatewayAuditMetadata{Enabled: input.Enabled, RuleCount: len(normalized)}, ctx.Request.Context())
			writeKubeGatewayEnqueueError(ctx)
			return
		}
		writeRuntimeClusterKubeGatewayError(ctx, err)
		return
	}
	_ = rbacRules // Validation and provider spec generation share the same normalized rules.
	auditWithSafeMetadata(h, user.ID, "runtime_cluster.kube_gateway.update", cluster.ID, true, "", kubeGatewayAuditMetadata{Enabled: input.Enabled, RuleCount: len(normalized)}, ctx.Request.Context())
	ctx.JSON(http.StatusAccepted, runtimeClusterKubeGatewayResponse{
		Enabled: input.Enabled, ExtraResourceRules: normalized, Status: "reconciling",
	})
}

func (h *Handlers) findManagedRuntimeCluster(ctx *gin.Context) (model.RuntimeCluster, bool) {
	var cluster model.RuntimeCluster
	if err := h.dbFor(ctx).First(&cluster, "id = ? and type in ?", strings.TrimSpace(ctx.Param("clusterId")), []string{"kubernetes", "k3s"}).Error; err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "runtime_cluster.not_found", "runtime cluster was not found")
		return model.RuntimeCluster{}, false
	}
	if !runtimecluster.IsActive(cluster) {
		writeErrorCode(ctx, http.StatusConflict, "runtime_cluster.not_active", "runtime cluster is not active")
		return model.RuntimeCluster{}, false
	}
	return cluster, true
}

func (h *Handlers) observeRuntimeClusterKubeGateway(ctx context.Context, cluster model.RuntimeCluster) runtimeClusterKubeGatewayResponse {
	rules, _ := decodeRuntimeClusterKubeGatewayRules(cluster.KubeGatewayExtraResourceRules)
	response := runtimeClusterKubeGatewayResponse{Enabled: cluster.KubeGatewayEnabled, ExtraResourceRules: rules}
	now := time.Now().UTC()
	response.LastCheckedAt = &now
	manager, spec, err := h.runtimeClusterKubeGatewayManagerAndSpec(ctx, cluster)
	if err != nil {
		response.Status = "unavailable"
		response.ObservationCode = "kube_gateway.unavailable"
		return response
	}
	observation, err := manager.ObserveGatewayAccess(ctx, spec)
	if err != nil {
		response.Status = "unavailable"
		response.ObservationCode = "kube_gateway.unavailable"
		return response
	}
	response.Status = observation.Status
	if response.Status == "reconciling" {
		response.ObservationCode = "kube_gateway.reconciling"
	}
	return response
}

func (h *Handlers) runtimeClusterKubeGatewayManagerAndSpec(ctx context.Context, cluster model.RuntimeCluster) (*kubeprovider.KubectlGatewayManager, kubeprovider.GatewayAccessSpec, error) {
	kubeconfig := h.secrets.ResolveContext(ctx, cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		return nil, kubeprovider.GatewayAccessSpec{}, kubeaccess.ErrGatewayUnavailable
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, kubeprovider.GatewayAccessSpec{}, kubeaccess.ErrGatewayUnavailable
	}
	projects, err := h.runtimeClusterKubeGatewayProjects(ctx, cluster)
	if err != nil {
		return nil, kubeprovider.GatewayAccessSpec{}, err
	}
	rules, err := decodeRuntimeClusterKubeGatewayRules(cluster.KubeGatewayExtraResourceRules)
	if err != nil {
		return nil, kubeprovider.GatewayAccessSpec{}, err
	}
	spec := kubeprovider.GatewayAccessSpec{RuntimeClusterID: cluster.ID, Enabled: cluster.KubeGatewayEnabled}
	for _, project := range projects {
		spec.Projects = append(spec.Projects, kubeprovider.GatewayAccessProjectSpec{ProjectID: project.ID, Namespace: project.KubernetesNamespace})
	}
	spec.ExtraProjectRules = kubeGatewayRBACRules(rules)
	spec.ExtraManagedResources = kubeGatewayManagedGVRs(rules)
	return kubeprovider.NewKubectlGatewayManager(client), spec, nil
}

func (h *Handlers) runtimeClusterKubeGatewayProjects(ctx context.Context, cluster model.RuntimeCluster) ([]model.Project, error) {
	db := h.dbWithContext(ctx)
	query := db.Model(&model.Project{}).Where("delete_status = ? and kubernetes_namespace <> ''", "active")
	switch strings.ToLower(strings.TrimSpace(cluster.Scope)) {
	case "project":
		query = query.Where("id in (?)", db.Model(&model.ScopedResourceProjectBinding{}).Select("project_id").Where("resource_type = ? and resource_id = ?", scopedResourceRuntimeCluster, cluster.ID))
	case "user":
		query = query.Where("id in (?)", db.Model(&model.ProjectMember{}).Select("project_id").Where("user_id = ?", cluster.OwnerRef))
	}
	var projects []model.Project
	err := query.Order("id asc").Find(&projects).Error
	return projects, err
}

func (h *Handlers) validateRuntimeClusterKubeGatewayRules(ctx context.Context, cluster model.RuntimeCluster, input []runtimeClusterKubeGatewayRule) ([]runtimeClusterKubeGatewayRule, []rbacv1.PolicyRule, error) {
	if len(input) == 0 {
		return []runtimeClusterKubeGatewayRule{}, nil, nil
	}
	kubeconfig := h.secrets.ResolveContext(ctx, cluster.KubeconfigRef)
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, nil, kubeaccess.ErrGatewayUnavailable
	}
	normalized := normalizeRuntimeClusterKubeGatewayRules(input)
	extra := make([]kubecatalog.ExtraResourceRule, 0, len(normalized))
	for _, rule := range normalized {
		extra = append(extra, kubecatalog.ExtraResourceRule{
			APIGroup: rule.APIGroup, APIVersion: rule.APIVersion, Resource: rule.Resource,
			Subresources: rule.Subresources, Verbs: rule.Verbs, Action: authz.Action(rule.Action),
		})
	}
	if _, err := kubecatalog.NewWithExtra(ctx, kubeprovider.NewDiscoveryScopeResolver(client), extra); err != nil {
		return nil, nil, err
	}
	return normalized, kubeGatewayRBACRules(normalized), nil
}

func normalizeRuntimeClusterKubeGatewayRules(input []runtimeClusterKubeGatewayRule) []runtimeClusterKubeGatewayRule {
	result := append([]runtimeClusterKubeGatewayRule(nil), input...)
	for index := range result {
		rule := &result[index]
		rule.APIGroup = strings.TrimSpace(rule.APIGroup)
		rule.APIVersion = strings.ToLower(strings.TrimSpace(rule.APIVersion))
		rule.Resource = strings.ToLower(strings.TrimSpace(rule.Resource))
		rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
		rule.Subresources = normalizedStrings(rule.Subresources)
		rule.Verbs = normalizedStrings(rule.Verbs)
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := json.Marshal(result[i])
		right, _ := json.Marshal(result[j])
		return string(left) < string(right)
	})
	return result
}

func normalizedStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func decodeRuntimeClusterKubeGatewayRules(raw string) ([]runtimeClusterKubeGatewayRule, error) {
	if strings.TrimSpace(raw) == "" {
		return []runtimeClusterKubeGatewayRule{}, nil
	}
	var rules []runtimeClusterKubeGatewayRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, err
	}
	return normalizeRuntimeClusterKubeGatewayRules(rules), nil
}

func kubeGatewayRBACRules(rules []runtimeClusterKubeGatewayRule) []rbacv1.PolicyRule {
	result := make([]rbacv1.PolicyRule, 0, len(rules))
	for _, rule := range rules {
		resources := []string{rule.Resource}
		for _, subresource := range rule.Subresources {
			resources = append(resources, rule.Resource+"/"+subresource)
		}
		result = append(result, rbacv1.PolicyRule{APIGroups: []string{rule.APIGroup}, Resources: resources, Verbs: append([]string(nil), rule.Verbs...)})
	}
	return result
}

func kubeGatewayManagedGVRs(rules []runtimeClusterKubeGatewayRule) []schema.GroupVersionResource {
	result := make([]schema.GroupVersionResource, 0, len(rules))
	for _, rule := range rules {
		result = append(result, schema.GroupVersionResource{
			Group: rule.APIGroup, Version: rule.APIVersion, Resource: rule.Resource,
		})
	}
	return result
}

func writeRuntimeClusterKubeGatewayError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, kubecatalog.ErrExtraRuleInvalid), errors.Is(err, kubecatalog.ErrExtraRuleClusterScoped), errors.Is(err, kubecatalog.ErrExtraRuleDenied):
		writeErrorCode(ctx, http.StatusBadRequest, "kube_gateway.rule_invalid", "kubernetes gateway rule is invalid")
	case errors.Is(err, kubeaccess.ErrGatewayUnavailable):
		writeErrorCode(ctx, http.StatusServiceUnavailable, "kube_gateway.unavailable", "kubernetes gateway is unavailable")
	case errors.Is(err, gorm.ErrRecordNotFound):
		writeErrorCode(ctx, http.StatusNotFound, "runtime_cluster.not_found", "runtime cluster was not found")
	default:
		writeErrorCode(ctx, http.StatusInternalServerError, "kube_gateway.internal_error", "kubernetes gateway operation failed")
	}
}
