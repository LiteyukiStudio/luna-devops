# 通知

通知用于把平台内部事件发送到外部协作工具或邮箱。当前版本先覆盖失败类事件，避免把构建、发布和网关同步中的噪音全部推给管理员。

## 工作方式

通知链路分为四层：

1. 平台在构建、发布、Hook 或访问入口失败时产生结构化事件。
2. 通知规则按事件类型、严重级别、项目空间、应用和部署配置过滤事件。
3. 规则命中后生成投递记录，并交给 worker 异步发送。
4. 通知适配器把事件渲染为目标平台需要的请求或邮件。

业务模块只负责产生事件，不直接关心飞书、企业微信、SMTP 或自定义 Webhook 的细节。

## 渠道

通知渠道目前支持：

- Webhook：自定义 `method`、`url`、`headers` 和 JSON Body 模板，适合飞书、企业微信、Slack 类机器人，也可以对接自建告警入口。
- SMTP：通过标准 SMTP、STARTTLS 或 TLS 发送邮件。

Webhook 渠道会限制请求方法为 `POST`、`PUT` 或 `PATCH`，并按平台公共出站策略校验目标 URL，避免把通知发到内网敏感地址。渠道密钥写入 Secret Store，业务表只保存 secret 引用，API 响应只返回 `secretSet`。

## 预设快照

平台参考夜莺监控的通知媒介拆分方式，将常见协作平台做成 Webhook 预设。通过预设创建渠道时，平台会把预设转成普通 Webhook 渠道和默认模板快照：

- 你只需要填写预设要求的 token 或 key。
- 已创建渠道不会跟随未来预设变更自动修改。
- 如果需要调整消息格式，可以编辑生成的模板或创建新的模板。

内置预设：

| 预设 | 必填密钥 | 消息形态 | 说明 |
| --- | --- | --- | --- |
| Feishu Bot | `WebhookToken` | 富文本 `post` | 使用飞书自定义机器人地址 `open.feishu.cn`，模板包含事件摘要、项目空间、应用、部署配置、构建/发布/Hook/访问入口详情和可点击详情链接。 |
| Lark Bot | `WebhookToken` | 富文本 `post` | 使用 Lark 国际站自定义机器人地址 `open.larksuite.com`，模板字段使用英文标签，并附带平台详情链接。 |
| WeCom Bot | `WebhookKey` | Markdown | 使用企业微信群机器人 `markdown` 消息，包含资源上下文和平台详情链接。 |
| Gotify | `GotifyHost`、`AppToken` | Markdown message | `GotifyHost` 填不含协议的域名，例如 `gotify.example.com`；平台默认使用 HTTPS 和 `X-Gotify-Key`，消息 extras 使用 Markdown 展示详情。 |
| DingTalk Bot | `AccessToken` | Markdown | 使用钉钉自定义机器人 `access_token` 地址；需要加签的机器人后续用专用能力支持。 |
| Slack Incoming Webhook | `WebhookPath` | `mrkdwn` blocks | 只填写 `hooks.slack.com/services/` 后面的路径，避免完整 Webhook URL 明文落表；block 中会带上详情链接。 |
| Discord Webhook | `WebhookID`、`WebhookToken` | embeds | 使用 Discord Webhook execute API 的 embed 结构，description 中包含完整事件详情。 |

Webhook 渠道支持 `testJsonBodyTemplate`，预设会为每个平台写入对应的测试消息体。这样点击“测试”并二次确认后，会使用一组预设模板变量渲染平台协议匹配的测试消息，而不是用一个通用 JSON 去尝试所有机器人。规则未显式选择模板时，预设渠道也会复用这份平台匹配的消息体作为默认投递模板，避免飞书、Lark、企业微信等机器人收到不兼容的通用 Webhook JSON。

参考：

- 飞书自定义机器人：[飞书开放平台文档](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)
- Lark 自定义机器人：[Lark Open Platform](https://open.larksuite.com/document/client-docs/bot-v3/add-custom-bot)
- 企业微信群机器人：[企业微信开发者文档](https://developer.work.weixin.qq.com/document/path/91770)
- Gotify 消息接口与 Markdown extras：[Gotify API](https://gotify.net/api-docs)、[Message Extras](https://gotify.net/docs/msgextras)
- 钉钉自定义机器人：[钉钉开放平台文档](https://open.dingtalk.com/document/robots/custom-robot-access)
- Slack Incoming Webhooks：[Slack API 文档](https://api.slack.com/messaging/webhooks)
- Discord Webhook Execute：[Discord Developer Docs](https://discord.com/developers/docs/resources/webhook)

## 模板变量

通知模板使用 Go template，并开启缺失字段报错，避免变量写错后静默发送空内容。常用变量：

| 变量 | 说明 |
| --- | --- |
| `.Event.Type` | 事件类型，例如 `build.failed`。 |
| `.Event.Severity` | 严重级别，例如 `error`。 |
| `.Event.Message` | 失败摘要。 |
| `.Event.Project.Name` / `.Event.Project.Slug` | 项目空间名称和标识。 |
| `.Event.Application.Name` / `.Event.Application.Slug` | 应用名称和标识。 |
| `.Event.DeploymentTarget.Name` / `.Event.DeploymentTarget.Slug` | 部署配置名称和阶段。 |
| `.Event.Build.ID` / `.Event.Build.Image` / `.Event.Build.GitRef` | 构建上下文。 |
| `.Event.Release.ID` / `.Event.Release.ImageRef` / `.Event.Release.Revision` | 发布上下文。 |
| `.Event.Hook.Name` / `.Event.Hook.Phase` | Hook 上下文。 |
| `.Event.Gateway.Domain` / `.Event.Gateway.Path` | 访问入口上下文。 |
| `.Event.Actor.Name` / `.Event.Actor.Email` | 操作人上下文；由事件来源提供，可能为空。 |
| `.Event.Links` | 平台详情链接集合。内置失败事件会在配置 `PUBLIC_BASE_URL` 后生成 `primary`、`project`、`application` 以及 `build`/`release`/`hook`/`gateway` 等链接，并按事件跳到构建、部署或访问入口 tab。读取时建议使用 `{{ link .Event.Links "primary" }}`，避免链接 key 不存在时模板报错。 |
| `.Secrets.<Name>` | 渠道密钥值，仅在渲染时注入，不会回显到 API。 |

可用函数：

- `json`：把值安全编码为 JSON 字符串。
- `time`：格式化时间。
- `default`：为空时使用默认值。
- `detailsTitle`：生成统一事件标题，例如 `[error] release.failed`。
- `details`：生成包含资源上下文、事件专属字段和详情链接的多行文本，第二个参数可传 `zh` 或 `en`。
- `link`：从 `.Event.Links` 中安全读取链接，避免 map key 缺失导致模板渲染失败。
- `truncate`：截断长文本。

如果希望通知能直达平台页面，需要为 API 和 Worker 配置同一个 `PUBLIC_BASE_URL`。未配置时事件仍会正常发送，但不会附带平台详情链接。

## 规则

规则至少需要选择一个通知渠道。当前支持的失败事件：

- `build.failed`
- `release.failed`
- `hook.failed`
- `gateway.apply_failed`

投递失败会记录到投递记录。模板渲染错误、渠道配置错误和 Webhook 平台返回的非 429 的 4xx 错误会直接标记为失败，不再重复重试；网络错误、429 和 5xx 错误仍按队列策略重试。

过滤条件 JSON 示例：

```json
{
  "severities": ["error"],
  "projectIds": ["prj_xxx"],
  "applicationIds": [],
  "deploymentTargetIds": []
}
```

数组为空表示不过滤该维度。当前 UI 先提供全局管理员规则，后续可以扩展项目空间级规则。

## SMTP 示例

渠道配置 JSON 示例：

```json
{
  "host": "smtp.example.com",
  "port": 587,
  "security": "starttls",
  "username": "notice@example.com",
  "from": "Liteyuki DevOps <notice@example.com>",
  "to": ["ops@example.com"],
  "timeoutSeconds": 15
}
```

在密钥键值中填写：

```text
password=邮箱或 SMTP 密码
```

保存后密码会写入 Secret Store，后续编辑时不用再次填写；需要替换密码时重新填写 `password=...`。

## 验收

建议按以下顺序验收：

1. 创建一个 Webhook 或 SMTP 渠道。
2. 点击渠道的测试按钮，确认测试弹窗里的模板变量说明，然后发送测试消息。
3. 创建或确认通知模板。
4. 创建规则，选择失败事件和通知渠道。
5. 人为触发一次构建失败或 Hook 失败。
6. 在投递记录里确认状态、尝试次数、错误信息和脱敏后的请求快照。
