# 配套 Skills

Luna DevOps 使用单一事实源维护领域工作流规程（`skills/`），帮助外部 AI 客户端掌握渐进式单域工具检索、参数消歧、`L2` 两阶段预检锁以及控制台人工边界。尚未连接 MCP Server 时，请先完成 [MCP 接入](./index)。

你可以通过以下两种方式使用 Skills。

## 方式一：通过 MCP 在线加载（零本地安装，推荐）

任何已连接 `/api/v1/mcp` 的客户端无需在本地拷贝文件，即可直接从服务端获取与当前运行版本一致的规程：

- **调用元工具**：按当前任务调用 `load_skill`（单次可传入 1～3 个 `skillId`）。
- **读取 MCP Resources / Prompts**：直接读取入口资源 `luna://skills/luna-devops`、各业务域资源 `luna://skills/domains/<skillId>`，或使用服务端暴露的同名 MCP Prompts。

### 7 个业务域工作流（`skillId`）

| `skillId` | 覆盖范围与典型任务 |
| --- | --- |
| `workspace-iam` | 项目空间、看板、成员角色、网络隔离策略、跨项目服务共享、个人安全摘要与凭据吊销。 |
| `source-registry-build` | 代码仓库探查与绑定、镜像站与推送/拉取凭据、构建变量、构建配置与触发构建。 |
| `application-deployment` | 应用管理、应用市场模板安装、共享运行配置、部署配置规格/环境变量/密钥/挂载卷、扩缩容、发布与回滚。 |
| `runtime-volumes` | 运行集群状态、部署配置一站式诊断、Pod/事件/运行日志、单次容器命令、数据卷（PVC）与部署 Hook。 |
| `gateway-topology` | 访问入口（域名与 TLS 证书观察）、域名冲突与 DNS 检查、项目服务拓扑与服务关系。 |
| `diagnostics-governance` | 跨领域故障排障顺序、全局搜索与事件中心、个人账单与站内信、个人及项目通知规则。 |
| `platform-admin` | 平台管理员全局设置、AI 模型与遥测、用户/OIDC/邮件策略、应用市场模板、平台费率调账与数据保留。 |

## 方式二：本地安装 `SKILL.md`（可选）

如果你使用的客户端支持本地 `SKILL.md` 目录（如 Claude Code、Cursor、Codex、Antigravity），可以将官方仓库 [`LiteyukiStudio/luna-devops`](https://github.com/LiteyukiStudio/luna-devops/tree/main/skills) 中的 `skills/luna-devops` 与 `skills/domains` 一并复制到客户端的 Skills 目录。这样模型在发起第一次 MCP 调用前就能直接识别路由规则与安全门禁。

以 Claude Code 用户级目录 `~/.claude/skills` 为例（其他客户端可将目标路径替换为 `.cursor/skills`、`~/.codex/skills` 或工作区级 `.claude/skills`）：

```bash
git clone --depth 1 https://github.com/LiteyukiStudio/luna-devops.git /tmp/luna-devops
mkdir -p ~/.claude/skills
cp -R /tmp/luna-devops/skills/luna-devops /tmp/luna-devops/skills/domains ~/.claude/skills/
rm -rf /tmp/luna-devops
```

保持 `luna-devops/` 与 `domains/` 同级存放，以便入口 `luna-devops/SKILL.md` 中的相对路径 `../domains/<skillId>/SKILL.md` 能正常解析。平台版本升级后，可重新同步该目录，或直接优先使用 `load_skill` 读取服务端内嵌的最新规程。
