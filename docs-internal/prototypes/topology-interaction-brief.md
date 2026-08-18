# 任务：服务拓扑图交互与连线视觉迭代（纯前端）

## 背景

服务拓扑图已用 React Flow + dagre 实现（`web/src/pages/projects/project-topology-chart.tsx`）。本次做一轮交互与连线视觉迭代，**不改后端、不改数据类型、不改布局算法**。所有改动聚焦 chart 组件，必要时小幅配合 panel。

## 必读

1. 当前实现：`web/src/pages/projects/project-topology-chart.tsx`（React Flow + dagre，自定义 `service` 节点 / `topology` 边 / `lane` 泳道背景节点）
2. 面板：`web/src/pages/projects/project-topology-panel.tsx`（选中边逻辑、`onSelectEdge`、跳转服务详情页的 `Link` 先例）
3. 详情侧滑：`web/src/pages/projects/project-topology-detail-sheet.tsx`（边详情）
4. 视觉基准原型：`docs-internal/prototypes/topology-redesign.html`
5. 规范：仓库根 `AGENTS.md` 第 6 节、MUST i18n、设计 token；状态语义 `web/src/components/common/status-tone.ts`
6. 既有循环色板先例：`web/src/pages/settings/agent-observability-chart.tsx` 的 `chartColors[index % length]`

## 需求（逐条实现）

### 1. 连线改为自然曲线
- 把边的路径从 smoothstep 直角圆角改为**自然平滑的贝塞尔曲线**（React Flow 的 `getBezierPath`，或自定义 cubic bezier）。
- 目的：避免多条直角线重叠，让每条服务关系走向清晰可辨。
- 保持：边中点的协议·端口标签、16px 透明热区点击、箭头指向被调方、实线=service_binding/虚线=manual。

### 2. hover 聚焦（双向）
- **hover 某条线** → 高亮这条线 + 它连接的两个服务节点；其余所有节点/边/泳道**淡出**（降低不透明度，"没那么突出"）。
- **hover 某个服务节点** → 高亮这个节点 + 与它相连的所有线 + 那些线另一端的节点；其余淡出。
- 鼠标移开 → 恢复原状。
- 淡出用 `opacity`（如 0.15~0.3）+ 轻微 `saturate` 降低，过渡 200ms ease，与现有焦点态过渡一致。
- hover 聚焦是**临时态**，与现有的点击聚焦（focusedNodeId）独立；hover 优先于或叠加于点击态，实现时注意两者不冲突（建议用单独的 hoveredId state）。

### 3. 点击服务打开服务详情
- 点击服务节点卡片 → 跳转到该应用的服务详情页（路由 `/projects/:projectId/apps/:appId`，参考 panel 中已有的 `Link` 用法）。
- **注意**：现在点击节点是「点击聚焦（focusedNodeId）」。需要协调两者——建议：**点击 = 跳转服务详情**；原「点击聚焦」能力由 hover 聚焦（需求 2）覆盖，因此可以移除点击聚焦，或改为单击聚焦/再次点击跳转。选择一种清晰不冲突的交互，并在交付中说明你选了哪种及理由。
- 点击线段 → 打开线段详情侧滑（现有 `onSelectEdge` 逻辑保留）。

### 4. 双向线
- 当两个服务**互相调用**（存在 A→B 和 B→A 两条边）时，合并渲染为**一条带双箭头的线**（`markerStart` + `markerEnd` 都指向各自被调方），而不是两条分开的线。
- 合并后的双向线：标签可合并展示（如两个协议），点击时打开其中一条的详情（或按 source/target 顺序取第一条）；hover 时按一条线处理。
- 注意：service_binding 和 manual 混在同一对节点间时也要正确归并；单向边保持现状。
- **不改后端、不改数据**：只是渲染层把成对反向边识别出来合并。

### 5. 每线固定循环配色
- 定义一个**语义 token 色板数组**（参考 `agent-observability-chart.tsx` 的 `chartColors`，用 `--primary`/`--color-info`/`--color-warning`/`--color-success`/`--color-danger` 等已有 token，可适当扩充到 6~8 个保证区分度，light/dark 都成立）。
- 给每条边按**稳定规则**分配色板颜色（如按边在排序后数组中的 index `colors[i % len]`），保证同一数据每次渲染颜色一致、相邻边尽量不同色，用完循环。
- 异常状态的边仍可保留语义色（warning/danger）叠加逻辑，或让位于循环色板——选择一种并在交付中说明（推荐：正常边用循环色板，degraded/unavailable 仍用 warning/danger 语义色突出异常，符合"零值异常不按中性展示"的原则）。
- 图例无需为每条线列颜色（太多），但可在交付中说明配色规则。

## 硬性约束（违反会被打回）

- **MUST i18n**：任何新增用户可见文本走 react-i18next，5 语言（zh-CN/en-US/zh-TW/ja-JP/ko-KR）`projectTopology` 命名空间补齐。本次若纯交互无新文案则不强制加 key。
- **颜色走语义 token**，适配 light/dark。
- **不改后端、不改数据类型、不改 panel 的筛选/分页/CRUD 逻辑**；`onSelectEdge` 契约保留。
- 改动聚焦 chart（+ 必要 panel 配合），不碰无关模块。
- **不主动 git commit**。
- React Flow API 使用正确：`getBezierPath`/`getSmoothStepPath` 返回 `[path, labelX, labelY]`；双向箭头用 `markerStart`/`markerEnd`。

## 验证（必须全过）

```bash
pnpm --dir web lint
pnpm --dir web build        # 含 tsc，会查 i18n 缺 key
```
若动了依赖再跑 `pnpm --dir web check:singletons`。

## 交付

一段话总结：曲线怎么换的、hover 聚焦怎么实现（state 设计、与点击态如何协调）、点击服务/线各自行为、双向线如何识别合并、循环色板规则与异常态取舍、lint/build 实际结果。不贴大段代码。
