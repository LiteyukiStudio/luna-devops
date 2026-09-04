# Luna DevOps 前端增量规范

本文件只保存 `web/` 特有规则；根 `AGENTS.md` 的产品、安全、全链路、文档和 Git 约束始终生效。
依赖、脚本与工具配置分别以 `package.json`、`vite.config.ts`、`tsconfig*.json`、
`eslint.config.js`、`components.json` 和 CI 脚本为事实源。

## 1. 开工与范围

- 修改前阅读根规范、`web/README.md`、目标页面、相邻组件、API domain、类型、翻译和就近测试。
- 在 `components/ui`、`components/common`、`lib` 和相邻页面搜索现有能力，避免平行实现。
- 对照根 OpenAPI 确认请求、响应、错误、权限、分页和枚举；先理清数据流再写代码。
- 只改当前需求和必要调用链。不预建状态库、插件层、兼容层、fallback 或只有一个调用方的公共包装。
- 保留工作树中的用户改动；不顺手升级依赖、统一命名、重画页面或清理无关历史。

## 2. 目录与依赖方向

- 页面和私有组件、Hook、模型、测试放 `src/pages/<module>` 并共置。
- `components/ui` 只放 shadcn/ui primitive；`components/common` 只放至少两个页面稳定复用或职责明确的
  业务组合。只转发 props 的单调用方包装留在页面或直接内联。
- `api/domains` 只处理领域请求；`api/core.ts` 统一 Cookie、语言、Trace Context、JSON 与 `ApiError`；
  `app` 放 Session、主题和应用级 Provider；`lib` 放页面无关的纯逻辑与查询策略。
- `src` 下跨目录共享引用使用 `@/`；相对导入只用于当前页面或组件目录的私有文件。
- UI primitive 不依赖页面、API 或业务权限；API 不依赖 React 视图；不要用 barrel 隐藏循环依赖。
- 测试与源码共置，命名 `*.test.ts` 或 `*.test.tsx`；全局测试设施才放 `src/test`。

## 3. TypeScript 与运行时边界

- 外部 JSON、存储内容和捕获异常先视为 `unknown`，在入口用 Zod、类型守卫或既有解析器收窄。
- 不用 `any`、批量断言、`@ts-ignore`、随意非空断言或关闭规则掩盖契约问题。
- 有限状态使用可辨识联合并穷尽；API DTO、OpenAPI schema 和领域枚举只维护一套语义。
- Zod transform 明确区分 `z.input` 与 `z.output`；运行时校验不承担旧契约兼容。
- React、React DOM、CodeMirror 及依赖 Context、Symbol、`instanceof` 或扩展对象身份的库必须保持单例。

## 4. API、路由与服务端状态

- 页面和组件只通过 `@/api` 的领域 API 请求；不得直接 `fetch` 或编排第三方平台 API。
- 新操作先更新 OpenAPI，再同步领域类型/适配器、`api/domains/<domain>.ts`、
  `api/client.ts` 的 `domainOperations` 与契约测试；不要用宽松 fallback 维持漂移。
- 请求可取消时传递 `AbortSignal`。资源切换后，旧请求和旧订阅不得覆盖新资源。
- 路由集中在 `App.tsx`，页面用 `lazyTranslated` 加载真实需要的 bundle；新路由同步鉴权、导航、
  重定向、翻译和测试。
- 可刷新、可分享状态放 URL；Dialog、Popover、选中行等瞬时状态留在最近组件。
- 服务端实体、列表和任务状态属于 TanStack Query；表单草稿属于 RHF；Session、主题和公开配置使用现有
  app Provider。同一事实不得复制到 Query、Context、state 和表单。
- Query key 包含请求使用的资源、分页、排序、搜索和筛选参数；缺必需 ID 时用 `enabled`，不发送空 ID。
- mutation 期间阻止重复提交；成功后只更新或失效受影响缓存，并以权威回读确认终态。
- 实时状态使用 `liveObservationQueryPolicy`；失败展示 `unavailable`，不得保留旧成功值。
- 轮询复用 `lib/polling.ts`；终态、页面隐藏或条件不适用时停止。长连接在 cleanup 中关闭。

## 5. React、表单与 Effect

- render 必须纯净：不得请求、toast、导航、写 storage、改 DOM 或修改输入对象。
- Hook 只在组件或自定义 Hook 顶层调用。用户动作副作用放事件处理器；Effect 只同步 DOM、媒体查询、
  EventSource、WebSocket、Observer、计时器等外部系统。
- 能从 props、Query 或 state 计算的值在 render 派生。禁止用 `useEffect + setState` 复制数据、
  回填默认值、修剪选项或重置筛选/页码。
- Effect 依赖完整、cleanup 对称，并能安全经历开发模式重复挂载；列表使用稳定领域 ID 作为 key。
- `memo`、`useMemo`、`useCallback` 只服务于已测量成本或稳定引用契约，正确性不依赖 memoization。
- 新增或修改表单使用 React Hook Form、`zodResolver` 与 Zod；从 schema 推导表单类型。
- `defaultValues` 完整稳定；打开编辑器或切换资源时一次 `reset` 建立基线，不逐字段 Effect 同步。
- 原生输入优先 `register`；Select、Checkbox、Switch 等按需使用 `Controller`，不得双重注册。
- RHF 直接拥有的动态行使用 `useFieldArray` 和 `field.id`；提交通过 `handleSubmit`。
- 前置条件缺失、表单无效或 mutation pending 时主操作 disabled；字段错误与控件通过 ARIA 关联。
- Secret、Token 和密码不回显；留空表示不修改，清除必须是服务端支持的显式动作。

## 6. UI 与设计系统

- 新增基础组件前先检查 `src/components/ui`、`components.json` 和 shadcn/ui 官方目录；有现成 primitive
  时不手写同类组件。修改 Radix/shadcn 组件必须保留键盘、焦点、ARIA、Portal 和受控语义。
- 项目已安装组件以 `src/components/ui` 和 `components.json` 为准，不维护人工组件清单或引入愿望单。
- shadcn/ui 不提供完整平台资源选择器；统一复用
  `components/common/search-select.tsx` 的 `SearchSelect` / `SearchMultiSelect`。固定小枚举继续用
  `Select` / `NativeSelect`，带独立策略的选项保留领域编辑器。
- Button/Input/Select 使用标准控件圆角；Badge、状态标签、头像和真正的 segmented control 才使用
  胶囊形。页面导航与表单控件不得伪装成 Badge。
- 页面使用 `PageShell` 的 `content/full/settings/tool` 语义；标题、工具和 Tabs 使用 `PageChrome` /
  `ContentTabs`。全局画布 padding 由布局层提供，业务页不加根 padding、负间距或独立全宽 topbar。
- 区块优先 `Surface`、`Section`、`MetricGroup`；列表唯一外壳是 `DataList`。不嵌套无业务意义 Card。
- 搜索、筛选和排序放 DataList 工具区；创建操作放 PageChrome。`total === 0` 时不显示分页。
- Loading、empty、error 互斥并分别使用结构化 skeleton、`EmptyState`、`ErrorState`；首次空状态给下一步，
  筛选为空给清除入口。
- 编辑、删除、测试和绑定使用独立按钮或菜单；整行点击只用于明确详情导航并支持键盘。
- 状态使用 `StatusBadge` / `StatusValueBadge` 与语义 tone；业务代码不硬编码 red/green/amber 状态色。
- 间距、圆角、颜色、表面和动效使用 Tailwind 标准 token 或 `styles/design-tokens.css`。普通 Surface
  不加常驻边框/阴影，阴影只用于 Dialog、Popover、悬浮层与 raised 状态。
- 同页或 Tab 默认最多一个实心主操作。同级动作使用 outline、ghost 或菜单。
- 页面不得读取“标准/简约/跟随平台”偏好或建立局部主题分支；“Project”在 UI 统一称“项目空间”。

## 7. i18n、可访问性与安全

- 标题、描述、按钮、菜单、表单、toast、错误、空状态、确认、`aria-label`、Zod 文案和状态 badge
  全部使用 i18next。语言和 bundle 事实以 `src/i18n/config.ts` 与实际资源为准。
- 新 key 在所有支持语言中保持结构和插值一致；不拼接译文。日期、金额、数字和列表使用 `Intl` 或现有 helper。
- 动态 key 只为真实 AST 无法展开的调用在 `scripts/i18n-dynamic-key-allowlist.mjs` 精确登记，不使用宽泛豁免。
- 使用原生 button/a/input/table/fieldset；每个输入有 label，hint/error 用 `aria-describedby`，图标按钮有
  本地化名称。Dialog/Sheet 保留 Title 和 Description。
- 所有操作支持键盘和 `focus-visible`；Tooltip 同时支持 hover/focus，颜色不是唯一状态信息；
  动画尊重 `prefers-reduced-motion`。
- 视觉变化检查桌面、移动、浅色、深色、键盘焦点、长文案和 loading/empty/error。
- `VITE_*` 只能承载公开构建配置。Session 凭据不复制到 localStorage、URL、日志或遥测。
- 不渲染未经清洗的 HTML；外链校验协议并设置安全 `rel`；用户输入和敏感正文不进入遥测。
- 前端权限只改善体验；后端仍执行认证、资源归属、审批与审计。

## 8. 依赖、测试与验证

- 从根目录使用 `pnpm --dir web ...`；只维护 `web/pnpm-lock.yaml`，常规安装使用
  `--frozen-lockfile`。依赖变更用 pnpm 命令生成最小锁文件 diff。
- 新依赖先评估现有能力、bundle、初始化、维护和许可证；单例依赖变更还要检查 `pnpm list`，
  必要时仅用最小范围 workspace override 对齐依赖族。
- 测试以用户可观察行为和 DOM 为主：优先 role/label/text 查询，异步交互必须 await，
  `waitFor` 内不执行副作用，不用大快照替代行为断言。
- Query 测试使用独立且关闭 retry 的 QueryClient；路由用 MemoryRouter；API 在领域客户端边界 mock。
- 页面/表单覆盖 loading、success、empty、error 和 pending；实时功能覆盖 unavailable、资源切换与 cleanup；
  缺陷修复覆盖原问题。
- 最低验证按风险执行：

| 改动 | 验证 |
| --- | --- |
| 纯函数、Hook、组件 | 就近测试 |
| 页面、路由、Query、表单 | 相关测试、lint、build |
| i18n | `check:i18n`、相关测试、lint、build |
| 依赖 | `check:singletons`、`pnpm list`、test、lint、build |
| 布局与交互 | test、lint、build、浏览器桌面/移动与明暗主题 |
| 权限、Secret、长连接、外部平台 | 根规范要求的完整成功/失败链路 |

完整门禁：

```bash
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web check:singletons
pnpm --dir web build
```

不得通过关闭规则、放宽类型、批量 disable、扩大 i18n 白名单或依赖 bundler 偶然去重来通过门禁。
