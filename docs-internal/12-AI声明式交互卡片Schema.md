# AI 声明式交互内容与卡片 Schema

## 1. 目标

AI 助手需要在对话中生成受控的内容和交互界面，而不是只输出文字、普通选项，
或把用户跳转到其他页面继续操作。动态表单只是其中一种输入能力；同一协议还要
表达资源列表、对比、指标、诊断、计划、审批、进度和结果等内容。

首个落地场景是应用安装：

```text
用户要求安装 PostgreSQL
  -> Agent 搜索平台应用模板
  -> 没有合适模板时按需搜索互联网
  -> Agent 读取并核验候选来源
  -> 展示 1～5 张应用候选卡片
  -> 用户在卡片内补充安装参数
  -> 用户提交
  -> Agent 创建 installAppTemplate Tool Call
  -> 平台按当前用户权限、风险策略、确认与 MFA 流程执行
  -> 卡片显示执行进度和结果
```

卡片是声明式交互协议，不是模型生成的任意前端组件。模型通过 Tool Call 生成
模板、内容块、输入字段和动作，前端收到已校验的工具请求后按固定组件渲染。模型
不能返回 HTML、JSX、CSS、DOM Selector、脚本、任意 API 地址或可执行表达式。

协议采用五层结构：

```text
模板 Template
  -> 决定稳定的信息拓扑和主要交互模式
内容块 Content Block
  -> 呈现文本、指标、列表、表格、代码、差异、时间线和拓扑
输入 Field
  -> 收集文本、数值、布尔、选择、Secret 和结构化键值
动作 Action
  -> 发送消息、站内导航或提出受控平台 Tool Call
运行态 Runtime
  -> 平台接管校验、权限、确认、MFA、执行、进度、失败和结果
```

## 2. 核心决策

### 2.1 不使用应用或表单专属的根协议

`appIcon + appName + appConfig` 可以表达单个安装场景，但不能覆盖后续的诊断修复、
资源创建、配置修改和参数补充。

根协议统一为 `InteractionCardGroup`。应用安装只是 `catalog` 模板的一种用途，
表单也只是卡片的可选区域。纯内容卡片可以没有字段，查询结果卡片可以只有选择，
进度卡片的状态由服务端实时更新。

### 2.2 只保留当前版本

项目尚未发布，不实现旧版本兼容、迁移器或多版本渲染器。当前唯一版本为
`schemaVersion: 1`；前后端遇到其他版本直接拒绝并提示刷新卡片。

### 2.3 卡片不直接调用业务 API

V1 复用现有 Agent Run 链路：浏览器在本地完成可见字段校验与受控参数绑定，把
`operationId + arguments` 封装为结构化的用户操作意图，再创建新的 Agent Run。
Agent 只能从 Tool Catalog 选择已注册操作，Luna API 使用当前用户绑定的短时委托
令牌重新校验权限、确认、MFA 和参数摘要后执行。

浏览器不会从卡片中的 `operationId` 拼接业务 URL，也不能绕过：

- 当前登录用户权限校验；
- Tool Catalog；
- 风险分类；
- 二次确认；
- Step-up MFA；
- 幂等与审计。

卡片中的 Secret 字段不进入对话消息。V1 仅允许使用模板的后端自动生成能力；
需要用户手动填写 Secret 的卡片显示不可用说明。后续若增加卡片提交端点，Secret
也只能通过一次性加密通道提交给 Luna API，不能进入 Agent 上下文或 Timeline。

### 2.4 模型组合受控内容，不决定任意组件

前端实现一组固定模板和内容块。模型可以选择模板、排列允许的内容块、填写文案和
绑定可信数据，但不能指定任意布局参数。

应用候选模板示例：

```text
候选组标题
  -> 候选卡片
      -> 图标、名称、版本、来源、简介和状态
      -> 展开/收起配置
      -> 固定字段组件
      -> 主提交按钮
      -> 执行状态与错误恢复
```

模型可以提供标题、说明、可信来源引用、内容块、字段定义和动作绑定；不能决定
任意颜色、尺寸、动画、CSS 类名或组件名称。

### 2.5 AI 生成定义，平台拥有执行权

`create_interaction_cards` 的 Tool Call 请求就是声明式 UI 定义。Agent 可以根据
当前问题和可信工具结果动态生成卡片，但生成不等于执行：

- 内容可由模型摘要，但事实值必须能关联到当前 Run 的来源；
- 资源标识、操作参数和选项值必须来自可信 Tool Result；
- 展示模板不能改变操作风险；
- Tool Action 只表达用户意图，平台仍重新鉴权和执行；
- 审批、MFA、危险说明和最终结果由平台生成，模型不能伪造。

## 3. 卡片创建工具

内建纯 UI 工具命名为 `create_interaction_cards`。它不读取外部数据，不执行安装，
只校验并持久化声明式卡片。

`create_options` 继续用于无需丰富内容的轻量选择、追问和导航。出现以下任一情况时
使用 `create_interaction_cards`：

- 需要展示多个带属性的候选资源；
- 需要对比、诊断证据、指标、配置差异、步骤或执行结果；
- 需要用户补充结构化参数；
- 需要用户选择一项或多项资源后继续操作；
- 需要跟踪审批、MFA 或长任务状态。

```ts
interface CreateInteractionCardsInputV1 {
  schemaVersion: 1
  title: string
  description?: string
  template: Exclude<CardTemplateV1, "approval">
  display?: {
    density?: "comfortable" | "compact"
    selection?: "none" | "single" | "multiple"
  }
  cards: InteractionCardInputV1[] // 1～12
  groupActions?: CardActionV1[] // 0～3
}

type CardTemplateV1 =
  | "catalog"
  | "comparison"
  | "inspector"
  | "form"
  | "wizard"
  | "diagnosis"
  | "plan"
  | "approval"
  | "progress"
  | "result"
  | "dashboard"

interface InteractionCardInputV1 {
  id: string
  presentation: CardPresentationV1
  sourceRefs?: CardSourceRefV1[]
  blocks?: CardContentBlockV1[] // 0～12
  form?: DynamicFormV1
  actions?: CardActionV1[] // 0～4
}
```

约束：

- `title` 最长 120 字符，`description` 最长 300 字符。
- 每组最多 12 张卡片；超过上限使用服务端分页游标或先缩小搜索范围，不让模型
  一次复制任意数量的数据。
- `comparison` 最多 4 张，`approval` 固定 1 张，`progress` 和 `result` 每组最多
  3 张。
- `id` 在当前卡片组内唯一，只允许字母、数字、下划线和短横线。
- 单张卡片序列化后最大 32 KiB，整组最大 96 KiB。
- 所有资源 ID、模板 ID、工具 ID 和选项值必须来自当前 Run 的可信工具结果，
  不能由模型猜测。
- `approval` 模板及危险操作的影响摘要必须由平台补全，模型不能自行把普通卡片
  标记为已批准。
- `approval` 属于平台内部模板，不在 `create_interaction_cards` 的模型输入联合
  类型内。
- 模板只决定拓扑，不决定权限。同一个 Tool Action 在任何模板下具有相同风险。

## 4. 卡片展示 Schema

```ts
interface CardPresentationV1 {
  variant:
    | "application"
    | "resource"
    | "form"
    | "finding"
    | "plan"
    | "task"
    | "receipt"
    | "summary"
  title: string
  subtitle?: string
  description?: string
  icon?: CardIconV1
  badges?: Array<{
    label: string
    tone: "neutral" | "success" | "warning"
  }>
}

type CardIconV1 =
  | {
      type: "asset"
      assetRef: string
      alt: string
    }
  | {
      type: "category"
      name:
        | "database"
        | "cache"
        | "queue"
        | "storage"
        | "observability"
        | "application"
        | "repository"
        | "registry"
        | "cluster"
        | "build"
        | "deployment"
        | "gateway"
        | "security"
        | "billing"
        | "notification"
      alt: string
    }

interface CardSourceRefV1 {
  type:
    | "app_template"
    | "web_search_result"
    | "web_page"
    | "platform_resource"
    | "platform_event"
    | "tool_result"
  refId: string
  label: string
  trust: "platform" | "official" | "community"
}
```

安全要求：

- `icon` 不接受任意 URL。平台应用模板使用已有静态资源引用；互联网候选图标必须
  先由后端下载、校验、代理或缓存为 `assetRef`。
- `danger` 不允许作为模型可选视觉 tone。危险状态由平台根据 Tool Policy 计算。
- `trust` 由搜索、抓取或平台工具结果产生，模型不能提升来源可信级别。
- 互联网搜索结果不能仅凭标题和摘要标记为“可安装”。必须经过页面读取和安装方案
  核验后，才能绑定安装工具。

## 5. 内容块 Schema

内容块用于在卡片内部呈现信息。它们是固定组件，不是任意富文本布局。

```ts
type CardContentBlockV1 =
  | MarkdownBlockV1
  | CalloutBlockV1
  | KeyValueBlockV1
  | MetricBlockV1
  | ItemListBlockV1
  | StatusListBlockV1
  | DataTableBlockV1
  | CodeBlockV1
  | DiffBlockV1
  | TimelineBlockV1
  | ChartBlockV1
  | RelationBlockV1
  | ProgressBlockV1
  | ResourceLinksBlockV1

interface ContentBlockBaseV1 {
  id: string
  title?: string
  sourceRefIds?: string[]
  collapsible?: boolean
  defaultExpanded?: boolean
}
```

### 5.1 说明、提醒与键值

```ts
interface MarkdownBlockV1 extends ContentBlockBaseV1 {
  type: "markdown"
  content: string
}

interface CalloutBlockV1 extends ContentBlockBaseV1 {
  type: "callout"
  tone: "info" | "success" | "warning" | "error"
  content: string
}

interface KeyValueBlockV1 extends ContentBlockBaseV1 {
  type: "key_value"
  items: Array<{
    label: string
    value: string
    format?: "text" | "code" | "status" | "duration" | "date_time" | "bytes" | "currency"
    copyable?: boolean
  }> // 1～16
}
```

Markdown 使用现有安全渲染器，只允许文本格式、列表、表格、代码和经过校验的
站内链接或来源链接；禁止原始 HTML、图片、iframe 和活动内容。

### 5.2 指标、列表和状态

```ts
interface MetricBlockV1 extends ContentBlockBaseV1 {
  type: "metrics"
  items: Array<{
    label: string
    value: string
    change?: string
    trend?: "up" | "down" | "flat"
    tone?: "neutral" | "success" | "warning" | "error"
  }> // 1～6
}

interface ItemListBlockV1 extends ContentBlockBaseV1 {
  type: "item_list"
  items: Array<{
    id: string
    primary: string
    secondary?: string
    meta?: string
    icon?: CardIconV1
  }> // 1～20
}

interface StatusListBlockV1 extends ContentBlockBaseV1 {
  type: "status_list"
  items: Array<{
    id: string
    label: string
    detail?: string
    status: "pending" | "running" | "success" | "warning" | "error" | "skipped"
  }> // 1～20
}
```

指标的语义 tone 可以由平台数据映射；模型不能仅凭猜测把未知状态标成成功。

### 5.3 表格、代码和差异

```ts
interface DataTableBlockV1 extends ContentBlockBaseV1 {
  type: "data_table"
  columns: Array<{
    key: string
    label: string
    format?: "text" | "code" | "status" | "duration" | "date_time" | "bytes" | "currency"
  }> // 1～8
  rows: Array<{
    id: string
    cells: Record<string, string>
  }> // 0～30
  rowSelection?: "none" | "single" | "multiple"
}

interface CodeBlockV1 extends ContentBlockBaseV1 {
  type: "code"
  language: string
  content: string
  filename?: string
}

interface DiffBlockV1 extends ContentBlockBaseV1 {
  type: "diff"
  language?: string
  beforeLabel?: string
  afterLabel?: string
  unifiedDiff: string
}
```

表格和代码可以在自身容器内横向滚动，不能撑宽 AI 小窗。表格选择结果只能绑定
当前表格已有的 `row.id`，不能把任意单元格文本当作资源标识。

### 5.4 时间线、图表、关系和进度

```ts
interface TimelineBlockV1 extends ContentBlockBaseV1 {
  type: "timeline"
  items: Array<{
    id: string
    title: string
    detail?: string
    timestamp?: string
    status?: "pending" | "running" | "success" | "warning" | "error"
  }> // 1～20
}

interface ChartBlockV1 extends ContentBlockBaseV1 {
  type: "chart"
  chartType: "line" | "bar" | "area" | "donut"
  xAxis?: string[]
  series: Array<{
    name: string
    values: number[]
    unit?: string
  }> // 1～4
}

interface RelationBlockV1 extends ContentBlockBaseV1 {
  type: "relations"
  nodes: Array<{
    id: string
    label: string
    category: string
    status?: "neutral" | "success" | "warning" | "error"
  }> // 1～30
  edges: Array<{
    source: string
    target: string
    label?: string
  }> // 0～50
}

interface ProgressBlockV1 extends ContentBlockBaseV1 {
  type: "progress"
  mode: "determinate" | "indeterminate"
  value?: number // 0～100
  label: string
  detail?: string
}

interface ResourceLinksBlockV1 extends ContentBlockBaseV1 {
  type: "resource_links"
  links: Array<{
    label: string
    routeName?: string
    routeParams?: Record<string, string>
    sourceRefId?: string
  }> // 1～8
}
```

图表、关系和进度中的原始数值与状态必须来自 Tool Result。模型可以选择合适的
呈现方式和解释，但不能生成不存在的监控采样、拓扑节点或执行进度。`progress`
在动作执行后由服务端运行态更新，不再接受模型覆盖。

## 6. 动态输入 Schema

V1 只支持满足当前需求的有限字段集合：

```ts
interface DynamicFormV1 {
  sections: FormSectionV1[] // 1～6
}

interface FormSectionV1 {
  id: string
  title?: string
  description?: string
  fields: FormFieldV1[] // 每节 1～12，整张卡片最多 24
}

type FormFieldV1 =
  | TextFieldV1
  | TextareaFieldV1
  | NumberFieldV1
  | BooleanFieldV1
  | SelectFieldV1
  | MultiSelectFieldV1
  | KeyValueFieldV1
  | SecretFieldV1

interface FieldBaseV1 {
  id: string
  label: string
  description?: string
  required?: boolean
  visibleWhen?: FieldConditionV1
}
```

### 6.1 文本

```ts
interface TextFieldV1 extends FieldBaseV1 {
  type: "text"
  defaultValue?: string
  placeholder?: string
  format?:
    | "plain"
    | "identifier"
    | "namespace"
    | "hostname"
    | "email"
    | "url"
    | "image_ref"
    | "cpu"
    | "memory"
    | "storage"
  minLength?: number
  maxLength?: number
}

interface TextareaFieldV1 extends FieldBaseV1 {
  type: "textarea"
  defaultValue?: string
  placeholder?: string
  minLength?: number
  maxLength?: number
  rows?: 2 | 3 | 4 | 5 | 6
}
```

不接受模型提供任意正则表达式。`format` 映射到平台维护的 Zod 校验器，避免 ReDoS、
前后端规则漂移和模型生成错误表达式。

### 6.2 数值和布尔

```ts
interface NumberFieldV1 extends FieldBaseV1 {
  type: "number"
  defaultValue?: number
  integer?: boolean
  min?: number
  max?: number
  step?: number
  unit?: string
}

interface BooleanFieldV1 extends FieldBaseV1 {
  type: "boolean"
  defaultValue?: boolean
}
```

### 6.3 选择

```ts
interface SelectFieldV1 extends FieldBaseV1 {
  type: "select"
  defaultValue?: string
  placeholder?: string
  display?: "select" | "radio" | "segmented"
  options: Array<{
    value: string
    label: string
    description?: string
    disabled?: boolean
  }> // 1～50
}

interface MultiSelectFieldV1 extends FieldBaseV1 {
  type: "multi_select"
  defaultValue?: string[]
  placeholder?: string
  minItems?: number
  maxItems?: number
  options: Array<{
    value: string
    label: string
    description?: string
    disabled?: boolean
  }> // 1～50
}
```

项目空间、集群、环境、版本、角色和事件类型等资源选择，先转换成经过权限过滤的
稳定选项；不允许卡片声明任意远程数据源。超过 50 个候选时，先由 Agent 调用搜索
或分页工具缩小范围。

### 6.4 键值和 Secret

```ts
interface KeyValueFieldV1 extends FieldBaseV1 {
  type: "key_value"
  defaultValue?: Array<{ key: string; value: string }>
  keyFormat?: "plain" | "identifier" | "environment_variable"
  valueMode?: "plain" | "secret"
  minItems?: number
  maxItems?: number // 最大 30
}
```

```ts
interface SecretFieldV1 extends FieldBaseV1 {
  type: "secret"
  placeholder?: string
  generation: "disabled" | "optional" | "required"
  defaultMode?: "manual" | "generate"
}
```

Secret 字段：

- 不允许 `defaultValue`；
- 不进入模型上下文、Timeline、Tool Display Result、前端日志或 localStorage；
- `generation: "required"` 时由后端生成，前端只显示“将自动生成”；
- 手动输入时浏览器直接提交给 Luna API，服务端立即写入 Secret 存储或转换为一次性
  Secret 引用；
- 展开历史卡片时永不回显原值。

`key_value.valueMode: "secret"` 的值与独立 Secret 字段使用相同隔离规则。

### 6.5 受限条件和多阶段交互

同一卡片允许简单的字段显隐，但不支持任意表达式：

```ts
interface FieldConditionV1 {
  fieldId: string
  operator: "equals" | "not_equals" | "contains" | "is_empty" | "is_not_empty"
  value?: string | number | boolean
}
```

每个字段最多一个 `visibleWhen`，只能引用同一表单中排在它之前的非 Secret 字段。
复杂依赖不在浏览器中编排：

- 选择项目空间后再查询集群；
- 选择仓库后再查询分支；
- 选择镜像仓库后再查询镜像和标签；
- 诊断后根据结果生成修复参数。

这些场景使用多阶段卡片：用户提交当前阶段后，平台执行只读 Tool，Agent 根据结果
生成下一组卡片。这样每一步都有权限、来源和状态记录，也不需要把业务 API 数据源
塞进前端 Schema。

## 7. 动作与参数绑定

```ts
type CardActionV1 =
  | ToolCardActionV1
  | MessageCardActionV1
  | NavigateCardActionV1

interface CardActionBaseV1 {
  id: string
  label: string
  description?: string
  emphasis?: "primary" | "secondary" | "ghost"
  repeatable?: boolean
}

interface ToolCardActionV1 extends CardActionBaseV1 {
  type: "tool"
  operationId: string
  bindings: ArgumentBindingV1[]
  selectionBinding?: {
    target: string
    source: "selected_cards" | "selected_rows"
    sourceBlockId?: string
  }
}

interface MessageCardActionV1 extends CardActionBaseV1 {
  type: "send_message"
  message: string
  selectionContext?: {
    source: "selected_cards" | "selected_rows"
    sourceBlockId?: string
  }
}

interface NavigateCardActionV1 extends CardActionBaseV1 {
  type: "navigate"
  routeName: string
  routeParams?: Record<string, string>
}

interface ArgumentBindingV1 {
  target: string // RFC 6901 JSON Pointer
  value:
    | { type: "field"; fieldId: string }
    | { type: "card"; property: "id" }
    | { type: "selected_row"; column: string }
    | { type: "literal"; value: string | number | boolean | null }
}
```

示例：

```json
{
  "id": "install",
  "type": "tool",
  "label": "安装 PostgreSQL",
  "operationId": "installAppTemplate",
  "bindings": [
    {
      "target": "/projectId",
      "value": { "type": "field", "fieldId": "projectId" }
    },
    {
      "target": "/templateId",
      "value": { "type": "literal", "value": "postgresql" }
    },
    {
      "target": "/applicationName",
      "value": { "type": "field", "fieldId": "applicationName" }
    },
    {
      "target": "/clusterId",
      "value": { "type": "field", "fieldId": "clusterId" }
    },
    {
      "target": "/values/username",
      "value": { "type": "field", "fieldId": "username" }
    }
  ]
}
```

不支持模板字符串、JavaScript、JSONPath、JQ、表达式语言或任意函数。服务端必须：

1. 确认 `operationId` 存在于创建卡片时固化的 Tool Catalog。
2. 确认每个 `target` 是该 Tool 输入 Schema 已声明的字段。
3. 确认 `fieldId` 存在且类型与目标字段兼容。
4. 确认 literal 中的资源和模板标识来自 `sourceRefs` 对应的可信工具结果。
5. 使用服务端保存的卡片定义绑定参数，不信任浏览器回传的绑定关系。
6. 对最终参数再次执行 Tool Schema 校验。
7. 创建新的 Tool Call，继续使用当前用户权限、确认、MFA、幂等和审计流程。

动作规则：

- `tool` 只提出平台操作，不能携带 URL；风险和批准策略由 Tool Catalog 决定。
- `send_message` 用于继续追问、解释当前结果或把选中内容作为下一轮上下文。
- `selectionContext` 只把平台保存的所选 ID 作为结构化上下文附加到消息，不能
  使用模板字符串把选择内容拼进任意指令。
- `navigate` 只用于用户要读取或查看完整页面时，不能代替本应在卡片内完成的操作。
- `repeatable` 只允许幂等的读取、导航和刷新；写操作由平台强制改为 `false`。
- 每张卡片最多一个 primary 动作；危险动作不能由模型指定红色，平台按风险渲染。
- `selectionBinding` 只能读取当前卡片组或表格已经显示并由用户选中的可信 ID。

## 8. 服务端运行态

模型不能填写运行态。卡片持久化后由服务端补充：

```ts
interface InteractionCardRuntimeV1 {
  cardId: string
  schemaVersion: 1
  schemaHash: string
  state:
    | "ready"
    | "selected"
    | "submitting"
    | "awaiting_input"
    | "awaiting_approval"
    | "awaiting_mfa"
    | "running"
    | "succeeded"
    | "failed"
    | "cancelled"
    | "expired"
  submittedToolCallId?: string
  activeActionId?: string
  expiresAt: string
  errorCode?: string
  requestId?: string
}
```

状态要求：

- 同一卡片每次只允许一个提交在途。
- 非幂等提交成功后禁用该卡片；同组其他卡片保持可用。
- 提交失败不消耗卡片，用户修正字段后可以重试。
- Tool Call 的批准、MFA、执行和取消状态实时映射到卡片。
- 只读卡片允许由服务端刷新内容；刷新产生新 `schemaHash`，不覆盖用户正在编辑的
  表单草稿。
- 资源版本变化或卡片过期时，不使用旧参数执行；提示重新搜索或刷新。
- 卡片定义作为 Timeline Item 持久化，表单草稿默认仅在浏览器内存中保存。

提交请求：

```ts
interface SubmitInteractionCardRequestV1 {
  cardId: string
  values: Record<string, string | number | boolean>
  secretValues?: Record<string, string>
  idempotencyKey: string
}
```

`secretValues` 只存在于当前 HTTPS 请求内。Luna API 必须在日志、Tracing、错误和
请求快照之前将其剥离，立即写入 Secret 存储或转换为一次性引用；Agent、Timeline
和卡片提交记录只接收引用，不能持久化或回显原文。

## 9. 应用安装示例

完整的安装卡片 JSON 示例可由上文 Schema 直接组合得到，此处不再逐项罗列。
示例中的项目空间、集群和模板标识只是协议示例。真实卡片必须使用当前用户查询结果。

## 10. 搜索与网页读取工具

### 10.1 搜索顺序

安装请求按以下优先级处理：

1. `searchAppTemplates`：搜索平台应用市场，返回可直接安装的可信模板。
2. `searchWeb`：平台模板无结果或用户明确要求搜索互联网时使用。
3. `fetchWebPage`：读取选定搜索结果的官网、仓库或安装文档。
4. `inspectApplicationCandidate`：把互联网来源转换为受控安装方案；未通过核验的候选
   只能显示“查看来源”或“继续分析”，不能绑定安装 Tool。

不能为了省事跳过平台模板搜索，直接让用户去应用市场页面。

### 10.2 工具输入

```ts
interface SearchWebInput {
  query: string
  language?: string
  freshness?: "day" | "week" | "month" | "year"
  maxResults?: number // 1～10
}

interface SearchWebResult {
  results: Array<{
    resultId: string
    title: string
    url: string
    domain: string
    snippet: string
    publishedAt?: string
    iconAssetRef?: string
  }>
}

interface FetchWebPageInput {
  resultId?: string
  url?: string
  maxCharacters?: number // 最大 30000
}
```

`resultId` 与 `url` 必须且只能提供一个。

`fetchWebPage.url` 只能是用户在当前消息中明确提供的 URL，或者当前 Run 的
`searchWeb` 结果；优先使用 `resultId`。后端必须校验：

- 只允许公开的 HTTP/HTTPS 目标；
- DNS 解析结果和每次重定向都执行 SSRF 检查；
- 禁止 loopback、链路本地、元数据地址、Service CIDR、Kubernetes API 和私网；
- 限制响应大小、超时、重定向次数和 Content-Type；
- 不携带平台 Cookie、Authorization、Secret 或用户浏览器凭据；
- 返回正文、标题、来源和引用位置，不返回活动脚本。

### 10.3 Provider 边界

Agent 注册 `searchWeb` 和 `fetchWebPage` 工具，但不直接访问第三方搜索或抓取 API。
工具仍通过 Luna API 执行，由后端 Provider 适配外部服务。

建议首版：

- 搜索 Provider：优先支持可自托管的 SearXNG；
- 托管搜索：后续可选 Tavily、Brave Search 或 Serper；
- 页面读取：先实现平台内置的受限 HTTP 提取器；
- 复杂页面：后续再增加 Tavily Extract、Jina 或 Firecrawl Provider。

搜索 Provider 与页面读取 Provider 必须分离。LibreChat 的公开配置同时区分
`searchProvider` 与 `scraperProvider`，LobeChat 的联网搜索 RFC 也把 SearXNG 搜索
和页面抓取拆成两个工具；本项目沿用这一边界，但所有配置仍进入 Web 管理页面，
不把第三方密钥交给 Agent 或浏览器。

参考：

- <https://github.com/danny-avila/LibreChat/blob/main/librechat.example.yaml>
- <https://github.com/lobehub/lobe-chat/discussions/6277>

## 11. 固定模板与复杂交互拓扑

模板不是业务类型，而是稳定的信息与交互拓扑。同一业务可以在不同阶段使用不同
模板，例如应用安装会依次使用 `catalog -> form -> approval -> progress -> result`。

### 11.1 Catalog：资源搜索与候选列表

适用：应用模板、数据库、仓库、镜像、集群、成员候选、通知预设。

```text
搜索摘要与筛选条件
  -> 1～12 个候选卡片或一张可选表格
      -> 名称、来源、版本、状态、关键属性
      -> 展开查看详情
      -> 单选或多选
  -> 配置所选项 / 执行所选项 / 加载下一页
```

复杂逻辑：

- 平台模板优先，互联网候选单独标注来源和可信级别；
- “所有可用数据库”按当前查询分页，不把全部结果塞进一次 Tool Call；
- 候选没有安装能力时只允许查看、继续核验或作为消息回复；
- 单项操作放在卡片内，批量操作放在组级动作；
- 用户选择一项后可以展开该项表单，其他候选保持可选；
- 搜索条件变化时生成新卡片组，旧组保留为历史但不可使用过期游标执行。

### 11.2 Comparison：方案对比与推荐

适用：PostgreSQL/MySQL、不同镜像标签、构建模板、集群、部署规格、通知渠道。

```text
比较目标
  -> 2～4 个方案
      -> 相同维度的属性和指标
      -> 优势、限制、来源
  -> 推荐理由
  -> 选择方案 / 修改需求 / 查看依据
```

复杂逻辑：

- 比较维度必须一致，缺失值显示“未知”，不能由模型补齐；
- 推荐是模型意见，事实值必须引用 Tool Result；
- 选择方案只写入当前交互状态，不代表已经执行安装或变更；
- 选择后进入下一张 `form` 或 `plan` 卡片，不在比较卡片中塞入全部配置。

### 11.3 Inspector：资源详情与内容阅读

适用：项目空间、应用、构建、发布、事件、账单记录、网页来源。

```text
资源摘要与状态
  -> 关键属性 / 指标 / 关联资源
  -> 可折叠的日志、代码、YAML、事件或来源
  -> 查看完整页面 / 继续分析 / 发起相关操作
```

复杂逻辑：

- 默认只展示最重要的信息，长日志、YAML 和来源正文折叠；
- 站内完整详情使用路由动作，当前任务可在卡片内完成时不强迫跳转；
- 读取类卡片允许刷新；刷新不得覆盖同卡片中尚未提交的输入。

### 11.4 Dashboard：摘要、指标与待办

适用：平台概况、项目健康、成本概览、运行环境健康。

```text
核心指标
  -> 风险与待办
  -> 最近活动或趋势
  -> 对异常项执行下钻或诊断
```

复杂逻辑：

- 正常零值弱化，异常零值或不可用状态明确标记；
- 图表只展示查询结果中的真实采样；
- 多个风险项可单独诊断，不把“全部修复”作为默认主操作。

### 11.5 Form：单阶段创建或修改

适用：创建项目空间、添加成员、创建通知渠道、简单配置更新。

```text
操作目的与影响范围
  -> 一个或多个字段分组
  -> 实时本地校验
  -> 提交平台 Tool
```

复杂逻辑：

- 资源选择只使用当前用户可见的候选；
- Secret 使用隔离通道；
- 编辑场景展示当前值，写保护 Secret 只显示“已设置”；
- 提交失败保留非 Secret 草稿，并把后端字段错误映射回对应字段。

### 11.6 Wizard：跨资源、多阶段配置

适用：绑定仓库并构建、选择镜像并发布、配置网关、接入 Git/OIDC/Registry。

```text
阶段 1：选择目标
  -> 只读 Tool 查询下一阶段候选
阶段 2：选择来源或方案
  -> 只读 Tool 查询可用分支、标签或运行环境
阶段 3：填写配置
  -> 生成执行计划
阶段 4：确认并执行
```

复杂逻辑：

- 每个阶段都是独立持久化卡片，不使用浏览器隐藏状态保存整个流程；
- 上游选择变化时，下游结果失效并重新查询；
- 用户可以返回修改上一阶段，但已完成的写操作不能伪装成可撤销表单步骤；
- 任何阶段缺少权限时立即停止，显示所需权限和可恢复路径。

### 11.7 Diagnosis：证据、判断与修复

适用：构建失败、发布失败、集群异常、域名/证书问题、Webhook 或连接失败。

```text
现象摘要
  -> 已执行检查及证据
  -> 1～N 个发现，按严重程度排序
  -> 最可能根因与置信说明
  -> 每个发现对应的修复、重试、查看证据或忽略动作
```

复杂逻辑：

- 工具失败原因、稳定错误码和请求编号必须可见，不能只显示“稍后重试”；
- 模型推断与平台事实分开展示；
- 修复前展示将修改的资源和字段；
- 多个发现互相独立，修复一个不应禁用其他发现；
- 修复后自动运行验证 Tool，并以新证据生成结果卡片。

### 11.8 Plan：执行计划与变更预览

适用：部署、回滚、批量删除、数据清理、配置变更、应用安装。

```text
目标状态
  -> 将执行的步骤
  -> 新建 / 修改 / 删除的资源
  -> 配置或 YAML 差异
  -> 风险、预计影响和不可逆项
  -> 执行 / 返回修改
```

复杂逻辑：

- 计划来自预览 Tool 或确定性参数绑定，不能仅由模型自由编造；
- 资源版本、计划摘要和参数哈希一起持久化；
- 执行前资源版本发生变化时计划失效，必须重新生成；
- `Plan` 不替代危险操作的正式批准卡片。

### 11.9 Approval：平台生成的正式确认

适用：删除、回滚、数据清理、密钥轮换、权限提升、生产变更。

```text
操作名称与风险级别
  -> 操作者、目标和影响范围
  -> 参数摘要 / 差异 / 不可逆说明
  -> 同意 / 拒绝 / 本轮全部同意
```

复杂逻辑：

- 卡片定义、危险文案、按钮和有效期由平台生成，不接受模型覆盖；
- “本轮全部同意”只适用于当前 Run 内、同一风险策略允许合并批准的后续 Tool Call；
- 不跨会话、不跨用户、不跳过 MFA，不适用于平台禁止批量批准的高危操作；
- 拒绝后 Agent 可以解释或调整方案，但不能重复提交相同操作骚扰用户；
- 批准时重新校验用户权限、资源版本、参数哈希和批准有效期。

### 11.10 Progress：长任务与实时状态

适用：构建、部署、安装、回滚、导出、清理、连接测试。

```text
任务摘要
  -> 当前阶段与总进度
  -> 步骤时间线
  -> 实时日志摘要或关键事件
  -> 取消 / 查看日志
```

复杂逻辑：

- 初始卡片由 AI 创建，进度、阶段和日志由服务端 SSE 更新；
- 多任务并行时每张卡片独立订阅，不锁住其他会话或卡片；
- 支持取消的任务展示取消动作，不支持时不伪造按钮；
- 断线恢复使用事件游标；终态后主动关闭订阅；
- 部分成功必须逐项显示成功和失败，不能把整组简单标成失败。

### 11.11 Result：结果、回执与后续动作

适用：创建完成、安装完成、发布完成、诊断完成、导出完成。

```text
明确的成功 / 部分成功 / 失败结论
  -> 新建或变更的资源
  -> 关键输出、访问地址、版本和请求编号
  -> 验证结果
  -> 相关的下一步操作
```

复杂逻辑：

- 结果由 Tool Result 生成，模型只做解释；
- 新资源使用 `resource_links`，站内路由不刷新页面；
- 下载链接必须是后端签发的短期地址，不能由模型拼接；
- 后续动作彼此独立：查看页面后仍可以继续发消息或执行其他动作。

## 12. Luna DevOps 场景覆盖

下表按现有前端页面和 OpenAPI 业务域归纳。它约束 Agent 应该使用什么交互拓扑，
避免所有需求最后都变成“请前往某页面”。

| 场景 | 推荐模板链路 | 主要内容块 | 复杂交互与终态 |
| --- | --- | --- | --- |
| 平台看板与项目健康 | `dashboard -> diagnosis` | metrics、status_list、chart、timeline | 异常项可分别下钻；选择异常后查询证据，不自动执行修复 |
| 搜索项目空间或应用 | `catalog -> inspector` | item_list、key_value、resource_links | 支持分页、单选/多选；读取详情留在卡片，确需完整工作台才导航 |
| 创建项目空间或应用 | `form -> approval? -> result` | markdown、key_value | 校验标识符；提交后显示资源链接，不先跳转到创建页面 |
| 搜索数据库或应用模板 | `catalog -> comparison? -> form -> plan -> progress -> result` | item_list、key_value、status_list | 展示所有当前页候选；选择后填写安装参数；安装由 Tool Call 执行 |
| 搜索互联网应用 | `catalog -> inspector -> plan` | markdown、key_value、code、source links | 平台模板无结果才联网；核验官网/仓库/安装说明后才能生成安装计划 |
| 绑定 Git 仓库 | `wizard -> plan -> result` | item_list、key_value、status_list | 账号→仓库→分支→构建选项分阶段查询；OAuth 缺失时给授权动作 |
| 管理 Git Provider 或账号 | `catalog/inspector -> form -> progress -> result` | status_list、key_value | 测试连接、OAuth 回调、刷新状态分别呈现；Secret 不回显 |
| 选择镜像仓库、镜像和标签 | `wizard -> comparison -> result` | item_list、data_table、key_value | Registry→Repository→Tag 逐级查询；上游变化使下游选择失效 |
| 创建或测试镜像站 | `form -> progress -> diagnosis/result` | status_list、timeline | 测试阶段展示 DNS、认证、权限和仓库访问的分项结果 |
| 配置构建 | `wizard -> plan -> progress -> result` | code、key_value、diff、timeline | 仓库/分支/模板/变量逐级收集；触发后实时展示构建步骤和日志摘要 |
| 构建失败排查 | `diagnosis -> plan -> approval? -> progress -> result` | callout、code、timeline、diff | 日志证据与模型判断分离；修复变量或重试后自动验证 |
| 创建部署目标 | `wizard -> plan -> approval? -> progress -> result` | key_value、data_table、diff、relations | 选择集群、镜像、端口、卷和资源；预览 Kubernetes 变更 |
| 发布、重启与回滚 | `plan -> approval -> progress -> result` | timeline、diff、status_list | 回滚明确目标版本和影响；运行中可取消的任务展示取消 |
| 查看运行指标 | `inspector/dashboard -> diagnosis` | metrics、chart、timeline | 选择时间范围后查询；异常采样可继续关联日志和事件 |
| 查看日志或事件 | `inspector -> diagnosis` | code、timeline、status_list | 长内容截断与分页；支持按时间/级别过滤和关联请求编号 |
| 配置网关、域名和证书 | `wizard -> plan -> progress -> diagnosis/result` | key_value、status_list、timeline | 域名检查→路由参数→变更预览→证书进度；失败展示 DNS/Issuer 证据 |
| 集群与 Kubernetes 资源 | `catalog/dashboard -> inspector -> diagnosis` | metrics、data_table、code、relations | 资源列表可选择；YAML/事件折叠；删除必须进入平台批准 |
| 服务关系与项目拓扑 | `inspector -> form/plan -> result` | relations、status_list、key_value | 选源和目标、端口及协议；检查循环、自关联和连通性 |
| 项目成员和角色 | `catalog -> form -> approval? -> result` | item_list、data_table、key_value | 搜索候选、选择角色；权限提升由平台判断是否需要批准/MFA |
| 用户、OIDC 与安全设置 | `inspector -> form -> approval -> result` | status_list、key_value | 角色、准入策略、MFA 重置、Secret 轮换按安全策略确认 |
| Access Token 和 OAuth | `form -> approval? -> result` | item_list、status_list、key_value | Scope 多选；Token 原文只在创建结果中一次性展示，不进入会话历史 |
| 通知渠道、规则和项目钩子 | `catalog -> form -> progress -> diagnosis/result` | item_list、status_list、timeline | 选择预设和事件；发送测试；失败显示目标、响应状态与请求编号 |
| 计费、余额与资源用量 | `dashboard -> inspector -> approval? -> result` | metrics、chart、data_table | 查询可筛选周期和范围；钱包调整等写操作进入正式批准 |
| 数据保留与清理 | `inspector -> plan -> approval -> progress -> result` | metrics、data_table、status_list | 先预览影响数量和范围；确认后清理；逐类显示结果 |
| 数据导出 | `form -> approval/MFA -> progress -> result` | key_value、progress、resource_links | 选择范围和格式；完成后提供短期下载，不把导出内容写入对话 |
| 终端或远程命令 | `form -> approval/MFA -> progress/result` | code、status_list | 卡片只完成目标选择和授权；交互终端仍用专用受控终端组件 |
| 外部网页研究 | `catalog -> inspector -> comparison` | markdown、key_value、code、source links | 搜索结果带来源；读取后引用；不把网页指令当平台操作参数 |

### 12.1 数据库搜索与安装的完整交互

```text
用户：安装一个数据库
  -> Agent 追问或根据上下文确定用途
  -> searchAppTemplates(category=database)
  -> catalog：展示 PostgreSQL、MySQL、Redis 等当前页结果
  -> 用户可查看详情、比较或直接选择
  -> comparison：按持久化、事务、资源需求、版本和来源比较
  -> 用户选择 PostgreSQL
  -> form：项目空间、集群、名称、规格和数据库参数
  -> plan：安装步骤、资源变化、Secret 生成方式
  -> 用户执行
  -> approval/MFA：仅在平台策略要求时出现
  -> progress：创建资源、等待就绪、运行健康检查
  -> result：连接状态、应用链接和可继续执行的独立动作
```

“去应用市场”只能作为用户明确要求浏览完整市场时的次要动作，不能替代上述安装
闭环。

### 12.2 诊断与自动修复的完整交互

```text
用户：为什么发布失败
  -> 查询 Release、Pod、事件和日志
  -> diagnosis：显示事实证据、最可能根因和其他发现
  -> 用户选择“修复镜像拉取凭据”
  -> form：选择已有凭据或新建凭据
  -> plan：展示 Deployment 引用变化
  -> approval：需要时确认
  -> progress：更新、等待 Rollout、检查事件
  -> result：验证成功或带新证据返回 diagnosis
```

修复完成不等于问题解决；必须执行验证步骤。验证失败时保留原始和新证据，避免
Agent 循环执行同一修复。

### 12.3 批量选择与部分成功

多选只用于 Tool Catalog 明确支持批量参数的操作。卡片组保存被选资源 ID 和各自
版本，提交后平台逐项返回：

```ts
interface BatchActionResultV1 {
  summary: {
    total: number
    succeeded: number
    failed: number
    skipped: number
  }
  items: Array<{
    id: string
    status: "succeeded" | "failed" | "skipped"
    code?: string
    requestId?: string
  }>
}
```

前端以 `result` 模板展示部分成功，允许只重试失败项。非批量 Tool 不能由浏览器
循环调用伪造成批量操作。

### 12.4 业务模板编译层

通用 `mode/template/cards` Schema 保留为底层表达能力，但模型处理高频场景时优先提交
`businessTemplate`。Agent 在持久化前把业务模板编译为同一个通用 Card Group，再执行现有
Zod 校验、Timeline 投影和前端渲染。前端不增加第二套协议，也不根据业务模板 ID 写特殊渲染。

首批业务模板：

| `templateId` | 交互职责 | 编译目标 |
| --- | --- | --- |
| `candidate_picker` | 从 2～5 个丰富候选中选择 | `interactive + catalog`，每候选一张可操作卡 |
| `candidate_select` | 从 6～50 个候选中选择 | `interactive + form`，使用单选字段 |
| `resource_configuration` | 在一轮内收集已知资源的结构化操作参数 | `interactive + form`，支持多个分组 section |
| `change_review` | 执行写操作前核对参数、影响和风险 | `interactive + plan`，不替代平台批准卡 |
| `diagnosis_report` | 展示诊断结论、检查项和证据 | `presentation + diagnosis` |
| `execution_progress` | 展示未结束任务的进度和步骤 | `presentation + progress` |
| `operation_result` | 展示已验证终态和回执 | `presentation + result` |
| `health_overview` | 汇总实时指标和分项健康 | `presentation + dashboard` |

业务模板选择依据是当前工作流阶段、是否等待用户、候选数量和证据类型，不是资源名称。
同一部署任务可以依次使用候选、配置、核对、进度和结果模板。只有关系图、代码、Diff、宽表格、
图表、条件字段或特殊多卡组合无法由业务模板表达时，模型才直接提供通用卡片结构。

`create_interaction_cards` 对模型暴露业务模板与通用卡片的联合输入，但执行器统一调用
`normalizeInteractionCardsInput` 编译并只持久化通用 V1 定义。这样模板可以独立演进，客户端
只维护一个渲染协议，审批、MFA、Secret 隔离和工具参数绑定也不产生旁路。

## 13. 卡片生成、传输与渲染时机

`create_interaction_cards` 仍按普通 Tool Call 流式传输，但前端不能渲染未完成的
JSON 参数：

```text
tool_call.started
  -> 展示固定尺寸的卡片骨架
tool_call.arguments.delta
  -> 只累计参数，不解析半截 Schema
tool_call.arguments.completed
  -> Agent 执行 Zod 校验并持久化
interaction_cards.created
  -> 前端拿到 cardGroupId、schemaHash 和已校验定义后渲染
interaction_cards.patched
  -> 仅服务端运行态、进度和结果可以增量更新
```

如果 Schema 校验失败：

- 不向用户展示损坏的半张卡片；
- Tool Call 显示稳定错误码和可展开的字段路径；
- 错误结果回灌模型，让 Agent 修正一次；
- 连续失败后回退为普通消息，不无限重试。

同一轮回复中的 Thinking、Message、Tool Call 和交互卡片继续按真实 Timeline 顺序
排列；卡片属于该轮唯一的 assistant 回复，不创建额外伪消息。

## 14. 前端交互要求

- 候选卡片默认展示摘要；配置表单渐进展开，小窗同一时间只展开一张。
- `catalog` 在宽窗口使用紧凑双列，窄窗口自动单列；不允许模型指定列数。
- `comparison` 在卡片区域内部横向滚动，外层消息列表不横向滚动。
- 内容块遵循模板的固定顺序；模型给出的非法组合在 Agent 侧直接拒绝。
- 小窗本体禁止横向滚动；字段、代码或来源内容需要横向空间时由自身容器滚动。
- 每个字段具有可见 label、说明、必填标识和就地错误信息。
- Enter 只在非输入法组合状态、非 textarea 且表单有效时提交。
- 所有交互支持键盘操作和可见焦点。
- 提交超过 300ms 显示加载状态，并阻止重复提交。
- 校验失败时聚焦第一个错误字段，错误同时提供文字和图标，不只依赖颜色。
- Secret 支持显示/隐藏和粘贴，不保存草稿。
- 卡片提交成功后展示新资源链接、Tool Call 状态和必要的后续操作。
- 卡片失败时保留用户的非 Secret 输入，展示稳定错误码、请求编号和可恢复操作。

实现时复用 shadcn/ui、React Hook Form 和 Zod，不创建第二套表单状态管理方案。

## 15. 暂不进入 V1

- 任意嵌套布局、栅格或模型自定义样式；
- 任意条件表达式和跨字段脚本，简单 `visibleWhen` 除外；
- 任意远程选项数据源；
- 文件上传、富文本和通用数组编辑器；
- 浏览器直接调用业务 API；
- 互联网搜索结果未经核验直接部署；
- 卡片旧版本兼容和迁移。

需要这些能力时，先提供第二个真实业务场景，再扩展当前 Schema。

## 16. 实施顺序

1. 把 `listAppTemplates` 和 `installAppTemplate` 纳入 Agent Tool Catalog，并补齐搜索
   参数、输入 Schema、风险策略和结果展示。
2. 实现 `create_interaction_cards` 的模板、内容块、字段、动作 Zod Schema，
   Timeline Item、SSE 事件和持久化。
3. 首批实现 `catalog`、`inspector`、`form`、`plan`、`progress`、`result` 模板，
   以及 Markdown、键值、列表、状态、表格、代码、时间线和进度内容块。
4. 实现 Web 动态输入渲染器、卡片提交端点和运行状态映射。
5. 用 PostgreSQL、Redis 和普通 Web 应用模板完成搜索、比较、安装、失败和重试的
   浏览器验收。
6. 实现 `diagnosis`、`comparison`、`approval` 和 `dashboard` 模板，并覆盖构建失败、
   发布回滚、连接测试和批量部分成功。
7. 实现后端 `searchWeb` 与 `fetchWebPage` Provider；平台模板无结果时才启用互联网
   回退。
8. 实现互联网应用候选核验，禁止未经验证的一键安装。
