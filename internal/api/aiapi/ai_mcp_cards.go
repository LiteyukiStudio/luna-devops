package aiapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/url"
	"regexp"
	"strings"

	"github.com/LiteyukiStudio/devops/docs"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/model"
)

const (
	mcpCardViewerResourceURI = "ui://luna-devops/card-viewer.html"
	mcpAppHTMLMimeType       = "text/html;profile=mcp-app"
	maxMCPCardPayloadBytes   = 128 * 1024
	maxMCPCardListItems      = 64
	maxMCPCardTextRunes      = 1024
)

var (
	allowedMCPCardTemplates = []string{
		"diagnostics",
		"change_review",
		"inventory",
		"topology",
	}
	inlineURLCredentialPattern = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)([^/\s@]+@)`)
	inlineQuerySecretPattern   = regexp.MustCompile(`(?i)([?&](?:access_token|refresh_token|id_token|token|api_key|apikey|secret|password|passwd|key|sig|signature|code|state)=)([^&#\s"']+)`)
)

type mcpRenderCardRequest struct {
	Template string          `json:"template"`
	Title    string          `json:"title"`
	Subtitle string          `json:"subtitle"`
	Locale   string          `json:"locale"`
	Data     json.RawMessage `json:"data"`
}

type mcpCardLabels struct {
	HTMLTag               string `json:"htmlLang"`
	Target                string `json:"target"`
	Cluster               string `json:"cluster"`
	Project               string `json:"project"`
	Tool                  string `json:"tool"`
	ReplicasAndRestarts   string `json:"replicasAndRestarts"`
	RestartsPrefix        string `json:"restartsPrefix"`
	UpdatedPrefix         string `json:"updatedPrefix"`
	AvailablePrefix       string `json:"availablePrefix"`
	Memory                string `json:"memory"`
	PVCSection            string `json:"pvcSection"`
	ColName               string `json:"colName"`
	ColStatus             string `json:"colStatus"`
	ColCapacity           string `json:"colCapacity"`
	ColMountPath          string `json:"colMountPath"`
	WarningsSection       string `json:"warningsSection"`
	ColReasonObject       string `json:"colReasonObject"`
	ColEventMessage       string `json:"colEventMessage"`
	ColCount              string `json:"colCount"`
	TargetResource        string `json:"targetResource"`
	DiffSection           string `json:"diffSection"`
	ColField              string `json:"colField"`
	ColBefore             string `json:"colBefore"`
	ColAfter              string `json:"colAfter"`
	SensitiveKeysPrefix   string `json:"sensitiveKeysPrefix"`
	ImpactSection         string `json:"impactSection"`
	WarningsCallout       string `json:"warningsCallout"`
	DefaultConfirmNote    string `json:"defaultConfirmNote"`
	InventoryItemsSection string `json:"inventoryItemsSection"`
	ColKindStage          string `json:"colKindStage"`
	ColAccessOrDetail     string `json:"colAccessOrDetail"`
	TopologyEntrySection  string `json:"topologyEntrySection"`
	ColEntryRoute         string `json:"colEntryRoute"`
	ColProtocolTLS        string `json:"colProtocolTls"`
	ColBackendTarget      string `json:"colBackendTarget"`
	TopologyNodesSection  string `json:"topologyNodesSection"`
	ColNode               string `json:"colNode"`
	ColReplicasOrNote     string `json:"colReplicasOrNote"`
	TopologyBindSection   string `json:"topologyBindSection"`
	ColFrom               string `json:"colFrom"`
	ColTo                 string `json:"colTo"`
	ColEndpointInjection  string `json:"colEndpointInjection"`
}

func cardLabelsForLocale(locale string) mcpCardLabels {
	if docs.NormalizeLocale(locale) == "en" {
		return mcpCardLabels{
			HTMLTag:               "en",
			Target:                "Target: ",
			Cluster:               "Cluster: ",
			Project:               "Project: ",
			Tool:                  "Tool: ",
			ReplicasAndRestarts:   "Pod Ready / Restarts",
			RestartsPrefix:        "Restarts ",
			UpdatedPrefix:         "Updated ",
			AvailablePrefix:       "Available ",
			Memory:                "Memory",
			PVCSection:            "Persistent Volumes (PVC)",
			ColName:               "Name",
			ColStatus:             "Status",
			ColCapacity:           "Capacity",
			ColMountPath:          "Mount Path",
			WarningsSection:       "Recent Warning Events",
			ColReasonObject:       "Reason / Object",
			ColEventMessage:       "Message",
			ColCount:              "Count",
			TargetResource:        "Target Resource: ",
			DiffSection:           "Change Diff (Before → After)",
			ColField:              "Field",
			ColBefore:             "Before",
			ColAfter:              "After",
			SensitiveKeysPrefix:   "Sensitive Keys Touched (names only): ",
			ImpactSection:         "Impact Scope",
			WarningsCallout:       "Risk Warnings",
			DefaultConfirmNote:    "Read-only preview (not yet executed): reply with explicit confirmation in chat before the assistant calls execute_tool with previewLock.",
			InventoryItemsSection: "Resources & Endpoints",
			ColKindStage:          "Kind / Stage",
			ColAccessOrDetail:     "URL / Detail",
			TopologyEntrySection:  "Gateway Entrypoints (GatewayRoute)",
			ColEntryRoute:         "Route",
			ColProtocolTLS:        "Protocol / TLS",
			ColBackendTarget:      "Target Service",
			TopologyNodesSection:  "Service Nodes",
			ColNode:               "Node",
			ColReplicasOrNote:     "Replicas / Note",
			TopologyBindSection:   "Service Dependencies (ServiceBinding)",
			ColFrom:               "Upstream Caller",
			ColTo:                 "Downstream Target",
			ColEndpointInjection:  "Endpoint / Injection",
		}
	}
	return mcpCardLabels{
		HTMLTag:               "zh-CN",
		Target:                "目标：",
		Cluster:               "集群：",
		Project:               "项目：",
		Tool:                  "工具：",
		ReplicasAndRestarts:   "Pod 就绪副本 / 重启",
		RestartsPrefix:        "重启 ",
		UpdatedPrefix:         "已更新 ",
		AvailablePrefix:       "可用 ",
		Memory:                "内存",
		PVCSection:            "持久化数据卷（PVC）",
		ColName:               "名称",
		ColStatus:             "状态",
		ColCapacity:           "容量",
		ColMountPath:          "挂载路径",
		WarningsSection:       "近期 Warning 事件",
		ColReasonObject:       "原因 / 对象",
		ColEventMessage:       "事件详情",
		ColCount:              "次数",
		TargetResource:        "目标资源：",
		DiffSection:           "变更差异对比（Before → After）",
		ColField:              "字段",
		ColBefore:             "变更前",
		ColAfter:              "变更后",
		SensitiveKeysPrefix:   "涉及敏感键（仅显示键名）：",
		ImpactSection:         "影响范围",
		WarningsCallout:       "风险提示",
		DefaultConfirmNote:    "只读变更预览（尚未执行）：请在对话中向助手明确回复确认后，由助手携带 previewLock 调用 execute_tool 执行。",
		InventoryItemsSection: "资源与访问入口清单",
		ColKindStage:          "类型 / 环境",
		ColAccessOrDetail:     "访问地址 / 说明",
		TopologyEntrySection:  "公网网关入口（GatewayRoute）",
		ColEntryRoute:         "路由入口",
		ColProtocolTLS:        "协议 / TLS",
		ColBackendTarget:      "后端目标服务",
		TopologyNodesSection:  "服务节点",
		ColNode:               "节点",
		ColReplicasOrNote:     "副本 / 备注",
		TopologyBindSection:   "服务依赖绑定（ServiceBinding）",
		ColFrom:               "上游调用方",
		ColTo:                 "下游依赖服务",
		ColEndpointInjection:  "协议端口 / 注入",
	}
}

type mcpNormalizedCard struct {
	Template     string                        `json:"template"`
	TemplateTag  string                        `json:"templateTag"`
	Locale       string                        `json:"locale"`
	Labels       mcpCardLabels                 `json:"labels"`
	Title        string                        `json:"title"`
	Subtitle     string                        `json:"subtitle,omitempty"`
	Diagnostics  *mcpDiagnosticsCardViewModel  `json:"diagnostics,omitempty"`
	ChangeReview *mcpChangeReviewCardViewModel `json:"changeReview,omitempty"`
	Inventory    *mcpInventoryCardViewModel    `json:"inventory,omitempty"`
	Topology     *mcpTopologyCardViewModel     `json:"topology,omitempty"`
}

type mcpDiagnosticsCardInput struct {
	Status      string                       `json:"status"`
	StatusLabel string                       `json:"statusLabel"`
	Target      string                       `json:"target"`
	Cluster     string                       `json:"cluster"`
	Summary     string                       `json:"summary"`
	Replicas    *mcpFlexibleReplicasIn       `json:"replicas"`
	CPU         *mcpDiagnosticsMeterIn       `json:"cpu"`
	Memory      *mcpDiagnosticsMeterIn       `json:"memory"`
	PVCs        []mcpDiagnosticsPVCIn        `json:"pvcs"`
	Warnings    []mcpFlexibleDiagWarningItem `json:"warnings"`
}

type mcpFlexibleReplicasIn struct {
	Text      string
	Ready     int
	Desired   int
	Updated   *int
	Available *int
	Restarts  int
	IsObject  bool
}

func (r *mcpFlexibleReplicasIn) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		r.Text = s
		return nil
	}
	if trimmed[0] != '{' {
		var n int
		if err := json.Unmarshal(trimmed, &n); err != nil {
			return fmt.Errorf("replicas must be an object or string")
		}
		r.Text = fmt.Sprintf("%d", n)
		return nil
	}
	var obj struct {
		Ready     int  `json:"ready"`
		Desired   int  `json:"desired"`
		Updated   *int `json:"updated"`
		Available *int `json:"available"`
		Restarts  int  `json:"restarts"`
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&obj); err != nil {
		return err
	}
	r.Ready = obj.Ready
	r.Desired = obj.Desired
	r.Updated = obj.Updated
	r.Available = obj.Available
	r.Restarts = obj.Restarts
	r.IsObject = true
	return nil
}

type mcpFlexibleDiagWarningItem struct {
	Reason   string
	Object   string
	Pod      string
	Message  string
	Count    int
	LastSeen string
}

func (w *mcpFlexibleDiagWarningItem) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		w.Message = s
		return nil
	}
	var obj struct {
		Reason   string `json:"reason"`
		Object   string `json:"object"`
		Pod      string `json:"pod"`
		Message  string `json:"message"`
		Count    int    `json:"count"`
		LastSeen string `json:"lastSeen"`
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&obj); err != nil {
		return err
	}
	w.Reason = obj.Reason
	w.Object = obj.Object
	w.Pod = obj.Pod
	w.Message = obj.Message
	w.Count = obj.Count
	w.LastSeen = obj.LastSeen
	return nil
}

type mcpDiagnosticsMeterIn struct {
	Usage   string   `json:"usage"`
	Limit   string   `json:"limit"`
	Percent *float64 `json:"percent"`
}

type mcpDiagnosticsPVCIn struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Capacity  string `json:"capacity"`
	MountPath string `json:"mountPath"`
}

type mcpDiagnosticsCardViewModel struct {
	Status      string                      `json:"status,omitempty"`
	StatusLabel string                      `json:"statusLabel"`
	StatusTone  string                      `json:"statusTone"`
	Target      string                      `json:"target,omitempty"`
	Cluster     string                      `json:"cluster,omitempty"`
	Summary     string                      `json:"summary,omitempty"`
	Replicas    *mcpDiagnosticsReplicasView `json:"replicas,omitempty"`
	CPU         *mcpDiagnosticsMeterView    `json:"cpu,omitempty"`
	Memory      *mcpDiagnosticsMeterView    `json:"memory,omitempty"`
	PVCs        []mcpDiagnosticsPVCView     `json:"pvcs,omitempty"`
	Warnings    []mcpDiagnosticsWarningView `json:"warnings,omitempty"`
}

type mcpDiagnosticsReplicasView struct {
	ReadyText   string `json:"readyText"`
	Restarts    int    `json:"restarts"`
	HasRestarts bool   `json:"hasRestarts"`
	ExtraDetail string `json:"extraDetail,omitempty"`
	Tone        string `json:"tone"`
}

type mcpDiagnosticsMeterView struct {
	Label      string `json:"label"`
	ValueText  string `json:"valueText"`
	PercentInt int    `json:"percentInt"`
	HasPercent bool   `json:"hasPercent"`
	Tone       string `json:"tone"`
}

type mcpDiagnosticsPVCView struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Tone      string `json:"tone"`
	Capacity  string `json:"capacity,omitempty"`
	MountPath string `json:"mountPath,omitempty"`
}

type mcpDiagnosticsWarningView struct {
	Reason   string `json:"reason,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
	Count    int    `json:"count,omitempty"`
	LastSeen string `json:"lastSeen,omitempty"`
}

type mcpChangeReviewCardInput struct {
	ToolName         string                       `json:"toolName"`
	ApprovalTier     string                       `json:"approvalTier"`
	TargetResource   string                       `json:"targetResource"`
	TargetID         string                       `json:"targetId"`
	ProjectID        string                       `json:"projectId"`
	Summary          string                       `json:"summary"`
	Changes          []mcpChangeDiffItemIn        `json:"changes"`
	SensitiveKeys    []string                     `json:"sensitiveKeys"`
	Impact           []string                     `json:"impact"`
	Warnings         []mcpFlexibleDiagWarningItem `json:"warnings"`
	ConfirmationNote string                       `json:"confirmationNote"`
}

type mcpChangeDiffItemIn struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
	Action string `json:"action"`
}

type mcpChangeReviewCardViewModel struct {
	ToolName         string                  `json:"toolName,omitempty"`
	ApprovalTier     string                  `json:"approvalTier"`
	TierTone         string                  `json:"tierTone"`
	TargetResource   string                  `json:"targetResource"`
	ProjectID        string                  `json:"projectId,omitempty"`
	Summary          string                  `json:"summary,omitempty"`
	Changes          []mcpChangeDiffItemView `json:"changes,omitempty"`
	SensitiveKeys    []string                `json:"sensitiveKeys,omitempty"`
	Impact           []string                `json:"impact,omitempty"`
	Warnings         []string                `json:"warnings,omitempty"`
	ConfirmationNote string                  `json:"confirmationNote"`
}

type mcpChangeDiffItemView struct {
	Field     string `json:"field"`
	Before    string `json:"before"`
	After     string `json:"after"`
	HasBefore bool   `json:"hasBefore"`
	Action    string `json:"action,omitempty"`
}

type mcpInventoryCardInput struct {
	Summary string                   `json:"summary"`
	Stats   []mcpInventoryStatIn     `json:"stats"`
	Items   []mcpInventoryResourceIn `json:"items"`
}

type mcpInventoryStatIn struct {
	Label  string `json:"label"`
	Value  any    `json:"value"`
	Status string `json:"status"`
}

type mcpInventoryResourceIn struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Stage  string `json:"stage"`
	Status string `json:"status"`
	URL    string `json:"url"`
	Detail string `json:"detail"`
}

type mcpInventoryCardViewModel struct {
	Summary string                     `json:"summary,omitempty"`
	Stats   []mcpInventoryStatView     `json:"stats,omitempty"`
	Items   []mcpInventoryResourceView `json:"items,omitempty"`
}

type mcpInventoryStatView struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Status string `json:"status,omitempty"`
	Tone   string `json:"tone"`
}

type mcpInventoryResourceView struct {
	Name    string `json:"name"`
	Kind    string `json:"kind,omitempty"`
	Stage   string `json:"stage,omitempty"`
	Status  string `json:"status,omitempty"`
	Tone    string `json:"tone"`
	SafeURL string `json:"safeUrl,omitempty"`
	RawURL  string `json:"rawUrl,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type mcpTopologyCardInput struct {
	Summary     string                    `json:"summary"`
	Entrypoints []mcpTopologyEntrypointIn `json:"entrypoints"`
	Nodes       []mcpTopologyNodeIn       `json:"nodes"`
	Bindings    []mcpTopologyBindingIn    `json:"bindings"`
}

type mcpTopologyEntrypointIn struct {
	Host          string `json:"host"`
	Path          string `json:"path"`
	Protocol      string `json:"protocol"`
	TLSStatus     string `json:"tlsStatus"`
	TargetService string `json:"targetService"`
	URL           string `json:"url"`
	Status        string `json:"status"`
}

type mcpTopologyNodeIn struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Stage    string `json:"stage"`
	Status   string `json:"status"`
	Replicas any    `json:"replicas"`
	Detail   string `json:"detail"`
}

type mcpTopologyBindingIn struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Protocol  string `json:"protocol"`
	Port      int    `json:"port"`
	Injection string `json:"injection"`
	Status    string `json:"status"`
}

type mcpTopologyCardViewModel struct {
	Summary     string                      `json:"summary,omitempty"`
	Entrypoints []mcpTopologyEntrypointView `json:"entrypoints,omitempty"`
	Nodes       []mcpTopologyNodeView       `json:"nodes,omitempty"`
	Bindings    []mcpTopologyBindingView    `json:"bindings,omitempty"`
}

type mcpTopologyEntrypointView struct {
	RouteLabel    string `json:"routeLabel"`
	Protocol      string `json:"protocol,omitempty"`
	TLSStatus     string `json:"tlsStatus,omitempty"`
	TargetService string `json:"targetService"`
	SafeURL       string `json:"safeUrl,omitempty"`
	Status        string `json:"status,omitempty"`
	Tone          string `json:"tone"`
}

type mcpTopologyNodeView struct {
	Name     string `json:"name"`
	Kind     string `json:"kind,omitempty"`
	Stage    string `json:"stage,omitempty"`
	Status   string `json:"status,omitempty"`
	Tone     string `json:"tone"`
	Replicas string `json:"replicas,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type mcpTopologyBindingView struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Endpoint  string `json:"endpoint,omitempty"`
	Injection string `json:"injection,omitempty"`
	Status    string `json:"status,omitempty"`
	Tone      string `json:"tone"`
}

func mcpFlexibleDiffValueSchema() map[string]any {
	return map[string]any{
		"anyOf": []map[string]any{
			{"type": "string"},
			{"type": "number"},
			{"type": "boolean"},
			{"type": "array", "items": map[string]any{"type": "string"}},
			{"type": "object"},
		},
	}
}

func mcpRenderCardToolDefinition() map[string]any {
	return map[string]any{
		"name":        "render_card",
		"description": "将已查询到的部署诊断快照、L2 变更预览 Diff、资源与网关盘点或服务依赖拓扑渲染为自包含的只读 HTML 卡片（同时兼容 MCP Apps ui:// 视图与纯 HTML 字符串返回）。纯只读展示工具（L0），不执行任何写操作，也不能跳过 preview_tool。",
		"annotations": map[string]any{
			"title":           "渲染只读状态卡片",
			"readOnlyHint":    true,
			"destructiveHint": false,
			"idempotentHint":  true,
			"openWorldHint":   false,
		},
		"_meta": map[string]any{
			"ui": map[string]any{
				"resourceUri": mcpCardViewerResourceURI,
			},
			"ui/resourceUri": mcpCardViewerResourceURI,
		},
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"template", "title", "data"},
			"properties": map[string]any{
				"template": map[string]any{
					"type":        "string",
					"enum":        allowedMCPCardTemplates,
					"description": "只读卡片模板类型：diagnostics（部署诊断快照）、change_review（变更预览对比，可直接传入 preview_tool 返回的 approvalPreview 字段）、inventory（项目/应用/路由盘点）、topology（入口与服务依赖拓扑）。",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "卡片主标题。",
				},
				"subtitle": map[string]any{
					"type":        "string",
					"description": "可选副标题（如项目空间、集群名或统计范围）。",
				},
				"locale": map[string]any{
					"type":        "string",
					"enum":        []string{"zh", "en"},
					"description": "卡片界面语言（默认跟随当前用户语言偏好）。",
				},
				"data": map[string]any{
					"type":        "object",
					"description": "对应所选 template 的只读结构化数据（仅传当前模板所属字段）。",
					"properties": map[string]any{
						"status":      map[string]any{"type": "string", "description": "[diagnostics] 总体状态标识（如 healthy/degraded/failed/unavailable）。"},
						"statusLabel": map[string]any{"type": "string", "description": "[diagnostics] 状态徽章文本（如 Healthy / CrashLoopBackOff）。"},
						"target":      map[string]any{"type": "string", "description": "[diagnostics] 部署目标定位（如 project/app/stage）。"},
						"cluster":     map[string]any{"type": "string", "description": "[diagnostics] 所属运行集群名。"},
						"summary":     map[string]any{"type": "string", "description": "[通用] 简明根因结论或概览摘要。"},
						"replicas": map[string]any{
							"description": "[diagnostics] Pod 副本与重启统计（支持对象、简短字符串如 '2/2' 或整数）。",
							"anyOf": []map[string]any{
								{
									"type": "object",
									"properties": map[string]any{
										"ready":     map[string]any{"type": "integer"},
										"desired":   map[string]any{"type": "integer"},
										"updated":   map[string]any{"type": "integer"},
										"available": map[string]any{"type": "integer"},
										"restarts":  map[string]any{"type": "integer"},
									},
								},
								{"type": "string"},
								{"type": "integer"},
							},
						},
						"cpu": map[string]any{
							"type":        "object",
							"description": "[diagnostics] CPU 实时占用。",
							"properties": map[string]any{
								"usage":   map[string]any{"type": "string", "description": "如 '420m'"},
								"limit":   map[string]any{"type": "string", "description": "如 '1000m'"},
								"percent": map[string]any{"type": "number", "minimum": 0, "maximum": 100},
							},
						},
						"memory": map[string]any{
							"type":        "object",
							"description": "[diagnostics] 内存实时占用。",
							"properties": map[string]any{
								"usage":   map[string]any{"type": "string", "description": "如 '480Mi'"},
								"limit":   map[string]any{"type": "string", "description": "如 '512Mi'"},
								"percent": map[string]any{"type": "number", "minimum": 0, "maximum": 100},
							},
						},
						"pvcs": map[string]any{
							"type":        "array",
							"description": "[diagnostics] 挂载 PVC 状态列表。",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name":      map[string]any{"type": "string"},
									"status":    map[string]any{"type": "string"},
									"capacity":  map[string]any{"type": "string"},
									"mountPath": map[string]any{"type": "string"},
								},
							},
						},
						"warnings": map[string]any{
							"type":        "array",
							"description": "[diagnostics | change_review] 支持风险提示字符串或事件对象 {reason, pod, object, message, count, lastSeen}。",
							"items": map[string]any{
								"anyOf": []map[string]any{
									{"type": "string"},
									{
										"type": "object",
										"properties": map[string]any{
											"reason":   map[string]any{"type": "string"},
											"pod":      map[string]any{"type": "string"},
											"object":   map[string]any{"type": "string"},
											"message":  map[string]any{"type": "string"},
											"count":    map[string]any{"type": "integer"},
											"lastSeen": map[string]any{"type": "string"},
										},
									},
								},
							},
						},
						"toolName":       map[string]any{"type": "string", "description": "[change_review] 预检目标工具名。"},
						"approvalTier":   map[string]any{"type": "string", "description": "[change_review] 风险等级（如 L2 / L1）。"},
						"targetResource": map[string]any{"type": "string", "description": "[change_review] 目标资源标识（必填，或由 targetId 兜底）。"},
						"targetId":       map[string]any{"type": "string", "description": "[change_review] 可选目标资源 ID（兼容 preview_tool approvalPreview）。"},
						"projectId":      map[string]any{"type": "string", "description": "[change_review] 可选项目空间 ID（兼容 preview_tool approvalPreview）。"},
						"changes": map[string]any{
							"type":        "array",
							"description": "[change_review] before -> after 字段差异列表。",
							"items": map[string]any{
								"type":     "object",
								"required": []string{"field"},
								"properties": map[string]any{
									"field":  map[string]any{"type": "string"},
									"before": mcpFlexibleDiffValueSchema(),
									"after":  mcpFlexibleDiffValueSchema(),
									"action": map[string]any{"type": "string"},
								},
							},
						},
						"sensitiveKeys":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "[change_review] 涉及的敏感键名列表（严禁传明文值）。"},
						"impact":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "[change_review] 影响范围说明列表。"},
						"confirmationNote": map[string]any{"type": "string", "description": "[change_review] 底部只读确认说明。"},
						"stats": map[string]any{
							"type":        "array",
							"description": "[inventory] 概览统计指标卡列表。",
							"items": map[string]any{
								"type":     "object",
								"required": []string{"label", "value"},
								"properties": map[string]any{
									"label": map[string]any{"type": "string"},
									"value": map[string]any{
										"anyOf": []map[string]any{
											{"type": "string"},
											{"type": "number"},
											{"type": "boolean"},
										},
									},
									"status": map[string]any{"type": "string"},
								},
							},
						},
						"items": map[string]any{
							"type":        "array",
							"description": "[inventory] 资源状态与访问地址条目列表。",
							"items": map[string]any{
								"type":     "object",
								"required": []string{"name"},
								"properties": map[string]any{
									"name":   map[string]any{"type": "string"},
									"kind":   map[string]any{"type": "string"},
									"stage":  map[string]any{"type": "string"},
									"status": map[string]any{"type": "string"},
									"url":    map[string]any{"type": "string"},
									"detail": map[string]any{"type": "string"},
								},
							},
						},
						"entrypoints": map[string]any{
							"type":        "array",
							"description": "[topology] 上游网关路由入口列表。",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"host":          map[string]any{"type": "string"},
									"path":          map[string]any{"type": "string"},
									"protocol":      map[string]any{"type": "string"},
									"tlsStatus":     map[string]any{"type": "string"},
									"targetService": map[string]any{"type": "string"},
									"url":           map[string]any{"type": "string"},
									"status":        map[string]any{"type": "string"},
								},
							},
						},
						"nodes": map[string]any{
							"type":        "array",
							"description": "[topology] 服务与中间件节点列表。",
							"items": map[string]any{
								"type":     "object",
								"required": []string{"name"},
								"properties": map[string]any{
									"name":     map[string]any{"type": "string"},
									"kind":     map[string]any{"type": "string"},
									"stage":    map[string]any{"type": "string"},
									"status": map[string]any{"type": "string"},
									"replicas": map[string]any{
										"anyOf": []map[string]any{
											{"type": "string"},
											{"type": "integer"},
											{
												"type": "object",
												"properties": map[string]any{
													"ready":   map[string]any{"type": "integer"},
													"desired": map[string]any{"type": "integer"},
												},
											},
										},
									},
									"detail": map[string]any{"type": "string"},
								},
							},
						},
						"bindings": map[string]any{
							"type":        "array",
							"description": "[topology] ServiceBinding 服务依赖关系列表。",
							"items": map[string]any{
								"type":     "object",
								"required": []string{"from", "to"},
								"properties": map[string]any{
									"from":      map[string]any{"type": "string"},
									"to":        map[string]any{"type": "string"},
									"protocol":  map[string]any{"type": "string"},
									"port":      map[string]any{"type": "integer"},
									"injection": map[string]any{"type": "string"},
									"status":    map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
			"additionalProperties": false,
		},
	}
}

func mcpCardViewerUIMeta() map[string]any {
	return map[string]any{
		"ui": map[string]any{
			"prefersBorder": false,
			"csp": map[string]any{
				"connectDomains":  []string{},
				"resourceDomains": []string{},
			},
		},
	}
}

func mcpCardViewerResourceMetadata() map[string]any {
	return map[string]any{
		"uri":         mcpCardViewerResourceURI,
		"name":        "luna-devops-card-viewer",
		"description": "Luna DevOps 只读状态与变更预览卡片视图（MCP Apps 兼容，零外部依赖）",
		"mimeType":    mcpAppHTMLMimeType,
		"_meta":       mcpCardViewerUIMeta(),
	}
}

func (h *Handler) mcpCallRenderCard(user model.User, rawArgs json.RawMessage) mcpCallToolResult {
	if len(rawArgs) > maxMCPCardPayloadBytes {
		return mcpToolErrorResult(aitool.BadRequest("ai.tool_arguments_invalid", "render_card arguments exceed maximum size (128 KiB)"))
	}
	var req mcpRenderCardRequest
	allowed := map[string]bool{
		"template": true,
		"title":    true,
		"subtitle": true,
		"locale":   true,
		"data":     true,
	}
	if err := decodeStrictJSONObject(rawArgs, &req, allowed); err != nil {
		return mcpToolErrorResult(err)
	}
	effectiveLocale := strings.TrimSpace(req.Locale)
	if effectiveLocale == "" {
		effectiveLocale = user.Language
	}
	card, err := buildNormalizedMCPCard(req, effectiveLocale)
	if err != nil {
		return mcpToolErrorResult(err)
	}
	renderedHTML, err := renderStandaloneMCPCardHTML(card)
	if err != nil {
		return mcpToolErrorResult(aitool.NewError(500, "ai.card_render_failed", "failed to render HTML card"))
	}

	structured := map[string]any{
		"template":    card.Template,
		"title":       card.Title,
		"locale":      card.Locale,
		"readOnly":    true,
		"resourceUri": mcpCardViewerResourceURI,
		"html":        renderedHTML,
		"card":        card,
	}
	if card.Subtitle != "" {
		structured["subtitle"] = card.Subtitle
	}
	// Content returns the raw self-contained HTML string directly (zero duplicate card tree or \u003c JSON-escape bloat)
	// so non-MCP-Apps desktop clients that only read content[0].text can render the HTML artifact directly,
	// while MCP Apps hosts render structuredContent.card via ui://luna-devops/card-viewer.html.
	return mcpCallToolResult{
		Content:           []mcpContentBlock{{Type: "text", Text: renderedHTML}},
		StructuredContent: structured,
		IsError:           false,
	}
}

func buildNormalizedMCPCard(req mcpRenderCardRequest, locale string) (mcpNormalizedCard, error) {
	tpl := strings.TrimSpace(req.Template)
	title := scrubInlineCredentials(clampCardText(req.Title))
	if title == "" {
		return mcpNormalizedCard{}, aitool.BadRequest("ai.tool_arguments_invalid", "title is required")
	}
	if len(bytes.TrimSpace(req.Data)) == 0 || string(bytes.TrimSpace(req.Data)) == "null" {
		return mcpNormalizedCard{}, aitool.BadRequest("ai.tool_arguments_invalid", "data must be a non-empty JSON object")
	}
	normLocale := docs.NormalizeLocale(locale)
	labels := cardLabelsForLocale(normLocale)

	card := mcpNormalizedCard{
		Template: tpl,
		Locale:   normLocale,
		Labels:   labels,
		Title:    title,
		Subtitle: scrubInlineCredentials(clampCardText(req.Subtitle)),
	}

	switch tpl {
	case "diagnostics":
		card.TemplateTag = "DIAGNOSTICS"
		var in mcpDiagnosticsCardInput
		hint := "diagnostics data supports: status, statusLabel, target, cluster, summary, replicas ({ready, desired, updated, available, restarts} or string), cpu ({usage, limit, percent}), memory ({usage, limit, percent}), pvcs ([{name, status, capacity, mountPath}]), warnings ([{reason, pod, object, message, count, lastSeen}] or [string])"
		if err := decodeStrictTypedStruct(req.Data, &in, hint); err != nil {
			return mcpNormalizedCard{}, err
		}
		vm, err := normalizeDiagnosticsCard(in, labels)
		if err != nil {
			return mcpNormalizedCard{}, err
		}
		card.Diagnostics = &vm
	case "change_review":
		card.TemplateTag = "CHANGE REVIEW · READ-ONLY"
		var in mcpChangeReviewCardInput
		hint := "change_review data supports: targetResource (required unless targetId is set), targetId, projectId, toolName, approvalTier, summary, changes ([{field, before, after, action}]), sensitiveKeys ([string]), impact ([string]), warnings ([string]), confirmationNote"
		if err := decodeStrictTypedStruct(req.Data, &in, hint); err != nil {
			return mcpNormalizedCard{}, err
		}
		vm, err := normalizeChangeReviewCard(in, labels)
		if err != nil {
			return mcpNormalizedCard{}, err
		}
		card.ChangeReview = &vm
	case "inventory":
		card.TemplateTag = "INVENTORY"
		var in mcpInventoryCardInput
		hint := "inventory data supports: summary, stats ([{label, value, status}]), items ([{name, kind, stage, status, url, detail}])"
		if err := decodeStrictTypedStruct(req.Data, &in, hint); err != nil {
			return mcpNormalizedCard{}, err
		}
		vm, err := normalizeInventoryCard(in)
		if err != nil {
			return mcpNormalizedCard{}, err
		}
		card.Inventory = &vm
	case "topology":
		card.TemplateTag = "TOPOLOGY"
		var in mcpTopologyCardInput
		hint := "topology data supports: summary, entrypoints ([{host, path, protocol, tlsStatus, targetService, url, status}]), nodes ([{name, kind, stage, status, replicas, detail}]), bindings ([{from, to, protocol, port, injection, status}])"
		if err := decodeStrictTypedStruct(req.Data, &in, hint); err != nil {
			return mcpNormalizedCard{}, err
		}
		vm, err := normalizeTopologyCard(in)
		if err != nil {
			return mcpNormalizedCard{}, err
		}
		card.Topology = &vm
	default:
		return mcpNormalizedCard{}, aitool.NewErrorWithHint(
			400,
			"ai.tool_arguments_invalid",
			fmt.Sprintf("unsupported card template %q", tpl),
			"Allowed template values: diagnostics, change_review, inventory, topology.",
		)
	}
	return card, nil
}

func decodeStrictTypedStruct(raw json.RawMessage, dst any, hint string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return aitool.NewErrorWithHint(400, "ai.tool_arguments_invalid", "data must be a JSON object", hint)
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return aitool.NewErrorWithHint(400, "ai.tool_arguments_invalid", "invalid card data fields: "+err.Error(), hint)
	}
	if dec.More() {
		return aitool.NewErrorWithHint(400, "ai.tool_arguments_invalid", "unexpected trailing JSON data", hint)
	}
	return nil
}

func normalizeDiagnosticsCard(in mcpDiagnosticsCardInput, labels mcpCardLabels) (mcpDiagnosticsCardViewModel, error) {
	status := clampCardText(in.Status)
	rawLabel := clampCardText(in.StatusLabel)
	label := firstNonEmpty(rawLabel, status, "observed")
	vm := mcpDiagnosticsCardViewModel{
		Status:      status,
		StatusLabel: label,
		StatusTone:  classifyStatusTone(firstNonEmpty(status, rawLabel)),
		Target:      clampCardText(in.Target),
		Cluster:     clampCardText(in.Cluster),
		Summary:     scrubInlineCredentials(clampCardText(in.Summary)),
	}
	if in.Replicas != nil {
		if in.Replicas.IsObject {
			ready := max(0, in.Replicas.Ready)
			desired := max(0, in.Replicas.Desired)
			restarts := max(0, in.Replicas.Restarts)
			tone := "success"
			if desired > 0 && ready == 0 {
				tone = "danger"
			} else if ready < desired || restarts > 0 {
				tone = "warning"
			}
			var extras []string
			if in.Replicas.Updated != nil {
				extras = append(extras, fmt.Sprintf("%s%d", labels.UpdatedPrefix, max(0, *in.Replicas.Updated)))
			}
			if in.Replicas.Available != nil {
				extras = append(extras, fmt.Sprintf("%s%d", labels.AvailablePrefix, max(0, *in.Replicas.Available)))
			}
			vm.Replicas = &mcpDiagnosticsReplicasView{
				ReadyText:   fmt.Sprintf("%d / %d", ready, desired),
				Restarts:    restarts,
				HasRestarts: true,
				ExtraDetail: strings.Join(extras, " · "),
				Tone:        tone,
			}
		} else if txt := clampCardText(in.Replicas.Text); txt != "" {
			vm.Replicas = &mcpDiagnosticsReplicasView{
				ReadyText: txt,
				Tone:      "neutral",
			}
		}
	}
	if in.CPU != nil {
		m := normalizeMeter("CPU", *in.CPU)
		vm.CPU = &m
	}
	if in.Memory != nil {
		m := normalizeMeter(labels.Memory, *in.Memory)
		vm.Memory = &m
	}
	for i, pvc := range in.PVCs {
		if i >= maxMCPCardListItems {
			break
		}
		name := clampCardText(pvc.Name)
		if name == "" {
			continue
		}
		st := firstNonEmpty(clampCardText(pvc.Status), "Unknown")
		vm.PVCs = append(vm.PVCs, mcpDiagnosticsPVCView{
			Name:      name,
			Status:    st,
			Tone:      classifyStatusTone(st),
			Capacity:  clampCardText(pvc.Capacity),
			MountPath: clampCardText(pvc.MountPath),
		})
	}
	for i, w := range in.Warnings {
		if i >= maxMCPCardListItems {
			break
		}
		msg := scrubInlineCredentials(clampCardText(w.Message))
		if msg == "" {
			continue
		}
		vm.Warnings = append(vm.Warnings, mcpDiagnosticsWarningView{
			Reason:   clampCardText(w.Reason),
			Source:   firstNonEmpty(clampCardText(w.Pod), clampCardText(w.Object)),
			Message:  msg,
			Count:    max(0, w.Count),
			LastSeen: clampCardText(w.LastSeen),
		})
	}
	if status == "" && rawLabel == "" && vm.Target == "" && vm.Summary == "" && vm.Replicas == nil && vm.CPU == nil && vm.Memory == nil && len(vm.PVCs) == 0 && len(vm.Warnings) == 0 {
		return mcpDiagnosticsCardViewModel{}, aitool.BadRequest("ai.tool_arguments_invalid", "diagnostics data requires at least one non-empty status, metric, pvc, warning, or summary field")
	}
	return vm, nil
}

func normalizeMeter(label string, in mcpDiagnosticsMeterIn) mcpDiagnosticsMeterView {
	usage := firstNonEmpty(clampCardText(in.Usage), "-")
	limit := clampCardText(in.Limit)
	valText := usage
	if limit != "" {
		valText = usage + " / " + limit
	}
	pctInt := 0
	hasPct := false
	tone := "info"
	if in.Percent != nil && !math.IsNaN(*in.Percent) && !math.IsInf(*in.Percent, 0) {
		hasPct = true
		clamped := math.Max(0, math.Min(100, math.Round(*in.Percent)))
		pctInt = int(clamped)
		if pctInt >= 90 {
			tone = "danger"
		} else if pctInt >= 75 {
			tone = "warning"
		} else {
			tone = "success"
		}
	}
	return mcpDiagnosticsMeterView{
		Label:      label,
		ValueText:  valText,
		PercentInt: pctInt,
		HasPercent: hasPct,
		Tone:       tone,
	}
}

func normalizeChangeReviewCard(in mcpChangeReviewCardInput, labels mcpCardLabels) (mcpChangeReviewCardViewModel, error) {
	target := firstNonEmpty(clampCardText(in.TargetResource), clampCardText(in.TargetID))
	if target == "" {
		return mcpChangeReviewCardViewModel{}, aitool.NewErrorWithHint(
			400,
			"ai.tool_arguments_invalid",
			"change_review data.targetResource (or data.targetId) is required",
			"Pass the target resource locator (for example 'alpha-svc/api/prod') in data.targetResource.",
		)
	}
	projectID := clampCardText(in.ProjectID)
	if projectID != "" && (target == projectID || strings.HasPrefix(target, projectID+"/")) {
		projectID = ""
	}
	tier := strings.ToUpper(strings.TrimSpace(in.ApprovalTier))
	if tier == "" {
		tier = "L2"
	}
	tierTone := "warning"
	if tier == "L1" {
		tierTone = "info"
	}
	note := scrubInlineCredentials(clampCardText(in.ConfirmationNote))
	if note == "" {
		note = labels.DefaultConfirmNote
	}

	sensitiveKeySet := make(map[string]bool, len(in.SensitiveKeys))
	var sensitiveKeyList []string
	for i, k := range in.SensitiveKeys {
		if i >= maxMCPCardListItems {
			break
		}
		if trimmed := clampCardText(k); trimmed != "" {
			sensitiveKeyList = append(sensitiveKeyList, trimmed)
			sensitiveKeySet[strings.ToLower(trimmed)] = true
		}
	}

	vm := mcpChangeReviewCardViewModel{
		ToolName:         clampCardText(in.ToolName),
		ApprovalTier:     tier,
		TierTone:         tierTone,
		TargetResource:   target,
		ProjectID:        projectID,
		Summary:          scrubInlineCredentials(clampCardText(in.Summary)),
		SensitiveKeys:    sensitiveKeyList,
		ConfirmationNote: note,
	}
	for i, ch := range in.Changes {
		if i >= maxMCPCardListItems {
			break
		}
		field := clampCardText(ch.Field)
		if field == "" {
			continue
		}
		secretField := isCardSecretFieldName(field) || matchesSensitiveKeySet(field, sensitiveKeySet)
		hasBefore := ch.Before != nil
		beforeStr := ""
		if hasBefore {
			if secretField {
				beforeStr = "[REDACTED]"
			} else {
				beforeStr = formatCardAnyValue(ch.Before, sensitiveKeySet)
			}
		}
		afterStr := "[REDACTED]"
		if !secretField {
			afterStr = formatCardAnyValue(ch.After, sensitiveKeySet)
		}
		vm.Changes = append(vm.Changes, mcpChangeDiffItemView{
			Field:     field,
			Before:    beforeStr,
			After:     afterStr,
			HasBefore: hasBefore,
			Action:    clampCardText(ch.Action),
		})
	}
	for i, imp := range in.Impact {
		if i >= maxMCPCardListItems {
			break
		}
		if trimmed := scrubInlineCredentials(clampCardText(imp)); trimmed != "" {
			vm.Impact = append(vm.Impact, trimmed)
		}
	}
	for i, w := range in.Warnings {
		if i >= maxMCPCardListItems {
			break
		}
		text := clampCardText(w.Message)
		if r := clampCardText(w.Reason); r != "" && text != "" {
			text = r + ": " + text
		} else if r != "" {
			text = r
		}
		if trimmed := scrubInlineCredentials(text); trimmed != "" {
			vm.Warnings = append(vm.Warnings, trimmed)
		}
	}
	return vm, nil
}

func normalizeInventoryCard(in mcpInventoryCardInput) (mcpInventoryCardViewModel, error) {
	vm := mcpInventoryCardViewModel{
		Summary: scrubInlineCredentials(clampCardText(in.Summary)),
	}
	for i, st := range in.Stats {
		if i >= maxMCPCardListItems {
			break
		}
		label := clampCardText(st.Label)
		if label == "" {
			continue
		}
		status := clampCardText(st.Status)
		vm.Stats = append(vm.Stats, mcpInventoryStatView{
			Label:  label,
			Value:  formatCardAnyValue(st.Value, nil),
			Status: status,
			Tone:   classifyStatusTone(status),
		})
	}
	for i, item := range in.Items {
		if i >= maxMCPCardListItems {
			break
		}
		name := clampCardText(item.Name)
		if name == "" {
			continue
		}
		safeURL, rawURL := sanitizeCardHTTPURL(item.URL)
		status := clampCardText(item.Status)
		vm.Items = append(vm.Items, mcpInventoryResourceView{
			Name:    name,
			Kind:    clampCardText(item.Kind),
			Stage:   clampCardText(item.Stage),
			Status:  status,
			Tone:    classifyStatusTone(status),
			SafeURL: safeURL,
			RawURL:  rawURL,
			Detail:  scrubInlineCredentials(clampCardText(item.Detail)),
		})
	}
	if len(vm.Stats) == 0 && len(vm.Items) == 0 && vm.Summary == "" {
		return mcpInventoryCardViewModel{}, aitool.BadRequest("ai.tool_arguments_invalid", "inventory data requires at least one of stats, items, or summary")
	}
	return vm, nil
}

func normalizeTopologyCard(in mcpTopologyCardInput) (mcpTopologyCardViewModel, error) {
	vm := mcpTopologyCardViewModel{
		Summary: scrubInlineCredentials(clampCardText(in.Summary)),
	}
	for i, ep := range in.Entrypoints {
		if i >= maxMCPCardListItems {
			break
		}
		host := clampCardText(ep.Host)
		targetSvc := clampCardText(ep.TargetService)
		if host == "" && targetSvc == "" {
			continue
		}
		path := clampCardText(ep.Path)
		routeLabel := host
		if path != "" {
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			if host != "" {
				routeLabel = host + path
			} else {
				routeLabel = path
			}
		}
		safeURL, _ := sanitizeCardHTTPURL(ep.URL)
		status := firstNonEmpty(clampCardText(ep.Status), clampCardText(ep.TLSStatus))
		vm.Entrypoints = append(vm.Entrypoints, mcpTopologyEntrypointView{
			RouteLabel:    firstNonEmpty(routeLabel, safeURL, "-"),
			Protocol:      strings.ToUpper(clampCardText(ep.Protocol)),
			TLSStatus:     clampCardText(ep.TLSStatus),
			TargetService: firstNonEmpty(targetSvc, "-"),
			SafeURL:       safeURL,
			Status:        status,
			Tone:          classifyStatusTone(status),
		})
	}
	for i, node := range in.Nodes {
		if i >= maxMCPCardListItems {
			break
		}
		name := clampCardText(node.Name)
		if name == "" {
			continue
		}
		status := clampCardText(node.Status)
		replicasStr := ""
		if node.Replicas != nil {
			replicasStr = formatTopologyNodeReplicas(node.Replicas)
		}
		vm.Nodes = append(vm.Nodes, mcpTopologyNodeView{
			Name:     name,
			Kind:     clampCardText(node.Kind),
			Stage:    clampCardText(node.Stage),
			Status:   status,
			Tone:     classifyStatusTone(status),
			Replicas: replicasStr,
			Detail:   scrubInlineCredentials(clampCardText(node.Detail)),
		})
	}
	for i, b := range in.Bindings {
		if i >= maxMCPCardListItems {
			break
		}
		from := clampCardText(b.From)
		to := clampCardText(b.To)
		if from == "" || to == "" {
			continue
		}
		var epParts []string
		if proto := strings.ToUpper(clampCardText(b.Protocol)); proto != "" {
			epParts = append(epParts, proto)
		}
		if b.Port > 0 && b.Port <= 65535 {
			epParts = append(epParts, fmt.Sprintf(":%d", b.Port))
		}
		status := clampCardText(b.Status)
		vm.Bindings = append(vm.Bindings, mcpTopologyBindingView{
			From:      from,
			To:        to,
			Endpoint:  strings.Join(epParts, ""),
			Injection: clampCardText(b.Injection),
			Status:    status,
			Tone:      classifyStatusTone(status),
		})
	}
	if len(vm.Entrypoints) == 0 && len(vm.Nodes) == 0 && len(vm.Bindings) == 0 && vm.Summary == "" {
		return mcpTopologyCardViewModel{}, aitool.BadRequest("ai.tool_arguments_invalid", "topology data requires at least one of entrypoints, nodes, bindings, or summary")
	}
	return vm, nil
}

func formatTopologyNodeReplicas(raw any) string {
	if m, ok := raw.(map[string]any); ok {
		ready, hasReady := m["ready"]
		desired, hasDesired := m["desired"]
		if hasReady && hasDesired {
			return fmt.Sprintf("%s/%s", formatCardAnyValue(ready, nil), formatCardAnyValue(desired, nil))
		}
	}
	return formatCardAnyValue(raw, nil)
}

func clampCardText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) > maxMCPCardTextRunes {
		return string(runes[:maxMCPCardTextRunes]) + "…"
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func matchesSensitiveKeySet(field string, sensitiveKeys map[string]bool) bool {
	if len(sensitiveKeys) == 0 {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(field))
	if sensitiveKeys[lower] {
		return true
	}
	for _, part := range strings.FieldsFunc(lower, func(r rune) bool {
		return r == '.' || r == '[' || r == ']' || r == '/'
	}) {
		if sensitiveKeys[part] {
			return true
		}
	}
	return false
}

func redactNestedValue(v any, sensitiveKeys map[string]bool) any {
	switch val := v.(type) {
	case string:
		return scrubInlineCredentials(val)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			if isCardSecretFieldName(k) || matchesSensitiveKeySet(k, sensitiveKeys) {
				out[k] = "[REDACTED]"
			} else {
				out[k] = redactNestedValue(item, sensitiveKeys)
			}
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = redactNestedValue(item, sensitiveKeys)
		}
		return out
	default:
		return v
	}
}

func formatCardAnyValue(v any, sensitiveKeys map[string]bool) string {
	if v == nil {
		return "-"
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return `""`
		}
		return scrubInlineCredentials(clampCardText(val))
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		if val == math.Trunc(val) && math.Abs(val) < 1e15 {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%g", val)
	default:
		sanitized := redactNestedValue(val, sensitiveKeys)
		encoded, err := json.Marshal(sanitized)
		if err != nil {
			return scrubInlineCredentials(clampCardText(fmt.Sprintf("%v", sanitized)))
		}
		return clampCardText(string(encoded))
	}
}

func isCardSecretFieldName(field string) bool {
	lower := strings.ToLower(strings.TrimSpace(field))
	for _, token := range []string{
		"secret",
		"password",
		"passwd",
		"token",
		"privatekey",
		"private_key",
		"apikey",
		"api_key",
		"credential",
		"kubeconfig",
		"dsn",
		"bearer",
		"authorization",
		"cookie",
		"signing",
		"signsecret",
		"sign_secret",
		"sshkey",
		"ssh_key",
		"certkey",
		"cert_key",
		"tlskey",
		"tls_key",
		"access_key",
		"accesskey",
		"webhookurl",
		"webhook_url",
	} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func isSensitiveQueryParam(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	switch lower {
	case "key", "sig", "signature", "code", "state":
		return true
	}
	return isCardSecretFieldName(lower)
}

func scrubInlineCredentials(raw string) string {
	if raw == "" {
		return ""
	}
	scrubbed := inlineURLCredentialPattern.ReplaceAllString(raw, "${1}[REDACTED]@")
	return inlineQuerySecretPattern.ReplaceAllString(scrubbed, "${1}REDACTED")
}

// sanitizeCardHTTPURL allows strictly http:// or https:// URLs with a non-empty host,
// strips embedded userinfo credentials, and redacts sensitive query parameters.
func sanitizeCardHTTPURL(raw string) (safeHref string, fallbackText string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}
	if len([]rune(trimmed)) > maxMCPCardTextRunes {
		return "", scrubInlineCredentials(clampCardText(trimmed))
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" {
		return "", scrubInlineCredentials(clampCardText(trimmed))
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", scrubInlineCredentials(clampCardText(trimmed))
	}
	parsed.User = nil
	if parsed.RawQuery != "" {
		q := parsed.Query()
		mutated := false
		for k := range q {
			if isSensitiveQueryParam(k) {
				q.Set(k, "REDACTED")
				mutated = true
			}
		}
		if mutated {
			parsed.RawQuery = q.Encode()
		}
	}
	return parsed.String(), ""
}

func classifyStatusTone(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "" {
		return "neutral"
	}
	// Evaluate negated & failure states BEFORE positive substrings (e.g. NotReady before ready, Unbound before bound).
	switch {
	case strings.Contains(lower, "notready"),
		strings.Contains(lower, "not_ready"),
		strings.Contains(lower, "not ready"),
		strings.Contains(lower, "invalid"),
		strings.Contains(lower, "revoked"),
		strings.Contains(lower, "expired"),
		strings.Contains(lower, "evicted"),
		strings.Contains(lower, "backoff"),
		strings.Contains(lower, "fail"),
		strings.Contains(lower, "error"),
		strings.Contains(lower, "errimage"),
		strings.Contains(lower, "crash"),
		strings.Contains(lower, "oom"),
		strings.Contains(lower, "unhealthy"),
		strings.Contains(lower, "critical"),
		strings.Contains(lower, "danger"),
		strings.Contains(lower, "lost"),
		strings.Contains(lower, "rejected"),
		strings.Contains(lower, "异常"),
		strings.Contains(lower, "失败"),
		strings.Contains(lower, "崩溃"),
		strings.Contains(lower, "未就绪"),
		strings.Contains(lower, "已吊销"),
		strings.Contains(lower, "已过期"):
		return "danger"
	case strings.Contains(lower, "unbound"),
		strings.Contains(lower, "inactive"),
		strings.Contains(lower, "degrad"),
		strings.Contains(lower, "warn"),
		strings.Contains(lower, "pending"),
		strings.Contains(lower, "unavailable"),
		strings.Contains(lower, "terminating"),
		strings.Contains(lower, "waiting"),
		strings.Contains(lower, "drifted"),
		strings.Contains(lower, "告警"),
		strings.Contains(lower, "不可达"),
		strings.Contains(lower, "未绑定"),
		strings.Contains(lower, "待"):
		return "warning"
	case strings.Contains(lower, "progress"),
		strings.Contains(lower, "running"),
		strings.Contains(lower, "building"),
		strings.Contains(lower, "deploying"),
		strings.Contains(lower, "syncing"),
		strings.Contains(lower, "发布中"),
		strings.Contains(lower, "运行中"):
		return "info"
	case lower == "ok",
		strings.Contains(lower, "health"),
		strings.Contains(lower, "ready"),
		strings.Contains(lower, "bound"),
		strings.Contains(lower, "succeed"),
		strings.Contains(lower, "active"),
		strings.Contains(lower, "issued"),
		strings.Contains(lower, "valid"),
		strings.Contains(lower, "正常"),
		strings.Contains(lower, "健康"),
		strings.Contains(lower, "已签发"):
		return "success"
	default:
		return "neutral"
	}
}

// Compact self-contained stylesheet using background-color (never background shorthand) to prevent CSS url() fetches.
const mcpCardSharedStyles = `:root{color-scheme:light dark;--ld-bg:#ffffff;--ld-surface:#f8fafc;--ld-border:#e2e8f0;--ld-text:#0f172a;--ld-muted:#64748b;--ld-mono:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;--ld-sans:var(--font-sans,-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif);--ld-ok-bg:#ecfdf5;--ld-ok-fg:#047857;--ld-ok-bd:#a7f3d0;--ld-warn-bg:#fffbeb;--ld-warn-fg:#b45309;--ld-warn-bd:#fde68a;--ld-err-bg:#fef2f2;--ld-err-fg:#b91c1c;--ld-err-bd:#fecaca;--ld-info-bg:#eff6ff;--ld-info-fg:#1d4ed8;--ld-info-bd:#bfdbfe;--ld-neu-bg:#f1f5f9;--ld-neu-fg:#475569;--ld-neu-bd:#cbd5e1}` +
	`@media (prefers-color-scheme:dark){:root:not([data-theme="light"]){--ld-bg:#0f172a;--ld-surface:#1e293b;--ld-border:#334155;--ld-text:#f8fafc;--ld-muted:#94a3b8;--ld-ok-bg:rgba(16,185,129,.14);--ld-ok-fg:#34d399;--ld-ok-bd:rgba(52,211,153,.32);--ld-warn-bg:rgba(245,158,11,.15);--ld-warn-fg:#fbbf24;--ld-warn-bd:rgba(251,191,36,.35);--ld-err-bg:rgba(239,68,68,.15);--ld-err-fg:#f87171;--ld-err-bd:rgba(248,113,113,.35);--ld-info-bg:rgba(59,130,246,.15);--ld-info-fg:#60a5fa;--ld-info-bd:rgba(96,165,250,.35);--ld-neu-bg:rgba(148,163,184,.14);--ld-neu-fg:#cbd5e1;--ld-neu-bd:rgba(148,163,184,.28)}}` +
	`:root[data-theme="dark"]{--ld-bg:#0f172a;--ld-surface:#1e293b;--ld-border:#334155;--ld-text:#f8fafc;--ld-muted:#94a3b8;--ld-ok-bg:rgba(16,185,129,.14);--ld-ok-fg:#34d399;--ld-ok-bd:rgba(52,211,153,.32);--ld-warn-bg:rgba(245,158,11,.15);--ld-warn-fg:#fbbf24;--ld-warn-bd:rgba(251,191,36,.35);--ld-err-bg:rgba(239,68,68,.15);--ld-err-fg:#f87171;--ld-err-bd:rgba(248,113,113,.35);--ld-info-bg:rgba(59,130,246,.15);--ld-info-fg:#60a5fa;--ld-info-bd:rgba(96,165,250,.35);--ld-neu-bg:rgba(148,163,184,.14);--ld-neu-fg:#cbd5e1;--ld-neu-bd:rgba(148,163,184,.28)}` +
	`*{box-sizing:border-box}html,body{margin:0;padding:0;background-color:transparent;color:var(--color-text-primary,var(--ld-text));font-family:var(--ld-sans);font-size:13px;line-height:1.5;-webkit-font-smoothing:antialiased}` +
	`.ld-card{background-color:var(--color-background-primary,var(--ld-bg));border:1px solid var(--color-border-primary,var(--ld-border));border-radius:12px;padding:14px 16px;display:grid;gap:12px;max-width:100%}` +
	`.ld-hdr{display:flex;flex-wrap:wrap;align-items:flex-start;justify-content:space-between;gap:8px}` +
	`.ld-eyebrow{font-size:10px;font-weight:600;letter-spacing:.06em;color:var(--color-text-secondary,var(--ld-muted));text-transform:uppercase}` +
	`.ld-title{margin:2px 0 0;font-size:15px;font-weight:600;line-height:1.35;word-break:break-word}` +
	`.ld-sub{margin:2px 0 0;font-size:12px;color:var(--color-text-secondary,var(--ld-muted));word-break:break-word}` +
	`.ld-summary{margin:0;padding:8px 10px;border-radius:8px;background-color:var(--color-background-secondary,var(--ld-surface));border:1px solid var(--color-border-primary,var(--ld-border));font-size:12px;word-break:break-word}` +
	`.ld-badge{display:inline-flex;align-items:center;padding:2px 8px;border-radius:999px;font-size:11px;font-weight:600;border:1px solid transparent;white-space:nowrap;font-variant-numeric:tabular-nums}` +
	`.ld-tone-success{background-color:var(--ld-ok-bg);color:var(--ld-ok-fg);border-color:var(--ld-ok-bd)}` +
	`.ld-tone-warning{background-color:var(--ld-warn-bg);color:var(--ld-warn-fg);border-color:var(--ld-warn-bd)}` +
	`.ld-tone-danger{background-color:var(--ld-err-bg);color:var(--ld-err-fg);border-color:var(--ld-err-bd)}` +
	`.ld-tone-info{background-color:var(--ld-info-bg);color:var(--ld-info-fg);border-color:var(--ld-info-bd)}` +
	`.ld-tone-neutral{background-color:var(--ld-neu-bg);color:var(--ld-neu-fg);border-color:var(--ld-neu-bd)}` +
	`.ld-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:8px}` +
	`.ld-stat{padding:8px 10px;border-radius:8px;background-color:var(--color-background-secondary,var(--ld-surface));border:1px solid var(--color-border-primary,var(--ld-border))}` +
	`.ld-stat-hdr{display:flex;align-items:center;justify-content:space-between;gap:6px}` +
	`.ld-stat-k{font-size:11px;color:var(--color-text-secondary,var(--ld-muted))}` +
	`.ld-stat-v{margin-top:2px;font-size:15px;font-weight:600;font-variant-numeric:tabular-nums;word-break:break-word}` +
	`.ld-stat-sub{margin-top:2px;font-size:11px;color:var(--color-text-secondary,var(--ld-muted))}` +
	`.ld-bar{margin-top:6px;height:6px;width:100%;border-radius:999px;background-color:var(--ld-neu-bg);overflow:hidden}` +
	`.ld-bar-fill{display:block;height:100%;border-radius:999px}` +
	`.ld-bar-success{background-color:var(--ld-ok-fg)}.ld-bar-warning{background-color:var(--ld-warn-fg)}.ld-bar-danger{background-color:var(--ld-err-fg)}.ld-bar-info{background-color:var(--ld-info-fg)}` +
	`.ld-sec{display:grid;gap:6px}.ld-sec-title{font-size:11px;font-weight:600;color:var(--color-text-secondary,var(--ld-muted))}` +
	`.ld-table-wrap{overflow-x:auto;border:1px solid var(--color-border-primary,var(--ld-border));border-radius:8px}` +
	`table.ld-table{width:100%;border-collapse:collapse;font-size:12px}` +
	`.ld-table th,.ld-table td{padding:6px 10px;text-align:left;border-bottom:1px solid var(--color-border-primary,var(--ld-border));vertical-align:top;word-break:break-word}` +
	`.ld-table tr:last-child td{border-bottom:none}` +
	`.ld-table th{background-color:var(--color-background-secondary,var(--ld-surface));font-size:11px;font-weight:600;color:var(--color-text-secondary,var(--ld-muted))}` +
	`.ld-mono{font-family:var(--ld-mono);font-size:11.5px}` +
	`.ld-callout{padding:8px 10px;border-radius:8px;font-size:12px;border:1px solid var(--ld-warn-bd);background-color:var(--ld-warn-bg);color:var(--ld-warn-fg)}.ld-callout ul{margin:4px 0 0;padding-left:18px}` +
	`.ld-note{padding:8px 10px;border-radius:8px;font-size:11.5px;border:1px solid var(--ld-info-bd);background-color:var(--ld-info-bg);color:var(--ld-info-fg)}` +
	`.ld-link{color:var(--ld-info-fg);text-decoration:none;font-family:var(--ld-mono);font-size:11.5px}.ld-link:hover{text-decoration:underline}`

var standaloneMCPCardTemplate = template.Must(template.New("luna_mcp_card").Funcs(template.FuncMap{
	"joinComma": func(items []string) string {
		return strings.Join(items, ", ")
	},
}).Parse(standaloneMCPCardSource))

const standaloneMCPCardSource = `<!DOCTYPE html>
<html lang="{{.Labels.HTMLTag}}">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} - Luna DevOps</title>
<style>` + mcpCardSharedStyles + `</style>
</head>
<body>
{{- $L := .Labels -}}
<article class="ld-card" data-luna-card-template="{{.Template}}" data-readonly="true">
  <header class="ld-hdr">
    <div>
      <div class="ld-eyebrow">Luna DevOps · {{.TemplateTag}}</div>
      <h1 class="ld-title">{{.Title}}</h1>
      {{- if .Subtitle}}<p class="ld-sub">{{.Subtitle}}</p>{{end}}
    </div>
    {{- if .Diagnostics}}
    <span class="ld-badge ld-tone-{{.Diagnostics.StatusTone}}">{{.Diagnostics.StatusLabel}}</span>
    {{- end}}
    {{- if .ChangeReview}}
    <span class="ld-badge ld-tone-{{.ChangeReview.TierTone}}">{{.ChangeReview.ApprovalTier}}</span>
    {{- end}}
  </header>

  {{- if .Diagnostics}}
  {{- with .Diagnostics}}
  {{- if or .Target .Cluster}}
  <div class="ld-summary">
    {{- if .Target}}<span>{{$L.Target}}<code class="ld-mono">{{.Target}}</code></span>{{end}}
    {{- if and .Target .Cluster}} · {{end}}
    {{- if .Cluster}}<span>{{$L.Cluster}}<code class="ld-mono">{{.Cluster}}</code></span>{{end}}
  </div>
  {{- end}}
  {{- if .Summary}}<p class="ld-summary">{{.Summary}}</p>{{end}}
  {{- if or .Replicas .CPU .Memory}}
  <div class="ld-grid">
    {{- if .Replicas}}
    <div class="ld-stat">
      <div class="ld-stat-k">{{$L.ReplicasAndRestarts}}</div>
      <div class="ld-stat-v">{{.Replicas.ReadyText}}{{if .Replicas.HasRestarts}} <span class="ld-badge ld-tone-{{.Replicas.Tone}}">{{$L.RestartsPrefix}}{{.Replicas.Restarts}}</span>{{end}}</div>
      {{- if .Replicas.ExtraDetail}}<div class="ld-stat-sub">{{.Replicas.ExtraDetail}}</div>{{end}}
    </div>
    {{- end}}
    {{- if .CPU}}
    <div class="ld-stat">
      <div class="ld-stat-k">{{.CPU.Label}}{{if .CPU.HasPercent}} ({{.CPU.PercentInt}}%){{end}}</div>
      <div class="ld-stat-v ld-mono">{{.CPU.ValueText}}</div>
      {{- if .CPU.HasPercent}}<div class="ld-bar"><span class="ld-bar-fill ld-bar-{{.CPU.Tone}}" style="width:{{.CPU.PercentInt}}%"></span></div>{{end}}
    </div>
    {{- end}}
    {{- if .Memory}}
    <div class="ld-stat">
      <div class="ld-stat-k">{{.Memory.Label}}{{if .Memory.HasPercent}} ({{.Memory.PercentInt}}%){{end}}</div>
      <div class="ld-stat-v ld-mono">{{.Memory.ValueText}}</div>
      {{- if .Memory.HasPercent}}<div class="ld-bar"><span class="ld-bar-fill ld-bar-{{.Memory.Tone}}" style="width:{{.Memory.PercentInt}}%"></span></div>{{end}}
    </div>
    {{- end}}
  </div>
  {{- end}}
  {{- if .PVCs}}
  <section class="ld-sec">
    <div class="ld-sec-title">{{$L.PVCSection}}</div>
    <div class="ld-table-wrap">
      <table class="ld-table">
        <thead><tr><th>{{$L.ColName}}</th><th>{{$L.ColStatus}}</th><th>{{$L.ColCapacity}}</th><th>{{$L.ColMountPath}}</th></tr></thead>
        <tbody>
          {{- range .PVCs}}
          <tr>
            <td class="ld-mono">{{.Name}}</td>
            <td><span class="ld-badge ld-tone-{{.Tone}}">{{.Status}}</span></td>
            <td class="ld-mono">{{if .Capacity}}{{.Capacity}}{{else}}-{{end}}</td>
            <td class="ld-mono">{{if .MountPath}}{{.MountPath}}{{else}}-{{end}}</td>
          </tr>
          {{- end}}
        </tbody>
      </table>
    </div>
  </section>
  {{- end}}
  {{- if .Warnings}}
  <section class="ld-sec">
    <div class="ld-sec-title">{{$L.WarningsSection}}</div>
    <div class="ld-table-wrap">
      <table class="ld-table">
        <thead><tr><th>{{$L.ColReasonObject}}</th><th>{{$L.ColEventMessage}}</th><th>{{$L.ColCount}}</th></tr></thead>
        <tbody>
          {{- range .Warnings}}
          <tr>
            <td class="ld-mono">{{if .Reason}}{{.Reason}}{{else}}Warning{{end}}{{if .Source}}<br>{{.Source}}{{end}}</td>
            <td>{{.Message}}</td>
            <td class="ld-mono">{{if gt .Count 0}}{{.Count}}{{else}}-{{end}}{{if .LastSeen}}<br>{{.LastSeen}}{{end}}</td>
          </tr>
          {{- end}}
        </tbody>
      </table>
    </div>
  </section>
  {{- end}}
  {{- end}}
  {{- end}}

  {{- if .ChangeReview}}
  {{- with .ChangeReview}}
  <div class="ld-summary">
    {{- if .ProjectID}}<span>{{$L.Project}}<code class="ld-mono">{{.ProjectID}}</code></span> · {{end}}
    <span>{{$L.TargetResource}}<code class="ld-mono">{{.TargetResource}}</code></span>
    {{- if .ToolName}} · <span>{{$L.Tool}}<code class="ld-mono">{{.ToolName}}</code></span>{{end}}
  </div>
  {{- if .Summary}}<p class="ld-summary">{{.Summary}}</p>{{end}}
  {{- if .Changes}}
  <section class="ld-sec">
    <div class="ld-sec-title">{{$L.DiffSection}}</div>
    <div class="ld-table-wrap">
      <table class="ld-table">
        <thead><tr><th>{{$L.ColField}}</th><th>{{$L.ColBefore}}</th><th>{{$L.ColAfter}}</th></tr></thead>
        <tbody>
          {{- range .Changes}}
          <tr>
            <td><code class="ld-mono">{{.Field}}</code>{{if .Action}} <span class="ld-badge ld-tone-neutral">{{.Action}}</span>{{end}}</td>
            <td class="ld-mono">{{if .HasBefore}}{{.Before}}{{else}}-{{end}}</td>
            <td class="ld-mono">{{.After}}</td>
          </tr>
          {{- end}}
        </tbody>
      </table>
    </div>
  </section>
  {{- end}}
  {{- if .SensitiveKeys}}
  <div class="ld-summary">{{$L.SensitiveKeysPrefix}}<code class="ld-mono">{{joinComma .SensitiveKeys}}</code></div>
  {{- end}}
  {{- if .Impact}}
  <section class="ld-sec">
    <div class="ld-sec-title">{{$L.ImpactSection}}</div>
    <div class="ld-summary">
      {{- range $idx, $line := .Impact}}{{if $idx}}<br>{{end}}• {{$line}}{{end}}
    </div>
  </section>
  {{- end}}
  {{- if .Warnings}}
  <div class="ld-callout">
    <strong>{{$L.WarningsCallout}}</strong>
    <ul>{{- range .Warnings}}<li>{{.}}</li>{{end}}</ul>
  </div>
  {{- end}}
  <div class="ld-note">{{.ConfirmationNote}}</div>
  {{- end}}
  {{- end}}

  {{- if .Inventory}}
  {{- with .Inventory}}
  {{- if .Summary}}<p class="ld-summary">{{.Summary}}</p>{{end}}
  {{- if .Stats}}
  <div class="ld-grid">
    {{- range .Stats}}
    <div class="ld-stat">
      <div class="ld-stat-hdr">
        <span class="ld-stat-k">{{.Label}}</span>
        {{- if .Status}}<span class="ld-badge ld-tone-{{.Tone}}">{{.Status}}</span>{{end}}
      </div>
      <div class="ld-stat-v">{{.Value}}</div>
    </div>
    {{- end}}
  </div>
  {{- end}}
  {{- if .Items}}
  <section class="ld-sec">
    <div class="ld-sec-title">{{$L.InventoryItemsSection}}</div>
    <div class="ld-table-wrap">
      <table class="ld-table">
        <thead><tr><th>{{$L.ColName}}</th><th>{{$L.ColKindStage}}</th><th>{{$L.ColStatus}}</th><th>{{$L.ColAccessOrDetail}}</th></tr></thead>
        <tbody>
          {{- range .Items}}
          <tr>
            <td class="ld-mono">{{.Name}}</td>
            <td>{{if .Kind}}{{.Kind}}{{else}}-{{end}}{{if .Stage}} · <code class="ld-mono">{{.Stage}}</code>{{end}}</td>
            <td>{{if .Status}}<span class="ld-badge ld-tone-{{.Tone}}">{{.Status}}</span>{{else}}-{{end}}</td>
            <td>
              {{- if .SafeURL}}<a class="ld-link" href="{{.SafeURL}}" target="_blank" rel="noopener noreferrer">{{.SafeURL}}</a>
              {{- else if .RawURL}}<code class="ld-mono">{{.RawURL}}</code>{{end}}
              {{- if .Detail}}{{if or .SafeURL .RawURL}}<br>{{end}}{{.Detail}}{{end}}
              {{- if and (not .SafeURL) (not .RawURL) (not .Detail)}}-{{end}}
            </td>
          </tr>
          {{- end}}
        </tbody>
      </table>
    </div>
  </section>
  {{- end}}
  {{- end}}
  {{- end}}

  {{- if .Topology}}
  {{- with .Topology}}
  {{- if .Summary}}<p class="ld-summary">{{.Summary}}</p>{{end}}
  {{- if .Entrypoints}}
  <section class="ld-sec">
    <div class="ld-sec-title">{{$L.TopologyEntrySection}}</div>
    <div class="ld-table-wrap">
      <table class="ld-table">
        <thead><tr><th>{{$L.ColEntryRoute}}</th><th>{{$L.ColProtocolTLS}}</th><th>{{$L.ColBackendTarget}}</th><th>{{$L.ColStatus}}</th></tr></thead>
        <tbody>
          {{- range .Entrypoints}}
          <tr>
            <td>
              {{- if .SafeURL}}<a class="ld-link" href="{{.SafeURL}}" target="_blank" rel="noopener noreferrer">{{.RouteLabel}}</a>
              {{- else}}<code class="ld-mono">{{.RouteLabel}}</code>{{end}}
            </td>
            <td class="ld-mono">{{if .Protocol}}{{.Protocol}}{{else}}HTTP{{end}}{{if .TLSStatus}} · {{.TLSStatus}}{{end}}</td>
            <td class="ld-mono">{{.TargetService}}</td>
            <td>{{if .Status}}<span class="ld-badge ld-tone-{{.Tone}}">{{.Status}}</span>{{else}}-{{end}}</td>
          </tr>
          {{- end}}
        </tbody>
      </table>
    </div>
  </section>
  {{- end}}
  {{- if .Nodes}}
  <section class="ld-sec">
    <div class="ld-sec-title">{{$L.TopologyNodesSection}}</div>
    <div class="ld-table-wrap">
      <table class="ld-table">
        <thead><tr><th>{{$L.ColNode}}</th><th>{{$L.ColKindStage}}</th><th>{{$L.ColStatus}}</th><th>{{$L.ColReplicasOrNote}}</th></tr></thead>
        <tbody>
          {{- range .Nodes}}
          <tr>
            <td class="ld-mono">{{.Name}}</td>
            <td>{{if .Kind}}{{.Kind}}{{else}}-{{end}}{{if .Stage}} · <code class="ld-mono">{{.Stage}}</code>{{end}}</td>
            <td>{{if .Status}}<span class="ld-badge ld-tone-{{.Tone}}">{{.Status}}</span>{{else}}-{{end}}</td>
            <td>{{if .Replicas}}<code class="ld-mono">{{.Replicas}}</code>{{end}}{{if .Detail}}{{if .Replicas}} · {{end}}{{.Detail}}{{end}}{{if and (not .Replicas) (not .Detail)}}-{{end}}</td>
          </tr>
          {{- end}}
        </tbody>
      </table>
    </div>
  </section>
  {{- end}}
  {{- if .Bindings}}
  <section class="ld-sec">
    <div class="ld-sec-title">{{$L.TopologyBindSection}}</div>
    <div class="ld-table-wrap">
      <table class="ld-table">
        <thead><tr><th>{{$L.ColFrom}}</th><th>{{$L.ColTo}}</th><th>{{$L.ColEndpointInjection}}</th><th>{{$L.ColStatus}}</th></tr></thead>
        <tbody>
          {{- range .Bindings}}
          <tr>
            <td class="ld-mono">{{.From}}</td>
            <td class="ld-mono">→ {{.To}}</td>
            <td class="ld-mono">{{if .Endpoint}}{{.Endpoint}}{{else}}-{{end}}{{if .Injection}} ({{.Injection}}){{end}}</td>
            <td>{{if .Status}}<span class="ld-badge ld-tone-{{.Tone}}">{{.Status}}</span>{{else}}-{{end}}</td>
          </tr>
          {{- end}}
        </tbody>
      </table>
    </div>
  </section>
  {{- end}}
  {{- end}}
  {{- end}}
</article>
</body>
</html>
`

func renderStandaloneMCPCardHTML(card mcpNormalizedCard) (string, error) {
	var buf bytes.Buffer
	if err := standaloneMCPCardTemplate.Execute(&buf, card); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// mcpCardViewerAppHTML returns the self-contained MCP Apps (SEP-1865 / io.modelcontextprotocol/ui)
// read-only viewer document served at ui://luna-devops/card-viewer.html.
// It locks CSP to default-src 'none', verifies event.source === window.parent, sanitizes CSS variables,
// handles ui/resource-teardown & error/cancel notifications, and builds DOM strictly via createElement + textContent.
func mcpCardViewerAppHTML() string {
	return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Luna DevOps Card Viewer</title>
<style>` + mcpCardSharedStyles + `</style>
</head>
<body>
<div id="root"></div>
<script>
(function () {
  "use strict";
  var root = document.getElementById("root");
  var nextRpcId = 1;
  var initReqId = nextRpcId++;
  var initialized = false;
  var hostCaps = {};
  var hostLocale = "";
  var resizeObs = null;
  var SAFE_VAR_NAME = /^--(color|font)-[a-zA-Z0-9-]{1,64}$/;
  var UNSAFE_CSS_VALUE = /url\s*\(|image-set\s*\(|src\s*\(|expression\s*\(|@import|\/\*|[;{}<>\\]/i;

  function postRpc(msg) {
    if (window.parent && window.parent !== window) {
      window.parent.postMessage(msg, "*");
    }
  }

  function safeTone(t) {
    return (t === "success" || t === "warning" || t === "danger" || t === "info") ? t : "neutral";
  }

  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined && text !== null) node.textContent = String(text);
    return node;
  }

  function safeHttpUrl(raw) {
    if (typeof raw !== "string" || !raw || raw.length > 1024) return "";
    try {
      var u = new URL(raw);
      if (!u.hostname || (u.protocol !== "http:" && u.protocol !== "https:")) return "";
      u.username = "";
      u.password = "";
      return u.toString();
    } catch (_) {
      return "";
    }
  }

  function makeLink(url, label) {
    var safe = safeHttpUrl(url);
    if (!safe) return el("code", "ld-mono", label || url || "-");
    var a = el("a", "ld-link", label || safe);
    a.href = safe;
    a.target = "_blank";
    a.rel = "noopener noreferrer";
    a.addEventListener("click", function (ev) {
      if (hostCaps && hostCaps.openLinks) {
        ev.preventDefault();
        postRpc({ jsonrpc: "2.0", id: nextRpcId++, method: "ui/open-link", params: { url: safe } });
      }
    });
    return a;
  }

  function applyHostContext(ctx) {
    if (!ctx || typeof ctx !== "object") return;
    if (ctx.theme === "dark" || ctx.theme === "light") {
      document.documentElement.setAttribute("data-theme", ctx.theme);
    }
    if (typeof ctx.locale === "string" && ctx.locale) {
      hostLocale = ctx.locale;
      document.documentElement.lang = /^en\b/i.test(ctx.locale) ? "en" : "zh-CN";
    }
    var vars = ctx.styles && ctx.styles.variables;
    if (vars && typeof vars === "object") {
      var keys = Object.keys(vars);
      for (var i = 0; i < keys.length; i++) {
        var k = keys[i];
        var v = vars[k];
        if (SAFE_VAR_NAME.test(k) && typeof v === "string" && v.length <= 200 && !UNSAFE_CSS_VALUE.test(v)) {
          document.documentElement.style.setProperty(k, v);
        }
      }
    }
  }

  function reportSize() {
    if (!root || !initialized) return;
    var rect = root.getBoundingClientRect();
    var h = Math.ceil(rect.height || document.documentElement.scrollHeight || 120);
    var w = Math.ceil(rect.width || document.documentElement.scrollWidth || 320);
    postRpc({ jsonrpc: "2.0", method: "ui/notifications/size-changed", params: { width: w, height: h } });
  }

  function buildTable(headers, rows) {
    var wrap = el("div", "ld-table-wrap");
    var tbl = el("table", "ld-table");
    var thead = document.createElement("thead");
    var hr = document.createElement("tr");
    for (var i = 0; i < headers.length; i++) hr.appendChild(el("th", "", headers[i]));
    thead.appendChild(hr);
    tbl.appendChild(thead);
    var tbody = document.createElement("tbody");
    for (var r = 0; r < rows.length; r++) {
      var tr = document.createElement("tr");
      for (var c = 0; c < rows[r].length; c++) {
        var cell = rows[r][c];
        var td = document.createElement("td");
        if (cell && cell.nodeType) {
          td.appendChild(cell);
        } else {
          td.textContent = cell === undefined || cell === null || cell === "" ? "-" : String(cell);
        }
        tr.appendChild(td);
      }
      tbody.appendChild(tr);
    }
    tbl.appendChild(tbody);
    wrap.appendChild(tbl);
    return wrap;
  }

  function isEnglish() {
    return /^en\b/i.test(hostLocale);
  }

  function renderStatusPlaceholder(titleText, detailText, tone) {
    root.textContent = "";
    var card = el("article", "ld-card");
    var hdr = el("header", "ld-hdr");
    var box = document.createElement("div");
    box.appendChild(el("div", "ld-eyebrow", "Luna DevOps"));
    box.appendChild(el("h1", "ld-title", titleText));
    hdr.appendChild(box);
    if (tone) hdr.appendChild(el("span", "ld-badge ld-tone-" + safeTone(tone), tone.toUpperCase()));
    card.appendChild(hdr);
    if (detailText) card.appendChild(el("p", "ld-summary", detailText));
    root.appendChild(card);
    reportSize();
  }

  function renderLoading(args) {
    var fallback = isEnglish() ? "Rendering card…" : "正在生成卡片…";
    renderStatusPlaceholder((args && args.title) ? args.title : fallback, "", "");
  }

  function renderCardModel(card) {
    if (!card || typeof card !== "object") return;
    var L = card.labels || {};
    if (L.htmlLang) document.documentElement.lang = L.htmlLang;
    root.textContent = "";
    var article = el("article", "ld-card");
    article.setAttribute("data-luna-card-template", card.template || "");
    article.setAttribute("data-readonly", "true");

    var hdr = el("header", "ld-hdr");
    var titleWrap = document.createElement("div");
    titleWrap.appendChild(el("div", "ld-eyebrow", "Luna DevOps · " + (card.templateTag || "CARD")));
    titleWrap.appendChild(el("h1", "ld-title", card.title || ""));
    if (card.subtitle) titleWrap.appendChild(el("p", "ld-sub", card.subtitle));
    hdr.appendChild(titleWrap);

    if (card.diagnostics) {
      hdr.appendChild(el("span", "ld-badge ld-tone-" + safeTone(card.diagnostics.statusTone), card.diagnostics.statusLabel || "observed"));
    } else if (card.changeReview) {
      hdr.appendChild(el("span", "ld-badge ld-tone-" + safeTone(card.changeReview.tierTone), card.changeReview.approvalTier || "L2"));
    }
    article.appendChild(hdr);

    if (card.diagnostics) {
      var d = card.diagnostics;
      if (d.target || d.cluster) {
        var meta = el("div", "ld-summary");
        if (d.target) { meta.appendChild(el("span", "", L.target || "目标：")); meta.appendChild(el("code", "ld-mono", d.target)); }
        if (d.target && d.cluster) meta.appendChild(document.createTextNode(" · "));
        if (d.cluster) { meta.appendChild(el("span", "", L.cluster || "集群：")); meta.appendChild(el("code", "ld-mono", d.cluster)); }
        article.appendChild(meta);
      }
      if (d.summary) article.appendChild(el("p", "ld-summary", d.summary));
      if (d.replicas || d.cpu || d.memory) {
        var grid = el("div", "ld-grid");
        if (d.replicas) {
          var rs = el("div", "ld-stat");
          rs.appendChild(el("div", "ld-stat-k", L.replicasAndRestarts || "Pod 就绪副本 / 重启"));
          var rv = el("div", "ld-stat-v", d.replicas.readyText + (d.replicas.hasRestarts ? " " : ""));
          if (d.replicas.hasRestarts) {
            rv.appendChild(el("span", "ld-badge ld-tone-" + safeTone(d.replicas.tone), (L.restartsPrefix || "重启 ") + (d.replicas.restarts || 0)));
          }
          rs.appendChild(rv);
          if (d.replicas.extraDetail) rs.appendChild(el("div", "ld-stat-sub", d.replicas.extraDetail));
          grid.appendChild(rs);
        }
        [d.cpu, d.memory].forEach(function (m) {
          if (!m) return;
          var ms = el("div", "ld-stat");
          ms.appendChild(el("div", "ld-stat-k", m.label + (m.hasPercent ? " (" + m.percentInt + "%)" : "")));
          ms.appendChild(el("div", "ld-stat-v ld-mono", m.valueText));
          if (m.hasPercent) {
            var bar = el("div", "ld-bar");
            var fill = el("span", "ld-bar-fill ld-bar-" + safeTone(m.tone));
            fill.style.width = Math.max(0, Math.min(100, Number(m.percentInt) || 0)) + "%";
            bar.appendChild(fill);
            ms.appendChild(bar);
          }
          grid.appendChild(ms);
        });
        article.appendChild(grid);
      }
      if (Array.isArray(d.pvcs) && d.pvcs.length) {
        var pvcSec = el("section", "ld-sec");
        pvcSec.appendChild(el("div", "ld-sec-title", L.pvcSection || "持久化数据卷（PVC）"));
        pvcSec.appendChild(buildTable([L.colName || "名称", L.colStatus || "状态", L.colCapacity || "容量", L.colMountPath || "挂载路径"], d.pvcs.map(function (p) {
          return [el("code", "ld-mono", p.name), el("span", "ld-badge ld-tone-" + safeTone(p.tone), p.status), el("code", "ld-mono", p.capacity || "-"), el("code", "ld-mono", p.mountPath || "-")];
        })));
        article.appendChild(pvcSec);
      }
      if (Array.isArray(d.warnings) && d.warnings.length) {
        var warnSec = el("section", "ld-sec");
        warnSec.appendChild(el("div", "ld-sec-title", L.warningsSection || "近期 Warning 事件"));
        warnSec.appendChild(buildTable([L.colReasonObject || "原因 / 对象", L.colEventMessage || "事件详情", L.colCount || "次数"], d.warnings.map(function (w) {
          return [el("code", "ld-mono", (w.reason || "Warning") + (w.source ? " (" + w.source + ")" : "")), w.message, el("code", "ld-mono", (w.count > 0 ? String(w.count) : "-") + (w.lastSeen ? " · " + w.lastSeen : ""))];
        })));
        article.appendChild(warnSec);
      }
    }

    if (card.changeReview) {
      var cr = card.changeReview;
      var crMeta = el("div", "ld-summary");
      if (cr.projectId) {
        crMeta.appendChild(el("span", "", L.project || "项目："));
        crMeta.appendChild(el("code", "ld-mono", cr.projectId));
        crMeta.appendChild(document.createTextNode(" · "));
      }
      crMeta.appendChild(el("span", "", L.targetResource || "目标资源："));
      crMeta.appendChild(el("code", "ld-mono", cr.targetResource || "-"));
      if (cr.toolName) {
        crMeta.appendChild(document.createTextNode(" · " + (L.tool || "工具：")));
        crMeta.appendChild(el("code", "ld-mono", cr.toolName));
      }
      article.appendChild(crMeta);
      if (cr.summary) article.appendChild(el("p", "ld-summary", cr.summary));
      if (Array.isArray(cr.changes) && cr.changes.length) {
        var diffSec = el("section", "ld-sec");
        diffSec.appendChild(el("div", "ld-sec-title", L.diffSection || "变更差异对比（Before → After）"));
        diffSec.appendChild(buildTable([L.colField || "字段", L.colBefore || "变更前", L.colAfter || "变更后"], cr.changes.map(function (c) {
          var fieldWrap = document.createElement("span");
          fieldWrap.appendChild(el("code", "ld-mono", c.field));
          if (c.action) {
            fieldWrap.appendChild(document.createTextNode(" "));
            fieldWrap.appendChild(el("span", "ld-badge ld-tone-neutral", c.action));
          }
          return [fieldWrap, el("code", "ld-mono", c.hasBefore ? c.before : "-"), el("code", "ld-mono", c.after)];
        })));
        article.appendChild(diffSec);
      }
      if (Array.isArray(cr.sensitiveKeys) && cr.sensitiveKeys.length) {
        var sk = el("div", "ld-summary", L.sensitiveKeysPrefix || "涉及敏感键（仅显示键名）：");
        sk.appendChild(el("code", "ld-mono", cr.sensitiveKeys.join(", ")));
        article.appendChild(sk);
      }
      if (Array.isArray(cr.impact) && cr.impact.length) {
        var impSec = el("section", "ld-sec");
        impSec.appendChild(el("div", "ld-sec-title", L.impactSection || "影响范围"));
        impSec.appendChild(el("div", "ld-summary", cr.impact.join(" · ")));
        article.appendChild(impSec);
      }
      if (Array.isArray(cr.warnings) && cr.warnings.length) {
        var box = el("div", "ld-callout");
        box.appendChild(el("strong", "", L.warningsCallout || "风险提示"));
        var ul = document.createElement("ul");
        cr.warnings.forEach(function (w) { ul.appendChild(el("li", "", w)); });
        box.appendChild(ul);
        article.appendChild(box);
      }
      if (cr.confirmationNote) article.appendChild(el("div", "ld-note", cr.confirmationNote));
    }

    if (card.inventory) {
      var inv = card.inventory;
      if (inv.summary) article.appendChild(el("p", "ld-summary", inv.summary));
      if (Array.isArray(inv.stats) && inv.stats.length) {
        var sGrid = el("div", "ld-grid");
        inv.stats.forEach(function (s) {
          var st = el("div", "ld-stat");
          var sh = el("div", "ld-stat-hdr");
          sh.appendChild(el("span", "ld-stat-k", s.label));
          if (s.status) sh.appendChild(el("span", "ld-badge ld-tone-" + safeTone(s.tone), s.status));
          st.appendChild(sh);
          st.appendChild(el("div", "ld-stat-v", s.value));
          sGrid.appendChild(st);
        });
        article.appendChild(sGrid);
      }
      if (Array.isArray(inv.items) && inv.items.length) {
        var itemSec = el("section", "ld-sec");
        itemSec.appendChild(el("div", "ld-sec-title", L.inventoryItemsSection || "资源与访问入口清单"));
        itemSec.appendChild(buildTable([L.colName || "名称", L.colKindStage || "类型 / 环境", L.colStatus || "状态", L.colAccessOrDetail || "访问地址 / 说明"], inv.items.map(function (it) {
          var wrapSpan = document.createElement("span");
          if (it.safeUrl) wrapSpan.appendChild(makeLink(it.safeUrl, it.safeUrl));
          else if (it.rawUrl) wrapSpan.appendChild(el("code", "ld-mono", it.rawUrl));
          if (it.detail) {
            if (it.safeUrl || it.rawUrl) wrapSpan.appendChild(document.createTextNode(" · "));
            wrapSpan.appendChild(document.createTextNode(it.detail));
          }
          if (!it.safeUrl && !it.rawUrl && !it.detail) wrapSpan.appendChild(document.createTextNode("-"));
          return [
            el("code", "ld-mono", it.name),
            (it.kind || "-") + (it.stage ? " · " + it.stage : ""),
            it.status ? el("span", "ld-badge ld-tone-" + safeTone(it.tone), it.status) : "-",
            wrapSpan
          ];
        })));
        article.appendChild(itemSec);
      }
    }

    if (card.topology) {
      var tp = card.topology;
      if (tp.summary) article.appendChild(el("p", "ld-summary", tp.summary));
      if (Array.isArray(tp.entrypoints) && tp.entrypoints.length) {
        var epSec = el("section", "ld-sec");
        epSec.appendChild(el("div", "ld-sec-title", L.topologyEntrySection || "公网网关入口（GatewayRoute）"));
        epSec.appendChild(buildTable([L.colEntryRoute || "路由入口", L.colProtocolTLS || "协议 / TLS", L.colBackendTarget || "后端目标服务", L.colStatus || "状态"], tp.entrypoints.map(function (ep) {
          return [
            ep.safeUrl ? makeLink(ep.safeUrl, ep.routeLabel) : el("code", "ld-mono", ep.routeLabel),
            el("code", "ld-mono", (ep.protocol || "HTTP") + (ep.tlsStatus ? " · " + ep.tlsStatus : "")),
            el("code", "ld-mono", ep.targetService),
            ep.status ? el("span", "ld-badge ld-tone-" + safeTone(ep.tone), ep.status) : "-"
          ];
        })));
        article.appendChild(epSec);
      }
      if (Array.isArray(tp.nodes) && tp.nodes.length) {
        var ndSec = el("section", "ld-sec");
        ndSec.appendChild(el("div", "ld-sec-title", L.topologyNodesSection || "服务节点"));
        ndSec.appendChild(buildTable([L.colNode || "节点", L.colKindStage || "类型 / 环境", L.colStatus || "状态", L.colReplicasOrNote || "副本 / 备注"], tp.nodes.map(function (n) {
          return [
            el("code", "ld-mono", n.name),
            (n.kind || "-") + (n.stage ? " · " + n.stage : ""),
            n.status ? el("span", "ld-badge ld-tone-" + safeTone(n.tone), n.status) : "-",
            [n.replicas, n.detail].filter(Boolean).join(" · ") || "-"
          ];
        })));
        article.appendChild(ndSec);
      }
      if (Array.isArray(tp.bindings) && tp.bindings.length) {
        var bdSec = el("section", "ld-sec");
        bdSec.appendChild(el("div", "ld-sec-title", L.topologyBindSection || "服务依赖绑定（ServiceBinding）"));
        bdSec.appendChild(buildTable([L.colFrom || "上游调用方", L.colTo || "下游依赖服务", L.colEndpointInjection || "协议端口 / 注入", L.colStatus || "状态"], tp.bindings.map(function (b) {
          return [
            el("code", "ld-mono", b.from),
            el("code", "ld-mono", "→ " + b.to),
            el("code", "ld-mono", (b.endpoint || "-") + (b.injection ? " (" + b.injection + ")" : "")),
            b.status ? el("span", "ld-badge ld-tone-" + safeTone(b.tone), b.status) : "-"
          ];
        })));
        article.appendChild(bdSec);
      }
    }

    root.appendChild(article);
    reportSize();
  }

  window.addEventListener("message", function (event) {
    if (window.parent === window || event.source !== window.parent) return;
    var data = event.data;
    if (!data || typeof data !== "object" || data.jsonrpc !== "2.0") return;

    if (data.method === "ping" && data.id !== undefined) {
      postRpc({ jsonrpc: "2.0", id: data.id, result: {} });
      return;
    }
    if (data.method === "ui/resource-teardown") {
      if (resizeObs) {
        resizeObs.disconnect();
        resizeObs = null;
      }
      if (data.id !== undefined) {
        postRpc({ jsonrpc: "2.0", id: data.id, result: {} });
      }
      return;
    }
    if (!initialized && data.id === initReqId && data.result) {
      initialized = true;
      hostCaps = data.result.hostCapabilities || {};
      applyHostContext(data.result.hostContext);
      postRpc({ jsonrpc: "2.0", method: "ui/notifications/initialized", params: {} });
      reportSize();
      return;
    }
    if (data.method === "ui/notifications/host-context-changed") {
      applyHostContext(data.params);
      return;
    }
    if (data.method === "ui/notifications/tool-input" || data.method === "ui/notifications/tool-input-partial") {
      renderLoading(data.params && data.params.arguments);
      return;
    }
    if (data.method === "ui/notifications/tool-cancelled") {
      renderStatusPlaceholder(isEnglish() ? "Card cancelled" : "已取消生成卡片", "", "neutral");
      return;
    }
    if (data.method === "ui/notifications/tool-result") {
      var params = data.params || {};
      var sc = params.structuredContent;
      if (!params.isError && sc && sc.card) {
        renderCardModel(sc.card);
      } else {
        var errMsg = (sc && sc.message) ? String(sc.message) : (isEnglish() ? "Unable to render card." : "卡片参数校验未通过，无法渲染。");
        renderStatusPlaceholder(isEnglish() ? "Card render failed" : "卡片渲染失败", errMsg, "danger");
      }
      return;
    }
    if (typeof data.method === "string" && data.id !== undefined) {
      postRpc({ jsonrpc: "2.0", id: data.id, error: { code: -32601, message: "Method not found" } });
    }
  });

  if (typeof ResizeObserver !== "undefined" && root) {
    resizeObs = new ResizeObserver(reportSize);
    resizeObs.observe(root);
  }

  postRpc({
    jsonrpc: "2.0",
    id: initReqId,
    method: "ui/initialize",
    params: {
      protocolVersion: "2026-01-26",
      appInfo: { name: "luna-devops-card-viewer", version: "1.0.0" },
      appCapabilities: {}
    }
  });
})();
</script>
</body>
</html>
`
}
