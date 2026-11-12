# 访问入口：同域名多路径（一域名多应用）可行性与现状缺口

记录日期：2026-11-10
类型：只读调研结论，未改动任何代码

## 1. 目标场景

同一个域名下，不同 path 分别指向不同应用：

```text
example.com/      -> 前端应用（frontend Service）
example.com/api   -> 后端应用（api Service）
```

当前平台做不到，且不是“少一个入口”的问题，而是 host 被当成了访问入口的资源主键。

## 2. Traefik / Gateway API 侧结论：支持

平台下发的是标准 Gateway API 对象，Traefik 的 Gateway API provider 原生支持同 hostname 多条
HTTPRoute 用 path 前缀区分，按最长前缀优先级转发。

```yaml
# 前端
hostnames: ["example.com"]
rules: [{ matches: [{ path: { type: PathPrefix, value: / } }], backendRefs: [{ name: frontend, port: 80 }] }]
# 后端
hostnames: ["example.com"]
rules: [{ matches: [{ path: { type: PathPrefix, value: /api } }], backendRefs: [{ name: api, port: 8080 }] }]
```

- 同 hostname、不同 path 的多条 HTTPRoute 可共存（含跨 namespace），`/api` 比 `/` 更具体。
- 需要剥前缀时用 `URLRewrite.replacePrefixMatch`，Traefik 支持；平台已有 `route.URLRewrite`
  字段（`internal/worker/kube_specs.go` → `internal/provider/kubernetes/gateway_api.go`
  `parseURLRewriteFilter`）。
- 平台自动创建的 Gateway listener 不写 `hostname` 约束，`AllowedRoutes: FromAll`，不会挡第二条路由。
- 唯一前提是两条路由挂到同一个 listener（同一 `sectionName`），当前默认都落 `web`/`websecure`。

即：手工 `kubectl apply` 上述两条 HTTPRoute，今天就能跑通。

## 3. 平台现状为什么不支持

### 3.1 硬阻塞：host 全局唯一校验

`internal/api/gatewayapi/gateway_route_validation.go:71`：

```go
if h.gatewayHostExists(host, routeID, ctx.Request.Context()) {
    writeError(ctx, http.StatusBadRequest, "域名已被占用")
```

判据只有 host，不看 path（`gateway_route_domain.go` 的 `gatewayHostExists`）：

```go
query := h.dbWithContext(ctx).Model(&model.GatewayRoute{}).Where("host = ? and id <> ?", host, routeID)
```

域名可用性检查同口径，且不带 project 过滤（全集群唯一，`gateway_route_domain.go:44`）：

```go
Where("host = ?", host) // -> status = "conflict"
```

表结构印证了这是设计意图：`migrations/000103_baseline.up.sql` 只有普通索引
`idx_gateway_routes_host`，没有 `(host, path)` 唯一约束，唯一性完全由应用层这段代码兜住。

副作用：这条校验把同一应用内部的多端口路径映射也一起堵死了。前端表单列语义是
“路径 → 端口”（`web/src/i18n/locales/zh-CN/gatewayRoutesPage.ts` `pathToPort`），本意像要做
同域多路径，但 host 唯一校验让它永远只有一条能落地。

### 3.2 记录结构：一条路由 = 一个后端

- 模型里没有“域名聚合实体”，`GatewayRoute` 同时承担域名、路径、后端和 TLS（`internal/model/gateway.go`）。
- 下发时一条路由生成一个 HTTPRoute，单 rule、单 match、单 backendRef（`httpRouteObject`）；
  后端来自该路由绑定部署配置的 Service 名（`gateway_runner.go` `gatewayServiceName`），端口还
  必须属于该部署配置暴露的端口（`deploymentTargetHasServicePort`）。
- 因此 `/api → 另一个应用` 在当前数据模型里没有表达位置，只能新建 host 不同的路由。

### 3.3 证书模型：Secret 按 host，Certificate 按 route

- Secret 名 `dnsLabel("tls-" + route.Host)`（`kube_specs.go`）→ 同 host 必然同 Secret 名。
- Certificate 对象名 `resourcename.GatewayRoute(route.ID)`（`kube_specs.go` `gatewayRuntimeName`）
  → 每个 route 一个 Certificate。
- 放开 host 唯一后立刻出现：两个 Certificate 抢同一个 Secret（cert-manager 要求 Certificate↔Secret
  1:1），HTTP-01 重复下单；删除其中一条路由时 `removeGatewayCertificateRef` 按 Secret 名从 listener
  摘引用，会把另一条仍在用的路由证书引用一起摘掉。
- 证书归属键是 host，生命周期却挂在 route 上——两把钥匙开一把锁。

### 3.4 访问地址与“主入口”是应用级语义

`internal/api/applicationapi/application_runtime_summary.go` 的 `primaryGatewayRoutes` 每个应用只挑
一条主路由（`IsDefault` 优先，再按 stage/创建时间），应用列表、概览、拓扑都用这条生成 `accessUrl`。
产品心智即“一个应用一个主域名”。

### 3.5 前端/交互按 host 建模

- 创建表单只能在当前应用自己的部署配置里选后端（`application-gateway-form-fields.tsx`、
  `application-gateway-panel.tsx`），没有“同域名下挂另一个应用”的选项。
- 默认域名自动生成按 host 避让，冲突加 `-2`、`-3`（`gateway_route_domain.go` `defaultGatewayHost`）。
- `checkGatewayDomain` 只返回 `available/current/conflict`，没有路径维度的冲突概念。

### 3.6 观测与计费口径

`internal/gatewayprobe` 按 route ID 打 label 采集，同 host 多路由不会算错；但项目概览的
“入口就绪数”按路由计，放开后同一域名会在统计里出现多次，需先定口径。

## 4. 支持该场景需要补齐的缺口

1. 唯一性判据从 `host` 改为 `(host, path, pathMatchType)`，补前缀重叠规则（`/` + `/api` 允许，
   `/api` + `/api/v2` 的 Prefix/Exact 混排需明确），并加对应迁移索引。
2. 移除 host-only 冲突口径，`checkGatewayDomain` 改为返回“该域名已有路径 + 建议可用路径”。
3. 证书归属从 route 改为 host：Certificate 名按 host 派生（或引入一条域名证书记录），路由只引用；
   删除时按 host 引用计数，而不是按 Secret 名硬摘 listener 引用。
4. 访问入口编辑视图引入“域名 → 多条路径”结构，允许一条路由指向同项目内其他应用的部署配置。
5. 明确计费与概览口径：入口数是否按 host 去重。

## 5. 短期绕行

- 两个域名 + `RequestRedirect`/`URLRewrite` 拼出效果。
- 平台管理员绕过平台直接在 Gateway 上手工挂 Traefik HTTPRoute/Middleware：手工资源不被
  `gatewayprobe`、证书回收和删除清理链路覆盖，属明确越界操作。

## 6. 相关记录

`TODO.md` 已有一条优化项：“访问入口支持一个域名多个应用走不同的 /path 使用”。
