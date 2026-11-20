---
name: gateway-topology
description: 管理公网访问入口 GatewayRoute、域名占用与 DNS CNAME 预检、TLS 证书自动签发与观测、路径重写与重定向互斥规则、项目服务架构拓扑图以及控制环境变量注入与 NetworkPolicy 入站放行的内网服务依赖绑定（ServiceBinding）。当用户绑定域名、配置 HTTPS、设置重写/重定向或打通微服务间内网连接时加载本规程。
---

# 访问入口（GatewayRoute）与服务依赖拓扑规程

## 目录
- [1. 核心入口与双类连线职责区分](#1-核心入口与双类连线职责区分)
- [2. SOP 1：公网访问入口创建、互斥切换与单次回读](#2-sop-1公网访问入口创建互斥切换与单次回读)
- [3. SOP 2：内网服务依赖绑定（ServiceBinding）闭环](#3-sop-2内网服务依赖绑定servicebinding闭环)
- [4. 异步假象与错误恢复速查表](#4-异步假象与错误恢复速查表)

---

## 1. 核心入口与双类连线职责区分

- **高频入口工具**：`network_list_gateway_routes`、`network_check_gateway_domain`、`network_create_gateway_route`、`network_update_gateway_route`、`network_delete_gateway_route`、`network_get_topology`、`network_list_service_bindings`、`network_save_service_binding`。
- **严格区分两类拓扑连线（高频概念陷阱）**：
  - **`network_save_service_binding`（真实运行时绑定）**：实际控制两件事——① 向调用方（`sourceTarget`）容器自动注入目标服务地址环境变量及跨服务 Secret 映射；② 在 `ingressMode: "dependencies"` 模式下自动放行被调方（`targetTarget`）的 Kubernetes `NetworkPolicy` 入站流量。
  - **`network_save_topology_edge`（纯展示架构连线）**：**仅用于在拓扑画布上绘制说明性连线**，不注入任何环境变量，也不改变任何防火墙或网络策略。业务服务调用数据库/Redis/下游微服务时，**必须使用 `network_save_service_binding`**。

---

## 2. SOP 1：公网访问入口创建、互斥切换与单次回读

### 2.1 创建前硬门禁（Gate）
- [ ] **目标已发版且非内网专属**：调用 `delivery_get_target` 确认目标 `internalOnly: false`，且至少完成过一次发布（已生成底层 Kubernetes Service），同时核对 `servicePorts` 中的真实端口号。
- [ ] **自定义域名与 DNS 预检**：使用自定义域名时先调 `network_check_gateway_domain` 检查全局域名占用冲突与 DNS CNAME 解析状态。
- [ ] **TLS 模式精确映射**：`tlsMode` 仅允许三个枚举值——`http-only`（纯 HTTP 不启用 TLS，对应口语 `none`）｜`http-challenge`（自动申请并续期 ACME HTTP-01 证书）｜`manual-cert`（使用手动证书）。
- [ ] **多域名拆分原则**：单条 `GatewayRoute` 固定对应单个 `host`；若需为同一服务绑定备用域名，调用 `network_create_gateway_route` 新建独立路由，不要直接覆盖原生产主域名。

### 2.2 增量更新与重写/重定向原子互斥切换（`network_update_gateway_route`，`L2`）
Kubernetes Gateway API 规定同一路由上 `urlRewrite` 与 `requestRedirect` 严格互斥：
- 从存量 `urlRewrite` 切换为 `requestRedirect` 时，必须在**同一次**调用中显式传入 `"clearUrlRewrite": true`；
- 从存量 `requestRedirect` 切换为 `urlRewrite` 时，必须在**同一次**调用中显式传入 `"clearRequestRedirect": true`。

```json
{
  "project": "core-backend",
  "route": "api.example.com",
  "clearUrlRewrite": true,
  "requestRedirect": { "scheme": "https", "statusCode": 301 }
}
```

### 2.3 路由与证书异步对账（禁止循环轮询）
新建或修改路由后，后台网关控制器与证书组件异步完成下发与 HTTP-01 验证：**只需调用 `network_list_gateway_routes`（可传 `route` 精确回读单条）1 次**，确认配置已落库并读取实时 `status`、`observationCode`、`conditions` 与 `certificateStatus`。刚创建时处于 `progressing` 属于正常异步收敛过程，向用户说明 DNS/证书生效预期即可，**严禁在单轮对话中循环轮询等待状态变绿**。

---

## 3. SOP 2：内网服务依赖绑定（`ServiceBinding`）闭环

将调用方（如 `user-api` 的 `prod`）连接到同项目中间件或下游服务（如 `redis` 的 `prod`）的标准低自由度序列：

1. **选择匹配的协议与注入模式**：
   - PostgreSQL / MySQL / Redis 等 TCP 中间件：`protocol: "tcp"` + `injectionMode: "host_port"`，显式指定大写环境变量名 `hostEnvVar` 与 `portEnvVar`；
   - HTTP/HTTPS 微服务：可选 `injectionMode: "url"` 并指定 `urlEnvVar`（如 `ORDER_SERVICE_URL`）。
   - 如需跨服务引用被调方已配置的加密密钥（如数据库密码），通过 `upsertCredentialMap` 按键名安全映射，**严禁手抄明文密码**。

```json
{
  "project": "core-backend",
  "sourceApplication": "user-api",
  "sourceTarget": "prod",
  "targetApplication": "redis",
  "targetTarget": "prod",
  "targetPort": 6379,
  "protocol": "tcp",
  "injectionMode": "host_port",
  "hostEnvVar": "REDIS_HOST",
  "portEnvVar": "REDIS_PORT"
}
```

2. **调用方合并发版生效（Gate）**：
   - `network_save_service_binding` 保存成功后，`NetworkPolicy` 会自动异步对账放行；但**新增或变更的连接环境变量不会热注入已运行容器**——检查返回中的 `requiresRelease: true`，待所有配置就绪后对**调用方（`sourceTarget`）**执行一次 `delivery_deploy_target`。

---

## 4. 异步假象与错误恢复速查表

| 现象或错误码 | 根因 | 标准恢复动作（含禁止行为） |
| --- | --- | --- |
| 创建路由报目标尚未部署或无可用 Service | 目标环境仅创建了草稿，未执行过 `delivery_deploy_target` | 先完成首次发布生成 K8s Service 后再创建 `GatewayRoute` |
| 创建路由报目标为 `internalOnly` | 内网专属环境禁止绑定公网入口 | 与用户确认是否确实需要对外暴露；若是则先调 `delivery_update_target_runtime` 设 `internalOnly: false` |
| 更新路由报 `urlRewrite` 与 `requestRedirect` 冲突 | 未在切换时显式清除另一方存量规则 | 在同一次 `network_update_gateway_route` 请求中补上 `clearUrlRewrite: true` 或 `clearRequestRedirect: true` |
| 新建路由后 `status` 为 `progressing` 或证书签发中 | Gateway 控制器与 ACME HTTP-01 验证存在秒至分钟级异步延迟 | **禁止连续轮询**；核对 `network_check_gateway_domain` 确认 CNAME 解析无误后如实告知异步签发状态 |
| 保存了 `ServiceBinding` 但调用方容器报读不到 `REDIS_HOST` | 未对调用方触发新 Release 重建 Pod 注入新环境变量 | 对 `sourceApplication` 的 `sourceTarget` 执行 `delivery_deploy_target` |
