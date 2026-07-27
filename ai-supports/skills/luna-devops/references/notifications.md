# 通知

## 机器目录

先执行
`luna help catalog category=notification limit=100 output=json interactive=false agent=true`
发现当前通知能力。调用具体工具前使用
`luna help command path=<category.tool> output=json interactive=false agent=true`
读取输入 Schema、风险和服务端支持状态；分类名称与具体工具始终以机器目录为准。

## 工作流

1. 明确项目范围、事件类型、接收目标和语言。
2. 按目录能力读取或维护渠道、渠道预设、模板、规则和投递记录。
3. 创建渠道时通过安全输入提交 Webhook、SMTP 或适配器 Secret。
4. 测试通知前确认真实接收目标、模板变量与外部影响，只发送一次。
5. 排障时检查事件、规则匹配、渲染结果、尝试次数、外部响应和最终适配器判定。

## 风险与验证

- 不用 `api request` 创建渠道、规则或发送测试通知。
- Webhook URL、SMTP 密码、签名 Secret 和 Token 不回显。
- 测试和重放会触达外部系统，必须明确确认目标并防止重复发送。
- 外部 HTTP 2xx 不一定代表业务平台接收成功，以后端适配器记录的终态为准。
- 修改模板或预设不应被假定会改变已有渠道快照，需读取目标对象确认。
