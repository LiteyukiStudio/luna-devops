# Luna DevOps 全项目冗余与过度设计审计

审计日期：2026-09-04

## 1. 结论

当前最值得处理的不是依赖升级或零散格式问题，而是四类已经产生维护成本的结构：

1. `internal/api` 的领域拆分主要移动了文件，却保留了根包 facade、14 组 bridge、导出别名和胖 `Host` 接口。引入拆分的提交在 bridge/handler/exports/compat 文件组新增了 10,040 行，当前仍有 4,660 行显式兼容桥。
2. 前端存在可确认的死代码：一个 448 行整文件副本、294 个失效 i18n key、18 个无生产调用的 API Client 方法、约 100 行零引用声明，以及约 429 KiB 未引用静态资源。
3. 后端模型中保留了不存在的 Environment 领域、只有一个实现的策略字段、三层网关域名来源和半实现的多 Builder/Lease/缓存脚手架。这些字段让 API、数据库、CLI 和 Web 都承担了不存在的产品能力。
4. 文档和流程同时维护事实、历史和未来构想：`TODO.md`、CLI 规格、Agent README、两套 AGENTS、检查 SOP、更新日志镜像分别复制代码或 GitHub 已经拥有的事实，并且已经发生漂移。
5. 补充的代码过度防御专项检查确认了三种会掩盖错误的模式：内部不变量被测试专用的 nil/optional 分支稀释、私有契约被多形态 fallback 重复“修复”、明确引用或 2xx 响应失败后仍被转换成成功。它们不是可靠性增强，而是在延迟暴露装配或契约错误。

建议先做不改变产品行为的直接删除，再分别立项处理 API facade、伪领域模型、OpenAPI 类型单源化和 Agent/CLI 协议兼容层。不要把它们塞进一个“大清理”提交。

报告现列出 54 个编号处置项；其中本次过度防御专项新增 8 项，并对已有相关项补充了生产调用链证据。

保守估算：

- 高置信直接清理可减少约 4,000～5,500 行跟踪文本和约 453 KiB 静态资产；其中大头是失效翻译和过期 CLI 规格，不是产品代码。
- API facade 重构预计再净减 2,500～3,500 行生产胶水。
- Web OpenAPI 类型单源化预计再净减 1,500～2,200 行手写契约。
- 如果确认不交付两份 Grafana Dashboard，可再删除 3,035 行 JSON。
- CLI 的两个生成 TypeScript 文件合计 135,212 行，但当前都有真实消费者且 release 流程要求它们预先存在；停止跟踪只是待设计的构建策略，不应算作当前可删代码。更直接的收益是先从运行时 catalog 去掉无消费者的 `responses`，预计减小约 2.31 MB。
- 过度防御专项新增项预计可再净减约 200～350 行生产/测试胶水；其中 Worker、Agent 依赖装配和第一方 CLI Scope 残留是主要代码收益，构建引用、Webhook 和 HTTP 响应项的首要收益则是停止伪成功，不能只按行数衡量。

上述区间存在交叉，不能简单相加。

## 2. 范围与方法

审计覆盖：

- Go API、Worker、模型、Provider、配置、迁移和命令入口；
- `web/` 页面、组件、API Client、类型、i18n、依赖和静态资源；
- `luna-agent/` 执行器、Prompt、Skill、Provider、测试和协议投影；
- `docs/`、`docs-internal/`、`TODO.md`、贡献规范和组件清单；
- Helm、Compose、GitHub Actions、CI/release 脚本、Grafana 资产；
- 相邻仓库 `/Users/sfkm/Developer/liteyuki-studio/luna-cli`，以及主仓忽略目录 `cli/luna-cli` 中尚未合并的源码、契约、Skill 和文档。

快照口径：主仓索引有 1,866 个路径，其中 2 个处于删除状态；当前仍存在的跟踪内容为 318,222 行，另有 2 个未跟踪迁移共 35 行。相邻 CLI 索引有 188 个路径，其中 2 个处于删除状态；186 个当前存在的跟踪文件共 182,391 行。主仓、相邻 CLI 与忽略目录中的独立 CLI 工作树在审计前都已有改动，本报告按当前工作树判断，没有覆盖或整理这些改动。

证据方法包括：静态 import/route 图、符号和路径反向引用、Git 变更统计、OpenAPI/模型/迁移交叉核对、Vitest 收集列表、依赖来源、文档站根和部署入口核对。`go mod tidy -diff` 无差异，未发现可直接删除的 Go module 依赖。

处置含义：

- **删除**：无生产消费者，或只是无效/重复入口，可独立清理。
- **收敛**：保留能力，但合并重复事实源或只保留一个有效实现。
- **重构后删除**：当前有调用，不能直接删；先替换结构，再删兼容层。
- **门禁后删除**：代码已死，但涉及数据、外部调用或未确认的运维习惯。
- **保留**：审计过但有明确职责，不能因体积或相似度删除。

风险分级：低＝无用户可见行为；中＝开发/运维入口或局部契约改变；高＝公开 API、数据库、认证、跨进程或核心流程改变。

### 2.1 代码过度防御专项检查

原审计虽然在 C03、C07、R03、R04、R07、R12、R16 等条目中零散识别了假安全开关、fixture 兼容、静默默认化和历史协议投影，但没有把“过度防御”作为独立维度沿生产调用链系统检查；因此此前不能视为已经完整覆盖。本次补查 Go API/Worker、Web、Agent、CLI、Helm 和 CI，并使用以下判据：

1. 输入来自仓内唯一、强类型或受 schema 约束的生产者，却仍猜测多种历史形态；
2. 启动装配已经保证依赖存在，业务方法仍允许 nil/缺能力并返回空成功；
3. 明确指定的对象、凭据或响应解析失败后，代码静默换值、回退或标记成功；
4. 同一配置在受控内部边界被重复默认化，导致非法值在更深层被“修好”；
5. 保护分支只有测试 fixture、未发布旧字段或无消费者元数据能够触发。

专项结果落入新增 C09～C13、R20～R22，并补强 D05、D11、D13、R03、R12、R16、R17、R19。主要模式如下：

| 模式 | 高置信证据 | 处置项 |
| --- | --- | --- |
| 测试便利泄漏到生产装配 | Worker 允许 nil DB；Agent runtime/executor 把生产必有依赖做成 optional | C09、R22 |
| 内部契约多形态猜测 | CLI 私有 workspace contract、Agent 工具错误体和前端 tool result 被逐层归一化 | D11、R12、R21 |
| 静默 fallback / 伪成功 | 显式构建凭据/目标查询失败后换值，Webhook 配置失败仍成功，2xx 非 JSON 被记为成功 | C10～C12 |
| 重复默认和无效元数据 | Worker 配置二次默认化；CLI Scope 被大量写入后统一清空 | C09、R19 |
| fail-safe 范围失控 | 已知 README/内部文档/报告变更触发全部质量门禁和 4 个镜像构建 | D13 |
| 未发布兼容残留 | 空删除状态、旧 Helm guard、双 interaction operation ID | C07、R07、R16 |

以下分支被逐项复核后明确排除，不能按“过度防御”删除：鉴权失败关闭、Secret/遥测脱敏、SSRF 与 DNS rebinding 防护、用户输入及外部网络 schema 校验、Kubernetes resourceVersion/NotFound 处理、卷 CAS/lease/事务、SSE/终端字节协议、超时与取消、原子文件写入、资源清理、外部协议版本兼容和有界重试。它们处理的是真实安全、并发、资源或不可信边界，详见第 9 节。

## 3. 建议执行顺序

| 批次 | 目标 | 原因 |
| --- | --- | --- |
| A | 死文件、死符号、坏重定向、死 CI 声明、未收集测试 | 证据闭合，产品行为不变 |
| B | 失效 i18n、无调用 Web API、无效配置项、过期文档 | 需要一次完整 Web/文档验证，但无需架构迁移 |
| C | `internal/service`、`cmd/tasks`、Worker/构建错误语义、Agent/Helm 小兼容层 | 会改变内部入口或失败终态，应各自单独提交 |
| D | API facade、Environment、网关域名、Builder 脚手架 | 跨 API/DB/Worker/Web/CLI，必须端到端重构 |
| E | Web 类型生成、Agent 工作流检索、CLI 生成物策略 | 先做小领域试点或构建方案，再决定全量迁移 |

## 4. 高置信直接清理

### D01. 删除重复的应用概览页面

- 证据：`web/src/pages/applications/application-overview-panel.tsx` 与 `web/src/pages/applications/overview/application-overview-panel.tsx` 均为 448 行，除一条相对 import 外相同。唯一生产入口 `web/src/pages/applications/ApplicationConfigPage.tsx:32` 只导入 `overview/` 版本。
- 动作：删除根目录副本。
- 收益：448 行、1 个文件；避免未来只修改其中一份。
- 影响：无运行时变化；风险低。
- 验证：全仓符号检索、Web lint/test/build。

### D02. 删除 `internal/service` 转发空壳

- 证据：`internal/service/git_policy.go`、`registry_policy.go` 的 5 个导出策略函数零调用；`common.go` 只服务这些死函数。`access_token.go` 的 5 个函数只转发 `internal/authz` 或 `internal/openapiscope`；生产调用位于 `internal/api/identityapi/session.go:179,192` 和 `access_token_handlers.go:48,132,136`，另有两个测试消费者，均可直连权威包。
- 动作：生产调用直连权威包，删除整个 `internal/service` 及重复包装测试。
- 收益：预计净减 150～190 行；消除第二套权限策略入口。
- 影响：无业务、权限或数据变化；风险低。
- 验证：`internal/authz`、`internal/openapiscope`、`identityapi`、`aitool` 和全量 Go 测试。

### D03. 给 i18n 增加反向门禁并删除 294 个失效 key

- 证据：`web/scripts/check-i18n.mjs:323` 只检查“引用缺失”和多语言一致性，没有检查“定义后未使用”。审计覆盖 `t()`、`i18next.t()`、`i18nKey`、模板前缀、动态 key 白名单及服务端/Agent key 生产者后，仍有 294 个高置信无引用 key。
- 主要分布：`deploymentsPage` 51、`apps` 49、`repositories` 48、`buildsPage` 46、`projectSpaces` 24、`dashboardPage` 18、`billingPage` 12，其余 46。
- 代表残留：`apps.ts:62` 的旧 Dockerfile/构建文案、`buildsPage.ts:3` 的旧 Build Provider CRUD、`repositories.ts:95` 的旧 Webhook 面板，以及 `accessTokens.title`、`accessTokens.listTitle` 等零散旧文案。
- 动作：先让检查器报告未使用 key，并只对白名单中的动态前缀放行；再按 namespace 分批删除。完整 key 清单见附录 A。
- 收益：五个 locale 至少 1,470 个叶子项，约 1,500 行。
- 影响：当前 UI 无变化；风险中，主要风险是漏掉运行时拼接 key。
- 验证：i18n 检查、Web lint/test/build、Agent interaction-card 契约测试。

### D04. 删除 18 个无生产调用的 Web API 方法及专属类型

- 证据：从应用入口构建调用图并排除 API 对象动态索引后，方法只在声明处出现；一个陈旧 test mock 不构成产品消费者。
- 分布：`web/src/api/domains/applications.ts` 2 个、`builds.ts` 4 个、`gateway.ts` 1 个、`git.ts` 2 个、`projects.ts` 4 个、`registries.ts` 2 个、`runtime.ts` 2 个、`volumes.ts` 1 个。完整方法名见附录 B。
- 连带类型：`HookRun`、`HookRunLog`、`BuildLog`、`GitFileContent`、`GitContentItem`、`ReleaseRuntimeExecResult`、流量结算类型和 transfer 查询 helper，集中于 `web/src/api/types.ts:450-1902` 与 `volume-types.ts:163`。
- 收益：预计净减 175～190 行。
- 影响：只删除 Web 未使用的封装；后端路由若仍供 CLI/Agent 使用必须保留。风险低至中。
- 验证：逐符号检索、API Client 测试、TypeScript 构建和相关页面测试。

### D05. 删除沿生产入口不可达的 Web 声明和不可能状态 fallback

- 不可达位置：`web/src/api/ai-types.ts:162`、`web/src/api/types.ts:31,1811`、`web/src/api/core.ts:38`、`web/src/lib/identifier-limits.ts:5`、`web/src/lib/telemetry.ts:155`、`web/src/pages/applications/deployments/application-deployments-panel-utils.ts:105`、`web/src/pages/code-repositories/code-repositories-form-model.ts:76`。其中部分类型会被另一条不可达声明引用，因此判据是“从生产入口不可达”，不是字面零引用。
- 只被自身测试调用：`brandThemeIsComposite`、`findSuggestedModelPrice`、`isPlatformRole`、`isProjectRole`。
- 不可能状态 fallback：`BuildVariableSet.variableCount` 在 `web/src/api/types.ts:1030` 被误写为 optional，页面再用 `item.variableCount ?? buildVariableCount(item.variables)`；但 Go 响应字段无 `omitempty` 且所有返回路径都赋值。`buildVariableCount` 为此甚至兼容服务端不再返回的旧式 `KEY=VALUE` 文本。
- 动作：删除声明及只验证这些死导出的测试；若内部实现仍被生产逻辑使用，只取消导出。把 `variableCount` 改为必填并直接渲染，删除仅服务该 fallback 的计数 helper；`buildVariableRecord` 仍有真实消费者，应保留。
- 收益：约 105～125 行。
- 影响：无产品行为；风险低。
- 验证：符号检索、Web lint/test/build。

### D06. 删除 8 个未引用 Web 静态资源

- 文件：`sso-platform-favicon.{png,svg}`、`sso-platform-illustration.{png,svg}`、`yuki-id-logo.{png,svg}`、`icons.svg`、`build-templates/icons/python.svg`。
- 证据：全仓无文件名或路径引用；当前 favicon 是 `web/index.html:5` 的 `luna-devops-logo.svg`，Python 选择器使用 `python.png`。删除 Python SVG 时同步删 `web/public/build-templates/icons/SOURCES.md:12` 的来源记录。
- 收益：439,545 bytes，约 429 KiB。
- 影响：当前 UI、README、文档无变化；仓库外直接访问裸静态 URL 的消费者不可由源码证明。风险低至中。
- 验证：逐 basename 检索、静态资源访问日志/发布约定核对和 Web 构建。

### D07. 删除 6 个未引用旧教程 SVG

- 文件：`docs/docs/public/guide/deploy-web-project/01-project-space.svg` 至 `06-gateway.svg`。
- 证据：均被跟踪，但全仓无 basename 引用。
- 收益：6 个文件、23,809 bytes。
- 影响：当前文档页面无变化；风险低。
- 验证：逐 basename 检索和文档构建。

### D08. 删除孤儿且口径错误的 `docs/billing.md`

- 证据：该文件 169 行，只由 `docs/README.md:5` 引用；Rspress 根是 `docs/rspress.config.ts:5` 指定的 `docs/docs`，因此它不发布。文件第 52～76 行仍按部署规格估算运行费用，与现行公开计费文档冲突。
- 动作：删除文件和 `docs/README.md` 链接；现行中英文计费页继续保留。
- 收益：约 172 行。
- 影响：不删除任何已发布页面，避免开发者读到错误口径；风险低。
- 验证：反向链接检索和文档构建。

### D09. 把 `TODO.md` 恢复为真实未完成清单

- 证据：`docs-internal/README.md:7-8` 和 `代码检查流程.md:77` 都规定完成记录由 Git 历史保存；`TODO.md` 453 行中至少有 35 个全完成章节、216 行纯历史。
- 漂移实例：`TODO.md:317` 把已实现的运行计费列为待办；`:345` 把已存在的构建镜像结果处理列为待办；`:394` 把已接入的实时日志列为待办；`:441` 仍规划 CLI PKCE，而当前 CLI 只支持 Device Code。
- 无承诺的未来构想：Temporal、Service Mesh 推断、外部 CI、长期记忆/外部 MCP，以及“这样的场景还有很多”等开放式愿望。
- 动作：删除完成历史、已实现/已替代项和没有负责人、入口、验收的 someday 条目；真实产品计划迁 Issue 或留下短期可验收 TODO。
- 收益：预计从 453 行压到 80～150 行，净减约 300 行。
- 影响：无运行时变化；会失去单文件历史浏览，但 Git 已保存。风险低。
- 验证：每个保留的 `[ ]` 都应有明确消费者和验收条件。

### D10. 修复未被 Vitest 收集的 Agent 测试

- 证据：`luna-agent/src/internal-secret.test.ts` 共 23 行；`luna-agent/vitest.config.ts:9` 只收集 `tests/**/*.test.ts`，`vitest list --filesOnly` 不包含该文件。
- 动作：将测试移入 `luna-agent/tests/`，而不是删除测试内容。
- 收益：不以减行数为目标；删除一个“看似有覆盖、实际不运行”的假象。
- 影响：CI 会新增一条真实执行的密钥派生契约；风险低，可能暴露已经存在的失败。
- 验证：Vitest 收集列表和 Agent test。

### D11. 删除 Agent 的内部导入与错误体兼容层

- 证据：`luna-agent/src/executor.ts` 只 re-export `executor/`；注释明确写着可移除。仅 `src/bootstrap.ts:3` 和一个测试仍使用旧入口。
- 同类证据：`luna-agent/src/tools/orchestrator.ts:438-453` 同时猜测 Luna API 错误体的 `body.code/retryable` 与 `body.error.code/retryable`。生产 client 只调用本仓 Go API，而 `internal/api/transport/response.go:164-205` 和 OpenAPI 都定义为平铺结构；Agent 自身嵌套错误体属于另一个服务器边界，不会进入这里。
- 动作：改两个 import 后删除 barrel；错误解析只接受权威平铺契约，并以契约测试锁定。不要把 Agent 自己的 HTTP 响应形态误当成 Luna API tool client 的兼容需求。
- 收益：约 8～10 行和两个历史分支。
- 影响：内部 import 路径改变；伪造嵌套 Luna API 错误体的仓外测试不再工作。风险低。
- 验证：Agent typecheck/test/build、Go error envelope → Agent tool client 契约测试。

### D12. 删除两个零调用 Router 构造重载

- 证据：`internal/api/router.go:19-25` 的 `NewRouterWithStaticFS`、`NewRouterWithStaticFSAndMetrics` 无调用；测试使用 `NewRouter`，生产使用完整 config 版本。
- 动作：保留 `NewRouter` 和 `NewRouterWithStaticFSAndMetricsConfig`，删除中间两层。
- 收益：约 8 行。
- 影响：仓库内无影响；风险低。若它们被作为外部 Go library API 使用则会破坏外部编译，但当前项目不是对外 Go SDK。
- 验证：全仓符号检索和 Go test。

### D13. 收紧 CI 的过宽 fail-safe，并删除死声明和虚假前置工具

- 证据：`.github/workflows/build-publish.yml:15-18` 的 4 个 `IMAGE_*` 全仓只有定义，镜像矩阵在第 241～264 行另行声明；`scripts/ci/detect-changes.sh:170-178` 匹配不存在的 `Dockerfile.api/worker/web/agent`，第 218 行匹配不存在的 `helm/`。
- 过宽 fail-safe：`detect-changes.sh:235-238` 对任何未匹配路径调用 `mark_all`；只读 dry-run 已确认 `README.md`、`docs-internal/README.md`、`reports/example.md` 都会打开 6 类质量门禁并构建 API、Worker、Agent、Gateway Probe 四个镜像。它把“真正未知路径”和“已知纯文本”混成一类。
- 虚假环境阻断：`scripts/release-check.sh:30-32` 强制要求 `rg`、`zip`、`unzip`，但完整 release 调用链没有使用这三个命令。
- 动作：删除死 env、旧路径和无调用工具要求；显式分类根说明、内部文档与报告，把“全部检查”和“全部镜像”拆开，只对真正未知的新根路径保留 fail-safe `mark_all`。真实入口 `Dockerfile`、`luna-agent/Dockerfile`、`charts/*` 必须保留。
- 收益：直接行数很小，但每次纯文本改动可避免最多 6 个无关质量 job 和 4 个镜像构建，同时减少虚假 release 环境阻断。
- 影响：CI 触发矩阵会改变；风险中，需防止未来的 embed/codegen 输入被误归为纯文本。
- 验证：为 README、内部文档、报告、未知根文件增加表驱动用例；运行 `bash scripts/ci/test-detect-changes.sh` 和 Workflow YAML 解析。

### D14. 删除或修正 6 条重定向后仍 404 的规则

- 证据：`docs/vercel.json` 的中英文规则分别指向不存在的 `start/first-project`、`download/installation`、`reference/configuration`。
- 动作：如果旧 URL 不再承诺兼容，删除 6 条；如果仍承诺，改到当前真实页面。不能继续保留“重定向成功、落点 404”。
- 收益：删除时约 30 行。
- 影响：删除会让旧地址直接 404，修正则恢复兼容；风险低。
- 验证：JSON 解析，逐项确认 destination 存在。

### D15. 保持 `release-compatibility.json` 已删除状态

- 证据：HEAD 中只有 8 行静态 CLI Skill 版本策略，全仓无引用；当前工作树已经删除。
- 动作：不要恢复。若仓库外发布系统消费它，应把契约接入仓内检查后再恢复，而不是保留不可验证文件。
- 影响：仓内无变化；外部消费者未知，风险低至中。

### D16. 删除 4 个未接入配置的 Web ESLint 直接依赖

- 证据：`web/package.json:57,68,70,73` 直接声明 `@eslint/js`、`eslint-plugin-react-hooks`、`globals`、`typescript-eslint`；`web/eslint.config.js` 只使用 `@antfu/eslint-config`，没有 import 这四个包。`pnpm why` 显示 Antfu 已带所需 globals 和 TypeScript ESLint parser/plugin，直接声明还引入了另一组版本。
- 动作：删除四个直接依赖并重新生成 lockfile；先后对比 `eslint --print-config`，确认规则集没有变化。
- 收益：manifest 4 行及 lockfile 中可达性消失的 Babel/Hermes/重复 ESLint 依赖树。
- 影响：按当前配置 lint 行为不变；风险低至中。
- 验证：clean install、print-config diff、lint、singleton check、test、build。

### D17. 删除 CLI 的 `authorization_code_pkce` 死类型分支

- 证据：`src/auth/oauth.ts:29` 的 `OAuthLoginMode` 仍声明 `authorization_code_pkce | device_code`；唯一生产调用 `src/commands/local.ts:104` 固定传 `device_code`，`beginOAuthLogin` 在 `oauth.ts:91-95` 对任何其他值直接报 `oauth_login_mode_unsupported`。
- 动作：删除 union 中的 PKCE 成员；如果 mode 不再有第二个实现，进一步删除内部 request 的 mode 字段。
- 收益：行数很小，但消除类型层“已支持浏览器 PKCE”的错误能力暗示。
- 影响：无现行行为；风险低。
- 验证：CLI auth typecheck/test/build。

## 5. 小范围能力、契约与入口收敛

### C01. 删除未发布、未文档化的 `cmd/tasks`

- 证据：`cmd/tasks/main.go` 只提供 Asynq `list-archived/run/delete`，目录 139 行；Dockerfile、Helm、Compose、Workflow、脚本和公开文档均无构建或使用入口。它还单独带出 `LoadTasks`、`TasksConfig`、`tasksEnvironment` 和测试。
- 动作：删除 `cmd/tasks/` 和专用配置链。
- 收益：预计净减 160～200 行。
- 影响：失去“从源码手工运行隐藏二进制来删除/重跑归档任务”的能力；正常 API/Worker 不受影响。风险中。
- 决策：如果运维实际依赖它，就正式发布、授权、审计并写文档；否则删除，不维持隐形破坏入口。
- 验证：Go test、API/Worker 镜像构建、Helm/Compose smoke。

### C02. 删除配置定义死文案和 3 个无效计费开关

- 证据：`internal/api/configs.go:25` 的 `Label`、`Description` 为 `json:"-"`；`ListConfigDefinitions` 只从 key 生成 i18n key，从不读取这些文案。`billing.freeQuotaCredits`、`billing.overdueGracePeriodHours`、`billing.allowNegativeBalance` 各自没有任何业务消费者；`billing.lowBalanceThresholdCredits` 有消费者，应保留。
- 动作：删除死字段/字面量；删除 3 个无效定义、五语言文案和数据库旧 key。
- 收益：约 100～160 行。
- 影响：管理员不再看到“可以保存但完全不生效”的设置；现行业务不变。风险中，因为配置 API/UI 会缩减。
- 验证：配置定义/更新 API、站点设置页、余额和构建/部署拦截测试。

### C03. 删除 CLI `api request` 的假开关，再决定是否保留诊断入口

- 证据：CLI 已宣称公开业务 API 由高层命令/协议命令覆盖，但仍在 `src/commands/local.ts:686-728` 注册任意同源 `/api/` 请求，并在 `src/commands/api.ts:169-202` 实现。文档和 Skill 又反复警告 Agent 不得使用它。
- 额外问题：`allowDiagnostic=true` 被元数据、示例和帮助描述成显式开关，但 handler 在 `local.ts:719` 直接丢弃 `_allow`，从未要求其为 true，属于无效安全仪式。
- 高置信动作：删除 `allowDiagnostic` 参数、文案和示例，或者真正要求其值为 `true`；当前状态不能保留。
- 产品决策：整个命令在 README 和 CLI spec 中被明确设计为 human-only 的新接口诊断入口，`agentAllowed:false`，限制同源 `/api/` 且仍经过后端 RBAC，因此不能判成死代码。如果产品决定不需要该入口，再删除命令、`ApiDiagnosticRequest`/`ApiPort.request` 和仅为 `additionalProperties:true` 服务的第二套参数解析分支。
- 收益：删除假开关很小；删除整个诊断能力预计超过 80～150 行源码/类型/测试/文档。
- 影响：只有删除整个命令时，人类才会失去 CLI 内的临时原始端点调试。风险中。
- 验证：CLI command catalog、帮助快照、覆盖率检查、test/build。

### C04. 精简 Agent README

- 证据：`luna-agent/README.md` 173 行中，第 3～14 行长期辩护框架选择，第 32～92 行镜像目录，第 94～161 行重复内部规格、根规范和 roadmap。第 62 行还指向不存在的 `openai-compatible.ts`，真实文件是 `openai-chat-completions.ts`。
- 动作：缩为用途、启动、测试、内部规格链接，约 30～50 行。
- 收益：约 110～140 行。
- 影响：丢失方便但易漂移的架构镜像；权威代码和内部规格保留。风险低。

### C05. 精简 CLI README 与 1,964 行规格文档

- 证据：CLI `README.md:215` 开始再放一份不完整英文说明，而 `README_EN.md` 已单独存在。`docs/cli-spec.md` 同时保存架构、实现快照、分期计划、发布 SOP 和验收历史；第 49 行说 Device Code 已实现，第 70 行又说未实现；第 47、85、101、992～1009、1384～1386、1460、1746、1841 行继续把 PKCE/loopback 写成支持目标或验收，而当前 `src/auth/oauth.ts:91-95` 明确只接受 `device_code`。第 1808～1868 行的 Phase 0～4 大量已完成事项仍以未来计划存在。
- 动作：中文 README 只链接 `README_EN.md`；CLI spec 只保留稳定架构与协议决策，命令参数以帮助为准、API 以 OpenAPI 为准、发布过程放贡献/发布文档，删除已完成 phase 和过期快照。
- 收益：README 约 94 行，CLI spec 预计超过 1,000 行。
- 影响：无运行时变化；降低新贡献者按过期设计实现 PKCE/Scope 的风险。风险低至中。

### C06. 合并 5 份完全相同的语言名称表

- 证据：`web/src/i18n/locales/*/languages.ts` 五份内容完全相同，语言名使用 endonym，不需要按 UI locale 翻译。
- 动作：改为一个共享资源或共享常量，保留 i18n namespace 接口。
- 收益：约 36 行。
- 影响：无 UI 变化；风险低。收益很小，排在结构清理之后。

### C07. 移除测试 fixture 专用的空删除状态兼容

- 证据：`internal/runtimecluster/state.go:17-24` 只为 legacy 内存 fixture 把空值当 `active`；注释明确持久化行已由迁移回填，不依赖该规则。
- 动作：所有 fixture 显式设置 `DeleteStatusActive`，然后删除空值 fallback 和对应兼容断言。
- 收益：少量代码，但收紧状态不变量。
- 影响：可能暴露遗漏初始化的测试或非数据库构造路径；风险低至中。

### C08. 把 README 专用大图移出 `web/public`

- 证据：`web/public/brand/mascot-luna-devops.png` 约 2.2 MB、`web/public/images/luna-devops-banner-v4.png` 约 2.0 MB，只被根中英文 README 引用。Vite 会复制整个 public，因此 `web/scripts/optimize-dist.mjs:10-17` 又专门在每次构建后删除它们。
- 动作：把两张 README 资产迁到不参与 Web public copy 的仓库位置，例如 `.github/assets/`，更新 README 引用，删除 post-build 专用排除项。
- 收益：仓库字节基本不变，但删掉一条“先复制再删除”的构建特殊分支，Web 构建输入边界更准确。
- 影响：README 图片路径改变；Web 产品无变化。风险低。

### C09. 用 Worker 装配不变量替代 nil DB no-op 和二次默认化

- 生产调用链：`cmd/worker/main.go:62-122` 先成功打开数据库，再把非空 DB 传给 `NewRunner`；`NewRunner(nil, ...)` 只出现在 Worker 测试和通知测试。生产入口不存在“无数据库继续运行”的合法模式。
- 过度防御：`internal/worker/worker.go:150-221` 仍允许 nil DB，多个账单、重协调、过期构建、通知和资源清理 handler 又在 DB 为空时直接返回 `nil`。这会把装配错误记录成任务成功，而不是提高可用性。`internal/runtimecluster/state.go:34-38` 的 `ActiveScope(nil)` 也没有实际保护作用，调用者随后继续 `.Where(...)` 仍会 panic。
- 重复默认：`internal/config/schema.go:61-70` 已声明环境默认，`internal/config/worker.go:17-74` 再完成校验；`NewRunner` 又为镜像、egress、cache tag、构建超时/TTL、部署超时和传输上限补第二套默认。尤其配置层允许 TTL 为 0，构造层却静默改成 3600，语义相互冲突。
- 相邻兼容残留：`internal/provider/kubernetes/volume_transfer_job.go:73-75` 仍导出旧名 `VolumeTransferJobProvider`；生产仅 Runner 字段使用旧名，当前实现和其他生产调用都已使用 `VolumeTransferProvider`。
- 动作：在构造/启动边界拒绝空核心依赖；测试注入最小真实依赖或显式 fake，不让测试便利形成生产降级态。配置默认值和合法性只由 `internal/config` 负责，Runner 接收已验证值；真正独立可选的能力使用窄依赖表达，不用 nil DB 控制。Runner 改用当前 provider 类型并删除旧 alias。
- 收益：预计净减 60～90 行，并把“任务伪成功”改为可观测的启动或执行失败。
- 影响：约 9 处 nil Runner 测试需要调整；TTL=0 必须先明确为“禁用”还是“非法”。风险中。
- 验证：Worker 构造测试、各 task 成功/失败终态、配置边界表格测试、Asynq 失败与重试 smoke。

### C10. 显式构建引用失败时停止静默换对象

- 凭据链路：`worker.handleBuildRun` → `ResolveBuildTask` → `registryCredentialForBuild`。`internal/buildruntime/runtime.go:177-185` 在显式 `CredentialRef` 查询失败后，会改选另一个 user/project/global push credential；不存在、不可见和数据库错误都被吞掉，可能以非预期身份推送镜像。
- 目标链路：同文件 `:79-85` 在非空 `DeploymentTargetID` 查询失败后忽略错误，再用零值 target 生成默认 stage/仓库地址，可能把镜像推到另一个位置。
- 动作：只有未指定 credential 时才执行自动选择；显式 credential 或 deployment target 查询失败必须返回稳定错误，并区分 not found/forbidden/数据库故障。删除零值对象 fallback。
- 收益：净减行数很小，但消除两个错误身份/错误目标的成功路径。
- 影响：此前依赖备用凭据或孤儿 target 侥幸成功的 BuildRun 会明确失败；风险中。
- 验证：显式引用不存在、无权限和 DB 错误契约；断言没有 registry push 副作用。

### C11. Webhook 自动配置不能吞错后报告启用成功

- 证据：`internal/api/gitapi/repository_handlers.go:200-230,233-286` 先保存 `WebhookEnabled=true` 的绑定，再调用 `tryConfigureRepositoryWebhook`；`:567-595` 的 wrapper 直接丢弃 Git client、上游创建、Secret 存储或二次保存错误，随后 API 仍返回 201/200。
- 动作：删除无信息的 `try*` 包装并明确一种产品语义：要么创建/补偿失败后返回失败，要么保存稳定的 `configuration_failed` 状态并向前端返回该状态。不能继续用布尔 `enabled` 表示尚未成功的外部副作用。
- 收益：包装本身约 3 行；正确的状态或补偿实现可能净增少量代码，但会删除“配置成功”的假象。
- 影响：涉及数据库与 Git Provider 双写；风险中。
- 验证：成功、Provider 失败、Secret 失败、DB 保存失败和补偿失败链路；检查响应、权威回读、审计与外部 webhook 副作用一致。

### C12. 2xx 非 JSON 响应不能被 Agent 记为工具成功

- 证据：`luna-agent/src/tools/luna-api-client.ts:46-55,178-180` 捕获任何 JSON 解析错误并返回 `{code: "invalid_json_response"}`；`tools/orchestrator.ts:335-352` 只按 HTTP status 判定终态。因此 200 HTML/截断 JSON 会带着错误对象进入 `succeeded`，合法 204/空正文也得到相同伪错误对象。
- 动作：先核对 OpenAPI 中允许无正文的成功操作；204/约定空正文映射为 `undefined`，其他 2xx 解析失败必须以稳定契约错误失败。非 2xx 仍可保留基于状态码的通用错误。
- 收益：行数约持平，但消除错误终态和污染模型上下文的伪结果。
- 影响：若某个现有 API 错误返回 2xx 非 JSON，将从假成功变为明确失败；风险中。
- 验证：补 200 malformed、200 HTML、204 和非 2xx malformed；断言 tool call 终态、审计和 trace status。

### C13. Helm 只在真正托管连接串时创建 connection Secret

- 证据：`charts/luna-devops/templates/secret.yaml:85-106` 的外层条件只要启用内置 Redis 就创建 connection Secret；当数据库使用 `externalDatabase.existingSecret`，Redis 同时使用 `redis.auth.existingSecret` 时，模板仍产出 `stringData` 为空的 Secret。实际 workload 又通过 `_helpers.tpl:121-147` 直接引用两个外部 Secret，因此该空对象没有消费者。
- 动作：先计算是否需要 chart 托管 database URL 或 Redis URL，至少一项为真才渲染 connection Secret；不要为穷举真值表再增加空对象兜底。
- 收益：代码约持平，但删除一个无消费者的 Kubernetes 对象并缩小 Secret 分支矩阵。
- 影响：只影响“两侧都由 existingSecret 提供”的组合；风险低。
- 验证：为内置/外置数据库与 Redis 的组合补 render matrix，并断言 workload 引用的 key 均存在。

## 6. 结构性过度设计：先替换，再删除

### R01. API 领域拆分留下了第二套 facade

- 规模：14 个 `internal/api/*_bridge.go` 共 4,566 行，`transport_compat.go` 94 行；bridge 内有 143 个 type alias，加上 transport compat 共 145 个；另有 357 个接收 `*gin.Context` 的 handler wrapper 和 14 个领域 Handler 工厂。路由表约 283 条，不能与 wrapper 数量混为一谈。
- Git 证据：引入领域拆分的提交 `0c21d0e9` 在 `internal/api` 改 245 个文件，新增 11,986 行、删除 1,159 行，净增 10,827 行；其中 bridge/handler/exports/compat 44 个文件一次新增 10,040 行。
- 结构问题：路由仍注册根 `Handlers` facade；转发时反复调用 `h.applicationAPI()`、`h.buildAPI()` 等构造领域 Handler。领域 `Host` 接口反向暴露根对象能力，`deploymentapi.Host` 44 个方法、`runtimeapi.Host` 42 个方法，已经不是窄依赖。
- 测试问题：根 `internal/api` 仍约 15,337 行测试，子领域包测试约 2,287 行，与“领域测试共置”的完成声明不一致。`source_files_test.go` 只禁止子包 import 根包，却通过 Host adapter/export alias 绕开真正边界。
- 目标结构：启动装配时每个领域 Handler 只构造一次；每个领域提供 route registrar 并直接注册；Handler 依赖窄业务 service/repository/provider，不依赖根 god object；根包只留全局 middleware、静态资源、跨域集成测试。
- 可删除边界：纯 route wrapper、测试 alias、`exports.go` 兼容导出、`transport_compat.go` 和不再需要的 Host adapter。
- 收益：保守预计净减 2,500～3,500 行生产胶水，并恢复真实领域边界。
- 影响：用户行为应不变，但回归面覆盖全部 HTTP、OAuth/RBAC、审计、SSE/WebSocket、OpenAPI；风险高。
- 验证：路由表与 OpenAPI operation 对齐、全量 Go 测试，以及认证、构建、部署、运行终端、卷和流式接口真实链路。

### R02. 删除不存在的 Environment 领域和重复 `environmentId`

- 证据：`internal/model/deployment.go:69` 定义完整 GORM `Environment`，但迁移没有 `environments` 表，API/OpenAPI 没有 Environment CRUD/schema。运行对象只是从 `DeploymentTarget` 临时复制成 Environment；`worker/kube_specs.go:134` 的 `deploymentNamespace(project, _ model.Environment)` 完全忽略它。
- 重复数据：`environment_id` 与已有 `deployment_target_id`/`stage` 并存于 deployment targets、gateway routes、hook runs、releases 四表和四个索引；空值时 Kubernetes label 又回退为 target ID，API 返回仍为空。
- 动作：API/Worker 直接传 `DeploymentTarget`；用 `deploymentTargetId`/`stage` 取代 `environmentId`；删除模型、转换器、桥方法、列、索引和重复 label。
- 收益：预计 200～400 行、4 列、4 索引和一个伪领域。
- 影响：公开 API、数据库和 Kubernetes label 改变；历史非空值需审计，现存资源需要重协调或短读取兼容期。风险高。

### R03. 删除只有一个真实实现的策略/Provider 字段

- `Project.NamespaceStrategy` 只存储/展示，OpenAPI 唯一值是 `project`，没有命名空间决策消费者。
- `RuntimeCluster.GatewayProvider` 唯一值是 `gateway-api`，normalizer 对任何输入都返回该值，Worker/Provider 不读取。
- `RuntimeCluster.Type` 声明 Kubernetes/K3s，但 normalizer 把 K3s 存为 Kubernetes，同时暗中接受 OpenAPI 未声明、系统无 Provider 的 `docker-compose`；运行路径再重复过滤 Kubernetes/K3s。
- 动作：固定“一项目一 Namespace”、Kubernetes-compatible 集群和 Gateway API；K3s 作为发行版信息，不作为策略类型。删除 3 列/API 字段/单选 UI/重复分支。
- 收益：预计 80～160 行。
- 影响：破坏性 API/DB 简化；迁移前统计现有值，处理任何 `docker-compose` 异常行。风险高。

### R04. 收敛网关域名的三层事实源

- 证据：`runtime_clusters` 同时保存 `gateway_root_domain` 和 `gateway_domain_suffixes`；写入时 `runtime_cluster_input.go:52` 混合多值、单值和隐藏全局 fallback，并把首项回写单值；Web 表单也提交两份相同事实。`gateway_route_domain.go:141` 再读取三层来源。
- 隐蔽行为：历史 `gateway.rootDomain`/`gateway.publicScheme` 并不在配置定义中，但 config cache 会把数据库未知 key 载入，仍可暗中改变行为。
- 动作：只保留 `gatewayDomainSuffixes`；多值为空时用旧单值一次回填，然后删除单值列和隐藏 config 行；证书默认后缀规则显式化。
- 收益：预计 80～150 行。
- 影响：域名和证书选择敏感；必须逐集群对比迁移前后的首选后缀。风险高。

### R05. 删除半实现的多 Builder/Lease/缓存/计量脚手架

- 证据：Builder task 的 `StreamID`、`LeaseToken`、`LeaseUntil`、`TargetBuilder` 无读写；BuildJob `Type` 永远为 `build`，`BuilderID` 无写入，lease 无获取/续租生产者，`LastHeartbeatAt` 只写不读。
- `BuildLabels` 只复制不参与调度；前端直接称其“为后续多构建能力预留”。`CacheConfig` 存储但 BuildKit 不消费。`CPUCoreSeconds`、`MemoryMBSeconds`、`CreditCost` 只存在于模型/Schema/TS，现值恒为零。
- 动作：删除未生效字段、索引、OpenAPI/TS/i18n；超时只使用 `started_at + build_timeout`。保留真实 attempts、BuildKit Job、日志和结果。
- 收益：预计 80～150 行和约 10 个列/索引。
- 影响：公开 API/DB 简化；先确认没有外部程序直接写这些列。风险高。

### R06. Web 与 OpenAPI 双维护 2,743 行 DTO

- 证据：`web/src/api/types.ts`、`ai-types.ts`、`topology-types.ts`、`volume-types.ts` 共 2,743 行、280 个导出声明；OpenAPI 同时维护 287 个 schema。`web/AGENTS.md:136` 要求人肉同步，仓库没有 Web TypeScript OpenAPI generator。
- 动作：先选择一个领域，从根 OpenAPI 生成不入库的 transport schema types；只保留 UI view model 和显式 adapter。通过 CI 生成/漂移检查维持契约。
- 收益：试点后预计净减 1,500～2,200 行手写类型。
- 影响：会暴露 OpenAPI 中的 nullable/union/缺失 schema，产生广泛类型改动；风险高。
- 验证：OpenAPI 校验、领域契约测试、全量 TypeScript test/build。

### R07. Agent 同时维护两套交互卡片 operation ID

- 证据：模型使用 `present_card`、`request_input`、`request_choice`，但 `luna-agent/src/executor/cards.ts` 为兼容 Web 全部持久化成已下线的 `create_interaction_cards`，并另存 `modelOperationId`。`model-history.ts:3-25` 又在模型历史中反向恢复真实 ID；Web 的 turns/timeline/display/state 和测试继续分支处理旧 ID。
- 动作：时间线直接保存真实 operation ID，或保存稳定 `interactionKind`；Web 直接识别三类当前操作。删除 `modelOperationId`、反投影和旧 ID 测试夹具。
- 收益：预计数十到一百余行，更重要的是删除双协议语义。
- 影响：Agent、Web、数据库历史共同改变；未发布历史卡片需要迁移或明确丢弃。风险高。

### R08. Agent 手写关键词路由重复工具检索机制

- 证据：`luna-agent/src/prompt/system.ts:22-36` 手工维护 13 组中英关键词、operation/route 信号；第 76～166 行自制打分，只加载最多 3 篇参考。与此同时系统 Prompt 已要求通过 `search_tools`/`get_tool_details` 的 Unicode/BM25 目录发现真实能力。
- 维护面：`luna-agent/skills` 有 1,631 行 Markdown；`system-prompt.test.ts` 对参考文案原句做断言，把测试耦合到说明文字。
- 动作：不要整包删除 Skill。先区分“工具 schema 可推导事实”和“跨工具流程不变量”；前者交给工具目录/详情，后者压缩成少量稳定参考或由同一检索器检索，删除关键词表和原句快照测试。
- 收益：可能减少数百至一千余行 Prompt/测试，并降低关键词漏匹配与文档漂移。
- 影响：可能降低复杂交付/诊断编排质量；先建立场景 eval 对照再切换。风险高。

### R09. CLI 生成契约的跟踪策略只能在重做构建链后调整

- 证据：`packages/api-contract/src/generated/operations.ts` 118,996 行、约 3.80 MB，是 `catalog.ts` 的运行时命令目录；`generated/schema.ts` 16,216 行、约 0.55 MB，是 `api-types.ts` 的编译期 `paths` 类型。两者都有真实消费者。
- 当前门禁：`verify-contract-drift.mjs` 会先读取两文件再生成；release quality gate 先做 drift 再 typecheck/test；binary job 直接 build 而不 generate。因此当前 fresh checkout/release 不能缺少它们。
- 动作：这是未来仓库策略，不是当前删除项。只有先改为 prepare/build/CI 确定生成并验证 fresh checkout、离线构建和发布后，才能停止跟踪；也可只跟踪紧凑 runtime catalog 与必要声明。
- 收益：减少 135,212 行代码审查噪声和约 4.35 MB 工作树文本。
- 影响：构建更依赖 codegen，IDE 在 bootstrap 前没有类型；离线构建和发布可重复性必须先解决。风险中至高。

### R10. 从 CLI 运行时 catalog 去掉无消费者的完整 `responses`

- 证据：`packages/api-contract/scripts/generate-operations.mjs:33-55` 先生成 `responses`，再从中派生 `outputSchema`/`errorSchema`；成品 `responses` 只在类型声明和测试 fixture 出现。CLI `src/commands/openapi.ts:100-166` 只消费 parameters、requestBody、inputSchema、outputSchema、errorSchema，从不读取完整 responses。
- 体积：当前 302 项 operations JSON 约 3,800,631 bytes；生成阶段派生 schema 后不落 `responses`，估算降至 1,494,186 bytes，净减 2,306,445 bytes，约 61%。它当前还参与 `OPERATION_CATALOG` digest 并被 tsup 打入约 4 MiB 的 CLI dist。
- 动作：生成阶段仍读取 OpenAPI responses，用它派生必要输出/错误 schema，但不把完整 responses 写入运行时 operation entry；同步调整类型、digest fixture 和契约测试。
- 收益：是真实发布体积、启动解析和审查噪声降低，不只是停止跟踪生成文件。
- 影响：catalog digest 会改变；依赖 digest 的 Skill/版本检查需同步。风险中。

### R11. CLI 运行时快照不应复制已经忽略的 `requiredScopes`

- 证据：Scope 简化后，CLI 已从 catalog、OpenAPI command、help、registry 和 preflight 删除 scope 消费，但 `packages/api-contract/src/types.ts:122` 和 generator 仍把平台 OpenAPI 的 256 处 `x-luna-cli.requiredScopes` 原样写入运行时快照，约 15,689 bytes。
- 动作：平台 OpenAPI 中的 `requiredScopes` 必须保留，PAT、Agent tool catalog 和服务端授权仍使用它；只在 CLI generator 的投影白名单中排除该字段。
- 收益：完成第一方 CLI scope 简化闭环，缩小运行时 catalog 和错误概念面。
- 影响：CLI catalog digest 改变；平台授权契约不变。风险低至中。

### R12. 删除私有 workspace contract 的虚构多形态兼容

- 证据：忽略目录 `cli/luna-cli` 中，`src/entry.ts` 始终把静态 `* as apiContract` 传入 command registration；`src/commands/openapi.ts:29-178` 却兼容 `OPERATION_CATALOG`、`commandCatalog`、`COMMAND_CATALOG`、`cliCommandCatalog`、`default`，函数/数组、`commands`/`entries`，三处 metadata，以及 command/`xLunaCli`/`x-luna-cli`/cli 多层字段。`@luna-devops/api-contract` 是 private workspace 包，固定导出 `OPERATION_CATALOG`/`OPERATION_CATALOG_METADATA`，并被根 build 直接打包，没有第二个生产实现；现有测试也只构造 canonical 形态。
- 动作：以强类型直接消费唯一权威导出；保留真正必要的 OpenAPI→CLI normalization，删除模块形态猜测和固化虚构形态的测试。
- 收益：预计净减 30～50 行和约 20 个 fallback 分支，让契约不匹配在编译/启动时直接失败。
- 影响：若仓外有人直接导入未发布的 private API 会受影响；仓内风险低至中。

### R13. 规范、README、SOP 形成多重事实源

- 证据：根 `AGENTS.md` 250 行、`web/AGENTS.md` 427 行、`docs-internal/代码检查流程.md` 279 行，加上 Web README/CONTRIBUTING 合计超过千行。`@/` import、单例检查、i18n、DataList、验证命令等同一规则出现 3～5 次；Web README 与 Web AGENTS 还互相要求必读。
- 动作：根 AGENTS 只留跨域硬约束；Web AGENTS 只留前端增量；SOP 只留审计频率、分类和报告模板；README 只留运行入口；CONTRIBUTING 链接权威规则。先做规则账本，再逐条合并，不能整文件粗删。
- 收益：预计 300～450 行。
- 影响：无运行时变化，但误删唯一安全/验收约束会影响后续工程质量；风险中。

### R14. `SHADCN_COMPONENTS.md` 是人工复制目录和愿望单

- 证据：第 28～90 行复制官网组件清单，第 94～120 行保存完成历史和未来优先级，第 133～145 行列尚无消费者的愿望；真实已安装组件由 `web/src/components/ui` 和 `components.json` 决定。
- 动作：把第 7～27、122～131 行中项目独有规则合入 Web AGENTS，随后删文件和引用。
- 收益：约 120～150 行。
- 影响：丢失人工 roadmap，不丢产品能力；风险低至中。

### R15. Helm 首管同时支持 Secret 和 inline 密码

- 证据：`charts/luna-devops/values.yaml:34-46` 同时提供 `existingSecret` 与 email/name/password；README 明示 inline 密码会进入 Helm values/release history。`secret.yaml`、render tests 和中英文文档为两条路径维护大量分支。
- 动作：只保留 `existingSecret`，安装文档先展示创建 Secret，再安装 Chart。
- 收益：约 80～100 行，并删除一条不安全配置路径。
- 影响：首次安装多一步创建 Secret；旧 values 不再工作。风险中。

### R16. 删除未发布 Helm 字段的反向兼容守卫

- 证据：`charts/luna-devops/templates/_helpers.tpl:159-162` 专门拒绝已不存在的 `app.extraEnv`；render test 和 README 继续维护该历史。
- 相邻残留：同文件 managed env 列表中的 `AI_AGENT_ADDR` 没有现行配置或应用消费者，只有 render test 与 Go 配置契约的“必须不存在”断言为它自证。
- 动作：按项目“不保留未发布兼容层”的规则，删除 `app.extraEnv` guard、文档和测试；同时删除无消费者的 `AI_AGENT_ADDR` managed-list 项及空断言。数据库、Redis、OTel、Secret 和首管配置的覆盖保护有真实安全职责，必须保留。
- 收益：约 10～15 行。
- 影响：旧 values 中该字段会被 Helm 忽略，不再得到定制错误；风险低至中。

### R17. 更新日志镜像和自动写回链只复制 GitHub Release

- 证据：中英文 changelog 页面自己声明 GitHub Release 是事实源，顶级 `_nav.json` 没有入口；`changelog-sync.yml` 仍在任意 main/dev/tag Build & Publish 成功后申请写 token、fetch tags、生成页面、提交并 push main，再手动触发 build workflow。生成器唯一输入是 Git tags，所以普通 push 多数只完成一轮有权限的 no-op。
- 放大链：真正写回时，App token push 本身会触发 main workflow，随后手动 dispatch 又让 change detector 强制 `mark_all`；结果可能重复全量门禁、构建四个镜像，并再触发两次 changelog no-op。`release-check.sh` 还在生成 changelog 后只 build、不检查 Git diff，陈旧页面可被悄悄改写后仍显示检查通过。
- 动作：如果产品不需要站内更新日志，删除整链并直接链接 GitHub Releases；如果需要，构建时读取 release feed，不把生成结果反向提交主分支。
- 收益：直接删除约 222 行和一个有写权限的 Workflow，同时移除普通 push 的无效特权 job 与写回后的重复全量构建。
- 影响：站内直达 URL 消失或改为外链；风险中，需确认是否已有外部链接。

### R18. 两份 Grafana Dashboard 没有交付链

- 证据：`grafana/dashboards/luna-agent-llm-observability.json` 793 行、`luna-devops-overview.json` 2,242 行；没有 provisioning、Compose/Helm mount、公开导入说明或文件名引用。它们只是随根目录进入 Docker source context。
- 动作：二选一：正式提供 provisioning/导入文档并验证查询；否则删除，不保留隐藏资产。
- 收益：删除时 3,035 行、约 77.5 KiB。
- 影响：不再提供仓库内手工导入 Dashboard；现有运行/部署不变。风险中。

### R19. 完成第一方 CLI Scope 元数据和文案的移除

- 证据：当前产品和 CLI 文档已规定第一方 CLI 登录后与用户权限对等，不再请求独立 Scope；但忽略目录 `cli/luna-cli` 中的 `CommandMetadata`/`NormalizedCommandMetadata` 仍定义 `scopes`，协议与卷传输 helper 继续写入几十处 Scope，`config/resolve.ts` 还为 token 填 `scopes: []`。生产侧没有任何 `.scopes` 读取者，`registry.ts:159-170` 反而在 normalize 时无条件把它覆盖为空；只有测试继续读取并证明这些无效元数据。CLI README、Skill security reference 和公开 workflow 文档仍要求扩 Scope 后重登。
- 动作：从 CLI metadata、protocol wrapper、volume helper、测试自证、帮助/i18n、README/Skill 和公开文档移除第一方 CLI Scope。平台 OpenAPI 的 `requiredScopes`、PAT、第三方 OAuth、Agent 工具与服务身份授权仍有真实消费者，必须保留；外部 OAuth 标准 `invalid_scope` 错误映射也不在删除范围。
- 收益：预计净减 50～80 行并消除约 80 个误导性 Scope 命中。
- 影响：第一方 CLI 行为不变，测试与帮助契约会缩小；风险低至中。

### R20. Agent 工具目录对内部契约应 fail closed，并且只构建一次

- 契约证据：Go `internal/aitool/openapi_catalog.go:17-40` 对 `name`、`summary`、`tags`、语义说明、`requiresApproval`、请求和输出 schema 等字段都有唯一生产逻辑并稳定序列化；JSON 契约测试也要求这些字段存在。Agent `luna-agent/src/tools/catalog.ts:27-62` 却把其中大量字段设为 optional/default，并再次生成 name、summary、tags、purpose，甚至把缺失的 `requiresApproval` 默认为 `false`。
- 风险：如果权威 producer 漏字段，Agent 不会拒绝配置，而会继续用自己推测的语义；`requiresApproval=false` 还可能把契约破坏降级为无需审批，属于危险的内部 fail-open。
- 重复构建：`bootstrap.ts:47-66` 先在 `RemoteConfigSnapshot` validator 中 `ToolCatalog.load`，随后立即再次 load；每次 refresh 也先校验，再由 `ToolCatalogRegistry.refresh` 重新 parse、排序、计算 digest 并构建 BM25 索引。
- 动作：Go producer 明确拥有默认化，Agent 对安全和语义必填字段严格校验；真正允许省略的字段保持窄 optional。把一次解析得到的不可变 `ToolCatalog` 作为候选配置的一部分原子发布，不在 validator 与 registry 中构建两次。
- 收益：预计净减 15～30 行 fallback，并避免每次配置刷新重复构建完整检索索引。
- 影响：旧或损坏的配置会在初始化/刷新时被拒绝并保留最后有效快照；风险中。
- 验证：Go JSON → Agent schema 契约、缺 `requiresApproval`/语义字段必须失败、刷新原子性、digest snapshot 与在途 run catalog 保留测试。

### R21. Tool result 只在协议入口校验一次，删除前端二次修复和旧回捞

- 证据：Agent `timeline-presenter.ts:135-205` 已把工具结果稳定投影为 `AIToolDisplayResult`；前端 `api/ai-types.ts:56-113` 也声明了同一结构。但 `web/src/components/common/ai-assistant/state.ts:641-674` 又从 `unknown` 逐字段归一，缺 `summaryKey` 时伪造 completed；`:127-154` 还从独立 `tool_result.parts.structured_data` 回捞旧结果。
- 生产链路：`luna-agent/src/tools/postgres-store.ts:55-83` 把 result 写进 `tool_call` 内容；当前 presenter 会随 `toolCall.result` 返回。Agent 源码没有 `structured_data` 生产者，旧回捞只由前端测试 fixture 触发。
- 动作：在 SSE/HTTP ingress 用一次 schema 校验约束不可信网络数据，reducer 接收已验证的 typed result；删除 `normalizeToolResult`、`results` map、`structuredResult` fallback 和只证明旧形态的 fixture。不要删除模型输入、UI action 或敏感字段本身的校验。
- 收益：预计净减 32～42 行生产代码及旧 fixture，消除“Agent 投影一次、Web 再猜一次”的双协议。
- 影响：损坏 payload 会在协议入口进入明确错误/重同步，而不是被 reducer 修成完成结果；风险低至中。
- 验证：用 presenter 真实 fixture 驱动 timeline 与 SSE reducer；覆盖 malformed payload、重放和重同步。

### R22. Agent 生产依赖不应为了测试被设计成三种 resolver 和多组 optional

- 证据：生产 `bootstrap.ts:135-158` 始终给 `ModelRuntime` 传完整 `{resolve, search, details}` registry，并给 `RunExecutor`/`buildServer` 传 tools、remote config、catalog registry、stream bus、cancel 与 digest。`provider/provider.ts:47-71` 和 `model-runtime.ts:62-146` 仍兼容数组、函数、registry 三种 resolver；缺少 search/details 时返回空结果，把误装配伪装成“没有工具”。
- 同类问题：`executor/index.ts:38-54` 与 `server.ts:44-70` 把同一组生产必需依赖声明为 optional/default；readiness 在完全没有 remote config 时反而视 provider 为可用。`RunExecutor` 的 `Config` 参数只被 `void config` 消耗。这些分支主要服务单元测试构造，不是产品运行模式。
- 动作：生产构造只接受完整 `ModelToolRegistry` 和明确的必需依赖；测试通过统一 `testRegistry`/server fixture 注入 fake。删除空工具、无 remote config 的伪降级态和未使用 Config 参数；若确需测试无配置 readiness，应直接测试启动失败契约。
- 收益：预计净减 45～75 行生产分支，测试 fixture 可能增加 10～20 行，但依赖图更真实。
- 影响：Agent 内部构造签名和大量测试会调整；生产行为只会从静默降级改为启动失败。风险中。
- 验证：Agent typecheck/test/build，启动装配、readiness、cancel、SSE、工具 search/details 和无效配置失败链路。

## 7. 门禁满足后再删除

### G01. `retained_volumes` 与部署旧存储列

- 证据：`internal/model/retained_volume.go` 明确仅供旧 backfill；生产 API/Worker/Provider 不使用。Deployment 上 7 个旧存储字段同样只为回填保留。
- 当前不能删的原因：不可逆数据删除，可能仍有未映射到 `project_volumes`/`deployment_volume_mounts` 的 PVC 历史或归属。
- 动作：完成真实数据快照、逐行映射、备份恢复演练后，一次 migration 删除旧表/列，再删模型和负向兼容测试。
- 收益：约 60～120 行；如果未发布环境允许重写 baseline，约 120～180 行。
- 影响：数据风险高。

### G02. 是否重做迁移 baseline

- 现状：迁移 67 是项目文档明确的最早支持版本；68～97 包含多轮未发布功能增删，包括卷、AI budget、delivery queue、kubectl 与 CLI scope。
- 判断：它看起来冗长，但当前是已声明的升级契约，不是垃圾。只有在确认所有持久环境可重建、正式抬高最早支持版本后，才能把最新 schema 重新 baseline。
- 收益：可显著缩短迁移和兼容测试；不代表生产代码减少。
- 影响：旧数据库不能原地升级，风险高。

## 8. 本地忽略产物与独立工作树

这些都被主仓忽略，但“被忽略”不等于“可删除”。本报告未删除任何一项：

| 路径 | 体积/状态 | 影响 |
| --- | ---: | --- |
| `cli/luna-cli` | 约 217 MB | **不可直接删除**：该独立仓库 `main` 比 `origin/main` 领先 1 个唯一提交（`7b6d8d1`），当前还有 39 个工作树路径变更；相邻 `../luna-cli` 停在 `8d71f9e`，尚不包含这些内容。必须先合并/迁移提交和未提交改动，再决定删除 clone |
| `cli/luna-cli/node_modules` | 约 206 MB | 可由 lockfile 重新安装；只清该依赖目录不会丢失 CLI 源码改动 |
| `web/dist` | 约 12 MB | 可再生成构建产物 |
| `luna-agent/dist` | 约 2.4 MB | 可再生成构建产物 |
| `docs/doc_build` | 约 3.5 MB | 可再生成文档产物 |
| `.local/builder-workspace` | 空目录 | 本地残留 |
| `internal/kubeaccess`、`kubecatalog`、`kubepolicy`、`kubeproxy`、`runtimecommand` | 空目录 | 已删除功能留下的目录壳 |
| `web/src/pages/access-tokens`、`web/src/pages/projects/kubectl` | 空目录 | 页面迁移/功能删除残留 |
| `luna-agent/sql`、`src/tools/generated`、`tests/fixtures` | 空目录 | 本地目录壳 |

## 9. 明确保留，避免误删

- 根 OpenAPI、各 pnpm lockfile、应用市场模板和 AI interaction-card contract：都有真实生成、发布或跨进程消费者。
- `luna-agent/src/run-stream-bus.ts` 与 `run-stream-hub.ts`：前者负责生产/持久提交，后者负责读取/PG 观察/SSE fanout/backpressure，不是重复实现。
- Agent 写库与 Go `agentobservability` 读模型：分别服务权威写入和管理端聚合读取；除非删除整个 Operations 可观测产品，不应按文件相似度合并。
- 文档站和 Web 各自的 logo/mascot：属于两个独立静态构建根；为约 164 KiB 再引入复制脚本不划算。
- 鉴权与网络安全：`internal/authz` 的失败关闭、Secret/Audit/RBAC、SSRF/DNS rebinding、受信代理、敏感遥测脱敏都必须保留；这些分支面对不可信输入或权限边界，不是内部兼容胶水。
- Kubernetes 与卷：kubeconfig 写入和使用前的双边校验、动态对象 type/nil guard、resourceVersion 冲突、NotFound 幂等删除、卷 CAS/lease/事务、单调进度和流传输终态唯一提交都有真实并发或外部状态职责。
- Web 实时与模型边界：SSE envelope/sequence/replay、终端字节和控制帧、模型生成 UI action/内部 URL 白名单、浏览器存储异常与用户 kubeconfig YAML 校验必须保留。
- Agent：取消、CAS 终态、stream cleanup、敏感工具参数、外部 Provider/config schema、超时、有界重试，以及刷新失败时保留最后有效配置都必须保留；Tempo v1/v2 兼容也对应真实外部协议。
- CLI 卷传输：checksum/byte limit、`O_NOFOLLOW`、`.part` 原子 rename、AbortSignal、finally/close/cancel，以及 OAuth refresh 并发控制和禁止 redirect 转发凭据均是文件、网络和中断边界的必要防御。
- Helm/Compose/CI：Secret/数据库/Redis/OTel env 覆盖保护、首管输入约束、随机密钥复用、checksum rollout、NetworkPolicy/securityContext、readiness/healthcheck、数据库隔离、race 检查和发布幂等应保留。
- `scripts/ci/*.sh` 整体：除了本报告点名的死分支，其余均有 Workflow/release 脚本调用。
- `.env.example`：是 Compose 和本地启动的可执行配置入口，变量均有消费者。
- 当前公开文档中的 `kubectl`/`kubeconfig`：剩余内容用于平台安装、管理员配置运行集群或安全警告，不等于已删除的终端用户 kubectl 兼容功能。

## 10. 验证矩阵

| 改动类型 | 最小验证 |
| --- | --- |
| Web 死代码/i18n/API Client | i18n 反向检查、lint、test、build、singleton check |
| Go 空壳/Router/config/cmd | 相关包测试 + `go test ./...`；配置/权限项需真实 API 回读 |
| Worker 装配/默认值 | 非空依赖构造、配置边界表格测试、Asynq 成功/失败/重试终态、关键任务不得 nil-success |
| 内部契约 fail-closed | producer JSON → consumer schema 契约；缺安全字段、malformed 2xx、显式引用失效均必须失败且无副作用 |
| API facade | 路由/OpenAPI 契约 + 全量 Go + OAuth/RBAC/审计/SSE/WebSocket/业务 E2E |
| 数据模型/迁移 | 67→最新、fresh schema、非空 fixture、备份恢复、API/Worker/Web/CLI 契约 |
| Agent 协议/Prompt | lint/typecheck/test/build、交互卡片 E2E、复杂场景 eval、历史回放 |
| 文档/redirect/changelog | 中英文导航与链接、`pnpm --dir docs build`、Vercel route smoke、Workflow YAML |
| Helm | render tests、敏感值不进入 release history、安装/升级 smoke |
| CLI | contract generation/coverage、help/catalog snapshot、test/build、fresh checkout release |

## 附录 A：294 个高置信未使用 i18n key

删除前仍应由新增反向检查器在当时的工作树重新生成，避免并行改动使清单过时。

### accessTokens（2）

```text
accessTokens.title
accessTokens.listTitle
```

### accountPage（1）

```text
accountPage.description
```

### aiAssistant（2）

```text
aiAssistant.options.selected
aiAssistant.options.unavailable
```

### appTemplatesPage（4）

```text
appTemplatesPage.description
appTemplatesPage.loading
appTemplatesPage.port
appTemplatesPage.resources
```

### apps（49）

```text
apps.dockerfile
apps.dockerfileHint
apps.dockerfileRequired
apps.dockerfilePlaceholder
apps.selectDockerfile
apps.noDockerfilesDetected
apps.buildContext
apps.buildContextHint
apps.buildContextRequired
apps.buildContextPlaceholder
apps.selectBuildContext
apps.targetImageRef
apps.targetImageRefHint
apps.targetImageRefPlaceholder
apps.buildConfigsDescription
apps.deploymentTargetEntries
apps.deploymentTargetEntriesDescription
apps.createBuildConfig
apps.editBuildConfig
apps.buildConfigDialogDescription
apps.buildConfigCreated
apps.buildConfigUpdated
apps.buildConfigDeleted
apps.deleteBuildConfigTitle
apps.deleteBuildConfigDescription
apps.manageInApplication
apps.templateVariablesTitle
apps.templateDocs
apps.templateVariablesDescription
apps.templateVariablesColumn
apps.templateVariableUsageColumn
apps.templateVariableExampleColumn
apps.templateVariables.fullSha
apps.templateVariables.shortSha
apps.templateVariables.refName
apps.templateVariables.refType
apps.templateVariables.ref
apps.buildLabels
apps.buildLabelsHint
apps.buildLabelsPlaceholder
apps.buildHooksEnabled
apps.buildRepositoryBinding
apps.buildRepositoryBindingHint
apps.integerPort
apps.positivePort
apps.deleted
apps.openConfigAria
apps.bindRepository
apps.detailDescription
```

### auth（1）

```text
auth.backToLogin
```

### billingPage（12）

```text
billingPage.monthSpend
billingPage.monthlyCategoriesTitle
billingPage.monthlyCategoriesDescription
billingPage.emptyMonthlyCategories
billingPage.periodCategoriesDescription
billingPage.selectedPeriod
billingPage.filters
billingPage.scopeTitle
billingPage.selectProject
billingPage.allRelatedProjects
billingPage.allProjectSpaces
billingPage.selectedProjects
```

### buildsPage（46）

```text
buildsPage.description
buildsPage.providers
buildsPage.providerNameRequired
buildsPage.providerSlugRequired
buildsPage.providerSlug
buildsPage.providerSlugHint
buildsPage.providerDisplayName
buildsPage.providerDisplayNameHint
buildsPage.variableSets
buildsPage.runs
buildsPage.jobs
buildsPage.createProvider
buildsPage.editProvider
buildsPage.providerDialogDescription
buildsPage.providerCreated
buildsPage.providerUpdated
buildsPage.providerDeleted
buildsPage.deleteProviderTitle
buildsPage.deleteProviderDescription
buildsPage.emptyProviders
buildsPage.variableSetsHint
buildsPage.selectProject
buildsPage.commitLabel
buildsPage.pushedBy
buildsPage.currentStep
buildsPage.relativeHoursMinutesAgo
buildsPage.relativeMinutesAgo
buildsPage.relativeJustNow
buildsPage.durationHoursMinutesSeconds
buildsPage.durationMinutesSeconds
buildsPage.emptyJobs
buildsPage.provider
buildsPage.targetRepository
buildsPage.targetTag
buildsPage.targetTagPlaceholder
buildsPage.targetTagTemplateHint
buildsPage.targetImagePreview
buildsPage.targetImagePreviewHint
buildsPage.sourceCommit
buildsPage.inheritedBuildConfigHint
buildsPage.buildRun
buildsPage.attempts
buildsPage.buildLogs
buildsPage.emptyLogs
buildsPage.config
buildsPage.typePlatform
```

### codeRepositoriesPage（1）

```text
codeRepositoriesPage.description
```

### dashboardPage（18）

```text
dashboardPage.description
dashboardPage.heading
dashboardPage.subtitle
dashboardPage.workOverview
dashboardPage.projectSample
dashboardPage.projects
dashboardPage.applications
dashboardPage.failedBuilds
dashboardPage.unhealthyClusters
dashboardPage.noIssues
dashboardPage.recentBuilds
dashboardPage.recentBuildsDescription
dashboardPage.buildMeta
dashboardPage.noBuilds
dashboardPage.availableGlobalAndScoped
dashboardPage.projectOverview
dashboardPage.projectOverviewDescription
dashboardPage.noBuild
```

### deploymentsPage（51）

```text
deploymentsPage.description
deploymentsPage.clusters
deploymentsPage.releases
deploymentsPage.clusterDialogDescription
deploymentsPage.emptyReleases
deploymentsPage.buildRun
deploymentsPage.buildRunHint
deploymentsPage.deploymentTargets
deploymentsPage.deploymentTargetsDescription
deploymentsPage.bundleImport.invalidStage
deploymentsPage.deploymentConfigs
deploymentsPage.deploymentConfigsDescription
deploymentsPage.saveBuildConfigBeforeDeploymentConfig
deploymentsPage.saveModuleBeforeDeploymentConfig
deploymentsPage.createDeploymentConfig
deploymentsPage.editDeploymentConfig
deploymentsPage.deploymentConfigDialogDescription
deploymentsPage.environmentSlugHint
deploymentsPage.deleteDeploymentTargetTitle
deploymentsPage.deleteDeploymentTargetDescription
deploymentsPage.emptyDeploymentConfigs
deploymentsPage.emptyDeploymentConfigsDescription
deploymentsPage.emptyDeploymentTargets
deploymentsPage.targetCount
deploymentsPage.requireApproval
deploymentsPage.servicePortHint
deploymentsPage.runtimeSecretValueRequired
deploymentsPage.sourceBranch
deploymentsPage.internalEndpoint
deploymentsPage.internalEndpointHint
deploymentsPage.terminalStdout
deploymentsPage.terminalStderr
deploymentsPage.command
deploymentsPage.runCommand
deploymentsPage.emptyStdout
deploymentsPage.metricsUnavailable
deploymentsPage.runtimeProfile
deploymentsPage.cpuUnits.m
deploymentsPage.kubernetesAdvancedStorage
deploymentsPage.kubernetesValues.ClusterIP
deploymentsPage.kubernetesValues.NodePort
deploymentsPage.kubernetesValues.LoadBalancer
deploymentsPage.kubernetesValues.Cluster
deploymentsPage.kubernetesValues.Local
deploymentsPage.dataVolumeSourceProjectVolume
deploymentsPage.clusterHint
deploymentsPage.defaultCluster
deploymentsPage.autoNamespace
deploymentsPage.endpoint
deploymentsPage.typeK3s
deploymentsPage.typeDockerCompose
```

### eventsPage（5）

```text
eventsPage.description
eventsPage.listTitle
eventsPage.filters.scope
eventsPage.scopes.mine
eventsPage.scopes.all
```

### gatewayRoutesPage（6）

```text
gatewayRoutesPage.description
gatewayRoutesPage.cnameTarget
gatewayRoutesPage.optionalGatewayConfig
gatewayRoutesPage.optionalGatewayConfigDescription
gatewayRoutesPage.enableGatewayConfig
gatewayRoutesPage.gatewayRouteWillUpdate
```

### inbox（2）

```text
inbox.description
inbox.detail.requestStatus
```

### notificationsPage（1）

```text
notificationsPage.description
```

### projectHooks（3）

```text
projectHooks.runOrder
projectHooks.dragHint
projectHooks.orderUpdated
```

### projectMembers（3）

```text
projectMembers.searchingUsers
projectMembers.selectedUsers
projectMembers.removeSelectedUser
```

### projectSpaces（24）

```text
projectSpaces.listTitle
projectSpaces.editAria
projectSpaces.deleteAria
projectSpaces.repositories
projectSpaces.workspaceDescription
projectSpaces.repositoryBindingsEntry
projectSpaces.quickActions
projectSpaces.repositoriesPreviewDescription
projectSpaces.demoPanelTitle
projectSpaces.demoPanelDescription
projectSpaces.openFullPage
projectSpaces.allProjects
projectSpaces.selectedProjectCount
projectSpaces.expandProjects
projectSpaces.collapseProjects
projectSpaces.expandProjectApps
projectSpaces.collapseProjectApps
projectSpaces.pinProject
projectSpaces.unpinProject
projectSpaces.emptyCompact
projectSpaces.scope
projectSpaces.scopeOptions.related
projectSpaces.scopeOptions.all
projectSpaces.neverUsed
```

### projectTopology（8）

```text
projectTopology.graphView
projectTopology.fit
projectTopology.generatedAt
projectTopology.deploymentTarget
projectTopology.unknownDeploymentTarget
projectTopology.chart.focusRelation
projectTopology.form.serviceBindingDescription
projectTopology.form.sameApplicationAllowed
```

### registriesPage（2）

```text
registriesPage.selectProject
registriesPage.listTitle
```

### repositories（48）

```text
repositories.title
repositories.description
repositories.providerCreated
repositories.accountCreated
repositories.accountRefreshed
repositories.webhookReconfigured
repositories.providerNameRequired
repositories.providerRequired
repositories.usernameRequired
repositories.providerTitle
repositories.name
repositories.type
repositories.github
repositories.gitea
repositories.gitlab
repositories.serviceUrl
repositories.serviceUrlPlaceholder
repositories.authType
repositories.oauth
repositories.githubApp
repositories.clientId
repositories.scopes
repositories.patFallback
repositories.createProvider
repositories.providerNamePlaceholder
repositories.connectAccount
repositories.oauthConnect
repositories.provider
repositories.selectProvider
repositories.username
repositories.externalUserId
repositories.status
repositories.saveAccount
repositories.refreshAccount
repositories.saveBeforeWebhook
repositories.reconfigureWebhook
repositories.webhookConfigTitle
repositories.webhookConfigDescription
repositories.webhookConfigNoBinding
repositories.webhookCallbackUrl
repositories.webhookCallbackWarningTitle
repositories.webhookCallbackMissingPublicUrl
repositories.webhookCallbackLocalhostWarning
repositories.webhookId
repositories.webhookLastEvent
repositories.webhookLastCommit
repositories.webhookLastReceivedAt
repositories.webhookRecentActivity
```

### settings（3）

```text
settings.ai.modelRequired
settings.billingRateRulesDescription
settings.securityDescription
```

### usersPage（1）

```text
usersPage.listTitle
```

### 顶层 key（1）

```text
backToProjectWorkspace
```

## 附录 B：18 个无生产调用 Web API 方法

```text
applicationsApi
  listDeploymentTargetsPage
  reconfigureRepositoryWebhook

buildsApi
  listProjectHookRuns
  getProjectHookRunLogs
  listBuildJobsPage
  getBuildJobLogs

gatewayApi
  listGatewayRoutesPage

gitApi
  readGitFile
  listGitContents

projectsApi
  listSystemComponents
  createGatewayTrafficUsage
  listProjectPins
  updateProjectOrder

registriesApi
  getDefaultRegistry
  listRegistryCredentials

runtimeApi
  listReleasesPage
  execReleaseRuntimeCommand

volumesApi
  listVolumeTransfers
```

删除前必须逐符号复查，以当时的工作树为准。

## 附录 C：报告交付状态

本报告只做审计，没有删除或重构任何业务文件，也没有执行提交或推送。仓库 `.gitignore:39` 忽略 `/reports/`，所以本文件默认是本地交付物；如需纳入版本控制，应显式调整忽略策略或单独强制添加。
