package platformapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
)

const (
	SearchTypeProject             = "project"
	SearchTypeApplication         = "application"
	SearchTypeDeploymentTarget    = "deployment_target"
	SearchTypeGatewayRoute        = "gateway_route"
	SearchTypeRepositoryBinding   = "repository_binding"
	SearchTypeProjectVolume       = "project_volume"
	SearchTypeRuntimeConfigSet    = "runtime_config_set"
	SearchTypeBuildVariableSet    = "build_variable_set"
	SearchTypeNotificationChannel = "notification_channel"
	SearchTypeRuntimeCluster      = "runtime_cluster"
	SearchTypeArtifactRegistry    = "artifact_registry"
	SearchTypeBuildRun            = "build_run"
	SearchTypeRelease             = "release"
	SearchTypeUser                = "user"

	defaultSearchLimitPerType = 6
	maxSearchLimitPerType     = 20
	maxSearchQueryRunes       = 160
)

var allSearchResourceTypes = []string{
	SearchTypeApplication,
	SearchTypeGatewayRoute,
	SearchTypeDeploymentTarget,
	SearchTypeProject,
	SearchTypeRepositoryBinding,
	SearchTypeProjectVolume,
	SearchTypeRuntimeConfigSet,
	SearchTypeBuildVariableSet,
	SearchTypeNotificationChannel,
	SearchTypeRuntimeCluster,
	SearchTypeArtifactRegistry,
	SearchTypeBuildRun,
	SearchTypeRelease,
	SearchTypeUser,
}

var defaultKeywordSearchTypes = []string{
	SearchTypeApplication,
	SearchTypeGatewayRoute,
	SearchTypeDeploymentTarget,
	SearchTypeProject,
	SearchTypeRepositoryBinding,
	SearchTypeProjectVolume,
	SearchTypeRuntimeConfigSet,
	SearchTypeBuildVariableSet,
	SearchTypeNotificationChannel,
	SearchTypeRuntimeCluster,
	SearchTypeArtifactRegistry,
	SearchTypeUser,
}

var allowedSearchTypeSet = func() map[string]int {
	out := make(map[string]int, len(allSearchResourceTypes))
	for index, itemType := range allSearchResourceTypes {
		out[itemType] = index + 1
	}
	return out
}()

var inlineSearchPrefixTypes = map[string][]string{
	"proj":     {SearchTypeProject},
	"project":  {SearchTypeProject},
	"space":    {SearchTypeProject},
	"app":      {SearchTypeApplication},
	"apps":     {SearchTypeApplication},
	"host":     {SearchTypeGatewayRoute},
	"domain":   {SearchTypeGatewayRoute},
	"route":    {SearchTypeGatewayRoute},
	"gateway":  {SearchTypeGatewayRoute},
	"deploy":   {SearchTypeDeploymentTarget},
	"target":   {SearchTypeDeploymentTarget},
	"pod":      {SearchTypeDeploymentTarget},
	"image":    {SearchTypeDeploymentTarget},
	"repo":     {SearchTypeRepositoryBinding},
	"git":      {SearchTypeRepositoryBinding},
	"vol":      {SearchTypeProjectVolume},
	"volume":   {SearchTypeProjectVolume},
	"pvc":      {SearchTypeProjectVolume},
	"config":   {SearchTypeRuntimeConfigSet, SearchTypeBuildVariableSet},
	"env":      {SearchTypeRuntimeConfigSet, SearchTypeBuildVariableSet},
	"notify":   {SearchTypeNotificationChannel},
	"channel":  {SearchTypeNotificationChannel},
	"cluster":  {SearchTypeRuntimeCluster},
	"k8s":      {SearchTypeRuntimeCluster},
	"registry": {SearchTypeArtifactRegistry},
	"harbor":   {SearchTypeArtifactRegistry},
	"build":    {SearchTypeBuildRun},
	"release":  {SearchTypeRelease},
	"user":     {SearchTypeUser},
	"member":   {SearchTypeUser},
}

var resourceIDPrefixToSearchType = []struct {
	prefix   string
	itemType string
}{
	{prefix: "prj_", itemType: SearchTypeProject},
	{prefix: "app_", itemType: SearchTypeApplication},
	{prefix: "dplt_", itemType: SearchTypeDeploymentTarget},
	{prefix: "gwr_", itemType: SearchTypeGatewayRoute},
	{prefix: "rpb_", itemType: SearchTypeRepositoryBinding},
	{prefix: "rbnd_", itemType: SearchTypeRepositoryBinding},
	{prefix: "pvol_", itemType: SearchTypeProjectVolume},
	{prefix: "prcs_", itemType: SearchTypeRuntimeConfigSet},
	{prefix: "bvs_", itemType: SearchTypeBuildVariableSet},
	{prefix: "nch_", itemType: SearchTypeNotificationChannel},
	{prefix: "clu_", itemType: SearchTypeRuntimeCluster},
	{prefix: "rcl_", itemType: SearchTypeRuntimeCluster},
	{prefix: "reg_", itemType: SearchTypeArtifactRegistry},
	{prefix: "bldr_", itemType: SearchTypeBuildRun},
	{prefix: "bld_", itemType: SearchTypeBuildRun},
	{prefix: "rel_", itemType: SearchTypeRelease},
	{prefix: "usr_", itemType: SearchTypeUser},
}

type SearchResultItem struct {
	Type              string    `json:"type"`
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Subtitle          string    `json:"subtitle,omitempty"`
	Identifier        string    `json:"identifier,omitempty"`
	Scope             string    `json:"scope,omitempty"`
	Status            string    `json:"status,omitempty"`
	Related           bool      `json:"related"`
	ProjectID         string    `json:"projectId,omitempty"`
	ProjectName       string    `json:"projectName,omitempty"`
	ProjectIdentifier string    `json:"projectIdentifier,omitempty"`
	ApplicationID     string    `json:"applicationId,omitempty"`
	ApplicationName   string    `json:"applicationName,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type SearchResponse struct {
	Query      string             `json:"query"`
	Visibility string             `json:"visibility"`
	Items      []SearchResultItem `json:"items"`
}

type parsedSearchQuery struct {
	raw           string
	term          string
	lowerTerm     string
	escapedLower  string
	urlHostLower  string
	inlineTypes   []string
	idMatchedType string
	commitSHA     bool
	emptyTerm     bool
}

type searchExecutionScope struct {
	user                   model.User
	platformAdmin          bool
	allProjects            bool
	conversationProjectID  string
	relatedProjectIDs      []string
	notificationProjectIDs []string
	limitPerType           int
}

type searchSQLBranch struct {
	sql  string
	args []any
}

type searchSQLRow struct {
	ItemType          string    `gorm:"column:item_type"`
	ID                string    `gorm:"column:id"`
	Title             string    `gorm:"column:title"`
	Subtitle          string    `gorm:"column:subtitle"`
	Identifier        string    `gorm:"column:identifier"`
	Scope             string    `gorm:"column:scope"`
	Status            string    `gorm:"column:status"`
	IsRelated         bool      `gorm:"column:is_related"`
	ProjectID         string    `gorm:"column:project_id"`
	ProjectName       string    `gorm:"column:project_name"`
	ProjectIdentifier string    `gorm:"column:project_identifier"`
	ApplicationID     string    `gorm:"column:application_id"`
	ApplicationName   string    `gorm:"column:application_name"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
	MatchRank         int       `gorm:"column:match_rank"`
	KindOrder         int       `gorm:"column:kind_order"`
}

func (h *Handlers) SearchGlobal(ctx *gin.Context) {
	transportapi.MarkLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	visibility, ok := h.resolveListVisibility(ctx, user)
	if !ok {
		return
	}

	rawQuery := strings.TrimSpace(ctx.Query("q"))
	if rawQuery == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "search.query_required", "q is required")
		return
	}
	if utf8.RuneCountInString(rawQuery) > maxSearchQueryRunes {
		writeErrorCode(ctx, http.StatusBadRequest, "search.query_too_long", "q must be at most 160 characters")
		return
	}

	explicitTypes, ok := parseRequestedSearchTypes(ctx)
	if !ok {
		return
	}
	limitPerType, ok := parseSearchLimitPerType(ctx)
	if !ok {
		return
	}

	parsed := parseSearchQuery(rawQuery)
	effectiveVisibility := visibility
	conversationProjectID := strings.TrimSpace(h.aiConversationProjectID(ctx))
	if conversationProjectID != "" {
		effectiveVisibility = projectservice.ListVisibilityRelated
	}

	if parsed.emptyTerm {
		ctx.JSON(http.StatusOK, SearchResponse{
			Query:      rawQuery,
			Visibility: string(effectiveVisibility),
			Items:      []SearchResultItem{},
		})
		return
	}

	execScope, err := h.resolveSearchExecutionScope(ctx, user, effectiveVisibility, conversationProjectID, limitPerType)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "search.load_failed", err.Error())
		return
	}

	selectedTypes := resolveCandidateSearchTypes(parsed, explicitTypes)
	branches := make([]searchSQLBranch, 0, len(selectedTypes))
	for _, itemType := range selectedTypes {
		if !h.searchTypeAllowedByRequestScope(ctx, execScope, itemType) {
			continue
		}
		branch, include := buildSearchBranch(itemType, parsed, execScope)
		if include {
			branches = append(branches, branch)
		}
	}

	if len(branches) == 0 {
		ctx.JSON(http.StatusOK, SearchResponse{
			Query:      rawQuery,
			Visibility: string(effectiveVisibility),
			Items:      []SearchResultItem{},
		})
		return
	}

	unionParts := make([]string, 0, len(branches))
	unionArgs := make([]any, 0, len(branches)*8)
	for index, branch := range branches {
		unionParts = append(unionParts, fmt.Sprintf("SELECT * FROM (%s) AS search_branch_%d", branch.sql, index))
		unionArgs = append(unionArgs, branch.args...)
	}
	finalSQL := fmt.Sprintf(
		"SELECT * FROM (%s) AS search_union ORDER BY is_related DESC, match_rank ASC, kind_order ASC, updated_at DESC, id ASC",
		strings.Join(unionParts, " UNION ALL "),
	)

	var rows []searchSQLRow
	if err := h.dbFor(ctx).Raw(finalSQL, unionArgs...).Scan(&rows).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "search.load_failed", err.Error())
		return
	}

	items := make([]SearchResultItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, SearchResultItem{
			Type:              row.ItemType,
			ID:                row.ID,
			Title:             row.Title,
			Subtitle:          row.Subtitle,
			Identifier:        row.Identifier,
			Scope:             row.Scope,
			Status:            row.Status,
			Related:           row.IsRelated,
			ProjectID:         row.ProjectID,
			ProjectName:       row.ProjectName,
			ProjectIdentifier: row.ProjectIdentifier,
			ApplicationID:     row.ApplicationID,
			ApplicationName:   row.ApplicationName,
			UpdatedAt:         row.UpdatedAt,
		})
	}

	ctx.JSON(http.StatusOK, SearchResponse{
		Query:      rawQuery,
		Visibility: string(effectiveVisibility),
		Items:      items,
	})
}

func parseRequestedSearchTypes(ctx *gin.Context) ([]string, bool) {
	rawValues := ctx.QueryArray("types")
	if len(rawValues) == 0 {
		return nil, true
	}
	seen := make(map[string]bool, len(rawValues))
	out := make([]string, 0, len(rawValues))
	for _, rawValue := range rawValues {
		for _, part := range strings.Split(rawValue, ",") {
			normalized := strings.ToLower(strings.TrimSpace(part))
			if normalized == "" {
				continue
			}
			if _, valid := allowedSearchTypeSet[normalized]; !valid {
				writeErrorCode(ctx, http.StatusBadRequest, "search.invalid_type", fmt.Sprintf("unsupported search type %q", normalized))
				return nil, false
			}
			if !seen[normalized] {
				seen[normalized] = true
				out = append(out, normalized)
			}
		}
	}
	return out, true
}

func parseSearchLimitPerType(ctx *gin.Context) (int, bool) {
	raw := strings.TrimSpace(ctx.Query("limitPerType"))
	if raw == "" {
		return defaultSearchLimitPerType, true
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 || parsed > maxSearchLimitPerType {
		writeErrorCode(ctx, http.StatusBadRequest, "search.invalid_limit", "limitPerType must be an integer between 1 and 20")
		return 0, false
	}
	return parsed, true
}

func parseSearchQuery(raw string) parsedSearchQuery {
	trimmed := strings.TrimSpace(raw)
	result := parsedSearchQuery{
		raw:  trimmed,
		term: trimmed,
	}

	if prefix, remainder, hasColon := strings.Cut(trimmed, ":"); hasColon && !strings.HasPrefix(remainder, "//") {
		if inlineTypes, ok := inlineSearchPrefixTypes[strings.ToLower(strings.TrimSpace(prefix))]; ok {
			result.inlineTypes = inlineTypes
			result.term = strings.TrimSpace(remainder)
			if result.term == "" {
				result.emptyTerm = true
				return result
			}
		}
	}

	result.lowerTerm = strings.ToLower(result.term)
	result.escapedLower = escapeSQLLikeLiteral(result.lowerTerm)
	result.urlHostLower = extractURLHostLower(result.term)

	for _, candidate := range resourceIDPrefixToSearchType {
		if strings.HasPrefix(result.lowerTerm, candidate.prefix) && len(result.lowerTerm) > len(candidate.prefix) {
			result.idMatchedType = candidate.itemType
			break
		}
	}

	result.commitSHA = isCommitSHACandidate(result.lowerTerm)
	return result
}

func escapeSQLLikeLiteral(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(value)
}

func extractURLHostLower(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func isCommitSHACandidate(lower string) bool {
	if len(lower) < 7 || len(lower) > 40 {
		return false
	}
	for index := 0; index < len(lower); index++ {
		ch := lower[index]
		isHex := (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')
		if !isHex {
			return false
		}
	}
	return true
}

func resolveCandidateSearchTypes(parsed parsedSearchQuery, explicitTypes []string) []string {
	var base []string
	switch {
	case parsed.idMatchedType != "":
		base = []string{parsed.idMatchedType}
	case len(parsed.inlineTypes) > 0:
		base = append([]string(nil), parsed.inlineTypes...)
	case len(explicitTypes) > 0:
		base = append([]string(nil), explicitTypes...)
	default:
		base = append([]string(nil), defaultKeywordSearchTypes...)
		if parsed.commitSHA {
			base = append(base, SearchTypeBuildRun)
		}
	}

	if len(parsed.inlineTypes) > 0 && parsed.idMatchedType != "" {
		base = intersectSearchTypes(base, parsed.inlineTypes)
	}
	if len(explicitTypes) > 0 {
		base = intersectSearchTypes(base, explicitTypes)
	}
	return base
}

func intersectSearchTypes(primary, filter []string) []string {
	allowed := make(map[string]bool, len(filter))
	for _, item := range filter {
		allowed[item] = true
	}
	out := make([]string, 0, len(primary))
	for _, item := range primary {
		if allowed[item] {
			out = append(out, item)
		}
	}
	return out
}

func (h *Handlers) resolveSearchExecutionScope(
	ctx *gin.Context,
	user model.User,
	visibility projectservice.ListVisibility,
	conversationProjectID string,
	limitPerType int,
) (searchExecutionScope, error) {
	platformAdmin := authz.IsPlatformAdmin(user.Role)
	scope := searchExecutionScope{
		user:                  user,
		platformAdmin:         platformAdmin,
		allProjects:           visibility == projectservice.ListVisibilityAll && conversationProjectID == "",
		conversationProjectID: conversationProjectID,
		limitPerType:          limitPerType,
	}

	type memberRow struct {
		ProjectID string
		Role      string
	}
	var memberships []memberRow
	if err := h.dbFor(ctx).
		Model(&model.ProjectMember{}).
		Select("project_id, role").
		Where("user_id = ?", user.ID).
		Find(&memberships).Error; err != nil {
		return scope, err
	}

	memberRoles := make(map[string]string, len(memberships))
	for _, row := range memberships {
		projectID := strings.TrimSpace(row.ProjectID)
		if projectID == "" {
			continue
		}
		memberRoles[projectID] = strings.TrimSpace(row.Role)
	}

	if conversationProjectID != "" {
		role, isMember := memberRoles[conversationProjectID]
		if platformAdmin {
			var count int64
			if err := h.dbFor(ctx).
				Model(&model.Project{}).
				Where("id = ? AND deleted_at IS NULL AND delete_status IN ?", conversationProjectID, model.ActiveDeleteStatuses).
				Count(&count).Error; err != nil {
				return scope, err
			}
			if count > 0 {
				scope.relatedProjectIDs = []string{conversationProjectID}
				scope.notificationProjectIDs = []string{conversationProjectID}
			}
			return scope, nil
		}
		if isMember && authz.ProjectRoleAllows(role, authz.ActionProjectRead) {
			scope.relatedProjectIDs = []string{conversationProjectID}
			if authz.ProjectRoleAllows(role, authz.ActionNotificationRead) {
				scope.notificationProjectIDs = []string{conversationProjectID}
			}
		}
		return scope, nil
	}

	related := make([]string, 0, len(memberRoles))
	notifReadable := make([]string, 0, len(memberRoles))
	for projectID, role := range memberRoles {
		if authz.ProjectRoleAllows(role, authz.ActionProjectRead) {
			related = append(related, projectID)
		}
		if authz.ProjectRoleAllows(role, authz.ActionNotificationRead) {
			notifReadable = append(notifReadable, projectID)
		}
	}
	scope.relatedProjectIDs = related
	scope.notificationProjectIDs = notifReadable
	return scope, nil
}

func (h *Handlers) searchTypeAllowedByRequestScope(ctx *gin.Context, execScope searchExecutionScope, itemType string) bool {
	switch itemType {
	case SearchTypeProject:
		return h.requestAllowsScope(ctx, string(authz.ActionProjectRead))
	case SearchTypeApplication:
		return h.requestAllowsScope(ctx, string(authz.ActionApplicationRead))
	case SearchTypeDeploymentTarget:
		return h.requestAllowsScope(ctx, string(authz.ActionDeploymentRead))
	case SearchTypeGatewayRoute:
		return h.requestAllowsScope(ctx, string(authz.ActionGatewayRead))
	case SearchTypeRepositoryBinding:
		return h.requestAllowsScope(ctx, string(authz.ActionGitRead))
	case SearchTypeProjectVolume:
		return h.requestAllowsScope(ctx, string(authz.ActionVolumeRead))
	case SearchTypeRuntimeConfigSet, SearchTypeBuildVariableSet:
		return h.requestAllowsScope(ctx, string(authz.ActionSecretReadSummary))
	case SearchTypeNotificationChannel:
		return h.requestAllowsScope(ctx, string(authz.ActionNotificationRead))
	case SearchTypeRuntimeCluster:
		return h.requestAllowsScope(ctx, string(authz.ActionClusterRead)) || h.requestAllowsScope(ctx, string(authz.ActionClusterManage))
	case SearchTypeArtifactRegistry:
		return h.requestAllowsScope(ctx, string(authz.ActionRegistryRead))
	case SearchTypeBuildRun:
		return h.requestAllowsScope(ctx, string(authz.ActionBuildRead))
	case SearchTypeRelease:
		return h.requestAllowsScope(ctx, string(authz.ActionDeploymentRead))
	case SearchTypeUser:
		if !execScope.platformAdmin || execScope.conversationProjectID != "" {
			return false
		}
		return h.requestAllowsScope(ctx, string(authz.ActionUserRead)) || h.requestAllowsScope(ctx, string(authz.ActionUserManage))
	default:
		return false
	}
}

func projectVisibilityClause(column string, execScope searchExecutionScope) (whereSQL string, relatedExpr string, argsWhere []any, argsRelated []any, ok bool) {
	if execScope.allProjects {
		if len(execScope.relatedProjectIDs) == 0 {
			return "1 = 1", "FALSE", nil, nil, true
		}
		return "1 = 1", fmt.Sprintf("(%s IN ?)", column), nil, []any{execScope.relatedProjectIDs}, true
	}
	if len(execScope.relatedProjectIDs) == 0 {
		return "", "", nil, nil, false
	}
	return fmt.Sprintf("%s IN ?", column), "TRUE", []any{execScope.relatedProjectIDs}, nil, true
}

func buildSearchBranch(itemType string, parsed parsedSearchQuery, execScope searchExecutionScope) (searchSQLBranch, bool) {
	kindOrder := allowedSearchTypeSet[itemType]
	exact := parsed.lowerTerm
	prefix := parsed.escapedLower + "%"
	contains := "%" + parsed.escapedLower + "%"

	switch itemType {
	case SearchTypeProject:
		projWhere, relatedExpr, whereArgs, relatedArgs, ok := projectVisibilityClause("p.id", execScope)
		if !ok {
			return searchSQLBranch{}, false
		}
		args := make([]any, 0, 12)
		args = append(args, relatedArgs...)
		args = append(args, exact, exact, exact, prefix, prefix, prefix)
		args = append(args, model.ActiveDeleteStatuses)
		args = append(args, whereArgs...)
		if parsed.idMatchedType == SearchTypeProject {
			sql := fmt.Sprintf(`
				SELECT
					CAST('project' AS TEXT) AS item_type,
					p.id AS id,
					p.name AS title,
					p.kubernetes_namespace AS subtitle,
					p.identifier AS identifier,
					CAST('' AS TEXT) AS scope,
					CAST('' AS TEXT) AS status,
					%s AS is_related,
					p.id AS project_id,
					p.name AS project_name,
					p.identifier AS project_identifier,
					CAST('' AS TEXT) AS application_id,
					CAST('' AS TEXT) AS application_name,
					p.updated_at AS updated_at,
					CASE
						WHEN LOWER(p.id) = ? OR LOWER(p.name) = ? OR LOWER(p.identifier) = ? THEN 0
						WHEN LOWER(p.id) LIKE ? ESCAPE '\' OR LOWER(p.name) LIKE ? ESCAPE '\' OR LOWER(p.identifier) LIKE ? ESCAPE '\' THEN 1
						ELSE 2
					END AS match_rank,
					%d AS kind_order
				FROM projects p
				WHERE p.deleted_at IS NULL AND p.delete_status IN ? AND %s
					AND LOWER(p.id) LIKE ? ESCAPE '\'
				ORDER BY is_related DESC, match_rank ASC, p.updated_at DESC
				LIMIT ?`, relatedExpr, kindOrder, projWhere)
			args = append(args, prefix, execScope.limitPerType)
			return searchSQLBranch{sql: sql, args: args}, true
		}
		sql := fmt.Sprintf(`
			SELECT
				CAST('project' AS TEXT) AS item_type,
				p.id AS id,
				p.name AS title,
				p.kubernetes_namespace AS subtitle,
				p.identifier AS identifier,
				CAST('' AS TEXT) AS scope,
				CAST('' AS TEXT) AS status,
				%s AS is_related,
				p.id AS project_id,
				p.name AS project_name,
				p.identifier AS project_identifier,
				CAST('' AS TEXT) AS application_id,
				CAST('' AS TEXT) AS application_name,
				p.updated_at AS updated_at,
				CASE
					WHEN LOWER(p.id) = ? OR LOWER(p.name) = ? OR LOWER(p.identifier) = ? THEN 0
					WHEN LOWER(p.id) LIKE ? ESCAPE '\' OR LOWER(p.name) LIKE ? ESCAPE '\' OR LOWER(p.identifier) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM projects p
			WHERE p.deleted_at IS NULL AND p.delete_status IN ? AND %s
				AND (
					LOWER(p.name) LIKE ? ESCAPE '\'
					OR LOWER(p.identifier) LIKE ? ESCAPE '\'
					OR LOWER(p.kubernetes_namespace) LIKE ? ESCAPE '\'
					OR LOWER(p.id) LIKE ? ESCAPE '\'
				)
			ORDER BY is_related DESC, match_rank ASC, p.updated_at DESC
			LIMIT ?`, relatedExpr, kindOrder, projWhere)
		args = append(args, contains, contains, contains, prefix, execScope.limitPerType)
		return searchSQLBranch{sql: sql, args: args}, true

	case SearchTypeApplication:
		projWhere, relatedExpr, whereArgs, relatedArgs, ok := projectVisibilityClause("p.id", execScope)
		if !ok {
			return searchSQLBranch{}, false
		}
		args := make([]any, 0, 14)
		args = append(args, relatedArgs...)
		args = append(args, exact, exact, exact, prefix, prefix, prefix)
		args = append(args, model.ActiveDeleteStatuses, model.ActiveDeleteStatuses)
		args = append(args, whereArgs...)
		matchFilter := `(LOWER(a.name) LIKE ? ESCAPE '\' OR LOWER(a.identifier) LIKE ? ESCAPE '\' OR LOWER(a.id) LIKE ? ESCAPE '\')`
		matchArgs := []any{contains, contains, prefix}
		if parsed.idMatchedType == SearchTypeApplication {
			matchFilter = `LOWER(a.id) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		}
		sql := fmt.Sprintf(`
			SELECT
				CAST('application' AS TEXT) AS item_type,
				a.id AS id,
				a.name AS title,
				CAST('' AS TEXT) AS subtitle,
				a.identifier AS identifier,
				CAST('' AS TEXT) AS scope,
				CAST('' AS TEXT) AS status,
				%s AS is_related,
				p.id AS project_id,
				p.name AS project_name,
				p.identifier AS project_identifier,
				a.id AS application_id,
				a.name AS application_name,
				a.updated_at AS updated_at,
				CASE
					WHEN LOWER(a.id) = ? OR LOWER(a.name) = ? OR LOWER(a.identifier) = ? THEN 0
					WHEN LOWER(a.id) LIKE ? ESCAPE '\' OR LOWER(a.name) LIKE ? ESCAPE '\' OR LOWER(a.identifier) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM applications a
			JOIN projects p ON p.id = a.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE a.deleted_at IS NULL AND a.delete_status IN ? AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, a.updated_at DESC
			LIMIT ?`, relatedExpr, kindOrder, projWhere, matchFilter)
		args = append(args, matchArgs...)
		args = append(args, execScope.limitPerType)
		return searchSQLBranch{sql: sql, args: args}, true

	case SearchTypeDeploymentTarget:
		projWhere, relatedExpr, whereArgs, relatedArgs, ok := projectVisibilityClause("p.id", execScope)
		if !ok {
			return searchSQLBranch{}, false
		}
		args := make([]any, 0, 18)
		args = append(args, relatedArgs...)
		args = append(args, exact, exact, exact, exact, prefix, prefix, prefix)
		args = append(args, model.ActiveDeleteStatuses, model.ActiveDeleteStatuses, model.ActiveDeleteStatuses)
		args = append(args, whereArgs...)
		matchFilter := `(
			LOWER(dt.name) LIKE ? ESCAPE '\'
			OR LOWER(dt.stage) LIKE ? ESCAPE '\'
			OR LOWER(dt.kubernetes_name) LIKE ? ESCAPE '\'
			OR LOWER(dt.image_ref) LIKE ? ESCAPE '\'
			OR LOWER(dt.id) LIKE ? ESCAPE '\'
		)`
		matchArgs := []any{contains, contains, contains, contains, prefix}
		if parsed.idMatchedType == SearchTypeDeploymentTarget {
			matchFilter = `LOWER(dt.id) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		}
		sql := fmt.Sprintf(`
			SELECT
				CAST('deployment_target' AS TEXT) AS item_type,
				dt.id AS id,
				dt.name AS title,
				CASE WHEN dt.image_ref <> '' THEN dt.image_ref ELSE dt.kubernetes_name END AS subtitle,
				dt.stage AS identifier,
				dt.workload_type AS scope,
				CAST('' AS TEXT) AS status,
				%s AS is_related,
				p.id AS project_id,
				p.name AS project_name,
				p.identifier AS project_identifier,
				a.id AS application_id,
				a.name AS application_name,
				dt.updated_at AS updated_at,
				CASE
					WHEN LOWER(dt.id) = ? OR LOWER(dt.name) = ? OR LOWER(dt.kubernetes_name) = ? OR LOWER(dt.stage) = ? THEN 0
					WHEN LOWER(dt.id) LIKE ? ESCAPE '\' OR LOWER(dt.name) LIKE ? ESCAPE '\' OR LOWER(dt.kubernetes_name) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM deployment_targets dt
			JOIN applications a ON a.id = dt.application_id AND a.deleted_at IS NULL AND a.delete_status IN ?
			JOIN projects p ON p.id = dt.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE dt.deleted_at IS NULL AND dt.delete_status IN ? AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, dt.updated_at DESC
			LIMIT ?`, relatedExpr, kindOrder, projWhere, matchFilter)
		args = append(args, matchArgs...)
		args = append(args, execScope.limitPerType)
		return searchSQLBranch{sql: sql, args: args}, true

	case SearchTypeGatewayRoute:
		projWhere, relatedExpr, whereArgs, relatedArgs, ok := projectVisibilityClause("p.id", execScope)
		if !ok {
			return searchSQLBranch{}, false
		}
		hostExact := exact
		if parsed.urlHostLower != "" {
			hostExact = parsed.urlHostLower
		}
		args := make([]any, 0, 18)
		args = append(args, relatedArgs...)
		args = append(args, exact, hostExact, prefix, escapeSQLLikeLiteral(hostExact)+"%")
		args = append(args, model.ActiveDeleteStatuses, model.ActiveDeleteStatuses, model.ActiveDeleteStatuses)
		args = append(args, whereArgs...)

		matchFilter := `(
			LOWER(gr.host) LIKE ? ESCAPE '\'
			OR LOWER(gr.path) LIKE ? ESCAPE '\'
			OR LOWER(gr.hostname_aliases) LIKE ? ESCAPE '\'
			OR LOWER(gr.id) LIKE ? ESCAPE '\'
		)`
		matchArgs := []any{contains, contains, contains, prefix}
		if parsed.urlHostLower != "" {
			urlHostContains := "%" + escapeSQLLikeLiteral(parsed.urlHostLower) + "%"
			matchFilter = `(
				LOWER(gr.host) = ?
				OR LOWER(gr.host) LIKE ? ESCAPE '\'
				OR LOWER(gr.hostname_aliases) LIKE ? ESCAPE '\'
			)`
			matchArgs = []any{parsed.urlHostLower, urlHostContains, urlHostContains}
		} else if parsed.idMatchedType == SearchTypeGatewayRoute {
			matchFilter = `LOWER(gr.id) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		}

		sql := fmt.Sprintf(`
			SELECT
				CAST('gateway_route' AS TEXT) AS item_type,
				gr.id AS id,
				CASE WHEN gr.path <> '' AND gr.path <> '/' THEN gr.host || gr.path ELSE gr.host END AS title,
				gr.tls_mode AS subtitle,
				gr.path AS identifier,
				CAST('' AS TEXT) AS scope,
				CASE WHEN gr.enabled THEN CAST('enabled' AS TEXT) ELSE CAST('disabled' AS TEXT) END AS status,
				%s AS is_related,
				p.id AS project_id,
				p.name AS project_name,
				p.identifier AS project_identifier,
				a.id AS application_id,
				a.name AS application_name,
				gr.updated_at AS updated_at,
				CASE
					WHEN LOWER(gr.id) = ? OR LOWER(gr.host) = ? THEN 0
					WHEN LOWER(gr.id) LIKE ? ESCAPE '\' OR LOWER(gr.host) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM gateway_routes gr
			JOIN applications a ON a.id = gr.application_id AND a.deleted_at IS NULL AND a.delete_status IN ?
			JOIN projects p ON p.id = gr.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE gr.deleted_at IS NULL AND gr.delete_status IN ? AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, gr.updated_at DESC
			LIMIT ?`, relatedExpr, kindOrder, projWhere, matchFilter)
		args = append(args, matchArgs...)
		args = append(args, execScope.limitPerType)
		return searchSQLBranch{sql: sql, args: args}, true

	case SearchTypeRepositoryBinding:
		projWhere, relatedExpr, whereArgs, relatedArgs, ok := projectVisibilityClause("p.id", execScope)
		if !ok {
			return searchSQLBranch{}, false
		}
		args := make([]any, 0, 16)
		args = append(args, relatedArgs...)
		args = append(args, exact, exact, exact, prefix, prefix)
		args = append(args, model.ActiveDeleteStatuses, model.ActiveDeleteStatuses)
		args = append(args, whereArgs...)
		matchFilter := `(
			LOWER(rb.owner || '/' || rb.repo) LIKE ? ESCAPE '\'
			OR LOWER(rb.clone_url) LIKE ? ESCAPE '\'
			OR LOWER(rb.id) LIKE ? ESCAPE '\'
		)`
		matchArgs := []any{contains, contains, prefix}
		if parsed.idMatchedType == SearchTypeRepositoryBinding {
			matchFilter = `LOWER(rb.id) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		}
		sql := fmt.Sprintf(`
			SELECT
				CAST('repository_binding' AS TEXT) AS item_type,
				rb.id AS id,
				rb.owner || '/' || rb.repo AS title,
				rb.clone_url AS subtitle,
				rb.default_branch AS identifier,
				CAST('' AS TEXT) AS scope,
				CAST('' AS TEXT) AS status,
				%s AS is_related,
				p.id AS project_id,
				p.name AS project_name,
				p.identifier AS project_identifier,
				a.id AS application_id,
				a.name AS application_name,
				rb.updated_at AS updated_at,
				CASE
					WHEN LOWER(rb.id) = ? OR LOWER(rb.repo) = ? OR LOWER(rb.owner || '/' || rb.repo) = ? THEN 0
					WHEN LOWER(rb.repo) LIKE ? ESCAPE '\' OR LOWER(rb.owner || '/' || rb.repo) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM repository_bindings rb
			JOIN applications a ON a.id = rb.application_id AND a.deleted_at IS NULL AND a.delete_status IN ?
			JOIN projects p ON p.id = rb.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE rb.deleted_at IS NULL AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, rb.updated_at DESC
			LIMIT ?`, relatedExpr, kindOrder, projWhere, matchFilter)
		args = append(args, matchArgs...)
		args = append(args, execScope.limitPerType)
		return searchSQLBranch{sql: sql, args: args}, true

	case SearchTypeProjectVolume:
		projWhere, relatedExpr, whereArgs, relatedArgs, ok := projectVisibilityClause("p.id", execScope)
		if !ok {
			return searchSQLBranch{}, false
		}
		args := make([]any, 0, 16)
		args = append(args, relatedArgs...)
		args = append(args, exact, exact, exact, prefix, prefix, prefix)
		args = append(args, model.ActiveDeleteStatuses, model.ProjectVolumeLifecycleDeleting)
		args = append(args, whereArgs...)
		matchFilter := `(
			LOWER(pv.display_name) LIKE ? ESCAPE '\'
			OR LOWER(pv.claim_name) LIKE ? ESCAPE '\'
			OR LOWER(pv.id) LIKE ? ESCAPE '\'
		)`
		matchArgs := []any{contains, contains, prefix}
		if parsed.idMatchedType == SearchTypeProjectVolume {
			matchFilter = `LOWER(pv.id) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		}
		sql := fmt.Sprintf(`
			SELECT
				CAST('project_volume' AS TEXT) AS item_type,
				pv.id AS id,
				pv.display_name AS title,
				pv.capacity_request AS subtitle,
				pv.claim_name AS identifier,
				pv.access_mode AS scope,
				pv.lifecycle_state AS status,
				%s AS is_related,
				p.id AS project_id,
				p.name AS project_name,
				p.identifier AS project_identifier,
				CAST('' AS TEXT) AS application_id,
				CAST('' AS TEXT) AS application_name,
				pv.updated_at AS updated_at,
				CASE
					WHEN LOWER(pv.id) = ? OR LOWER(pv.display_name) = ? OR LOWER(pv.claim_name) = ? THEN 0
					WHEN LOWER(pv.id) LIKE ? ESCAPE '\' OR LOWER(pv.display_name) LIKE ? ESCAPE '\' OR LOWER(pv.claim_name) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM project_volumes pv
			JOIN projects p ON p.id = pv.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE pv.deleted_at IS NULL AND pv.lifecycle_state <> ? AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, pv.updated_at DESC
			LIMIT ?`, relatedExpr, kindOrder, projWhere, matchFilter)
		args = append(args, matchArgs...)
		args = append(args, execScope.limitPerType)
		return searchSQLBranch{sql: sql, args: args}, true

	case SearchTypeRuntimeConfigSet:
		projWhere, relatedExpr, whereArgs, relatedArgs, ok := projectVisibilityClause("p.id", execScope)
		if !ok {
			return searchSQLBranch{}, false
		}
		args := make([]any, 0, 14)
		args = append(args, relatedArgs...)
		args = append(args, exact, exact, prefix, prefix)
		args = append(args, model.ActiveDeleteStatuses, model.ActiveDeleteStatuses)
		args = append(args, whereArgs...)
		matchFilter := `(LOWER(rcs.name) LIKE ? ESCAPE '\' OR LOWER(rcs.id) LIKE ? ESCAPE '\')`
		matchArgs := []any{contains, prefix}
		if parsed.idMatchedType == SearchTypeRuntimeConfigSet {
			matchFilter = `LOWER(rcs.id) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		}
		sql := fmt.Sprintf(`
			SELECT
				CAST('runtime_config_set' AS TEXT) AS item_type,
				rcs.id AS id,
				rcs.name AS title,
				CAST('' AS TEXT) AS subtitle,
				rcs.id AS identifier,
				CAST('project' AS TEXT) AS scope,
				CASE WHEN rcs.enabled THEN CAST('enabled' AS TEXT) ELSE CAST('disabled' AS TEXT) END AS status,
				%s AS is_related,
				p.id AS project_id,
				p.name AS project_name,
				p.identifier AS project_identifier,
				CAST('' AS TEXT) AS application_id,
				CAST('' AS TEXT) AS application_name,
				rcs.updated_at AS updated_at,
				CASE
					WHEN LOWER(rcs.id) = ? OR LOWER(rcs.name) = ? THEN 0
					WHEN LOWER(rcs.id) LIKE ? ESCAPE '\' OR LOWER(rcs.name) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM project_runtime_config_sets rcs
			JOIN projects p ON p.id = rcs.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE rcs.deleted_at IS NULL AND rcs.delete_status IN ? AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, rcs.updated_at DESC
			LIMIT ?`, relatedExpr, kindOrder, projWhere, matchFilter)
		args = append(args, matchArgs...)
		args = append(args, execScope.limitPerType)
		return searchSQLBranch{sql: sql, args: args}, true

	case SearchTypeBuildVariableSet:
		return buildScopedBuildVariableSetBranch(kindOrder, exact, prefix, contains, parsed, execScope)

	case SearchTypeNotificationChannel:
		return buildScopedNotificationChannelBranch(kindOrder, exact, prefix, contains, parsed, execScope)

	case SearchTypeRuntimeCluster:
		return buildScopedClusterBranch(kindOrder, exact, prefix, contains, parsed, execScope)

	case SearchTypeArtifactRegistry:
		return buildScopedRegistryBranch(kindOrder, exact, prefix, contains, parsed, execScope)

	case SearchTypeBuildRun:
		projWhere, relatedExpr, whereArgs, relatedArgs, ok := projectVisibilityClause("p.id", execScope)
		if !ok {
			return searchSQLBranch{}, false
		}
		args := make([]any, 0, 16)
		args = append(args, relatedArgs...)
		args = append(args, exact, exact, prefix, prefix)
		args = append(args, model.ActiveDeleteStatuses, model.ActiveDeleteStatuses)
		args = append(args, whereArgs...)

		matchFilter := `(LOWER(br.id) LIKE ? ESCAPE '\' OR LOWER(br.source_commit) LIKE ? ESCAPE '\' OR LOWER(br.image_ref) LIKE ? ESCAPE '\')`
		matchArgs := []any{prefix, prefix, contains}
		if parsed.idMatchedType == SearchTypeBuildRun {
			matchFilter = `LOWER(br.id) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		} else if parsed.commitSHA && len(parsed.inlineTypes) == 0 {
			matchFilter = `LOWER(br.source_commit) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		}
		sql := fmt.Sprintf(`
			SELECT
				CAST('build_run' AS TEXT) AS item_type,
				br.id AS id,
				br.id AS title,
				br.image_ref AS subtitle,
				CASE WHEN br.source_commit <> '' THEN SUBSTR(br.source_commit, 1, 12) ELSE br.source_branch END AS identifier,
				br.source_branch AS scope,
				br.status AS status,
				%s AS is_related,
				p.id AS project_id,
				p.name AS project_name,
				p.identifier AS project_identifier,
				a.id AS application_id,
				a.name AS application_name,
				br.updated_at AS updated_at,
				CASE
					WHEN LOWER(br.id) = ? OR LOWER(br.source_commit) = ? THEN 0
					WHEN LOWER(br.id) LIKE ? ESCAPE '\' OR LOWER(br.source_commit) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM build_runs br
			JOIN applications a ON a.id = br.application_id AND a.deleted_at IS NULL AND a.delete_status IN ?
			JOIN projects p ON p.id = br.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE br.deleted_at IS NULL AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, br.updated_at DESC
			LIMIT ?`, relatedExpr, kindOrder, projWhere, matchFilter)
		args = append(args, matchArgs...)
		args = append(args, execScope.limitPerType)
		return searchSQLBranch{sql: sql, args: args}, true

	case SearchTypeRelease:
		projWhere, relatedExpr, whereArgs, relatedArgs, ok := projectVisibilityClause("p.id", execScope)
		if !ok {
			return searchSQLBranch{}, false
		}
		args := make([]any, 0, 14)
		args = append(args, relatedArgs...)
		args = append(args, exact, prefix)
		args = append(args, model.ActiveDeleteStatuses, model.ActiveDeleteStatuses)
		args = append(args, whereArgs...)

		matchFilter := `(LOWER(rel.id) LIKE ? ESCAPE '\' OR LOWER(rel.image_ref) LIKE ? ESCAPE '\')`
		matchArgs := []any{prefix, contains}
		if parsed.idMatchedType == SearchTypeRelease {
			matchFilter = `LOWER(rel.id) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		}
		sql := fmt.Sprintf(`
			SELECT
				CAST('release' AS TEXT) AS item_type,
				rel.id AS id,
				rel.id AS title,
				rel.image_ref AS subtitle,
				rel.type AS identifier,
				CAST('' AS TEXT) AS scope,
				rel.status AS status,
				%s AS is_related,
				p.id AS project_id,
				p.name AS project_name,
				p.identifier AS project_identifier,
				a.id AS application_id,
				a.name AS application_name,
				rel.updated_at AS updated_at,
				CASE
					WHEN LOWER(rel.id) = ? THEN 0
					WHEN LOWER(rel.id) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM releases rel
			JOIN applications a ON a.id = rel.application_id AND a.deleted_at IS NULL AND a.delete_status IN ?
			JOIN projects p ON p.id = rel.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE rel.deleted_at IS NULL AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, rel.updated_at DESC
			LIMIT ?`, relatedExpr, kindOrder, projWhere, matchFilter)
		args = append(args, matchArgs...)
		args = append(args, execScope.limitPerType)
		return searchSQLBranch{sql: sql, args: args}, true

	case SearchTypeUser:
		if !execScope.platformAdmin || execScope.conversationProjectID != "" {
			return searchSQLBranch{}, false
		}
		matchFilter := `(LOWER(u.name) LIKE ? ESCAPE '\' OR LOWER(u.email) LIKE ? ESCAPE '\' OR LOWER(u.id) LIKE ? ESCAPE '\')`
		matchArgs := []any{contains, contains, prefix}
		if parsed.idMatchedType == SearchTypeUser {
			matchFilter = `LOWER(u.id) LIKE ? ESCAPE '\'`
			matchArgs = []any{prefix}
		}
		args := []any{exact, exact, exact, prefix, prefix, prefix}
		args = append(args, matchArgs...)
		args = append(args, execScope.limitPerType)
		sql := fmt.Sprintf(`
			SELECT
				CAST('user' AS TEXT) AS item_type,
				u.id AS id,
				u.name AS title,
				u.email AS subtitle,
				u.role AS identifier,
				CAST('' AS TEXT) AS scope,
				CASE WHEN u.disabled THEN CAST('disabled' AS TEXT) ELSE CAST('active' AS TEXT) END AS status,
				TRUE AS is_related,
				CAST('' AS TEXT) AS project_id,
				CAST('' AS TEXT) AS project_name,
				CAST('' AS TEXT) AS project_identifier,
				CAST('' AS TEXT) AS application_id,
				CAST('' AS TEXT) AS application_name,
				u.updated_at AS updated_at,
				CASE
					WHEN LOWER(u.id) = ? OR LOWER(u.name) = ? OR LOWER(u.email) = ? THEN 0
					WHEN LOWER(u.id) LIKE ? ESCAPE '\' OR LOWER(u.name) LIKE ? ESCAPE '\' OR LOWER(u.email) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank,
				%d AS kind_order
			FROM users u
			WHERE u.deleted_at IS NULL AND %s
			ORDER BY match_rank ASC, u.updated_at DESC
			LIMIT ?`, kindOrder, matchFilter)
		return searchSQLBranch{sql: sql, args: args}, true

	default:
		return searchSQLBranch{}, false
	}
}

func scopedVisibilityClauses(
	alias string,
	resourceType string,
	userID string,
	projectIDs []string,
	allowUserScope bool,
	allProjects bool,
) (whereSQL string, relatedExpr string, whereArgs []any, relatedArgs []any) {
	whereParts := []string{fmt.Sprintf("%s.scope = 'global'", alias)}
	relatedParts := []string{fmt.Sprintf("%s.scope = 'global'", alias)}

	if allowUserScope {
		whereParts = append(whereParts, fmt.Sprintf("(%s.scope = 'user' AND %s.owner_ref = ?)", alias, alias))
		whereArgs = append(whereArgs, userID)
		relatedParts = append(relatedParts, fmt.Sprintf("(%s.scope = 'user' AND %s.owner_ref = ?)", alias, alias))
		relatedArgs = append(relatedArgs, userID)
	}

	if allProjects {
		whereParts = append(whereParts, fmt.Sprintf("%s.scope = 'project'", alias))
		if len(projectIDs) > 0 {
			relatedParts = append(relatedParts, fmt.Sprintf(
				"(%s.scope = 'project' AND EXISTS (SELECT 1 FROM scoped_resource_project_bindings b WHERE b.resource_type = ? AND b.resource_id = %s.id AND b.project_id IN ?))",
				alias, alias,
			))
			relatedArgs = append(relatedArgs, resourceType, projectIDs)
		}
	} else if len(projectIDs) > 0 {
		whereParts = append(whereParts, fmt.Sprintf(
			"(%s.scope = 'project' AND EXISTS (SELECT 1 FROM scoped_resource_project_bindings b WHERE b.resource_type = ? AND b.resource_id = %s.id AND b.project_id IN ?))",
			alias, alias,
		))
		whereArgs = append(whereArgs, resourceType, projectIDs)
		relatedParts = []string{"TRUE"}
		relatedArgs = nil
	} else {
		relatedParts = []string{"TRUE"}
		relatedArgs = nil
	}

	return "(" + strings.Join(whereParts, " OR ") + ")", "(" + strings.Join(relatedParts, " OR ") + ")", whereArgs, relatedArgs
}

func buildScopedBuildVariableSetBranch(kindOrder int, exact, prefix, contains string, parsed parsedSearchQuery, execScope searchExecutionScope) (searchSQLBranch, bool) {
	const resourceType = "build_variable_set"
	visWhere, relatedExpr, visArgs, relatedArgs := scopedVisibilityClauses(
		"bvs", resourceType, execScope.user.ID, execScope.relatedProjectIDs, true, execScope.allProjects,
	)

	matchFilter := `(LOWER(bvs.name) LIKE ? ESCAPE '\' OR LOWER(bvs.id) LIKE ? ESCAPE '\')`
	matchArgs := []any{contains, prefix}
	if parsed.idMatchedType == SearchTypeBuildVariableSet {
		matchFilter = `LOWER(bvs.id) LIKE ? ESCAPE '\'`
		matchArgs = []any{prefix}
	}

	lateralProjectFilter := ""
	var lateralArgs []any
	lateralArgs = append(lateralArgs, model.ActiveDeleteStatuses, resourceType)
	if !execScope.allProjects && len(execScope.relatedProjectIDs) > 0 {
		lateralProjectFilter = "AND b.project_id IN ?"
		lateralArgs = append(lateralArgs, execScope.relatedProjectIDs)
	}

	args := make([]any, 0, 16)
	args = append(args, relatedArgs...)
	args = append(args, exact, exact, prefix, prefix)
	args = append(args, visArgs...)
	args = append(args, matchArgs...)
	args = append(args, execScope.limitPerType)
	args = append(args, lateralArgs...)

	sql := fmt.Sprintf(`
		SELECT
			CAST('build_variable_set' AS TEXT) AS item_type,
			matched.id AS id,
			matched.name AS title,
			CAST('' AS TEXT) AS subtitle,
			matched.id AS identifier,
			matched.scope AS scope,
			CASE WHEN matched.enabled THEN CAST('enabled' AS TEXT) ELSE CAST('disabled' AS TEXT) END AS status,
			matched.is_related AS is_related,
			COALESCE(bp.id, CAST('' AS TEXT)) AS project_id,
			COALESCE(bp.name, CAST('' AS TEXT)) AS project_name,
			COALESCE(bp.identifier, CAST('' AS TEXT)) AS project_identifier,
			CAST('' AS TEXT) AS application_id,
			CAST('' AS TEXT) AS application_name,
			matched.updated_at AS updated_at,
			matched.match_rank AS match_rank,
			%d AS kind_order
		FROM (
			SELECT
				bvs.id,
				bvs.name,
				bvs.scope,
				bvs.enabled,
				bvs.updated_at,
				%s AS is_related,
				CASE
					WHEN LOWER(bvs.id) = ? OR LOWER(bvs.name) = ? THEN 0
					WHEN LOWER(bvs.id) LIKE ? ESCAPE '\' OR LOWER(bvs.name) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank
			FROM build_variable_sets bvs
			WHERE bvs.deleted_at IS NULL AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, bvs.updated_at DESC
			LIMIT ?
		) AS matched
		LEFT JOIN LATERAL (
			SELECT p.id, p.name, p.identifier
			FROM scoped_resource_project_bindings b
			JOIN projects p ON p.id = b.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE b.resource_type = ? AND b.resource_id = matched.id %s
			ORDER BY b.is_default DESC, b.project_id ASC
			LIMIT 1
		) bp ON matched.scope = 'project'`, kindOrder, relatedExpr, visWhere, matchFilter, lateralProjectFilter)

	return searchSQLBranch{sql: sql, args: args}, true
}

func buildScopedNotificationChannelBranch(kindOrder int, exact, prefix, contains string, parsed parsedSearchQuery, execScope searchExecutionScope) (searchSQLBranch, bool) {
	resourceType := notification.ScopedResourceChannelType
	visWhere, relatedExpr, visArgs, relatedArgs := scopedVisibilityClauses(
		"nc", resourceType, execScope.user.ID, execScope.notificationProjectIDs, false, execScope.allProjects,
	)

	matchFilter := `(
		LOWER(nc.name) LIKE ? ESCAPE '\'
		OR LOWER(nc.adapter_kind) LIKE ? ESCAPE '\'
		OR LOWER(nc.preset_id) LIKE ? ESCAPE '\'
		OR LOWER(nc.id) LIKE ? ESCAPE '\'
	)`
	matchArgs := []any{contains, contains, contains, prefix}
	if parsed.idMatchedType == SearchTypeNotificationChannel {
		matchFilter = `LOWER(nc.id) LIKE ? ESCAPE '\'`
		matchArgs = []any{prefix}
	}

	lateralProjectFilter := ""
	var lateralArgs []any
	lateralArgs = append(lateralArgs, model.ActiveDeleteStatuses, resourceType)
	if !execScope.allProjects && len(execScope.notificationProjectIDs) > 0 {
		lateralProjectFilter = "AND b.project_id IN ?"
		lateralArgs = append(lateralArgs, execScope.notificationProjectIDs)
	}

	args := make([]any, 0, 18)
	args = append(args, relatedArgs...)
	args = append(args, exact, exact, prefix, prefix)
	args = append(args, visArgs...)
	args = append(args, matchArgs...)
	args = append(args, execScope.limitPerType)
	args = append(args, lateralArgs...)

	sql := fmt.Sprintf(`
		SELECT
			CAST('notification_channel' AS TEXT) AS item_type,
			matched.id AS id,
			matched.name AS title,
			matched.adapter_kind AS subtitle,
			matched.preset_id AS identifier,
			matched.scope AS scope,
			CASE WHEN matched.enabled THEN CAST('enabled' AS TEXT) ELSE CAST('disabled' AS TEXT) END AS status,
			matched.is_related AS is_related,
			COALESCE(bp.id, CAST('' AS TEXT)) AS project_id,
			COALESCE(bp.name, CAST('' AS TEXT)) AS project_name,
			COALESCE(bp.identifier, CAST('' AS TEXT)) AS project_identifier,
			CAST('' AS TEXT) AS application_id,
			CAST('' AS TEXT) AS application_name,
			matched.updated_at AS updated_at,
			matched.match_rank AS match_rank,
			%d AS kind_order
		FROM (
			SELECT
				nc.id,
				nc.name,
				nc.adapter_kind,
				nc.preset_id,
				nc.scope,
				nc.enabled,
				nc.updated_at,
				%s AS is_related,
				CASE
					WHEN LOWER(nc.id) = ? OR LOWER(nc.name) = ? THEN 0
					WHEN LOWER(nc.id) LIKE ? ESCAPE '\' OR LOWER(nc.name) LIKE ? ESCAPE '\' THEN 1
					ELSE 2
				END AS match_rank
			FROM notification_channels nc
			WHERE nc.deleted_at IS NULL AND nc.owner_user_id = '' AND %s AND %s
			ORDER BY is_related DESC, match_rank ASC, nc.updated_at DESC
			LIMIT ?
		) AS matched
		LEFT JOIN LATERAL (
			SELECT p.id, p.name, p.identifier
			FROM scoped_resource_project_bindings b
			JOIN projects p ON p.id = b.project_id AND p.deleted_at IS NULL AND p.delete_status IN ?
			WHERE b.resource_type = ? AND b.resource_id = matched.id %s
			ORDER BY b.is_default DESC, b.project_id ASC
			LIMIT 1
		) bp ON matched.scope = 'project'`, kindOrder, relatedExpr, visWhere, matchFilter, lateralProjectFilter)

	return searchSQLBranch{sql: sql, args: args}, true
}

func buildScopedClusterBranch(kindOrder int, exact, prefix, contains string, parsed parsedSearchQuery, execScope searchExecutionScope) (searchSQLBranch, bool) {
	const resourceType = "runtime_cluster"
	visWhere, relatedExpr, visArgs, relatedArgs := scopedVisibilityClauses(
		"rc", resourceType, execScope.user.ID, execScope.relatedProjectIDs, true, execScope.allProjects,
	)

	matchFilter := `(LOWER(rc.name) LIKE ? ESCAPE '\' OR LOWER(rc.endpoint) LIKE ? ESCAPE '\' OR LOWER(rc.id) LIKE ? ESCAPE '\')`
	matchArgs := []any{contains, contains, prefix}
	if parsed.idMatchedType == SearchTypeRuntimeCluster {
		matchFilter = `LOWER(rc.id) LIKE ? ESCAPE '\'`
		matchArgs = []any{prefix}
	}

	args := make([]any, 0, 14)
	args = append(args, relatedArgs...)
	args = append(args, exact, exact, prefix, prefix)
	args = append(args, visArgs...)
	args = append(args, matchArgs...)
	args = append(args, execScope.limitPerType)

	sql := fmt.Sprintf(`
		SELECT
			CAST('runtime_cluster' AS TEXT) AS item_type,
			rc.id AS id,
			rc.name AS title,
			rc.endpoint AS subtitle,
			rc.id AS identifier,
			rc.scope AS scope,
			CAST('' AS TEXT) AS status,
			%s AS is_related,
			CAST('' AS TEXT) AS project_id,
			CAST('' AS TEXT) AS project_name,
			CAST('' AS TEXT) AS project_identifier,
			CAST('' AS TEXT) AS application_id,
			CAST('' AS TEXT) AS application_name,
			rc.updated_at AS updated_at,
			CASE
				WHEN LOWER(rc.id) = ? OR LOWER(rc.name) = ? THEN 0
				WHEN LOWER(rc.id) LIKE ? ESCAPE '\' OR LOWER(rc.name) LIKE ? ESCAPE '\' THEN 1
				ELSE 2
			END AS match_rank,
			%d AS kind_order
		FROM runtime_clusters rc
		WHERE rc.deleted_at IS NULL AND rc.delete_status = 'active' AND %s AND %s
		ORDER BY is_related DESC, match_rank ASC, rc.updated_at DESC
		LIMIT ?`, relatedExpr, kindOrder, visWhere, matchFilter)

	return searchSQLBranch{sql: sql, args: args}, true
}

func buildScopedRegistryBranch(kindOrder int, exact, prefix, contains string, parsed parsedSearchQuery, execScope searchExecutionScope) (searchSQLBranch, bool) {
	const resourceType = "artifact_registry"
	visWhere, relatedExpr, visArgs, relatedArgs := scopedVisibilityClauses(
		"ar", resourceType, execScope.user.ID, execScope.relatedProjectIDs, true, execScope.allProjects,
	)

	matchFilter := `(
		LOWER(ar.name) LIKE ? ESCAPE '\'
		OR LOWER(ar.endpoint) LIKE ? ESCAPE '\'
		OR LOWER(ar.namespace) LIKE ? ESCAPE '\'
		OR LOWER(ar.id) LIKE ? ESCAPE '\'
	)`
	matchArgs := []any{contains, contains, contains, prefix}
	if parsed.idMatchedType == SearchTypeArtifactRegistry {
		matchFilter = `LOWER(ar.id) LIKE ? ESCAPE '\'`
		matchArgs = []any{prefix}
	}

	args := make([]any, 0, 16)
	args = append(args, relatedArgs...)
	args = append(args, exact, exact, prefix, prefix)
	args = append(args, visArgs...)
	args = append(args, matchArgs...)
	args = append(args, execScope.limitPerType)

	sql := fmt.Sprintf(`
		SELECT
			CAST('artifact_registry' AS TEXT) AS item_type,
			ar.id AS id,
			ar.name AS title,
			ar.endpoint AS subtitle,
			ar.provider AS identifier,
			ar.scope AS scope,
			CAST('' AS TEXT) AS status,
			%s AS is_related,
			CAST('' AS TEXT) AS project_id,
			CAST('' AS TEXT) AS project_name,
			CAST('' AS TEXT) AS project_identifier,
			CAST('' AS TEXT) AS application_id,
			CAST('' AS TEXT) AS application_name,
			ar.updated_at AS updated_at,
			CASE
				WHEN LOWER(ar.id) = ? OR LOWER(ar.name) = ? THEN 0
				WHEN LOWER(ar.id) LIKE ? ESCAPE '\' OR LOWER(ar.name) LIKE ? ESCAPE '\' THEN 1
				ELSE 2
			END AS match_rank,
			%d AS kind_order
		FROM artifact_registries ar
		WHERE ar.deleted_at IS NULL AND %s AND %s
		ORDER BY is_related DESC, match_rank ASC, ar.updated_at DESC
		LIMIT ?`, relatedExpr, kindOrder, visWhere, matchFilter)

	return searchSQLBranch{sql: sql, args: args}, true
}
