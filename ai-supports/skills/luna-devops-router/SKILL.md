---
name: luna-devops-router
description: Luna DevOps skill 路由器。用于根据用户意图选择并按需加载 workspace、source、registry、build、deployment、topology、runtime、gateway、billing、notifications、security、system、debugging 等模块 skill。
---

# Luna DevOps Skill 路由器

## 路由规则

- 提到“项目空间、成员、概览、项目列表”：加载 `luna-devops-workspace`。
- 提到“Git、GitHub、Gitea、仓库、分支、Webhook、绑定”：加载 `luna-devops-source`。
- 提到“镜像站、Harbor、DockerHub、OCI、镜像 tag、凭据”：加载 `luna-devops-registry`。
- 提到“构建、BuildKit、Dockerfile、构建日志、变量、模板”：加载 `luna-devops-build`。
- 提到“应用、部署配置、发布、回滚、重启、副本、资源限制”：加载 `luna-devops-deployment`。
- 提到“服务依赖、拓扑、ServiceBinding、服务引用、环境变量注入”：加载 `luna-devops-topology`。
- 提到“集群、Kubernetes、Pod、Service、YAML、事件、终端”：加载 `luna-devops-runtime`。
- 提到“域名、访问入口、Gateway、HTTPRoute、证书、TLS”：加载 `luna-devops-gateway`。
- 提到“余额、账单、用量、credits、费率、充值、补偿”：加载 `luna-devops-billing`。
- 提到“通知、渠道、模板、规则、投递”：加载 `luna-devops-notifications`。
- 提到“登录、MFA、OIDC、OAuth、用户、Access Token、scope”：加载 `luna-devops-security`。
- 提到“站点设置、应用市场、系统组件、数据保留”：加载 `luna-devops-system`。
- 提到“为什么失败、怎么排查、日志、状态异常”：加载 `luna-devops-debugging`，并按故障域追加对应模块。

## 加载策略

- 每次只加载当前任务需要的模块 skill。
- 跨模块任务先加载主模块，再按需要加载辅助模块。
- 不要为了“可能会用到”提前加载所有 skills。

