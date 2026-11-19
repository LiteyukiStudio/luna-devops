# Luna DevOps AI 领域工具目录规格（ACI 防呆定稿）

- 日期：2026-11-19
- 设计基准：[2026-11-18-anthropic-aci-tool-design-rules.md](./2026-11-18-anthropic-aci-tool-design-rules.md)（`R1..R12` + 7 项核对表）与 [AGENTS.md](../AGENTS.md)
- 当前状态：全平台 5 批 7 大业务域合并落盘版（仅落盘工具规格说明，暂不编写实现代码）

## 全域工具规模与分层总览

将原先直接 1:1 暴露给 Agent 的 ~265 个碎片化前端 REST 接口，按业务意图与 **Poka-Yoke（结构性防呆）** 重构为独立领域门面工具层（Domain Tool Facade Layer）：

| 批次 | 领域前缀 | 核心职责范围 | `L0` 只读 | `L1` 会话可记忆 | `L2` 逐次绑定审批 | 小计 | 目录可见性 |
| --- | --- | --- | ---: | ---: | ---: | ---: | --- |
| 第 1 批 | `delivery_*` | 应用、应用市场模板安装、运行配置集、部署目标规格/配置/卷挂载、扩缩容/发布/回滚/重启 | 9 | 2 | 15 | **26** | `tenant` + `admin` |
| 第 2 批 | `build_*` | Git 凭据与仓库绑定/代码探查、制品库与推拉凭据、构建模板/变量集/环境、流水线与构建运行 | 11 | 1 | 17 | **29** | `tenant` + `admin` |
| 第 3A 批 | `runtime_*` | 运行集群、NetworkPolicy/出站例外/K8s 资源治理、容器排障/日志/Exec、数据卷与 Hook | 9 | 0 | 16 | **25** | `tenant` + `admin` |
| 第 3B 批 | `network_*` | 网关访问入口（HTTPRoute/TLS）、域名预检、服务间依赖（ServiceBinding）与依赖拓扑 | 4 | 1 | 6 | **11** | `tenant` + `admin` |
| 第 4A 批 | `workspace_*` | 项目空间、看板排序/收藏、计费归属转移、成员角色、项目网络隔离策略与跨项目内网共享 | 5 | 3 | 9 | **17** | `tenant` + `admin` |
| 第 4B 批 | `ops_*` | 全局检索/Dashboard、平台事件、个人账单、站内信/待办、个人/项目通知、账户安全与受控联网 | 10 | 3 | 12 | **25** | `tenant` + `admin` |
| **租户合计** | — | **普通用户（`tenant` 快照）可见的全部平台领域工具** | **48** | **10** | **75** | **133** | `tenant` |
| 第 5 批 | `admin_*` | 系统组件、GitProvider、全局构建环境、用户/准入/OIDC/SMTP、系统配置、AI 模型/可观测、全平台账单/调账、平台应用模板、全局通知、数据保留清理 | 11 | 0 | 22 | **33** | 仅 `admin`（`platform_admin`） |
| **全域合计** | — | **平台管理员（`admin` 快照 = `tenant + adminOnly`）可见的全部平台领域工具** | **59** | **10** | **97** | **166** | `admin` |

- **Agent 运行时内置协议与知识工具（`internalToolOperationIds`，共 12 个，由 `luna-agent` 本地编排，不走平台业务 API 目录）**：
  - 渐进式工具与技能加载（3）：`search_tools`、`get_tool_details`、`load_skill`
  - 公开文档知识库检索（3）：`search_docs`、`list_docs`、`read_doc`
  - 声明式交互卡片与前端导航（4）：`present_card`、`request_input`、`request_choice`、`navigate_to_route`
  - 异步任务挂起与会话元数据（2）：`wait_for_task`、`rename_conversation`
- **永久禁止暴露为 AI 工具的接口红线（`agentDisabledOperations` 显式黑名单 + 协议级过滤）**：
  - 认证、注册、浏览器重定向与回调协议：`getPublicConfigs`、`login`、`logout`、`requestEmailRegistrationCode`、`completeEmailRegistration`、`startOIDC`、`completeOIDC`、`startGitOAuth`、`completeGitOAuth`、`receiveGitWebhook`；
  - OAuth 交互授权、Token 交换与**第三方 OAuth 应用写操作**：`startOAuthDeviceAuthorization`、`getOAuthDeviceVerification`、`decideOAuthDeviceVerification`、`exchangeOAuthToken`、`revokeOAuthToken`、`getOAuthAuthorizationRequest`、`decideOAuthAuthorization`、**`createOAuthApplication`（响应含一次性明文 `clientSecret`）、`updateOAuthApplication`（可篡改 `redirectUris` 劫持回调且全量覆盖 `allowedScopes`）**、`rotateOAuthApplicationSecret`；
  - 用户本人主凭据签发与修改：`createAccessToken`、`updateMyPassword`；
  - 明文密钥回显、流式长连接/终端/二进制上传下载及内部计量上报协议：`*/runtime-secrets/reveal`、SSE/WebSocket/Terminal/Upload/Download、`createGatewayTrafficProbeHello`、`createGatewayTrafficUsage`、`createExternalBillingTransaction`、`retryVolumeTransfer`。

---

## 0. 全局通用约定与跨层执行架构契约（适用于全部领域工具）

0. **OpenAPI 唯一事实源、门面路由与跨层同步契约（遵守 `AGENTS.md §2 / §6 / §7`）**：
   - 全部 166 个 AI 领域门面工具统一在 `openapi/` 下声明为独立强类型 OpenAPI Operation（路由挂载于 `/api/v1/ai-tools/<domain>/...`，设置 `x-luna-cli: { hidden: true, requiredScopes: [...] }` 与 `x-luna-agent: { facade: true, adminOnly, approvalTier, effect, verification, purpose, avoidWhen, preconditions, successEvidence }`），底层复用各业务域共享 Go Domain Service（严禁拷贝业务逻辑，Web UI 与 `luna-cli` 既有 REST 路由保持原样不受影响）；
   - **`internal/aitool/openapi_catalog.go` 同步改造**：① `PlatformCatalog()` 仅导出声明了 `x-luna-agent.facade == true`（且不在 `agentDisabledOperations` 中）的领域门面工具，且在 `agentEligibleOperation` 中**优先放行 `x-luna-agent.facade == true` 再判断旧 `tag == "AI Assistant"` 过滤规则**（确保 `admin_inspect_agent_observability` 与 `admin_test_ai_integration` 不被误拦截，同时继续排除用户聊天 BFF 路由）；② 在构建测试期强制校验门面工具 `parameters` 与 `requestBody.properties` 无重名键，直接将 `requestBody` 属性展平合并至顶层 `InputSchema.properties`；③ 升级 `splitOperationID`（`openapi_catalog.go:539-553`）同时按 `_`（`snake_case`）与驼峰边界切词生成英文检索词与中英文别名；
   - **内部目录契约 Schema、`luna-agent/skills/domains/*/SKILL.md` 与 Web 前端工具展示名同步改造**：① 同步更新 `openapi/openapi.yaml` 中的内部契约 `AIPlatformToolCatalogOperation`（`additionalProperties: false`：新增 `adminOnly: boolean`；`verification.items` 新增 `whenArgumentEquals / whenResultEquals / collectionPath / staticArguments` 并从 `family` 枚举移除已退场的 `runtime_secrets / runtime_config_secrets`；补充 `/internal/v1/ai/tools/preview` 只读预检契约）；② 全量更新全部 6 个领域 Skill 文件 `luna-agent/skills/domains/*/SKILL.md`（`application-deployment`、`source-registry-build`、`runtime-volumes`、`gateway-topology`、`workspace-iam`、`diagnostics-governance`）中的工具名、扁平双模定位参数（移除禁止传 `identifier` 的旧约束）、独立 `delivery_scale_target` / 固定 `L2` 模板安装及已合并的构建日志/诊断入口；③ 同步升级 `web/src/components/common/ai-assistant/tool-display-name.ts` 与多语言词条（`web/src/i18n/locales/**`），支持 `<domain>_<verb>_<resource>` 门面工具名展示，杜绝时间线与 `L1/L2` 审批卡片退化为泛化文案 `platformOperation`。
1. **双模资源定位、展平顶层 Schema、会话项目沙箱与权威主键回显（`R8`）**：
   - `project`：输入支持项目主键 ID（`prj_xxx`）或唯一 `identifier`；**所有带项目作用域的工具响应体必须统一回显解析后的权威主键 `projectId: "prj_xxx"`**（非项目作用域的平台/个人级工具除外；`workspace_reorder_my_dashboard_projects` 回显有序权威 `projectIds: string[]`），供内置工具 `wait_for_task`、`present_card` 与 `outcome.ts` 写后校验直接引用。
   - **展平顶层 `InputSchema` 与项目会话沙箱全路径严禁 `fail-open`（防跨项目穿透、防混合路由误拒与防侧信道）**：
     - 领域工具目录注册器生成的 `InputSchema` 统一采用**顶层扁平参数结构**（不再套一层 `properties["body"]` 包装），确保顶层参数名与 `sensitivePaths`（如 `secretValues.*`、`upsertSecrets.*`）1:1 对齐；
     - Go 侧 `AIToolExecutionIdentityMiddleware`（`internal/api/aiapi/ai_tool_identity.go`）必须**全路径覆盖 `GET / DELETE`（`RequestBody == false`）与 `POST / PUT / PATCH`（`RequestBody == true`）**：① 在 `operation.Parameters`（`path / query`）及无请求体 Query 中同时识别 `project / projects` 与旧 `projectId / projectIds`（严禁只检查 `RequestBody == true` 导致 `GET` 只读工具 `fail-open`！）；② 当扫描展平顶层 `operation.InputSchema` 校验 JSON Body 中的 `project / projects / addProjects / removeProjects`（含嵌套 `upsertGrants[].project`）时，**自动排除已声明在 `operation.Parameters`（`path / query / header`）中的同名字段**（防止 `luna-api-client.ts` 将 URL 已消费参数从 JSON Body 剥离后误报缺失）；③ `aiRequestTargetsProductionStage` 同步识别展平顶层 `InputSchema["properties"]["stage"]` 与旧 `properties["body"]`；
     - 在项目绑定会话（`conversationProjectID != ""`）中：① **全部 `adminOnly == true`（第 5 批 33 个 `admin_*`）平台全局治理工具（及 `workspace_create_project`、`workspace_reorder_my_dashboard_projects`、`visibility="all"`）由 `AIToolExecutionIdentityMiddleware` 与 `/internal/v1/ai/tools/preview` 无条件前置拦截返回 `403 ai.project_scope_mismatch`，按 `R11` 指引切换至全局会话执行**（杜绝单项目会话跨项写系统项目/全局资源及后续写后验证死锁）；② 其余工具的沙箱校验**直接按主键加载 `conversationProjectID` 对应的唯一项目记录**，仅当请求传入的项目定位值精确等于该项目的 `id` 或 `identifier` 时放行（不做跨租户 `projects` 全表反查防侧信道）；③ 对于可选 `project` 只读工具省略 `project` 时自动注入 `conversationProjectID`（个人钱包工具 `#20/#21` 允许省略 `project` 查本人余额与 `ai.*` 费用）；④ 对于三层 Scoped（`user / project / global`）写/删工具在存量态与目标态各自独立强制 `(scope == "user" && ownerRef == currentUser.ID) || (scope == "project" && equalSingle(projectIDs, conversationProjectID))`（100% 阻断在单项目会话操作 `global` 资源或跨项目共享资源）。
   - `application`：支持应用主键 ID（`app_xxx`）或同项目唯一 `identifier`（响应回显 `applicationId: "app_xxx"`）。
   - `target`：支持部署配置主键 ID（`dplt_xxx`）、同应用唯一公开 `stage`（`"dev" | "test" | "staging" | "prod"`）或同应用唯一显示 `name`（响应回显 `deploymentTargetId: "dplt_xxx"` 与 `stage`）。
   - 其余关联资源（`cluster`、`configSet`、`projectVolume`、`repositoryBinding`、`targetRegistry`、`buildTemplate`、`variableSet`、`hook`、`channel` 等）均支持 ID 或同作用域内唯一名称；重名时统一返回 `409 ai.resource_ambiguous` 及候选列表 `{id, name}`。
2. **零全量覆盖、三态防呆与全链路 `sensitivePaths` 加密/脱敏闭环（`R5 / R6 / R7` + `AGENTS.md §2`）**：
   - 所有局部修改工具严禁要求模型重传完整数组或字典；任何字段省略或传入 `null` 均严格视为 **`No-Op`（保持现状，且数据库未暴露的底层高级列必须从存量记录完整回填保留）**；清空、移除或恢复继承态必须走显式 `remove* / clear* / reset* / detach*` 字段或显式枚举。
   - **敏感字段 Schema、Checkpoint 密文保护与全链路（含 `L2` 唤醒及模型出站适配器）脱敏强制契约**：
     - 任何写入 Secret / Token / 密码 / 私钥 / Kubeconfig 的字段必须与明文字段物理隔离，显式声明 `writeOnly: true` 与 `x-luna-sensitive: true`；对于字典型敏感参数（如 `secretValues: record<string, string>`、`upsertSecrets: record<string, string>`），**必须将 `writeOnly: true, x-luna-sensitive: true` 声明在 `additionalProperties` 值 Schema 上（生成 `*.secretValues.*` / `*.upsertSecrets.*` 路径）**，使审批快照保留键名而将值脱敏为 `"[REDACTED]"`；
     - **多工具并发批次挂起密文存储（修复 `ai.run_checkpoints.state` 明文落盘漏口）**：同一模型步返回多个工具调用且前序调用触发挂起（如 `L1/L2` 审批或 `wait_for_task`）时，`tool-step.ts` 的 `toPendingToolCalls` / `run-loop.ts` 写入 `ai.run_checkpoints.state.pendingBatchCalls` 的未执行工具 `arguments` **必须经 `PayloadCipher` 加密存储密文**（唤醒续跑时解密供实际执行，严禁明文落入 JSONB 列，亦不可直接脱敏破坏待执行密文）；
     - `luna-agent` 必须在 **PostgreSQL 持久化（`postgres-store.ts` 的 `ai.tool_calls.arguments` 与 `ai.items.content.arguments`）、`L2` 审批唤醒执行路径（`run-loop.ts` `resumeToolCall()` 推入 `toolState.continuationMessages` 前）、中断批次崩溃恢复（`tool-step.ts` `recoverInterruptedBatch`）、同 Run 后续推理步历史工具调用回传（`tool-step.ts` `commitCallResult` 的 `state.continuationMessages`）、内存/事件投影层（`orchestrator.ts` `ProjectingToolCallStore.emit`、`parameterSummary` 与审批请求载荷）、Timeline 投影（`timeline-presenter.ts`）以及 OTel Span** 全链路统一调用 `redactSensitivePaths(args, operation.sensitivePaths)`；
     - **模型出站硬门禁（纵深防御）**：`provider/openai-chat-completions.ts`（及其子类 `deepseek-chat-completions.ts`）的 `assistantPayload()` 在将 `tool_calls[].function.arguments` 序列化发送给外部 AI Provider 前，必须结合工具 `sensitivePaths` 与 `redact` 强制执行出站脱敏兜底，100% 阻断任何分支将解密后的密钥、证书或私密文件内容回传第三方大模型！
3. **毁灭性操作物理独立、防会话记忆穿透、幂等键与 `R12` 审批前服务端预检协议（`R3 / R11 / R12`）**：
   - 所有删除、归档、解绑、吊销操作均为独立 `*_delete_* / *_remove_* / *_revoke_* / *_unbind_* / *_archive_*` 工具，审批级别固定 `L2`（逐次参数绑定审批，严禁混入 `previewOnly` 参数）；所有同步删除/解绑工具废除空 `204`，统一返回结构化 JSON（如 `{ id, deleted: true, projectId? }` 或 `{ id, unbound: true, projectId }`）供写后权威回读确认。
   - **请求头 `Idempotency-Key` 优先消费**：非天然幂等的异步创建与财务写工具（如 `runtime_create_volume`、`runtime_create_volume_export`、`admin_adjust_user_wallet`）统一优先消费 `luna-agent` 请求头传入的确定性 `Idempotency-Key`（`aiidem_<sha256>`），确保未知网络超时经 `retryFailed` 换新 `toolCallId` 重试时绝不重复创建资源或重复入账/扣款。
   - **整段会话记忆防穿透红线**：`luna-agent` 的 `L1`“允许并记住”落表于 `ai.conversation_tool_approvals(conversation_id, operation_id)`（**跨 Run 整段会话级按 `operationId` 免审**），且双模定位（如 `target: "dplt_xxx"`）在编排层无法静态区分生产环境。因此**凡涉及工作负载变动、重启、扩缩容、独占资源锁（PVC）、密钥/配置变更、发版、网络暴露、外部发信探测、平台管理变更的操作一律固定为 `L2`**；仅纯元数据新建或非破坏性个人视图偏好（全域共 10 个）允许设为 `L1`。
   - **`L2` 审批前服务端只读预检协议（`POST /internal/v1/ai/tools/preview`，含交互卡片 `direct` 路径统一收敛）**：
     - 无论是模型发起的 `L2` 工具调用，还是前端交互卡片 `type: "tool"` 触发的 `prepareDirectAction`（`server.ts` / `orchestrator.ts` 入队阶段一律保持 `status: "proposed"`，严禁在 HTTP 入队阶段跳过预检直接标为 `awaiting_approval`），统一由 `RunExecutor` 在 `running` 态转入 `awaiting_approval` 前调用 Go 内部只读预检端点 `/internal/v1/ai/tools/preview`：
     1) 若参数非法、目标禁用或存在 `409` 依赖冲突（如入站 `ServiceBinding`、活跃 `Release` 进行中、HPA 开启），直接按 `R11` 返回结构化自纠错错误，**绝不向用户弹出注定失败的无效审批卡片**；
     2) 预检通过时返回结构化 `approvalPreview`（含 `before -> after` 明文 Diff、敏感键名变更清单、级联清理或解绑的 `GatewayRoute / ProjectVolume / DeploymentTarget` 影响范围、发版/回滚 `currentImage -> targetImage`、数据保留匹配行数），并**将预检锁定的确定性执行载荷（如发版解析出的 `resolvedImageRef / resolvedBuildRunId`、回滚 `resolvedToReleaseId`、数据卷 `expectedRevision / dataAction`）封装为绑定 `(runId, toolCallId, operationId, argumentsHash)` 的独立不透明 HMAC 签名令牌**持久化在审批元数据中；批准执行时由 `HttpLunaApiToolClient` 通过独立内部请求头 `X-Luna-AI-Preview-Lock` 透传给 Go 端校验消费（**绝不修改 `call.arguments` 或 `argumentsHash`**，既兼容 `additionalProperties: false` AJV 校验，又严防审批期间并发变更导致 TOCTOU 版本漂移）。
4. **生效时机透明化、异步任务与写后权威回读验证契约（`R9 / R10` + `outcome.ts` 对齐）**：
   - 凡修改了需创建 `Release` 才下发 K8s 的模板/规格/配置字段，响应返回 `requiresRelease: true` 与待发布摘要；仅修改控制面立即生效字段时返回 `requiresRelease: false`；
   - **异步任务挂起与进度卡片契约（含 `R8` 双模项目解析对齐）**：
     - 仅返回 `buildRunId`（`taskType: "build_run"`）或 `releaseId`（`taskType: "release"`）的异步工具可调用内置 `wait_for_task(projectId, taskType, taskId)` 挂起等待（多组件模板安装若需等待全部组件就绪，可遍历 `components[].releaseId` 依次等待）；`projectapi / Host` 新增双模项目鉴权入口 `AuthorizeProjectRef`（按 `(id = ? OR identifier = ?) AND deleted_at IS NULL` 解析项目并完成 RBAC，将权威 `project.ID` 透传给 `GetAIProgress`、`StreamAIProgress` 的 `resolveAIProgress` / `MonitorProjectAuthorization` 以及未绑定会话的 `GetAITaskStatusInternal`）；在项目绑定会话（`binding.ConversationProjectID != ""`）中，`GetAITaskStatusInternal` **严格遵循 §0.1“先沙箱、后 RBAC”顺序**：先按主键 `binding.ConversationProjectID` 单行加载唯一会话项目，仅当 `:projectId` 精确等于 `convProject.ID` 或 `convProject.Identifier` 时放行（否则立即返回 `403 ai.project_scope_mismatch`，零跨租户表扫描），随后调用 `AuthorizeProjectByID(ctx, convProject.ID, requiredAction)` 与 `resolveAITaskStatusInternal(ctx, convProject.ID, taskType, taskID)`；
     - 支持通过内置 `present_card(template: "live_task")` 内嵌 `live_progress` 内容块绑定的异步操作类型严格对齐现有 `binding.operationType` 枚举：`"build_run" | "release" | "hook_run" | "app_template_installation"`；
     - 数据卷传输（`transferId`）与异步资源删除（`deleteStatus: "deleting"`）**不走 `wait_for_task` 或 `live_progress`**，仅返回当前异步状态供按需查询详情确认。
   - **写后权威回读验证（`openapi_catalog.go` + `catalog.ts` + `domain.ts` + `outcome.ts` 全 Family 同步升级）**：
     - 升级验证器与 `.strict()` `toolVerificationRuleSchema`，**彻底移除 `outcome.ts` 对旧 `call.arguments.body` 嵌套及旧 `projectId / applicationId / routeId / pageSize: 100` 的硬编码**：
       1) **项目定位与分页封顶**：全 Family 统一按 `result.projectId ?? call.arguments.project ?? firstElement(result.projectIds) ?? firstElement(call.arguments.addProjects)` 提取 `project`，并按目标只读工具 `inputSchema.properties.pageSize.maximum` 动态封顶注入 `pageSize`（`<= 50`）；
       2) **全部专用 Family 映射对齐新门面工具**：`build_run` -> `{ project, buildRunId: result.buildRunId ?? result.id ?? call.arguments.buildRunId }`（`build_get_run`）；`release` -> `{ project, releaseId: result.releaseId ?? result.primaryReleaseId ?? result.release?.id ?? result.id ?? call.arguments.releaseId }`（`delivery_get_release`）；`gateway_route` -> `{ project, route: result.routeId ?? result.id ?? call.arguments.route }`（`network_list_gateway_routes`）；`application` -> `{ project, application: result.applicationId ?? result.primaryApplicationId ?? result.application?.id ?? result.id ?? call.arguments.application }`（`delivery_get_application`）；`deployment_target` -> `{ project, application: result.applicationId ?? call.arguments.application, target: result.deploymentTargetId ?? result.primaryDeploymentTargetId ?? result.id ?? call.arguments.target }`（`delivery_get_target`）；`project_order` -> 从 `result.projectIds`（回退 `call.arguments.projects`）读取期望有序主键数组并调用 `workspace_list_projects(sortBy: "dashboardOrder", sortOrder: "asc", page: 1, pageSize: 50)`；`billing_rate_rules` -> 从顶层 `call.arguments.rules` 读取局部变更项，合并 `staticArguments: { view: "rate_rules" }` 调用 `admin_inspect_billing` 并仅对本次显式传入的 `creditsPerUnit / enabled` 子集做局部一致性断言；
       3) **通用 `resource` Family、密钥键名断言与多态规则分发**：退场已废弃的独立 `runtime_secrets / runtime_config_secrets` family（项目运行配置集 `#14 / #15 / #16` 统一绑定单条权威详情工具 `delivery_get_config_set(project, configSet)`），在 `deployment_target` 与通用 `resource`（`resourceVerificationForCall`）两个分支中统一支持从顶层 `secretActions / secrets` 提取 `requiredKeys / absentKeys` 对照回读结果的 `secretKeys`（如 `delivery_get_target`、`delivery_get_config_set`、`build_list_variable_sets`、`build_get_scoped_env`、`admin_get_global_build_env`）完成加密键名增删校验；新增 `whenArgumentEquals?: Record<string, string | boolean>` 与 `whenResultEquals?: Record<string, string | boolean>` 条件匹配（支撑 `ops_revoke_my_credential_or_grant` 与 `admin_delete_platform_notification_resource` 多态精确路由），支持点分与 `[]` 数组穿透集合路径 `collectionPath`（如 `"accounts.items"`、`"items[].credentials"`、`"tokens"`）及规则级静态参数 `staticArguments`；
       4) **退出码判定收窄与角色隔离**：将非零 `exitCode` 判定精确限定为 `operation.operationId === "runtime_exec_command"`（严禁误杀其他 `effect === "execute"` 探测工具）；所有 `write / async` 领域工具均声明同角色快照内可见的 `L0` `verification` 规则（`tenant` 工具严禁引用 `admin_*` 工具），彻底杜绝 `ai.verification_inconclusive` 假失败。
   - 凡读取 K8s 或外部平台实时状态的工具，统一设置 `Cache-Control: no-store`，不可达时返回 `status: "unavailable"` + 稳定 `observationCode`。
5. **角色隔离双确定性目录快照与 Prompt KV Cache 保活契约（`R4`）**：
   - Go `/internal/v1/ai/provider-config` 为每个领域工具显式输出 `adminOnly: boolean` 元数据，同时同步扩展 Go BFF -> Agent HMAC 签名上下文 `ActorContext.platformRole`（`ai_proxy_handlers.go` 与 `luna-agent/src/auth.ts`）；`luna-agent` 的 `ToolCatalogRegistry` 维护确定性排序的 **`tenant` 快照**（物理剔除全部 `adminOnly` 工具）与 **`admin` 快照**（`tenant + adminOnly`）及各自独立的 `toolCatalogDigest`，`retain()` 同时保活两个活跃快照 Digest；
   - HTTP 入队（`/turns`、`/tool-actions`）按签名 `actor.platformRole` 绑定对应快照，后台 `RunExecutor` 在 `claimNextRun` 与唤醒恢复时实时回查 PostgreSQL `users(role, disabled)` 与 `user_sessions(run.actorSessionId)` 解析 `catalogRegistry.forRole(liveRole)`（若降权导致 `liveCatalog.digest !== run.toolCatalogDigest` 立即调用 `updateRunToolCatalogDigest` 持久化更新 `ai.runs.tool_catalog_digest` 同步拦截挂起中的 `admin_*` 调用），并在 `ToolStepRunner.initialState()` 对 `checkpoint` 恢复分支与跨轮继承分支的 `run.selectedOperationIds`、`executedOperationIds`、`reconstructSelectedOperationIds` 及 `restoredInternalToolResults` 一律按当前实时角色目录过滤裁剪并持久化回写（释放 16 槽位配额），既保证同角色跨轮 Prompt KV Cache 100% 命中，又彻底杜绝普通租户可见性污染与管理员中途降权后的跨轮缓存穿透。

---

## 第 1 批：`delivery`（应用、模板、部署配置、运行配置集与发布域，共 26 个工具）

> 跨批次边界说明：部署目标上的 CI/CD 构建流水线细节（Dockerfile/构建模板切换、构建参数、构建资源、构建变量集、自动构建分支/Tag 规则、发版门禁 `requireApproval`、构建/发布生命周期 Hook 绑定）可由第 2 批 `build_configure_target_pipeline` 完整维护（`governance.requireApproval` 在本批亦支持同步读写）；应用间 `ServiceBinding` 与网关域名路由归入第 3 批 `network` 域；Pod 日志/事件/Exec/诊断快照归入第 3 批 `runtime` 域。
> 实现层增量说明（严禁直接透传旧 REST DTO）：本批工具底层调用共享 Go Domain Service，统一将领域冲突错误码（如 `deployment.runtime_environment_key_conflict`、`deployment_target.not_deployed`、`deployment_target.disabled`）规范化为 `409 Conflict` 结构化错误返回。

### 1.1 只读发现与权威回读层（9 个，固定 `L0`）

**1. `delivery_list_applications`** — 分页列出项目内应用及各环境运行状态摘要
- `project`: `string`，必填，项目 ID 或 `identifier`
- `search`: `string`，可选，按名称或标识模糊搜索
- `includeRuntime`: `boolean`，可选，默认 `true`，返回各环境实时健康度、`observationCode` 与主访问 URL
- `sortBy`: `"createdAt" | "name" | "identifier"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**2. `delivery_get_application`** — 查看单个应用详情、下属部署目标清单摘要、服务依赖计数与绑定数据卷概览
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- 实现契约说明：Service 层在应用详情基础上补齐下属每个 target 的定位与状态字段 `{id, name, stage, clusterId, effectiveClusterId, clusterName, workloadType, sourceType, enabled, requireApproval, deleteStatus, deleteCode, deployed, desiredReplicas, readyReplicas, status, observationCode, currentRelease}`，并汇总入站/出站 `ServiceBinding` 数量与已绑定 `ProjectVolume` 列表（替代独立的 `list_targets` 与删除预检接口）。

**3. `delivery_get_target`** — 查看单个部署目标的完整期望配置（含运行时配置与构建流水线配置）及 K8s 实时副本状态
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId`（`dplt_xxx`）、`stage`（`"dev" | "test" | "staging" | "prod"`）或唯一 `name`
- `includeImageCandidates`: `boolean`，可选，默认 `false`，按需拉取最近成功构建及关联镜像仓候选 Tag（上限 `30` 条，外部镜像仓不可达时返回 `registryAvailable: false` 与回退候选）
- 实现契约说明：完整返回：
  1) 运行时规格与治理：`clusterId` 与解析后的权威 `effectiveClusterId`、算力请求（说明容器 Limit 由所属集群超卖比自动派生）、`imageRef`、`imagePullPolicy`、HPA 配置（含 `autoScalingBehavior` 摘要）、端口、探针、命令/参数、`securityAndScheduling`、`internalOnly`、`governance: {enabled, webConsolePolicy, requireApproval}` 及存量高级 K8s 配置非空摘要；
  2) 配置与存储：公有环境变量键值、**已配置加密环境变量键名数组 `secretKeys: string[]`**、明文配置文件列表、**已配置加密文件路径数组 `secretFilePaths: string[]`**（从 `SecretFiles` JSON 字典提取排序路径键名，绝不暴露内部引用串）、绑定配置集（含 `live | snapshot` 模式）、数据卷挂载（含 `logicalName` 与 `activationState`）；
  3) 构建流水线完整子结构 `buildPipeline`（含 `sourceType, repositoryBindingId, buildDefinitionMode, buildTemplateId, buildTemplateVersion, buildTemplateValues, dockerfilePath, buildContext, buildDirectory, buildArgs, buildCpuRequest, buildMemoryRequest, buildTimeoutSeconds, targetRegistryId, targetRepository, targetTag, buildEnvironmentId, buildVariableSetIds, autoDeploy, requireApproval, buildConcurrencyPolicy, branchPattern, tagPattern, buildHooksEnabled, buildHookBindings`；注：部署级 `BuildEnvironmentConfig` 明文变量与密钥键名严格受 `ActionSecretViewValue` 门控，对无该权限的 `Developer / Viewer` 隐藏变量明细，作为第 2 批 `build_configure_target_pipeline` 的权威只读事实源）；
  4) 集群实时副本数与 `observationCode`。

**4. `delivery_list_templates`** — 分页搜索或浏览租户可安装的应用模板目录摘要
- `query`: `string`，可选，按模板名称、标识或简介搜索
- `category`: `"collaboration" | "database" | "developerTool" | "middleware" | "observability" | "security" | "storage"`，可选，精确对齐平台 7 类模板枚举
- `sortBy`: `"popularityWeight" | "name" | "createdAt"`，可选，默认 `"popularityWeight"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`，遵循 `AGENTS.md §7` 统一分页结构 `items/page/pageSize/sortBy/sortOrder/total/totalPages`）
- 实现契约说明：自动过滤排除 `kind == "system_component"`（或 `systemComponent != ""`）的平台系统探针模板（系统组件仅在 `admin` 域暴露）。

**5. `delivery_get_template`** — 读取单个应用模板的完整参数定义（普通 `values` 与敏感 `secretValues` 分类）及数据卷要求
- `template`: `string`，必填，模板 ID 或 `slug`（若指向 `system_component` 系统模板则返回 `404`）
- 备注：安装模板前必调；返回参数定义（明确区分 `secret=false` 的普通参数与 `secret=true` / `autoGenerate=true` 的敏感参数）、单组件顶层 `dataVolumes` 要求（对应安装参数 `projectVolume`）及多组件 `components[].key` 的 `dataVolumes` 要求（对应安装参数 `componentVolumes`）。

**6. `delivery_list_releases`** — 分页查询发布历史记录
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，可选，应用 ID 或 `identifier`
- `target`: `string`，可选，传入 `application` 时支持 `targetId | stage | name`；未传 `application` 时支持 `targetId` 或公开 `stage`（跨应用按阶段过滤；若传非公开自定义 `name` 且跨应用重名则返回 `409 ai.resource_ambiguous`）
- `status`: `"pending" | "running" | "succeeded" | "failed" | "canceled"`，可选（Service 层补齐 `status` 过滤条件）
- `sortBy`: `"createdAt" | "revision" | "status"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**7. `delivery_get_release`** — 查看单次发布详情及尾部截断的权威发布日志
- `project`: `string`，必填，项目 ID 或 `identifier`
- `releaseId`: `string`，必填，发布记录 ID（`rel_xxx`）
- `includeLogs`: `boolean`，可选，默认 `true`，读取 `release_logs`（含 pre/post Hook 与 K8s Rollout 进展）
- `logTailBytes`: `integer`，可选，默认 `8192`（上限 `32768`），超长按 UTF-8 字符边界截取尾部并标注截断字节数

**8. `delivery_list_config_sets`** — 分页列出项目共享运行配置集摘要
- `project`: `string`，必填，项目 ID 或 `identifier`
- `search`: `string`，可选，按名称、变量或文件路径搜索
- `sortBy`: `"createdAt" | "name"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**9. `delivery_get_config_set`** — 查看单个项目运行配置集的公有变量、明文文件及加密键名/加密文件路径列表
- `project`: `string`，必填，项目 ID 或 `identifier`
- `configSet`: `string`，必填，配置集 ID（`prcs_xxx`）或同项目唯一名称
- 实现契约说明：Service 层新增单条详情组装，返回公开 `environmentVariables`、明文 `configFiles`、`secretKeys: string[]`（来自 `SecretRefs`）与 `secretFilePaths: string[]`（来自 `SecretFiles` 排序路径键名，绝不暴露引用串）及 `affectedDeploymentTargetCount`。

---

### 1.2 应用与模板编排层（4 个）

**10. `delivery_create_application`** — 创建新应用（`L1`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `identifier`: `string`，必填，应用唯一英文标识（DNS/slug 规范，创建后不可修改）
- `name`: `string`，必填，应用显示名称
- `icon`: `string`，可选，内置 32 种图标枚举名（如 `"server"`、`"database"`、`"globe"`、`"box"`）或合法 `http(s)://` / 站内图标路径（非法图标名严禁静默回退 `"box"`，按 `R11` 返回 `argument_invalid` 与允许列表）

**11. `delivery_update_application`** — 修改应用名称或图标（`L1`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `name`: `string`，可选，新显示名称
- `icon`: `string`，可选，新图标（校验规则同创建，传空串不修改）
- `resetIcon`: `boolean`，可选，重置为默认图标 `"box"`

**12. `delivery_delete_application`** — 异步删除应用及其下属部署环境（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- 备注：`/internal/v1/ai/tools/preview` 预检阶段若发现入站 `ServiceBinding` 直接返回 `409` 阻塞依赖列表，无冲突则在 `L2` 审批卡片展示将清理的环境数及将解绑保留数据的 `ProjectVolume` 列表；执行后返回 `{ id: "app_xxx", applicationId: "app_xxx", projectId: "prj_xxx", deleteStatus: "deleting" }`。

**13. `delivery_install_template`** — 从应用模板安装单组件或多组件应用（固定 `L2`，防止会话记忆后批量建应用与抢占锁定 PVC）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `template`: `string`，必填，租户应用模板 ID 或 `slug`（拒绝 `system_component` 系统模板）
- `stage`: `"dev" | "test" | "staging" | "prod"`，必填，目标环境
- `applicationName`: `string`，可选，省略默认使用模板名称
- `applicationIdentifier`: `string`，可选，省略自动生成（多组件模板下作为各组件应用 `identifier` 前缀）
- `cluster`: `string`，可选，目标集群 ID 或名称（省略使用项目默认集群）
- `values`: `record<string, string>`，可选，仅传模板中 `secret=false` 的普通参数键值对（误传 `secret=true` 敏感键名时按 `R11` 报错指引改传 `secretValues`）
- `secretValues`: `record<string, string>`，可选，仅传模板中 `secret=true` 的敏感参数键值对（**在 `additionalProperties` 值 Schema 上声明 `writeOnly: true, x-luna-sensitive: true`**，生成 `secretValues.*` 敏感路径全链路脱敏；省略时对 `autoGenerate=true` 的密钥由服务端自动随机生成）
- `projectVolume`: `string`，可选，**单组件模板**需挂载持久化卷时传入数据卷 ID 或 `displayName`（与多组件 `componentVolumes` 互斥）
- `componentVolumes`: `record<string, string>`，可选，**多组件模板**按 `components[].key` 映射各组件独立数据卷 ID 或 `displayName`（校验不重用同一卷、同集群且 `Filesystem/Block` 模式匹配）
- `installNow`: `boolean`，可选，默认 `true`
  - 返回与验证契约：无论单组件还是多组件模板，统一返回非空数组 `components: array<{key: string, applicationId: string, deploymentTargetId: string, releaseId?: string}>`、权威 `projectId`、`installationId`（供 `present_card(template: "live_task")` 内嵌 `live_progress(operationType: "app_template_installation")` 绑定），并**始终在顶层回显入口主组件 `primaryApplicationId`、`primaryDeploymentTargetId` 及（当 `installNow=true` 时）`primaryReleaseId`**，使单主键 `RunToolVerification`（`whenResultHas: "primaryReleaseId" -> family: "release"`，回退 `family: "application"`）100% 确定性闭合；`installNow=false` 时返回 `requiresRelease: true`。

---

### 1.3 项目共享运行配置集写操作层（3 个，固定 `L2`）

**14. `delivery_create_config_set`** — 创建项目共享运行配置集（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `name`: `string`，必填，配置集名称
- `enabled`: `boolean`，可选，默认 `true`
- `publicEnv`: `array<{key: string, value: string}>`，可选，初始普通环境变量（键名须匹配 `^[A-Za-z_][A-Za-z0-9_]*$`，且不得与 `secrets` 同键名冲突）
- `secrets`: `array<{key: string, operation?: "set" | "generate", value?: string, encoding?: "base64" | "hex" | "alphanumeric" | "numeric", length?: integer}>`，可选，初始加密环境变量（`value` 声明 `writeOnly: true, x-luna-sensitive: true`；`operation` 省略时默认 `"set"` 以原生兼容前端 `request_input` `KeyValueInput` `{key, value}` 绑定，且 `operation=="set"` 时强制校验 `value` 非空，严禁静默跳过返回空成功；`length` 范围 `8..256` 默认 `32`；单事务内串联配置集创建与加密存储）
- `configFiles`: `array<{path: string, content: string}>`，可选，初始明文挂载文件（非根绝对路径，校验本列表内部无重复或父子路径前缀冲突，且与 `secretFiles` 无同路径或父子路径冲突）
- `secretFiles`: `array<{path: string, content: string}>`，可选，初始加密挂载文件（`content` 声明 `writeOnly: true, x-luna-sensitive: true` 且强制非空，校验本列表内部无重复或父子路径前缀冲突，且与 `configFiles` 无同路径或父子路径冲突）

**15. `delivery_configure_config_set`** — 纯增量修改项目共享运行配置集（`L2`，绝不全量覆盖）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `configSet`: `string`，必填，配置集 ID 或唯一名称
- `name`: `string`，可选，重命名配置集
- `enabled`: `boolean`，可选，启停配置集
- `setPublicEnv`: `array<{key: string, value: string}>`，可选，新增或更新普通变量（与已存加密变量冲突时统一返回 `409 deployment.runtime_environment_key_conflict`）
- `removePublicEnvKeys`: `string[]`，可选，删除指定普通变量
- `secretActions`: `array<{key: string, operation?: "set" | "generate" | "clear", value?: string, encoding?: "base64" | "hex" | "alphanumeric" | "numeric", length?: integer}>`，可选，单键写入/随机生成（`length: 8..256`）/清除加密变量（`value` 为 `writeOnly + x-luna-sensitive`；`operation` 省略时默认 `"set"`，且有效 `operation=="set"` 时 `value` 必须非空，空串直接报 `400 argument_invalid` 严禁静默跳过）
- `upsertConfigFiles`: `array<{path: string, content: string}>`，可选，按非根绝对路径新增或更新明文文件（校验对象内明文文件间无父子路径前缀冲突，且与已存/本次 `secretFiles` JSON 字典路径键无同路径或父子路径冲突）
- `removeConfigFilePaths`: `string[]`，可选，按绝对路径删除明文文件
- `secretFileActions`: `array<{path: string, operation?: "set" | "clear", content?: string}>`，可选，按非根绝对路径写入或清除加密文件（`content` 为 `writeOnly + x-luna-sensitive`；`operation` 省略默认 `"set"`，且有效 `operation=="set"` 时 `content` 必须非空；校验对象内加密文件间无父子路径前缀冲突，且与已存/本次 `configFiles` 无同路径或父子路径冲突）
- 备注：当修改后的配置集处于 `enabled == true`（含本次将 `enabled: false -> true` 重新启用的场景）时，按修改后的完整配置集键值与文件路径，对所有通过 `mode == "live"` 绑定本配置集的活跃 `DeploymentTarget`（合并该目标自身、该目标同时绑定的其他已生效 `live / snapshot` `ConfigSet`、`dataVolumes` 及活跃出站 `ServiceBinding`）同步执行前置校验——① 变量/密钥键不得与目标活跃出站 `ServiceBinding` 完整注入键集合（`URLEnvVar / HostEnvVar / PortEnvVar` + `SecretMap[].SourceEnvVar`）冲突（`409 service_binding.env_conflict`）；② 跨层同类文件（`configFiles <-> configFiles`、`secretFiles <-> secretFiles`）相同 `path` 视为合法分层覆盖（对齐 `mergeRuntimeConfigFiles`），仅拦截三类真实 K8s 挂载冲突（`409`）：异类文件（`configFiles <-> secretFiles`）同路径冲突、任意两个挂载文件（`configFiles ∪ secretFiles`）父子路径前缀冲突、以及任意挂载文件与目标 `dataVolumes.mountPath` 的同路径或父子路径冲突；返回 `affectedDeploymentTargetCount` 与 `requiresRelease: true`（`live` 绑定目标需重新发版生效；`snapshot` 绑定目标需刷新快照后发版）。

**16. `delivery_delete_config_set`** — 异步删除项目共享运行配置集（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `configSet`: `string`，必填，配置集 ID 或唯一名称
- 备注：`/internal/v1/ai/tools/preview` 预检阶段在 `L2` 审批卡片明确展示将被自动解绑 `live` 模式引用的 `affectedDeploymentTargetCount` 及受影响部署目标列表；执行后返回 `{ id: "prcs_xxx", projectId: "prj_xxx", deleteStatus: "deleting" }`（对齐 `id.New("prcs")`，写后校验绑定 `#9 delivery_get_config_set`）。

---

### 1.4 部署目标期望配置层（5 个，固定 `L2`，按实际改动字段返回精确 `requiresRelease: boolean`）

**17. `delivery_create_target`** — 创建部署环境目标，支持镜像源、代码仓库源或跨环境安全克隆（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `stage`: `"dev" | "test" | "staging" | "prod"`，必填，同应用每环境最多 1 个活跃目标
- `name`: `string`，可选，省略默认同 `stage`
- `cloneFromStage`: `"dev" | "test" | "staging" | "prod"`，可选，从同应用已有环境克隆非敏感配置
- `volumeOverrides`: `record<string, string>`，可选，配合 `cloneFromStage` 使用，按源挂载 `logicalName` 映射目标环境独占的新 `projectVolume`（ID 或 `displayName`，须与目标集群及 `VolumeMode` 兼容）
- `omitProjectVolumes`: `boolean`，可选，配合 `cloneFromStage` 使用，显式跳过源环境的持久化卷挂载（若源环境含持久化卷且既未传 `volumeOverrides` 也未设 `omitProjectVolumes=true`，按 `R11` 返回待映射卷清单拒绝盲目克隆）
- `cluster`: `string`，可选，集群 ID 或名称（**省略时对齐 `InstallAppTemplate` 解析并持久化项目默认运行集群的权威 `clusterId`**，避免落库空串导致后续持久化卷绑定校验 `volume.incompatible_cluster` 失败）
- `workloadType`: `"Deployment" | "StatefulSet"`，可选，默认 `"Deployment"`
- `sourceType`: `"image" | "repository"`，可选，`cloneFromStage` 未传时必填
- `imageRef`: `string`，可选，`sourceType="image"` 时必填
- `repositoryBinding`: `string`，可选，`sourceType="repository"` 时必填（仓库绑定 ID 或 `owner/repo`）
- `targetRegistry`: `string`，可选，`sourceType="repository"` 时必填（制品库 ID 或名称）
- `dockerfilePath`: `string`，可选，仓库 Dockerfile 模式默认 `"Dockerfile"`
- `buildContext`: `string`，可选，默认 `"."`
- `buildTemplate`: `string`，可选，构建模板 ID（传入时自动设 `buildDefinitionMode="template"`）
- `buildTemplateValues`: `record<string, string>`，可选，配合 `buildTemplate` 传入必填构建模板参数
- `replicas`: `integer`，可选，默认 `1`（`>= 0`）
- `cpuRequest`: `string`，可选，默认 `"1"`（如 `"250m"`；注：容器 CPU/内存 Limit 由所属集群超卖策略自动派生）
- `memoryRequest`: `string`，可选，默认 `"1Gi"`（如 `"512Mi"`）
- `servicePorts`: `array<{name?: string, port: integer, appProtocol?: string}>`，可选，默认 `[{name: "http", port: 8080}]`（K8s `targetPort` 固定等于 `port`）
- `internalOnly`: `boolean`，可选，默认 `false`
- 实现契约说明（克隆模式防抢跑、防越权与防串环境）：
  1) **防缺密钥自动抢跑上线**：`cloneFromStage` 新建出的目标环境强制默认设置 `autoDeploy: false`（若 `secretRequirements` 非空同时在 `warnings` 中提示先补齐密钥再按需开启 `autoDeploy` 或执行首次发布）；
  2) **部署级 `BuildEnvironmentConfig` 权限门控**：当源环境存在非空部署级 `BuildEnvironmentConfig`（构建变量或构建密钥）时，严格对齐 `deployment_bundle_handlers.go:165` 校验调用者具备 `ActionSecretViewValue`（项目 `Owner / Admin`）；`Developer` 无该权限时自动跳过部署级 `BuildEnvironmentConfig` 克隆且不枚举泄露源构建变量/密钥键名（在 `warnings` 标注已跳过需管理员配置的部署级构建环境）；
  3) **镜像与配置集快照隔离**：对齐 `shouldResetGeneratedTargetRepository` 按新 `stage` 重新派生默认 `targetRepository / targetTag`（防止覆盖源环境镜像 Tag）；对 `snapshot` 模式的 `RuntimeConfigRefs` 从现存配置集重新抓取最新快照而非直接浅拷贝源 `SecretRefs`；
  4) 返回结构化 `secretRequirements: {build: string[], runtimeEnv: string[], runtimeFile: string[]}` 指引后续补齐新环境密钥。

**18. `delivery_update_target_runtime`** — 增量修改部署目标基础属性、容器规格、镜像、命令、端口、探针、安全调度与治理策略（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- `name`: `string`，可选，修改环境显示名称
- `cluster`: `string`，可选，切换运行集群（ID 或名称；**仅允许在首次发布前 `COUNT(releases) == 0` 执行**——若已有任何 `Release` 记录立即按 `R11` 返回 `409 deployment_target.cluster_immutable_after_deploy` 指引使用 `delivery_create_target(cloneFromStage)` 在新集群克隆环境，彻底杜绝原集群 K8s 工作负载与路由沦为不可清理、漏计费的永久孤儿；且在 `LockDeploymentTarget` 事务与预检中同步拦截异集群 `DeploymentVolumeMount` `409 volume.incompatible_cluster`、活跃 `GatewayRoute` `409 deployment_target.gateway_route_in_use` 及入站/出站 `ServiceBinding` `409 service_binding_in_use`）
- `workloadType`: `"Deployment" | "StatefulSet"`，可选，切换工作负载控制器类型（发版时自动替换）
- `imageRef`: `string`，可选，更新期望基准镜像地址（显式传入时同步刷新 `deployment_targets.image_ref_updated_at = NOW()`；未传时绝不触碰该时间戳）
- `imagePullPolicy`: `"IfNotPresent" | "Always" | "Never"`，可选
- `resetImagePullPolicy`: `boolean`，可选，清空自定义拉取策略
- `cpuRequest`: `string`，可选，容器 CPU 请求规格（正数 Quantity；Limit 由所属集群超卖比自动派生）
- `memoryRequest`: `string`，可选，容器内存请求规格（正数 Quantity）
- `autoScaling`: `{enabled?: boolean, minReplicas?: integer, maxReplicas?: integer, cpuPercent?: integer, memoryPercent?: integer}`，可选
  - 备注：`cpuPercent / memoryPercent` 范围 `0..1000`（`0` 表示禁用该维度指标，`enabled=true` 时至少一项 `>= 1`）；服务端 `enabled=false` 时**必须原样保留已存阈值与自定义 `autoScalingBehavior`**，严禁重置清零。
- `command`: `string[]`，可选，容器 `ENTRYPOINT` 数组（严禁把空格分隔的多参数塞入单元素）
- `args`: `string[]`，可选，容器 `CMD` 参数数组
- `resetCommand`: `boolean`，可选，清空自定义 `command` 恢复镜像默认
- `resetArgs`: `boolean`，可选，清空自定义 `args` 恢复镜像默认
- `upsertPorts`: `array<{name: string, port: integer, appProtocol?: string}>`，可选，按 `name` 新增或更新服务端口（若修改端口号导致任何活跃入站 `ServiceBinding(TargetPortName, TargetPort)` 或活跃 `GatewayRoute(ServicePort)` 失效，在预检与保存事务中立即返回 `409` 列出受影响依赖项）
- `removePortNames`: `string[]`，可选，按 `name` 删除服务端口（禁止删空最后一个端口，且仍被活跃入站 `ServiceBinding` 或 `GatewayRoute` 引用时 `409` 拦截）
- `probes`: `{readiness?: ProbeSpec, liveness?: ProbeSpec, startup?: ProbeSpec}`，可选，新增或更新健康探针（禁止传 `null`）
- `clearProbes`: `array<"readiness" | "liveness" | "startup">`，可选，显式移除指定探针
- `securityAndScheduling`: `{runAsUser?: integer, runAsGroup?: integer, fsGroup?: integer, readOnlyRootFilesystem?: boolean, clearRunAsUser?: boolean, clearRunAsGroup?: boolean, clearFsGroup?: boolean, clearSecurityContext?: boolean, setNodeSelector?: record<string, string>, removeNodeSelectorKeys?: string[], clearNodeSelector?: boolean}`，可选（支持独立清除 `runAsUser / runAsGroup / fsGroup` 而不误伤数据卷依赖的其余安全字段；平台固定 `allowPrivilegeEscalation=false`）
- `internalOnly`: `boolean`，可选，切换仅集群内网可访问（立即触发 NetworkPolicy 对账；若仍绑定活跃 `GatewayRoute` 则 `409 deployment_target.internal_has_routes` 拦截）
- `governance`: `{enabled?: boolean, requireApproval?: boolean, webConsolePolicy?: "inherit" | "disabled"}`，可选
  - 备注：`webConsolePolicy` 仅支持 `"inherit"`（`nil` 继承项目安全天花板）或 `"disabled"`（`false`）；`requireApproval=true` 阻止自动构建完成后无人工确认自动发版；`enabled=false` 仅禁止新建发布与自动构建发版，**不会停止运行中的 Pod**（停机请用 `delivery_scale_target(replicas: 0)`）。
- 实现契约说明：
  1) **存量高级与构建字段 100% 保护**：Service 层必须先加载数据库 `existing` 完整记录，将本工具 Schema 未暴露的所有高级 K8s 列（`Lifecycle / InitContainers / SidecarContainers / FSGroupChangePolicy / CapabilityAdd / CapabilityDrop / Tolerations / Affinity / TopologySpreadConstraints / PriorityClassName / ServiceAccountName / AutomountServiceAccountToken / ServiceType / ServiceAnnotations / ServiceExternalTrafficPolicy / ServiceSessionAffinity / AutoScalingBehavior`）及全部构建流水线列原样回填保留，严禁重置为默认值；
  2) **动态计算 `requiresRelease`**：修改了镜像/算力/HPA/命令/端口/探针/安全调度/集群/工作负载类型时返回 `requiresRelease: true`；仅修改立即生效的 `name / internalOnly / governance` 时返回 `requiresRelease: false`。

**19. `delivery_configure_target_config`** — 纯增量修改部署目标环境变量、加密密钥、明文文件、加密文件与配置集绑定（绝不全量覆盖，需发版生效，`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- `setPublicEnv`: `array<{key: string, value: string}>`，可选，新增或更新普通环境变量（键名须匹配 `^[A-Za-z_][A-Za-z0-9_]*$`；与同名已有加密变量冲突返回 `409 deployment.runtime_environment_key_conflict` 并提示同时传 `secretActions: clear`；与本目标活跃出站 `ServiceBinding` 完整注入键集合——同时包含连接变量 `URLEnvVar / HostEnvVar / PortEnvVar` 与凭证映射源变量 `SecretMap[].SourceEnvVar`，严禁复用漏计 `SecretMap` 的旧 `bindingEnvironmentKeys`——冲突时前置返回 `409 service_binding.env_conflict`）
- `removePublicEnvKeys`: `string[]`，可选，删除指定普通环境变量
- `secretActions`: `array<{key: string, operation?: "set" | "generate" | "clear", value?: string, encoding?: "base64" | "hex" | "alphanumeric" | "numeric", length?: integer}>`，可选，单键写入/随机生成（`length: 8..256` 默认 `32`）/清除加密环境变量（`value` 为 `writeOnly + x-luna-sensitive`；`operation` 省略默认 `"set"` 以原生兼容前端 `KeyValueInput` 动态密钥绑定，且有效 `operation=="set"` 时强制校验 `value` 非空；与普通变量冲突返回 `409 deployment.runtime_environment_key_conflict`，与活跃出站 `ServiceBinding` 完整注入键集合 `URLEnvVar / HostEnvVar / PortEnvVar / SecretMap[].SourceEnvVar` 冲突返回 `409 service_binding.env_conflict`，`clear` 仍被活跃入站 `ServiceBinding.SecretMap` 引用的 `targetSecretKey` 时前置返回 `409 service_binding.secret_in_use` 列出依赖方）
- `upsertConfigFiles`: `array<{path: string, content: string}>`，可选，按非根绝对路径新增或更新明文挂载文件（使用支持解析 `SecretFiles` JSON 字典键名的统一路径校验器：允许与已绑生效 `ConfigSet` 的同类 `configFiles` 同路径分层覆盖；严格 `409` 拦截与本目标或已绑生效 `ConfigSet` 的异类 `secretFiles` 同路径冲突、与任意明文/加密文件的父子路径前缀冲突、以及与 `dataVolumes.mountPath` 的同路径或父子路径冲突）
- `removeConfigFilePaths`: `string[]`，可选，按绝对路径删除明文挂载文件
- `secretFileActions`: `array<{path: string, operation?: "set" | "clear", content?: string}>`，可选，按非根绝对路径写入或清除加密挂载文件（`content` 为 `writeOnly + x-luna-sensitive`；`operation` 省略默认 `"set"` 且有效 `operation=="set"` 时强制校验 `content` 非空；允许与已绑生效 `ConfigSet` 的同类 `secretFiles` 同路径分层覆盖，严格 `409` 拦截与本目标或已绑生效 `ConfigSet` 的异类 `configFiles` 同路径冲突、与任意明文/加密文件的父子路径前缀冲突、以及与 `dataVolumes.mountPath` 的同路径或父子路径冲突）
- `upsertConfigSets`: `array<{configSet: string, mode?: "live" | "snapshot", refreshSnapshot?: boolean}>`，可选，绑定或更新项目运行配置集（支持 ID 或名称，支持刷新 `snapshot` 快照；当生效配置集 `enabled == true` 时，同步前置校验其环境变量/密钥键不与本目标活跃出站 `ServiceBinding` 完整注入键集合 `URLEnvVar / HostEnvVar / PortEnvVar / SecretMap[].SourceEnvVar` 冲突 `409 service_binding.env_conflict`，且保留跨层同类文件 `configFiles <-> configFiles` / `secretFiles <-> secretFiles` 同路径覆盖语义的同时，严格 `409` 拦截跨层异类文件 `configFiles <-> secretFiles` 同路径冲突、任意挂载文件间父子路径前缀冲突及与 `dataVolumes.mountPath` 的同路径或父子路径冲突）
- `detachConfigSets`: `string[]`，可选，解绑指定运行配置集（支持 ID 或名称）

**20. `delivery_configure_target_volumes`** — 增量绑定或解绑持久化数据卷与临时目录（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- `upsertMounts`: `array<{logicalName?: string, sourceType: "projectVolume" | "emptyDir", projectVolume?: string, mountPath?: string, devicePath?: string, readOnly?: boolean, emptyDirMedium?: "" | "Memory", emptyDirSizeLimit?: string}>`，可选
  - 备注：`logicalName` 须符合小写 DNS label（`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`）；**若省略 `logicalName`，服务端优先按规范化 `mountPath / devicePath` 匹配存量挂载并继承其既有 `logicalName`，未命中时再按路径 `basename` 生成**；对历史存量 `target.ClusterID == ""` 的部署目标先按 `runtimecluster.ResolveForTarget` 解析回填权威集群 ID 再执行 `ReserveDeploymentVolumeMount`；`Filesystem` 卷或 `emptyDir` 必填 `mountPath`（**对称双向校验与已存 `configFiles` 数组、`secretFiles` JSON 字典路径键及已绑生效 `ConfigSet` 的明文/加密文件路径无同路径或父子路径冲突**），`Block` 卷必填 `devicePath`，`emptyDir` 禁止 `readOnly: true`；更换已 `active` 挂载的底层卷时按 `R11` 提示先解绑发版释放。
- `removeMounts`: `string[]`，可选，按 `logicalName`、`mountPath` 或 `devicePath` 解绑挂载项（**RWO 独占锁安全释放契约**：在 `LockDeploymentTarget` 事务内，仅当挂载处于 `reserved` **且该 `deploymentTarget` 尚无任何历史/活跃 `Release` 记录（`COUNT(releases) == 0`）** 时立即调用 `BeginDeploymentVolumeUnbind + CompleteDeploymentVolumeUnbind` 释放（若无其他挂载变更则返回 `requiresRelease: false`）；只要该目标存在任意 `Release` 记录或挂载处于 `active/error`（包括 `release_pending -> reserved` 重绑态或发版中途超时态），一律置为 `release_pending` 并返回 `requiresRelease: true`，待下次 `Release` 对账卸下线上 Pod 挂载后释放 PVC 独占锁！）

**21. `delivery_delete_target`** — 异步删除单个部署环境及级联清理其网关路由（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- 备注：`/internal/v1/ai/tools/preview` 预检阶段若有入站 `ServiceBinding` 直接 `409` 拦截并列出依赖方，否则在审批卡片自动列出将级联删除的 `GatewayRoute` 域名与将解绑的数据卷；执行后返回 `{ id: "dplt_xxx", deploymentTargetId: "dplt_xxx", applicationId: "app_xxx", projectId: "prj_xxx", deleteStatus: "deleting" }`。

---

### 1.5 集群运行态变更与发布执行层（5 个，全部固定 `L2`，严格隔离同步立即生效 vs 异步 `Release`）

**22. `delivery_scale_target`** — 扩缩部署目标副本数，`0` 表示停机缩容至零释放算力（固定 `L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- `replicas`: `integer`，必填，`>= 0`
- 实现契约说明：
  1) **数据库 + K8s 线上残留 HPA 双重冲突强防呆（遵守 `AGENTS.md §3`）**：若数据库 `autoScalingEnabled == true`，**或者目标已在集群部署且命名空间中仍存在同名活跃 K8s `HorizontalPodAutoscaler` 对象**（如刚在数据库关闭 HPA 但尚未执行 `Release` 删除线上 HPA），统一按 `R11` 返回 `409 deployment_target.autoscaling_enabled` 拦截，指引先关闭 HPA 并调用 `delivery_deploy_target` 移除线上 HPA，彻底杜绝缩容后被残留 HPA 控制器在 15 秒内拉起；
  2) **真实生效状态与控制器切换回退识别**：按数据库期望 `workloadType` 查找 K8s 工作负载时若返回 `NotFound`，必须回退检查同命名空间归属同一 `deploymentTargetId` 标签的另一控制器类型（`Deployment <-> StatefulSet`，覆盖刚调 Tool 18 切换 `workloadType` 尚未发版的过渡窗口）——若线上存在旧类型控制器则同步对其执行 Scale 并返回 `appliedLive: true, requiresRelease: true`（正常未切换控制器返回 `appliedLive: true, requiresRelease: false`）；仅当两种控制器均不存在（尚未首次发布）时更新期望 `replicas` 并返回 `appliedLive: false, requiresRelease: true`。

**23. `delivery_deploy_target`** — 异步创建发布任务（`Release`）将期望配置与镜像下发至 K8s（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- `imageRef`: `string`，可选，指定本次单次发布覆盖镜像（跨环境晋升镜像或覆盖默认镜像时使用；对 `sourceType=="image"` 仅作为本次 `Release` 覆盖而不改写 `target.ImageRef` 基准列，永久修改基准镜像请用 Tool 18）
- `buildRunId`: `string`，可选，指定**同一部署目标**下 `succeeded` 状态的构建记录 ID
- `forceImagePull`: `boolean`，可选，默认 `false`，同 Tag 镜像重推后强制 K8s 拉取新镜像
- `message`: `string`，可选，发布备注说明
- 实现契约说明：
  1) **服务端默认镜像权威解析与审批锁定（修复 `validateReleaseForCreate` 空镜像报错与晋升/回滚后改配置发版静默回退隐患）**：通过 `migrations/` 为 `deployment_targets` 新增专用时间戳列 `image_ref_updated_at`（迁移回填 `created_at`；**仅在新建目标设置 `image_ref`、局部修改工具 Tool 18 / `build_configure_target_pipeline` 显式传入 `imageRef`、或传统整表单 `UpdateDeploymentTarget` 实际修改了 `image_ref` 值时刷新为 `NOW()`**，修改环境变量/算力/探针/副本数等任何非镜像字段绝不触碰该列！）。当 Tool 23 省略 `imageRef` 且省略 `buildRunId` 时——① 对 `sourceType == "repository"` 目标，服务端自动继承该 `deploymentTarget` 最近一条 `succeeded` `Release` 的 `(imageRef, buildRunId)`，尚无成功发布时回退选取该目标最近一条 `succeeded` `BuildRun`（均无则按 `R11` 返回 `409 release.image_required` 指引先触发构建）；② 对 `sourceType == "image"` 目标，若存在最近一条 `succeeded` `Release` 且 `target.ImageRefUpdatedAt` 未晚于该次 `Release.CreatedAt`（即自该次发布/晋升/回滚以来用户未更新过基准 `target.ImageRef`），则继承当前线上成功 `Release.ImageRef`（否则使用最新 `target.ImageRef`）；`/internal/v1/ai/tools/preview` 在 `approvalPreview` 显式展示 `currentImage -> resolvedImageRef` 并通过 `X-Luna-AI-Preview-Lock` 锁定 `(resolvedImageRef, resolvedBuildRunId)` 防 TOCTOU 漂移；
  2) **`LockDeploymentTarget` 行锁串行化与服务端 `revision` 自增**：与 Tool 24 及自动发版统一在 `LockDeploymentTarget(tx, projectId, targetId)` 行锁事务内原子校验 `enabled == true`（否则 `409 deployment_target.disabled`）、无 `pending / running` 活跃发布（否则 `409 release.in_progress` 附 `activeReleaseId` 指引先调 `delivery_cancel_release`），调用 `nextReleaseRevisionFor(tx, projectId, appId, targetId)` 服务端严格自增分配 `revision`（修复原接口未传时恒为 `1` 缺陷），返回权威 `projectId` 与 `releaseId`（`status: "pending"`）。

**24. `delivery_rollback_target`** — 异步创建镜像回滚发布任务（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- `toReleaseId`: `string`，可选（推荐显式传入），必须归属**同一 `deploymentTarget`** 且状态为 `succeeded` 的历史 `releaseId`（跨目标传入返回 `400 argument_invalid`）；省略时的精确默认规则（在已排除 `pending / running` 活跃发布后）：**若该环境最新一条 Release 为 `failed / canceled`，默认恢复到最近一次 `succeeded` 版本；仅当最新一条 Release 本身已是 `succeeded` 时，才恢复到其前一条 `succeeded` 版本**
- `message`: `string`，可选，回滚原因备注
- 实现契约说明：
  1) **`LockDeploymentTarget` 行锁并发门禁与 `revision` 自增**：同 Tool 23 在 `LockDeploymentTarget` 事务内原子校验 `enabled == true`（`409 deployment_target.disabled`）、无 `pending / running` 活跃发布（`409 release.in_progress` 指引先调 `delivery_cancel_release`），并调用 `nextReleaseRevisionFor` 自增分配 `revision`；
  2) **审批锁定防 TOCTOU 漂移（`R12`）**：`/internal/v1/ai/tools/preview` 在 `L2` 审批卡片明确展示 `currentImage -> targetImage (from resolvedToReleaseId, revision)`，并通过 `X-Luna-AI-Preview-Lock` 签名锁定 `resolvedToReleaseId`——批准执行时强制按锁定的 `resolvedToReleaseId` 执行；
  3) 返回权威 `projectId` 与新建回滚 `releaseId`（`type: "rollback"`）。**说明书强约束**：回滚仅恢复历史版本的容器镜像与构建引用，**不会回退环境变量、密钥、挂载文件或资源规格**。

**25. `delivery_restart_target`** — 同步原地滚动重启现有 K8s Pod（固定 `L2`，防止 `L1` 会话记忆穿透误重启生产 `dplt_xxx`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- 备注：仅向现有 K8s Deployment/StatefulSet 写入 `restartedAt` 注解触发滚动重建（若尚未部署则统一返回 `409 deployment_target.not_deployed` 提示先调 `delivery_deploy_target`），**不创建 `Release`、不返回 `releaseId`、也不会下发任何尚未发布的环境变量/密钥/探针/规格改动**。

**26. `delivery_cancel_release`** — 取消排队中或运行中的发布任务（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `releaseId`: `string`，必填，发布记录 ID
- 备注：仅 `pending / running` 可取消（已 `canceled` 幂等返回）；说明书与响应明确提示：**若 Worker 已将资源下发至集群并处于等待 Rollout 阶段，取消仅终止任务等待，不会自动回退线上 Pod**，如需恢复旧镜像请继续调用 `delivery_rollback_target`。

---

## 第 2 批：`build`（Git 账号与仓库绑定、制品库与凭据、构建变量/模板/流水线与构建执行域，共 29 个工具）

> 遵守 `notes/2026-11-19-ai-domain-tools-catalog.md` 第 0 节全局约定（双模定位与项目级响应统一回显权威 `projectId: "prj_xxx"` `R8`、零全量覆盖与三态防呆 `R5/R6/R7`、删除物理独立与防会话记忆穿透 `R3`、Secret `writeOnly + x-luna-sensitive`、实时状态 `no-store` 与异步任务契约对齐）。
> 跨批次权限、项目会话沙箱与边界自洽说明：
> 1. **平台管理员专属资源已物理迁入第 5 批 `admin` 域（遵守 `R4` 零 `403` 暴露）**：`GitProvider` 的创建/修改/删除及全量盘点（`provider_handlers.go:278,344,410` 全部强制 `requirePlatformAdmin`）以及 `scope="global"` 的全局 `BuildEnvironmentConfig`（读写均强制 `IsPlatformAdmin`）均已归入第 5 批 `admin` 域（`admin_list_git_providers`、`admin_save_git_provider`、`admin_delete_git_provider`、`admin_get_global_build_env`、`admin_configure_global_build_env`）。
> 2. **三层 Scoped 资源防静默提权与项目会话沙箱（`aiToolExecutionIdentityMiddleware` + Service 层）全闭环**：
>    - **新建与增量更新作用域保持**：`GitAccount`、`ArtifactRegistry`、`RegistryCredential`、`BuildVariableSet` 新建时若传入非空 `addProjects` 则默认 `scope="project"`，否则默认 `scope="user"`（仅 `platform_admin` 在全局会话显式传 `scope="global"` 时创建全局资源）；**更新模式下省略 `scope / addProjects / removeProjects` 时必须 100% 继承数据库现存 `existing.Scope / existing.OwnerRef / existing.ProjectIDs`**，严禁将空串传入底层 `normalizeOwnerScope("")` 造成误跳 `"global"`；
>    - **项目绑定会话（`conversationProjectID != ""`）与多项目成员凭据隔离全闭环（零 `403` 死锁、零跨项目凭据借用）**：
>      1) **中间件层（`internal/api/aiapi/ai_tool_identity.go`）**：对可选 `project` 的 `GET` 只读工具（`#1 / #3 / #5 / #6 / #8` 全模式统一）在 `model / direct / verification` 全模式下允许省略 `project` 并向请求自动注入 `conversationProjectID`；对无 `project` 参数的 `#2 build_search_git_repositories` 中间件放行交 Service 校验；对含 `scope + addProjects / removeProjects` 的三层 Scoped 写工具，中间件拒绝显式 `scope == "global"` 并校验非空 `addProjects / removeProjects` 匹配 `conversationProjectID`，对 `scope == "user"` 或省略 `scope / addProjects` 放行至 Service 层；
>      2) **Service 层存量态/目标态独立不变量与外部凭据按项目严格过滤**：① 在 `conversationProjectID != ""` 时，`#12/#13/#17/#18/#19/#20/#22/#23` 对存量态（更新/删除）与目标态（新建/更新）各自独立强制满足 `(scope == "user" && ownerRef == currentUser.ID) || (scope == "project" && equalSingle(projectIDs, conversationProjectID))`（100% 阻断操作 `global` 资源、他项专属资源或多项目共享资源）；② **修复全路径跨项目凭据借用漏洞**：当存在 `effectiveProjectID`（显式 `project` 或注入的 `conversationProjectID`）时，`#5`（**常规列表健康观测 `observeArtifactRegistries -> pingRegistry`、`items[].credentials` 及 `testRegistry`**）与 `#6`（`remote_repositories / remote_tags` 同时校验 `ArtifactRegistry` 可见性与 `RegistryCredential` 可见性）统一透传 `effectiveProjectID` 并使用 `applyScopedResourceVisibilityForProject(effectiveProjectID)`；同理 `#14 / #15 / #21 / #25 / #26 / #27`（**含 `gitAccount / provider / registry / attachVariableSets`，以及 `prepareBuildRunRequest` `build_validation.go:170` 与 `buildruntime.ResolveBuildTask` `runtime.go:57` 对父表 `ArtifactRegistry` `run.TargetRegistryID` 的作用域可见性复核**，以及第 1 批 `delivery_create_target`）强制校验所引用的父子资源对 `(actor, targetProjectID)` 可用（`scope == "global" || (scope == "user" && ownerRef == actorID) || (scope == "project" && contains(projectIDs, targetProjectID))`），**并同步删除 `buildapi/build_variables.go:270` 与 `buildruntime/runtime.go:354` 在 `set.Scope == "project"` 分支的 `PlatformRoleAdmin` 跨项目免检漏洞**（严防管理员绑定账号触发 Webhook 时将他项变量集密钥跨项目注入构建容器！）；
>      3) **平台管理员全局盘点与多项目资源写后回读闭环**：`#1 / #5 / #6 / #8` 在全局会话未传 `project`（`effectiveProjectID == ""`）且当前用户为 `platform_admin` 时，默认对齐 `includeAllProjects = true`（`ListVisibilityAll`）返回全平台租户项目关联资源（普通用户仍严格限定本人 `user` + 所属成员项目 `project` + `global`）；同时 `outcome.ts` 填充可选 `project` 时按 `result.projectId ?? call.arguments.project ?? firstElement(result.projectIds)` 提取，彻底消除管理员操作未入项或跨多项目 `scope="project"` 资源时的写后验证盲区。
> 3. **部署目标构建流水线只读、写后验证（`catalog.ts` + `outcome.ts`）与审计闭环**：
>    - 部署目标上全部构建流水线配置字段（含 `buildTemplateVersion` 与 `buildEnvironmentId`）统一由第 1 批 `delivery_get_target` 的 `buildPipeline` 子结构完整回读；
>    - 同步升级 `internal/aitool/openapi_catalog.go`、`luna-agent/src/tools/catalog.ts`（扩展 `.strict()` 的 `toolVerificationRuleSchema`）、`domain.ts` 与 `outcome.ts`：按目标只读工具 `inputSchema.maximum` 封顶注入 `pageSize`（`<= 50`），`collectionPath` 支持点路径与 `[]` 数组穿透展平（如 `"accounts.items"` 与 `"items[].credentials"`），支持 `staticArguments`，可选 `project` 支持从 `result.projectIds[0]` 回退提取，并将 `family: "build_run"` 统一映射 `{ project: result.projectId ?? call.arguments.project, buildRunId: result.buildRunId ?? result.id ?? call.arguments.buildRunId } -> build_get_run`；
>    - 本批所有同步删除工具（`#13/#16/#18/#20/#23/#29`）废除空 `204`，统一返回 JSON `{ id: "<resolved_id>", deleted: true, ...(projectId ? { projectId } : {}) }`，所有写工具在 Go Service 层统一补齐业务 `AuditLog`。

---

### 2.1 只读发现与构建回读层（11 个，固定 `L0`）

**1. `build_list_git_sources`** — 分页查看当前可见的 Git 平台提供商（`GitProvider`）与已连接 Git 账号/凭据（`GitAccount`）
- `project`: `string`，可选，按项目可见范围筛选（传入或处于项目绑定会话时回显权威 `projectId`）
- `gitAccount`: `string`，可选，按单个 Git 账号 ID（`gita_xxx`）或当前可见范围内唯一用户名精确过滤（闭环 `#12 / #13` 写后校验）
- `search`: `string`，可选，按名称、地址或用户名搜索
- `includeProviders`: `boolean`，可选，默认 `true`
- `includeAccounts`: `boolean`，可选，默认 `true`（返回账号实时连通状态 `status / observationCode`，绝不回显访问令牌）
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）
- 返回契约：固定返回 `{ providers?: Paginated<GitProvider>, accounts?: Paginated<GitAccount> }` 双独立分页结构，避免合并 `total` 歧义。

**2. `build_search_git_repositories`** — 通过可见 Git 账号或匿名搜索远端 GitHub / Gitea 仓库列表
- `gitAccount`: `string`，必填，Git 账号 ID（`gita_xxx`）、当前可见范围内唯一用户名（含项目共享/全局账号，重名可通过 `provider` 消歧），或传 `"anonymous"` 搜索公开仓库（注：`gitlab` 类型 Provider 当前不支持远端仓库搜索 API，调用时按 `R11` 返回明确说明；项目绑定会话下强制校验所用 `gitAccount / provider` 对 `conversationProjectID` 可用）
- `provider`: `string`，可选，GitProvider ID 或名称（**当 `gitAccount="anonymous"` 时必填**，对齐 `repository_handlers.go:647`；亦可消除跨平台同名账号歧义）
- `search`: `string`，可选（当 `gitAccount="anonymous"` 时必填），仓库关键词
- `includePublic`: `boolean`，可选，默认 `false`，账号仓库无匹配时回退搜索公开仓库
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**3. `build_inspect_repository_code`** — 探查远端 GitHub / Gitea 仓库的分支、构建探测建议（Dockerfile/暴露端口/推荐模板）、目录结构或单个文件内容
- `project`: `string`，可选，使用 `repositoryBinding` 定位时必填（项目绑定会话省略时自动注入 `conversationProjectID`；Service 层同时强制校验 `repository_bindings.project_id == effectiveProjectID` **以及底层 `GitAccount / GitProvider` 对 `currentUser` 与 `effectiveProjectID` 可用**——`account.Scope == "user"` 仅允许 `ownerRef == currentUser.ID`，严防项目内只读 `viewer` 借用项目 Owner 个人 PAT/OAuth 令牌读取私有仓库代码！）
- `application`: `string`，可选，同项目 Monorepo 多应用绑定同一 `owner/repo` 时辅助消歧
- `repositoryBinding`: `string`，可选，已绑定仓库 ID（`rpb_xxx`）或 `owner/repo`（与 `gitAccount + owner + repo` 二选一）
- `gitAccount`: `string`，可选，未绑定时传可见 Git 账号 ID/用户名或 `"anonymous"`（项目绑定会话下校验对 `conversationProjectID` 可用）
- `provider`: `string`，可选，当 `gitAccount="anonymous"` 或同名账号消歧时传入 GitProvider ID 或名称
- `owner`: `string`，可选，传 `gitAccount` 时必填
- `repo`: `string`，可选，传 `gitAccount` 时必填
- `ref`: `string`，可选，分支、Tag 或 Commit SHA（省略取仓库默认分支）
- `includeBranches`: `boolean`，可选，默认 `true`，返回匹配分支列表（上限 `50` 条）
- `branchSearch`: `string`，可选，分支名过滤
- `includeBuildOptions`: `boolean`，可选，默认 `false`，扫描仓库目录返回 `dockerfiles`、`directories`、`exposedPorts` 与 `recommendedTemplateIds`
- `path`: `string`，可选，**传 `"/"` 或 `"."` 列出仓库根目录条目**；传子目录路径返回目录条目 `entries`；传文件路径返回文本文件正文（省略 `path` 则不拉取目录或文件）
- `maxFileBytes`: `integer`，可选，默认 `16384`（上限 `65536`）
- 实现契约说明：Service 层自适应区分上游 Contents API 返回的目录数组与单文件对象，检测拦截二进制文件，并对超长文本按 UTF-8 字符边界截断（返回 `truncated: boolean`）。

**4. `build_list_repository_bindings`** — 分页列出项目下应用绑定的代码仓库及实时 Webhook 状态
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，可选，应用 ID 或 `identifier`
- `binding`: `string`，可选，按仓库绑定 ID（`rpb_xxx`）精确过滤单条记录（闭环 `#14/#15/#16` 写后校验）
- `search`: `string`，可选，按 `owner/repo`、应用名或账号搜索
- `sortBy`: `"createdAt" | "repo" | "owner"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）
- 实现契约说明：Service 层使用 `LEFT JOIN git_providers / git_accounts`，确保关联账号或 Provider 被删除后的孤儿绑定仍可见并可解绑。

**5. `build_list_registries`** — 分页列出可见容器制品库、连通状态、项目默认制品库及嵌套推拉凭据摘要（支持按需主动探测单个制品库）
- `project`: `string`，可选，项目 ID 或 `identifier`（传入时返回该项目解析出的 `defaultRegistryId`；配合 `application` 返回渲染后的默认 `targetImageRef / targetRepository / targetTag`）
- `registry`: `string`，可选，按制品库 ID（`reg_xxx`）或精确名称过滤（纯 DB 过滤不触发网络 Ping，闭环 `#17/#18/#19/#20` 写后校验）
- `credential`: `string`，可选，按凭据 ID（`regc_xxx`）或名称精确过滤 `items[].credentials`（配合 `collectionPath: "items[].credentials"` 闭环 `#19 / #20` 凭据写后校验）
- `application`: `string`，可选，配合 `project` 预览默认目标镜像命名模板（`registry_image_template_handlers.go:21`）
- `stage`: `"dev" | "test" | "staging" | "prod"`，可选，配合 `project + application` 渲染镜像模板
- `testRegistry`: `string`，可选，传入可见制品库 ID 或名称时执行只读 `pingRegistry` 并返回 `{success, statusCode, message, endpoint?}`（实现契约：项目绑定会话下须对该项目可见；若当前用户对该制品库不具备 `canInspectScopedResourceConfigByID` 配置查看权限，必须将探测结果中的 `endpoint` 置空脱敏，对齐 `registry_types.go:30-34` 防止泄露共享内网制品库地址）
- `search`: `string`，可选，按名称或 Endpoint 搜索
- `includeCredentials`: `boolean`，可选，默认 `true`，在每个制品库元素 `items[].credentials` 下附带可见凭据摘要（回显 `id`、`name`、`registryId`、`scope`、`ownerRef`、`projectIds`、`username`、`usage`、`repositoryTemplate`、`tagTemplate` 与 `passwordSet / tokenSet` 布尔标记，有 `effectiveProjectID` 时严格按 `applyScopedResourceVisibilityForProject` 过滤凭据，绝不回显密文）
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**6. `build_search_registry_images`** — 搜索远端制品库中的仓库/Tag 或分页查询平台已登记的容器镜像台账
- `source`: `"recorded_images" | "remote_tags" | "remote_repositories"`，可选，默认 `"recorded_images"`（明确单一查询源，默认 `"recorded_images"` 原生闭环 `#21 build_register_container_image` 写后校验）
- `registry`: `string`，可选，`remote_tags / remote_repositories` 时必填；`recorded_images` 时可选按制品库筛选（Service 层在 `container_image_handlers.go` 补齐 `registry_id`、`application_id` 与规范化 `repository` SQL 过滤谓词）
- `repository`: `string`，可选，`remote_tags` 时必填（如 `"library/nginx"`）；`recorded_images` 时亦支持传入并按 `strings.Trim(repository, "/")` 精确过滤（无缝兼容 `outcome.ts` 对 `#21` 同名参数自动透传）
- `project`: `string`，可选，全模式通用的项目可见性过滤（项目绑定会话省略时统一自动注入 `conversationProjectID`；`recorded_images` 按项目过滤台账，`remote_tags / remote_repositories` 同时限定 `registry` 与 `RegistryCredential` 对 `effectiveProjectID` 可见）
- `application`: `string`，可选，`recorded_images` 按应用筛选
- `imageId`: `string`，可选，`recorded_images` 按镜像台账 ID（`img_xxx`）精确单查
- `search`: `string`，可选，关键词过滤（Service 层对 `remote_tags` 同步补齐内存子串过滤）
- `page`: `integer`，可选，默认 `1`（注：`remote_tags` 为单页返回，不支持翻页）
- `pageSize`: `integer`，可选，默认 `20`（`remote_repositories` 硬上限 `20`，其余上限 `50`）

**7. `build_list_templates`** — 列出内置构建模板（Buildpack/语言模板）并支持纯内存渲染预览 Dockerfile
- `template`: `string`，可选，传入模板 ID 时返回参数 Schema；配合 `previewValues` 返回渲染后的 Dockerfile 预览（纯内存计算，零副作用）
- `version`: `string`，可选，模板版本
- `previewValues`: `record<string, string>`，可选，预览渲染参数

**8. `build_list_variable_sets`** — 分页列出可见的构建变量与密钥集（普通变量键值按权限脱敏 + 已配置加密密钥键名数组 `secretKeys: string[]`）
- `project`: `string`，可选，按项目筛选
- `variableSet`: `string`，可选，按构建变量集 ID（`bvs_xxx`）或精确名称过滤（闭环 `#22 / #23` 写后校验）
- `search`: `string`，可选，按名称搜索
- `sortBy`: `"createdAt" | "name" | "scope"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）
- 实现契约说明：严格对齐 `build_variables.go:328-345` 权限边界——当调用者对某变量集不具备配置查看权限（如非 `platform_admin` 查看 `global` 变量集，或无 `ActionProjectManage` 的 `developer / viewer` 查看 `project` 变量集）时，必须将明文 `variables` 脱敏为 `{}` 并返回 `canInspectVariables: false` 与 `variableCount: integer`，既严防越权泄露，又避免模型误判变量集为空。

**9. `build_get_scoped_env`** — 读取应用级或部署环境级的专属构建期变量与已配置密钥键名数组 `secretKeys: string[]`（需项目 `owner / admin` 权限 `ActionSecretViewValue`）
- `scope`: `"application" | "deployment"`，必填（注：平台全局级 `"global"` 仅限管理员通过 `admin_get_global_build_env` 读取；**构建期变量仅在镜像构建阶段注入，容器运行期环境变量请用 `delivery_get_target`**）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，可选，`scope="deployment"` 时必填（支持 `targetId | stage | name`）

**10. `build_list_runs`** — 分页查询项目下的构建运行历史记录
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，可选，应用 ID 或 `identifier`
- `target`: `string`，可选，传入 `application` 时支持 `targetId | stage | name`；未传 `application` 时支持 `targetId` 或公开 `stage`（跨应用按阶段过滤）
- `status`: `"queued" | "running" | "succeeded" | "failed" | "canceled" | "timeout"`，可选
- `triggerType`: `"manual" | "webhook" | "push" | "tag" | "api" | "retry"`，可选
- `sourceBranch`: `string`，可选
- `search`: `string`，可选，按分支、Commit、镜像 Tag 搜索
- `sortBy`: `"createdAt" | "status" | "sourceCommit" | "sourceBranch" | "triggerType"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**11. `build_get_run`** — 查看单次构建详情、`BuildJob` 步骤进度、尾部截断的权威构建日志及关联 `HookRun` 执行摘要
- `project`: `string`，必填，项目 ID 或 `identifier`
- `buildRunId`: `string`，必填，构建记录 ID（`bldr_xxx`）
- `includeLogs`: `boolean`，可选，默认 `true`，自动聚合关联 `BuildJob` 的 `BuildLog` 尾部及 `HookRun` 状态摘要（**严禁在摘要中回传最高 64KB 的 `HookRun.scriptSnapshot` 全文**；如需查看单条 Hook 详细输出可调 `runtime_list_hook_runs(hookRunId)`）
- `logTailBytes`: `integer`，可选，默认 `8192`（上限 `32768`，UTF-8 字符边界截断）

---

### 2.2 Git 账号凭据与仓库绑定写操作层（5 个，固定 `L2`）

**12. `build_save_git_account`** — 创建或纯增量更新 Git 访问凭据/账号，或刷新 OAuth 访问令牌（`L2`）
- `gitAccount`: `string`，可选，省略为新建，传入账号 ID（`gita_xxx`）或当前可管理范围内唯一用户名为存量基底增量修改（跨 Provider 重名可通过 `provider` 消歧，否则返回 `409 ai.resource_ambiguous`）
- `provider`: `string`，可选，新建必填（GitProvider ID 或名称；**创建后不可修改所属 `provider`**，更新时传入仅用于跨平台同名账号定位消歧，与存量 `providerId` 不一致时返回 `400 argument_invalid`）
- `username`: `string`，可选，新建必填
- `accessToken`: `string`，可选，**新建时必填非空**（`writeOnly: true, x-luna-sensitive: true`；更新时省略保留原加密令牌，与 `refreshOAuth` 互斥）
- `refreshOAuth`: `boolean`，可选，仅更新已有 OAuth 账号时设为 `true` 主动使用已存 `RefreshToken` 刷新令牌（与 `accessToken` 互斥）
- `scope`: `"user" | "project" | "global"`，可选，新建传非空 `addProjects` 默认 `"project"` 否则默认 `"user"`（`"global"` 仅平台管理员在全局会话可用，须在父 `provider` 作用域内；**更新省略时严格保留原 `scope`**）
- `addProjects`: `string[]`，可选，`scope="project"` 时增量授权关联项目
- `removeProjects`: `string[]`，可选，`scope="project"` 时移除关联项目
- 实现契约说明：更新模式下必须保留账号原有的 `ExternalUserID / AvatarURL / Scopes`；**当省略 `accessToken` 且未设 `refreshOAuth: true` 时原样保留 `RefreshTokenRef / ExpiresAt`；当显式传入非空 `accessToken`（手动替换静态 PAT）时，在单数据库事务内清空 `ExpiresAt = nil` 并通过 `StoreContextWithDB` + `DeleteRefContextWithDB` 原子写入新令牌、物理删除旧 `RefreshTokenRef` 与旧 `AccessTokenRef`（防止过期 OAuth 账号替换 PAT 后仍触发 `gitAccountNeedsRefresh` 刷新报错或覆盖新令牌）；当 `refreshOAuth: true`（`refreshGitAccountForUser` `account_handlers.go:381-391` 从上游拿到新令牌后的持久化阶段）时，同样必须在单数据库事务内调用 `StoreContextWithDB` + `DeleteRefContextWithDB` 原子更新令牌引用并物理清理被覆盖的旧 `AccessTokenRef`（以及上游轮换返回新 `RefreshToken` 时的旧 `RefreshTokenRef`）**。

**13. `build_delete_git_account`** — 删除 Git 访问凭据/账号（`L2`）
- `gitAccount`: `string`，必填，账号 ID（`gita_xxx`）或当前可管理范围内唯一用户名
- `provider`: `string`，可选，跨平台同名账号删除消歧
- 返回与沙箱契约：仍被活跃 `RepositoryBinding` 引用时 `409` 拦截；项目绑定会话下严格校验存量作用域满足第 2 条不变量；删除成功返回 `{ id: "gita_xxx", deleted: true }`。

**14. `build_bind_repository`** — 为应用绑定代码仓库并按需注册远端 Webhook（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `gitAccount`: `string`，必填，可见 Git 账号 ID（`gita_xxx`）或唯一用户名（自动推导所属 `gitProviderId`）
- `owner`: `string`，必填，仓库组织或用户名（禁止 `/` 或 `-` 前缀）
- `repo`: `string`，必填，仓库名（禁止 `/` 或 `-` 前缀）
- `cloneUrl`: `string`，可选，省略时按 Provider 与 `owner/repo` 自动补全（`gitlab` Provider 必填 `cloneUrl`）
- `defaultBranch`: `string`，可选，默认 `"main"`（禁止以 `-` 开头防 Git 选项注入）
- `webhookEnabled`: `boolean`，可选，GitHub/Gitea 默认 `true`；**若所属 Provider 为 `"gitlab"`，当前仅支持 PAT 克隆构建，不支持自动注册 Webhook，必须显式传 `webhookEnabled: false`（否则按 `R11` 返回明确提示）**
- 实现安全契约（防 Builder 令牌外泄铁律，`AGENTS.md §2 / §8`）：Service 层强制校验 `gitAccount` 对当前目标 `project` 可用（禁止跨项目借用他项专属凭据），同应用重复绑定同一仓库统一返回稳定错误码 `409 git.repository_binding_exists`（`#15` 同此）；因 `run.sh:6-11` 会向 `CloneURL` 注入 `GIT_ACCESS_TOKEN`，对 `cloneUrl` 强制执行严格同源校验——必须为合法 `http(s)://` URL、严禁包含 `url.User` 明文凭据、严禁 `RawQuery (?...)` 与 `Fragment (#...)`、严禁 `-` 前缀，**且 `cloneUrl` 的 Origin（`scheme + host`，大小写不敏感）必须与所属 `GitProvider.BaseURL` 的 Origin 完全一致**（GitHub 固定对齐 `https://github.com`；若存量自建 Provider 未配置 `BaseURL` 则报错指引管理员先配置 `baseUrl`），并通过 `security.EgressPolicy` 校验，100% 阻断通过自定义 `cloneUrl` 将共享 Git 令牌外泄至第三方服务器！

**15. `build_update_repository_binding`** — 增量修改应用的代码仓库绑定或修复重建远端 Webhook（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，可选，同项目 Monorepo 多应用绑定同一 `owner/repo` 时传入应用消歧
- `binding`: `string`，必填，仓库绑定 ID（`rpb_xxx`）或唯一 `owner/repo`
- `gitAccount`: `string`，可选，切换关联可见 Git 账号
- `owner`: `string`，可选，仓库迁移改名时更新
- `repo`: `string`，可选，仓库迁移改名时更新
- `cloneUrl`: `string`，可选（同 `#14` 强制与所属 `GitProvider.BaseURL` 同源且禁内嵌凭据/Query/Fragment）
- `defaultBranch`: `string`，可选（禁止 `-` 前缀）
- `webhookEnabled`: `boolean`，可选（禁止同时传 `webhookEnabled: false` 与 `reconfigureWebhook: true`）
- `reconfigureWebhook`: `boolean`，可选，设为 `true` 时旋转密钥并重建远端 Webhook
- 实现契约说明：
  1) **仓库/账号变更无条件同步 `cloneUrl`**：当修改了 `owner`、`repo` 或切换了 `gitAccount`（Provider）且未显式传入新 `cloneUrl` 时，**无条件**按新 `provider + owner + repo` 重算 `defaultCloneURL`（若目标为 `gitlab` 且未传新 `cloneUrl` 则按 `R11` 报错提示必传），彻底消除任何存量自定义 `CloneURL` 残留指向旧仓的风险；
  2) **Webhook 关停与目标变更彻底闭环**：① 修复 `ReceiveGitWebhook` / `enqueueBuildRunsForWebhook`（`webhook_build.go:61`）在 `!binding.WebhookEnabled` 时立即忽略并返回 `ignoredReason: "webhook_disabled"`，杜绝关停后远端推送仍触发构建；② 当设为 `webhookEnabled: false`、变更 `owner / repo / gitAccount` 或 `reconfigureWebhook: true` 时，若有旧 `WebhookID / WebhookSecret`，先尽力调用 `client.DeleteWebhook` 清理旧远端 Hook 并在事务内调用 `DeleteRefContextWithDB` 删除旧 `WebhookSecret` 密文、清空旧 `WebhookID`。

**16. `build_unbind_repository`** — 解绑应用的代码仓库（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，可选，Monorepo 同仓消歧
- `binding`: `string`，必填，仓库绑定 ID（`rpb_xxx`）或 `owner/repo`
- 实现契约说明：若仍被活跃 `DeploymentTarget` 引用，对齐存量权威错误码返回 `409 git.repository_binding_in_use` 并列出目标环境；解绑时尽力清理远端 Webhook、删除 `WebhookSecret` 密文引用并写入业务 `AuditLog`，返回 `{ id: "rpb_xxx", projectId: "prj_xxx", deleted: true }`。

---

### 2.3 制品库与镜像凭据写操作层（5 个）

**17. `build_save_registry`** — 创建或纯增量修改容器制品库配置（`L2`）
- `registry`: `string`，可选，省略为新建，传入制品库 ID（`reg_xxx`）或唯一名称为存量基底增量修改
- `name`: `string`，可选，新建必填
- `provider`: `"harbor" | "dockerhub" | "gitea-registry" | "generic-oci"`，可选，新建默认 `"harbor"`
- `endpoint`: `string`，可选，新建必填（如 `"https://registry.example.com"`，严禁内嵌 `url.User` 明文凭据）
- `scope`: `"user" | "project" | "global"`，可选，新建传 `addProjects` 默认 `"project"` 否则默认 `"user"`（仅 `"global"` 要求 `platform_admin` 且在全局会话可用；**更新省略时严格保留原 `scope`**）
- `addProjects`: `string[]`，可选，`scope="project"` 时增量授权项目列表
- `removeProjects`: `string[]`，可选，`scope="project"` 时增量移除项目列表
- `isDefault`: `boolean`，可选，在对应 `user / project / global` 作用域内设为默认制品库（**实现契约**：更新 `scope="project"` 制品库且省略 `isDefault` 时，必须从 `scoped_resource_project_bindings` 继承存量 `DefaultProjectIDs` 并仅剔除 `removeProjects`，严禁因主表 `existing.IsDefault==false` 清空或串改项目级默认标记！）

**18. `build_delete_registry`** — 删除容器制品库及其下属凭据（`L2`）
- `registry`: `string`，必填，制品库 ID（`reg_xxx`）或名称
- 实现契约说明：Service 层新增活跃 `DeploymentTarget.TargetRegistryID` 引用检查，仍被引用时返回 `409 registry.in_use` 拒绝直删；项目绑定会话下严格执行第 2 条存量态不变量校验；成功返回 `{ id: "reg_xxx", deleted: true }`。

**19. `build_save_registry_credential`** — 创建或纯增量修改制品库推拉凭据（`L2`，绝不静默清空密码/Token 或篡改作用域）
- `registry`: `string`，必填，制品库 ID（`reg_xxx`）或名称
- `credential`: `string`，可选，省略为新建，传入凭据 ID（`regc_xxx`）或唯一名称为增量更新
- `name`: `string`，可选，新建必填
- `username`: `string`，可选（清空传 `clearUsername: true`）
- `clearUsername`: `boolean`，可选
- `password`: `string`，可选，密码（`writeOnly: true, x-luna-sensitive: true`；更新时省略保留原密码）
- `token`: `string`，可选，访问令牌（`writeOnly: true, x-luna-sensitive: true`；更新时省略保留原令牌）
- `clearPassword`: `boolean`，可选，显式清除密码（注：清除后 `password` 与 `token` 至少须保留一项非空，否则报 `400 argument_invalid`）
- `clearToken`: `boolean`，可选，显式清除令牌
- `usage`: `"push-pull" | "push" | "pull"`，可选，新建默认 `"push-pull"`
- `scope`: `"user" | "project" | "global"`，可选，新建传非空 `addProjects` 默认 `"project"` 否则默认 `"user"`，且须在父制品库可见范围内（`"global"` 仅管理员在全局会话可用；**更新省略时严格保留原 `scope / projectIds`**）
- `addProjects`: `string[]`，可选
- `removeProjects`: `string[]`，可选
- `repositoryTemplate`: `string`，可选，自定义镜像仓库路径模板（重置默认传 `resetRepositoryTemplate: true`）
- `resetRepositoryTemplate`: `boolean`，可选
- `tagTemplate`: `string`，可选，自定义镜像 Tag 模板（重置默认传 `resetTagTemplate: true`）
- `resetTagTemplate`: `boolean`，可选
- 返回与密文清理契约：在单数据库事务内使用 `StoreContextWithDB` + `DeleteRefContextWithDB` 物理清理 `clearPassword / clearToken` 或轮换覆盖掉的旧 `secret_values` 记录；统一返回 `{ id: "regc_xxx", registryId: "reg_xxx", ... }`，通过 `#5 build_list_registries(registry, credential)` 的 `items[].credentials` 权威回读校验。

**20. `build_delete_registry_credential`** — 删除制品库凭据（`L2`）
- `registry`: `string`，必填，制品库 ID（`reg_xxx`）或名称
- `credential`: `string`，必填，凭据 ID（`regc_xxx`）或名称（项目绑定会话下严格执行第 2 条存量态不变量校验；成功返回 `{ id: "regc_xxx", registryId: "reg_xxx", deleted: true }`）

**21. `build_register_container_image`** — 手动登记外部已构建镜像到项目镜像台账（`L1`，仅写静态台账，不影响线上工作负载）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，可选，关联应用 ID 或 `identifier`（Service 层强制校验归属当前 `project`）
- `registry`: `string`，必填，制品库 ID 或名称
- `repository`: `string`，必填，镜像路径（如 `"team/api"`）
- `tag`: `string`，可选，默认 `"latest"`
- `digest`: `string`，可选，如 `"sha256:..."`
- `sourceCommit`: `string`，可选

---

### 2.4 构建变量、部署目标构建流水线与构建执行层（8 个，全部固定 `L2`）

**22. `build_save_variable_set`** — 创建或纯增量修改构建变量与密钥集（`L2`，彻底消除原 REST 全量覆盖删密钥与静默提权隐患）
- `variableSet`: `string`，可选，省略为新建，传入 ID（`bvs_xxx`）或唯一名称为存量基底增量修改
- `name`: `string`，可选，新建必填
- `scope`: `"user" | "project" | "global"`，可选，新建传 `addProjects` 默认 `"project"` 否则默认 `"user"`（`"global"` 仅平台管理员在全局会话可用；**更新省略时严格保留原 `scope / ownerRef / projectIds`**）
- `addProjects`: `string[]`，可选，`scope="project"` 时增量授权项目
- `removeProjects`: `string[]`，可选，`scope="project"` 时移除授权项目
- `enabled`: `boolean`，可选，新建默认 `true`
- `setVariables`: `array<{key: string, value: string}>`，可选，新增或更新普通构建变量（键名匹配 `^[A-Za-z_][A-Za-z0-9_]*$`，禁止 `LITEYUKI_*` 平台保留前缀防止构建期被静默丢弃；与同名加密密钥双向冲突检查统一返回 `409 build.variable_key_conflict`）
- `removeVariableKeys`: `string[]`，可选，删除指定普通构建变量
- `secretActions`: `array<{key: string, operation?: "set" | "clear", value?: string}>`，可选，单键写入或清除构建加密密钥（`value` 声明 `writeOnly: true, x-luna-sensitive: true`，`operation` 省略默认 `"set"` 且有效 `operation=="set"` 时强制校验 `value` 非空，禁止 `LITEYUKI_*` 保留前缀，双向校验不与未移除普通变量同键冲突；保存后普通变量与加密密钥不可同时为空集）
- 实现契约说明：Service 层补齐 `build_variable_set.create / update` 业务 `AuditLog`，响应按权限回显 `variables` 与 `secretKeys: string[]`。

**23. `build_delete_variable_set`** — 删除构建变量与密钥集并自动从关联部署目标解绑（`L2`）
- `variableSet`: `string`，必填，变量集 ID（`bvs_xxx`）或唯一名称（项目绑定会话下严格执行第 2 条存量态不变量校验，禁止跨项/全局删除；补齐 `build_variable_set.delete` 业务 `AuditLog` 并返回 `{ id: "bvs_xxx", deleted: true }`）

**24. `build_configure_scoped_env`** — 纯增量配置应用级或部署环境级专属构建期变量与密钥（`L2`，需项目 `owner / admin` 权限 `ActionSecretViewValue`）
- `scope`: `"application" | "deployment"`，必填（注：全局级请用管理员 `admin_configure_global_build_env`；**本工具仅影响镜像构建阶段，修改容器运行期环境变量请调 `delivery_configure_target_config`**）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，可选，`scope="deployment"` 时必填（支持 `targetId | stage | name`）
- `setVariables`: `array<{key: string, value: string}>`，可选，新增或更新普通构建变量（禁 `LITEYUKI_*` 前缀，与已有加密密钥双向同键冲突返回 `409 build.variable_key_conflict`）
- `removeVariableKeys`: `string[]`，可选，删除指定普通构建变量
- `secretActions`: `array<{key: string, operation?: "set" | "clear", value?: string}>`，可选，单键写入或清除构建加密密钥（`value` 为 `writeOnly + x-luna-sensitive`，`operation` 省略默认 `"set"` 且有效 `"set"` 强制非空，禁 `LITEYUKI_*` 前缀，双向同键冲突返回 `409 build.variable_key_conflict`；**必须在单事务内调用 `StoreContextWithDB` + `DeleteRefContextWithDB` 物理删除被清除或覆盖的旧 `secret_values` 密文行，阻断通过历史构建快照解密已吊销密钥**）

**25. `build_configure_target_pipeline`** — 纯增量配置部署目标的代码源/镜像源模式、Dockerfile/构建模板、构建规格、自动构建发版策略与生命周期 Hook 绑定（`L2`，先调 `delivery_get_target` 查看现值）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- `sourceType`: `"repository" | "image"`，可选，切换构建源模式
- `imageRef`: `string`，可选，当最终 `sourceType == "image"` 且目标尚无 `imageRef` 时必填（显式传入时同步刷新 `deployment_targets.image_ref_updated_at = NOW()` 对齐第 1 批 `delivery_deploy_target` 基准镜像判定）
- `repositoryBinding`: `string`，可选，绑定的仓库 ID 或 `owner/repo`（当最终 `sourceType == "repository"` 且存量无绑定时必填，对齐 `deployment_target.repository_binding_required`）
- `buildDefinitionMode`: `"repository_dockerfile" | "template"`，可选
- `dockerfilePath`: `string`，可选，仓库内干净相对路径如 `"Dockerfile"` 或 `"services/api/Dockerfile"`（与第 1 批 `delivery_create_target` 共用路径安全校验器，**严禁 `/` 绝对路径或 `..` 父目录逃逸**）
- `buildContext`: `string`，可选，仓库内干净相对路径如 `"."` 或 `"services/api"`（**严禁绝对路径或 `..` 路径逃逸**）
- `buildDirectory`: `string`，可选，仓库内干净相对子目录（严禁绝对路径或 `..` 逃逸；恢复仓库根目录传 `clearBuildDirectory: true`）
- `clearBuildDirectory`: `boolean`，可选
- `buildTemplate`: `string`，可选，构建模板 ID（传入且未指定 `buildDefinitionMode` 时自动切为 `"template"`；**当切换为不同 `buildTemplate` 时，Service 层自动清理不属于新模板参数表 `Parameters` 的旧模板参数键，防止残留旧键触发校验失败**）
- `buildTemplateVersion`: `string`，可选
- `upsertTemplateValues`: `record<string, string>`，可选，增量设置构建模板参数
- `removeTemplateValueKeys`: `string[]`，可选，删除指定构建模板参数
- `upsertBuildArgs`: `array<{key: string, value: string}>`，可选，增量设置 Docker `--build-arg`（**禁止 `LITEYUKI_*` 平台保留前缀**，避免被 Worker `NormalizedBuildEnv` 静默剥离）
- `removeBuildArgKeys`: `string[]`，可选，删除指定 `--build-arg`
- `buildCpuRequest`: `string`，可选，如 `"2"`
- `buildMemoryRequest`: `string`，可选，如 `"4Gi"`
- `buildTimeoutSeconds`: `integer`，可选，范围 `60..86400`
- `targetRegistry`: `string`，可选，目标制品库 ID 或名称（当最终 `sourceType == "repository"` 且存量 `TargetRegistryID == ""` 时，若省略 `targetRegistry`，**Service 层自动调用 `defaultRegistryFor` 解析并持久化项目默认制品库权威 ID**；若项目亦无默认制品库则按 `R11` 返回明确提示必传）
- `targetRepository`: `string`，可选，自定义目标镜像仓库路径
- `targetTag`: `string`，可选，自定义目标镜像默认 Tag
- `resetTargetImage`: `boolean`，可选，清空自定义 `targetRepository / targetTag`，恢复按制品库凭据模板自动推导
- `attachVariableSets`: `string[]`，可选，绑定构建变量集（ID 或名称）
- `detachVariableSets`: `string[]`，可选，解绑构建变量集
- `autoDeploy`: `boolean`，可选，构建成功后是否自动创建 `Release`（注意：底层 `deployment_auto.go:30` 同时要求 `requireApproval == false` 才会自动发版！）
- `requireApproval`: `boolean`，可选，自动构建产物的发版门禁开关（设为 `true` 时阻止 `autoDeploy` 自动发版，需人工触发发布）
- `buildConcurrencyPolicy`: `"queue" | "cancel_in_progress" | "parallel"`，可选
- `branchPattern`: `string`，可选，自动触发分支匹配规则（清空传 `clearBranchPattern: true`）
- `clearBranchPattern`: `boolean`，可选
- `tagPattern`: `string`，可选，自动触发 Tag 匹配规则（清空传 `clearTagPattern: true`）
- `clearTagPattern`: `boolean`，可选
- `buildHooksEnabled`: `boolean`，可选
- `upsertHookBindings`: `array<{hook: string, phase: "prePull" | "postPull" | "preBuild" | "postBuild" | "prePush" | "postPush" | "preDeployment" | "postDeployment", runOrder?: integer}>`，可选，按 `(hook, phase)` 新增或更新顺序（**实现契约**：持久化层保留显式传入的 `runOrder` 整数值而非硬编码改写为 `index + 1`，省略 `runOrder` 时默认取当前阶段 `max(run_order) + 1`）
- `removeHookBindings`: `array<{hook: string, phase: "prePull" | "postPull" | "preBuild" | "postBuild" | "prePush" | "postPush" | "preDeployment" | "postDeployment"}>`，可选，解绑指定阶段 Hook
- 实现与返回契约：
  1) **存量运行时配置与持久化卷挂载 100% 隔离保护**：在 `DeploymentTarget` `FOR UPDATE` 行锁事务内，原样保留全部运行时规格/环境变量/密钥/文件/K8s 高级列与 `BuildEnvironmentID`，**严禁调用 `syncDeploymentTargetVolumeMounts`**（杜绝因 `dataVolumes = nil` 误解绑全部持久化 PVC！），且仅当传入 `upsertHookBindings` 或 `removeHookBindings` 时更新 `DeploymentTargetHookBinding`；
  2) **修复 `BUILD_DIRECTORY` 执行器消费与分模式软链防逃逸**：`internal/builder/executor/run.sh` 克隆源码目录为 `/workspace/source`——① `template` 构建模式固定使用只读挂载的 `/executor/template.Dockerfile`（不对 `/executor` 做 `/workspace/source` 校验），同时对 `context` 与 `BUILD_DIRECTORY` 执行 `realpath` 校验；② `repository_dockerfile` 构建模式对 `context`、`BUILD_DIRECTORY`、`dockerfile_dir` **以及 `$dockerfile_dir/$dockerfile_name` 文件本身**一律执行 `realpath` 强校验（严格要求解析路径等于 `/workspace/source` 或以 `/workspace/source/` 开头，防根级软链 `Dockerfile -> ../home/.docker/config.json` 或 `/workspace/source-evil` 前缀绕过）；克隆完成后的 `postPull / preBuild / prePush / postPush / postBuild` Hook 在 **`run_hook` 子 Shell 内**切换至 `/workspace/source/$BUILD_DIRECTORY`（`prePull` 保持原目录，严禁改动顶层 `$PWD`），并在 `buildDefinitionMode == "template"` 且 `BUILD_CONTEXT == "."` 时将 `/workspace/source/$BUILD_DIRECTORY` 作为构建上下文；
  3) 按实际改动字段返回精确 `requiresRelease: boolean`（仅当修改了 `imageRef` 时为 `true`；纯修改代码构建流水线/触发规则/Hook 绑定时返回 `requiresRelease: false`，需先运行 `build_trigger_run` 产出新镜像后再部署）。

**26. `build_trigger_run`** — 异步触发一次基于当前最新配置的容器镜像构建任务（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，可选，支持 `targetId | stage | name`（省略时仅当该应用下存在**唯一 1 个** `enabled && sourceType=="repository"` 活跃目标时自动选中，存在多个环境时按 `R8` 返回 `409 ai.resource_ambiguous` 候选列表，严禁盲选首个环境误触生产！）
- `sourceBranch`: `string`，可选，构建分支（严禁以 `-` 开头防 Git 参数注入；与 `sourceTag` 互斥，均省略时取绑定仓库默认分支）
- `sourceTag`: `string`，可选，构建 Git Tag（严禁以 `-` 开头；与 `sourceBranch` 互斥）
- `sourceCommit`: `string`，可选，精确十六进制 Commit SHA（`^[0-9a-fA-F]{7,64}$`）
- `targetTag`: `string`，可选，覆盖本次构建产物镜像 Tag
- `overrideBuildArgs`: `record<string, string>`，可选，仅对本次构建在目标 `buildArgs` 基础上逐键合并覆盖的 `--build-arg`（**禁止 `LITEYUKI_*` 保留前缀**）
- 实现契约说明（跨层全链路同步）：
  1) **修复 API 入参吞没**：`prepareBuildRunRequest` 在目标 `BuildArgs` 之上逐键合并 `overrideBuildArgs`，且显式传入的 `targetTag` 严禁被 `config.TargetTag` 或凭据模板覆盖；
  2) **修复 `BuildRuntime -> Worker -> Executor` Tag/Commit 检出链路**：当 `run.SourceTag != ""` 时，`buildruntime.ResolveBuildTask`（`internal/buildruntime/runtime.go:118`）严禁将 `SourceBranch` fallback 为默认分支；`internal/builder/executor/run.sh` 对 `SOURCE_TAG` 使用 `--branch "$SOURCE_TAG"` 克隆且与 `SOURCE_BRANCH` 互斥 checkout（绝不在 checkout Tag 后再被 `SOURCE_BRANCH` 切回分支 HEAD），对指定历史 `SOURCE_COMMIT` 在浅克隆未命中时执行 `git fetch --depth 1 origin "$SOURCE_COMMIT"`；
  3) `prepareBuildRunRequest`（`build_validation.go:170`）与 Worker `buildruntime.ResolveBuildTask`（`runtime.go:57`）必须同步校验父级 `ArtifactRegistry`（`run.TargetRegistryID`）对 `(actor, run.ProjectID)` 的作用域可见性（阻断跨用户暴露私有制品库 `Endpoint` 或越权推送）；若目标 `enabled == false` 返回 `409 deployment_target.disabled`；成功写入业务 `AuditLog` 并返回权威 `projectId` 与 `buildRunId`（`status: "queued"`），供 `wait_for_task(taskType: "build_run")` 与 `present_card(template: "live_task")` 内嵌 `live_progress(operationType: "build_run")` 绑定。

**27. `build_retry_run`** — 基于历史构建当时的配置与变量快照重新排队构建（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `buildRunId`: `string`，必填，历史构建记录 ID
- 实现契约说明：若所属部署目标 `enabled == false` 对齐返回 `409 deployment_target.disabled`；**在 `prepareBuildRunRequest`（`build_validation.go:170`）与 `buildruntime.ResolveBuildTask`（`runtime.go:57`）双端强制复核历史 `previous.TargetRegistryID` 指向的父级 `ArtifactRegistry` 当前仍对 `(actor, run.ProjectID)` 可见**（严防制品库从项目解绑撤权后仍凭成员个人 `scope == "user"` 凭据向他项专属制品库推送产物！）；**修复 `prepareBuildRunRequest`（`build_validation.go:106-139, 203`）覆盖历史快照与跨用户/已撤权变量集泄露缺陷**——当 `TriggerType == "retry"` 时，严格保留原 `BuildRun` 快照中的 `BuildDefinitionMode / BuildTemplateID / BuildTemplateVersion / BuildTemplateValues / BuildTemplateChecksum / BuildTemplateDockerfile / DockerfilePath / BuildContext / BuildDirectory / BuildArgs / BuildEnvironmentID / BuildVariableSetIDs`（当原记录 `BuildVariableSetIDs == ""` 时禁止从当前 `config.BuildVariableSetIDs` 回填污染）；**将 `buildVariableSetIDs(previous.BuildVariableSetIDs)` 与 `previous.BuildSecretRefsSnapshot` 中所有 `secret-id:*` 在 `secret_values.resource`（`"build_variable_set:<setID>:<key>"`）反查出的 `<setID>`（含自动注入的项目 `defaultSets`）取并集**，只要并集非空即逐一执行 `Unscoped` 安全复核（无视 `BuildSecretRefsSnapshot` 是否为空，同时覆盖纯变量个人集）：**① 若原变量集为 `scope == "user"` 且 `ownerRef != currentUser.ID` 则返回 `403` 拒绝重试（严防同项目其他成员挂载 Hook 窃取原触发人个人私有变量或密钥！）；② 若原变量集未软删除（`deleted_at IS NULL`）且 `scope == "project"` 则强制校验当前仍绑定 `run.ProjectID`（防撤权后继续解密共享变量集密钥；仅对已软删除的 `project / global` 历史变量集放行快照复用）**；写入业务 `AuditLog` 并返回权威 `projectId` 与新建 `buildRunId`（`status: "queued"`）。**说明书强防呆提示（`R9`）**：重试会原样复用历史构建当时的全部配置与变量快照；**若刚修改过构建变量、密钥或构建流水线配置，请勿调用 `build_retry_run`，必须调用 `build_trigger_run` 才会读取最新配置**！

**28. `build_cancel_run`** — 终止排队中或运行中的构建任务（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `buildRunId`: `string`，必填，构建记录 ID（`queued / running` 可取消；Service 层对已 `canceled` 状态幂等返回成功，其余终态统一返回 `409 build.not_cancelable`）

**29. `build_delete_run`** — 删除已结束（终态）的构建记录及其构建日志与关联 Hook 日志（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `buildRunId`: `string`，必填，仅终态（`succeeded / failed / canceled / timeout`）构建可删除（返回 `{ id: "bldr_xxx", projectId: "prj_xxx", deleted: true }`，写后校验通过 `#11 build_get_run` `404 -> absent` 确认）

---

## 第 3 批：`runtime`（运行集群、运行时排障、项目数据卷、生命周期脚本）+ `network`（网关路由、服务依赖绑定与拓扑域）

> 遵守 `notes/2026-11-19-ai-domain-tools-catalog.md` 第 0 节全局约定（双模定位与权威 `projectId` 回显 `R8`、零全量覆盖与三态防呆 `R5/R6/R7`、删除物理独立与防会话记忆穿透 `R3`、Secret `writeOnly + x-luna-sensitive`、实时状态 `no-store` 与异步任务契约精确对齐）。
> 跨批次边界、项目会话沙箱与权限自洽说明：
> 1. **运行集群三层作用域与项目绑定会话沙箱（`aiToolExecutionIdentityMiddleware` + Service 层）对齐**：
>    - `RuntimeCluster` 与 `GitAccount / ArtifactRegistry / BuildVariableSet` 共用 `scopedResource` 三层权限——普通用户可维护 `scope="user"` 集群，项目 Owner/Admin 可维护 `scope="project"` 集群，仅 `scope="global"`、`isDefault=true` 及特权网关转发头要求 `platform_admin`。仅 `platform_admin` 独占的系统组件安装（`admin_list_system_components / admin_install_system_component`）归入第 5 批 `admin` 域；
>    - **项目绑定会话（`conversationProjectID != ""`）中间件与 Service 双层闭环（零 `403` 死锁、零跨项穿透）**：
>      1) **中间件层（`internal/api/aiapi/ai_tool_identity.go`）**：凡请求显式传入的任何 `project / projects / addProjects / removeProjects`（含嵌套 `upsertGrants[].project`）均严格比对 `conversationProjectID`（`id | identifier`）；对支持自动收敛或存量复核的工具（`#1, #2, #10, #13`），当 `len(projectIDs) == 0` 或 `scope == "user"` 时中间件拒绝显式 `scope == "global"` 并放行至 Service 层（不提前抛 `403` 误杀模型调用）；
>      2) **Service 层归属校验**：`#1 / #2` 省略 `project` 时自动注入 `conversationProjectID`（`#2` 各子视图仅返回关联 `conversationProjectID` 的策略/例外/共享/K8s 资源，且 `k8s_resource_detail` 与 `#16` 强制校验受管资源 `snapshot.ProjectID == conversationProjectID`）；`#10 / #12` 对存量态与目标态独立强制 `(scope == "user" && ownerRef == currentUser.ID) || (scope == "project" && equalSingle(projectIDs, conversationProjectID))`（禁止在单项目会话中操作 `global` 或跨项目集群）；`#11` 仅允许探测对 `conversationProjectID` 可用的集群且复用结构化 DTO；`#13` 新建强制要求 `upsertGrants` 非空且仅含 `conversationProjectID`，`#13` 更新与 `#14` 删除强制校验**数据库存量非空单项目精确匹配 `len(existing.Grants) == 1 && existing.Grants[0].ProjectID == conversationProjectID`**（空 `Grants` 草稿或含他项授权一律返回 `403 ai.project_scope_mismatch`，且 `#13` 更新后目标态 `Grants` 仍须非空仅含 `conversationProjectID`——若在单项目会话传 `removeProjects` 试图清空唯一项目授权则返回 `400 argument_invalid` 指引直接调 `runtime_delete_cluster_egress_exception`）；`#15` 强制校验调用者具备集群维护权、`conversationProjectID` 可使用该集群且 **`share.ProviderProjectID == conversationProjectID`**（`L2` 预检显著展示 `providerProject -> consumerProject`）；
>      3) **写后验证与错误码对齐**：协同升级 `outcome.ts`（动态读取 `readOperation.inputSchema.properties.pageSize.maximum` 封顶 `pageSize <= 50` 并支持 `staticArguments`），且 `internal/provider/kubernetes` `getGatewayAPIResource` 保留 `apierrors.IsNotFound` 错误链（`%w`）、导出非受管哨兵错误 `ErrResourceUnmanaged`，确保 11 类白名单资源 100% 命中 `404 cluster.resource_not_found` / `403 cluster.resource_unmanaged`（严禁吐出 `502`）。
> 2. **项目级网络隔离策略与跨项目网络共享（租户侧）**：项目的 `IngressMode / EgressMode / EgressAllowlist`（`workspace_get_network_policy / workspace_configure_network_policy`）、租户发起/接受/撤销跨项目共享（`workspace_list_network_shares / workspace_create_network_share / workspace_accept_network_share / workspace_delete_network_share`）以及项目级近 7 天构建/发布/网关诊断（`workspace_get_project(includeDiagnostics: true)`）统一归入第 4 批 `workspace` 域，跨批无空白、无重叠。

---

### 3A. `runtime`（运行集群、工作负载排障、项目数据卷与 Hook 域，共 25 个工具）

#### 3A.1 只读诊断与观测层（9 个，固定 `L0`）

**1. `runtime_list_clusters`** — 分页查看当前可见的运行集群配置摘要、网关域名后缀及实时负载等级
- `cluster`: `string`，可选，按运行集群 ID（`clu_xxx`）或精确名称定位单个集群（闭环从 `effectiveClusterId` 反查及 `#10/#11/#12` 写后校验）
- `project`: `string`，可选，按项目可用集群筛选（传入时回显权威 `projectId`）
- `includePressure`: `boolean`，可选，默认 `true`
  - 实现契约说明：严格遵守跨租户隔离（`runtime_cluster_pressure_handlers.go:87`）——对普通租户用户仅返回定性 `pressureLevel`（`"idle" | "light" | "moderate" | "heavy" | "full" | "unavailable"`）与 `status / observationCode`，绝不泄露全局集群物理节点数与容量明细；仅 `platform_admin` 额外返回 `pressureScore` 与 `details`。
- `search`: `string`，可选，按集群名称或 Endpoint 模糊搜索
- `sortBy`: `"createdAt" | "name" | "scope"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）
- 返回契约：DTO 层将底层 JSON/换行分隔列（`GatewayDefaultRequestHeaders / GatewayDefaultResponseHeaders`、域名后缀与全部 CIDR/Port 列表）反序列化为结构化 `record<string, string>` 与 `string[]` 数组返回，严禁吐出二次转义文本。

**2. `runtime_inspect_cluster_governance`** — 查看可管理或可使用集群的 NetworkPolicy 对账状态、出站例外清单、待审批跨项目共享或项目受管 K8s 资源/脱敏 YAML/事件
- `cluster`: `string`，必填，运行集群 ID（`clu_xxx`）或唯一名称
- `section`: `"network_policies" | "egress_exceptions" | "pending_network_shares" | "k8s_resources" | "k8s_resource_detail"`，必填，明确单一子视图（杜绝异构返回与爆 Token）
- `exception`: `string`，可选，`section="egress_exceptions"` 时按出站例外 ID（`nex_xxx`）或同集群唯一 `name` 精确过滤（闭环 `#13 / #14` 写后校验）
- `shareId`: `string`，可选，`section="pending_network_shares"` 时按跨项目共享记录 ID 精确过滤（闭环超过 50 条时 `#15 runtime_approve_cluster_network_share` 写后校验）
- `project`: `string`，可选，按项目筛选（普通用户自动按 `filterClusterResourceSnapshots` 仅可见自身有权限项目的受管资源；项目绑定会话自动注入 `conversationProjectID`）
- `application`: `string`，可选，`k8s_resources` 按应用筛选
- `target`: `string`，可选，`k8s_resources` 按部署目标（`targetId | stage | name`）筛选
- `resourceCategory`: `"namespaces" | "workloads" | "services" | "configs" | "storage"`，可选，`section="k8s_resources"` 列表模式时必填（对齐 `runtime_cluster_resource_contract.go:10`）
- `resourceKind`: `"Namespace" | "Deployment" | "StatefulSet" | "Pod" | "HorizontalPodAutoscaler" | "Service" | "HTTPRoute" | "Gateway" | "ConfigMap" | "Secret" | "PersistentVolumeClaim"`，可选，`section="k8s_resource_detail"` 单资源详情模式时必填（对齐 11 类白名单）
- `namespace`: `string`，可选，`k8s_resources` 过滤或 `k8s_resource_detail`（非 `Namespace` 资源必填）
- `resourceName`: `string`，可选，`section="k8s_resource_detail"` 时必填，返回脱敏 YAML（`Secret` 自动脱敏，且对超大 `ConfigMap` 等施加 `65536` 字节 UTF-8 截断保护并返回 `truncated: boolean, truncatedBytes?: integer`）与首页（`page=1`）关联 K8s Events
- `page`: `integer`，可选，默认 `1`（注：K8s Events 接口固定仅查 `page=1`）
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）
- 实现契约说明：当 K8s 返回 `apierrors.IsNotFound` 时统一映射为 `404 cluster.resource_not_found`，非平台受管资源映射为 `403 cluster.resource_unmanaged`（严禁笼统返回 `502 Bad Gateway` 触发 `luna-api-client` 退避重试或破坏删除后 `404` 校验）。

**3. `runtime_get_target_diagnostics`** — 一站式读取单个部署环境的健康诊断快照（容器排障首选入口）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- 实现契约说明：在现有 `GetDeploymentTargetDiagnostics`（目标状态、异常优先 Top 10 Pods、Warning 优先 Top 10 Events、最近 5 次 Releases、关联网关路由与实时 TLS 证书状态）基础上，**Service 层补齐聚合该目标的 `volumeMounts`（含 `activationState` 及关联 `ProjectVolume` 实时 PVC `phase / observationCode`）**，覆盖存储挂载阻塞 Pod 启动的排障闭环。

**4. `runtime_list_target_pods_and_events`** — 查看部署环境当前的 K8s Pod 明细列表与集群事件流
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`
- `releaseId`: `string`，可选，按特定发布版本筛选关联 Pod
- `eventType`: `"Warning" | "Normal"`，可选，筛选事件级别（排障推荐 `"Warning"`）
- `search`: `string`，可选，按 Pod 名、事件原因或消息过滤
- `includePods`: `boolean`，可选，默认 `true`
- `includeEvents`: `boolean`，可选，默认 `true`
- `podLimit`: `integer`，可选，默认 `20`（上限 `50`）
- `eventLimit`: `integer`，可选，默认 `20`（上限 `50`）

**5. `runtime_get_pod_logs`** — 读取部署环境或指定发布版本的容器实时/崩溃前日志（带服务端字节流硬上限）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，可选，传 `target` 时必填
- `target`: `string`，可选，支持 `targetId | stage | name`（`target` 与 `releaseId` 至少传其一；同时传入时在指定环境下筛选特定 `releaseId` 的 Pod）
- `releaseId`: `string`，可选，发布记录 ID
- `pod`: `string`，可选，指定单个 Pod 名称（省略且 `preferAbnormal=true` 时自动优先选取异常 Pod）
- `container`: `string`，可选，指定普通容器或 `initContainer` 名称
- `previous`: `boolean`，可选，默认 `false`，读取上一次崩溃退出容器的终止日志
- `preferAbnormal`: `boolean`，可选，默认 `true`
- `timestamps`: `boolean`，可选，默认 `true`
- `tailLines`: `integer`，可选，默认 `200`（上限 `1000`）
- `maxBytes`: `integer`，可选，默认 `32768`（上限 `65536`）
- 实现契约说明：Provider/Service 层必须对日志流施加硬上限读取与 UTF-8 字符边界尾部截断（严禁裸 `io.ReadAll` 无上限读入内存），响应返回 `truncated: boolean` 与 `truncatedBytes?: integer`。

**6. `runtime_list_volumes`** — 分页列出项目持久化数据卷（`ProjectVolume`）、实时 PVC 观测状态及集群可用 StorageClass
- `project`: `string`，必填，项目 ID 或 `identifier`
- `cluster`: `string`，可选，集群 ID 或名称（传入时 Service 层附带查询该集群 `storageClasses` 列表；若集群暂不可达仅标记 `storageClassesStatus: "unavailable"`，不阻断卷列表返回）
- `availability`: `"available" | "reserved" | "in_use" | "unavailable"`，可选（精确对齐 `model/project_volume.go:25-28`）
- `lifecycleState`: `"provisioning" | "ready" | "deleting" | "error"`，可选
- `sourceKind`: `"blank" | "managed" | "retained" | "archive_import" | "snapshot_restore" | "existing_claim"`，可选（精确对齐 `model/project_volume.go:13-18` 全部 6 类来源枚举）
- `ownershipMode`: `"managed" | "referenced"`，可选
- `volumeMode`: `"Filesystem" | "Block"`，可选
- `search`: `string`，可选，按显示名或 PVC Claim 名搜索
- `sortBy`: `"createdAt" | "updatedAt" | "displayName" | "capacity"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**7. `runtime_get_volume`** — 查看单个项目数据卷详情、当前 `revision`、挂载绑定列表与导入导出传输记录
- `project`: `string`，必填，项目 ID 或 `identifier`
- `volume`: `string`，可选，数据卷 ID（`pvol_xxx`）或同项目唯一 `displayName`（**`volume` 与 `transferId` 至少传其一**）
- `transferId`: `string`，可选，传输任务 ID（`vtx_xxx`；单独传入时由 Service 层经 `GetVolumeTransfer` 反解所属项目数据卷并一并返回卷详情与该条传输进度，闭环 `#22 / #23` 写后校验）

**8. `runtime_list_hooks`** — 分页列出项目生命周期脚本配置（`ProjectHookConfig`），或查看单个脚本完整内容
- `project`: `string`，必填，项目 ID 或 `identifier`
- `hook`: `string`，可选，传入 Hook ID（`hook_xxx`）或同项目唯一 `name` 时返回该脚本完整 `script` 正文；列表模式下省略原始 `64KB` `script`，仅返回 `scriptBytes / scriptLines / scriptPreview`（`<= 512` 字符）防爆上下文（`R10`）
- `sortBy`: `"createdAt" | "name"`，可选，默认 `"createdAt"`（注：`ProjectHookConfig` 表无 `phase` 列，阶段绑定在部署目标上通过 `build_configure_target_pipeline` 配置）
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**9. `runtime_list_hook_runs`** — 分页查询项目构建或发布触发的生命周期脚本执行记录，支持按需读取单条脚本快照与截断日志
- `project`: `string`，必填，项目 ID 或 `identifier`
- `buildRunId`: `string`，可选，按构建记录过滤
- `releaseId`: `string`，可选，按发布记录过滤
- `phase`: `"prePull" | "postPull" | "preBuild" | "postBuild" | "prePush" | "postPush" | "preDeployment" | "postDeployment"`，可选
- `status`: `"queued" | "running" | "succeeded" | "failed" | "canceled"`，可选（Service 层补齐 `status` SQL 过滤谓词）
- `hookRunId`: `string`，可选，传入具体 `hookRunId` 时精确返回该条记录、截断至 `32768` 字节内的 `scriptSnapshot` 以及 UTF-8 尾部截断的 `HookRunLog`（列表模式下剔除 `scriptSnapshot` 防爆上下文 `R10`）
- `logTailBytes`: `integer`，可选，默认 `8192`（上限 `32768`）
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

---

#### 3A.2 运行集群治理、受控容器 Exec、数据卷与 Hook 写操作层（16 个，全部固定 `L2`）

**10. `runtime_save_cluster`** — 创建或纯增量修改运行集群配置（`L2`，Kubeconfig `writeOnly + x-luna-sensitive`，更新时严禁重置未传字段）
- `cluster`: `string`，可选，省略为新建，传入集群 ID 或唯一名称为增量更新
- `name`: `string`，可选，新建必填
- `endpoint`: `string`，可选
- `scope`: `"user" | "project" | "global"`，可选，新建传非空 `addProjects` 默认 `"project"` 否则默认 `"user"`（`"global"` 与 `isDefault=true` 仅 `platform_admin` 在全局会话可用；**更新省略时严格保留原 `scope / ownerRef / projectIds`**）
- `addProjects`: `string[]`，可选，`scope="project"` 时增量授权项目列表（新建可直接传）
- `removeProjects`: `string[]`，可选，`scope="project"` 时增量移除授权项目
- `kubeconfig`: `string`，可选，内联安全 HTTPS Kubeconfig（`writeOnly: true, x-luna-sensitive: true`；更新时省略保留原凭据，仅创建者或平台管理员可替换）
- `isDefault`: `boolean`，可选，仅 `global` 集群可设
- `maxConcurrentBuilds`: `integer`，可选
- `resourcePolicy`: `{cpuRequestPercent?: integer, memoryRequestPercent?: integer, cpuLimitPercent?: integer, memoryLimitPercent?: integer}`，可选
- `gatewayConfig`: `{addDomainSuffixes?: string[], removeDomainSuffixes?: string[], publicScheme?: "http" | "https", publicPort?: integer, controllerType?: "traefik" | "generic", className?: string, gatewayName?: string, gatewayNamespace?: string, httpListenerName?: string, httpListenerPort?: integer, httpsListenerName?: string, httpsListenerPort?: integer, tlsSecretName?: string, tlsSecretNamespace?: string, clearTlsSecret?: boolean, certIssuerKind?: "ClusterIssuer" | "Issuer", certIssuerName?: string, clearCertIssuerName?: boolean, certificateNamespace?: string, clearCertificateNamespace?: boolean, wildcardCertEnabled?: boolean, wildcardCertDomain?: string, wildcardCertSecretName?: string, clearWildcardCert?: boolean, externalTlsMode?: "none" | "gateway" | "upstream", forwardedHeadersMode?: "preserve" | "overwrite" | "none", addTrustedProxyCidrs?: string[], removeTrustedProxyCidrs?: string[], clearTrustedProxyCidrs?: boolean, upsertDefaultRequestHeaders?: record<string, string>, removeDefaultRequestHeaderKeys?: string[], upsertDefaultResponseHeaders?: record<string, string>, removeDefaultResponseHeaderKeys?: string[]}`，可选
- `networkSecurity`: `{networkGatewayNamespace?: string, clearNetworkGatewayNamespace?: boolean, addNetworkBlockedCidrs?: string[], removeNetworkBlockedCidrs?: string[], buildEgressMode?: "restricted" | "permissive", addBuildPrivateEgressCidrs?: string[], removeBuildPrivateEgressCidrs?: string[], addBuildPrivateEgressPorts?: string[], removeBuildPrivateEgressPorts?: string[]}`，可选
- 实现契约说明：更新模式下必须先从数据库加载 `existing` 完整记录，仅合并本次显式传入的非 `null` 子字段与显式 `clear*` 标记（例如 `clearNetworkGatewayNamespace: true` 重置为空串恢复继承 `gatewayNamespace`；`trustedProxyCidrs` 严格走 `add/remove/clear` 增量集合操作），严禁对未传字段套用新建默认值覆盖；响应同 `#1` 输出结构化对象/数组。

**11. `runtime_test_cluster`** — 主动探测运行集群 API Server 连接与权限并记录 `AuditLog`（固定 `L2`，防止会话记忆后高频主动探测集群，Service 层补齐业务审计，响应同 `#1 / #10` 返回结构化解码 DTO）
- `cluster`: `string`，必填，集群 ID 或名称

**12. `runtime_delete_cluster`** — 异步移除运行集群配置（`L2`，仍有活跃工作负载或数据卷时拦截，返回 `{ id: "clu_xxx", deleteStatus: "deleting" }`）
- `cluster`: `string`，必填，集群 ID 或名称

**13. `runtime_save_cluster_egress_exception`** — 创建或增量更新集群受限网段出站例外及项目授权（`L2`，需具备该集群维护权限）
- `cluster`: `string`，必填，集群 ID 或名称
- `exception`: `string`，可选，省略为新建，传入例外 ID（`nex_xxx`）或同集群唯一 `name` 为增量更新
- `name`: `string`，可选，新建必填
- `cidr`: `string`，可选，新建必填
- `ports`: `string`，可选，逗号分隔端口如 `"443,5432"`（省略不改；更新为放行该协议全部端口时传 `clearPorts: true`）
- `clearPorts`: `boolean`，可选，清空端口限制以放行该协议全部端口
- `protocol`: `"tcp" | "udp" | "any"`，可选，新建默认 `"tcp"`（精确对齐小写枚举 `runtime_cluster_egress_exception_handlers.go:259`）
- `reason`: `string`，可选，新建必填，审计原因说明
- `upsertGrants`: `array<{project: string, targets?: string[], allTargets?: boolean, includeBuilds?: boolean}>`，可选，按项目新增或更新例外授权（`targets` 支持传 `dplt_xxx` 或 `<appIdentifier>:<stage>`，与 `allTargets: true` 互斥；**`R5/R7` 防扩权铁律**：新建项目授权省略 `targets` 默认整项目生效，但更新既有项目授权时省略 `targets` 严格保持原目标范围不变，仅在显式传 `allTargets: true` 时放宽为整项目有效！）
- `removeProjects`: `string[]`，可选，撤销指定项目的该条出站例外授权（统一命名为 `removeProjects` 以对齐全局项目沙箱字段识别；多项目例外在全局会话下局部撤权使用，单项目绑定会话下若试图删空当前项目唯一授权则返回 `400 argument_invalid` 指引改调 `runtime_delete_cluster_egress_exception`）

**14. `runtime_delete_cluster_egress_exception`** — 删除集群出站例外及其全部项目授权并触发受影响项目 NetworkPolicy 对账（`L2`，项目绑定会话下强制要求 `len(existing.Grants) == 1 && existing.Grants[0].ProjectID == conversationProjectID`，空 `Grants` 或含他项均 `403` 拦截；同步删除返回 `{ id: "nex_xxx", deleted: true }`）
- `cluster`: `string`，必填，集群 ID 或名称
- `exception`: `string`，必填，例外 ID 或同集群唯一 `name`

**15. `runtime_approve_cluster_network_share`** — 作为运行集群维护者批准持有出站例外的项目对外提供的跨项目内网服务共享（`L2`）
- `cluster`: `string`，必填，集群 ID 或名称
- `shareId`: `string`，必填，待审批共享记录 ID

**16. `runtime_delete_cluster_k8s_resource`** — 删除运行集群中受控 11 类白名单内的单个受管 K8s 孤儿/异常资源（`L2`，项目级需 `ActionClusterManage` 权限）
- `cluster`: `string`，必填，集群 ID 或名称
- `resourceKind`: `"Namespace" | "Deployment" | "StatefulSet" | "Pod" | "HorizontalPodAutoscaler" | "Service" | "HTTPRoute" | "Gateway" | "ConfigMap" | "Secret" | "PersistentVolumeClaim"`，必填（与 `#2` 参数名严格统一以支持自动写后校验）
- `namespace`: `string`，可选，非 `Namespace` 资源时必填
- `resourceName`: `string`，必填（与 `#2` 参数名严格统一；K8s `IsNotFound` 返回 `404 cluster.resource_not_found`，非受管资源返回 `403 cluster.resource_unmanaged`）

**17. `runtime_exec_command`** — 在已开启 Web 控制台权限的线上容器中通过 `/bin/sh -lc` 执行单次非交互诊断命令（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `releaseId`: `string`，可选，指定发布记录 ID（若省略则必填 `application + target`）
- `application`: `string`，可选，省略 `releaseId` 时必填
- `target`: `string`，可选，省略 `releaseId` 时必填（支持 `targetId | stage | name`）
- `pod`: `string`，可选，指定 Pod 名称
- `container`: `string`，可选，指定容器名称
- `command`: `string`，必填，单次非交互命令（`1..2000` 字符，30s 超时；无 `/bin/sh` 的 distroless/scratch 镜像不适用）
- 实现契约说明：省略 `releaseId` 时，Service 层优先选取该目标最新一条 `status == "succeeded"` 的 `Release`（尚无成功版本时回退最新 `status == "running"` 版本，均无则返回 `409 deployment_target.not_deployed`）以绑定 `release_runtime.exec` 审计；输出 `stdout / stderr` 在工具层按 `32768` 字节做 UTF-8 截断保护；同步更新 `luna-agent/src/tools/outcome.ts:57` 精确匹配 `operation.operationId === "runtime_exec_command"`（替换旧 `"execReleaseRuntimeCommand"`，严禁泛化至全部 `effect === "execute"` 以免误杀无 `exitCode` 的 `runtime_test_cluster` 等工具）。

**18. `runtime_create_volume`** — 异步创建空白托管数据卷、从快照恢复数据卷或纳管已有 PVC（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `displayName`: `string`，必填，数据卷显示名称
- `cluster`: `string`，必填，目标集群 ID 或名称
- `sourceType`: `"blank" | "volumeSnapshot" | "existingClaim"`，必填（精确对齐 `project_volume_handlers.go:455`）
- `capacity`: `string`，可选，`blank / volumeSnapshot` 时必填（如 `"10Gi"`；`existingClaim` 模式忽略并从现有 PVC 自动读取）
- `storageClassName`: `string`，可选，`blank / volumeSnapshot` 时可传（若省略，Service 层自动查询集群唯一 `isDefault == true` 的 StorageClass 填入；若无默认或存在多个默认类，按 `R11` 返回可用 `storageClasses` 候选列表提示显式指定；创建前亦可先调 `runtime_list_volumes(project, cluster)` 查看）
- `accessMode`: `"ReadWriteOnce" | "ReadWriteMany" | "ReadOnlyMany" | "ReadWriteOncePod"`，可选，默认 `"ReadWriteOnce"`
- `volumeMode`: `"Filesystem" | "Block"`，可选，默认 `"Filesystem"`
- `claimName`: `string`，可选，`sourceType="existingClaim"` 时必填
- `ownershipMode`: `"managed" | "referenced"`，可选，仅 `sourceType="existingClaim"` 时可选（默认 `"managed"`；`"managed"` 删卷时物理删除 PVC，`"referenced"` 删卷时仅 `detach` 保留底层 PVC）
- `snapshotName`: `string`，可选，`sourceType="volumeSnapshot"` 时必填
- 实现契约说明：**优先消费 `luna-agent` 请求头传入的确定性 `Idempotency-Key`（`aiidem_<sha256>`，确保 `UnknownToolOutcomeError` 未知结果经 `retryFailed` 换新 `toolCallId` 重试时绝不重复创建 PVC；仅无该请求头时按 `(runId, toolCallId)` 兜底派生）**；说明书提示对 `WaitForFirstConsumer` 存储类，新建卷处于 `lifecycleState="provisioning"` 时即可直接通过 `delivery_configure_target_volumes` 挂载并发版触发首次消费者绑定，无需死等 `ready`（且数据卷操作不走 `wait_for_task`）。

**19. `runtime_update_volume`** — 重命名项目数据卷或异步在线扩容 PVC 容量（`L2`，审批快照锁定 `expectedRevision`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `volume`: `string`，必填，数据卷 ID 或同项目唯一 `displayName`
- `displayName`: `string`，可选，新显示名称
- `capacity`: `string`，可选，扩容目标容量（须大于当前容量）
- `expectedRevision`: `integer`，可选，乐观锁版本号
- 实现契约说明：生成 `L2` 审批快照（`R12`）时若未显式传入 `expectedRevision`，服务端读取并锁定当时的 `volume.revision` 写入审批载荷；批准执行阶段严格携带该锁定 `expectedRevision` 提交（审批期间若卷已被并发改动则返回 `409` 拒绝盲覆盖）。

**20. `runtime_retry_volume_operation`** — 重试失败的数据卷创建（`provision`）、扩容（`expand`）或删除（`delete`）操作（`L2`，不支持传输任务重试）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `volume`: `string`，必填，数据卷 ID 或 `displayName`
- `expectedRevision`: `integer`，可选（同 `#19` 在 `L2` 审批生成阶段读取并锁定 `revision`，重试 `delete` 校验 `volume:delete` 权限，重试 `provision/expand` 校验 `volume:write` 权限）

**21. `runtime_delete_volume`** — 异步删除托管数据卷或解管引用数据卷（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `volume`: `string`，必填，数据卷 ID 或 `displayName`
- `expectedRevision`: `integer`，可选
- 实现契约说明：存在活跃挂载或运行中传输任务时直接阻断；生成 `L2` 审批卡片（`R12`）时服务端调用 `PreviewProjectVolumeDeletion` 锁定当时 `revision` 并自动推导必填 `dataAction`（`ownershipMode=="managed" -> "delete"` 物理销毁 PVC；`ownershipMode=="referenced" -> "detach"` 保留底层 PVC），在审批卡片显著展示 `underlyingClaimWillBeDeleted`，执行阶段携带锁定的 `(expectedRevision, dataAction)` 提交。

**22. `runtime_create_volume_export`** — 异步创建项目数据卷归档导出任务，完成后供用户在控制台下载（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `volume`: `string`，必填，数据卷 ID 或 `displayName`
- `format`: `"tar_gz" | "raw_zst"`，可选，省略时服务端按目标卷 `volumeMode` 自动推导（`Filesystem -> "tar_gz"`，`Block -> "raw_zst"`）
- `consistency`: `"auto" | "snapshot" | "live" | "unmounted"`，可选，默认 `"auto"`（自动优先未挂载离线导出或 CSI 快照一致性导出，`Block` 卷禁止 `"live"`）
- 实现契约说明：**优先消费请求头 `Idempotency-Key`（仅缺失时按 `(runId, toolCallId)` 兜底派生，确保 `retryFailed` 幂等不重复创建导出任务）**；返回 `projectId` 与 `transferId`（注：`transferId` 不走 `wait_for_task`，可通过 `runtime_get_volume(project, transferId)` 查看进度）。

**23. `runtime_cancel_volume_transfer`** — 取消进行中的数据卷导入或导出传输任务（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `transferId`: `string`，必填，传输任务 ID（写后校验通过 `runtime_get_volume(project, transferId)` 回读）

**24. `runtime_save_hook`** — 创建或纯增量修改项目生命周期脚本（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `hook`: `string`，可选，省略为新建，传入 Hook ID（`hook_xxx`）或同项目唯一 `name` 为增量更新（同名冲突返回 `409 ai.resource_ambiguous`）
- `name`: `string`，可选，新建必填
- `script`: `string`，可选，新建必填（非空且 `<= 65536` 字节；更新时省略保留原脚本）
- `shell`: `"sh" | "bash"`，可选，新建默认 `"sh"`（精确对齐 `normalizeHookShell`，非法值按 `R11` 拒绝而非静默降级为 `"sh"`；更新时省略保留原值）
- `timeoutSeconds`: `integer`，可选，范围 `1..3600`（新建默认 `300`，越界返回 `argument_invalid`；更新时省略保留原值）
- `failurePolicy`: `"fail" | "ignore"`，可选，新建默认 `"fail"`（更新时省略保留原值）
- 备注：绑定 Hook 到具体部署目标的构建或发布阶段请调用第 2 批 `build_configure_target_pipeline`。

**25. `runtime_delete_hook`** — 删除项目生命周期脚本并自动解绑关联部署目标（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `hook`: `string`，必填，Hook ID 或同项目唯一 `name`（返回权威 `projectId` 与删除摘要 JSON）

---

### 3B. `network`（网关访问入口、服务依赖绑定与拓扑视图域，共 11 个工具）

#### 3B.1 只读观测与校验层（4 个，固定 `L0`）

**26. `network_list_gateway_routes`** — 分页列出项目的公网/网关访问入口及实时路由、TLS 证书与可访问 URL 状态
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，可选，应用 ID 或 `identifier`
- `route`: `string`，可选，传入 `routeId`（`gwr_xxx`）或唯一 `host` 精确查看单条路由完整高级配置与实时状态（同 `host` 多路径重名时返回 `409 ai.resource_ambiguous` 候选 `{id, host, path}`）
- `search`: `string`，可选，按 `host` 或 `path` 搜索
- `sortBy`: `"createdAt" | "host" | "enabled"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）
- 返回契约：DTO 层统一回显 `sectionName`、`parentGatewayName`、`parentGatewayNamespace`，并将数据库 `RequestHeaders / ResponseHeaders / URLRewrite / RequestRedirect` JSON 字符串解码为强类型对象返回（`#30 / #31` 响应同此契约，**剔除未实现预留列 `hostnameAliases`**），消除读写结构割裂。

**27. `network_check_gateway_domain`** — 检查候选域名在目标运行集群下是否可用、所需 CNAME 目标及实时 DNS 解析状态
- `project`: `string`，必填，项目 ID 或 `identifier`
- `host`: `string`，必填，待检查完整域名或子域名前缀
- `application`: `string`，可选，配合 `target` 定位运行集群
- `target`: `string`，可选，支持 `targetId | stage | name`（**`application + target` 与 `route` 至少传一组**，以定位所属集群与允许的域名后缀）
- `route`: `string`，可选，更新既有路由域名时传入 `routeId`（`gwr_xxx`）或唯一 `host`（多路径同名 `host` 返回 `409 ai.resource_ambiguous`），将自身当前域名识别为 `status: "current", available: true`
- `domainSuffix`: `string`，可选
- 实现契约说明：Service 层在现有 `CheckGatewayDomain`（`available, host, status`）基础上，复用 `gatewayCNAMETarget` 与 `dnsprovider.CheckCNAME` 补齐返回 `cnameName`、`cnameTarget` 与实时 `dnsStatus`。

**28. `network_get_topology`** — 查看项目服务依赖拓扑图，或查看单个应用的上下游子图与 K8s 实时资源拓扑
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，可选，传入时聚焦该应用；省略时返回整项目应用节点与依赖连线拓扑
- `stage`: `"dev" | "test" | "staging" | "prod"`，可选，按环境阶段过滤依赖拓扑
- `origins`: `array<"service_binding" | "manual">`，可选，按连线来源过滤
- `includeKubernetesResources`: `boolean`，可选，默认 `false`（仅传入 `application` 时有效，`true` 时额外聚合 `GetApplicationTopology` 的 Deployment/Pod/Service/HTTPRoute/PVC 实时资源树；查看网关域名列表请用 `network_list_gateway_routes`）

**29. `network_list_service_bindings`** — 分页列出项目内服务间调用依赖（`ServiceBinding`），支持按需实时探测 K8s Service/Endpoint 连通性
- `project`: `string`，必填，项目 ID 或 `identifier`
- `bindingId`: `string`，可选，纯 DB 精确过滤单条依赖绑定（不触发 K8s 实时探测，闭环 `#33 / #34` 超过 50 条时的写后校验）
- `checkBindingId`: `string`，可选，传入具体 `bindingId` 时实时探测目标 K8s Service 存在性、端口解析与就绪 Endpoint 状态（`CheckServiceBinding`）
- `sortBy`: `"createdAt" | "updatedAt" | "protocol" | "enabled"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）
- 实现契约说明：DTO 层必须通过 `model.DecodeSecretMap` 将数据库 `SecretMap` JSON 字符串解码为结构化数组 `credentialMap: array<{sourceEnvVar: string, targetSecretKey: string}>` 返回，严禁输出二次转义 JSON 字符串。

---

#### 3B.2 网关路由、服务依赖与拓扑连线写操作层（7 个）

**30. `network_create_gateway_route`** — 为已部署且非 `internalOnly` 的部署环境创建网关访问入口（`L2`，异步投递网关配置下发）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `application`: `string`，必填，应用 ID 或 `identifier`
- `target`: `string`，必填，支持 `targetId | stage | name`（后端校验 K8s Service 已存在且暴露对应端口，未首次发版前返回 `409 gateway_route.backend_service_missing` 指引先调 `delivery_deploy_target`）
- `host`: `string`，可选，省略时按 `<project>-<app>-<stage>.<clusterDomainSuffix>` 自动生成默认域名
- `domainSuffix`: `string`，可选，集群允许的网关域名后缀
- `path`: `string`，可选，默认 `"/"`
- `pathMatchType`: `"PathPrefix" | "Exact"`，可选，默认 `"PathPrefix"`（注：设为 `"Exact"` 需项目 Owner/Admin 权限）
- `servicePort`: `integer`，可选，省略默认取目标首个服务端口
- `tlsMode`: `"http-only" | "http-challenge" | "manual-cert"`，可选，默认 `"http-only"`（精确对齐 `normalizeTLSMode`，非法值直接按 `R11` 报错，严禁静默降级为 `"http-only"`）
- `sectionName`: `string`，可选，指定父 Gateway 监听器名称（如 `"web" | "websecure"`，具备 `gateway:manage` 的 Developer 及以上可设，对齐 `gateway_route_validation.go:250`）
- `parentGatewayName`: `string`，可选，自定义入口 Gateway 名称（对齐 `gateway_route_validation.go:25, 251` 与 `kube_specs.go:251`，设置自定义父网关需项目 Owner/Admin 权限）
- `parentGatewayNamespace`: `string`，可选，自定义入口 Gateway 所在命名空间（对齐 `gateway_route_validation.go:26, 252`，需项目 Owner/Admin 权限）
- `enabled`: `boolean`，可选，默认 `true`
- `isDefault`: `boolean`，可选，默认 `false`
- 备注：返回 `{ id: "gwr_xxx", routeId: "gwr_xxx", projectId: "prj_xxx", status: "progressing" }`（网关对账不走 `wait_for_task`，`family: "gateway_route"` 自动映射 `network_list_gateway_routes(project, route)` 回读确认）。

**31. `network_update_gateway_route`** — 纯增量修改网关访问入口配置与高级转发规则（`L2`，以数据库存量记录为基底合并，绝不清空未传字段）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `route`: `string`，必填，路由 ID（`gwr_xxx`）或唯一 `host`（多路径同名 `host` 返回 `409 ai.resource_ambiguous`）
- `application`: `string`，可选，支持同项目内跨应用蓝绿切换后端（省略默认取当前路由所属应用；**若切换了 `application` 必须同步传入新应用下的 `target`，否则按 `R11` 返回 `400 argument_invalid` 修正提示**）
- `target`: `string`，可选，切换指向目标应用下的 `targetId | stage | name`（若切换了 `target` 且未显式传 `servicePort`，当新目标不包含原 `existing.ServicePort` 时自动回退取新目标首个服务端口 `deploymentTargetServicePort(newTarget)`）
- `host`: `string`，可选
- `domainSuffix`: `string`，可选
- `path`: `string`，可选
- `pathMatchType`: `"PathPrefix" | "Exact"`，可选（高级配置需项目 Owner/Admin）
- `servicePort`: `integer`，可选
- `tlsMode`: `"http-only" | "http-challenge" | "manual-cert"`，可选
- `sectionName`: `string`，可选，指定父 Gateway 监听器名称（如 `"web" | "websecure"`，具备 `gateway:manage` 的 Developer 及以上可设，对齐 `gateway_route_validation.go:250`；清空用 `clearSectionName: true`）
- `clearSectionName`: `boolean`，可选
- `parentGatewayName`: `string`，可选，自定义入口 Gateway 名称（需项目 Owner/Admin；恢复集群默认父网关传 `clearParentGatewayName: true`）
- `clearParentGatewayName`: `boolean`，可选
- `parentGatewayNamespace`: `string`，可选，自定义入口 Gateway 命名空间（需项目 Owner/Admin；恢复集群默认网关命名空间传 `clearParentGatewayNamespace: true`）
- `clearParentGatewayNamespace`: `boolean`，可选
- `enabled`: `boolean`，可选
- `isDefault`: `boolean`，可选
- `backendWeight`: `integer`，可选，`>= 1`（`!= 1` 需项目 Owner/Admin）
- `upsertRequestHeaders`: `record<string, string>`，可选，增量设置注入请求头（需项目 Owner/Admin；`Authorization / Cookie / Host / Set-Cookie / X-Forwarded-* / X-Real-IP` 9 类特权头仅平台管理员可设，禁逐跳头）
- `removeRequestHeaderKeys`: `string[]`，可选，按 Header 名删除注入请求头
- `upsertResponseHeaders`: `record<string, string>`，可选，增量设置注入响应头（权限同上）
- `removeResponseHeaderKeys`: `string[]`，可选，按 Header 名删除注入响应头
- `urlRewrite`: `{hostname?: string, replacePrefixMatch?: string, replaceFullPath?: string}`，可选，强类型 HTTPRoute URL 重写规则（精确对齐 `provider/kubernetes/gateway_api.go:324` 顶层扁平键，`additionalProperties: false` 且 `minProperties: 1` 至少填一项；`replacePrefixMatch` 与 `replaceFullPath` 互斥；与 `requestRedirect` 互斥，需项目 Owner/Admin）
- `clearUrlRewrite`: `boolean`，可选，清除 URL 重写规则
- `requestRedirect`: `{scheme?: "http" | "https", hostname?: string, statusCode?: 301 | 302, replacePrefixMatch?: string, replaceFullPath?: string}`，可选，强类型请求重定向规则（精确对齐 `gateway_api.go:343` 顶层扁平键，`additionalProperties: false` 且 `minProperties: 1` 至少填一项防生成空 Filter；`replacePrefixMatch` 与 `replaceFullPath` 互斥；与 `urlRewrite` 互斥，需项目 Owner/Admin）
- `clearRequestRedirect`: `boolean`，可选，清除请求重定向规则
- 实现契约与防呆说明（遵守 `AGENTS.md §1/§10` 物理隐藏未实现死预留字段 `HostnameAliases`）：
  1) **单路由单域名说明**：单条网关路由仅承载一个生效 `host`（底层保留数据库存量 `existing.HostnameAliases` 原值不改、不暴露；如需为同一服务绑定额外备用域名如 `www`，说明书明确指路调用 `network_create_gateway_route` 新建独立路由，杜绝保存但集群未下发的空成功假象）；
  2) **同侧局部合并与跨侧互斥保护**：Service 层先加载存量 `GatewayRoute` 全部列作为基底；当存量已有 `urlRewrite`（或 `requestRedirect`）且本次继续传入同侧非空对象时，按存量同侧 JSON 对象合并非空子键（`replacePrefixMatch` 与 `replaceFullPath` 自动互清）；若存量设置了 `urlRewrite` 而本次传 `requestRedirect` 却未传 `clearUrlRewrite: true`（或反之），返回 `409 gateway_route.conflicting_filter` 指引显式传对侧 `clear*` 开关。

**32. `network_delete_gateway_route`** — 异步删除网关访问入口并回收集群路由资源（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `route`: `string`，必填，路由 ID（`gwr_xxx`）或唯一 `host`（返回 `{ id: "gwr_xxx", routeId: "gwr_xxx", projectId: "prj_xxx", deleteStatus: "deleting" }` JSON 摘要）

**33. `network_save_service_binding`** — 创建或纯增量修改项目内服务间调用依赖（`L2`，立即触发项目 NetworkPolicy 对账，需将项目 `ingressMode` 设为 `"dependencies"` 按依赖放行，并返回 `requiresRelease: true` 提示重新发布调用方注入环境变量）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `bindingId`: `string`，可选，省略为新建，传入 `bindingId` 为存量基底增量修改
- `sourceApplication`: `string`，可选，新建必填，调用方应用 ID 或 `identifier`（更新模式下若切换 `sourceApplication` 必须同步传入新应用下的 `sourceTarget`，否则返回 `400 argument_invalid` 修正提示）
- `sourceTarget`: `string`，可选，新建必填，调用方环境（`targetId | stage | name`）
- `targetApplication`: `string`，可选，新建必填，被调方应用 ID 或 `identifier`（更新模式下若切换 `targetApplication` 必须同步传入新应用下的 `targetTarget`，否则返回 `400 argument_invalid` 修正提示）
- `targetTarget`: `string`，可选，新建必填，被调方环境（`targetId | stage | name`）
- `targetPortName`: `string`，可选，被调服务端口名
- `targetPort`: `integer`，可选，被调服务端口号
- `protocol`: `"http" | "https" | "tcp"`，可选，新建必填（精确对齐 `dependency/service.go:309`，Redis/Postgres 等中间件传 `"tcp"`）
- `injectionMode`: `"url" | "host_port"`，可选，新建必填（`"url"` 仅支持 `"http" | "https"`）
- `urlEnvVar`: `string`，可选，`injectionMode="url"` 时必填（匹配大写 `^[A-Z_][A-Z0-9_]*$`，禁 `LUNA_* / LUNA_DEVOPS_* / KUBERNETES_*` 保留前缀）
- `path`: `string`，可选，`injectionMode="url"` 时可选（以 `/` 开头的纯路径）
- `clearPath`: `boolean`，可选，清空 `url` 拼接路径
- `hostEnvVar`: `string`，可选，`injectionMode="host_port"` 时必填（不可与 `portEnvVar` 相同）
- `portEnvVar`: `string`，可选，`injectionMode="host_port"` 时必填
- `upsertCredentialMap`: `array<{sourceEnvVar: string, targetSecretKey: string}>`，可选，按 `sourceEnvVar` 增量绑定跨服务 K8s Secret 键名引用（仅传键名，不接触密文；更新时未提及映射原样保留！）
- `removeCredentialEnvVars`: `string[]`，可选，按 `sourceEnvVar` 移除跨服务 Secret 键名引用
- `enabled`: `boolean`，可选，新建默认 `true`
- 实现与返回契约：
  1) **端口双写互斥自动重置（防 `CodePortNotFound` 死锁）**：因 `PrepareServiceBinding` 始终将解析后的 `(port.Name, port.Port)` 同时落库，更新模式下若仅传入 `targetPort`（省略 `targetPortName`）则自动清空存量 `targetPortName = ""` 按新端口号解析；若仅传入 `targetPortName`（省略 `targetPort`）则自动置存量 `targetPort = 0` 按新端口名解析；若切换了 `targetApplication` 或 `targetTarget` 且二者均未传则重置 `(targetPortName = "", targetPort = 0)` 取新目标默认首端口；
  2) **注入模式与协议互斥自动清理（通过 `validateServiceBindingShape`）**：有效 `injectionMode` 变为 `"host_port"` 时自动清空存量 `urlEnvVar` 与 `path`，变为 `"url"` 时自动清空存量 `hostEnvVar / portEnvVar`，有效 `protocol == "tcp"` 时自动清空存量 `path`；
  3) 响应统一经 `model.DecodeSecretMap` 解码结构化 `credentialMap` 数组，并回显权威 `projectId`、`requiresRelease: true` 及受影响需重新发版的 `sourceDeploymentTargetId`。

**34. `network_delete_service_binding`** — 删除服务间调用依赖并同步重算项目 NetworkPolicy（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `bindingId`: `string`，必填，依赖绑定 ID（返回 `{ id: "sbind_xxx", projectId: "prj_xxx", deleted: true, requiresRelease: true }`）

**35. `network_save_topology_edge`** — 创建或纯增量修改项目拓扑图自定义架构说明连线（非流量策略，仅拓扑图展示，`L1`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `edgeId`: `string`，可选，省略为新建，传入 `edgeId` 为存量基底增量修改
- `sourceApplication`: `string`，可选，新建必填（更新切换应用时若原存量绑定了旧 `sourceTarget` 且未同步传新 `sourceTarget` 或 `clearSourceTarget: true`，返回 `400 argument_invalid` 修正提示）
- `sourceTarget`: `string`，可选，支持 `targetId | stage | name`（清空用 `clearSourceTarget?: boolean`）
- `clearSourceTarget`: `boolean`，可选
- `targetApplication`: `string`，可选，新建必填（更新切换应用时若原存量绑定了旧 `targetTarget` 且未同步传新 `targetTarget` 或 `clearTargetTarget: true`，返回 `400 argument_invalid` 修正提示）
- `targetTarget`: `string`，可选，支持 `targetId | stage | name`（清空用 `clearTargetTarget?: boolean`）
- `clearTargetTarget`: `boolean`，可选
- `relationType`: `"depends_on" | "calls" | "reads_writes" | "publishes_to" | "consumes_from"`，可选，新建必填（精确对齐 `dependency/service.go:336`）
- `protocol`: `"http" | "https" | "tcp"`，可选（清空用 `clearProtocol?: boolean`）
- `clearProtocol`: `boolean`，可选
- `port`: `integer`，可选，`1..65535`（清空用 `clearPort?: boolean`）
- `clearPort`: `boolean`，可选
- `description`: `string`，可选（清空用 `clearDescription?: boolean`）
- `clearDescription`: `boolean`，可选

**36. `network_delete_topology_edge`** — 删除项目拓扑图自定义说明连线（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `edgeId`: `string`，必填（返回权威 `projectId` 与删除确认 JSON）

---

## 第 4 批：`workspace`（项目空间、成员协作、项目网络策略与跨项目共享）+ `ops`（工作台搜索、事件、账单用量、站内信、个人/项目通知与外部网页检索）

> 遵守 `notes/2026-11-19-ai-domain-tools-catalog.md` 第 0 节全局约定（双模定位与项目级响应统一回显权威 `projectId: "prj_xxx"` `R8`、零全量覆盖与三态防呆 `R5/R6/R7`、删除物理独立与防会话记忆穿透 `R3`、Secret `writeOnly + x-luna-sensitive`、实时状态 `no-store`）。
> 安全红线声明（同步对齐并加固 `internal/aitool/openapi_catalog.go` `agentDisabledOperations`）：登录/登出/注册、修改密码（`updateMyPassword`）、签发个人访问令牌（`createAccessToken`）、**创建与修改第三方 OAuth 应用（`createOAuthApplication` 响应含一次性明文 `clientSecret`，`updateOAuthApplication` 可篡改 `redirectUris` 劫持回调且全量覆盖 `allowedScopes`，二者均必须列入禁用名单！）**、轮换 OAuth 密钥（`rotateOAuthApplicationSecret`）及内部计费流量上报接口**严禁暴露为 AI 工具**。AI 仅暴露安全的凭据/应用/授权只读列表与吊销/解绑工具。

---

### 4A. `workspace`（项目空间、成员协作、项目网络隔离与跨项目共享域，共 17 个工具）

#### 4A.1 只读发现与诊断层（5 个，固定 `L0`）

**1. `workspace_list_projects`** — 分页列出可访问的项目空间及个人收藏状态
- `visibility`: `"related" | "all"`，可选，默认 `"related"`（对齐 `project/list_scope.go:20`：默认仅返回本人参与的项目；`"all"` 仅允许 `platform_admin` 在全局会话 `conversationProjectID == ""` 下传入，非管理员传 `"all"` 返回 `403 auth.forbidden`，**项目绑定会话下显式传 `"all"` 统一返回 `403 ai.project_scope_mismatch`**，省略时自动收敛至当前绑定项目）
- `search`: `string`，可选，按项目名称或 `identifier` 搜索
- `identifier`: `string`，可选，精确匹配 `identifier`
- `pinnedOnly`: `boolean`，可选，默认 `false`，仅列出当前用户已收藏置顶的项目
- `sortBy`: `"createdAt" | "updatedAt" | "lastUsedAt" | "useCount" | "dashboardOrder" | "name" | "identifier"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）
- 实现契约说明：Service 层将左连接修正为 `LEFT JOIN project_pins ON project_pins.project_id = projects.id AND project_pins.user_id = currentUser.ID` 统一回显每个项目的 `pinned: boolean`（全局会话 `visibility="related"` 且 `pinnedOnly=true` 时同步覆盖 `platform_admin` 本人收藏的非入项项目）；当处于项目绑定会话（`conversationProjectID != ""`）时经 `AuthorizeProjectByID(conversationProjectID, ActionProjectRead)` 鉴权后直接按 `projects.id = conversationProjectID` 收敛（不叠加 `project_members.user_id` 过滤，避免非成员 `platform_admin` 在项目会话拿到空列表）；同时修复 `ListProjects` 排序白名单（`project_handlers.go:57-59`，将 `"createdAt"` 修正映射到 `"projects.created_at"` 并放行 `"updatedAt" / "lastUsedAt" / "useCount" / "dashboardOrder"` 至 `projectListOrderClause`——其中 `"dashboardOrder"` 排序子句必须写作 `CASE WHEN COALESCE(project_members.dashboard_order, 0) > 0 THEN 0 ELSE 1 END ASC, project_members.dashboard_order <order>, projects.created_at DESC, projects.id ASC`，将已排入前 8 看板的正整数 `1..8` 排在默认 `0` 之前，确保用户参与项目超过 50 个时 `#9` `project_order` 写后校验 `sortBy="dashboardOrder"` 100% 在第 1 页命中有序前排项目）。

**2. `workspace_get_project`** — 查看单个项目空间配置、资源计数概览、当前用户角色权限及近 7 天交付/网关诊断摘要
- `project`: `string`，必填，项目 ID 或 `identifier`
- `includeOverview`: `boolean`，可选，默认 `true`，附带应用数、活跃/失败发布数、网关入口状态及部署目标数、成员数、代码仓库绑定计数概览
- `includeDiagnostics`: `boolean`，可选，默认 `false`，按需执行近 7 天活跃/失败构建、近 7 天活跃/失败发布及异常网关入口/TLS 证书巡检（需调用者具备 `ActionBuildRead + ActionDeploymentRead + ActionGatewayRead` 权限；**说明书明确边界**：不含 K8s Pod 实时运行健康度，排查容器 CrashLoop/OOM 请调 `delivery_list_applications` 或 `runtime_get_target_diagnostics`）
- 实现契约说明：AI 工具层读取项目详情时**严禁调用 `recordProjectUsage` 写库**，避免巡检或自动校验污染用户工作台“最近使用/最常使用”排序；在现有 `GetProjectOverview` 基础上补充 `deploymentTargetsCount / membersCount / repositoryBindingsCount` 计数 SQL 与当前用户 `pinned: boolean`（闭环 `#8 workspace_pin_project` 确定性单项目写后校验），当调用者为项目 `owner` 或当前 `billingOwner` 时一并回显只读 `pendingBillingTransfer?: { requestId, recipientUserId, status, expiresAt, createdAt }`（闭环 `#10 workspace_request_billing_transfer` 发起人视角写后校验），保持实时网关观测 `Cache-Control: no-store`，统一回显权威 `projectId`。

**3. `workspace_list_members`** — 分页查看项目成员列表，或（项目 Owner/Admin）搜索未入项的候选平台用户
- `project`: `string`，必填，项目 ID 或 `identifier`
- `member`: `string`，可选，按成员记录 ID、用户 ID 或唯一邮箱精确过滤单条成员（闭环项目成员超过 50 人时 `#12 / #13` 的确定性写后回读校验）
- `search`: `string`，可选，按现有成员姓名、邮箱或角色搜索
- `candidateSearch`: `string`，可选，仅项目 `owner / admin` 可传（`ActionProjectManage`），搜索尚未加入本项目的平台活跃用户候选列表（上限 `50` 条）
- `sortBy`: `"createdAt" | "name" | "email" | "role"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"asc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**4. `workspace_get_network_policy`** — 查看项目空间入站/出站网络隔离模式、出站 CIDR 白名单及平台授权的只读出站例外
- `project`: `string`，必填，项目 ID 或 `identifier`

**5. `workspace_list_network_shares`** — 分页列出本项目对外提供（`provided`）或外部授权给本项目使用（`consumed`）的跨项目内网服务共享
- `project`: `string`，必填，项目 ID 或 `identifier`
- `direction`: `"provided" | "consumed"`，必填，明确共享方向
- `shareId`: `string`，可选，按跨项目共享记录 ID 精确过滤单条记录（闭环超过 50 条时 `#15 / #16 / #17` 写后校验）
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

---

#### 4A.2 项目空间、成员与网络治理写操作层（12 个）

**6. `workspace_create_project`** — 创建新项目空间（`L1`，仅限全局会话 `conversationProjectID == ""` 调用）
- `identifier`: `string`，必填，项目唯一英文标识（创建后不可修改，映射 K8s Namespace）
- `name`: `string`，必填，项目显示名称
- `description`: `string`，可选
- `maxConcurrentBuilds`: `integer`，可选
- `webConsoleEnabled`: `boolean`，可选
- 会话沙箱契约：在单项目绑定会话（`conversationProjectID != ""`）中落库前直接拦截返回 `403 ai.project_scope_mismatch` 并按 `R11` 指引切换全局会话创建新项目（防止逃逸当前项目沙箱及写后回读校验失败）。

**7. `workspace_update_project`** — 增量修改项目名称、描述、构建并发上限或项目级 Web 控制台安全天花板开关（固定 `L2`，防止会话记忆提权穿透开启生产 Web 控制台，需 `ActionProjectManage`）
- `project`: `string`，必填，项目 ID 或 `identifier`（物理隔离不可修改的 `identifier`）
- `name`: `string`，可选
- `description`: `string`，可选（空串或省略不改；清空描述传 `clearDescription: true`）
- `clearDescription`: `boolean`，可选
- `maxConcurrentBuilds`: `integer`，可选
- `webConsoleEnabled`: `boolean`，可选，项目级容器远程执行/终端总开关（关闭时下属全部环境强制禁止 Exec/Terminal）

**8. `workspace_pin_project`** — 切换当前用户对单个项目的个人收藏置顶状态（`L1`，单项目级视图偏好，回显权威 `projectId`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `pinned`: `boolean`，必填，`true` 收藏置顶，`false` 取消收藏

**9. `workspace_reorder_my_dashboard_projects`** — 调整当前用户个人工作台看板前排项目展示顺序（`L1`，跨项目个人视图偏好，仅限全局会话 `conversationProjectID == ""` 调用）
- `projects`: `string[]`，必填，有序项目 ID 或 `identifier` 非空数组（`1..8` 个，校验当前用户为各项真实 `project_members` 成员，同一事务内清理被移出前 8 项目的残留 `dashboard_order`；项目绑定会话下调用返回 `403 ai.project_scope_mismatch` 指引切换全局会话；**响应统一回显解析后的权威主键数组 `projectIds: string[]`** 以对齐 `outcome.ts` `project_order` 写后校验）

**10. `workspace_request_billing_transfer`** — 发起项目计费归属人（Billing Owner）变更确认申请（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `targetUser`: `string`，必填，目标项目成员的用户 ID 或邮箱
- 备注：发起人必须同时是当前项目 `billingOwner` 且为项目 `owner` 成员；目标用户必须已是本项目成员（未入项时按 `R11` 指引先调 `workspace_save_member`），且同项目无未决转移申请。

**11. `workspace_delete_project`** — 异步销毁整个项目空间及其名下全部应用、环境、路由与托管资源（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`（平台系统项目返回 `403 project.system_protected`；执行后返回 `{ id: "prj_xxx", projectId: "prj_xxx", deleteStatus: "deleting" }`）

**12. `workspace_save_member`** — 向项目添加成员或调整现有成员的项目角色（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `user`: `string`，必填，用户 ID、成员记录 ID 或唯一邮箱
- `role`: `"owner" | "admin" | "developer" | "viewer"`，必填，严格枚举校验（严禁对未知字符串静默降级为 `"viewer"`！）
- 备注：授予或降级 `"owner"` 角色必须由当前项目的真实 `"owner"` 成员操作（平台管理员若非该项目 `"owner"` 成员同样不可越权），且项目至少保留 1 名 `"owner"`。

**13. `workspace_remove_member`** — 将成员移出项目空间（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `member`: `string`，必填，成员记录 ID、用户 ID 或唯一邮箱（禁止移除当前操作者自身或最后一名 `"owner"`；返回 `{ id, projectId, deleted: true }`）

**14. `workspace_configure_network_policy`** — 纯增量修改项目空间入站/出站网络策略与公网出站 CIDR 白名单（`L2`，立即触发项目 NetworkPolicy 异步对账）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `ingressMode`: `"isolated" | "dependencies"`，可选
- `egressMode`: `"internet" | "allowlist" | "none"`，可选
- `upsertAllowlistEntries`: `array<{cidr: string, ports?: integer[], allPorts?: boolean}>`，可选，按规范化 CIDR 新增或更新公网出站白名单规则（全项目上限 `64` 条 CIDR、单条上限 `16` 个端口 `1..65535`，`ports` 与 `allPorts: true` 互斥；**`R7` 防扩权契约**：新增 CIDR 时省略 `ports` 表示放行全部端口，命中已存在规范化 CIDR 时省略 `ports` 严格保留原端口限制不变，仅在显式传 `allPorts: true` 时放宽为全部端口；严禁 `/0` 与私网/保留地址，私网访问需由集群维护者通过 `runtime_save_cluster_egress_exception` 授权）
- `removeAllowlistCIDRs`: `string[]`，可选，按 CIDR 移除出站白名单规则
- `clearAllowlist`: `boolean`，可选，清空全部出站白名单规则
- 备注：若更新了白名单但当前有效 `egressMode != "allowlist"`，响应附带 `warnings` 明确提示白名单仅在 `egressMode="allowlist"` 下生效。

**15. `workspace_create_network_share`** — 将本项目的某个部署环境在内网共享给指定消费方项目空间（`L2`）
- `project`: `string`，必填，提供方项目 ID 或 `identifier`
- `application`: `string`，可选，配合 `stage / name` 定位部署环境
- `target`: `string`，必填，支持 `targetId | stage | name`
- `consumerProjectIdentifier`: `string`，必填，消费方项目唯一 `identifier`（需消费方调用 `workspace_accept_network_share` 确认生效；若提供方持有集群出站例外，还需集群维护者调用 `runtime_approve_cluster_network_share` 批准）

**16. `workspace_accept_network_share`** — 作为消费方项目管理员确认接受跨项目内网服务共享（`L2`）
- `project`: `string`，必填，消费方项目 ID 或 `identifier`
- `shareId`: `string`，必填，跨项目共享记录 ID

**17. `workspace_delete_network_share`** — 提供方撤销或消费方退出跨项目内网服务共享并重算双方 NetworkPolicy（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `shareId`: `string`，必填，共享记录 ID（返回 `{ id: shareId, projectId, direction: "provided" | "consumed", deleted: true }`，按 `share.ProviderProjectID == projectId` 派生 `direction` 以供 `#5 workspace_list_network_shares` 精确校验 `expect: "absent"`）

---

### 4B. `ops`（全局检索、平台事件、个人账单、站内信、个人/项目通知与外部网页检索域，共 25 个工具）

> 架构防呆隔离：源码中个人通知（DB `scope="user"`, 仅允许 6 种固定预设 Webhook + 12 种默认失败事件开关，无 `project` 参数，允许在全局或项目会话随时调用）与项目通知（DB `scope="project"`, 支持自定义 Webhook/SMTP 渠道、跨项目共享绑定、规则级 `filter` 与 `channelTargets`，需 `project` 作用域与项目 `owner` 权限）底层安全模型与会话沙箱校验完全异构。因此在只读、配置与测试/重试全链路将**个人通知**与**项目通知**彻底物理拆清，杜绝参数幻觉、多项目绑定误清空与项目会话 `403 ai.project_scope_mismatch` 死锁！

#### 4B.1 全局检索、事件、账单、站内信、通知与账户安全只读层（10 个，固定 `L0`）

**18. `ops_search_global`** — 全局跨项目快速搜索资源或读取工作台 Dashboard 概览
- `query`: `string`，可选，`1..160` 字符关键词（省略时仅返回 Dashboard 概览统计与实时集群/制品库状态）
- `visibility`: `"related" | "all"`，可选，默认 `"related"`（`"all"` 仅 `platform_admin` 在全局会话下可传，非管理员传 `"all"` 报 `403 auth.forbidden`，项目绑定会话显式传 `"all"` 报 `403 ai.project_scope_mismatch`）
- `types`: `array<"project" | "application" | "deployment_target" | "gateway_route" | "repository_binding" | "project_volume" | "runtime_config_set" | "build_variable_set" | "notification_channel" | "runtime_cluster" | "artifact_registry" | "build_run" | "release" | "user">`，可选，精确对齐 `search_handlers.go` 14 类资源枚举（显式包含 `"build_run"` 或 `"release"` 时跨项目搜构建/发布；`"user"` 仅对 `platform_admin` 在全局会话生效）
- `limitPerType`: `integer`，可选，每类资源返回条数上限，默认 `6`（上限 `20`，防止 `14 × 20` 撑爆上下文）
- `includeDashboard`: `boolean`，可选，传 `query` 时是否同时附带 Dashboard 概览，默认 `false`
- 会话沙箱契约：在项目绑定会话（`conversationProjectID != ""`）中，搜索分支与 Dashboard 分支（`GetDashboard`）均强制收敛 `VisibleProjectIDs = [conversationProjectID]`，且 Dashboard 的 `attention / activities` 事件聚合严格限定 `project_id = conversationProjectID`（严禁通过 `actor_id = user.ID` 混入当前用户在其他项目触发的事件或无关集群/制品库状态）。

**19. `ops_list_events`** — 分页查询可见范围内的平台事件流，支持精确查看单条事件详情及事件类型目录
- `visibility`: `"related" | "all"`，可选，默认 `"related"`（`"all"` 仅 `platform_admin` 在全局会话下可传，项目绑定会话显式传 `"all"` 报 `403 ai.project_scope_mismatch`）
- `eventId`: `string`，可选，精确过滤单条事件 ID 并附带完整 `detail`
- `project`: `string`，可选，项目 ID 或 `identifier`（项目绑定会话省略时自动注入 `conversationProjectID`；**当 `effectiveProjectID != ""` 时统一先经 `AuthorizeProjectByID(effectiveProjectID, ActionProjectRead)` 鉴权——既拦截非成员/已移出前成员探测与残留读取，又放行非成员 `platform_admin`——鉴权通过后直接按 `platform_events.project_id = effectiveProjectID` 查询而不再叠加仅查 `project_members` 的 `platformEventsVisibleTo`**；当传入 `eventId` 时校验 `canReadPlatformEvent` 且在 `effectiveProjectID != ""` 时强制 `event.ProjectID == effectiveProjectID`）
- `application`: `string`，可选，应用 ID 或 `identifier`
- `target`: `string`，可选，支持 `targetId | stage | name`
- `category`: `string`，可选
- `type`: `string`，可选
- `severity`: `"info" | "warning" | "error"`，可选（精确对齐 `platformevent/catalog.go` 三级严重度）
- `status`: `"in_progress" | "succeeded" | "failed" | "canceled"`，可选
- `dateFrom`: `string`，可选，支持 `RFC3339` 或 `YYYY-MM-DD`
- `dateTo`: `string`，可选，支持 `RFC3339` 或 `YYYY-MM-DD`
- `search`: `string`，可选
- `includeCatalog`: `boolean`，可选，默认 `false`，附带标准事件类型目录
- `sortBy`: `"occurredAt" | "createdAt" | "severity" | "type" | "category"`，可选，默认 `"occurredAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**20. `ops_get_billing_summary`** — 查询**当前登录用户本人**的钱包余额、今日/本月消费汇总、按应用/环境的消费排行及平台计费单价规则
- `project`: `string`，可选，限定查看本人作为 `billingOwner` 的特定项目费用归属（注：`AGENTS.md §8` 规定 `ai.*` 费用 `project_id` 恒为空，查个人总余额或 AI 费用请勿传 `project`）
- `periodStart`: `string`，可选，RFC3339 起始时间（与 `periodEnd` 成对传入，半开区间 `[periodStart, periodEnd)`）
- `periodEnd`: `string`，可选，RFC3339 结束时间
- `timeZone`: `string`，可选，IANA 时区名（如 `"Asia/Shanghai"`，默认 `"UTC"`，决定今日/本月自然边界）
- `includeDeploymentSpend`: `boolean`，可选，默认 `true`，返回按应用/环境的构建、运行、存储、网关分项费用排行
- `includeRateRules`: `boolean`，可选，默认 `false`，附带平台计费单价表
- 实现与会话沙箱契约：固定锁定当前登录用户本人视角（`userIDs = [currentUser.ID]` 且强制 `PlatformTotal = false` 固定个人 `account` 余额口径），同时聚合返回只读 `gatewayTrafficStatus`（对齐 `GET /api/v1/billing/gateway-traffic-status` `gateway_traffic_handlers.go:35-110` 与前端账单页，回显网关流量计量探针 `available / status / observationCode` 鲜活度摘要，不暴露底层集群内部细节）；**当显式传入 `project` 时，全角色（含 `platform_admin`）统一强制校验 `project.BillingOwnerUserID == currentUser.ID`**——不匹配时统一返回 `403 billing.project_forbidden`（若调用者为 `platform_admin` 按 `R11` 指引改调 `admin_inspect_billing`，严禁静默返回全 `0` 误导模型！）；在项目绑定会话（`conversationProjectID != ""`）中网关允许省略 `project` 查询个人总余额与 `ai.*` 消费，仅显式传 `project` 时校验 `resolvedProjectID == conversationProjectID`。

**21. `ops_list_billing_records`** — 分页查询当前登录用户本人的账单流水（`ledger`）或资源计量明细（`usage`）
- `recordType`: `"ledger" | "usage"`，必填
- `project`: `string`，可选，全角色（含 `platform_admin`）统一限定本人作为 `billingOwner` 的项目可查（不匹配返回 `403 billing.project_forbidden`，管理员查他人项目按 `R11` 指引改调 `admin_inspect_billing`；项目绑定会话允许省略 `project` 查本人全账户流水与 `ai.*` 明细；若同时传 `project` 与 `ai.*` `meter` 直接报错提示 `ai.*` 费用不归属项目）
- `meter`: `"build.cpu_vcpu_minute" | "build.memory_gib_minute" | "build.job" | "runtime.cpu_vcpu_hour" | "runtime.memory_gib_hour" | "storage.gib_day" | "storage.transfer_gib" | "gateway.egress_gib" | "gateway.requests_1000" | "ai.prompt_tokens_1m" | "ai.completion_tokens_1m" | "ai.cached_prompt_tokens_1m" | "ai.cache_write_prompt_tokens_1m"`，可选，`recordType="usage"` 时按真实计量项枚举过滤
- `ledgerType`: `"debit" | "credit" | "adjustment"`，可选，`recordType="ledger"` 时按流水类型过滤
- `periodStart`: `string`，可选，带时区偏移的 RFC3339（与 `periodEnd` 成对传入，半开区间 `[periodStart, periodEnd)`）
- `periodEnd`: `string`，可选，带时区偏移的 RFC3339
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**22. `ops_list_inbox`** — 查看当前用户的站内信列表、未读总数及单条待办请求（ActionRequest）的乐观锁版本 `rowVersion`
- `filter`: `"all" | "unread" | "action"`，可选，默认 `"all"`
- `category`: `"action" | "project" | "billing" | "security" | "delivery" | "system"`，可选
- `messageId`: `string`，可选，精确读取单条未归档站内信详情及 `actionRequest.rowVersion`（**实现契约**：修复 `internal/inbox/query.go:89` `Service.Get` 漏过滤归档态的缺陷，统一加上 `AND archived_at IS NULL` 与列表查询对齐——已归档消息单查返回 `404`，确保 `#29 ops_archive_inbox_message` 写后校验 `expect: "absent"` 精确闭环）
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**23. `ops_get_my_notifications`** — 查看当前用户个人的通知偏好开关、6 种可用个人机器人预设、已配置个人通知渠道及最近投递记录
- `includePresets`: `boolean`，可选，默认 `false`，返回 `PersonalWebhookPresets()` 6 类个人机器人预设及所需 Secret 键说明
- `includeChannels`: `boolean`，可选，默认 `true`（仅回显 `secretSet` 布尔标记，绝不回显令牌/签名密钥）
- `includePreferences`: `boolean`，可选，默认 `true`，返回 `emailEnabled` 与已订阅的失败事件类型
- `includeDeliveries`: `boolean`，可选，默认 `false`，返回最近个人通知投递记录（上限 `20` 条）

**24. `ops_list_project_notifications`** — 查看指定项目空间的通知预设、已绑定/可绑定通知渠道、项目通知规则或最近投递记录（全局会话下亦支持跨项目盘点可管理的 `scope="project"` 渠道）
- `project`: `string`，可选，项目 ID 或 `identifier`（`overview / rules / deliveries / presets` 必填；项目绑定会话省略时统一自动注入 `conversationProjectID`；仅在**全局会话（`conversationProjectID == ""`）且 `section == "channels"`** 时允许省略 `project`，对齐 `GET /api/v1/notification-channels` 列出当前用户可管理的全部跨项目 `scope="project"` 渠道）
- `section`: `"overview" | "channels" | "rules" | "deliveries" | "presets"`，可选，默认 `"overview"`（`"overview"` 返回渠道与规则摘要；单选子视图时支持精确分页）
- `includeBindableChannels`: `boolean`，可选，默认 `false`，仅在全局会话（`conversationProjectID == ""`）传入 `project` 且 `section == "channels"` 时有效，额外返回当前用户在其他项目拥有 `ActionNotificationManage` 可通过 `#35` 并集复用的候选 `scope="project"` 渠道（项目绑定会话显式传 `true` 返回 `403 ai.project_scope_mismatch`）
- `channel`: `string`，可选，`channels / deliveries` 按渠道 ID 或名称精确过滤（闭环超过 50 条时 `#35 / #36 / #37` 写后校验）
- `rule`: `string`，可选，`rules / deliveries` 按规则 ID 或名称精确过滤（闭环超过 50 条时 `#38 / #39` 写后校验）
- `status`: `string`，可选，`deliveries` 按投递状态（如 `"pending" | "sending" | "succeeded" | "failed" | "enqueue_failed"`）筛选
- `eventType`: `string`，可选，`deliveries` 按事件类型筛选
- `eventId`: `string`，可选，`deliveries` 按关联平台事件 ID 筛选
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**25. `ops_get_my_account_security`** — 查看当前用户个人资料（含 `passwordSet: boolean`）、外部 OIDC 绑定身份、个人访问令牌（PAT）摘要、已授权第三方应用（Grants）及本人创建的 OAuth 应用列表
- `includeTokens`: `boolean`，可选，默认 `true`（绝不回显明文 Token）
- `includeExternalIdentities`: `boolean`，可选，默认 `true`
- `includeOAuthGrants`: `boolean`，可选，默认 `true`
- `includeOwnedOAuthApps`: `boolean`，可选，默认 `true`（对应 `ListOAuthApplications`，不回显密钥）
- 代管会话（Impersonation）隔离降级契约：当处于管理员代管会话（`session.ImpersonatorID != ""`）或非主 Cookie 会话时，Service 层自动跳过 `oauthGrants / ownedOAuthApps` 子查询并返回 `oauthSectionsRestricted: true`（保持 `200 OK` 正常返回个人资料、PAT 与外部身份，既严防代管会话越权读取第三方 OAuth 授权，又保障 `#41 ops_update_my_profile` 写后权威回读畅通）。

**26. `ops_web_search`** — 通过受控代理搜索互联网公开技术资料（`L0`）
- `query`: `string`，必填，`1..300` 字符（自动拦截含凭据 URL/敏感词查询）
- `limit`: `integer`，可选，默认 `5`（范围 `1..10`，精确对齐 `internal/aitool/web.go:36`；返回体固定附带 `contentTrust: "untrusted_external"`）

**27. `ops_web_fetch`** — 通过受控 SSRF 防护代理抓取公开网页正文并按字符数截断（`L0`）
- `url`: `string`，必填，公开 `http(s)://` URL（禁止内网、元数据地址与内嵌凭据）
- `maxCharacters`: `integer`，可选，默认 `20000`（范围 `1..50000`，按 Unicode 字符数截断，精确对齐 `internal/aitool/web.go:91`；返回 `truncated: boolean` 与 `contentTrust: "untrusted_external"`）

---

#### 4B.2 站内信、个人通知、项目通知与个人账户安全写操作层（15 个）

**28. `ops_mark_inbox_read`** — 幂等标记单条或全部站内信为已读（`L1`，不含不可逆归档）
- `markAll`: `boolean`，可选，设为 `true` 时标记全部未读站内信为已读
- `messageId`: `string`，可选，`markAll` 未设时必填，标记指定站内信已读（返回结构化 `{ markedAll, messageId?, unreadCount }` 替代空 `204`）

**29. `ops_archive_inbox_message`** — 归档隐藏单条站内信（`L2`，因归档后不可恢复取消，物理独立防误触）
- `messageId`: `string`，必填，站内信 ID（返回 `{ id: messageId, archived: true }`，写后回读校验 `expect: "absent"`）

**30. `ops_decide_inbox_action`** — 接受或拒绝站内信中的待办确认请求（如项目计费归属转移，`L2`）
- `requestId`: `string`，必填，ActionRequest ID
- `decision`: `"accept" | "reject"`，必填
- `expectedVersion`: `integer`，必填，`>= 1`，乐观锁版本号 `rowVersion`（从 `ops_list_inbox` 获取；响应回显 `requestId`、关联 `messageId` 与终态 `status`，写后校验绑定 `ops_list_inbox(filter: "all", messageId)` 避免决策后移出 `filter="action"` 假失败）

**31. `ops_save_my_notification_channel`** — 创建或增量修改个人机器人通知渠道（`L2`，单用户上限 10 个，仅支持 6 种个人预设，Secret 只进不出）
- `channel`: `string`，可选，省略为新建，传入个人渠道 ID 或唯一名称为增量修改
- `name`: `string`，可选，新建必填
- `presetId`: `"feishu-bot" | "lark-bot" | "wecom-bot" | "dingtalk-bot" | "slack-webhook" | "discord-webhook"`，可选，**新建必填，创建后不可修改**
- `upsertSecrets`: `record<string, string>`，可选，按预设白名单写入机器人令牌/签名密钥（在 `additionalProperties` 值 Schema 上声明 `writeOnly: true, x-luna-sensitive: true`，生成 `upsertSecrets.*` 脱敏路径，未提及键原样保留）
- `clearSecretKeys`: `string[]`，可选，显式清除可选密钥（如取消 `SignSecret`）
- `enabled`: `boolean`，可选，新建默认 `true`

**32. `ops_delete_my_notification_channel`** — 删除个人机器人通知渠道（`L2`）
- `channel`: `string`，必填，个人渠道 ID 或唯一名称（返回 `{ id, deleted: true }`）

**33. `ops_configure_my_notification_preferences`** — 增量修改个人邮件通知开关与订阅的失败事件类型（`L1`）
- `emailEnabled`: `boolean`，可选
- `addEventTypes`: `array<"build.failed" | "build.webhook_rejected" | "release.failed" | "runtime.pod_oom_killed" | "runtime.pod_crash_loop" | "runtime.pod_start_failed" | "runtime.workload_degraded" | "runtime.recovered" | "hook.failed" | "gateway.apply_failed" | "certificate.failed" | "certificate.expired">`，可选，增量开启订阅事件（精确锁死 12 类个人失败事件枚举）
- `removeEventTypes`: `string[]`，可选，增量取消订阅事件

**34. `ops_test_or_retry_my_notification`** — 发送个人机器人渠道测试消息或重试失败的个人通知投递（固定 `L2`，无 `project` 参数，全局与项目会话均可直接调用）
- `channel`: `string`，可选，测试指定个人通知渠道 ID 或名称（与 `retryDeliveryId` 严格互斥二选一，受 `10/min` 速率限制保护）
- `retryDeliveryId`: `string`，可选，重试归属当前用户（`recipient_user_id == currentUser.ID`）的失败个人通知投递记录 ID（Service 层仅放行 `delivery.Status` 为 `"failed" | "enqueue_failed"` 或已超时的 `"sending"`；对 `"succeeded" | "pending" | "retry_pending"` 及未超时的 `"sending"` 统一返回 `409 notification.delivery_not_retryable` 杜绝重复外呼轰炸）

**35. `ops_save_project_notification_channel`** — 创建或纯增量修改项目作用域通知渠道，或将已有可管理共享渠道并集绑定到当前项目（`L2`，需项目 `owner` 权限 `notification:manage`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `channel`: `string`，可选，省略为新建，传入渠道 ID 或唯一名称为增量更新
- `name`: `string`，可选，新建必填
- `presetId`: `"feishu-bot" | "lark-bot" | "wecom-bot" | "gotify" | "dingtalk-bot" | "slack-webhook" | "discord-webhook"`，可选，精确对齐 `notification/presets.go` 7 类预设枚举（自定义 Webhook 或 SMTP 渠道省略 `presetId`；**更新已有渠道时 `presetId / adapterKind` 不可跨类型变更，且预设渠道禁止混传 `upsertConfig / removeConfigKeys`**）
- `adapterKind`: `"webhook" | "smtp"`，可选，精确对齐后端注册表枚举（默认 `"webhook"`）
- `upsertConfig`: `record<string, any>`，可选，增量合并自定义渠道非敏感配置（如 Webhook URL/Headers、SMTP Host/Port/From；鉴权头须采用模板占位如 `headers: {"Authorization": "Bearer {{.Secrets.BearerToken}}"}` 配合 `upsertSecrets`——**Service 层严禁传入 `password`、未走 `{{.Secrets.*}}` 占位的敏感 Header（`Authorization / Cookie / *Token* / *Key* / *Secret*`）或内嵌 `url.User` 凭据的 URL，误传时按 `R11` 报错引导改走 `upsertSecrets`**）
- `removeConfigKeys`: `string[]`，可选，移除指定可选配置键
- `upsertSecrets`: `record<string, string>`，可选，增量写入签名密钥、Bearer Token 或 SMTP 密码（在 `additionalProperties` 值 Schema 上声明 `writeOnly: true, x-luna-sensitive: true`，生成 `upsertSecrets.*` 脱敏路径）
- `clearSecretKeys`: `string[]`，可选，显式清除指定密钥键（清除 SMTP `"password"` 时同步清理 `config_json` 中的残留 `passwordRef`）
- `enabled`: `boolean`，可选，新建默认 `true`
- 实现与沙箱安全契约：
  1) **严格锁死 `scope == "project"`**：Service 层强制校验 `channel.Scope == "project" && channel.OwnerUserID == ""`（对 `#24` 中可见的 `global` 全局渠道返回 `400 notification.channel_scope_not_project` 防止误改全局渠道）；
  2) **并集保留与多项目共享渠道沙箱保护**：新建绑定 `[projectId]`；操作已有项目渠道时执行 `existingProjectIDs ∪ {projectId}` 并校验并集全部项目具备 `ActionNotificationManage`。当处于单项目绑定会话（`conversationProjectID != ""`）且 `existingProjectIDs` 包含 `conversationProjectID` 之外的其他项目时，仅允许纯并集引入绑定（不携带 `name / upsertConfig / removeConfigKeys / upsertSecrets / clearSecretKeys / enabled` 变更），若尝试修改多项目共享渠道配置则返回 `403 ai.project_scope_mismatch` 指引在全局会话修改！

**36. `ops_unbind_project_notification_channel`** — 将跨多项目共享的通知渠道从当前项目解绑（不删除渠道本身，`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `channel`: `string`，必填，渠道 ID 或名称
- 备注：若该渠道仅绑定了当前 1 个项目，后端拒绝解绑并返回 `409 notification.project_required`，按 `R11` 指引改调 `ops_delete_project_notification_channel`；解绑成功统一返回结构化 `{ id, projectId, unbound: true }`（替代空 `204`，闭环 `#24(section: "channels")` 写后校验）。

**37. `ops_delete_project_notification_channel`** — 彻底删除单项目独占的通知渠道（`L2`，仍绑定其他项目或被通知规则引用时阻断删除）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `channel`: `string`，必填，渠道 ID 或名称
- 实现契约说明：在加锁事务内**全会话统一**强制校验 `channel.Scope == "project" && channel.OwnerUserID == ""` 且 `projectId ∈ existingProjectIDs`（未绑定当前 `project` 返回 `404` 严防跨项误删）；若 `len(existingProjectIDs) > 1`（仍共享绑定其他项目），项目会话返回 `403 ai.project_scope_mismatch`、全局会话返回 `409 notification.channel_shared_multiple_projects`，统一按 `R11` 指引先调 `ops_unbind_project_notification_channel` 解绑其他项目直至单项目独占后再删；若渠道仍被规则引用（`channelReferencedByAnyRule`）返回 `409` 拦截（他项无权限引用仅回显 `externalReferencedRuleCount`）；删除成功返回 `{ id, projectId, deleted: true }`。

**38. `ops_save_project_notification_rule`** — 创建或纯增量更新项目通知订阅规则（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `rule`: `string`，可选，省略为新建，传入规则 ID 或同项目唯一名称为增量更新
- `name`: `string`，可选，新建必填
- `addEventTypes`: `string[]`，可选，新增订阅的事件类型（新建可直接传入）
- `removeEventTypes`: `string[]`，可选，移除已订阅的事件类型
- `addChannels`: `string[]`，可选，新增投递渠道 ID 或名称（新建可直接传入）
- `removeChannels`: `string[]`，可选，移除投递渠道
- `upsertChannelTargets`: `array<{channel: string, to?: string[], cc?: string[], clearCc?: boolean}>`，可选，配置共享 SMTP 邮件渠道的收件人列表（引用无默认收件人的 SMTP 渠道时必填 `to`；支持 `clearCc: true` 单独清空抄送）
- `removeChannelTargets`: `string[]`，可选，按渠道移除 `channelTargets`
- `filter`: `{setSeverities?: array<"info" | "warning" | "error">, clearSeverities?: boolean, addApplications?: string[], removeApplications?: string[], clearApplications?: boolean, addTargets?: string[], removeTargets?: string[], clearTargets?: boolean, clearFilter?: boolean}`，可选，纯增量规则过滤条件（`targets` 支持 `dplt_xxx` 或 `<appIdentifier>:<stage>`，同项目多应用同名 `stage` 未消歧时返回 `409 ai.resource_ambiguous`；`clearFilter: true` 重置为整项目 `{scope: "projects", projectIds: [projectId]}`，杜绝整组覆盖误伤其他过滤维度；注：对齐前端与后端 RBAC，管理员专属 `NotificationTemplate` 不向租户项目规则暴露，更新时底层原样保留 `existing.TemplateID`）
- `locale`: `"zh-CN" | "en-US"`，可选
- `timeZone`: `string`，可选，IANA 时区名
- `remindIntervalMinutes`: `integer`，可选，范围 `0..10080`（新建默认 `60`，更新省略保留原值）
- `maxReminders`: `integer`，可选，范围 `0..100`（新建默认 `3`，更新省略保留原值）
- `notifyOnRecovery`: `boolean`，可选
- `enabled`: `boolean`，可选，新建默认 `true`

**39. `ops_delete_project_notification_rule`** — 删除项目通知订阅规则（`L2`）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `rule`: `string`，必填，规则 ID 或唯一名称（返回 `{ id, projectId, deleted: true }`）

**40. `ops_test_or_retry_project_notification`** — 发送项目通知渠道测试消息或重试失败的项目通知投递记录（固定 `L2`，需当前项目 `ActionNotificationManage` 权限）
- `project`: `string`，必填，项目 ID 或 `identifier`
- `channel`: `string`，可选，测试已绑定至当前 `project` 的项目通知渠道 ID 或名称（Service 层强制校验 `channel.Scope == "project" && channel.OwnerUserID == ""` 且 `projectId ∈ boundProjectIDs`；与 `retryDeliveryId` 严格互斥二选一，受 `10/min` 速率限制保护）
- `testRecipient`: `string`，可选，SMTP 渠道测试收件邮箱
- `retryDeliveryId`: `string`，可选，重试归属当前 `project` 的项目通知失败投递记录 ID（**Service 层严格对齐 `scoped_notification_handlers.go:857` 强制校验 `delivery.ProjectID == projectId && delivery.RecipientUserID == ""`，严防跨用户重发或读取项目内他人个人通知**；仅放行 `"failed" | "enqueue_failed"` 或超时 `"sending"`，对 `"succeeded" | "pending" | "retry_pending"` 及未超时 `"sending"` 返回 `409 notification.delivery_not_retryable`）

**41. `ops_update_my_profile`** — 增量修改当前用户显示名称、头像、界面语言或主题色预设（`L1`，未传字段严格保持原值）
- `name`: `string`，可选
- `avatarUrl`: `string`，可选（省略绝不清空原头像！）
- `clearAvatar`: `boolean`，可选，显式清空个人头像
- `language`: `"zh-CN" | "en-US"`，可选
- `brandColorPreset`: `"inherit" | "blue" | "violet" | "teal" | "green" | "orange" | "pink"`，可选（精确对齐 `brand_theme.go`，`"inherit"` 映射空串跟随平台默认主题色）

**42. `ops_revoke_my_credential_or_grant`** — 吊销本人个人访问令牌（PAT）、吊销本人创建的第三方 OAuth 应用、撤销 OAuth 授权或解绑外部 OIDC 身份（`L2`）
- `targetType`: `"access_token" | "oauth_application" | "oauth_grant" | "external_identity"`，必填
- `targetId`: `string`，必填，对应 `tokenId`、`applicationId`、`grantId` 或 `identityId`
- 实现契约说明：`targetType` 为 `"oauth_application" | "oauth_grant"` 时严格保留 `oauthCookieUser` 本人非代管主会话校验（拒绝管理员代管会话越权吊销）；`targetType="external_identity"` 时，Service 层必须在数据库事务内对当前用户行加锁（`SELECT ... FOR UPDATE`）再校验登录方式保底规则——当 `passwordSet == false` 且仅剩最后 1 个 `external_identity` 时拒绝解绑并返回 `409 auth.last_login_method_protected`（防并发解绑锁死账号），且事务内必须补写 `external_identity.unbind` 业务 `AuditLog`（`AGENTS.md §6`）；统一返回 `{ id: targetId, targetType, deleted: true }`，并通过 `whenArgumentEquals: { targetType }` 精确路由至 `#25 ops_get_my_account_security` 对应的 `tokens / ownedOAuthApps / oauthGrants / externalIdentities` 子数组完成 `expect: "absent"` 写后校验。

---

## 第 5 批：`admin`（平台管理员专属治理域，共 33 个工具 = 11 `L0` + 22 `L2`，普通租户目录物理不可见，仅限全局会话执行）

> 架构与实现契约说明：
> 1. **双快照动态角色隔离与全局会话强沙箱（覆盖 HTTP 入口、异步 `RunExecutor`、`ToolOrchestrator`、`preview` 与 `checkpoint` 恢复，零穿透、保缓存）**：
>    - Go `/internal/v1/ai/provider-config`（`ai_provider_handlers.go`）导出的每个工具定义显式携带 `adminOnly: boolean` 元数据（不靠名称前缀猜测），Go BFF（`ai_proxy_handlers.go`）在签名请求头中携带当前 `actor.platformRole`；
>    - **项目绑定会话统一前置拦截（防跨项写系统项目/删全局资源及写后验证自锁）**：在单项目绑定会话（`conversationProjectID != ""`）中，`AIToolExecutionIdentityMiddleware`（`internal/api/aiapi/ai_tool_identity.go`）与 `/internal/v1/ai/tools/preview` 对全部 33 个 `adminOnly == true` 工具统一前置返回 `403 ai.project_scope_mismatch` 并按 `R11` 指引切换至全局会话执行；
>    - `luna-agent` 的 `ToolCatalogRegistry` 维护确定性排序的 `tenant` 目录（排除 `adminOnly`）与 `admin` 目录（`tenant + adminOnly`）双快照及各自独立的 `toolCatalogDigest`，`retain()` 同时保活两个活跃快照 Digest 防止 GC 误回收；所有 `tenant` 工具的 `verification` 严禁引用任何 `admin_*` 工具；
>    - HTTP 入口（`server.ts` `/turns`、`/tool-actions`）按签名 `actor.platformRole` 绑定初始角色快照并校验 `directToolAction`；后台异步 `RunExecutor`（`executor/index.ts`）在 `claimNextRun` 与唤醒恢复时直接从共享 PostgreSQL 实时回查 `users(role, disabled)` 与 `user_sessions(run.actorSessionId)`（对齐 `persistence/postgres.ts:108`），按实时角色解析 `catalogRegistry.forRole(liveRole)`（会话失效/账号禁用立即拒绝；若管理员中途降权导致 `liveCatalog.digest !== run.toolCatalogDigest`，**立即调用 `updateRunToolCatalogDigest` 持久化更新 `ai.runs.tool_catalog_digest`**，使读取 DB 状态的 `ToolOrchestrator`、审批决策与 `executePendingCall` 同步切至 `tenant` 快照并阻断挂起中的 `admin_*` 调用），同时 `ToolStepRunner.initialState()` 对 **`checkpoint` 分支恢复的 `run.selectedOperationIds`** 以及跨轮 `reconstructSelectedOperationIds() / restoredInternalToolResults()`（`executor/resume.ts:209-247`）一律按当前实时角色目录过滤裁剪！
> 2. **三层作用域边界归正**：支持 `user / project` 作用域的 `RuntimeCluster` 全套管理已归正至第 3 批 `runtime_*` 域；跨租户全局项目/全局搜索/全局事件通过第 4 批 `visibility="all"`（仅 `platform_admin` 可传）支持；本批 `admin_*` **仅收录 100% `platform_admin` 独占的平台治理能力**。
> 3. **业务审计硬约束（`AGENTS.md §6`）**：本批所有写工具在 Go Service 事务内必须写入明确的业务 `AuditLog`（实现时同步补齐 `CreateUser/UpdateUser`、`UpdateAuthAdmissionPolicy`、`Create/UpdateAuthProvider`、`UpdateBillingRateRules`、`UpdateConfigs` 全键变更审计，并在 `identityapi/admission.go:230-238` `auditResourceType` 补齐全部 7 个缺失前缀：`system_component`、`build_environment`、`mail`、`config`、`platform_app_template`、`notification`、`data_retention`）。

### 5.1 平台治理只读观测层（11 个，固定 `L0`，仅 `platform_admin` 可见）

**1. `admin_list_system_components`** — 查看各运行集群上的平台系统组件（如网关流量探针）安装状态、Pod 实时健康度及最近 10 分钟流量计费上报鲜活度
- `cluster`: `string`，可选，运行集群 ID 或名称
- `componentId`: `string`，可选
- 实现契约说明：Service 层同时聚合 `ListSystemComponents` 与 `GetGatewayTrafficStatus`（`gateway_traffic_handlers.go:35, 92-104`），一并返回探针 `status`（`"ready" | "not_configured"` 等）、`available: boolean` 及稳定 `observationCode`（`"billing.gateway_traffic_waiting_report" | "billing.gateway_traffic_report_stale" | "billing.gateway_traffic_probe_not_configured"`）。

**2. `admin_list_users_and_auth`** — 分页查看平台用户列表（含钱包余额与启停状态），可选附带登录准入策略、自助注册设置、OIDC 身份源（含禁用态与回调地址）及系统 SMTP 配置摘要
- `user`: `string`，可选，按用户 ID（`usr_xxx`）或唯一邮箱精确过滤单条用户记录（闭环平台用户超过 50 人时 `#16 admin_save_user` 的确定性写后回读校验）
- `search`: `string`，可选，按邮箱或姓名搜索用户
- `sortBy`: `"createdAt" | "email" | "name" | "role" | "passwordSet" | "status"`，可选，默认 `"createdAt"`
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `includeAuthSettings`: `boolean`，可选，默认 `false`，附带 `admissionPolicy`、`registrationSettings`、全部 OIDC Providers（`includeDisabled=true`，不回显 `clientSecret`）、`oidcCallbackUrl` 与 `mailSettings`（不回显 SMTP 密码）
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**3. `admin_get_system_configs`** — 查看平台全部 47 项全局系统配置（42 项非敏感配置 + 5 项加密密钥配置）的元数据定义与当前值
- `keys`: `string[]`，可选，指定查询的配置键列表；省略时返回 `internal/api/configs.go:45-235` 全部 47 项平台配置定义与当前生效值（5 项 `secret` 类型仅回显 `secretSet: boolean`，绝不回显明文或密文引用串）

**4. `admin_list_ai_models`** — 查看平台已配置的全部 AI 模型列表（含启用/禁用状态、上下文与输出上限、输入/输出/缓存输入百万 Token 费率，不回显 API Key）

**5. `admin_inspect_agent_observability`** — 查询 AI Agent 运行可观测性概览、对话/Turn/工具调用统计或单条对话/Trace 链路详情
- `view`: `"overview" | "conversations" | "turns" | "tools" | "tool_calls" | "conversation_detail" | "trace_detail"`，必填
- `range`: `"1h" | "6h" | "24h" | "7d" | "30d" | "1y"`，可选，默认 `"1h"`（精确对齐 `agent_observability_handlers.go:341` 时间窗枚举）
- `search`: `string`，可选，按关键词筛选对话、Turn 或工具
- `operationId`: `string`，可选，`view="tool_calls"` 时必填
- `conversationId`: `string`，可选，`view="conversation_detail"` 时必填
- `traceId`: `string`，可选，`view="trace_detail"` 时必填
- `sortBy`: `string`，可选，对齐各列表视图白名单排序字段（如 `"createdAt" | "durationMs" | "totalTokens" | "callCount" | "errorRate"`）
- `sortOrder`: `"asc" | "desc"`，可选，默认 `"desc"`
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**6. `admin_list_platform_app_templates`** — 分页查看平台级应用市场模板目录（含已下架 `enabled=false` 模板），或将指定内置/平台模板导出为 `luna.app-template/v1` Bundle
- `template`: `string`，可选，按平台模板 ID 或 `slug` 在摘要列表模式下精确过滤单条记录（零副作用、不写导出 `AuditLog`、不拉取大体积 `configFiles`，闭环平台模板超过 50 个时 `#26 / #27` 的写后回读校验）
- `query`: `string`，可选，关键词搜索
- `category`: `"collaboration" | "database" | "developerTool" | "middleware" | "observability" | "security" | "storage"`，可选
- `enabled`: `boolean`，可选，按上架/下架状态过滤
- `exportTemplate`: `string`，可选，传入内置或平台模板 ID/`slug` 时单查返回完整 `PlatformTemplateBundle`（含完整 `workload / components / configFiles / values` 供克隆改造；省略时走分页目录摘要模式，不返回大体积 `configFiles` 正文防爆上下文 `R10`）
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**7. `admin_list_platform_notifications`** — 按明确子视图分页查看平台全局/项目通知渠道、通知消息模板、全局通知规则、预设或全局规则投递记录
- `section`: `"channels" | "templates" | "rules" | "deliveries" | "presets"`，必填（单视图独立分页，Service 层对 `channels` 默认过滤 `scope = 'global' AND owner_user_id = ''`（可通过 `channelScope` 扩展审计全平台 `project` 渠道，始终排除个人 `user` 渠道），对 `rules` 严格过滤 `project_id = ''`，对 `deliveries` 按**归属全局规则作用域**过滤 `recipient_user_id = '' AND (rule_id = '' OR EXISTS (SELECT 1 FROM notification_rules r WHERE r.id = notification_deliveries.rule_id AND r.project_id = ''))`，既完整保留全局规则命中各项目事件的投递记录，又严格排除租户项目级规则投递数据！）
- `channelScope`: `"global" | "project" | "all"`，可选，默认 `"global"`，仅 `section="channels"` 有效（默认 `"global"` 精确闭环 `#28 / #32` 全局渠道写后回读；传 `"project" | "all"` 供管理员盘点全平台项目级渠道）
- `search`: `string`，可选，`channels / templates / rules` 按名称或关键字搜索
- `adapterKind`: `"webhook" | "smtp"`，可选，`channels / templates` 按适配器类型过滤
- `status`: `string`，可选，`deliveries` 按投递状态（如 `"pending" | "sending" | "succeeded" | "failed" | "enqueue_failed"`）过滤
- `eventType`: `string`，可选，`deliveries / templates` 按事件类型过滤
- `channel`: `string`，可选，`channels / deliveries` 按渠道 ID 或名称精确过滤（闭环超过 50 条时 `#28 / #32` 写后校验）
- `template`: `string`，可选，`templates / rules` 按通知消息模板 ID 或名称精确过滤（闭环超过 50 条时 `#29 / #32` 写后校验）
- `rule`: `string`，可选，`rules / deliveries` 按规则 ID 或名称精确过滤（闭环超过 50 条时 `#30 / #32` 写后校验）
- `project`: `string`，可选，`deliveries` 按事件发生项目（`notification_deliveries.project_id`）过滤，或 `section="channels" & channelScope!="global"` 按绑定项目过滤
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**8. `admin_preview_data_retention`** — 查看可清理历史数据集目录，或零副作用预览指定时间区间将匹配的清理条数
- `datasets`: `array<"platform_events" | "notification_deliveries" | "build_logs" | "release_logs" | "hook_run_logs" | "expired_auth_data">`，可选，省略时仅返回 6 类数据集定义目录；传入时配合 `startAt / endAt` 执行条数预览（精确对齐 `retention/catalog.go`）
- `startAt`: `string`，可选，RFC3339 起始时间（传 `datasets` 时必填）
- `endAt`: `string`，可选，RFC3339 结束时间（传 `datasets` 时必填，须晚于 `startAt`）

**9. `admin_get_global_build_env`** — 读取平台全局级（`scope="global"`）共享构建环境变量与已配置加密密钥键名数组 `secretKeys: string[]`（`L0`）

**10. `admin_inspect_billing`** — 查询全平台或指定用户的账单汇总、钱包流水、资源用量明细、跨项目消费排行与计费单价规则（`L0`，闭环管理员跨租户财务审计及 `#24/#25` 写后权威回读）
- `view`: `"platform_summary" | "user_summary" | "ledger" | "usage" | "deployment_spend" | "rate_rules"`，必填（对齐 `billingapi/scope.go:19-61` 与 `query_handlers.go`）
- `user`: `string`，可选，目标用户 ID 或唯一邮箱（`view="user_summary"` 时必填；`ledger / usage / deployment_spend` 传入时限定该用户，省略时汇总全平台）
- `projects`: `string[]`，可选，按项目 ID 或 `identifier` 过滤（注：`ai.*` 用量不归属项目）
- `ledgerEntryId`: `string`，可选，`view="ledger"` 时按流水 ID（`bled_xxx`）精确回读单条调账记录（供 `#25 admin_adjust_user_wallet` `verification` 使用）
- `ledgerType`: `"debit" | "credit" | "adjustment"`，可选，`view="ledger"` 时过滤
- `meter`: `"build.cpu_vcpu_minute" | "build.memory_gib_minute" | "build.job" | "runtime.cpu_vcpu_hour" | "runtime.memory_gib_hour" | "storage.gib_day" | "storage.transfer_gib" | "gateway.egress_gib" | "gateway.requests_1000" | "ai.prompt_tokens_1m" | "ai.completion_tokens_1m" | "ai.cached_prompt_tokens_1m" | "ai.cache_write_prompt_tokens_1m"`，可选，`view="usage"` 时过滤
- `periodStart`: `string`，可选，RFC3339（与 `periodEnd` 成对传入）
- `periodEnd`: `string`，可选，RFC3339
- `timeZone`: `string`，可选，IANA 时区名，默认 `"UTC"`（仅 `platform_summary / user_summary` 使用）
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`）

**11. `admin_list_git_providers`** — 全平台视角分页查看全部 Git 代码提供商配置（含 `global / project / user` 全作用域、他项/他人归属及 `enabled=false` 禁用项，闭环 `#13/#14` 发现与写后权威回读）
- `provider`: `string`，可选，按 GitProvider ID（`gitp_xxx`）或唯一名称精确单查
- `scope`: `"global" | "project" | "user"`，可选
- `enabled`: `boolean`，可选
- `search`: `string`，可选，按名称或 `baseUrl` 搜索
- `page`: `integer`，可选，默认 `1`
- `pageSize`: `integer`，可选，默认 `20`（上限 `50`；仅回显 `clientId`、`clientSecretSet: boolean`、关联账号数与仓库绑定数，绝不回显 `clientSecret`）

---

### 5.2 平台治理受控变更层（22 个，全部强制 `L2` 逐次参数绑定审批与业务 `AuditLog`）

**12. `admin_install_system_component`** — 向指定运行集群安装或更新平台系统组件（如网关流量探针，`L2`，异步触发系统项目 `Release`）
- `template`: `string`，必填，系统组件模板 ID（如 `"luna-gateway-traffic-probe"`）
- `cluster`: `string`，必填，运行集群 ID 或名称
- `apiBaseUrl`: `string`，必填，探针回报的平台 API 根地址
- `mode`: `"traefik-metrics"`，可选，默认 `"traefik-metrics"`
- `traefikMetricsUrl`: `string`，可选，集群内 Traefik Metrics 地址
- `image`: `string`，可选，自定义探针镜像覆盖
- `provisionAccess`: `boolean`，可选，默认 `true`
- 返回与验证契约：因全部 `admin_*` 工具仅限全局会话（`conversationProjectID == ""`）执行，响应统一回显系统项目权威 `projectId`、`installationId` 与新建 `releaseId`（`status: "pending"`），原生闭环 `family: "release"`（`delivery_get_release`）写后验证，并可直接衔接 `wait_for_task(projectId, "release", releaseId)` 与 `present_card(template: "live_task")` 内嵌 `live_progress(operationType: "release")`。

**13. `admin_save_git_provider`** — 创建或纯增量修改平台 Git 代码提供商配置（`L2`，因 `provider_handlers.go` 强制 `requirePlatformAdmin` 物理归入 `admin` 域）
- `provider`: `string`，可选，省略为新建，传入 GitProvider ID 或唯一名称为存量基底增量修改
- `type`: `"github" | "gitea" | "gitlab"`，可选，新建必填（注：`type="github"` 全局单实例且固定 `scope="global"`、默认 `baseUrl="https://github.com"`，若误传非 `"global"` 的 `scope / ownerUser / addProjects` 按 `R11` 直接报错拒绝，严禁静默忽略；`type="gitlab"` 当前仅支持 `authType="pat"` 克隆构建）
- `name`: `string`，可选，新建必填
- `baseUrl`: `string`，可选，`gitea / gitlab` 新建必填
- `scope`: `"global" | "project" | "user"`，可选，新建默认 `"global"`（更新省略时严格保留原 `scope`）
- `ownerUser`: `string`，可选，`scope="user"` 时指定归属用户 ID 或邮箱（**实现契约**：修复 `scoped_resource_helpers.go:37-38` 强制覆盖为 `currentUser.ID` 的限制，Service 层允许 `platform_admin` 解析目标 `ownerUser` 写入 `owner_ref`；省略时新建默认当前管理员、更新保留存量 `existing.OwnerRef`）
- `addProjects`: `string[]`，可选，`scope="project"` 时增量授权项目
- `removeProjects`: `string[]`，可选，`scope="project"` 时移除授权项目
- `authType`: `"oauth" | "pat" | "github-app"`，可选，新建默认 `"oauth"`（精确对齐 `gitapi/helpers.go:408`）
- `clientId`: `string`，可选（**更新省略时严格保留存量 `existing.ClientID`**，仅传 `clearClientCredentials: true` 时清空）
- `clientSecret`: `string`，可选（`writeOnly: true, x-luna-sensitive: true`；省略保留原密钥）
- `clearClientCredentials`: `boolean`，可选，显式清空 OAuth 客户端 `clientId` 与 `clientSecret`
- `enabled`: `boolean`，可选，新建默认 `true`
- 写后校验：绑定 `#11 admin_list_git_providers(provider: result.id)` 权威回读。

**14. `admin_delete_git_provider`** — 删除平台 Git 代码提供商配置（`L2`，存在关联用户账号或仓库绑定时一律阻断）
- `provider`: `string`，必填，GitProvider ID 或唯一名称
- 实现契约说明：修复现有 `provider_handlers.go:443-448` 无绑定时静默硬删 `GitAccount` 的隐患——Service 层在 `bindingCount > 0 || len(accountIDs) > 0` 时均返回 `409 git.provider_in_use`（附带关联绑定数与关联账号数），严禁静默级联硬删用户已连接 Git 账号；写后校验绑定 `#11 admin_list_git_providers` 校验 `expect: "absent"`。

**15. `admin_configure_global_build_env`** — 纯增量配置平台全局级（`scope="global"`）共享构建期变量与加密密钥（`L2`）
- `setVariables`: `array<{key: string, value: string}>`，可选，新增或更新全局构建普通变量（禁止 `LITEYUKI_*` 保留前缀，与同名加密密钥冲突返回 `409 build.variable_key_conflict`）
- `removeVariableKeys`: `string[]`，可选，删除指定普通变量
- `secretActions`: `array<{key: string, operation?: "set" | "clear", value?: string}>`，可选，单键写入或清除全局构建加密密钥（`value` 声明 `writeOnly: true, x-luna-sensitive: true`，`operation` 省略默认 `"set"` 且有效 `"set"` 强制非空，禁 `LITEYUKI_*` 前缀）

**16. `admin_save_user`** — 创建平台本地用户，或修改现有用户的平台角色、禁用状态与管理员密码重置（`L2`）
- `user`: `string`，可选，省略为新建用户，传入用户 ID 或唯一邮箱为增量更新
- `email`: `string`，可选，新建必填
- `name`: `string`，可选，新建必填
- `role`: `"platform_admin" | "user"`，可选，新建默认 `"user"`
- `disabled`: `boolean`，可选，禁用或恢复用户
- `language`: `"zh-CN" | "en-US"`，可选
- `password`: `string`，可选，**新建用户时必填且长度 `>= 8`**；更新他人账户时传入即重置密码（`>= 8` 位，`writeOnly: true, x-luna-sensitive: true`）
- 实现与防锁死契约：
  1. **最后一名活跃平台管理员保护**：数据库事务内 `SELECT ... FOR UPDATE` 校验，严禁将全平台最后 1 名活跃 `platform_admin` 降权为 `"user"` 或设为 `disabled=true`（返回 `409 auth.last_platform_admin_protected`）；
  2. **当前操作管理员自保围栏**：严禁当前登录管理员通过 AI 工具禁用自身、将自身降权为 `"user"` 或重置自身 `password`（返回 `409 auth.self_credential_mutation_forbidden`，避免 `revokeUserAuthentication` 当场吊销当前 `actorSessionId` 中断活跃 Run 并绕过 `updateMyPassword` 禁令）；
  3. `R12` 审批卡片显著提示：对目标用户变更 `role`、`disabled=true` 或重置 `password` 将在同一事务内立即吊销该用户全部活跃 Session、PAT 与 OAuth Token。

**17. `admin_configure_auth_policy`** — 单事务纯增量更新平台登录准入策略与自助注册设置（`L2`）
- `allowLocalLogin`: `boolean`，可选
- `allowOidcLogin`: `boolean`，可选
- `requireVerifiedOidcEmail`: `boolean`，可选
- `defaultRole`: `"user" | "platform_admin"`，可选
- `addAllowedEmailDomains`: `string[]`，可选，增量添加准入邮箱域名
- `removeAllowedEmailDomains`: `string[]`，可选，移除准入邮箱域名
- `clearAllowedEmailDomains`: `boolean`，可选
- `addAllowedOidcGroups`: `string[]`，可选，增量添加准入 OIDC 用户组
- `removeAllowedOidcGroups`: `string[]`，可选
- `clearAllowedOidcGroups`: `boolean`，可选
- `addInvitedEmails`: `string[]`，可选，增量添加受邀邮箱白名单
- `removeInvitedEmails`: `string[]`，可选
- `clearInvitedEmails`: `boolean`，可选
- `allowEmailRegistration`: `boolean`，可选，开启邮箱验证码自助注册（开启前后端强制校验平台 SMTP 已配置）
- `allowOidcRegistration`: `boolean`，可选，开启 OIDC 首次登录自动注册
- `allowExternalIdentityPassword`: `boolean`，可选，是否允许外部身份用户设置本地密码
- 防锁死契约：Service 层在同一事务内合并存量准入策略与注册设置，若合并后有效 `allowLocalLogin == false && allowOidcLogin == false`（或关闭本地登录但全平台无 `enabled == true` 的 OIDC Provider），返回 `409 auth.login_method_lockout_forbidden` 阻断平台登录自锁。

**18. `admin_save_oidc_provider`** — 创建或纯增量修改平台 OIDC 身份提供商配置（`L2`）
- `provider`: `string`，可选，省略为新建，传入 OIDC Provider ID 或唯一名称为增量更新
- `name`: `string`，可选，新建必填
- `issuerUrl`: `string`，可选，新建必填
- `clientId`: `string`，可选，新建必填
- `clientSecret`: `string`，可选，新建必填（`writeOnly: true, x-luna-sensitive: true`；更新时省略保留原密钥）
- `addScopes`: `string[]`，可选，增量添加 OIDC Scope（新建未传时默认 `["openid", "profile", "email"]`；更新时与存量 `scopes` 求并集，服务端序列化为空格分隔字符串）
- `removeScopes`: `string[]`，可选，增量移除 OIDC Scope（严禁移除必选 `"openid"`，误删返回 `400 argument_invalid`）
- `emailClaim`: `string`，可选，默认 `"email"`
- `usernameClaim`: `string`，可选，默认 `"preferred_username"`
- `groupClaim`: `string`，可选，默认 `"groups"`
- `enabled`: `boolean`，可选，新建默认 `true`
- `isDefault`: `boolean`，可选
- 对称防锁死契约（与 `#17` 闭环）：当更新将 `enabled` 置为 `false` 时，在同一事务内加锁校验 `auth_admission_policies` 与其余 `auth_providers`——若当前 `allowLocalLogin == false` 且除本 Provider 外全平台无其他 `enabled == true` 的 OIDC Provider，返回 `409 auth.login_method_lockout_forbidden` 阻断关闭最后一个可用登录入口。

**19. `admin_configure_platform_mail`** — 增量更新平台系统发信 SMTP 配置或发送测试邮件（`L2`）
- `host`: `string`，可选，SMTP 主机
- `port`: `integer`，可选，SMTP 端口
- `security`: `"none" | "starttls" | "tls"`，可选，默认 `"starttls"`（精确对齐 `platformmail/service.go:71`）
- `username`: `string`，可选（省略保持原值）
- `clearUsername`: `boolean`，可选，显式清空 SMTP 用户名（配合 `clearPassword: true` 切换回免认证 SMTP，避免触发 `platformmail.Validate` 非空用户名必填密码报错）
- `password`: `string`，可选，SMTP 密码（`writeOnly: true, x-luna-sensitive: true`；省略保留原密码）
- `clearPassword`: `boolean`，可选，显式清除已保存 SMTP 密码
- `fromAddress`: `string`，可选，发件人邮箱
- `fromName`: `string`，可选，发件人显示名（清空传 `clearFromName?: boolean`）
- `clearFromName`: `boolean`，可选
- `personalEmailCooldownSeconds`: `integer`，可选，个人邮件通知冷却秒数，范围 `0..3600`（默认 `60`）
- `sendTestTo`: `string`，可选，保存后（或仅传 `sendTestTo` 时直接用已存配置）向指定收件邮箱发送连通性测试邮件

**20. `admin_update_system_configs`** — 纯增量修改平台全局配置项，严格物理隔离 42 项非敏感配置与 5 项加密密钥配置（`L2`）
- `setConfigs`: `record<string, string>`，可选，仅允许传入 `configs.go` 中 42 项非 `secret` 配置键的非空字符串值（误传 5 项 `secret` 键时按 `R11` 报错指引改用 `secretActions`；遵循 `R7` 严禁通过传空串 `""` 隐式清空配置，需清空可空字符串/多行文本配置请传 `clearConfigKeys`）
- `clearConfigKeys`: `string[]`，可选，显式清空允许置空的非敏感字符串/多行文本配置键（如 `site.logoUrl`、`security.egress.*`、`ai.observability.*_tenant_id` 等）
- `secretActions`: `array<{key: "ai.provider.api_key" | "ai.web.proxy_pool" | "ai.observability.prometheus_token" | "ai.observability.loki_token" | "ai.observability.tempo_token", operation?: "set" | "clear", value?: string}>`，可选，5 项平台敏感密钥专属增量设置或清除（`value` 声明 `writeOnly: true, x-luna-sensitive: true`，`operation` 省略默认 `"set"` 且有效 `"set"` 强制非空）
- 实现契约说明：执行 `clearConfigKeys` 或 `secretActions: clear` 时，Service 层结合存量配置与本次 `setConfigs` 做启用态联动自保校验——当有效 `ai.assistant.enabled == true` 时**同时禁止清空 `ai.provider.base_url` 与 `ai.provider.api_key`**（防止当场中断 AI 助手运行），当有效 `ai.web.proxy_enabled == true` 时禁止清除 `ai.web.proxy_pool`，当有效 `ai.observability.enabled == true` 时禁止清空任一可观测数据源 URL；合法清除密钥时在同一事务内置空配置表引用并调用 `DeleteRefContextWithDB` 清理 `secret_values` 密文记录。

**21. `admin_save_ai_model`** — 新增或增量修改平台 AI 模型配置（`L2`）
- `model`: `string`，可选，省略为新增，传入模型 ID（`aimod_xxx`）或唯一 `name` 为存量基底增量修改
- `name`: `string`，可选，新增必填（模型唯一标识，即传给上游 Provider 的模型名）
- `enabled`: `boolean`，可选，新增默认 `true`（禁止禁用最后一个可用模型）
- `maxContextTokens`: `integer`，可选，新增必填，范围 `4096..2097152`
- `maxOutputTokens`: `integer`，可选，新增必填，范围 `256..262144` 且必须严格小于 `maxContextTokens`
- `inputCreditsPerMillion`: `string`，可选，新增必填，非负高精度十进制字符串
- `outputCreditsPerMillion`: `string`，可选，新增必填，非负高精度十进制字符串
- `cachedInputCreditsPerMillion`: `string`，可选，**新增必填**（对齐 `aimodel/service.go:323`），非负高精度十进制字符串
- 实现契约说明：更新模式下 Service 层先读取存量 `AIModel` 回填未传字段，再调用 `aimodel.Service.Update`。

**22. `admin_delete_ai_model`** — 删除平台 AI 模型配置（`L2`，最后一个启用模型受保护不可删除）
- `model`: `string`，必填，模型 ID 或唯一 `name`

**23. `admin_test_ai_integration`** — 测试平台当前 AI Provider 连通性或测试可观测数据源（Prometheus / Loki / Tempo）连接（`L2`）
- `target`: `"ai_provider" | "observability_source"`，必填
- `source`: `"prometheus" | "loki" | "tempo"`，可选，`target="observability_source"` 时必填
- `url`: `string`，可选，`target="observability_source"` 时可传（省略时回退读取系统配置对应 `keys.URL`，未配置则按 `R11` 报错提示传入或先配置）
- `token`: `string`，可选，待测试数据源临时访问令牌（`writeOnly: true, x-luna-sensitive: true`；省略时使用已存配置令牌）
- `tenantId`: `string`，可选，多租户数据源 `X-Scope-OrgID`（省略时回退读取系统配置对应 `keys.TenantID`）
- 实现契约说明：Service 层执行 `target="ai_provider"` 时直接从当前已认证 `AIPlatformActor` / 管理员身份构造 `aiagent.ActorContext`，去除 `TestAIProviderConnection`（`ai_provider_handlers.go:164`）对浏览器 Cookie 的硬依赖。

**24. `admin_update_billing_rate_rules`** — 增量修改平台全局资源计费单价与启停状态（`L2`）
- `rules`: `array<{meter: "build.cpu_vcpu_minute" | "build.memory_gib_minute" | "runtime.cpu_vcpu_hour" | "runtime.memory_gib_hour" | "storage.gib_day" | "storage.transfer_gib" | "gateway.egress_gib" | "gateway.requests_1000", creditsPerUnit?: string, enabled?: boolean}>`，必填（对齐 `billing/rate.go:29, 89`；Service 层读取存量规则表回填未传计量项及未传的 `creditsPerUnit / enabled`，严禁因省略 `enabled` 把计费项静默关停！）

**25. `admin_adjust_user_wallet`** — 为指定平台用户钱包执行充值（`credit`）或正负调账（`adjustment`）（`L2`）
- `user`: `string`，必填，目标用户 ID 或唯一邮箱
- `type`: `"credit" | "adjustment"`，必填（精确对齐 `billing/ledger.go:49`：`credit` 须传正数金额；扣减余额请传 `type: "adjustment"` + 负数 `amountCredits`，自动校验活跃 AI 预占冻结额度）
- `amountCredits`: `string`，必填，非零高精度十进制金额字符串
- `description`: `string`，必填，财务调账审计原因说明
- 实现契约说明：**强制将 `luna-agent` 请求头 `Idempotency-Key` 透传至 `WalletTransactionInput.IdempotencyKey`（`billing/ledger.go:64, 73-86`）确保网络重试 100% 幂等绝不重复入账/扣款**；写后校验绑定 `#10 admin_inspect_billing(view: "ledger", ledgerEntryId: result.id)` 权威回读。

**26. `admin_save_platform_app_template`** — 导入 `PlatformTemplateBundle` 或创建/纯增量更新平台自定义应用市场模板（`L2`，强类型工作负载补丁，彻底废除裸 `spec`）
- `template`: `string`，可选，更新现有平台模板时传入模板 ID 或 `slug`
- `importBundle`: `record<string, any>`，可选，直接导入 `luna.app-template/v1` 标准 Bundle（与下方手工编辑字段可择一使用，或导入后再叠加局部增量修改；可先调 `admin_list_platform_app_templates(exportTemplate)` 获取基准模板）
- `overwrite`: `boolean`，可选，配合 `importBundle` 使用，是否覆盖同 `slug` 平台模板，默认 `false`
- `slug`: `string`，可选，非 Bundle 新建必填（`2..63` 位小写字母数字连字符，禁止占用内置模板 `slug`）
- `name`: `string`，可选，非 Bundle 新建必填
- `description`: `string`，可选
- `category`: `"collaboration" | "database" | "developerTool" | "middleware" | "observability" | "security" | "storage"`，可选，非 Bundle 新建必填
- `icon`: `string`，可选（`<= 64` 字符）
- `version`: `string`，可选（新建默认 `"1.0.0"`）
- `enabled`: `boolean`，可选，上架或下架模板（更新时仅传 `template + enabled` 即可安全上下架，绝不清空原有组件与参数定义！）
- `featured`: `boolean`，可选
- `popularityWeight`: `integer`，可选，`0..1000`
- `officialWebsite`: `string`，可选（清空传 `clearOfficialWebsite?: boolean`）
- `clearOfficialWebsite`: `boolean`，可选
- `officialRepository`: `string`，可选（清空传 `clearOfficialRepository?: boolean`）
- `clearOfficialRepository`: `boolean`，可选
- `workload`: `{image?: string, servicePort?: integer, defaultReplicas?: integer, defaultCPU?: string, defaultMemory?: string, containerCommand?: string, clearContainerCommand?: boolean, containerArgs?: string, clearContainerArgs?: boolean, fsGroup?: string, clearFsGroup?: boolean, upsertDataVolumes?: array<{logicalName: string, sourceType: "projectVolume" | "emptyDir", mountPath?: string, devicePath?: string, readOnly?: boolean, emptyDir?: {medium?: string, sizeLimit?: string}}>, removeDataVolumeNames?: string[], setEnv?: record<string, string>, removeEnvKeys?: string[], setSecretEnv?: record<string, string>, removeSecretEnvKeys?: string[], upsertConfigFiles?: array<{path: string, content: string}>, removeConfigFilePaths?: string[], upsertSecretFiles?: array<{path: string, content: string}>, removeSecretFilePaths?: string[]}`，可选，单组件模板工作负载定义或增量补丁（与多组件 `upsertComponents` 互斥；单组件新建时 `image` 与 `servicePort` `1..65535` 必填；对齐 `appstore/platform.go:352-354`：`setSecretEnv / upsertSecretFiles` 仅允许写 `{{PLACEHOLDER}}` 模板变量引用，严禁明文密钥字面量！）
- `upsertComponents`: `array<{key: string, name?: string, workload: <同上增量 Workload 补丁结构>}>`，可选，多组件模板按组件 `key`（`^[a-z][a-z0-9]*$`，首元素为入口组件）新增或增量更新组件工作负载
- `removeComponentKeys`: `string[]`，可选，多组件模板按 `key` 移除组件
- `upsertValues`: `array<{key: string, label?: string, description?: string, default?: string, clearDefault?: boolean, required?: boolean, secret?: boolean, autoGenerate?: boolean}>`，可选，按 `key` 增量新增或更新模板参数定义（`secret: true` 禁止带明文 `default`，将存量参数切换为 `secret: true` 时服务端自动清空存量 `default`；普通参数移除默认值传 `clearDefault: true`）
- `removeValueKeys`: `string[]`，可选，按 `key` 移除模板参数定义

**27. `admin_delete_platform_app_template`** — 删除平台自定义应用市场模板（`L2`）
- `template`: `string`，必填，平台模板 ID 或 `slug`

**28. `admin_save_global_notification_channel`** — 创建或纯增量修改平台全局通知渠道，支持预设创建（`L2`，测试连通性请调 `#31 admin_test_or_retry_global_notification`）
- `channel`: `string`，可选，省略为新建，传入全局渠道 ID 或唯一名称为增量修改
- `name`: `string`，可选，新建必填
- `presetId`: `"feishu-bot" | "lark-bot" | "wecom-bot" | "gotify" | "dingtalk-bot" | "slack-webhook" | "discord-webhook"`，可选，精确对齐 7 类 Webhook 预设（自定义 Webhook / SMTP 省略 `presetId`；创建后 `presetId / adapterKind` 不可跨类型修改，预设渠道禁传 `upsertConfig / removeConfigKeys`）
- `adapterKind`: `"webhook" | "smtp"`，可选，默认 `"webhook"`
- `upsertConfig`: `record<string, any>`，可选，增量合并非敏感渠道配置（鉴权头须使用 `{"Authorization": "Bearer {{.Secrets.BearerToken}}"}` 占位符引用 `upsertSecrets`；**Service 层严禁在 `upsertConfig` 中传 `password`、未走 `{{.Secrets.*}}` 占位的敏感 Header（`Authorization / Cookie / *Token* / *Key* / *Secret*`）或内嵌 `url.User` 凭据的 URL，误传时按 `R11` 报错引导改走 `upsertSecrets`**）
- `removeConfigKeys`: `string[]`，可选
- `upsertSecrets`: `record<string, string>`，可选，增量写入签名密钥、Token 或 SMTP `password`（在 `additionalProperties` 值 Schema 上声明 `writeOnly: true, x-luna-sensitive: true`，生成 `upsertSecrets.*` 脱敏路径）
- `clearSecretKeys`: `string[]`，可选（清除 SMTP `"password"` 时同步清理 `config_json` 中的残留 `passwordRef`）
- `enabled`: `boolean`，可选，新建默认 `true`
- 实现契约说明：Service 层严格锁死 `channel.Scope == "global" && channel.OwnerUserID == ""`（严禁误匹配或篡改 `scope="project"` 渠道），预设创建复用干净单事务渠道落库逻辑（不向 `notification_templates` 额外插入冗余模板行）；写后校验固定绑定 `#7 admin_list_platform_notifications(section: "channels", channelScope: "global", channel: result.id)`。

**29. `admin_save_notification_template`** — 创建或纯增量修改平台通知消息模板（`L2`）
- `template`: `string`，可选，省略为新建，传入消息模板 ID 或唯一名称为存量基底增量修改
- `name`: `string`，可选，新建必填
- `eventType`: `string`，可选，新建必填（支持精确事件类型或通配）
- `adapterKind`: `"webhook" | "smtp"`，可选，新建必填
- `locale`: `"zh-CN" | "en-US"`，可选，默认 `"zh-CN"`
- `subjectTemplate`: `string`，可选（清空自定义标题模板传 `clearSubjectTemplate: true`）
- `clearSubjectTemplate`: `boolean`，可选
- `bodyTemplate`: `string`，可选（清空文本正文模板传 `clearBodyTemplate: true`）
- `clearBodyTemplate`: `boolean`，可选
- `jsonBodyTemplate`: `string`，可选（清空 JSON 正文模板传 `clearJsonBodyTemplate: true`）
- `clearJsonBodyTemplate`: `boolean`，可选
- `enabled`: `boolean`，可选，新建默认 `true`
- 写后校验：固定绑定 `#7 admin_list_platform_notifications(section: "templates", template: result.id)`。

**30. `admin_save_global_notification_rule`** — 创建或纯增量修改平台全局通知规则（`L2`，重试失败投递已物理隔离至 `#31`）
- `rule`: `string`，可选，省略为新建规则，传入全局规则 ID 或唯一名称为增量更新（Service 层严格锁死 `rule.ProjectID == ""`，防止误改项目级规则）
- `name`: `string`，可选，新建规则必填
- `addEventTypes`: `string[]`，可选
- `removeEventTypes`: `string[]`，可选
- `addChannels`: `string[]`，可选，全局通知渠道 ID 或名称
- `removeChannels`: `string[]`，可选
- `upsertChannelTargets`: `array<{channel: string, to?: string[], cc?: string[], clearCc?: boolean}>`，可选
- `removeChannelTargets`: `string[]`，可选
- `template`: `string`，可选，绑定自定义通知消息模板 ID 或名称（更新省略保留存量 `TemplateID`，清空传 `clearTemplate: true`）
- `clearTemplate`: `boolean`，可选
- `filter`: `{scope?: "all" | "projects", addProjects?: string[], removeProjects?: string[], addApplications?: string[], removeApplications?: string[], clearApplications?: boolean, addTargets?: string[], removeTargets?: string[], clearTargets?: boolean, setSeverities?: array<"info" | "warning" | "error">, clearSeverities?: boolean, clearFilter?: boolean}`，可选，纯增量全局规则过滤条件（精确对齐 `notification/rule_filter.go:19-25` 全部维度；更新时保留存量未提及维度，`clearFilter: true` 重置为合法 JSON `{scope: "all"}` 而非空串）
- `locale`: `"zh-CN" | "en-US"`，可选
- `timeZone`: `string`，可选
- `remindIntervalMinutes`: `integer`，可选，`0..10080`
- `maxReminders`: `integer`，可选，`0..100`
- `notifyOnRecovery`: `boolean`，可选
- `enabled`: `boolean`，可选
- 写后校验：固定绑定 `#7 admin_list_platform_notifications(section: "rules", rule: result.id)`。

**31. `admin_test_or_retry_global_notification`** — 发送平台全局通知渠道测试消息或重试失败的全局规则通知投递（固定 `L2`，`effect: "execute"`，与第 4B 批 `#34 / #40` 架构对称）
- `channel`: `string`，可选，测试指定平台全局通知渠道 ID 或唯一名称（Service 层强制校验 `channel.Scope == "global" && channel.OwnerUserID == ""`；与 `retryDeliveryId` 严格互斥二选一，受 `10/min` 速率限制保护）
- `testRecipient`: `string`，可选，全局 SMTP 渠道测试收件邮箱
- `retryDeliveryId`: `string`，可选，重试由平台全局规则触发的失败通知投递记录 ID（Service 层按归属全局规则隔离校验 `recipient_user_id == ""` 且关联 `rule.ProjectID == ""`，允许事件本身 `delivery.ProjectID != ""`；仅放行 `"failed" | "enqueue_failed"` 及超时 `"sending"` 状态，对 `"succeeded" | "pending" | "retry_pending"` 及未超时 `"sending"` 统一返回 `409 notification.delivery_not_retryable`）

**32. `admin_delete_platform_notification_resource`** — 删除平台全局通知渠道、通知消息模板或全局通知规则（`L2`）
- `resourceType`: `"channel" | "template" | "rule"`，必填
- `resource`: `string`，必填，对应资源 ID 或唯一名称
- 实现契约说明：Service 层严格限定全局作用域（`channel.Scope == "global" && channel.OwnerUserID == ""`、`rule.ProjectID == ""`），且删除前/删除后必须校验记录存在（`RowsAffected == 0` 返回 `404` 并不写伪成功审计日志；全局渠道仍被规则引用时返回 `409` 阻断）；响应统一返回 `{ id: "<resolved_id>", resourceType, section: "channels" | "templates" | "rules", deleted: true }`，并通过 `whenArgumentEquals: { resourceType }` 路由至 `#7 admin_list_platform_notifications` 对应子视图（分别传入 `channel / template / rule` 精确过滤与 `channelScope: "global"`）完成 `expect: "absent"` 校验。

**33. `admin_execute_data_retention_cleanup`** — 执行指定历史数据集与时间区间的不可逆清理（`L2`，`effect: "execute"`，同步返回各数据集实际清理行数，审批卡片按 `R12` 自动嵌入 `Preview` 匹配行数）
- `datasets`: `array<"platform_events" | "notification_deliveries" | "build_logs" | "release_logs" | "hook_run_logs" | "expired_auth_data">`，必填，非空数据集枚举数组
- `startAt`: `string`，必填，RFC3339 起始时间
- `endAt`: `string`，必填，RFC3339 结束时间（须晚于 `startAt`）
