---
name: luna-devops-topology
description: 通过 Luna CLI 读取应用的实时 Kubernetes 资源拓扑；项目服务关系和 ServiceBinding 仅在机器 Help 出现对应工具后执行。
---

# 拓扑 Skill

先遵循 `luna-devops-cli`。当前可执行能力是应用级 Kubernetes 拓扑读取，不持久化拓扑快照。

## 工作流

1. 解析项目、应用和可选部署配置。
2. 读取应用拓扑，区分 Deployment、Pod、Service、配置、密钥和访问入口。
3. 将返回的节点、边、状态与缺失引用按事实展示。
4. 诊断异常时加载 `luna-devops-runtime` 或 `luna-devops-gateway`，但仍以各自目录能力为限。

## 尚未进入 CLI

- 项目空间服务关系 CRUD
- ServiceBinding 与环境变量注入
- 跨应用自定义依赖边
- 跨项目空间关系

不根据流量或名称自动推断持久依赖关系。
