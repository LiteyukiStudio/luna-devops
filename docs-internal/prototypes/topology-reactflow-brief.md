# 任务：用 React Flow + dagre 重写服务拓扑图，并把「仅绘图」做成一等能力

## 背景

项目服务拓扑图上一轮刚从 ECharts 环形布局改成手写 SVG 分层 DAG（`project-topology-chart.tsx`，630 行手写 Sugiyama 分层 + 正交曼哈顿走线 + DOM 测量 + 障碍检测绕行）。现在决定**不自己造轮子**，改用 React Flow + dagre 重写渲染层。拓扑非核心功能，允许直接替换、不需要保留旧实现。

同时把「仅绘制拓扑、不创建环境变量引用」从藏起来的 manual 下拉，提升为清晰的一等选项（纯前端 UX 改进，**不动后端**）。

## 技术选型（已确认兼容）

- 项目：React `^19.2.6` + Tailwind CSS v4 + TypeScript + Vite + `@/` 路径别名 + react-i18next + TanStack Query
- 新增依赖：`@xyflow/react@^12`（React Flow 官方当前包名，兼容 React 19）、`dagre@^0.8.5`、`@types/dagre`
- 安装：`pnpm --dir web add @xyflow/react dagre && pnpm --dir web add -D @types/dagre`
- **装完必须跑 `pnpm --dir web check:singletons`**，若有重复版本按 AGENTS.md 在 `web/pnpm-workspace.yaml` 加最小范围 overrides

## 必读

1. 视觉基准原型（浏览器可打开）：`docs-internal/prototypes/topology-redesign.html` —— 节点卡片、泳道、走线、图例、焦点高亮的样式都以此为准
2. 现有实现（要被替换）：`web/src/pages/projects/project-topology-chart.tsx`
3. 面板（小幅配合）：`web/src/pages/projects/project-topology-panel.tsx`
4. 关系 dialog（需求 2 主要改动点）：`web/src/pages/projects/project-topology-relation-dialog.tsx`
5. 数据类型：`web/src/api/topology-types.ts`（**不改**）
6. 规范：仓库根 `AGENTS.md` 第 6 节前端准则、MUST i18n、设计 token；状态语义 `web/src/components/common/status-tone.ts`

## Part 1 · React Flow 重写拓扑图

把 `project-topology-chart.tsx` 的手写 SVG 实现换成 React Flow：

- **布局**：dagre 计算分层坐标（`rankdir: 'TB'`，主调在上、被调在下），节点坐标喂给 React Flow。替代掉手写的 Kahn 拓扑排序 + 最长路径分层 + 重心法。
- **节点**：自定义 React 节点组件，**完全复刻现有卡片样式**（白底卡片 + 细边框 + 左侧 3px 分类色条 + 头部图标/名称 + meta 行状态徽章/部署阶段/出入度）。状态徽章继续走 `statusToneFor` 五档 tone + `StatusBadge` 风格类。分类色用 `index.css` 里已有的 `--topology-cat-access/core/support/infra` token。
  - 分类映射：按 dagre 分层后的层索引 0 接入 / 1 核心 / 2 支撑 / ≥3 基础设施（与现状一致）。
- **边**：用 React Flow 边（`smoothstep` 或 `step` 类型，正交走向），实线 = service_binding，虚线（`strokeDasharray`）= manual；异常状态（warning/danger tone）边上色，正常中性灰；箭头恒定指向被调方；边中点放协议·端口小标签（等宽字体）。边可点击 → 调 `onSelectEdge(edge.id)` 打开既有详情侧滑。
- **泳道**：保留分层语义。可用背景分层（React Flow 的节点层下方画泳道分隔）或保留外层 DOM 泳道标签。层标签 = 层名 + 调用深度副标 + 服务数（对齐原型与现有 i18n key `projectTopology.chart.lane*`）。
- **交互**：点击节点 → degree-of-interest 焦点高亮（非关联节点/边淡出，再点还原）；内置 `panOnDrag`/`zoomOnScroll`/`fitView` 替代手写测量。保留 `onSelectEdge` 对外契约不变。
- **图例**：保留底部图例栏（分类色 / 状态徽章 / 实虚线 / 箭头方向），复用现有 `projectTopology.chart.legend.*` i18n key。
- **响应式**：`md` 断点以下仍由 panel 渲染 `MobileRelationItem` 列表（现状逻辑保留），React Flow 画布只在 `md` 以上渲染。
- **删除**：手写的 Sugiyama / 走线 / DOM 测量 / ResizeObserver / 障碍检测代码全部删掉，不留残留分支。

## Part 2 · 「仅绘图，不创建环境变量引用」一等能力（纯前端）

现在 dialog 用 `mode: 'service_binding' | 'manual'` 下拉区分，「manual = 仅绘图、不注入 env」藏得很深。改进：

- 把 mode 选择改成**语义清晰的两个选项**（建议用带说明的卡片式单选或分段控件，而非裸下拉）：
  - **服务引用（注入环境变量）** → 现有 `service_binding` 分支，保留全部 env/port/注入字段
  - **仅绘制拓扑（不注入）** → 现有 `manual` 分支，只需源/目标应用、关系类型、可选协议/端口/描述，**明确提示「仅用于拓扑展示，不会创建环境变量或运行时配置」**
- 提示文案走 i18n，加到 `projectTopology.form.*`。
- **不改后端 API、不改 payload 结构**：`serviceBindingPayload` / `manualEdgePayload` 逻辑保持，manual 本就不发 env 字段。
- 保留编辑态 mode 不可切换、保存后刷新逻辑不变。

## 硬性约束（违反会被打回）

- **MUST i18n**：所有用户可见文本走 react-i18next，5 语言（`zh-CN`/`en-US`/`zh-TW`/`ja-JP`/`ko-KR`，各有目录+同名聚合 `.ts`）的 `projectTopology` 命名空间补齐，无缺 key。优先复用现有 `projectTopology.chart.*` / `form.*` key，新增 key 保持命名风格。
- **颜色走语义 token**（`index.css` / `design-tokens.css`），适配 light/dark；分类定性色板沿用已有 `--topology-cat-*`。
- **不改后端、不改数据类型、不改筛选/分页/CRUD 逻辑**（panel 的 stage/origin/search 过滤、查询、dialog 打开、onSelectEdge 契约都保留）。
- 新 UI 优先用 shadcn/ui 与 `web/src/components/common` 现有组件，不手写轮子。
- 改动聚焦：chart 重写 + relation-dialog 改进 + i18n + package 依赖，不顺手碰无关模块。
- **不主动 git commit**。

## 验证（必须全过）

```bash
pnpm --dir web check:singletons
pnpm --dir web lint
pnpm --dir web build        # 含 tsc，会查 5 语言 i18n 缺 key
```

## 交付

一段话总结：React Flow + dagre 怎么接入的（布局/节点/边/泳道/焦点各怎么映射）、需求 2 的 UX 怎么改的、新增依赖与 singletons 检查结果、新增/复用的 i18n key、lint/build 实际结果。不贴大段代码。
