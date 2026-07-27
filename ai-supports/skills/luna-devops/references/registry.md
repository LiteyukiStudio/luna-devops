# 镜像站

## 机器目录

先执行
`luna help catalog category=registry limit=100 output=json interactive=false agent=true`
读取镜像站领域命令，再用
`luna help command path=<category.tool> output=json interactive=false agent=true`
确认具体输入、Scope、风险和服务端支持状态。

## 工作流

1. 读取镜像站、作用域、健康状态、脱敏凭据元数据和项目默认值。
2. 写入凭据前确认个人、项目空间或全局作用域以及用途。
3. 使用 Help 指定的安全文件或标准输入提交 Secret。
4. 按目录能力读取远端或平台镜像记录、连接状态和候选镜像。
5. 变更后执行已登记的连接测试；删除前检查构建、部署和项目默认值引用。

## 风险与验证

- 不返回 password、token 或 robot secret。
- 连接测试成功不代表某个仓库或 tag 一定存在。
- 区分平台镜像记录与镜像站实时数据，不把缓存结果描述为远端事实。
- 凭据、镜像和镜像站的删除或覆盖按后端权限、MFA 与审计约束执行。
- 执行后重新读取资源、作用域和连接状态。
