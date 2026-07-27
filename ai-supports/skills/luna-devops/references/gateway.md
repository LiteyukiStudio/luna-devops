# 网关

## 机器目录

先执行
`luna help catalog category=gateway limit=100 output=json interactive=false agent=true`
读取当前访问入口能力，再用
`luna help command path=<category.tool> output=json interactive=false agent=true`
确认每个工具的字段、风险和服务端支持状态。只使用机器目录返回的工具。

## 工作流

1. 解析项目空间、应用、部署配置、Service 与端口。
2. 读取集群网关配置、域名后缀和已有访问入口。
3. 创建或修改前确认 host、path、外部协议与端口、TLS 模式、parent Gateway 和过滤规则。
4. 按目录能力读取或维护 GatewayRoute，并诊断 Gateway、HTTPRoute、Service、DNS 与证书状态。
5. 变更后验证 route conditions、backend refs、DNS、证书与最终访问 URL。

## 风险与验证

- 不用 `api request` 或 Kubernetes Gateway API 绕过平台业务命令。
- HTTP-01 不支持通配符证书；Issuer、邮箱和证书状态以平台返回为准。
- Header、rewrite、redirect 和 forwarded headers 可能改变安全边界，必须按 Help 权限和风险执行。
- 修改或删除访问入口会影响公网流量，需明确确认目标和回滚方式。
- HTTPRoute 被接受不代表后端可用，还要检查 ResolvedRefs、Service 与 endpoints。
