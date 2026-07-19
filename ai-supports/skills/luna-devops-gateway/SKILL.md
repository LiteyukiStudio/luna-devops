---
name: luna-devops-gateway
description: Luna DevOps 访问入口和网关操作。用于 gateway routes、domain check、Gateway API、HTTPRoute、TLS、证书、访问 URL 和网关故障诊断。
---

# 网关 Skill

## 适用能力

- gateway routes 列表、创建、更新、删除。
- domain check。
- Gateway/HTTPRoute/Service 关联诊断。
- TLS 和证书状态解释。

## 操作流程

1. 确认 project、application、deployment target 和 service port。
2. 创建 route 前检查域名、path、protocol 和目标服务端口。
3. 发布后检查 route status、certificate status、HTTPRoute events。
4. 访问失败时先看 route，再看 Service 和 Pod readiness。

## 风险边界

- 修改或删除 gateway route 会影响公网访问，至少 medium risk。
- 泛域名证书使用 DNS-01；HTTP challenge 不支持 wildcard。
- 不把内部服务地址误认为公网访问地址。

