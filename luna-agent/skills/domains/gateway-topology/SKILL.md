---
name: gateway-topology
description: 访问入口 GatewayRoute、域名检查、TLS 证书核验、应用拓扑、项目依赖边与 ServiceBinding 工作流。
---

# 访问入口与服务拓扑工作流

## 适用范围

处理项目网关路由（GatewayRoute）查询与配置、域名可用性检查、TLS 模式与证书状态核验、项目与应用的拓扑关系读取、显式拓扑边（TopologyEdge）维护及服务绑定（ServiceBinding）配置与连通性检查。

## 标准工作流

1. **访问入口配置与流量安全**：
   - 创建或修改访问入口前，先核对目标应用、部署目标、Service 端口、域名后缀、路径前缀与 TLS 模式，必要时先调用域名检查工具确认域名未冲突。
   - HTTP-01 自动签发不支持通配符域名；网关路由创建、更新与删除直接影响外部流量，均属 `L2` 逐次审批操作。
   - 访问入口核验不能只看配置保存成功：必须检查实时观察状态（`status`、`observationCode`、`conditions`、`certificateStatus`），区分路由未就绪、证书签发中、后端 Service 无可用副本与网关观测不可达。
2. **服务拓扑与依赖治理**：
   - 区分三类关系：集群实时计算的资源关联（Deployment/Pod/Service/Route）、仅用于架构可视化的拓扑边、以及会向源应用注入连接信息并可能触发重部署的服务绑定（ServiceBinding）。
   - 不根据服务命名相似或环境变量猜测自动写入持久化依赖关系；创建或更新 ServiceBinding 后，按需执行绑定检查并回读受影响应用状态。
