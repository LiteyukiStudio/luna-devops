# Luna DevOps AI 支持方案

这个目录用于存放 Luna DevOps 面向 AI 能力的设计文档、工具声明和配套 skills。

第一阶段目标是平台内嵌 AI 助手。外部 MCP 仍然保留在设计里，但应在内部助手、共享 Tool Kernel、二次确认、审计和输出脱敏稳定后再开放。

AI 能力应把 Luna DevOps 后端能力包装成安全 tools，同时继续使用现有 REST API、session/access-token 鉴权、RBAC、审计日志、MFA step-up 和 secret 脱敏作为安全边界。

## 目录结构

```text
ai-supports/
  assistant/
    design.md           平台内嵌 AI 助手设计
  mcp/
    design.md           MCP 接入设计
    security.md         风险、确认、审计和安全策略
    tools.yaml          MCP tool 白名单声明
  skills/
    luna-devops-router/
      SKILL.md          skill 路由器，先判断任务再按需加载模块
    luna-devops-operator/
      SKILL.md          跨模块运维入口
    luna-devops-workspace/
      SKILL.md          工作台、项目空间和成员
    luna-devops-source/
      SKILL.md          代码源、仓库、分支和 Webhook
    luna-devops-registry/
      SKILL.md          镜像站、镜像仓库和凭据
    luna-devops-build/
      SKILL.md          构建、构建模板、变量和日志
    luna-devops-deployment/
      SKILL.md          应用、部署配置、发布和回滚
    luna-devops-topology/
      SKILL.md          服务依赖、自定义拓扑和 ServiceBinding
    luna-devops-runtime/
      SKILL.md          集群、Kubernetes 资源和事件
    luna-devops-gateway/
      SKILL.md          访问入口、域名、证书和 Gateway API
    luna-devops-billing/
      SKILL.md          账单、余额、用量和费率
    luna-devops-notifications/
      SKILL.md          通知渠道、模板、规则和投递
    luna-devops-security/
      SKILL.md          认证、MFA、OIDC、用户和 Access Token
    luna-devops-system/
      SKILL.md          站点设置、应用市场、数据保留和系统组件
    luna-devops-debugging/
      SKILL.md          跨模块诊断和排障
```

## 设计原则

- 先做内部助手，再开放外部 MCP。
- 内部 ADK tools 和未来外部 MCP tools 共用同一个 Tool Kernel。
- 第一版只暴露小而实用的工具集，不自动发布所有 REST endpoint。
- 复用现有后端权限模型，AI tools 不能绕过 Luna DevOps RBAC。
- 删除、计费、secret、runtime exec、terminal、data export 等操作按高风险处理。
- mutation 优先走 preflight 和 confirmation，不直接执行。
- 内部助手使用平台内嵌确认弹窗；外部 MCP 返回平台 confirmation URL。
- tool 输出必须短、结构化、可审计、脱敏。
- 先写工具声明，再在声明后实现 adapter。
- skills 按模块渐进加载：先加载 `luna-devops-router` 判断意图，再只加载当前任务需要的一个或少数模块 skill。

## Skills 覆盖

当前 skills 按平台能力拆成 15 个模块，目标是覆盖 Luna DevOps 的主要用户路径和管理员路径，而不是把所有 REST endpoint 生硬映射成一个大说明。

| 模块 | 覆盖能力 |
| --- | --- |
| `luna-devops-router` | 意图识别、模块分流、按需加载 |
| `luna-devops-operator` | 综合巡检、跨模块运维、安全操作规划 |
| `luna-devops-workspace` | 看板、项目空间、成员、置顶和排序 |
| `luna-devops-source` | Git provider、Git account、仓库、分支、Webhook、代码源绑定 |
| `luna-devops-registry` | 镜像站、凭据、镜像模板、镜像仓库和 tag |
| `luna-devops-build` | build run、build job、构建模板、变量、日志、触发和取消 |
| `luna-devops-deployment` | 应用、部署目标、运行配置、发布、重启、回滚 |
| `luna-devops-topology` | 项目拓扑、ServiceBinding、自定义依赖边 |
| `luna-devops-runtime` | runtime cluster、Kubernetes 资源、YAML、事件、Pod 状态 |
| `luna-devops-gateway` | Gateway route、域名检查、TLS、证书、访问入口 |
| `luna-devops-billing` | 余额、账单、用量、费率、流水、网关流量 |
| `luna-devops-notifications` | 通知渠道、模板、规则、投递和测试 |
| `luna-devops-security` | 登录、MFA、OIDC、OAuth app、用户、Access Token、scope |
| `luna-devops-system` | 站点设置、公开配置、应用市场、系统组件、数据保留 |
| `luna-devops-debugging` | 构建、部署、网关、拓扑、账单、通知、权限排障 |

这些 skill 描述用于内部 Agent 的工具选择和未来 MCP/skill 分层，不替代后端 RBAC、审计、确认和脱敏逻辑。

## 当前方向

内部助手：

```text
前端 AI 小窗
  -> /api/v1/assistant/*
  -> 后端 Agent runtime，第一版使用 ADK Go
  -> shared Tool Kernel
  -> 现有 Luna DevOps services/API
```

未来外部接入：

```text
外部 Agent 平台 / MCP client
  -> /api/v1/mcp
  -> 现有 Luna DevOps Access Token
  -> shared Tool Kernel
  -> 现有 Luna DevOps services/API
```
