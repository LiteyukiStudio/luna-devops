# Luna DevOps 前端开发规范

本文件适用于 `web/` 及其全部子目录，用于把根目录规范落实为可执行的前端工程约定。它约束新增代码和被修改的既有代码；历史实现不自动成为新代码可以照搬的先例。

## 0. 适用范围与权威顺序

- 根目录 [`AGENTS.md`](../AGENTS.md) 的产品、安全、全链路、一致性、文档和 Git 规则始终生效，本文件不得放宽其中任何 MUST。
- 发生冲突时，依次以用户当前要求、根目录 `AGENTS.md`、OpenAPI 与后端真实契约、本文件、相邻模块既有模式为准；本文件不能反向覆盖排在前面的权威，根级安全硬约束不得放宽。
- [`SHADCN_COMPONENTS.md`](SHADCN_COMPONENTS.md) 是基础组件清单与选型事实源，`src/styles/design-tokens.css` 是设计 token 事实源；本文件不复制它们的易变明细。
- `package.json`、`vite.config.ts`、`tsconfig*.json`、`eslint.config.js` 和 CI 脚本是工具链与命令的事实源。
- 业界参考用于补充实现方法，不能据此新增用户未要求的产品行为、依赖、页面、状态或抽象。

本文件中的关键词含义：

- **必须 / 不得**：合并前必须满足，没有默认例外。
- **应该 / 优先**：除非现有契约或可验证的局部原因不适用，否则应遵守，并在变更说明中写明例外。
- **可以**：在不扩大需求和不破坏其他约束时允许采用。

## 1. 开工前检查

开始修改前必须：

1. 阅读根目录 `AGENTS.md`、本文件、[`README.md`](README.md) 和与任务相关的公开文档。
2. 阅读目标页面、相邻组件、API domain、类型、翻译资源和就近测试，先确认现有数据流再写代码。
3. 在 `src/components/ui`、`src/components/common`、`src/lib` 中搜索已有能力，避免重复组件和重复规则。
4. 对照 [`openapi/openapi.yaml`](../openapi/openapi.yaml) 确认请求、响应、错误码、枚举、权限和分页契约。
5. 检查工作树，只修改本轮目标及其必要调用链，保留用户已有的未提交改动。
6. 明确涉及的 Web、API、Worker、Agent 边界；不适用的层无需改动，但必须能说明原因。

范围控制：

- 用能完成当前验收的最少机制实现，不预建未来框架、通用状态库、插件层、兼容层或 fallback。
- 只有一个真实调用方时先保持局部；出现稳定的第二个复用点后再评估抽到 `components/common` 或 `lib`。
- 不顺手重写相邻页面、升级依赖、改变视觉体系或清理历史代码；发现范围外问题时仅报告证据。

## 2. 固定技术栈

| 领域 | 当前方案 | 约束 |
| --- | --- | --- |
| UI 运行时 | React 19 | 使用函数组件和 Hook，保持渲染纯净。 |
| 路由 | React Router 7 | 路由集中在 `src/App.tsx`，页面级懒加载。 |
| 语言与构建 | TypeScript 6、Vite 8 | 不放宽现有类型检查或构建门禁。 |
| 样式 | Tailwind CSS 4、语义 CSS variables | 业务代码只消费设计 token。 |
| 基础组件 | shadcn/ui、Radix UI | 不平行实现已有 primitive。 |
| 服务端状态 | TanStack Query 5 | 不把请求结果复制为另一套本地事实。 |
| 表单 | React Hook Form、Zod | schema 是校验和表单类型的单一事实源。 |
| 国际化 | i18next、react-i18next | 所有用户可见文案都进入翻译资源。 |
| 通知 | Sonner | 不自建第二套 toast 系统。 |
| 测试 | Vitest、Testing Library、jest-dom | 测用户可观察行为，不测实现细节。 |
| 包管理 | pnpm 11.1.0 | 不使用 npm、Yarn 或 Bun 修改前端依赖。 |

未经明确讨论，不引入 Redux、Zustand、MobX、另一套路由器、另一套表单库、CSS-in-JS、第二套组件库、Next.js/RSC、React Compiler、Playwright、MSW 或覆盖率平台。需要新增依赖时，先证明现有栈无法以更小成本完成同一目标。

## 3. 目录职责与依赖方向

### 3.1 目录边界

| 目录 | 职责 | 不得承担 |
| --- | --- | --- |
| `src/pages/<module>/` | 路由页面、页面私有组件、Hook、模型、工具和测试 | 跨页面基础组件、通用 API 请求器 |
| `src/components/ui/` | shadcn/ui 基础组件和跨业务 primitive | 业务查询、权限、领域状态 |
| `src/components/common/` | 至少两个页面稳定复用的业务组合组件 | 页面专属流程、重复的 shadcn primitive |
| `src/api/domains/` | 按业务域组织的 API 方法 | React 状态、toast、导航和视图格式化 |
| `src/api/core.ts` | 统一请求、Cookie、语言、Trace Context、错误边界 | 页面业务编排 |
| `src/app/` | Session、主题、公开配置、更新检查、应用级 Provider | 单页面局部状态 |
| `src/layouts/` | 应用壳、导航和路由出口布局 | 具体业务页面逻辑 |
| `src/lib/` | 与页面无关的纯逻辑、角色、轮询、遥测和查询策略 | JSX 页面容器和业务 CRUD |
| `src/i18n/` | 语言配置、核心资源、功能 bundle 和检查边界 | 后端原始数据或用户输入 |
| `src/styles/` | 设计 token、品牌和主题样式 | 页面私有补丁集合 |
| `src/test/` | 全局测试环境和跨测试基础设施 | 业务模块专属 fixture |

### 3.2 依赖规则

- 页面可以依赖 `@/api`、`@/app`、`@/components`、`@/lib` 和 `@/i18n`。
- `components/ui` 不得依赖页面、API domain 或业务权限。
- `components/common` 不得依赖某个页面目录中的私有模块；确需共享时把稳定能力上移。
- `api` 不得导入页面或视图组件；错误本地化仅通过现有 i18n 边界完成。
- 不通过 barrel export 隐藏循环依赖或扩大初始化副作用。新增 `index.ts` 前必须有清晰公共 API 价值。
- 跨页面私有目录互相导入通常表示边界错误，应抽取真正共享部分或保留各自局部实现。

## 4. 文件、命名、导入与 TypeScript

### 4.1 文件和命名

- 导出组件与页面使用 PascalCase；自定义 Hook 使用 `useXxx`；普通函数和变量使用 camelCase；常量仅在真正不可变且模块级时使用 UPPER_SNAKE_CASE。
- 页面入口沿用 `XxxPage.tsx`；普通文件遵循所在目录现有的 kebab-case 或既有约定，不为统一大小写批量重命名。
- 测试与源码共置，命名为 `*.test.ts` 或 `*.test.tsx`。
- 组件名表达业务语义，不使用 `CommonComponent`、`BaseWrapper`、`Manager` 等含义过宽的名称。
- 布尔值使用 `is`、`has`、`can`、`should` 等前缀；事件 props 使用 `onXxx`，内部处理器使用 `handleXxx`。

### 4.2 导入

- 引用 `src` 下共享模块必须使用 `@/` 根路径。
- 相对导入只用于同一页面、组件或功能目录中的私有文件。
- 类型使用 `import type` / `export type`，保持 `verbatimModuleSyntax` 兼容。
- 不创建跨越多个 `../` 的深层导入，不从未公开的内部文件穿透组件边界。

### 4.3 类型

- 不得用 `any`、批量类型断言、`@ts-ignore`、关闭检查规则或随意的非空断言掩盖契约问题。
- 外部输入、未知 JSON、捕获异常和存储内容先视为 `unknown`，通过 Zod、显式类型守卫或已有解析器收窄。
- 有限状态优先使用可辨识联合；switch 处理稳定枚举时应穷尽分支，新增值不得静默落入错误语义。
- API DTO、OpenAPI Schema 和领域枚举只维护一套语义；不要为页面再手写一份字段近似但含义不同的类型。
- Zod schema 含 transform 时明确区分 `z.input` 与 `z.output`。
- 不宣称当前项目已启用未在 `tsconfig` 中存在的选项；修改 TypeScript 配置属于工具链变更，需单独说明影响。
- 类型安全不能替代运行时边界校验，运行时校验也不能成为容忍前后端契约漂移的兼容层。

## 5. 应用入口、Provider 与路由

- `src/main.tsx` 是应用级 Provider 的唯一装配入口。QueryClient 必须在应用生命周期内保持单例，不得在组件渲染中创建。
- 新增全局 Provider 前先证明状态确实跨多数路由共享；页面级状态不得上提到应用根。
- 路由统一在 `src/App.tsx` 注册。页面使用 `lazyTranslated` 延迟加载，并列出真实依赖的翻译 bundle。
- 新路由必须同步：
  1. 页面模块和命名导出；
  2. `lazyTranslated` 或无翻译依赖时的 `lazyNamed`；
  3. 路由定义、鉴权布局和重定向；
  4. 所有支持语言的功能 bundle；
  5. 导航、权限可见性、测试和必要文档。
- 登录后页面放在 `AppLayout` 下；公开认证、初始化、OAuth 协议页保持现有独立边界。
- 可分享、可刷新恢复的导航状态优先放 URL、path、search 或既有 hash 协议；短暂展开、hover、Dialog 开关留在局部 state。
- 不在组件 render 中导航；用户动作在事件处理器中导航，服务端结果触发的导航放 mutation 成功路径。

## 6. API、契约与错误边界

### 6.1 请求入口

- 页面和组件不得直接调用 `fetch`，统一通过 `@/api` 的领域 API。
- 浏览器不得直接编排 GitHub、Gitea、GitLab、Harbor、DockerHub、OIDC、Kubernetes、Traefik、AI Provider 等外部平台；这些能力必须由 Luna 后端适配。
- `src/api/core.ts` 统一处理凭据、`Accept-Language`、Trace Context、JSON、错误码和 `ApiError`；新代码不得平行复制这些逻辑。
- 请求可取消时继续传递 `AbortSignal`；资源切换后旧请求不得覆盖新资源。

### 6.2 新增或修改 API 操作

必须依次确认：

1. OpenAPI 请求、响应、错误码、权限、分页和枚举已经表达真实业务语义。
2. `src/api/types.ts` 或专用类型文件与 OpenAPI 一致，没有宽松 fallback。
3. 方法加入正确的 `src/api/domains/<domain>.ts`。
4. 方法名同步登记到 `src/api/client.ts` 的 `domainOperations`，保持懒加载代理完整。
5. 页面通过稳定的 `api.operation(...)` 调用，并补齐领域 API 或页面契约测试。
6. 修改跨 Worker、Agent 或异步载荷时，同一事项同步所有参与层。

### 6.3 错误和分页

- 后端返回稳定 `code`、枚举和必要原始 detail；前端把稳定 code 映射为 i18n 文案。
- 用户界面不得直接展示堆栈、SQL、OIDC 原始错误、第三方响应体或开发者 detail。
- 未知错误使用通用、可恢复的用户文案，并保留已有 request/trace 标识供诊断。
- 未来可能超过 100 条的列表必须消费标准分页响应：`items/page/pageSize/sortBy/sortOrder/total/totalPages`。
- 分页、排序、搜索和筛选参数都必须进入请求和 query key；前端不得拉取全量后自行模拟服务端分页。

## 7. 状态管理与 TanStack Query

### 7.1 单一事实源

| 状态类型 | 所属位置 |
| --- | --- |
| 服务端实体、列表、详情、任务状态 | TanStack Query |
| 可分享的筛选、Tab、资源定位 | Router URL / 现有 hash 约定 |
| 表单编辑草稿与校验 | React Hook Form |
| Dialog、Popover、选中行等瞬时交互 | 最近的组件 `useState` |
| Session、主题、公开平台配置 | 现有 `src/app` Provider |

- 同一事实不得同时保存在 Query cache、Context、组件 state 和表单 state 中。
- Query 数据进入可编辑表单后可以成为独立草稿，但必须通过一次明确的 `reset` 建立基线，不持续双向同步。
- 能从 props、查询结果或已有 state 计算出的值在 render 中派生。

### 7.2 Query

- query key 顶层使用可序列化数组，并包含 `queryFn` 使用的项目空间 ID、资源 ID、分页、排序、搜索和筛选参数。
- 缺少必要 ID 时使用 `enabled` 阻止请求，不向 API 发送空资源标识。
- `queryFn` 保持纯粹，只获取并返回数据；toast、导航、本地存储和 UI 状态变化不放入其中。既有偏差只在相关任务范围内迁移，不为遵守本条扩大当前改动。
- 明确理解默认缓存语义，不给实时状态设置长 `staleTime`，也不靠缓存延续已经不可验证的当前事实。
- 需要转换展示数据时优先在 render 或 Query `select` 中做纯转换；不得复制进 state 后用 Effect 同步。
- 不为了形式统一新建全局 query-key 框架；稳定 key 可以与所属 domain 或页面共置，重复出现后再抽取。

### 7.3 Mutation

- 写操作使用 mutation，提交期间阻止重复请求并给出明确 pending 状态。
- 成功后精确更新或失效受影响的列表、详情和摘要，随后以权威回读确认最终状态。
- 不用无差别“清空全部缓存”代替依赖分析。
- 乐观更新仅用于可逆、低风险交互，并且必须同时具备快照、失败回滚和最终失效回读。
- 错误由统一错误边界转为安全文案；mutation 组件仍需决定字段错误、页面错误或 toast 的正确承载位置。

### 7.4 实时状态与轮询

- 健康度、当前副本数、实时指标和当前资源数量必须使用 `liveObservationQueryPolicy`。
- 实时请求失败时展示 `unavailable` 和稳定 `observationCode`，不得继续展示上次成功值冒充当前事实。
- 轮询间隔复用 `src/lib/polling.ts`；任务终止、页面隐藏或条件不适用时停止无意义轮询。
- SSE、WebSocket、计时器和订阅必须绑定当前资源 ID，并在 cleanup 中关闭。
- 旧资源的异步回调不得写入新资源状态；使用 AbortController、请求代次或等价机制保证归属。

## 8. React 组件、Hook 与 Effect

- 组件和 Hook 的 render 必须纯净、幂等；不得在 render 中发请求、toast、写 storage、改 DOM、导航或修改 props/state。
- Hook 只能在函数组件或自定义 Hook 顶层调用，不得放在条件、循环、事件、`try/catch` 或普通函数中。
- 用户动作引发的副作用放对应事件处理器；Effect 只用于 EventSource、WebSocket、DOM、媒体查询、计时器等外部系统同步。
- 禁止用 `useEffect + setState`：
  - 复制 props 或 Query 数据；
  - 计算派生值；
  - 修剪选择项；
  - 回填默认选项；
  - 重置搜索、筛选或页码；
  - 响应用户点击后再执行本可直接完成的动作。
- Effect 必须能安全经历开发模式的重复挂载，依赖完整，cleanup 对称，不泄漏监听器、连接或计时器。
- 列表 key 使用服务端或领域稳定 ID；可能增删、排序的列表不得使用 index、随机数或 render 时生成值。
- state 保持最小、规范化且归属最近使用者，避免多组会互相矛盾的布尔值。
- `memo`、`useMemo`、`useCallback` 只用于已测量成本或稳定引用契约；正确性不得依赖 memoization。
- 组件同时承担查询、复杂表单、权限、多个 Dialog 和视图编排时，应按真实职责拆为页面协调层、领域 Hook 和局部视图，而不是继续追加特殊 case。

## 9. 表单与 Zod

- 新增或修改表单必须使用 React Hook Form、`zodResolver` 和 Zod schema。
- schema 是运行时校验与表单类型的单一事实源，从 schema 推导类型，不再手写重复的 `FormValues`。
- 校验文案必须 i18n。需要 `t` 的 schema 使用 `createSchema(t)` 或等价工厂，并随当前语言稳定重建。
- `defaultValues` 必须完整、稳定，不使用 `undefined` 作为受控字段默认值。
- 原生输入优先 `register`；受控的 Select、Checkbox、Switch 等仅在需要时使用 `Controller`，不得同时重复 `register`。
- 由 RHF 直接拥有的动态行优先使用 `useFieldArray`，渲染 key 使用 `field.id`；独立领域编辑器或作为单字段整体序列化的值沿用其明确边界。
- 打开编辑 Dialog 或切换被编辑资源时一次 `reset` 建立表单基线；不得用多个 Effect 逐字段 `setValue`。
- 提交必须经过 `handleSubmit`。前置条件未满足、表单无效或 mutation pending 时，主操作保持 disabled，并提供 `aria-busy`。
- 字段错误紧邻字段展示，通过 `aria-invalid` 和 `aria-describedby` 与控件关联。
- 必填标记使用主题色，不用红色制造额外警报。
- 页面级表单使用 `FormActions`；Dialog 使用 `DialogFooter`。桌面端操作按内容宽度右对齐，移动端才全宽。
- Secret、Token、密码和 Registry Credential 编辑时不得回显旧值；留空默认表示不修改。清除敏感值必须是显式且经后端支持的独立动作。
- 前端校验只改善交互，不能替代服务端校验、权限和审计。

## 10. UI、布局与设计系统

### 10.1 组件选择

- 基础 UI 必须优先复用 shadcn/ui；新增前先查 [`SHADCN_COMPONENTS.md`](SHADCN_COMPONENTS.md) 和 `components.json`。
- `components/ui` 只做跨业务 primitive；领域查询、权限和状态不得进入基础组件。
- `components/common` 只放真实跨页面业务组合。仅转发 props、没有统一语义的薄包装不值得新增。
- 修改 shadcn/Radix 组件时保留其键盘、焦点、ARIA、Portal 和受控/非受控语义，不为样式改成可点击 `div`。

### 10.2 页面结构

- 页面根宽度和区块间距使用 `PageShell` 的 `content`、`full`、`settings`、`tool` 语义，不在业务页重复维护 `max-width` 或全局 padding。
- 桌面页面标题、工具和 Tabs 统一使用 `PageChrome`、`PageChromeTools`、`PageChromeTabs`、`ContentTabs`。
- 业务区块优先组合 `Surface`、`Section`、`MetricGroup`；没有业务语义的 Card 嵌套必须删除。
- 登录后画布和顶栏 padding 由布局层统一提供，业务页面不得使用负间距或额外根 padding 补偿。
- 页面主要、相关、字段组和行内间距优先使用 `gap-6`、`gap-4`、`gap-3`、`gap-2` 及项目语义 token。

### 10.3 颜色、表面和动作

- 颜色、前景、背景、边界、圆角、间距、阴影和动效时长必须使用 Tailwind 标准 token 或 `design-tokens.css` 的语义变量。
- 状态展示使用 `StatusBadge`、`StatusValueBadge` 或带 `tone` 的公共组件；业务代码不得直接拼写 `red-*`、`green-*`、`amber-*` 等状态色。
- 普通业务 Surface/Card 通过实体表面和圆角分层，不添加无意义外框或常驻阴影。阴影保留给 Dialog、Popover、悬浮层和明确 raised 状态。
- 同一页面或 Tab 默认最多一个实心主色主操作；同级操作使用 outline、ghost 或菜单。
- 页面不自行读取“标准 / 简约 / 跟随平台”偏好，也不为主题增加局部分支。
- 用户可见品牌固定为 “Luna DevOps”；用户界面的 “Project” 固定译为“项目空间”。

### 10.4 列表和状态

- 管理列表统一使用 `DataList`，不得另写同类表格、分页、搜索工具栏或移动端操作容器。
- 搜索、筛选和排序放 `DataList` 工具区；页面级创建操作放 `PageChrome` 标题工具。
- 页面标题已经表达上下文时，不重复传入“XX 列表”标题。
- 整行点击只用于明确的详情导航，并提供键盘行为和可访问名称；编辑、删除、测试、绑定等操作必须有独立按钮或菜单。
- `total === 0` 时不显示分页、每页条数或翻页器。
- 首次配置空状态给出明确下一步；筛选为空保持紧凑并提供清除条件。
- Loading、empty、error 是互斥状态：使用结构化 skeleton、`EmptyState` 和 `ErrorState`，不能用一行“加载中”替代真实页面骨架。
- 失败、不可用和待处理必须在摘要层给出语义 tone 与文字；颜色不能成为唯一信息。

## 11. i18n 与格式化

- 标题、描述、按钮、菜单、label、hint、placeholder、toast、错误、空状态、确认文案、`aria-label`、Zod 文案和状态 badge 全部使用 i18next。
- 当前支持语言以 `src/i18n/config.ts` 为准：`zh-CN`、`zh-TW`、`en-US`、`ja-JP`、`ko-KR`。新增 key 必须五种语言结构和插值变量一致。
- 核心通用资源放既有核心 bundle；页面或功能资源放对应 feature bundle，不把大功能文案全部塞入 root。
- 新路由或新增 bundle 必须同步 `App.tsx` 的 `lazyTranslated` 列表；测试环境会加载全部 bundle，但这不能代替真实路由加载配置。
- 不拼接完整译文，不靠字符串片段适配语序；使用 interpolation、复数规则和必要时的 `Trans`。
- 日期、时间、数字、金额和列表使用 `Intl` 或现有格式化 helper，不手写语言特定格式。
- 后端稳定 code、枚举和状态由前端映射翻译；日志、第三方原始文本和用户输入作为数据展示时保留原值，但不能冒充 UI 文案。
- 动态 key 优先稳定前缀。静态检查无法展开时，只能在 `scripts/i18n-dynamic-key-allowlist.mjs` 记录真实调用签名和理由，不用宽泛规则绕过检查。
- React 已转义普通文本；仍不得把翻译或用户输入直接送入 `dangerouslySetInnerHTML`。
- i18n 变更至少执行 `pnpm --dir web check:i18n`；`lint` 已包含同一门禁。

## 12. 可访问性与响应式

- 优先使用原生 `button`、`a`、`input`、`table`、`fieldset` / `legend`，不使用可点击 `div` 冒充控件。
- 每个表单控件必须有稳定 `id` 和可感知 label；hint、description、error 通过 `aria-describedby` 关联。
- 图标按钮必须有本地化可访问名称；装饰图标使用 `aria-hidden`，装饰图片使用空 `alt`。
- Dialog、AlertDialog、Sheet 必须有 Title 和 Description；视觉隐藏时使用 `sr-only`，不能删除语义节点。
- 打开浮层后焦点进入合理位置，关闭后返回触发器；不要破坏 Radix 已提供的焦点管理。
- 所有操作必须可由键盘完成并保留 `focus-visible`。Tooltip 必须同时支持 hover 与 focus，关键说明不能只存在于 Tooltip。
- 加载和提交使用 `aria-busy` / `role="status"`，需要立即提示的错误使用 `role="alert"`，避免重复播报。
- 动画和自动滚动必须尊重 `prefers-reduced-motion`。
- 桌面端超过四个筛选字段时，移动端使用 Sheet、Popover 或等价渐进披露；高频列表明确保留移动端关键列。
- fixed/sticky 控件必须为主操作、分页、toast、Dialog、Sheet 和 safe area 留出空间。
- 视觉改动至少检查桌面、移动、浅色、深色、键盘焦点、长文案和空/错/加载状态。

## 13. 权限、安全与隐私

- 前端隐藏或禁用按钮只改善体验，后端必须重新执行认证、授权、资源归属、审批和审计。
- 平台角色和项目角色复用 `src/lib/roles.ts` 及 OpenAPI 共享 schema，不散落角色字面量。
- `VITE_*` 会进入浏览器产物，只能承载公开构建配置，绝不存放 Secret、Token、密码或私钥。
- Session 依赖现有 HttpOnly Cookie 和统一 API Client；不把凭据复制到 localStorage、URL、日志或遥测。
- Secret 表单不回显原值，不在 toast、错误详情、测试 fixture 和截图中暴露。
- 不渲染后端或第三方未清洗 HTML。确需富文本、Markdown 或外部 URL 时，复用既有安全渲染器并审计协议、链接和内容边界。
- 新开外部链接时使用安全的 `rel`，URL 必须来自可信配置或经过明确协议校验。
- 用户输入、资源名、完整 URL 查询、Token、Cookie、Prompt 和请求正文不得进入 Span 名、Metric label 或普通日志属性。
- 客户端错误提示不应泄露实现细节；诊断标识可以复制，但必须明确其用途。

## 14. 性能与资源生命周期

- 路由页面、重型编辑器、图表、拓扑和大 Dialog 优先沿用现有懒加载边界，不把所有功能加入初始 bundle。
- 先消除重复请求、串行瀑布、无意义轮询和重复 render，再考虑 memoization。
- Query 的缓存和失效按数据语义配置，不用统一的长 `staleTime` 掩盖请求设计问题。
- 大列表使用服务端分页、筛选和排序；不得默认在浏览器载入全量数据。
- 订阅、Observer、计时器、Object URL 和第三方实例必须在资源切换或卸载时释放。
- React、React DOM、CodeMirror 等依赖对象身份的库在产物中必须只有一个兼容版本。
- 新增依赖需评估 bundle、初始化成本、维护状态、许可证和是否能被现有能力替代；不为一个小 helper 引入大型库。
- 性能优化必须有可复现证据或测量结果，不以“可能更快”为由增加复杂性。

## 15. 测试规范

### 15.1 测试原则

- 使用现有 Vitest、Testing Library、jest-dom 和 `src/test/setup.ts`；不擅自引入新的测试框架。
- 测试以用户可观察行为和 DOM 结果为主，不测试组件实例、内部 state 或私有 Hook 实现。只有当 class、`data-slot` 或 DOM 层级本身构成布局、安全或第三方集成契约时，才做对应精确断言。
- 查询优先级：`getByRole({ name })`、`getByLabelText`、可见文本；`data-testid` 仅用于没有用户可感知语义的目标。
- 立即存在用 `getBy*`，异步出现用 `findBy*`，断言不存在用 `queryBy*`；所有异步交互必须 await。
- 用户交互优先每个测试创建 `userEvent.setup()` 并 await；只有 user-event 无法表达的底层事件才使用 `fireEvent`。
- `waitFor` 中只放可重复断言，不放点击、提交或其他副作用，不使用任意 sleep。
- 优先写少量清晰的行为断言，不用大快照代替业务验证。

### 15.2 按场景覆盖

- 纯函数：正常值、边界值、非法输入和稳定枚举穷尽。
- Query 页面：loading、success、empty、error，以及关键筛选、分页和权威回读。
- 实时查询：成功、`unavailable`、资源切换、cleanup、停止轮询和旧响应不覆盖新资源。
- Mutation：pending 防重复、成功失效/回读、失败提示和权限失败。
- 表单：默认值、无效禁用、字段错误、有效提交、pending、防止敏感字段回显或空值覆盖。
- 路由：MemoryRouter 下的导航、重定向、参数和可分享状态。
- 可访问组件：可访问名称、键盘路径、Dialog 焦点和状态语义。
- 缺陷修复：先补能复现原问题的测试，再验证修复后的用户结果。

Query 测试使用独立 QueryClient 并关闭 retry，结束后清理缓存；路由测试使用 MemoryRouter；网络和 API 在领域客户端边界 mock，不直接 mock 被测组件内部 Hook。

## 16. 依赖与 shadcn 维护

- 从仓库根目录使用 `pnpm --dir web ...`，锁文件只维护 `web/pnpm-lock.yaml`。
- 常规本地安装和 CI 使用 `--frozen-lockfile`；明确修改依赖时通过 pnpm add/remove 或非 frozen install 更新锁文件，并检查 diff 中没有无关升级。
- 新增或升级运行时身份敏感依赖后必须运行 `check:singletons`，并用 `pnpm --dir web list` 检查依赖树。
- 出现重复 React、React DOM、CodeMirror 核心包时，在 `pnpm-workspace.yaml` 使用最小范围 override 统一依赖族，不依赖 bundler 偶然去重。
- shadcn 组件沿用 `components.json` 的 new-york、CSS variables、Lucide 和 `@/` alias 配置。
- 引入 shadcn 组件前检查现有 `components/ui`，避免生成重复文件或覆盖项目已有语义改造。
- 不提交 `node_modules`、`dist`、`*.local`、测试日志或临时报告。

## 17. 可观测与长连接

- 普通 HTTP 继续经过 `api/core.ts` 的 Trace Context 注入。
- 新增 EventSource 或 WebSocket 使用 `src/lib/telemetry.ts` 的 `createTracedEventSource` / `createTracedWebSocket`，不绕过现有追踪边界。
- 操作名使用稳定、低基数模板，不包含资源 ID、用户输入、完整 URL 或查询值。
- 长连接必须记录可关联的打开、错误和关闭终态，并在组件 cleanup 时正常结束。
- 遥测不得包含 Authorization、Cookie、Secret、Token、Prompt、模型正文、表单内容或后端原始错误体。
- 浏览器观测只辅助诊断，不能改变业务成功、权限或实时事实语义。

## 18. 文档与全链路一致性

- 用户可见行为、配置、限制或操作步骤变化时，同步 `docs/docs/zh` 与 `docs/docs/en`；纯内部实现不进入公开文档。
- OpenAPI 是 API 契约事实源。请求/响应、枚举、错误码、权限、Agent 工具和异步载荷变化必须同一事项同步。
- UI 只本地化稳定 code 和枚举；后端不得为了前端直接返回某一种语言的 UI 文案。
- 修改计划、验收或完成状态时才更新 `TODO.md`，文档整理本身不创造新的产品 TODO。
- 删除功能或字段时同步删除页面入口、API 类型、翻译、测试和现行文档，不保留无真实调用方的兼容叙述。

## 19. 验证矩阵

| 改动类型 | 最低验证 |
| --- | --- |
| 纯函数、Hook 或组件行为 | 就近测试；完整交付前再运行 lint 和 build |
| 页面、路由、表单、Query 或 Mutation | 相关测试、`pnpm --dir web lint`、`pnpm --dir web build` |
| i18n 文案或 bundle | `pnpm --dir web check:i18n`、相关测试、lint、build |
| 依赖或锁文件 | `pnpm --dir web check:singletons`、`pnpm --dir web list`、test、lint、build |
| 设计 token、布局或关键交互 | test、lint、build，并用真实浏览器检查桌面/移动和明暗主题 |
| 权限、认证、Secret、长连接、外部平台或跨服务契约 | 根规范要求的完整验证、真实成功/失败链路和必要浏览器验收 |
| 仅 README / AGENTS 文档 | `git diff --check`、链接与命令事实核对；未改代码时不为形式重复构建 |

完整前端门禁：

```bash
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web check:singletons
pnpm --dir web build
```

完成标准：

- 当前功能适用的成功、空、失败、pending 和取消路径符合预期。
- 参与调用链的类型、OpenAPI、API Client、权限、i18n 和错误语义一致。
- 没有复制 server state、派生 state、基础组件或设计 token。
- 键盘、可访问名称、移动端、主题、loading/empty/error 均已覆盖。
- 敏感信息不进入 UI、客户端存储、日志、遥测或构建变量。
- 测试与改动风险相称，lint/build 无新增 error 或 warning。
- 只包含本轮必要文件，没有格式化或覆盖用户的无关改动。

## 20. 禁止的常见反模式

- render 中请求、toast、写存储、改 DOM、导航或修改输入对象。
- 条件调用 Hook，或创建 `useEffectOnce` 等绕过 React 生命周期的包装。
- 用 Effect 保存派生值、复制 Query 数据、同步 props、修剪选项或重置页码。
- 每次 render 创建 QueryClient、缺变量的 query key、对空 ID 发请求。
- mutation 后失效全部缓存，或没有回滚的高风险乐观更新。
- 页面直接 `fetch`、直连第三方平台、展示后端原始异常。
- `Controller` 与 `register` 双重注册、`undefined` 默认值、字段数组用 index key。
- 重复维护 Zod schema 和手写表单类型。
- 拼接翻译句子、硬编码复数/日期格式、把原始错误动态当翻译 key。
- 业务页面重造 Button、Dialog、Select、Table、Pagination、Form Field、Empty State。
- 硬编码状态色、页面根 padding、任意像素间距、无语义 Card 嵌套或多枚实心主操作。
- 用可点击 `div`、无 label 输入、只靠颜色的状态、只支持 hover 的关键说明。
- 用 class、DOM 结构、内部 Hook 或 test id 代替用户行为测试；布局、安全或集成契约确需结构断言时除外。
- 通过放宽类型、lint、i18n 或单例门禁让代码“通过”。

## 21. 参考基线

本规范结合当前仓库事实与以下官方或成熟项目公开规范整理；外部资料不覆盖 Luna DevOps 的项目级规则：

- React：[组件与 Hook 纯度](https://react.dev/reference/rules/components-and-hooks-must-be-pure)、[不需要 Effect 的场景](https://react.dev/learn/you-might-not-need-an-effect)、[Hooks 规则](https://react.dev/reference/rules/rules-of-hooks)、[状态结构](https://react.dev/learn/choosing-the-state-structure)
- TypeScript：[类型收窄与可辨识联合](https://www.typescriptlang.org/docs/handbook/2/narrowing.html)
- Vite：[TypeScript](https://vite.dev/guide/features.html#typescript)、[环境变量与模式](https://vite.dev/guide/env-and-mode)
- TanStack Query：[Query Keys](https://tanstack.com/query/latest/docs/framework/react/guides/query-keys)、[重要默认值](https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults)、[Query 取消](https://tanstack.com/query/latest/docs/framework/react/guides/query-cancellation)
- React Hook Form：[`useForm`](https://react-hook-form.com/docs/useform)、[`Controller`](https://react-hook-form.com/docs/usecontroller/controller)、[`useFieldArray`](https://react-hook-form.com/docs/usefieldarray)
- Zod：[基础解析与错误处理](https://zod.dev/basics)
- i18next：[Namespaces](https://www.i18next.com/principles/namespaces)、[格式化](https://www.i18next.com/translation-function/formatting)、[react-i18next `useTranslation`](https://react.i18next.com/latest/usetranslation-hook)
- Testing Library：[指导原则](https://testing-library.com/docs/guiding-principles/)、[查询优先级](https://testing-library.com/docs/queries/about/)、[user-event](https://testing-library.com/docs/user-event/intro/)
- shadcn/ui：[设计理念](https://ui.shadcn.com/docs)、[组件目录](https://ui.shadcn.com/docs/components)、[主题](https://ui.shadcn.com/docs/theming)
- W3C WAI-ARIA：[Authoring Practices Guide](https://www.w3.org/WAI/ARIA/apg/)
- GitLab：[Frontend Development Guidelines](https://docs.gitlab.com/development/fe_guide/)
