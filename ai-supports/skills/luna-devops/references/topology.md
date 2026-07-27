# 拓扑

## 机器目录

先执行
`luna help catalog category=topology limit=100 output=json interactive=false agent=true`
读取拓扑领域工具，再用
`luna help command path=<category.tool> output=json interactive=false agent=true`
确认查询或 CRUD 的对象、作用域、风险和服务端支持状态。

## 工作流

1. 解析项目空间、应用、部署配置和关系对象的稳定 ID。
2. 按目录能力读取应用 Kubernetes 拓扑、项目服务关系或其他资源图。
3. 区分实时计算节点、持久关系、注入配置和仅用于展示的边。
4. 创建或修改关系前确认来源、目标、关系类型、方向和注入影响。
5. 变更后重新读取关系与受影响应用；诊断资源异常时追加读取
   [运行时](runtime.md) 或 [网关](gateway.md)。

## 边界

- 不根据流量、名称相似度或环境变量自动写入持久依赖关系。
- 不把 Deployment、Pod、Service、ConfigMap、Secret 或访问入口的关联误报为业务依赖。
- ServiceBinding、环境变量注入和跨应用关系可能触发重新部署，按机器 Help 风险处理。
- 不跨项目空间创建关系，除非机器目录和后端权限明确支持。
- CRUD 完成后必须验证关系和受影响资源，不能只依据写请求响应。
