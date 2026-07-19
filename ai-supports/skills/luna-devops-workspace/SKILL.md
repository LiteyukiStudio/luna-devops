---
name: luna-devops-workspace
description: Luna DevOps 项目空间和工作台操作。用于看板、项目空间、项目成员、项目顺序、置顶、项目概览、项目内资源入口和项目级权限判断。
---

# 项目空间 Skill

## 适用能力

- dashboard 概览。
- project spaces 列表、详情、创建、更新、删除。
- project pins 和项目排序。
- project members 管理。
- 项目概览里的 applications、builds、releases、routes、events 摘要。

## 操作流程

1. 获取当前用户和可见项目空间。
2. 需要进入项目时，确认 `projectId`。
3. 成员管理前确认当前 actor 是否为 Owner/Admin/平台管理员。
4. 删除项目空间前检查应用、部署、网关、账单和拓扑影响。
5. 对项目级 mutation 写入 audit，并返回清晰影响摘要。

## 风险边界

- 删除项目空间是 high risk，必须 confirmation。
- 成员角色变更会影响权限，至少按 medium risk 处理。
- 普通 Viewer 只查看，不创建、修改或删除。

