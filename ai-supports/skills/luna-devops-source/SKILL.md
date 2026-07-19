---
name: luna-devops-source
description: Luna DevOps 代码源操作。用于 Git provider、Git account、repository、branch、repository binding、Webhook 创建和重配、仓库文件读取与构建选项探测。
---

# 代码源 Skill

## 适用能力

- Git provider 管理。
- Git account 授权、刷新和删除。
- 仓库列表、分支列表、文件读取、构建选项探测。
- repository binding 创建、更新、删除。
- Webhook 创建和重新配置。

## 操作流程

1. 确认使用的 Git provider 和 Git account。
2. 选择 repository、branch 和应用。
3. 创建 repository binding 前，确认目标 project/application。
4. 创建 Webhook 前，检查 `PUBLIC_BASE_URL` 是否公网可达。
5. 构建前读取构建选项，确认 Dockerfile、上下文目录和分支。

## 安全边界

- Git token 只允许写入，不回显。
- OAuth callback 类 endpoint 不是普通 Agent 操作入口。
- 删除 Git account 或 repository binding 会影响构建触发，应要求 confirmation。

