---
name: gateway-topology
description: 访问入口 GatewayRoute、域名可用性与 DNS 预检、TLS 证书核验、服务依赖 ServiceBinding 与拓扑连线工作流。
---

# 访问入口与服务拓扑工作流

## 适用范围

处理网关访问入口与域名检查（`network_list_gateway_routes`、`network_check_gateway_domain`、`network_create_gateway_route`、`network_update_gateway_route`、`network_delete_gateway_route`）、项目与应用拓扑图（`network_get_topology`、`network_save_topology_edge`、`network_delete_topology_edge`）以及服务间调用依赖（`network_list_service_bindings`、`network_save_service_binding`、`network_delete_service_binding`）。

## 标准工作流

1. **访问入口配置、互斥规则与异步状态核验**：
   - 创建或修改网关路由前，先确认目标部署环境已部署且非 `internalOnly`，核对端口、域名后缀、路径前缀与 TLS 模式（`http-only` 表示不启用 TLS/纯 HTTP 监听即口语 `none`，`http-challenge` 表示自动签发 HTTP-01 证书，`manual-cert` 表示绑定手动证书），必要时调用 `network_check_gateway_domain` 检查域名占用与 CNAME 解析。
   - `network_update_gateway_route` 采用纯增量合并语义（未传字段原样保留）；注意 Gateway API 中 `urlRewrite` 与 `requestRedirect` 属于互斥规则——从路径重写切换为重定向时，必须在同一次调用显式传入 `clearUrlRewrite: true`（反向切换则传 `clearRequestRedirect: true`）。
   - **禁止循环轮询路由异步状态**：新建或更新路由后由后台控制器与证书组件异步对账，立即回读 `network_list_gateway_routes` **1 次**确认配置入库及实时 `status`（刚创建处于 `progressing` 或证书签发中属正常异步状态）、`observationCode`、`conditions` 与 `certificateStatus` 即可，不要在单轮对话中连续多次轮询等待状态变绿。
2. **服务拓扑与内网依赖治理**：
   - 严格区分仅用于架构展示的说明连线（`network_save_topology_edge`）与实际控制连接环境变量注入及 `dependencies` 模式 NetworkPolicy 入站放行的服务绑定（`network_save_service_binding`）。
   - 新增或修改 `ServiceBinding` 后会自动触发项目 NetworkPolicy 对账；若需将生成的连接环境变量（如 `<PREFIX>_HOST` / `<PREFIX>_PORT`）注入调用方容器，需按响应中的 `requiresRelease: true` 提示对调用方环境执行 `delivery_deploy_target`。
