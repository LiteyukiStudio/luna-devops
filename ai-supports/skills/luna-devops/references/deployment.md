# 应用与部署

## 机器目录

分别查询
`luna help catalog category=application limit=100 output=json interactive=false agent=true`
和
`luna help catalog category=deployment limit=100 output=json interactive=false agent=true`。
调用前使用
`luna help command path=<category.tool> output=json interactive=false agent=true`
读取具体契约，不凭名称猜测创建、发布或运行时工具。

## 工作流

1. 解析项目空间、应用、部署配置、发布和目标镜像的稳定 ID。
2. 按目录能力读取或维护应用、部署配置、候选镜像和部署相关设置。
3. 创建发布前确认镜像、环境、目标集群、钩子、成本与并发影响。
4. 对发布、重启、回滚、日志、等待终态、数据导出和终端授权，只调用目录存在的工具。
5. 异步操作返回后按 Help 指示轮询，最终重新读取发布、工作负载或导出状态。

## 风险与验证

- 应用、部署配置、发布和运行资源删除是关键操作。
- Secret 环境变量不回显，也不以内联参数传递。
- 取得终端或导出授权票据不代表会话已连接或文件已下载。
- 终端与数据导出的预授权要求 CLI OAuth 登录和对应 purpose 的 Step-up MFA；
  个人访问令牌不能用于绕过用户在场验证。
- 收到 `mfa_required` 时遵循根 Skill 的用户在场验证流程，不能改用其他 Token 绕过。
- 回滚和重启前确认目标版本与当前运行版本，执行后验证实际工作负载状态。
