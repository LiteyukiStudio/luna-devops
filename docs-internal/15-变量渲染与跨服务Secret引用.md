# 变量渲染与跨服务 Secret 引用 — 实施任务

## Phase 1: 同部署变量渲染 `${VAR_NAME}` ✅

### 1.1 实现 `expandEnvRefs` 函数 ✅
- `internal/variables/renderer.go` · `ExpandEnvRefs(map[string]string) map[string]string`
- 正则 `${[A-Za-z_][A-Za-z0-9_]*}`，多轮迭代至不动点
- 自引用和循环引用保留原样；缺失 key 保留原样

### 1.2 集成到部署渲染链路 ✅
- `internal/worker/kube_specs.go` · `expandEnvRefsCrossBoundary(config, secret)`
- 合并 config+secret 作为查找源，展开 config 中的 `${...}` 引用
- `deploy_runner.go` · `applyServiceBindingConfig` 后再次展开，捕获 Service Binding 注入的值

### 1.3 测试 ✅
- 6 个测试：基本展开、链式引用、自引用、缺失 key、空值、无引用

---

## Phase 2: 跨服务 Secret 引用（Service Binding 扩展） ✅

### 2.1 数据模型 ✅
- `internal/model/service_binding.go` · `SecretMapping` 结构体 (`sourceEnvVar`, `targetSecretKey`)
- `ServiceBinding.SecretMap` 字段 (JSON 列，API 暴露为 `credentialMap`)
- Migration `000064`：`service_bindings.secret_map`

### 2.2 API 层 ✅
- `ServiceBindingInput.SecretMap` (`credentialMap` JSON)
- 校验：每个条目 `sourceEnvVar` 和 `targetSecretKey` 不能为空

### 2.3 Worker 渲染集成 ✅
- `resolveServiceBindingConfig` → 解析 `credentialMap`，通过 `r.secrets.Resolve` 解析目标 SecretRefs
- 结果写入 `resolvedServiceBindingConfig.SecretValues`
- `applyServiceBindingConfig` → 合并 SecretValues 到 `spec.SecretData`

### 2.4 OpenAPI 更新（待定）
- ServiceBinding 的 `credentialMap` 字段需要同步到 OpenAPI spec

### 2.5 Agent 工具更新（按需）
- 如果 AI 需要声明 Secret 引用，Agent 工具定义需加 `credentialMap`

---

## Phase 3: 文档与验收

### 3.1 更新 skills
- `service-dependency-planning.md`：说明 AI 可用 key 名声明引用
- `delivery-orchestration.md`：变量渲染语法

### 3.2 端到端验收
- 完整链路：PostgreSQL 模板部署 → Service Binding 声明 Secret 引用 → Web App 部署 → 容器内验证连接
