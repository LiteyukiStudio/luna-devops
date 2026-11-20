# Luna DevOps Skills 编写与维护规范

> **打包隔离声明**：本文件（`skills/README.md`）仅面向仓库开发者与维护者说明 Skill 设计准则，**严禁打包进任何运行环境**（已通过 `skills/embed.go` 精确路径白名单、`luna-agent/Dockerfile` 定向复制及根目录 `.dockerignore` 物理排除，避免污染 Agent 上下文）。

本目录维护 **Luna DevOps 唯一一套工作流规程事实源**（`skills/luna-devops/`），同时服务于：
1. 对外公开的 `/api/v1/mcp` Streamable HTTP MCP 网关（通过 `load_skill` 元工具、`luna://skills/*` Resources 与 Prompts 下发）；
2. 平台内置 AI 助手 `luna-agent`（通过运行时 `load_skill` 按需加载）；
3. 支持本地 `SKILL.md` 开放标准（`agentskills.io`）的外部客户端（Claude Code、Cursor、Codex、Antigravity 等，可直接整包复制 `skills/luna-devops/`）。

---

## 1. 职责分工铁律（什么该写 vs. 什么严禁写）

遵循 Anthropic **“MCP 提供工具与实时契约，Skill 提供编排菜谱与工程防坑指南”** 的分工原则：

| 知识层级 | 唯一权威事实源 | `skills/luna-devops/` 中允许写什么 | 严禁写什么 |
| --- | --- | --- | --- |
| **工具输入/输出契约** | Go `internal/aitool` 注册表（经 `search_tools` / `get_tool_details` 实时输出） | 仅点名**核心入口工具**与**低自由度复合写操作的最小正确 JSON 骨架** | 严禁大段枚举某领域全部工具名，严禁复制完整字段表或枚举字典 |
| **产品规则与语法细节** | `docs/docs/` 公开文档（经 `search_docs` / `read_doc` 按版本内嵌检索） | 写出何时必须先查文档（如计费单价、内置变量名、Dockerfile 模板渲染细节） | 严禁把长篇产品手册正文粘贴进 Skill |
| **工作流与控制面约束** | **本目录 `SKILL.md` + `references/*.md`** | 跨工具先后顺序、硬门禁（Gate）、并发状态漂移重对齐（Re-grounding）、异步假象识别与错误恢复表 | 严禁科普通用 Kubernetes / Docker / HTTP / TLS 基础概念（默认模型已具备） |
| **Web 专属 UI 卡片/路由** | `luna-agent/src/prompt/` | **完全不写**（保持宿主无关性） | 严禁出现 `present_card`、`request_input`、`request_choice`、`navigate_to_route` 等管理台内部工具 |

---

## 2. 目录结构与三层渐进披露规范

采用标准自包含单包结构，确保外部用户单独复制 `skills/luna-devops/` 时所有相对引用均完整可用：

```text
skills/
├── README.md                                # 维护规范（不进入二进制与容器镜像）
├── embed.go                                 # Go 精确嵌入 luna-devops/SKILL.md 与 references/*.md
├── embed_test.go                            # 结构、TOC、宿主中立性与打包隔离单测
└── luna-devops/                             # 标准自包含 Agent Skill 目录
    ├── SKILL.md                             # Level 1 (Frontmatter) + Level 2 (全局铁律与路由表，< 500 行)
    └── references/                          # Level 3 (7 个正交业务域深度 SOP，单层扁平引用)
        ├── workspace-iam.md
        ├── source-registry-build.md
        ├── application-deployment.md
        ├── runtime-volumes.md
        ├── gateway-topology.md
        ├── diagnostics-governance.md
        └── platform-admin.md
```

### 硬性排版约束
1. **Level 1（YAML Frontmatter）**：
   - 仅保留 `name`（kebab-case，必须与目录名或注册 `skillId` 完全一致）和 `description`（单行，< 1024 字符）。
   - `description` 是触发路由器：必须以第三人称明确写出 **核心能力（What） + 典型用户意图/报错触发词（When） + 适用边界**。
2. **Level 2（`SKILL.md` 正文）**：
   - 总行数严格控制在 **500 行以内**（建议 120～200 行）。
   - 只写跨领域通用规则（渐进检索、`L0/L1/L2` 与 `previewLock`、稳定锚点与 Re-grounding、异步有界等待、安全与人工边界）及指向 `references/*.md` 的意图路由表。
3. **Level 3（`references/*.md`）**：
   - **单层扁平引用（Keep references 1 level deep）**：只能从 `SKILL.md` 直接指向 `references/<skillId>.md`，严禁在 `references/` 下建子目录或设计 `A.md -> B.md -> C.md` 的多跳阅读链。
   - **百行目录铁律（100-Line TOC Rule）**：任何超过 **100 行** 的 `reference` 文件，正文首个二级标题必须是 `## 目录`，便于模型局部定位章节而不遗漏后半段门禁。

---

## 3. SOP 编写四大模式（Anthropic 推荐实践）

### 3.1 按任务脆弱度设定自由度（Degrees of Freedom）
- **高自由度（启发式漏斗）**：用于诊断与探索（规定观测先后层级与收口原则，不限定僵死路径）。
- **低自由度（严格步骤 + 最小 JSON 骨架）**：用于高风险、强顺序依赖或含互斥/增量语义的写操作（如 `secretActions`、`urlRewrite` 与 `requestRedirect` 切换、`ServiceBinding` 注入、数据保留两阶段清理）。示例字段名必须与 Go 输入结构体 JSON Tag 完全一致。

### 3.2 `Plan → Gate → Execute → Verify` 与 Checklist
- 跨 3 个以上工具的端到端流程必须提供可复制核对的 `- [ ]` Checklist，并在异步或高危衔接处标明 **`Gate（硬门禁）`**——前置终态未满足时明确要求转入排障或停机汇报，禁止跳步执行下游工具。

### 3.3 防范系统性失效：稳定锚点与 Re-grounding 通则
不要用零散特例去修补动态环境变化，统一按三类控制面通则编写：
1. **稳定锚点 vs. 易失句柄（State Drift）**：区分稳定锚点（`project` / `application` / `target` / `route`）与随时消亡的快照句柄（`pod`、`previewLock`、`revision`）。对易失句柄收到 `not_found` 或 `conflict` 时，**原地重试次数强制为 `0`**，必须退回父级稳定锚点重新观测当前状态（例如滚动更新期间旧 Pod 消失，立即重读 `runtime_get_target_diagnostics`）。
2. **异步最终一致性假象（Async Illusions）**：明确标注哪些状态属于正常异步收敛而非故障（如 `WaitForFirstConsumer` 数据卷首次挂载前 `Pending`、新建 `GatewayRoute` 短期 `progressing`、`buildRunId`/`releaseId` 入队受理）。
3. **有界反馈循环（Bounded Feedback Loops）**：所有等待或重试指引必须给出明确的停止条件（最大轮询次数与退避节奏），严禁无上限循环调用。

### 3.4 高密度恢复表优于长段落叮嘱
每个 `reference` 末尾统一维护结构化的 **`异常 / 状态漂移与错误恢复速查表`**（`现象或错误码 | 根因 | 标准恢复动作（含禁止行为）`）。

---

## 4. 修改后的必跑验证清单

每次新增或调整 `skills/luna-devops/` 内容后，提交前必须通过以下检查：

1. **Go 嵌入、Frontmatter、TOC 与隔离测试**：
   ```bash
   go test ./skills/... ./internal/api/aiapi/... -run "TestEmbeddedSkills|TestMCP"
   ```
2. **166 工具目录真实性契约校验**（自动扫描 `skills/luna-devops/` 中所有反引号工具标识，防止拼写漂移）：
   ```bash
   pnpm --dir luna-agent test tests/skill-loader.test.ts tests/platform-catalog-retrieval.test.ts
   ```
3. **公开文档同步检查**（若变更了 `skillId` 或安装路径说明）：
   ```bash
   pnpm --dir docs build
   ```
