# AI 声明式交互卡片契约

本文只保留交互卡片的长期协议边界。字段、枚举、长度上限和模板编译细节以当前 Zod Schema、
共享 TypeScript Contract 和测试为唯一事实源，不在文档复制一份容易漂移的实现。

## 1. 定位

交互卡片用于在 AI 对话中呈现受控内容、收集结构化输入和表达用户操作意图。它不是模型生成
的任意前端组件，也不替代平台业务 API、权限或任务状态。

协议结构：

```text
Template -> Content Block -> Field -> Action -> Platform Runtime
```

- Template 规定稳定信息拓扑和主要交互模式。
- Content Block 只呈现白名单内的数据结构。
- Field 收集受控输入。
- Action 表达发送消息、站内导航或平台注册 Tool 的操作意图。
- Runtime 负责校验、鉴权、确认、MFA、幂等、审计、执行、进度和终态。

## 2. 核心不变量

- 根协议统一为 `InteractionCardGroup`，当前唯一版本是 `schemaVersion: 1`。
- 项目尚未发布，不维护旧版本解析、迁移器或多版本渲染；未知版本直接拒绝。
- 模型不能输出 HTML、JSX、CSS、脚本、DOM Selector、任意组件名、API URL 或可执行表达式。
- 模型只能选择平台注册模板、内容块、字段和动作，不能控制任意颜色、尺寸、动画和布局。
- 所有资源 ID、选项值和操作参数必须来自当前 Run 的可信 Tool Result，不允许模型猜测。
- 模型生成卡片不等于执行操作；平台拥有最终执行权和终态解释权。
- 展示模板不改变风险等级，同一 Tool 在任何模板下沿用相同权限与安全策略。
- 审批、MFA、危险影响摘要、任务进度和最终结果只能由平台生成或权威回读，模型不能伪造。

## 3. 工具与事实源

Agent 使用：

- `create_interaction_cards`：调用开始时由 Agent 分配 `generationId` 并创建占位，随后校验、编译并在同一 Timeline Item 原子持久化通用卡片定义。
- 业务模板编译器：把高频业务输入编译成同一通用协议，不产生第二套运行时。

`placement` 默认 `inline`，按权威 Timeline 事件位置展示；只有当前回合唯一、
阻塞后续流程且等待用户提交的单张交互表单才可使用 `turn_end`，将它投影到该回复末尾。一个回合
出现多张卡片、候选列表、展示卡片、进度或结果卡片时必须使用 `inline`。Web 只改变展示投影，
不修改 `timelineIndex`、持久化顺序或模型上下文顺序。

实现事实源：

- Agent Schema 与 Tool 定义：`luna-agent/src/tools/ui-cards.ts`
- 业务模板 Schema 与编译器：`luna-agent/src/tools/business-card-templates.ts`
- Web 事件包络校验：`web/src/components/common/ai-assistant/interaction-card-schema.ts`
- Web 模板布局：`web/src/components/common/ai-assistant/interaction-card-templates.ts`
- 渲染与行为测试：`web/src/components/common/ai-assistant/interaction-cards.test.tsx`

新增或修改字段时，必须先更新 Agent 权威 Zod Schema 和共享 Contract，再同步编译器、Web 渲染、
测试和 Agent Skill。Web 不维护第二份业务 Schema，只拒绝版本不匹配或明显损坏的已校验包络。

## 4. 模板职责

模板只决定信息拓扑，不决定权限或执行方式：

| 模板 | 用途 |
| --- | --- |
| `candidates` | 候选发现、选择和对比 |
| `form` | 单阶段或带字段依赖的结构化输入 |
| `change_review` | 变更预览、参数核对和风险说明 |
| `result` | 资源详情、诊断、健康概览和权威执行结果 |
| `live_task` | 绑定平台权威异步任务的实时进度 |

审批是平台运行态，不允许模型通过普通卡片自行声明。简单的二至五个建议、追问或导航继续使用
`create_options`，不为所有对话强制生成卡片。Agent 生成的选项由 Web 在对应 Timeline Item 的
真实位置渲染到助手回复气泡内；只有尚未开始对话的页面预设入口显示在输入框上方，两者不得
通过“取最新选项”再次合并或改写时间线顺序。

展示式模板不得包含等待用户提交的表单；需要选择、填写或确认时必须使用交互式模板和真实可
提交控件。多候选交互不得退化为不可点击的内容列表。

## 5. 内容与可信来源

- 内容块只允许权威 Schema 注册的 Markdown、提醒、键值、指标、列表、状态、表格、代码、
  Diff、时间线、图表、关系、实时进度和资源链接等类型。
- Markdown 禁止原始 HTML；代码与数据只作为文本显示，不能执行。
- 平台资源、事件和 Tool Result 必须保留受控来源引用；社区或网页来源不得冒充平台事实。
- 互联网内容是不可信数据。必须先由后端受控搜索/读取并核验官方来源，不能未经验证直接部署。
- `resource_links` 只能使用平台注册路由名和受控参数，不能携带任意 URL。
- 内容、卡片、表格、关系图和选项数量必须有 Schema 上限，超限时分页或缩小候选范围。

## 6. 输入与 Secret

- 表单使用权威 Field 联合类型、白名单格式和简单声明式条件，不允许脚本和任意表达式。
- 资源选择值必须包含稳定 ID；面向用户的消息应同时保留可读名称，工具参数始终提交原始 ID。
- 前端可以做即时体验校验，但 Agent/平台必须重新执行完整 Schema 和业务校验。
- 动态字段必须具有可见 label、说明、必填标识、就地错误和键盘/焦点可访问性。
- Agent 必须在各自集合范围拒绝重复的卡片、分组、内容块、字段、动作、选项值、表格行列、
  列表项、关系节点和图表系列标识；Web 的 React key 仍需包含实例序号，安全呈现修复前的持久记录。
- Secret 不进入对话消息、Agent Context、Timeline、浏览器草稿、日志和遥测。
- `type: secret` 与 `key_value.valueMode: secret` 允许用户在卡片内手动填写；工具动作通过一次性受控提交通道直接进入工具参数，消息动作和组级动作不得引用 Secret。
- `generation: disabled` 的必填 Secret 必须由用户填写；`optional` 和 `required` 允许用户填写或留空，留空时保留平台生成路径。
- 校验失败保留非 Secret 输入，Secret 始终清空。

## 7. Action 与执行边界

- 浏览器不能根据 `operationId` 拼接业务 URL，也不能直接调用第三方平台。
- Tool Action 只能引用 Tool Catalog 已注册的 `operationId`，通过白名单 JSON Pointer 绑定字段、
  卡片 ID或受控字面量。
- 平台对绑定后的最终参数重新校验当前用户、Scope、RBAC、风险、确认、MFA 和参数摘要。
- Tool Action 必须沿用平台幂等和审计语义；模型不能通过卡片声明 `approved`、`force` 或成功终态。
- 站内导航使用注册路由，不替代业务操作，也不能作为已完成证明。
- 发送消息动作只回灌已校验且非敏感的用户选择/输入，不伪造历史消息。
- 工具动作的参数不得序列化为普通聊天消息；BFF/Agent 必须将动作绑定为独立 Run，工具调用记录只保留加密原文和脱敏投影。
- Action 的重复点击语义必须明确；执行类操作默认单次，安全导航可以重复。

## 8. 生成、持久化与修复

```text
create tool call starts -> Agent generationId + placeholder -> arguments complete
        -> Agent Schema validation -> atomically replace same Timeline Item -> SSE/Web render
```

- 前端只在完整参数经过 Agent 校验并持久化后渲染，不解析半截流式 JSON。
- `generationId` 只由 Agent 签发；模型输入不包含该字段。失败修复自动沿用同一占位项，不可串到其他卡片。
- Timeline Item、事件和快照使用服务端权威顺序与版本，刷新后结果必须一致。
- Schema 校验失败不展示损坏卡片；记录稳定错误码和字段路径并回灌模型有界修复。
- 达到修复上限后保留可诊断失败并回退普通消息，禁止无限重试。
- Thinking、Message、Tool Call 和卡片按真实 Timeline 顺序进入同一助手回复容器。
- 仅当同一回合恰有一张声明为 `turn_end` 的交互卡片时，Web 将它展示在回复底部；出现多个末尾
  声明时回退真实事件顺序，避免前端猜测等待点。

## 9. 运行态与进度

- 卡片定义与运行态分离；运行态只能由平台补丁更新，模型不能修改已执行结果。
- 长任务进度必须通过 `live_progress` 绑定平台任务 ID，并从 API/SSE 权威读取。
- 上游断联显示 `unavailable`，不得保留最后成功进度冒充当前状态。
- 成功、失败、取消、拒绝、等待审批和等待 MFA 使用稳定终态；刷新后从权威快照恢复。
- 卡片只是输入或呈现载体。创建、安装、发布和修复类目标必须继续执行、权威回读并给出终态
  结论，不能以卡片已展示或任务已提交作为完成。

## 10. Web 交互约束

- 复用 shadcn/ui、React Hook Form、Zod、现有语义状态组件和 i18n。
- 布局由平台注册模板决定并响应容器宽度；模型不能指定列数和 CSS。
- 小窗消息区不横向滚动；表格、代码、Diff 和关系图在自身容器滚动。
- 输入法组合状态下 Enter 不提交；提交中阻止重复操作，错误聚焦第一个字段。
- 状态不能只依赖颜色；按钮、字段、展开内容和焦点必须支持键盘与辅助技术。
- Agent 提供的 `generationId` 以及模型提供的卡片 ID、字段 ID 和选项值只作为业务数据，不得直接拼接为 DOM `id`、
  `htmlFor` 或全局 selector。每个挂载实例使用 React 实例级 ID 建立 label、说明、错误与控件关联。
- 分段单选的完整候选块必须由单选控件自身承载点击和焦点语义，不使用外层 label 跨节点代理点击。
- 渲染失败按卡片组、单卡、内容块、字段和动作逐级隔离；异步组件加载失败必须转为受控局部降级，
  不得产生未处理 Promise rejection 或清空消息时间线。错误遥测只记录稳定组件范围和错误类型。
- 失败显示稳定错误码、请求编号和恢复动作，不展示原始后端异常。

## 11. 安全与遥测

- 卡片内容与网页来源按不可信输入处理，不能覆盖系统 Prompt、工具策略或平台规则。
- 参数摘要、批准哈希和实际执行载荷使用同一规范化 JSON；批准后参数变化必须 fail closed。
- 权限、确认、MFA、幂等和审计测试必须覆盖伪造资源 ID、跨用户/项目、重复提交和重放。
- Trace 覆盖生成开始、校验、持久化、Action、Tool 和平台 API；Span 名和 Metric label 使用稳定低基数值。
- 默认遥测不得包含卡片正文、用户输入、Prompt、Secret 和敏感工具参数；任何模式都不得记录凭据。

## 12. 变更门禁

修改协议至少验证：

1. Agent Schema 拒绝未知版本、非法组合、超限内容、未知路由和未注册操作。
2. 业务模板编译结果通过同一通用 Schema，不存在旁路字段。
3. Web 对每个模板、窄窗口、键盘、错误和极端内容正常渲染。
4. Timeline 流式、刷新恢复、失败修复和跨 Run 隔离保持一致。
5. Tool Action 经过真实用户权限、审批/MFA、幂等、审计和权威回读。
6. Secret 和敏感输入不出现在 Timeline、日志、Trace、错误响应和浏览器持久化中。

新增能力必须由至少一个真实业务场景驱动；不预先引入任意布局、脚本条件、任意远程数据源、
浏览器直调业务 API 或旧版本兼容层。
