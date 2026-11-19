package aitool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Spec defines a strongly-typed Code-First AI Domain Facade Tool.
type Spec[In, Out any] struct {
	Name              string
	Domain            string
	Title             string
	Summary           string
	Category          string
	Tags              []string
	AliasesZH         []string
	AliasesEN         []string
	Purpose           LocalizedText
	AvoidWhen         LocalizedText
	Preconditions     LocalizedList
	SuccessEvidence   LocalizedText
	AdminOnly         bool
	GlobalSessionOnly bool
	// AllowPersonalProjectOmit allows omitting `project` in a project-bound session
	// without auto-injecting conversationProjectId (for personal wallet queries).
	AllowPersonalProjectOmit bool
	ApprovalTier             string
	Idempotent               bool
	RequiredScopes           []string
	SensitivePaths           []string
	Effect                   string
	Verification             []OperationVerification
	RetryOnUnknown           bool
	VerificationOnly         bool

	// Preview is mandatory for all L2 tools and optional for L1 tools.
	Preview func(tc *ToolContext, in In) (*PreviewResult, error)
	// Execute performs the actual domain read or mutation.
	Execute func(tc *ToolContext, in In) (Out, error)
}

type RegisteredTool struct {
	Catalog                  CatalogOperation
	AllowPersonalProjectOmit bool
	HasPreview               bool
	previewFn                func(tc *ToolContext, rawArgs json.RawMessage) (*PreviewResult, error)
	executeFn                func(tc *ToolContext, rawArgs json.RawMessage) (any, error)
}

func (t *RegisteredTool) Preview(tc *ToolContext, rawArgs json.RawMessage) (*PreviewResult, error) {
	if t == nil || t.previewFn == nil {
		return nil, NewError(400, "ai.tool_preview_unsupported", "tool does not support preview")
	}
	return t.previewFn(tc, rawArgs)
}

func (t *RegisteredTool) Execute(tc *ToolContext, rawArgs json.RawMessage) (any, error) {
	if t == nil || t.executeFn == nil {
		return nil, NewError(500, "ai.tool_not_implemented", "tool execute handler is missing")
	}
	return t.executeFn(tc, rawArgs)
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]*RegisteredTool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*RegisteredTool, 180),
	}
}

var domainDefaultAliasesZH = map[string][]string{
	DomainDelivery:  {"应用交付", "部署目标", "发布", "应用模板", "配置集"},
	DomainBuild:     {"构建", "镜像", "代码仓库", "流水线", "构建变量"},
	DomainRuntime:   {"运行集群", "容器排障", "日志", "数据卷", "生命周期钩子"},
	DomainNetwork:   {"网关路由", "域名", "证书", "服务依赖", "拓扑"},
	DomainWorkspace: {"项目空间", "成员权限", "网络隔离", "跨项目共享"},
	DomainOps:       {"平台运维", "个人账单", "站内信", "通知规则", "安全令牌"},
	DomainAdmin:     {"平台管理", "系统设置", "全局治理", "用户管理", "数据清理"},
}

func Register[In, Out any](reg *Registry, spec Spec[In, Out]) {
	if reg == nil {
		panic("aitool.Register: nil registry")
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		panic("aitool.Register: empty tool Name")
	}
	domain := strings.TrimSpace(spec.Domain)
	switch domain {
	case DomainDelivery, DomainBuild, DomainRuntime, DomainNetwork, DomainWorkspace, DomainOps, DomainAdmin:
	default:
		panic(fmt.Sprintf("aitool.Register(%s): invalid domain %q", name, domain))
	}
	if !strings.HasPrefix(name, domain+"_") {
		panic(fmt.Sprintf("aitool.Register(%s): tool name must start with %q", name, domain+"_"))
	}
	tier := strings.ToUpper(strings.TrimSpace(spec.ApprovalTier))
	switch tier {
	case ApprovalTierL0, ApprovalTierL1, ApprovalTierL2:
	default:
		panic(fmt.Sprintf("aitool.Register(%s): invalid approvalTier %q", name, spec.ApprovalTier))
	}
	effect := strings.TrimSpace(spec.Effect)
	if effect == "" {
		if tier == ApprovalTierL0 {
			effect = EffectRead
		} else {
			effect = EffectWrite
		}
	}
	switch effect {
	case EffectRead, EffectWrite, EffectAsync, EffectExecute:
	default:
		panic(fmt.Sprintf("aitool.Register(%s): invalid effect %q", name, effect))
	}
	if spec.Execute == nil {
		panic(fmt.Sprintf("aitool.Register(%s): Execute handler is nil", name))
	}
	if tier == ApprovalTierL2 && spec.Preview == nil {
		panic(fmt.Sprintf("aitool.Register(%s): L2 tool must provide a Preview handler", name))
	}
	if len(spec.RequiredScopes) == 0 {
		panic(fmt.Sprintf("aitool.Register(%s): RequiredScopes must not be empty", name))
	}
	if (effect == EffectWrite || effect == EffectAsync) && len(spec.Verification) == 0 {
		panic(fmt.Sprintf("aitool.Register(%s): %s tool must declare at least one Verification rule", name, effect))
	}

	adminOnly := spec.AdminOnly || domain == DomainAdmin
	globalSessionOnly := spec.GlobalSessionOnly || adminOnly

	inputSchema, autoSensitive := SchemaForInput[In]()
	outputSchema := SchemaForOutput[Out]()
	sensitivePaths := dedupeSortedStrings(append(append([]string(nil), autoSensitive...), spec.SensitivePaths...))

	summary := strings.TrimSpace(spec.Summary)
	title := strings.TrimSpace(spec.Title)
	if title == "" {
		title = summary
	}
	if summary == "" {
		summary = title
	}
	purpose := spec.Purpose
	if strings.TrimSpace(purpose.ZH) == "" {
		purpose = Text(summary, summary)
	}
	avoidWhen := spec.AvoidWhen
	if strings.TrimSpace(avoidWhen.ZH) == "" {
		if tier == ApprovalTierL0 {
			avoidWhen = Text("需要修改配置或执行变更时不要调用此只读工具。", "Do not use this read-only tool when mutating resources.")
		} else {
			avoidWhen = Text("仅需查询状态或尚未确认目标资源时不要调用此变更工具。", "Do not use this mutating tool when only inspecting state.")
		}
	}
	preconditions := spec.Preconditions
	if len(preconditions.ZH) == 0 && len(preconditions.EN) == 0 {
		preconditions = List([]string{}, []string{})
	}
	successEvidence := spec.SuccessEvidence
	if strings.TrimSpace(successEvidence.ZH) == "" {
		if tier == ApprovalTierL0 {
			successEvidence = Text("返回结构化查询结果且无错误码。", "Returns structured read payload without error codes.")
		} else {
			successEvidence = Text("变更已提交并通过权威回读工具验证。", "Mutation is committed and confirmed by authoritative readback.")
		}
	}

	category := strings.TrimSpace(spec.Category)
	if category == "" {
		category = domain
	}
	tags := append([]string{domain}, spec.Tags...)
	tags = dedupePreserveOrder(tags)

	enTokens := SplitOperationIdentifier(name)
	aliasesEN := dedupePreserveOrder(append(append([]string{name, strings.Join(enTokens, " ")}, enTokens...), spec.AliasesEN...))
	aliasesZH := dedupePreserveOrder(append(append([]string{}, spec.AliasesZH...), domainDefaultAliasesZH[domain]...))

	idempotent := spec.Idempotent
	if tier == ApprovalTierL0 && effect == EffectRead {
		idempotent = true
	}

	catOp := CatalogOperation{
		OperationID:       name,
		Domain:            domain,
		Name:              title,
		Summary:           summary,
		Category:          category,
		Tags:              tags,
		Aliases:           OperationAliases{ZH: aliasesZH, EN: aliasesEN},
		Purpose:           purpose,
		AvoidWhen:         avoidWhen,
		Preconditions:     preconditions,
		SuccessEvidence:   successEvidence,
		AdminOnly:         adminOnly,
		GlobalSessionOnly: globalSessionOnly,
		ApprovalTier:      tier,
		RequiresApproval:  tier != ApprovalTierL0,
		Idempotent:        idempotent,
		RequiredScopes:    append([]string(nil), spec.RequiredScopes...),
		InputSchema:       inputSchema,
		OutputSchema:      outputSchema,
		SensitivePaths:    sensitivePaths,
		Effect:            effect,
		Verification:      append([]OperationVerification(nil), spec.Verification...),
		RetryOnUnknown:    spec.RetryOnUnknown,
		VerificationOnly:  spec.VerificationOnly,
	}

	var previewWrapper func(tc *ToolContext, rawArgs json.RawMessage) (*PreviewResult, error)
	if spec.Preview != nil {
		previewWrapper = func(tc *ToolContext, rawArgs json.RawMessage) (*PreviewResult, error) {
			in, err := decodeStrictArgs[In](rawArgs)
			if err != nil {
				return nil, err
			}
			return spec.Preview(tc, in)
		}
	}

	executeWrapper := func(tc *ToolContext, rawArgs json.RawMessage) (any, error) {
		in, err := decodeStrictArgs[In](rawArgs)
		if err != nil {
			return nil, err
		}
		return spec.Execute(tc, in)
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()
	if _, exists := reg.tools[name]; exists {
		panic(fmt.Sprintf("aitool.Register: duplicate tool %q", name))
	}
	reg.tools[name] = &RegisteredTool{
		Catalog:                  catOp,
		AllowPersonalProjectOmit: spec.AllowPersonalProjectOmit,
		HasPreview:               spec.Preview != nil,
		previewFn:                previewWrapper,
		executeFn:                executeWrapper,
	}
}

func decodeStrictArgs[In any](rawArgs json.RawMessage) (In, error) {
	var in In
	trimmed := bytes.TrimSpace(rawArgs)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		trimmed = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return in, BadRequest("ai.tool_arguments_invalid", err.Error())
	}
	return in, nil
}

func (r *Registry) Get(name string) (*RegisteredTool, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[strings.TrimSpace(name)]
	return tool, ok
}

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

func (r *Registry) Catalog() []CatalogOperation {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CatalogOperation, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Catalog)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].OperationID < out[j].OperationID
	})
	return out
}

func (r *Registry) ValidateCrossReferences() error {
	if r == nil {
		return fmt.Errorf("nil aitool registry")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, tool := range r.tools {
		for _, v := range tool.Catalog.Verification {
			target, ok := r.tools[v.OperationID]
			if !ok {
				return fmt.Errorf("tool %s references missing verification operation %q", name, v.OperationID)
			}
			if target.Catalog.ApprovalTier != ApprovalTierL0 {
				return fmt.Errorf("tool %s references non-L0 verification operation %q (%s)", name, v.OperationID, target.Catalog.ApprovalTier)
			}
			if !tool.Catalog.AdminOnly && target.Catalog.AdminOnly {
				return fmt.Errorf("tenant tool %s must not reference adminOnly verification operation %q", name, v.OperationID)
			}
		}
	}
	return nil
}

// SplitOperationIdentifier splits both snake_case and camelCase identifiers into lowercase tokens.
func SplitOperationIdentifier(identifier string) []string {
	var tokens []string
	for _, segment := range strings.FieldsFunc(identifier, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || unicode.IsSpace(r)
	}) {
		if segment == "" {
			continue
		}
		var current []rune
		runes := []rune(segment)
		for i, r := range runes {
			if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
				tokens = append(tokens, strings.ToLower(string(current)))
				current = []rune{r}
				continue
			}
			current = append(current, r)
		}
		if len(current) > 0 {
			tokens = append(tokens, strings.ToLower(string(current)))
		}
	}
	return slices.Compact(tokens)
}

func dedupePreserveOrder(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
