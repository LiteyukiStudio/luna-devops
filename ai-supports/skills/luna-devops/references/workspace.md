# 项目空间

## 机器目录

按任务查询：

- `luna help catalog category=dashboard limit=100 output=json interactive=false agent=true`
- `luna help catalog category=project limit=100 output=json interactive=false agent=true`

执行具体工具前使用
`luna help command path=<category.tool> output=json interactive=false agent=true`
读取参数、Scope、风险和服务端支持状态。

## 工作流

1. 读取活动实例、默认项目和用户可见项目空间。
2. 名称匹配不唯一时列出有限候选，使用稳定项目 ID 继续。
3. 按目录能力读取看板摘要、项目空间、成员、置顶和默认项目。
4. 变更前读取项目与成员状态，明确角色、计费归属和影响。
5. 创建、更新或成员变更后重新读取；删除按关键操作处理。

## 边界

- 默认项目只简化低风险读取，不等于获得项目权限。
- 不将项目名称、可变描述、Kubernetes 名称或展示标签当作项目 ID。
- 平台自有项目空间不可删除。
- 成员权限由后端 RBAC 最终判断，前端可见不代表允许写入。
- 看板只读取目录提供的摘要，不自行拼接或推测未登记统计。
