---
name: luna-devops-router
description: 将 Luna DevOps 请求路由到最少必要的领域 Skills；适用于跨模块任务或尚不能明确归类的 CLI 请求。
---

# Luna DevOps Skill 路由器

## 前置条件

- 先加载 `luna-devops-cli`，完成 CLI、实例和认证检查。
- CLI 不可用时只能分析或规划，不能直接调用 REST、Kubernetes 或第三方 API。
- 领域 Skill 描述的是工作顺序，不保证相关命令已经进入目录；执行前仍须查询机器 Help。

## 路由规则

- 项目空间、成员、概览、看板：`luna-devops-workspace`
- Git、GitHub、Gitea、仓库、分支、Webhook、绑定：`luna-devops-source`
- 镜像站、Harbor、DockerHub、OCI、镜像、凭据：`luna-devops-registry`
- 构建、BuildKit、Dockerfile、构建日志、变量、模板：`luna-devops-build`
- 应用、部署配置、发布、回滚、重启、资源规格：`luna-devops-deployment`
- 服务依赖、拓扑、ServiceBinding：`luna-devops-topology`
- 集群、Kubernetes、Pod、Service、YAML、事件、终端：`luna-devops-runtime`
- 域名、访问入口、Gateway、HTTPRoute、证书、TLS：`luna-devops-gateway`
- 余额、账单、用量、Credits、费率：`luna-devops-billing`
- 通知、渠道、模板、规则、投递：`luna-devops-notifications`
- 登录、MFA、OIDC、OAuth、用户、Access Token、Scope：`luna-devops-security`
- 全局设置、数据保留、应用市场、系统组件：`luna-devops-system`
- 失败、异常、日志或状态诊断：`luna-devops-debugging` 加对应领域 Skill

## 加载策略

- 单领域任务只加载根 CLI Skill 和一个领域 Skill。
- 跨领域任务先加载主领域，再按实际依赖追加。
- 命令缺失时记录能力缺口并停在规划层，不加载更多 Skill 试图绕过。
