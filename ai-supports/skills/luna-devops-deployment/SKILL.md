---
name: luna-devops-deployment
description: Luna DevOps 应用部署操作。用于 application、deployment target、runtime config、release、rollback、restart、release image candidates、资源配置和发布状态检查。
---

# 部署 Skill

## 适用能力

- applications 创建、更新、删除。
- deployment targets 创建、更新、删除、restart。
- release image candidates 查询。
- releases 创建、列表、日志、runtime logs、rollback。
- replicas、resources、env vars、service ports、runtime profile。

## 部署检查清单

1. 确认 project space、application 和 deployment target。
2. 检查 repository binding、build result、image candidate。
3. 检查 registry、runtime cluster、service ports 和 gateway route。
4. 准备 release plan：target、image、replicas、resources、env vars、route impact。
5. 创建 release 或 rollback 前必须 confirmation。
6. 发布后检查 release status、runtime events、gateway route 和最近 platform events。

## 风险边界

- release create 和 rollback 是 high risk。
- restart 是 medium risk。
- 删除 application/deployment target 是 high risk。
- runtime exec、terminal、data export 第一版不作为普通助手能力开放。

