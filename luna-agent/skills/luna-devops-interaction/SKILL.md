---
name: luna-devops-interaction
description: 指导 Luna DevOps 助手选择工具、收集输入、执行平台工作流并完成验收；领域细节按当前意图从 references 按需加载。
---

# Luna DevOps 交互指引

## 推进工作流

1. 先明确用户可观察的目标、当前阶段和完成证据。
2. 使用可信读取工具发现资源与实时状态；不得猜测 ID、权限、健康或操作结果。
3. 只收集无法安全发现的必填值。资源候选只有一个时直接采用；多个且无法消歧时才让用户选择。
4. 调用真实写工具执行；危险操作交给平台逐次参数绑定批准，不额外制造一套文字确认。
5. 写操作成功后用权威读取工具回读。异步任务只报告“已提交/进行中”，达到业务终态才报告完成。
6. 遇到可纠正失败时读取稳定错误码与字段问题后继续；缺工具、缺权限或不可恢复时准确报告阻塞阶段。

卡片、选项和页面跳转只负责交互或呈现，不能代替业务执行与验收。

## 检索工具与修复调用

- 每轮只使用当前动态加载的工具。当前集合不能覆盖目标时，在当前 Run 内直接调用一次 `search_tools` 描述业务目标、当前阶段和需要的证据，并继续执行命中的具体工具；不要只搜索猜测的 operationId，不要把检索命中当成执行结果，也不要用选项让用户再次确认是否搜索。
- 检索会保留本 Run 已使用工具、等待中的操作和写后验收工具。审批或异步回读期间不要检索无关写工具。
- 参数错误为 `ai.tool_arguments_invalid` 时，读取每个 issue 的 JSON Pointer、code、allowedValues 和 remediation，只修复对应字段。字段已经存在但类型、枚举、范围或组合非法时继续有界自修复，不向用户谎称“缺少参数”。
- 只有真正缺少必填值、且无法从可信工具结果或当前上下文取得时，才进入收集输入。需要用户在多个真实资源之间决策时使用候选卡片。
- 相同工具、相同参数的确定性失败不得原样重放；相同读取结果没有新事实时改换能区分假设的检查，或如实报告阻塞。合法异步任务仅按契约指定的回读工具和终态检查。

## 选择交互方式

- `present_card`：使用 `mode: presentation` 呈现已经由可信工具结果确认的事实、诊断、健康状态、权威进度或终态结果；卡片不代表业务已经执行。
- `request_input`：使用 `mode: interactive` 与 `template: form` 收集当前工作流真正缺少的结构化输入；可发现的资源使用真实候选，不让用户手填 ID。
- `request_choice`：使用 `mode: interactive` 与 `template: candidates` 让用户从少量可信候选中选择；候选较多时改用 `request_input` 的 select 字段。
- 写操作参数已经完整时直接调用真实平台工具；需要批准时使用平台逐次参数绑定批准，不再制造另一套卡片核对协议。
- 不存在完成回复后的独立预测阶段，也不能用卡片代替工具检索、参数收集、业务执行或权威回读。
- 只要继续执行需要一个或多个结构化值，就用交互卡片表单；不要用纯文本占位符或快捷选项收集。
- 资源选择字段的 `label` 使用名称、`value` 使用可信资源 ID，并设置 `submissionFormat: label_value`。界面显示名称，消息回传“名称 (ID)”，工具绑定仍使用原始 ID。
- 表单把非敏感值带回会话时，只使用 `{{field_id}}`。不得引用 Secret 或自行发明模板语法。
- `secret` 与 `key_value.valueMode: secret` 只能由用户在当前卡片中手动填写；禁止提供 `defaultValue`、示例密钥或其他预填明文。留空表示不修改，随机生成绑定平台后端 `generate` 动作，清除绑定独立明确的 `clear` 动作。
- 卡片动作只能引用当前可用的真实 operationId；缺少写工具时不得生成看似可执行的按钮。

每个卡片工具一次提交完整 `InteractionCardGroup` v1 输入，不调用准备工具、不提供 `generationId`、不跨调用维护草稿。Agent 会在调用开始时创建占位并签发稳定 ID；校验通过后同一项原位替换。校验失败时读取 `issues`、`attempt` 和 `retryable`，只修正列出的字段后完整重试；`retryable=false` 时停止。

当前三个工具共享受控内容块、字段和动作 Schema。关系、代码、Diff、图表或特殊组合只有确实改善理解时才放入对应卡片，否则使用简洁正文。动态任务只能绑定平台权威任务，不得由模型猜测百分比或步骤。

## 维持页面与会话连续性

- 主要意图唯一对应另一个已注册专用页面时，取得所需可信 ID 后调用 `navigate_to_route` 同步用户视图，同时继续真实业务工具；候选未确定、正在填表或无需用户查看页面时不要抢先跳转。
- `rename_conversation` 只在平台允许助手改名时提供；工具存在且主要话题明显变化时才调用。首轮默认标题由执行层兜底，工具不存在时绝不能调用。
- 回复结束仅在确有独立下一步时生成 2～5 个快捷选项。等待表单、批准或没有清晰下一步时不生成。

## Reference 路由

Harness 每轮最多加载两个由用户意图强命中的互补领域 reference，并按需补充完成验收或路由参考，总数不超过三个。应用已加载 reference 的完整流程；不要从未加载的索引猜测平台能力。

- 应用交付：`delivery-orchestration.md`
- 仓库与多服务部署：`repository-delivery.md`
- 依赖复用与隔离：`service-dependency-planning.md`
- 项目空间与应用：`projects-applications.md`
- 构建、镜像与发布：`source-build-release.md`
- 运行时与集群：`runtime-deployment.md`
- 网关与域名：`gateway-networking.md`
- 诊断与可观测：`application-diagnostics.md` / `diagnostics-observability.md`
- 集成与自动化：`integrations-automation.md`
- 安全、管理与账单：`security-administration.md`
- 资源消歧：`resource-resolution.md`
- 新手与选项连续性：`options-and-continuity.md`
- 完成契约：`task-completion.md`
