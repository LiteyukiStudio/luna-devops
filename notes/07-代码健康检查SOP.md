# 代码健康检查 SOP

本文档用于定期检查 AI 长期协作开发后的代码健康度，避免大量最小补丁逐渐堆积成不可维护结构。执行目标不是频繁大重构，而是及时识别需要收口、抽象、拆分或重新设计的区域。

本项目的检查重点要贴合当前交付主线：API/worker 模块化单体、BuildKit 构建 Job、Kubernetes 部署、网关证书、账单、应用市场、Rspress 文档站和中英 i18n。检查结论必须能落到具体文件、业务域和验证命令，不做泛泛的“代码质量建议”。

## 1. 适用范围

- AI 连续修改超过 1 周或累计改动超过 20 个文件后。
- 某个页面、handler、worker 或 provider 被连续修复 3 次以上后。
- 上线前、版本冻结前、重要验收场景完成后。
- 出现同类 bug 反复复发、状态语义混乱、权限边界不清或 UI 组件风格分裂时。
- 涉及认证、Secret、数据库迁移、计费、构建网络策略、证书申请、Kubernetes 资源删除或生产内嵌前端缓存策略时。

## 2. 固定节奏

### 2.1 每周轻量检查

每周执行一次，目标是快速发现补丁堆积点。

必做命令：

```bash
git status --short
git diff --stat
go test ./...
pnpm --dir web lint
pnpm --dir web build
pnpm --dir docs build
```

检查项：

- 最近一周改动最多的 5 个文件。
- 最近一周重复出现的 bug 类型。
- TODO 中是否出现相同模块的连续修复项。
- 是否新增硬编码 UI 文案、绕过 i18n、绕过 DataList 或状态 Badge。
- 前端 lint/build 是否出现新的 warning；重点检查同步 effect 回填派生状态、受控/非受控状态混用、订阅回调写入已切换资源等问题。可保留的刻意告警必须有最小范围说明，不允许全局关闭规则。
- 是否有本地运行产物、日志、构建目录进入 `git status`。
- 代码改动是否同步更新文档站；用户流程、配置项、部署链路变更必须能在 `docs/` 中找到入口。
- 是否出现新的 `.env` 依赖、明文 Secret、Token 回显或后端原始错误直出。

产出：

- 在本文件的“健康检查记录”追加一条简短记录。
- 对明确需要后续处理的问题同步写入 `TODO.md`。

### 2.2 双周结构检查

每两周执行一次，目标是识别应该重构的模块边界。

辅助命令：

```bash
git log --since="2 weeks ago" --name-only --pretty=format: | sort | uniq -c | sort -nr | head -30
rg --files | rg '^(cmd|internal|web/src|docs/docs|migrations)/' | rg -v '(^docs/pnpm-lock.yaml|\\.(webp|png|jpg|jpeg|gif|svg)$)' | sed 's#^#"#; s#$#"#' | xargs wc -l | sort -nr | head -30
rg -n "TODO|FIXME|临时|兼容|fallback|special case|module|Builder|builder" internal web/src docs/docs migrations
```

检查项：

- 是否存在超过 800 行且仍在频繁变更的前端页面组件。
- 是否存在超过 600 行且同时处理参数解析、权限、业务逻辑、数据库和外部平台调用的 handler。
- 是否存在 worker/provider/handler 重复实现同一业务规则。
- 是否存在同一概念多套命名，例如 `module`、`deploymentTarget`、`config`、`release`、`deployment` 混用。
- 是否存在列表、弹窗、菜单、表单、状态展示没有复用统一组件。
- 是否存在前端编排外部平台 API 或后端 handler 直接散落外部平台细节。
- 是否存在权限只靠前端隐藏按钮，后端没有最终校验。
- 是否存在迁移 SQL、GORM 模型、API DTO 和前端类型不同步。
- 是否存在 API/worker 对同一构建、部署、计费、清理规则的不同实现。
- 是否存在中文文档已更新但英文文档缺失，或中英文导航结构不一致。

产出：

- 给每个高风险模块标注：`保持观察`、`局部重构`、`重新设计`。
- 局部重构任务必须能在 1 到 2 天内完成；超过该范围要先写设计方案。

### 2.3 每月设计复盘

每月执行一次，目标是确认产品模型和代码模型是否仍然一致。

按业务域复盘：

- 项目空间与权限。
- 应用与部署配置。
- 构建、Worker、BuildKit Job 和构建网络策略。
- Release、运行配置、重启和重新部署。
- 集群资源、归属、删除和事件。
- 网关、DNS、证书。
- 账单、余额、充值补偿和资源用量归属。
- 应用市场模板、镜像、官网和官方仓库元数据。
- 数据库迁移、启动自动迁移和历史数据修复。
- Rspress 文档站、历史记录和中英文内容同步。
- 前端布局、DataList、ContentTabs、ActionMenu。

判断标准：

- 用户侧概念是否仍然清晰。
- 数据模型是否还有旧概念残留。
- API 是否以业务意图为边界，而不是暴露底层平台能力。
- 前端是否以业务组件组合，而不是页面内堆状态和副作用。

产出：

- 最多选择 1 到 2 个最痛的区域进入重构。
- 其余问题进入 TODO，不在同一轮扩大范围。

## 3. 重构触发条件

满足任一条件时，必须暂停继续打补丁，先评估重构：

- 同一文件连续 3 次修复后仍出现同类问题。
- 同一业务规则在前端、handler、worker 或 provider 中重复实现。
- 一个页面组件同时承担数据查询、表单状态、权限判断、业务编排、复杂 UI 和多弹窗控制。
- 一个 handler 同时承担参数解析、权限、数据库查询、外部平台调用和任务投递。
- 为修一个小问题需要跨越 5 个以上无关模块。
- 新需求需要绕过现有抽象才能实现。
- 状态字段或资源归属出现语义不一致。
- 代码里出现“临时兼容”“旧模型 fallback”“特殊 case”并持续保留。
- 数据库迁移必须依赖人工补跑才可恢复核心链路。
- 生产部署、构建或证书问题需要通过手动 patch 集群资源才能长期可用。
- 账单、资源删除、构建网络策略等安全边界只能靠 UI 约束，后端或 worker 没有最终保护。

## 4. 重构分级

### 4.1 保持观察

适用于问题明确但影响较小的区域。

动作：

- 记录到健康检查记录。
- 不立即改动。
- 下一次检查继续观察是否复发。

### 4.2 局部重构

适用于边界清楚、可在短时间内完成的改动。

常见动作：

- 拆出公共组件。
- 抽出 hook 或 helper。
- 把 handler 中的业务逻辑下沉到 service。
- 把重复状态映射收敛到统一函数。
- 删除不再需要的旧模型 fallback。

完成标准：

- 行为不变或有明确用户可见改进。
- 有针对性测试或浏览器验收。
- 文档和 TODO 同步更新。

### 4.3 重新设计

适用于产品模型或边界本身已经不对的区域。

触发例子：

- 旧模型继续影响新模型，比如“模块”残留影响“应用 / 部署配置”。
- 权限模型无法表达当前操作边界。
- 资源归属、运行状态、发布状态之间职责混乱。
- 一个页面已经无法通过局部拆分恢复清晰结构。

动作：

- 先写设计说明。
- 明确旧数据是否兼容。
- 明确迁移、验收和回滚方式。
- 拆成多步任务，不允许一次性无边界大改。

## 5. 当前项目重点关注区域

### 5.1 应用详情和部署面板

风险：

- `web/src/pages/applications/application-deployments-panel.tsx` 已成为前端最大热点之一。
- 应用详情涉及构建、部署、访问入口、运行配置、日志和 Web Console，容易把查询、表单、菜单和运行态聚合堆在一个面板里。

建议：

- 将构建、部署、访问拆为独立 panel 文件。
- 将部署配置表单、Release 菜单、运行状态聚合拆成独立组件或 hook。
- 保留应用详情页作为编排层，不直接承载所有业务细节。
- 对超过 800 行的页面面板保持拆分压力；如果新增功能只服务某个 tab，优先落到该 tab 的私有组件或 hook。

### 5.2 构建与部署链路

风险：

- `DeploymentTarget`、`BuildRun`、`Release`、运行配置、重启、重新部署和 Kubernetes 资源之间关系复杂。
- API、worker、Kubernetes provider 容易重复拼装资源名、namespace、labels、网络策略和运行配置。
- 构建 Job 的网络策略、镜像源、私有 registry 和非标端口白名单会直接影响用户能否构建成功。

建议：

- 后端抽出部署和构建 service，统一处理 DeploymentTarget 解析、Release 创建、资源名生成、网络策略生成和运行态校验。
- 明确“重启”和“重新部署”的语义：重启只 rollout restart；重新部署创建 Release 并重新下发资源。
- 网络策略默认值必须有单测覆盖；允许放行的私有网段、DNS、HTTP/HTTPS 和自定义镜像站端口要由白名单配置驱动。

### 5.3 集群资源归属

风险：

- 资源归属、删除权限、状态展示、事件查询都依赖 Kubernetes labels。
- labels 缺失或旧资源残留时容易出现误归属或误删风险。

建议：

- 为资源归属建立专门测试。
- 删除接口必须读取资源 labels 后再判断项目空间和应用归属。
- 前端只展示当前用户有权维护的操作。
- 对缺失 label 的历史资源只允许进入诊断/清理流程，不能自动归属到当前项目空间。

### 5.4 账单与余额

风险：

- 计费已经从项目空间余额调整为用户账户余额，资源用量仍来自项目空间、应用、构建和运行态。
- 创建人转移、资源删除、历史应用和迁移前数据可能造成计费归属漂移。

建议：

- 账单流水不可因项目空间或应用删除而删除。
- 用量归属必须记录当时的计费用户，不能在查询时只按当前 owner 反推。
- 充值、补偿、扣费、退款必须走同一套 ledger/service，避免 handler 直接改余额。

### 5.5 应用市场

风险：

- 模板同时包含镜像、官网、官方仓库、默认配置和安装参数，容易出现前端展示和后端模板字段不同步。
- Grafana 等应用可能依赖 Prometheus 等外部组件，单应用模板容易暗示“直接可用”。

建议：

- 模板字段新增或改名时同步检查 `internal/appstore/templates.json`、API 类型、前端卡片和文档。
- 对存在外部依赖的模板，在模板描述和文档中明确“可安装”和“完整可观测方案”的边界。
- 安装入口只表达单应用安装能力，不提前承诺组合市场。

### 5.6 数据库迁移

风险：

- API 滚动更新时会执行迁移，迁移脚本、GORM 模型和历史库状态不一致会造成部分老数据行为异常。
- 计费、软删除、唯一约束、Secret 加密字段属于迁移高风险区。

建议：

- 每个迁移都要说明是否可重复运行、是否修复历史数据、是否破坏兼容。
- 涉及核心链路的迁移需要用已有数据快照或集成库验证。
- 迁移失败时 API 应明确失败并暴露可诊断日志，不允许静默跳过。

### 5.7 文档站和历史记录

风险：

- 项目同时维护 `docs/` Rspress 用户文档和 `notes/` 设计记录，容易出现口径不一致。
- 中英文导航、配置项表格和部署教程不一致会直接影响用户上手。

建议：

- 用户操作、部署、配置和故障排查优先写入 `docs/docs/{zh,en}`。
- 工程内部设计沉淀写入 `notes/`，不要混入用户文档。
- 每次新增用户可见能力，检查中文和英文导航是否都有入口。

### 5.8 DataList、ContentTabs 和操作菜单

风险：

- 管理台列表很多，容易每页手写列宽、滚动、菜单和状态。

建议：

- 列表默认使用 DataList。
- 页签和工具区默认使用 ContentTabs。
- 行内操作默认使用三点菜单，删除类操作使用 destructive 样式。
- 移动端默认允许列表内部横向滑动，不撑开页面。

### 5.9 运行配置

风险：

- ConfigMap、Secret、配置文件、运行数据卷、重启和重新部署容易被用户混淆。

建议：

- UI 文案持续区分“保存”“重启”“重新部署”。
- 公共配置编辑后必须提示受影响部署配置。
- 配置文件路径必须做冲突校验。

## 6. 执行清单

每次健康检查按以下顺序执行：

1. 拉取当前变更范围：`git status --short`、`git diff --stat`。
2. 找大文件和热点文件：看最近提交、diff stat 和行数排行。
3. 执行验证命令：`go test ./...`、`pnpm --dir web lint`、`pnpm --dir web build`、`pnpm --dir docs build`。
4. 检查硬规则：i18n、权限、Secret、列表组件、状态 Badge、后端外部平台适配。
5. 检查重复业务规则：资源名、namespace、网络策略、状态映射、权限判断、运行配置合并、计费归属。
6. 检查迁移和文档：migrations 是否自动执行、docs 是否同步、TODO 是否需要更新。
7. 判断重构级别：保持观察、局部重构、重新设计。
8. 更新记录：本文件追加健康检查记录，必要时同步 TODO。
9. 只实施本轮明确范围内的修复，不顺手扩大。

## 7. 健康检查记录模板

```md
## YYYY-MM-DD 代码健康检查

### 范围

- 检查模块：
- 检查原因：
- 当前变更：

### 验证结果

- `go test ./...`：
- `pnpm --dir web lint`：
- `pnpm --dir web build`：
- `pnpm --dir docs build`：
- 其他针对性验证：

### 发现

| 模块 | 问题 | 风险 | 分级 | 处理 |
| --- | --- | --- | --- | --- |
|  |  |  | 保持观察 / 局部重构 / 重新设计 |  |

### 本轮结论

- 立即处理：
- 进入 TODO：
- 暂不处理：
- 文档同步：
```

## 8. 2026-06-14 代码健康检查

### 范围

- 检查模块：应用部署页、部署链路、集群资源状态刷新。
- 检查原因：AI 长时间连续修复部署配置、运行状态、DataList、重启和资源归属问题，存在补丁堆积风险。

### 验证结果

- `go test ./...`：通过。
- `pnpm --dir web lint`：通过，保留既有 `react/set-state-in-effect` warning。
- `pnpm --dir web build`：通过，保留 Vite chunk 体积提示。

### 发现

| 模块 | 问题 | 风险 | 分级 | 处理 |
| --- | --- | --- | --- | --- |
| `ApplicationConfigPage.tsx` | 页面职责过多，部署、构建、访问、运行配置和日志逻辑集中 | 后续补丁容易相互影响 | 局部重构 | 进入后续重构候选 |
| 部署链路 | 重启、重新部署、Release 状态、Kubernetes 运行态语义复杂 | 用户和代码都容易混淆 | 保持观察 | 已补文档语义，继续观察 |
| 集群资源状态 | 历史 Release 缺部署配置时曾退化为 `dplt` 资源名 | 误报 Kubernetes 资源丢失 | 局部重构 | 已修复 worker 防御逻辑 |

### 本轮结论

- 立即处理：修复后台轮询状态闪烁、部署重启入口、缺 DeploymentTarget 时误报 `dplt`。
- 进入 TODO：后续拆分 `ApplicationConfigPage.tsx`。
- 暂不处理：全局项目空间选择器、Bundle 拆分。

## 9. 2026-06-14 代码健康修复复查

### 范围

- 检查模块：异步资源清理、项目空间删除、删除态写保护、部署配置端口模型、worker 热点文件。
- 检查原因：全面检查发现项目空间删除只清理一个集群、删除中资源仍可被后端写入口操作、旧应用级服务端口字段残留、异步清理状态机测试不足。

### 验证结果

- `go test ./internal/worker ./internal/api ./internal/database`：通过。
- `go test ./...`：通过。
- `pnpm --dir web lint`：通过，保留既有 `react/set-state-in-effect` warning。
- `pnpm --dir web build`：通过，保留 Vite chunk 体积提示。

### 发现

| 模块 | 问题 | 风险 | 分级 | 处理 |
| --- | --- | --- | --- | --- |
| 项目空间删除 | 删除项目空间时只取最早环境的运行集群清理 namespace | 多集群项目空间会残留其他集群资源 | 局部重构 | 已改为按项目环境覆盖的集群逐个清理 |
| 删除态写保护 | 前端禁用 deleting 操作，但后端部分写入口仍可直接调用 | 清理任务与更新/重启/发布并发导致状态漂移 | 局部重构 | 已增加统一 mutate guard 并覆盖项目、环境、部署配置、访问入口、运行配置和运行时写操作 |
| 旧模型字段 | `applications.service_port` 从产品模型退场但已有库会保留 | 旧概念残留影响后续判断 | 保持观察 | 已加入启动清理 SQL，破坏式移除旧字段 |
| `worker.go` | 资源清理状态机继续堆入 worker 主文件 | 文件职责过多，后续难以测试 | 局部重构 | 已拆出 `internal/worker/resource_cleanup.go` 并补资源清理单测 |

### 本轮结论

- 立即处理：以上四项已完成。
- 进入 TODO：继续拆分 `ApplicationConfigPage.tsx`、`deployment_handlers.go`、`web/src/api/client.ts` 和 locale 文件。
- 暂不处理：旧异常业务数据兼容；项目未上线，必要时清库重建。

## 10. 2026-06-14 热点大文件拆分复查

### 范围

- 检查模块：运行集群 handler、前端 API client、i18n locale、应用配置页概览面板。
- 检查原因：SOP 热点文件仍处于高风险区，用户确认优先通过重构拆分不可维护的大文件，并允许 i18n 按 page/namespace 拆分。

### 验证结果

- `go test ./internal/api`：通过。
- `go test ./...`：通过。
- `pnpm --dir web lint`：通过，保留既有 5 个 `react/set-state-in-effect` warning。
- `pnpm --dir web build`：通过，保留 Vite chunk 体积提示。

### 发现

| 模块 | 问题 | 风险 | 分级 | 处理 |
| --- | --- | --- | --- | --- |
| `internal/api/deployment_handlers.go` | 运行集群 CRUD、资源快照和 kubeconfig 逻辑与部署/发布逻辑混在同一文件 | 单文件职责过多，后续改部署链路容易误碰集群管理 | 局部重构 | 已拆出 `internal/api/runtime_cluster_handlers.go` |
| `web/src/api/client.ts` | DTO 类型与请求实现混在一个 1200+ 行文件 | API 类型变更和请求逻辑变更互相干扰 | 局部重构 | 已拆出 `web/src/api/types.ts`，`client.ts` 保持 re-export 入口 |
| i18n locale | 中英 locale 均为 1600+ 行单文件 | 页面文案维护冲突高，review 噪音大 | 局部重构 | 已按语言目录和 namespace/page 拆分，根文件只聚合 |
| `ApplicationConfigPage.tsx` | 概览、构建、部署、访问、终端逻辑仍集中在一个页面文件 | 文件仍偏大，表单/运行态后续变更风险高 | 局部重构 | 已先拆 Overview 面板和 release 时间工具，剩余面板后续继续拆 |

### 本轮结论

- 立即处理：以上四类热点文件已完成第一轮拆分。
- 进入 TODO：继续按面板拆 `ApplicationConfigPage.tsx` 的构建、部署、访问区域；继续按资源族群拆 `deployment_handlers.go` 的环境、发布和部署配置逻辑。
- 暂不处理：为旧异常数据保留兼容层；项目未上线，异常数据优先清库重建。

## 11. 2026-07-01 部署表单输入焦点复查

### 范围

- 检查模块：应用部署页部署配置 Dialog、动态表单行编辑器、全项目前端输入表单。
- 检查原因：部署配置 Dialog 中服务端口输入框出现输入一个字符后失焦，需要重新聚焦才能继续输入。
- 当前变更：修复 `ServicePortsEditor` 动态行 key，避免 key 绑定正在编辑的端口名称和端口号。

### 验证结果

- `pnpm --dir web lint`：通过，保留既有 11 个 React warning。
- `pnpm --dir web build`：通过，保留 Vite chunk 体积提示。
- 其他针对性验证：使用 `rg` 扫描 `web/src` 下 `key={...}`、动态表单行和输入表单；动态配置文件、构建变量/密钥、运行数据卷等表单行均使用稳定 `id`，未发现同类可编辑值作为动态行 key 的问题。

### 发现

| 模块 | 问题 | 风险 | 分级 | 处理 |
| --- | --- | --- | --- | --- |
| `web/src/pages/applications/application-deployment-service-ports-editor.tsx` | 服务端口动态行使用 `${name}-${port}` 作为 React key，输入时 key 随可编辑值变化 | 输入框每次变更都会 remount，导致焦点丢失和输入被打断 | 局部重构 | 已改为组件内部稳定行 id，并保留提交前的端口 normalize 兜底 |
| 全项目前端表单 | 复扫动态表单行 key 和输入控件组合 | 同类 bug 若复发会影响 Dialog 表单连续输入 | 保持观察 | 当前未发现其他同类问题；后续新增动态行编辑器时禁止使用可编辑字段作为 key |

### 本轮结论

- 立即处理：修复部署配置服务端口编辑器的动态行 key。
- 进入 TODO：无，本轮未发现需要拆分成后续任务的问题。
- 暂不处理：非表单展示列表中使用业务字段作为 key 的场景不影响输入焦点，本轮不扩大范围。
- 文档同步：已在本 SOP 记录本次表单焦点问题、扫描范围和处理结论。

## 12. 2026-07-12 认证与发布安全复查

### 范围

- 检查模块：登录会话与生产初始化、TOTP/恢复码/Step-up MFA、管理员 MFA 重置、Web Console 项目/部署配置策略与持续授权、数据导出授权、发布质量门禁。
- 检查原因：安全审查发现 remember token 重放、MFA 绑定主身份复核、TOTP 重放、失败审计落库、长连接授权持续性及契约同步仍有缺口。
- 当前变更：完成后端安全加固、前端交互与测试、数据库迁移、OpenAPI、双语文档、TODO 和发布门禁同步。

### 验证结果

- `go test ./...`、`go vet ./...`：通过。
- PostgreSQL 集成测试：`AUTH_TEST_DATABASE_URL=... go test -count=1 ./internal/api ./internal/database` 通过。
- `pnpm --dir web test`：6 个测试文件、16 条测试通过。
- `pnpm --dir web lint`：0 error，保留 13 个既有 React warning；`pnpm --dir web build`：通过。
- `govulncheck ./...`：Go `1.26.5` 与 `quic-go 0.59.1` 下无可达漏洞。
- `pnpm --dir web audit --prod`、`pnpm --dir docs audit --prod`：无已知漏洞。
- `helm lint`、`helm template`：通过，仅保留可选 Chart icon 提示。
- `pnpm --dir docs build` 与 OpenAPI YAML 解析：通过；前端 production preview 的 `/`、`/account`、`/settings/users`、`/projects` 和主 JS 资源 HTTP smoke 通过。

### 发现

| 模块 | 问题 | 风险 | 分级 | 处理 |
| --- | --- | --- | --- | --- |
| Remember login | token 轮换缺少 family 绝对期限、主认证时间继承和完整 session 撤销 | 被盗旧 token 或旧 session 可能在轮换/退出后继续使用，remember 恢复可能伪装为新鲜 OIDC 登录 | 立即修复 | 已增加 token family、旧 token 墓碑、`primary_authenticated_at` 和 family 级 session/assertion 撤销；每族只保留最新 session，remember 恢复继承主认证时间且迁移前 OIDC session fail closed |
| MFA | 绑定前未复核主身份、TOTP 时间步可重复、最后管理员保护有并发竞态、管理员缺少受控重置入口 | 会话被劫持后可绑定验证器，验证码可重放，并发解绑可能让平台失去可用 MFA 管理员 | 立即修复 | 本地密码/OIDC 主认证复核、计数器防重放、事务行锁保护、带 Step-up 与最后管理员保护的审计重置入口均已实现 |
| Step-up 安全配置 | 进程内缓存只在当前 API 副本更新，策略开启与最后管理员解绑、禁用或降级之间存在并发窗口；事务持锁后从全局连接池重读配置可能导致池饥饿 | 多副本下其他实例可能仍按旧策略免验证，或留下“策略已开启但无人可验证”的锁死状态；单连接池可能自锁 | 立即修复 | 安全判断改为从共享 PostgreSQL 读取；读取失败使用启用 MFA 和短超时的 fail-closed 值，批量配置更新先完整验证再单事务写入；策略修改与管理员 MFA 解绑/重置/账号状态变更共用 PostgreSQL 事务锁，并只使用当前事务重读策略、复核 actor/session/assertion 和管理员集合 |
| Web Console | WebSocket 只在握手时检查，项目禁用可被部署级 true 覆盖，监视轮询会自行刷新空闲期限 | 长连接在撤权后继续存在，项目策略可绕过，空置高权限 shell 一直活到绝对期限 | 立即修复 | 项目开关改为硬上限，增加 HTTP 预检与每 3 秒持续复核；只有真实 stdin 输入节流刷新 idle，resize/ping/轮询不续期 |
| 数据导出 | GET 可直接触发副作用，预检不是强制票据；固定导出 Pod 会让并发任务互删 | 跨站顶层导航可诱导资源消耗，并发导出相互中断 | 立即修复 | authorize 签发 60 秒一次性、全维度绑定票据，生产 Redis `GETDEL` 原子消费且故障 fail closed；每次导出使用独立临时 Pod并先读取首块数据再提交下载头 |
| 审计日志 | GORM `default:true` 可能让失败审计落成成功，写入错误被静默忽略 | 安全审计结论失真或缺失 | 立即修复 | 审计写入改为显式字段 map，失败至少输出带上下文日志；MFA 解绑与管理员重置的成功审计和业务变更位于同一事务 |
| OpenAPI / 双语文档 | remember login、MFA 重认证与重置、持续终端授权、数据导出预检和 Web Console 硬上限未同步 | 客户端实现和运维操作可能依赖过期契约 | 局部重构 | 已按当前实现补齐并保持中英文一致 |
| TODO | MFA 聚合任务把已完成和未覆盖范围混在同一项 | 发布判断会把部分完成误认为全量完成 | 保持观察 | 已拆明完成项；资源删除和高风险部署 Step-up 仍作为后续范围保留 |
| 发布门禁 | Go 版本和发布检查前置条件缺少文档入口，CI 未提供 PostgreSQL 导致认证/迁移集成测试被 skip | RC 可能使用错误工具链或绕过真实数据库测试 | 立即修复 | 已固定 Go `1.26.5`，Quality Job 启动 PostgreSQL，`release-check.sh` 缺少 `AUTH_TEST_DATABASE_URL` 时拒绝继续；普通 Go/race 与非缓存 PostgreSQL 集成套件分开执行，避免同一测试在 CI 重复三次 |

### 本轮结论

- 立即处理：认证、MFA、Web Console、数据导出、审计、OpenAPI、双语文档和发布门禁缺口已完成。
- 进入 TODO：保留资源删除与高风险部署操作的 Step-up 覆盖，不把未实现范围误标为完成。
- 暂不处理：无；聚合发布脚本仍会按设计拒绝脏工作区，合并前需在干净工作区再执行一次。
- 文档同步：通过。

## 13. 认证与发布安全审计流程

### 13.1 范围与证据

1. 先固定基线提交、目标环境和改动文件，列出认证入口、会话/Token、OIDC、MFA/Step-up、权限、Secret、审计日志、长连接/导出及发布链路。
2. 同步检查数据库迁移、OpenAPI、前端类型与 i18n、中英文档、依赖锁文件、CI/镜像构建和 Helm Chart；不在范围的模块必须显式记录，不得默认为已审计。
3. 每条发现记录「路径/接口、攻击或失败前提、影响、分级、责任人、修复提交、测试证据、剩余风险」；结论必须可由命令输出或人工复现，不接受只看代码的「应该安全」。

### 13.2 并行 Agent 边界

- 主审 Agent 维护唯一发现清单和分级口径；按认证后端、PostgreSQL/迁移、前端/契约、依赖/CI/Helm 分配互斥文件边界。
- 每个 Agent 开工前声明「可修改路径、只读依赖路径、交叉点」；发现越界问题只上报，由对应责任人修改，不覆盖、回退或重写其他 Agent 的未提交改动。
- 交付时报告基线、实际改动文件、验证命令、未解决项和冲突点；主审 Agent 先重读最新 diff，再做跨域整合。

### 13.3 P0 / P1 / P2 分级

| 级别 | 判定 | 处理和退出条件 |
| --- | --- | --- |
| P0 | 可绕过认证/授权或 MFA，泄露 Secret，会话/Token 可重放，可跨租户读写，迁移可破坏数据，或发布链路可被接管 | 立即停止合并/发布；本轮修复并补回归测试，独立复审通过后才能解除阻断 |
| P1 | 需特定前提才能利用，但可造成提权、撤权失效、审计失真、并发不一致或安全默认值失效 | 发布候选版前修复；若暂缓，必须有可验证的缓解、责任人和截止日期，并由安全负责人接受风险 |
| P2 | 防御纵深、错误可观测性、文档/契约偏差或低可利用性加固项 | 记入 TODO 并给出优先级；不得将已知 P2 省略后宣称「无安全问题」 |

### 13.4 修复与独立复审

1. 修复者先写能稳定复现的失败测试，修复根因后补充失败路径、跨用户/跨项目、过期/撤销、重放和并发场景；权限和安全配置读取失败时默认 fail closed。
2. 复审者必须与修复者不同，从原始发现和最新完整 diff 独立建模；不仅确认测试变绿，还要尝试绕过、检查新增攻击面和剩余竞态。
3. 复审结论只能是「通过」、「带已接受剩余风险通过」或「重开 P0/P1」；P0/P1 不得由原修复者自己关闭。

### 13.5 PostgreSQL 并发与迁移测试

- 使用专用、可销毁的 PostgreSQL 库设置 `AUTH_TEST_DATABASE_URL`，执行 `go test -count=1 ./internal/api ./internal/database`；输出中出现因未配数据库而 `skip` 视为门禁失败。
- 迁移至少覆盖「历史 schema/数据 -> up -> 业务不变式 -> down 或明确不可逆 -> 重新 up」，校验 `NOT NULL`/唯一约束/外键/索引、回填顺序、旧数据 fail closed 和重启后状态。
- 对「最后管理员」、Token 轮换/撤销、一次性票据、安全配置更新等不变式，用独立连接和 barrier 同时提交冲突操作，循环多次并直接查库校验最终状态；同时执行 `go test -race ./internal/api ./internal/worker ./internal/provider/kubernetes ./internal/secret`。
- 测试必须证明行锁、事务级 advisory lock 或原子 SQL 真正保护共享数据；进程内 mutex 不能作为多副本安全证据。

### 13.6 基础发布门禁

| 领域 | 最低门禁 |
| --- | --- |
| 后端 | `gofmt` 无差异；`go test ./...`；上述 PostgreSQL 非缓存测试；`go vet ./...`；关键包 `go test -race` |
| 前端 | `pnpm --dir web install --frozen-lockfile`；`pnpm --dir web test`；`pnpm --dir web lint`；`pnpm --dir web build`；lint/build 不得有未解释的 warning；对登录、账号安全、管理员用户和项目空间路由做 production preview smoke |
| 契约/文档 | 解析 OpenAPI，核对 code/枚举、请求响应与前端类型；`pnpm --dir docs install --frozen-lockfile`；`pnpm --dir docs build`；用户可见流程中英文同步 |
| 依赖 | 审查 `go.mod`/`go.sum` 与 pnpm lockfile 差异；`govulncheck ./...`；`pnpm --dir web audit --audit-level=high`；`pnpm --dir docs audit --audit-level=high`；高危可达漏洞阻断发布 |
| Helm | `helm lint charts/luna-devops`；`helm template luna-devops charts/luna-devops` 结果非空；复核 Secret/RBAC、安全上下文、探针、资源限制、非浮动镜像 tag 和生产 values 覆盖 |

干净工作区的 RC 最终执行 `AUTH_TEST_DATABASE_URL=... ./scripts/release-check.sh`。聚合脚本通过不代替针对本轮发现的业务回归、并发测试和独立复审。

### 13.7 完成口径

- **本轮完成**：范围内的 P0 已清零，P1 已修复或完成显式风险接受，每条发现有证据和独立复审结论，针对性测试通过，契约、文档和 TODO 已同步。只能表述「本轮已审计范围完成」。
- **项目可发版**：除满足本轮条件外，还需所有发布范围已纳入审计，无未处置 P0/未接受 P1，干净 RC 提交的完整门禁通过，迁移、回滚/不可逆策略、部署配置、制品来源和运维文档均已验收。
- 本轮完成时仍要单列「未审计范围」、「已接受风险」和「项目整体发版阻断项」，不得用局部结论替代项目发版签字。
