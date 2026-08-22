# 配置访问入口

访问入口把域名和路径转发到已经发布的应用 Service。只需集群内访问的服务可以不创建。

## 创建入口

1. 选择项目空间、应用和部署阶段。
2. 选择集群中可用的 Gateway。
3. 填写域名、路径和目标端口。
4. 按实际链路选择 HTTP 或 TLS 模式并提交。

## TLS 选择

| 场景 | 选择 |
| --- | --- |
| CDN 或外层代理已经处理 HTTPS | 上游代理已终止 TLS |
| Gateway 使用已有证书 | Gateway 终止 TLS |
| 已配置 cert-manager HTTP-01 | HTTP Challenge 证书 |
| 已配置 DNS-01 通配符证书 | 集群中的通配符证书 |

平台使用集群中已有的 Gateway、Issuer 和证书资源，不会创建 ACME 账号或 DNS Provider 凭据。

创建后检查下发状态、DNS、证书、Service 端口和应用健康状态，再使用浏览器或 `curl` 发起真实请求。若应用生成了错误的 `http` 回调地址，请让管理员检查 CDN、反向代理和 Gateway 是否正确传递外部协议。
