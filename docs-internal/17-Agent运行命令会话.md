# Agent 运行命令会话

## 目标

Agent 诊断容器时可以创建短期、有状态的非 TTY Shell，在多条命令之间保留工作目录和环境变量。
浏览器 Web Console 继续使用原有 WebSocket + TTY 协议，两者不复用传输层。

## 调用链

1. Agent 通过 OpenAPI 工具调用创建会话接口。
2. API 按当前委托用户重新校验 Session、项目角色、资源策略、Web Console 开关、批准和 MFA。
3. API 解析 Release 对应的运行 Pod 与容器，并持有 Kubernetes SPDY exec 流。
4. API 返回包含 owner 的 `sessionId`。后续命令必须带回同一个 ID，并逐条重复第 2 步校验。
5. Broker 串行写入 Shell，通过随机边界标记拆分每条命令的输出和退出码。
6. 完成、取消、超时、空闲超时、绝对超时或 Shell 退出时关闭连接；Agent 完成诊断后应显式关闭。

会话固定绑定用户、登录 Session、Agent Run、项目空间、应用、Release、部署配置和容器。任何边界变化
都会拒绝复用。审计只记录命令长度和 SHA-256，不保存命令正文；返回输出有大小上限。

## 多副本边界

SPDY 流和 Shell 进程只能由创建它的 API 实例持有，不能存入 PostgreSQL 或 Redis。`sessionId` 编码
owner，非 owner 实例会返回稳定的 `runtime.command_session.owner_mismatch`，绝不能静默创建另一条 Shell
或继承不确定状态。

当前部署运行多个 API 副本时，必须对命令会话路径配置会话亲和或按 owner 路由。尚未提供 owner 自动
转发时，调用方收到 owner mismatch 后应关闭旧工作流、重新创建会话并恢复最少的已确认状态；不要宣称
跨副本透明。后续若需要无亲和的水平扩展，应拆出有稳定寻址的专用 Session Broker 服务，而不是尝试把
活连接序列化到 Redis。

## 不采用数据库迁移的原因

数据库无法恢复进程内 Shell、管道和 SPDY 连接。只持久化会话元数据会制造“记录存在但连接不存在”的
伪可用状态，因此当前不增加业务表。权威状态只存在于 owner API 的 Broker；审计日志用于事后追溯。

## 验收重点

- `cd`、环境变量等状态在同一会话内保持。
- 换用户、Session、Run 或资源后拒绝执行。
- 创建和每条命令均需要敏感操作批准与 `runtime_exec` MFA。
- 超时后关闭整个会话，避免继续使用状态未知的 Shell。
- 输出超限时保留边界标记并返回 `truncated=true`。
- 非 owner 实例返回明确冲突；同一会话中的命令严格串行。
