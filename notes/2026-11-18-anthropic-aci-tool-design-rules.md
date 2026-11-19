# AI 工具层（ACI）设计准则与防呆规范

记录日期：2026-11-18
参考来源：Anthropic *Building Effective Agents*、Anthropic *Writing Effective Tools for Agents*、Model Context Protocol (MCP) 工具设计指南，并结合 Luna DevOps 历史线上问题收敛。
用途：作为 Luna DevOps 全平台 AI 领域工具重构的设计标准与最终逐项验收 Checklist。

---

## 一、核心理念：ACI（智能体界面）≠ REST API

传统的 REST API 是为**确定性的前端代码**设计的（例如 React 表单在浏览器内存里持有全部 70 个字段，提交时做整包 `PUT` 覆盖）；而 **ACI（Agent-Computer Interface）** 是为**上下文受限、无状态、一次推理生成 JSON 的概率模型**设计的。

给 AI 设计工具的第一性原理是 **Poka-Yoke（结构性防呆）**：
> 不要试图在 System Prompt 里写几十条“千万不要漏传某某字段、千万不要清空密钥”的警告；**要直接从工具拆分和参数 Schema 上，让这类破坏性失误在物理结构上根本无法发生。**

---

## 二、12 条工具设计硬准则（设计完所有工具后逐条过检）

### A. 工具边界与粒度（防止选错工具、防止大杂烩误伤）

- **R1. 面向业务意图而非数据库表/UI 路由（High-Leverage Intent over Thin CRUD）**
  - 不按前端页面拆分重复工具（如禁止把个人/项目/全局通知拆成 4 套列表和 4 套新建接口，统一收敛为 `scope` 参数）。
  - 把人类眼中的“一件事”（如“绑定代码仓库并同步 Webhook”）封装为一个幂等工具，减少多步串联失误。
- **R2. 独立关注点物理隔离（Sub-Resource Isolation / 防误伤铁律）**
  - 同一个大对象下的独立子域（如部署目标的“算力扩缩容”“容器命令/探针/挂载”“环境变量与密钥”“构建触发规则”）**必须拆分为独立窄工具**。
  - 修改副本数或内存的工具 Schema 中**严禁出现**环境变量、密钥或卷挂载字段，确保改 A 绝对不可能碰坏 B。
- **R3. 破坏性删除 100% 独立（Explicit Destructive Tools）**
  - 所有删除、解绑、移除类不可逆操作必须定义为独立的 `*_delete_*` / `*_remove_*` / `*_unbind_*` 工具（固定 `L2` 逐次参数绑定审批）。
  - 严禁在 `save / update / configure` 工具里塞入 `action: "delete"` 分支，杜绝模型误传枚举或被 `L1` 会话记忆免审放行。
- **R4. 清晰领域前缀与角色可见性隔离（Namespacing & Role Filtering）**
  - 工具名统一采用 `<domain>_<verb>_<resource>`（如 `delivery_scale_target`、`network_upsert_route`），让检索与模型注意力边界一目了然。
  - 平台管理员专属工具（`admin_*`）对普通租户用户在目录层直接不可见（维护 `tenant` 与 `admin` 两套确定性快照，兼顾降噪与 Prompt Cache 稳定）。

### B. 参数结构防呆（彻底根除“误清空配置/密钥”事故）

- **R5. 严禁暴露“整组集合覆盖”，强制使用“逐键增量指令（Keyed Delta）”**
  - 凡是字典、键值、列表型子配置（环境变量、Secret、端口、数据卷挂载、配置文件、Header 规则），**严禁设计成传完整数组覆盖全量数据的参数**。
  - 必须设计为增量指令：`set / upsert`（仅新增或覆盖点名的键/挂载路径，未提及项 100% 原样保留）+ `removeKeys / removeNames`（仅删除显式点名的项）。
- **R6. Write-Only 敏感字段绝不参与常规更新，专用单向指令操作**
  - 模型永远读不到已保存的 Secret 明文，任何要求回传 Secret 字段的更新接口都会导致旧密钥丢失或被假占位符覆盖。
  - 所有密钥只能通过专用参数执行单向语义：`set`（写入新明文）、`generate`（服务端随机生成）、`clear`（显式点名清除某个键）。
- **R7. “省略 / null / 空数组 = 保持现状（No-Op）”，重置必须显式表态**
  - 更新工具中的所有可选字段一旦未传或为空，服务端读取数据库现值原样保留，**绝不能把空字符串或空数组解释为“清空该配置”**。
  - 若确实需要清空某项单值配置（如取消自定义启动命令恢复镜像默认 Entrypoint、关闭某类探针），必须提供显式布尔/枚举开关（如 `resetCommand: true`、`disableLivenessProbe: true`）。
- **R8. 人类可读标识与内部 ID 双模定位（Human-Friendly Locators）**
  - 禁止强迫模型为拿一个内部 UUID 先连跑三轮列表接口。
  - 定位参数同时接受内部主键（`prj_xxx` / `app_xxx`）或可读业务标识（`project: "mall"`, `application: "order-api"`, `stage: "prod"`）。服务端统一解析：唯一命中直接执行；重名或未找到时返回候选列表，引导模型调用 `request_choice`。

### C. 描述、返回值与自纠错护栏（省 Token、高准确率）

- **R9. 工具说明书按“新员工上岗手册”标准编写**
  - 每个工具描述必须明确交代三件事：**什么时候用它**、**什么时候别用它（指路正确的替代工具）**、以及**关键参数格式/单位示例**（如 CPU 写 `"500m"` 或 `"2"`，内存写 `"512Mi"` 或 `"2Gi"`）。
- **R10. 返回值兼顾可读上下文与 Token 预算（Bounded Semantic Output）**
  - 返回结构化业务摘要（同时带资源名称、业务 `identifier` 和内部 `id`、实时状态与 `observationCode`），不倾倒几十 KB 的无用原始序列化大字段。
  - 列表与日志工具内置合理默认上限（如默认 `pageSize=20`、诊断聚合默认截取异常优先 Top 10）并返回总数与截断提示，引导模型按需缩小过滤条件而非全量拉取。
- **R11. 自纠错错误反馈（Self-Correcting Errors）**
  - 参数冲突、状态不允许或资源版本（`revision`）过期时，严禁只抛裸 `400 Bad Request`。必须返回稳定 `errorCode` + 明确的修正指引（例如告知合法枚举值、当前最新 `revision` 或需要先执行的查询工具）。
- **R12. 落库前服务端 Diff 门禁（Server-Side ChangeSet & Approval Diff）**
  - 所有写工具在领域 Service 落库前计算结构化 `ChangeSet`（`added / updated / removed`）：
    1. 不具备删除职责的工具若因代码缺陷算出了非预期的字段清空/删除，服务端直接阻断报错；
    2. 审批卡片直接向用户展示 `before -> after` 差异（特别是飘红提示任何 `removed` 项），实现人机协同的最后一道硬防线。

---

## 三、工具定稿过检表（每批工具确认后核对）

1. [ ] 是否消除了前端页面导向的重复 CRUD？
2. [ ] 更新工具是否做到了局部补丁，且不混杂无关子域字段？
3. [ ] 数组/字典字段是否全部改为了 `set/upsert + removeKeys` 增量语义（零全量覆盖风险）？
4. [ ] Secret 字段是否与普通字段隔离且只用 `set / generate / clear`？
5. [ ] 删除动作是否已拆为独立的 `*_delete_*`（`L2`）工具？
6. [ ] 是否支持 `ID / identifier + stage` 双模定位？
7. [ ] 参数单位、枚举、边界条件是否自解释，无需模型瞎猜？
