# 控制台视觉体系

Luna DevOps 在 shadcn/ui 和 Tailwind CSS 之上增加了一层页面级视觉原语。它们只负责内容层级、布局和视觉语义，不承载查询、权限或提交逻辑。

## 页面模板

新增页面应先归入以下类型，再选择根布局：

| 页面类型 | `PageShell` 宽度 | 推荐结构 |
| --- | --- | --- |
| 资源列表 | `full` | `PageToolbar` + `DataList` |
| 看板与概览 | `content` | 待处理事项 + `MetricGroup` + `Section` |
| 设置 | `settings` | `ContentTabs` + `Section` / `Surface` |
| 日志、终端、拓扑 | `tool` | 工具栏 + 内嵌工作区 |

`DataList` 本身就是列表外壳，不需要额外套一层 Card。页面默认只保留一个实心主操作，搜索、筛选、排序和刷新应放入 `PageToolbar` 或 `ContentTabs.tools`。

## 表面层级

页面使用以下语义 token，不直接为业务容器选择临时背景色：

- `surface-base`：页面底色。
- `surface-raised`：主内容和浮起表面。
- `surface-subtle`：弱分组和 hover。
- `surface-inset`：代码、日志、诊断和筛选工作区。

业务区块优先使用 `Surface` 与 `Section`。`Card` 仍是基础组件，适合独立重复条目，但不应成为所有内容的默认外壳，也不要形成没有业务含义的 Card 嵌套。

浮层统一使用 `shadow-overlay`，需要与页面轻微分离的主表面使用 `shadow-raised`。

## 颜色职责

颜色分为三类：

1. Luna 品牌色用于 Logo 和固定品牌图形。
2. 交互主题色用于按钮、焦点、选中和 tab 指示器，可以跟随站点或用户偏好。
3. 成功、警告、危险和信息状态使用固定语义 token，不受个人主题色影响。

状态请复用 `StatusBadge`、`StatusValueBadge` 或 `Notice`。业务页面不要直接拼写 `red-*`、`amber-*`、`green-*` 等状态颜色；第三方品牌图标、终端和集中维护的图表色板除外。

## 间距与密度

- 页面主要区块使用 `gap-6`。
- 同一主题下的相关区块使用 `gap-4`。
- 表单字段和工具组合使用 `gap-3`。
- 行内按钮、Badge 和图标文字使用 `gap-2`。
- 紧凑元信息使用 `gap-1`。

优先使用 Tailwind 的标准间距与宽度 token，不为局部视觉微调引入任意像素值。

## 表单与操作区

- 设置类表单默认限制在 `max-w-3xl` 至 `max-w-4xl`，不要让短字段随内容区无限拉伸。
- 相关开关可以使用带轻量 `inset` 表面的选项组；字段较多时按业务含义分组，不用一张大 Card 等权包住所有内容。
- 页面表单的提交与取消操作统一使用 `FormActions`。桌面端按钮保持正常宽度并右对齐，移动端按钮才允许全宽。
- 同一设置页的不同 tab 必须把保存操作放在一致位置；默认位于当前表单末尾，不在部分 tab 使用顶栏、部分 tab 使用底部。
- 长表单的操作区使用顶部分隔线结束内容；Dialog 继续使用 `DialogFooter`，登录、注册等单任务窄屏流程可以保留全宽提交按钮。
- 表单按钮不得因为父级是 CSS Grid 而被默认拉伸成整行。

## 加载与空状态

加载状态按页面结构选择公共骨架：

- `AppLoadingState`：会话和公共配置初始化。
- `DataListSkeleton`：资源表头、数据行和分页轮廓。
- `OverviewSkeleton`：关注区、指标组和主次内容区。
- `SettingsSkeleton`：设置页 tab、标签和字段。
- `TemplateGridSkeleton`：应用市场工具栏与模板网格。
- `ToolViewportSkeleton`：iframe、日志、拓扑和终端工作区。

页面框架已经可用时只替换正文区域，禁止用一张大 Card 包裹单行“加载中”。`DataList` 在 `total === 0` 时会隐藏分页控件；首次配置场景使用可行动空状态，搜索或筛选无结果时使用紧凑空状态并提供清除条件入口。

## 操作与移动端

- 同一页面或 tab 默认只有一个实心主操作，其他同级操作使用 outline、ghost 或菜单。
- 看板和概览的异常必须在摘要层使用 `danger`、`warning`、`success` 或 `info` tone，并保留文字说明。
- 桌面筛选超过四项时，移动端使用 Sheet 或弹出层渐进披露；搜索、筛选入口和刷新保留在主页面。
- 高频移动端列表通过 `DataListColumn.mobile` 指定保留列，把次要信息放入主单元格第二行；复杂资源表格才保留横向滚动。
- 开发悬浮工具和其他 fixed/sticky 控件必须避让 Dialog、Sheet、toast、分页和固定操作列。

## 技术词汇

- 外部代码平台统一称为 `Git Provider`，身份认证平台统一称为 `OIDC Provider`，不在同类页面中交替使用“提供商”“平台”和裸 `Provider`。
- `Scope` 作为权限概念时展示为“权限范围”，需要帮助用户对应外部协议字段时写作“权限范围（Scopes）”。平台资源的可见边界继续称为“作用域”。
- 仓库字段优先展示“仓库所有者（Owner）”和“仓库名称（Repo）”；技术专名保留英文并在首次出现时提供中文语义。
- OIDC 的 Claim 字段使用“用户组字段（Group Claim）”“邮箱字段（Email Claim）”等中英组合，避免只显示英文缩写。
- 中文与英文、数字之间保留一个空格；API 枚举、命令、路径和变量名保持原始大小写。

## 验收

涉及公共视觉组件、页面模板或主题 token 的改动，至少检查：

- light 与 dark 模式。
- 默认主题色和一个非默认主题色。
- 1440px、1024px 和 390px 宽度。
- 空状态、异常状态、横向溢出和长文本。

视觉整改不能改变原有查询、权限、校验、提交和路由行为。
