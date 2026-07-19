---
name: luna-devops-runtime
description: Luna DevOps 运行集群和 Kubernetes 资源操作。用于 runtime clusters、cluster resources、resource YAML、resource events、Pod 状态、集群连通性和 runtime 诊断。
---

# 运行集群 Skill

## 适用能力

- runtime clusters 列表、创建、更新、删除、测试。
- cluster resources 查询和删除。
- resource YAML 查看。
- resource events 查看。
- Pod terminal 授权和 stream 边界说明。

## 操作流程

1. 先确认 clusterId 和 project/application/deployment target 关联。
2. 集群问题先 test cluster，再查 resource events。
3. Pod 异常先看 workload、Pod status、events，再看 release/runtime logs。
4. YAML 用于诊断，不默认让用户复制执行。

## 风险边界

- kubeconfig update 是 high risk，需要 step-up。
- 删除 cluster resource 是 high risk，默认只在用户明确指定资源后执行。
- terminal 和 runtime exec 是 critical，第一版不开放给外部 MCP；内部也必须 browser session + MFA + confirmation。
- 不返回 kubeconfig 内容。

