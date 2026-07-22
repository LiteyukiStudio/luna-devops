# shadcn/ui 组件清单与使用准则

来源：<https://ui.shadcn.com/docs/components>

更新时间：2026-06-06

## 使用准则

- 项目优先使用 shadcn/ui 官方组件。
- 有 shadcn/ui 现成组件时，不允许手写同类基础组件。
- `web/src/components/ui` 中的基础 UI 组件应来自 shadcn/ui registry 或严格按 shadcn/ui API/结构实现。
- `web/src/components/common` 只放业务组合组件，不放可由 shadcn/ui 直接提供的基础组件。
- 需要自定义组件时，必须满足以下任一条件：
  - shadcn/ui 没有对应组件。
  - 这是业务领域组合组件，例如 ProjectCard、BuildRunTimeline、RegistryCredentialPanel。
  - 这是对 shadcn/ui 组件的薄封装，并且封装能显著减少重复业务代码。
- 自定义组件必须在注释或文档中说明为什么不能直接使用 shadcn/ui。

## 视觉语义

- 普通 `Button`、`Input`、`Select`、`NativeSelect` 和搜索选择器使用标准圆角，不使用胶囊形覆盖。
- `Card`、`Dialog`、`Sheet` 和渐进区块使用同一层级的内容容器圆角；页面区块优先使用留白和分隔线，不嵌套无必要的卡片。
- `Badge`、状态标签、头像和真正的 `SegmentedControl` 可以使用 `rounded-full`。页面导航、行级命令和表单控件不能因为视觉统一而伪装成 Badge 或模式切换。
- 控件高度、间距、圆角、网格和字号必须使用 Tailwind token。除视口比例、CSS 变量或动态计算外，不在业务页面新增任意 `px` 值。
- 桌面管理页保持紧凑密度；移动端保证操作目标可触达，并将复杂筛选收敛为可换行工具栏或 Sheet。

## 官方组件列表

| Component | 本项目优先用途 |
| --- | --- |
| Accordion | 折叠说明、分组详情 |
| Alert | 普通提示、警告提示 |
| Alert Dialog | 危险确认弹窗，替代手写 ConfirmDialog |
| Aspect Ratio | 固定比例媒体区域 |
| Avatar | 用户头像、身份展示 |
| Badge | 状态、角色、scope 标签 |
| Breadcrumb | 层级导航 |
| Button | 所有按钮 |
| Button Group | 成组操作按钮 |
| Calendar | 日期选择 |
| Card | 内容容器 |
| Carousel | 轮播展示，MVP 少用 |
| Chart | 图表 |
| Checkbox | 多选和布尔表单 |
| Collapsible | 可折叠区域 |
| Combobox | 搜索选择资源 |
| Command | 命令面板、搜索选择 |
| Context Menu | 右键菜单 |
| Data Table | 大列表、可排序/分页/筛选表格 |
| Date Picker | 日期选择 |
| Dialog | 普通弹窗 |
| Direction | RTL/LTR 方向控制 |
| Drawer | 移动端抽屉 |
| Dropdown Menu | 下拉操作菜单 |
| Empty | 空状态，替代手写 EmptyState |
| Field | 表单字段布局、label、description、error |
| Hover Card | hover 详情 |
| Input | 输入框 |
| Input Group | 带图标/按钮/前后缀输入框 |
| Input OTP | 一次性验证码输入 |
| Item | 行项目展示 |
| Kbd | 快捷键展示 |
| Label | 表单 Label |
| Menubar | 菜单栏 |
| Native Select | 原生 select |
| Navigation Menu | 顶部/分组导航 |
| Pagination | 分页控件 |
| Popover | 浮层 |
| Progress | 进度 |
| Radio Group | 单选组 |
| Resizable | 可调整大小布局 |
| Scroll Area | 滚动区域 |
| Select | 下拉选择 |
| Separator | 分割线 |
| Sheet | 侧边抽屉 |
| Sidebar | 应用侧边栏布局，替代手写侧边栏 |
| Skeleton | 加载骨架 |
| Slider | 滑块 |
| Sonner | Toast 通知 |
| Spinner | 加载状态 |
| Switch | 开关 |
| Table | 基础表格 |
| Tabs | 标签页 |
| Textarea | 多行输入 |
| Toast | Toast 旧方案；本项目优先 Sonner |
| Toggle | 切换按钮 |
| Toggle Group | 分段/多选切换 |
| Tooltip | 字段说明、图标说明 |
| Typography | 排版样式 |

## 当前项目替换优先级

已完成：

- `Button`：`web/src/components/ui/button.tsx` 已改为 shadcn `cva` 结构。
- `Card`：`web/src/components/ui/card.tsx` 已改为 shadcn Card 组件族。
- `Badge`：`web/src/components/ui/badge.tsx` 已替代旧 `status.tsx`。
- `Input`、`Textarea`、`Native Select`、`Field`、`Label`、`Tooltip`：表单基础组件已拆分到 shadcn 组件文件，业务字段组合改为 `components/common/form-field.tsx`。
- `Alert Dialog`：`ConfirmDialog` 已改为组合 `web/src/components/ui/alert-dialog.tsx`。
- `Empty`、`Alert`：空状态和错误状态已切到底层 shadcn 组件。
- `Table`、`Pagination`：`DataList` 内部已改为组合 shadcn Table/Pagination。
- `Sidebar`、`Separator`：`AppLayout` 侧边栏已改为组合 shadcn Sidebar/Separator。
- `Input OTP`：邮箱注册、MFA 绑定和敏感操作验证统一使用 shadcn Input OTP，并保留系统一次性验证码自动填充语义。

高优先级：

- `Navigation Menu`：后续如引入顶部导航或更复杂菜单，再替换对应导航区域。

中优先级：

- `Avatar`、`Dropdown Menu`：优化侧边栏用户信息和用户操作。
- `Tabs`：用于账号安全、应用详情、镜像站详情等多块内容页面。
- `Sheet`、`Drawer`：用于移动端导航和编辑侧栏。
- `Skeleton`、`Spinner`：统一加载状态。
- `Combobox`、`Command`：用于 Git 仓库、镜像站、项目空间、用户等资源搜索选择。

## 资源选择组件

shadcn/ui 没有直接提供满足平台资源筛选需求的完整单选和多选组件。项目统一在 `web/src/components/common/search-select.tsx` 中组合 `Popover`、`Input`、`Button` 和选择状态，提供：

- `SearchSelect`：可搜索单选，适用于项目空间、用户、Git 分支等资源。
- `SearchMultiSelect`：可搜索多选，适用于跨项目空间筛选、批量选择用户、事件维度筛选等场景。
- 两者共用候选项结构、搜索、空状态、加载状态、最大展示数量和无障碍触发器。
- 项目空间、用户等领域对象应通过 `components/common` 下的薄包装器转换为统一候选项，不要在页面内重新实现 Popover 候选列表。
- 少量且固定的互斥枚举，例如启用状态、TLS 模式、语言和角色，继续使用 shadcn `Select` 或 `NativeSelect`；不要为了统一外观强行改成可搜索资源选择器。
- 某个选中项还带独立策略、编辑按钮或额外字段时，应保留专门的业务编辑器，例如部署配置的运行配置集引用策略。

低优先级：

- `Chart`、`Calendar`、`Date Picker`、`Resizable`、`Carousel`：等对应业务需要出现后再引入。

## 例外

允许保留业务组合组件，例如：

- `PageHeader`
- `AuthErrorPage`
- `ForbiddenPage`
- `AccessTokensPanel`
- `RegistryCredentialPanel`
- 未来的 `BuildRunTimeline`

这些组件内部也应尽量组合 shadcn/ui 基础组件。
