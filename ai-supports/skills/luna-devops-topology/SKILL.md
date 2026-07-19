---
name: luna-devops-topology
description: Luna DevOps 服务依赖和项目拓扑操作。用于 project topology、ServiceBinding、ProjectTopologyEdge、服务引用、手工关系、环境变量注入、依赖诊断和重新发布提示。
---

# 服务拓扑 Skill

## 适用能力

- 项目空间拓扑读取和筛选。
- ServiceBinding 创建、更新、删除、检查。
- ProjectTopologyEdge 创建、更新、删除。
- 服务依赖状态解释：`ready`、`pending_release`、`invalid`、`unavailable`、`declared`。

## 两类关系

- 服务引用：影响部署结果，为源 deployment target 注入目标服务地址。
- 手工关系：只展示逻辑关系，不注入环境变量，不触发发布。

## 操作流程

1. 确认源应用和目标应用。
2. 如果需要运行时地址，使用 ServiceBinding。
3. 如果只是画架构关系，使用 ProjectTopologyEdge。
4. ServiceBinding 必须选择源/目标 deployment target、target port、protocol、injection mode。
5. 保存 ServiceBinding 后提示需要重新发布源 deployment target。
6. 诊断时调用 check，区分 Service 不存在、端口不匹配、Endpoint 不可用、跨集群等问题。

## 安全边界

- 不把用户名、密码、token 拼进服务地址。
- 跨项目空间和跨集群 ServiceBinding 第一版不支持。
- 删除被引用服务前必须先检查影响列表。

