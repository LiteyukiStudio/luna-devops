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

## 验收

涉及公共视觉组件、页面模板或主题 token 的改动，至少检查：

- light 与 dark 模式。
- 默认主题色和一个非默认主题色。
- 1440px、1024px 和 390px 宽度。
- 空状态、异常状态、横向溢出和长文本。

视觉整改不能改变原有查询、权限、校验、提交和路由行为。
