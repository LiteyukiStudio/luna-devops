# AI 知识层、Skills、公开文档与人工交接引导审计备忘

- 日期：2026-11-19
- 前置依赖：待 [2026-11-19-ai-domain-tools-catalog.md](./2026-11-19-ai-domain-tools-catalog.md) 全平台 166 个领域门面工具编码落地后，按本清单一次性同步收口知识层、Skills、前端深链路由与中英文公开文档。
- 设计原则：遵循 Anthropic Context Engineering 与 `AGENTS.md §4`——**不是越多越好，是越精准、高信噪比、零冲突越好**。

---

## 一、前端深链导航与人工操作交接闭环（`navigate_to_route` + `ContentTabs` + Skills）

### 1.1 系统级路由断链修复
1. **补齐 `aiInternalRouteNames` / `internal-routes.ts` / `navigation-event.tsx` 漏注册的 5 个真实控制台路由**（对照 `web/src/App.tsx:45-62`）：
   - `application.deployment.edit` -> `/projects/:projectId/apps/:applicationId/deployments/:targetId/edit`（部署目标高级 K8s 字段唯一人工编辑页，支持锚点 `#section-advanced`）
   - `application.deployment.new` -> `/projects/:projectId/apps/:applicationId/deployments/new`
   - `inbox` -> `/inbox`（站内信与待办决策页，`query: { filter?, message? }`）
   - `notification-channels` -> `/notification-channels`（租户/跨项目通知渠道管理页）
   - `oauth.device` -> `/oauth/device`（设备码浏览器人工授权页，`query: { user_code? }`）
2. **打通二级 Tab 深链（消除 `#tab=` vs `?tab=` 协议失配）**：
   - 升级 `web/src/components/common/content-tabs.tsx:32-94` 订阅 React Router `useLocation()` 并同时兼容读取 `?tab=` 与 `#tab=`（覆盖 `/settings/account`、`/code-repositories`、`/registries`、`/clusters`、`/billing`、`/settings/auth-providers`、`/settings/operations`、`/settings/site` 8 个二级 Tab 页面）；
   - 为交互卡片 `resource_links` 与 `navigateAction`（`luna-agent/src/tools/ui-cards.ts` + `web/src/components/common/ai-assistant/interaction-cards.tsx`）补齐 `routeQuery?: Record<string, string>`（及可选锚点支持），不再硬编码 `query: {}`。

### 1.2 十类人工专属/半人工操作交接映射表（写入导航与对应领域 Skill）
- **Git OAuth 浏览器绑定**：`code-repositories`（`tab=credentials`）-> 点击「通过 `<Provider>` OAuth 连接」；
- **外部 OIDC 身份绑定**：`settings.account`（`tab=security`）->「已绑定外部身份」点击「绑定身份提供商」；
- **创建个人访问令牌（PAT，AI 永久禁签发）**：`settings.account`（`tab=tokens`）->「创建访问令牌」并在一次性弹窗复制保存；
- **修改本人密码（AI 永久禁执行）**：`settings.account`（`tab=security`）-> 填写原密码与 8–72 字节新密码（忘记密码联系平台管理员重置）；
- **第三方 OAuth 应用创建/改回调/轮换密钥 & 设备码验证**：`settings.account`（`tab=oauth-applications` / `tab=oauth-grants`）及 `oauth.device`（`/oauth/device`）；
- **容器交互式 WebSocket 终端**：单次诊断走 `runtime_exec_command`；交互式 TTY 跳 `application.detail`（`params: { projectId, applicationId }, query: { tab: "deployments" }`）点击「Web 控制台」（或 CLI `luna deployment exec`）；
- **项目数据卷本地归档上传导入 & 导出归档浏览器下载**：跳 `project.workspace`（`params: { projectId }, query: { tab: "volumes" }`）上传本地归档；导出由 AI 先调 `runtime_create_volume_export`，待 `ready` 后引导用户在卷详情抽屉「传输记录」点击「下载」；
- **部署目标高级 K8s 字段（`initContainers / sidecarContainers / lifecycle / tolerations / affinity / topologySpreadConstraints / serviceAnnotations / autoScalingBehavior`）**：跳 `application.deployment.edit`（`#section-advanced`）人工编辑保存并发版；
- **部署配置 JSON Bundle 导出与跨项目导入**：同应用跨环境克隆直接调 `delivery_create_target(cloneFromStage)`；JSON 导出/跨项目导入向导跳 `application.detail`（`query: { tab: "deployments" }`）；
- **站内信待办人工查看**：AI 可调 `ops_list_inbox -> ops_decide_inbox_action`，人工入口跳 `inbox`（`query: { filter: "action" }`）。

---

## 二、提示词与 Skills 脱水提纯及 100% 领域对齐（目标削减 >50% 常驻底噪）

1. **修复常驻冲突（P0 / P1）**：
   - `luna-agent/skills/luna-devops-interaction/SKILL.md:59`：将错误的 `template: summary | table | comparison | steps | code | review | live_task` 修正为真实 Schema 枚举 `result | change_review | live_task`；
   - `luna-agent/src/prompt/system.ts:11` 与 `luna-devops-interaction/SKILL.md:26`：修正过期的“`L1` 为常规非生产变更”表述，对齐定稿目录（工作负载/扩缩容/重启/配置/发版全环境固定 `L2`，全域仅 10 个轻量元数据/个人偏好工具为 `L1`）；
   - `luna-agent/src/tools/conversation-title.ts:5,10` 与 `executor/tool-step.ts:760`：统一 `rename_conversation` 始终提供语义与 `1..60` 字符上限。
2. **常驻 Token 瘦身（当前空载 53,090 字符 / ~16k–20k Tokens）**：
   - `luna-agent/src/tools/ui-cards.ts`：启用 Zod v4 `z.toJSONSchema(input, { io: "input", reused: "ref" })` 复用重复子结构，并剥离无需模型传入的 `schemaVersion / mode` 可选字面量；
   - 清理 `system.ts`、`luna-devops-interaction`、`luna-devops-navigation` 与内置工具 `description` / `guidance` 之间的 4～5 重重复叮嘱，将 `luna-devops-navigation/SKILL.md` 改造为高密度「已注册 `routeName` 参数字典 + 人工交接速查表」。
3. **按需领域 Skills 100% 覆盖与更新**：
   - 全量重写存量 6 个 `luna-agent/skills/domains/*/SKILL.md`（对齐 166 个 `<domain>_*` 工具名、`R8` 双模 `project / application / target` 定位、独立 `delivery_scale_target`、`build_retry_run` 快照防呆、个人 vs 项目通知拆分、跨项目 `NetworkShare` 闭环）；
   - **新增第 7 个按需领域 Skill `domains/platform-admin/SKILL.md`**（仅对 `platform_admin` 暴露），覆盖第 5 批 33 个 `admin_*` 工具、单项目会话 `403 ai.project_scope_mismatch` 切换全局会话引导、防锁死围栏、42 明文 + 5 密文系统配置与 Agent 可观测四层下钻。

---

## 三、公开文档（`docs/docs/{zh,en}`）失真修复与缺口补齐清单

### 3.1 6 处事实失真纠正（P0）
1. `docs/docs/en/index.md:12-31`（及 `en/start/connect-resources.md:95`）：补齐 `/en/start/...` 与 `/en/use/...` 前缀，修复英文首页按钮跳回中文站 Bug。
2. `docs/docs/{zh,en}/use/workflow/routes.md:14-21`：将路由级 `tlsMode` 更正为真实 3 项（`http-only / http-challenge / manual-cert`），与运行集群级 `externalTlsMode` 及通配符证书明确区分。
3. `docs/docs/{zh,en}/start/configuration/agent.md:52`：删除已废弃的“超过 32 轮保留 16 轮”，更正为按官方 `total_tokens > maxContextTokens - reserveTokens` 触发摘要压缩。
4. `docs/docs/{zh,en}/reference/channel-affinity.md:81,89`：更正 DeepSeek `reasoning_content` 规则为已完成历史轮次省略、仅当前活跃轮次工具链回传且缺省回填单空格 `" "`（同步修正 `web/src/i18n/locales/*/settings.ts` 旧描述）。
5. `docs/docs/{zh,en}/use/billing.md:50`：删除已废除的“进入对账 / 预授信值 credit hold”，更正为直接报 `ai.provider_usage_unavailable` 拒绝结算。
6. `docs/docs/{zh,en}/reference/compatibility.md:13` vs `start/connect-resources.md:91`：统一 GitHub / Gitea / GitLab 三类 Provider 精确能力矩阵（GitLab 支持 PAT 克隆 + 必填同源 `cloneUrl`，不支持远端搜仓与自动 Webhook）。

### 3.2 核心功能与精确语法补齐（P0 / P1，精炼高密度）
- **新增 `docs/docs/{zh,en}/use/account-and-access.md`**：覆盖本地改密、外部 OIDC 绑定/解绑保底、PAT 创建与 Scope 交集、第三方 OAuth 应用注册/密钥轮换/Grants 撤销及 Device Flow / Auth Code + PKCE 端点参考；
- **新增 `docs/docs/{zh,en}/reference/system-settings.md`**：覆盖 `configs.go` 全部 47 项系统配置（含 `security.egress.*` SSRF 黑白名单、`ai.runtime.*`、`storage.projectManagedCapacityLimitGiB`、6 类 `retention.*Days` 数据集与预览清理流程）、用户/准入/OIDC 防平台自锁、平台自定义应用模板 `luna.app-template/v1`（`{{PARAM_KEY}}` / `{{components.<key>.host}}`）；
- **补齐现有工作流与参考页**：
  - `build-and-release.md` / `build-variables.md` / `deployment.md`：**回滚仅回退镜像不回退配置**、原地重启不下发未发布改动、开 HPA 时缩容冲突、跨阶段克隆 `cloneFromStage`、`requireApproval`、首次发布后集群锁定、12 套内置构建模板、`buildDirectory`、变量集 `user/project/global` 作用域、Hooks 8 阶段与超时/失败策略；
  - `runtime-data-persistence.md`：**已发布目标解绑数据卷进入 `release_pending` 须发新 Release 释放 PVC 锁**、`managed vs referenced` 销毁/计费差异、归档流式上传/下载/取消传输步骤；
  - `project-space-and-apps.md` / `routes.md`：精确 4 角色权限表、`ServiceBinding`（`url / host_port` + 跨服务 `SecretMap`）与手工拓扑连线边界、网关 CNAME / `Exact` / `parentGateway` / 9 类特权 Header / `urlRewrite` / `requestRedirect`；
  - `notifications.md` / `billing.md` / `ai-assistant.md`：站内信 `/inbox` 与计费归属转移闭环、第 7 预设 `Gotify`、自定义 Webhook **`{{.Secrets.<KeyName>}}`** 占位符、全局通知 Go 模板 8 个内置函数、13 项精确 `meter` ID 与管理员 `credit / adjustment` 调账、AI 助手 `L0 / L1 / L2` 审批与单项目 vs 全局会话边界；
- **清理冗余水分（P2）**：删除 `configuration-and-secrets.md:17` 升级迁移说明、`docker-compose.md` 重复段落与口号、`binary.md:113-120` 重复小节、`agent.md` 内部编译器白盒描述，修复中英细微漏词与空格笔误。

---

## 四、知识检索与索引管线修复清单（`luna-agent/src/knowledge`）

1. **修复 `redact()` 误伤静态公开文档（P0）**：`luna-agent/src/redaction.ts:75` 为 `sk-` 补前置非字母数字边界 `(?<![A-Za-z0-9])`（修复 `task-notification` 被截断）；`search_docs / list_docs / read_doc` 静态公开文档返回体跳过运行时凭据正则擦除（保留 `%20` 鉴权头与连接串示例）。
2. **修复分词器与领域同义词召回盲区（P0）**：`retrieval/tokenizer.ts:60-61,156` 消除英文词双倍 push、过滤单字符 ASCII 碎片（修复 `"K8s 部署"` 跑偏）、平衡中文双字词权重；在 `docs-knowledge.ts` 增加轻量确定性同义词映射（`hpa <-> 自动伸缩`、`扩缩容 <-> 自动伸缩`、`k8s <-> kubernetes`、`pat <-> 个人访问令牌`、`pvc <-> 数据卷`、`callback <-> 回调`）。
3. **消除 `read_doc` 8,000 字符硬截断与切片缺陷（P1）**：支持可选 `offset` 分段续读（返回 `nextOffset`），标题解析扩展至 H1～H4 并补齐父标题面包屑，段落切分跟踪代码围栏状态防止劈断代码块，搜索摘要改为命中词居中窗口。
4. **阻断历史轮次 `read_doc` 正文跨轮重复计费与快照漂移（P1）**：跨轮 `historyModelMessages` 对已结束轮次的 `read_doc` 仅保留 `{ docId, sectionId, title, url }` 元数据摘要，同 Run 内增加 `cacheHit` 去重；`bundle-docs.ts` 显式排除 `changelog/` 并在 `scripts/ci/check-docs.sh` 联动执行知识打包与检索回归测试。
