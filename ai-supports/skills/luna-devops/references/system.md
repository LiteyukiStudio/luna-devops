# 系统管理

## 机器目录

按任务查询 `config`、`retention` 或 `system` 分类，例如：

- `luna help catalog category=config limit=100 output=json interactive=false agent=true`
- `luna help catalog category=retention limit=100 output=json interactive=false agent=true`
- `luna help catalog category=system limit=100 output=json interactive=false agent=true`

机器目录可能只返回当前版本存在的分类和工具。执行前使用
`luna help command path=<category.tool> output=json interactive=false agent=true`
确认权限、风险、Secret 语义和服务端支持状态。

## 全局配置

1. 读取配置定义与当前值。
2. 只更新用户明确指定的键，保留未指定字段。
3. Secret 配置不回显；留空、清除和替换语义以 Help 为准。
4. 执行后重新读取非敏感状态和审计结果。

## 数据保留

1. 读取支持的数据集、策略和有效期。
2. 清理前按同一数据集与时间范围执行预览。
3. 展示时间范围、预计条数、不可逆影响与保留边界。
4. 用户明确确认后执行一次，并重新查询结果。

## 系统组件

按目录能力读取或维护平台组件、应用模板、全局策略和安装状态。安装、更新或移除
前确认目标集群、命名空间、版本与影响，并在异步任务结束后验证实际状态。

## 风险与验证

- 不用 `api request` 补齐系统管理能力。
- 全局配置、数据清理和组件变更按机器 Help 风险处理。
- Agent 或非交互执行需要确认的操作时，必须先取得本次操作的明确授权并显式传入
  `--yes`；收到 `mfa_required` 时停止并遵循根 Skill。
- 不把任务已入队或数据库记录存在当作组件已经健康运行。
