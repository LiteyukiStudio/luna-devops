---
name: luna-devops-gateway
description: 规划和诊断访问入口、域名、Gateway API、HTTPRoute、TLS 与证书；当前机器 Help 没有 gateway 分类时不得执行平台变更。
---

# 网关 Skill

先遵循 `luna-devops-cli`。当前命令目录没有 `gateway` 分类，因此本 Skill 只用于分析现状、整理输入和规划操作。

## 规划清单

1. 项目、应用、部署配置与 Service 端口。
2. 域名、路径、协议、外部端口与 TLS 终止位置。
3. Gateway、listener、HTTPRoute、证书和 DNS 的预期关系。
4. 创建、修改或删除访问入口的影响与验证步骤。

## 边界

- 不用 `api request` 代替 Gateway 业务命令。
- 不直接调用 Kubernetes Gateway API。
- HTTP-01 不支持通配符证书；具体 Issuer、邮箱和状态以平台返回为准。
- 修改访问入口会影响公网流量，命令进入目录后仍需按风险元数据确认。
