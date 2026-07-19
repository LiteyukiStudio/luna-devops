---
name: luna-devops-notifications
description: Luna DevOps 通知系统操作。用于 notification presets、channels、templates、rules、deliveries、测试通知、投递失败诊断和事件订阅配置。
---

# 通知 Skill

## 适用能力

- notification presets 查看。
- channels 创建、更新、删除、测试。
- templates 创建、更新、删除。
- rules 创建、更新、删除。
- deliveries 查询和失败诊断。

## 操作流程

1. 先确认用户想通知哪些事件。
2. 选择 channel preset 或自定义 channel。
3. 配置 template 和 rule。
4. 保存后发送 test notification。
5. 投递失败时检查 delivery status、channel 配置、外部 webhook/SMTP 响应。

## 风险边界

- Webhook URL 和 SMTP secret 不回显。
- 删除 channel/template/rule 会影响告警，应要求 confirmation。
- 测试通知可能触达外部系统，执行前说明目标。

