---
name: luna-devops-registry
description: Luna DevOps 镜像站和镜像管理。用于 Harbor、DockerHub、Gitea Registry、通用 OCI registry、registry credentials、镜像仓库搜索、tag 查询和 image template。
---

# 镜像站 Skill

## 适用能力

- artifact registries 列表、创建、更新、删除、连通性测试。
- registry credentials 创建、更新、删除。
- image template default 查询。
- registry repository search 和 tag 查询。
- container image records 创建和列表。

## 操作流程

1. 确认 registry 类型、地址和项目空间作用域。
2. 添加凭据时只写入，不要求用户回显。
3. 保存后执行 registry test。
4. 构建前确认 push credential 可用。
5. 发布前确认 image repository 和 tag/digest。

## 安全边界

- credential mutation 是 high risk，必须 step-up 或 confirmation。
- 不返回 password、token、robot account secret。
- 删除 registry 前检查 build job、release 和 default registry 引用。

