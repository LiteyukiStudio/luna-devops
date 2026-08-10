# 应用部署面板后续重构方案

## 目标与边界

在不改变用户行为、权限、API 契约和实时状态来源的前提下，继续降低 `web/src/pages/applications/application-deployments-panel.tsx` 的职责密度。本方案采用局部、可逐步回滚的拆分；当前已完成的重量级 Dialog 延迟加载保持不变，不在本轮继续重写面板。

## 当前职责

部署面板目前同时承担以下职责：

- 组合部署配置列表、发布、日志、Web Console、运行配置和仓库绑定 Dialog。
- 管理发布、部署配置、运行配置、仓库绑定四组 React Hook Form 状态。
- 查询 Git 账号与分支、构建选项、镜像模板、运行配置、Hook、集群及工作负载实时资源。
- 聚合多集群实时资源，派生每个部署配置的运行状态、内部地址、最新发布和可执行操作。
- 执行创建发布、回滚、重启、拉取最新镜像、删除配置、保存运行配置、重部署及保存仓库绑定等 Mutation。
- 协调 Dialog 打开、关闭、默认值、异步回填、请求竞态保护和 Query 失效。

## 建议组件与 Hook

| 单元 | 建议文件 | 职责边界 |
| --- | --- | --- |
| 查询 Hook | `web/src/pages/applications/use-application-deployment-queries.ts` | 只声明资源查询、query key、enabled 条件和实时观察策略；输入必须包含 `projectId`、`applicationId`、当前 Dialog 作用域，输出原始查询结果，不持有表单状态。 |
| 实时快照 Hook | `web/src/pages/applications/use-application-runtime-observation.ts` | 按集群与资源 ID 建立查询，生成当前资源键对应的工作负载/Service 快照；资源键变化时旧结果不得进入新快照，断联统一输出 `unavailable`。 |
| 派生模型 | `web/src/pages/applications/application-deployment-view-model.ts` | 纯函数计算最新发布、可部署构建、部署行、摘要和可操作性；不发请求、不弹 toast、不读 React Context。 |
| 部署配置表单 Hook | `web/src/pages/applications/use-deployment-target-form.ts` | 管理默认值、编辑态重置、构建环境回填、运行配置引用和验证状态；通过显式命令响应用户操作，避免用同步 Effect 回填派生值。 |
| 发布表单 Hook | `web/src/pages/applications/use-release-form.ts` | 管理目标与构建选择、默认镜像和提交载荷；目标或构建切换由事件入口更新关联字段。 |
| 运行配置表单 Hook | `web/src/pages/applications/use-runtime-config-form.ts` | 管理配置文件/Secret 文件校验、保存后受影响部署配置及重部署确认状态。 |
| 仓库绑定表单 Hook | `web/src/pages/applications/use-repository-binding-form.ts` | 隔离 Git 账号、仓库、分支搜索和绑定提交状态；查询只在 Dialog 打开且前置选择完整时启用。 |
| Mutation Hook | `web/src/pages/applications/use-application-deployment-mutations.ts` | 统一 Mutation、成功后的精确 Query 失效和安全错误展示；不渲染 Dialog，不保存临时选中行。 |
| Dialog 编排 | `web/src/pages/applications/application-deployment-dialogs.tsx` | 继续作为延迟加载边界和 Dialog 宿主；只接收已准备好的 form、状态和命令，不自行拼接业务查询。 |
| 面板容器 | `web/src/pages/applications/application-deployments-panel.tsx` | 只组合列表、Hook 输出和 Dialog 宿主，并保留 `DeploymentsPanelHandle` 对外命令。 |

## 状态与查询隔离

- 服务器状态只由 TanStack Query 管理；表单草稿、当前 Dialog、待删除对象和日志视图属于本地 UI 状态。
- 所有 query key 必须包含项目空间、应用、集群、资源类型及影响结果的筛选条件；Dialog 专用查询在关闭时禁用。
- 实时查询继续复用 `liveObservationQueryPolicy`，以当前资源键生成快照。资源键变化时取消旧订阅并忽略旧回调，禁止展示上一资源的成功结果。
- Query Hook 不复制 `data` 到本地 state；派生行与摘要由纯函数或 `useMemo` 计算。
- 表单重置、关联字段变化和异步默认值回填由打开 Dialog、选择目标、选择构建等用户事件触发；只有 WebSocket、SSE、计时器等外部同步使用 Effect。

## Mutation、Dialog 与实时观察拆分

- Mutation Hook 只接收稳定 ID 和规范化载荷，返回 `mutate`、pending 状态及成功结果；调用方决定关闭哪个 Dialog。
- Query 失效按 `projectId`、`applicationId`、`clusterId` 精确定位，避免刷新无关页面数据。
- Dialog 继续在打开时动态导入。父级保留 React Hook Form 实例可确保 chunk 加载期间草稿不丢失；加载失败、权限拒绝、空状态和关闭动作使用现有公共边界。
- Web Console 与日志 Dialog 各自拥有 WebSocket/SSE 生命周期。订阅键必须包含 release、cluster、namespace、resource、container；键变化或关闭时先清理旧连接，再允许新快照写入。

## 可独立测试的部分

- 纯模型：最新发布选择、部署行组合、可重部署判定、摘要和端点计算。
- 查询 Hook：query key、enabled 条件、资源切换取消、断联 `unavailable`、不保留旧快照。
- 表单 Hook：新增/编辑默认值、构建与目标联动、Secret 留空语义、异步回填竞态。
- Mutation Hook：提交载荷、精确失效、失败不关闭 Dialog、重复提交保护。
- Dialog 宿主：仅在打开时加载、加载失败与重试、关闭后草稿/连接清理、权限拒绝和空状态。
- 面板集成：列表操作打开正确 Dialog，Imperative Handle 仍可触发创建发布和创建部署配置。

## 拆分顺序、风险与验收

1. **提取纯派生模型。** 风险是摘要或状态计算发生细微变化；验收为既有 fixture 输出逐字段一致，列表交互无变化。
2. **提取实时观察 Hook。** 风险是 query key 或取消时序错误导致旧快照闪现；验收为资源快速切换测试中旧响应不能写入新资源，断联显示 `unavailable`，请求保持 `no-store` 语义。
3. **分别提取四组表单 Hook。** 风险是默认值、dirty 状态或 Secret 留空语义改变；验收为新增、编辑、关闭重开和异步回填测试全部通过，提交载荷与重构前一致。
4. **提取 Mutation Hook。** 风险是失效范围不足或 Dialog 过早关闭；验收为成功后权威列表回读、失败保留草稿、pending 期间禁止重复提交。
5. **收敛 Dialog 宿主接口。** 风险是动态加载期间 form、WebSocket 或 SSE 生命周期被重建；验收为加载中/失败/权限拒绝/空状态完整，打开后草稿不丢失，关闭或资源切换后旧连接不再更新界面。
6. **精简面板容器。** 风险是 Imperative Handle 或列表操作接线遗漏；验收为创建发布、创建/编辑/删除部署配置、日志、Web Console、运行配置和仓库绑定路径的组件测试及浏览器 smoke 全部通过。

每一步应独立完成并验证，不跨步骤同时移动状态和改变行为；任一步无法证明行为等价时停止拆分并回退该步。

## 预计文件与契约影响

预计修改现有 `application-deployments-panel.tsx`、`application-deployment-dialogs.tsx` 及相应测试，并新增上述 Hook/模型文件。现阶段不需要修改后端 API、OpenAPI、luna-agent 或公开文档；若拆分中发现选择器需要超过 100 条或缺少服务端筛选，应另立端到端契约事项，不在前端恢复全量兼容。只有新增用户可见文本时才同步中英文 i18n；行为保持不变时仅更新本方案与 `TODO.md` 的实施状态。
