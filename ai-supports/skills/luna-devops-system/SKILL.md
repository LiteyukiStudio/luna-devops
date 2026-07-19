---
name: luna-devops-system
description: Luna DevOps 系统管理操作。用于站点配置、公开配置、数据保留、应用市场、系统组件安装、平台设置和系统级诊断。
---

# 系统管理 Skill

## 适用能力

- public configs 和 configs definitions。
- 站点 title、logo、favicon、登录页配置。
- data retention catalog、preview、cleanup。
- app templates 列表和安装。
- system components 列表和安装。

## 操作流程

1. 先确认是否需要平台管理员权限。
2. 修改站点配置前读取当前值并给出差异。
3. 数据保留 cleanup 前必须先 preview。
4. 应用市场安装前确认目标 project/application/runtime 配置。
5. 系统组件安装前检查已有安装状态。

## 风险边界

- data retention cleanup 是 critical，必须 confirmation + MFA step-up。
- 系统组件安装可能修改集群资源，至少 high risk。
- 站点配置会影响所有用户，至少 medium risk。

