---
name: luna-devops-operator
description: Luna DevOps 通用运维入口。用于无法明确归类到单一模块的综合操作、跨模块状态查看、项目整体巡检、平台能力解释和安全操作规划。
---

# Luna DevOps 通用运维 Skill

## 使用原则

- 先读取状态，再提出动作。
- 优先使用只读 tools，除非用户明确要求变更。
- 使用 tool 返回的稳定 ID，不要只凭名称猜测目标资源。
- mutation 前说明目标、影响和回滚方式。
- 高风险动作必须走 confirmation。
- 不索要或输出 secret、token、kubeconfig、recovery code、private key。

## 通用流程

1. 确认用户目标属于哪个模块。
2. 如果目标明确，加载对应模块 skill。
3. 若目标跨多个模块，先查看 project、application、build、release、gateway、event 摘要。
4. 汇总事实和风险。
5. 需要变更时，给出最小动作计划并等待确认。

## 模块分流

- 项目空间、成员、概览：加载 `luna-devops-workspace`。
- Git、仓库、分支、Webhook：加载 `luna-devops-source`。
- 镜像站、镜像、凭据：加载 `luna-devops-registry`。
- 构建、变量、模板、日志：加载 `luna-devops-build`。
- 应用、部署配置、发布、回滚：加载 `luna-devops-deployment`。
- 服务依赖、拓扑、ServiceBinding：加载 `luna-devops-topology`。
- 集群、Pod、YAML、事件：加载 `luna-devops-runtime`。
- 域名、Gateway route、证书：加载 `luna-devops-gateway`。
- 账单、余额、用量、费率：加载 `luna-devops-billing`。
- 通知渠道、规则、投递：加载 `luna-devops-notifications`。
- 登录、MFA、OIDC、OAuth、Access Token：加载 `luna-devops-security`。
- 站点配置、应用市场、系统组件、数据保留：加载 `luna-devops-system`。
- 故障排查：加载 `luna-devops-debugging`。

