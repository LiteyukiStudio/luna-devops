# 变量渲染与跨服务 Secret 引用 — 实施任务

## Phase 1: 同部署变量渲染 `${VAR_NAME}`

### 1.1 实现 `expandEnvRefs` 函数
- 在 `internal/variables/` 或 `internal/worker/` 中实现
- 接受 `map[string]string`，对每个 value 调用 `os.Expand`，引用同 map 内的 key
- 循环引用/缺失 key 时原样保留

### 1.2 集成到部署渲染链路
- `internal/worker/kube_specs.go` → `mergeKeyValueMaps` 调用后对 `configData` 执行展开
- SecretData 也需要（Secret 值也可能引用 Config 中的拼接）

### 1.3 测试
- 单元测试：基本展开、循环引用、缺失 key、空值处理

### 1.4 更新 docs-internal
- 记录变量渲染语法 `${VAR_NAME}` 到对应的规格文档

---

## Phase 2: 跨服务 Secret 引用（Service Binding 扩展）

### 2.1 数据模型
- `internal/model/` 新增 `SecretMapping` 结构体
- `ServiceBinding` 模型加 `SecretMap` 字段（JSON 存储）
- 数据库 migration：`service_bindings` 表加 `secret_map` 列

### 2.2 API 层
- `internal/api/service_binding_handlers.go`：创建/更新时接受 `secretMap`
- `internal/dependency/service.go`：`ServiceBindingInput` 加 `SecretMap`，校验
- `internal/dependency/repository.go`：读写 `secret_map`

### 2.3 Worker 渲染集成
- `internal/worker/service_bindings.go`：`resolveServiceBindings` 扩展
- 对每个 binding 的 `secretMap`，从目标 K8s Secret 读取值，注入源部署的 `SecretData`
- Worker 需要增加读取目标 K8s Secret 的能力

### 2.4 OpenAPI 更新
- 同步 ServiceBinding 的请求/响应 schema

### 2.5 Agent 工具更新（如果需要）
- Agent 侧 `platform.ts` 工具定义加 `secretMap` 参数

### 2.6 测试
- Service binding 创建带 secretMap
- Worker 渲染验证 SecretData 多出了目标秘密值
- 安全边界：API 返回不含 Secret 值

---

## Phase 3: 文档与验收

### 3.1 更新 skills
- `service-dependency-planning.md`：说明 AI 可用 key 名声明引用
- `delivery-orchestration.md`：变量渲染语法

### 3.2 端到端验收
- 完整链路：PostgreSQL 模板部署 → Service Binding 声明 Secret 引用 → Web App 部署 → 容器内验证连接
