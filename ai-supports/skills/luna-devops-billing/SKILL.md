---
name: luna-devops-billing
description: Luna DevOps 账单和计费操作。用于 billing summary、deployment spend、ledger、usage records、rate rules、wallet transactions、external transactions、gateway traffic usage 和 credits 解释。
---

# 账单 Skill

## 适用能力

- billing summary 查询。
- deployment spend、ledger、usage records 查询。
- rate rules 查看和更新。
- wallet transactions 和 external transactions。
- gateway traffic status 和 usage。

## 操作流程

1. 查询余额和本期消耗。
2. 按 user/project/application/deployment target 解释 ledger 和 usage。
3. 对异常计费，比较 usage records、rate rules 和 ledger entries。
4. 充值或补偿写入用户账户，不挂项目空间。
5. 费率调整前说明影响范围和生效时点。

## 风险边界

- `billing:write` 是高风险，默认只允许平台管理员或明确授权流程。
- 不自动给用户补偿或扣费。
- 账单记录不因项目或应用删除而删除。

