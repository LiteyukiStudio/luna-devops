# 任务：按原型重构服务拓扑前端

## 背景

项目服务拓扑目前用 ECharts 环形布局（`layout: 'circular'`），方向和可读性差。已产出一个设计原型稿，现在要把它落地为真实的 React 组件。**功能逻辑保持不变，视觉与交互严格对齐原型。**

## 必读（按顺序）

1. 原型稿（唯一视觉基准，浏览器可打开交互）：`docs-internal/prototypes/topology-redesign.html`
   - 重点看其中的 CSS 设计 token、`.svc-node` 服务卡片、`.lane` 泳道、`.edge-layer` SVG 走线、`.legend` 图例、`.detail-sheet` 详情侧滑
2. 现有实现：
   - `web/src/pages/projects/project-topology-chart.tsx` ← **主要重写对象**（ECharts 环形图）
   - `web/src/pages/projects/project-topology-panel.tsx` ← 面板/工具栏/筛选/空态，需小幅配合
   - `web/src/pages/projects/project-topology-detail-sheet.tsx` ← 详情侧滑，配合新选中逻辑
3. 数据类型：`web/src/api/topology-types.ts`（`ProjectTopologyNode` / `ProjectTopologyEdge`）——**不要改后端，数据已够用**
4. 项目规范：仓库根 `AGENTS.md`（尤其第 6 节前端准则、MUST i18n、设计 token）
5. 状态语义：`web/src/components/common/status-tone.ts`（五档 tone：success/warning/danger/info/neutral）

## 设计规范（从原型提炼，严格遵守）

- **分层 DAG 布局**（Sugiyama）：按边方向做最长路径分层，节点自上而下排列，主调在上、被调在下。替换掉 `layout: 'circular'`。
  - 布局算法：用 ECharts graph 的 `layout: 'none'` + 预计算坐标（layered）。可用重心法排序同层节点减少边交叉。不需要引入新依赖，手写一个轻量分层即可；若考虑 dagre 需先确认许可并在简报里说明。
- **服务卡片节点**（对齐原型 `.svc-node`）：
  - 左侧 3px 分类色条（`::before`），色相只区分服务分类（接入/核心/支撑/基础设施），用 Okabe-Ito 色盲安全色板
  - 头部：分类图标 + 服务名 + 类型说明
  - 底部 meta 行：状态徽章（色点+文字，双重编码，对齐 `StatusBadge`/`status-tone` 语义）、部署目标（如 `prod · 2/2`）、出入度计数（`←in →out`）
  - 不再用满版主色填充节点（现状的大色块要去掉），改为白底卡片 + 细边框 + 分类色条
- **泳道**：每层一条横向泳道，左侧绝对定位的层标签（层名+英文副标+服务数），节点区 `padding-left` 让位，点阵底纹（`radial-gradient` 20px 网格）。标签列不参与连线坐标系，连线不被截断。
- **连线**（对齐原型 SVG 走线）：
  - 正交曼哈顿路由，圆角转角，端点垂直出/入（底边→层间通道→目标顶边）
  - 中途障碍检测：连线不得穿过节点卡片，必要时经泳道底部横向通道绕行
  - 实线=服务绑定（service_binding），虚线=手动声明（manual）
  - 异常状态的边上色（degraded→warning、unavailable→danger），正常边用中性灰
  - 箭头恒定指向被调方；边中点放协议+端口小标签（等宽字体、白色描边压底）
  - 边可点击（宽 hitbox），点击打开对应详情
- **图例**：画布底部一条图例栏，说明 分类色 / 状态徽章 / 实线虚线 / 箭头方向（对齐原型 `.legend`）
- **焦点交互**：点击节点 → 非关联节点和边淡出（degree-of-interest），当前节点高亮描边，并打开详情侧滑；再点一次或关侧滑则还原
- **响应式**：`md` 断点以下隐藏 SVG 画布，复用现有移动端关系列表（`MobileRelationItem`），与现状一致

## 硬性约束（违反会被打回）

- **MUST i18n**：所有用户可见文本走 `react-i18next`，不硬编码。层名、状态、图例、分类、计数、aria-label 全部加进 `web/src/i18n/locales/` 下**全部 5 个语言**（`zh-CN`、`en-US`、`zh-TW`、`ja-JP`、`ko-KR`，注意各有目录和同名 `.ts` 聚合文件）的 `projectTopology` 命名空间（参考现有 `projectTopology.*` key）。分类名、泳道名这些原型里的中文也要做 key。
- **主题**：颜色用 `web/src/styles/design-tokens.css` 的语义 token（`--primary`、`--success`、`--warning`、`--danger`、`--muted`、`--border` 等），适配 light/dark。原型里的具体 hex 是参考，落地时映射到 token。分类定性色板的 4 个分类色是仅有的可保留具体色值之处（属于集中维护的图表色板例外），但也建议定义成 token 或常量集中管理。
- **不堆临时 patch**：环形→分层是布局模型的替换，直接重写 `project-topology-chart.tsx` 的布局与渲染，不要保留 circular 的残留分支。
- **不改后端 API、不改数据类型**（`topology-types.ts` 保持不变）。
- **不改筛选/分页/增删改逻辑**：`project-topology-panel.tsx` 里的 stage/origin/search 过滤、添加关系 dialog、service binding/manual edge 查询都保留，只把渲染层替换掉、选中边的逻辑对接好（现状是 `onSelectEdge` 打开 detail sheet）。
- **不主动 git commit**。改完不要提交。

## 验证（必须全部通过）

```bash
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web check:singletons   # 若动了依赖才需要
```

- 这 5 个 locale 不能有缺 key（构建/tsc 会查）
- 改动的文件数控制在少量几个（chart 为主、panel/detail-sheet/i18n 为辅），不要顺手重构无关模块

## 交付

改完后用一段话总结：改了哪些文件、分层布局算法怎么实现的、状态/分类色如何映射到 token、i18n key 加了哪些、以及 `lint`/`build` 的实际结果。不要贴大段代码。
