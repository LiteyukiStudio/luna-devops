# Notifications

Notifications send important platform events to collaboration tools or email. Failure events are enabled first so administrators are not interrupted by every successful build, release, and route sync.

## How it works

A notification goes through four steps from event creation to delivery:

1. The platform emits a structured event when a build, release, hook, or access route fails.
2. Notification rules filter events by event type, severity, project space, application, and deployment target.
3. A matching rule creates delivery records, then worker sends them asynchronously.
4. A notification adapter renders the event into the request or email required by the target platform.

Build, release, and other business modules only describe what happened. They do not need to know the protocol details of Feishu, WeCom, SMTP, or custom Webhooks.

## Channels

Supported channels:

- Webhook: custom `method`, `url`, `headers`, and JSON body templates for Feishu, WeCom, Slack-like bots, or internal alert endpoints.
- SMTP: standard SMTP, STARTTLS, or TLS email delivery.

Webhook channels restrict methods to `POST`, `PUT`, or `PATCH`, and validate target URLs with the platform public egress policy to avoid sending notifications to sensitive internal addresses. Channel secrets are stored in Secret Store. Business tables only keep secret references, and API responses only expose `secretSet`.

## Preset snapshots

The platform follows Nightingale's split between notification media, templates, and delivery records, and provides common collaboration tools as Webhook presets. Creating a channel from a preset turns the preset into an ordinary Webhook channel and default template snapshot:

- You only provide the token or key required by the preset.
- Existing channels do not automatically follow future preset changes.
- To change the message format, edit the generated template or create another template.

Built-in presets:

| Preset | Required secrets | Message shape | Notes |
| --- | --- | --- | --- |
| Feishu Bot | `WebhookToken` | Rich `post` | Uses the Feishu custom bot endpoint on `open.feishu.cn`; the template includes event summary, project space, application, deployment target, build/release/hook/gateway details, and a clickable detail link. |
| Lark Bot | `WebhookToken` | Rich `post` | Uses the Lark custom bot endpoint on `open.larksuite.com` with English field labels and a platform detail link. |
| WeCom Bot | `WebhookKey` | Markdown | Uses the WeCom group robot `markdown` message type with resource context and a platform detail link. |
| Gotify | `GotifyHost`, `AppToken` | Markdown message | `GotifyHost` is a host without scheme, such as `gotify.example.com`; the platform uses HTTPS and `X-Gotify-Key`, and message extras render Markdown details. |
| DingTalk Bot | `AccessToken` | Markdown | Uses the DingTalk custom robot `access_token` endpoint; signed robots need a later dedicated capability. |
| Slack Incoming Webhook | `WebhookPath` | `mrkdwn` blocks | Enter only the path after `hooks.slack.com/services/`, so the full Webhook URL is not stored as plain business data; the block includes the detail link. |
| Discord Webhook | `WebhookID`, `WebhookToken` | embeds | Uses the Discord Webhook execute API embed payload, with full event details in the description. |

Webhook channels support `testJsonBodyTemplate`. Presets write a platform-specific test body into the channel config, so after a second confirmation the test action renders a test event with preset template variables and sends a valid payload for that platform instead of one generic JSON shape. When a rule does not explicitly select a template, preset channels also reuse this platform-compatible body as the default delivery template, so Feishu, Lark, WeCom, and similar bots do not receive an incompatible generic Webhook JSON payload.

References:

- Feishu custom bot: [Feishu Open Platform](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)
- Lark custom bot: [Lark Open Platform](https://open.larksuite.com/document/client-docs/bot-v3/add-custom-bot)
- WeCom group robot: [WeCom Developer Docs](https://developer.work.weixin.qq.com/document/path/91770)
- Gotify message API and Markdown extras: [Gotify API](https://gotify.net/api-docs), [Message Extras](https://gotify.net/docs/msgextras)
- DingTalk custom robot: [DingTalk Open Platform](https://open.dingtalk.com/document/robots/custom-robot-access)
- Slack Incoming Webhooks: [Slack API docs](https://api.slack.com/messaging/webhooks)
- Discord Webhook Execute: [Discord Developer Docs](https://discord.com/developers/docs/resources/webhook)

## Template variables

Templates use Go template syntax with missing-field errors enabled, so typoed variables fail instead of silently sending empty content. Common variables:

| Variable | Description |
| --- | --- |
| `.Event.Type` | Event type, such as `build.failed`. |
| `.Event.Severity` | Severity, such as `error`. |
| `.Event.Message` | Failure summary. |
| `.Event.Project.Name` / `.Event.Project.Identifier` | Project space name and immutable identifier. |
| `.Event.Application.Name` / `.Event.Application.Identifier` | Application name and immutable identifier. |
| `.Event.DeploymentTarget.Name` / `.Event.DeploymentTarget.Identifier` | Deployment target name and stage identifier. |
| `.Event.Build.ID` / `.Event.Build.Image` / `.Event.Build.GitRef` | Build context. |
| `.Event.Release.ID` / `.Event.Release.ImageRef` / `.Event.Release.Revision` | Release context. |
| `.Event.Hook.Name` / `.Event.Hook.Phase` | Hook context. |
| `.Event.Gateway.Domain` / `.Event.Gateway.Path` | Access route context. |
| `.Event.Actor.Name` / `.Event.Actor.Email` | Actor context, when the event source provides it. |
| `.Event.Links` | Platform detail link map. Built-in failure events generate `primary`, `project`, `application`, and event-specific `build`/`release`/`hook`/`gateway` links when `PUBLIC_BASE_URL` is configured, jumping to the build, deployment, or gateway tab according to the event type. Use `{{ link .Event.Links "primary" }}` to avoid missing-key render errors. |
| `.Secrets.<Name>` | Channel secret value injected only while rendering. It is not echoed by APIs. |

When upgrading to immutable identifiers, saved notification templates and webhook channel configurations are migrated from `.Event.Project.Slug`, `.Event.Application.Slug`, and `.Event.DeploymentTarget.Slug` to the corresponding `.Identifier` fields. Historical event snapshots are converted as well.

Available functions:

- `json`: encode a value as a JSON string.
- `time`: format a timestamp.
- `default`: use a fallback for empty values.
- `detailsTitle`: render a consistent event title, such as `[error] release.failed`.
- `details`: render multiline event details with resource context, event-specific fields, and a detail link. Pass `zh` or `en` as the second argument.
- `link`: safely read a key from `.Event.Links` without failing on a missing map key.
- `truncate`: shorten long text.

To include direct platform links, configure the same `PUBLIC_BASE_URL` for both API and Worker. Events are still delivered without it, but no platform detail link is added.

After updating a Docker Compose deployment, recreate the Worker container so it receives `PUBLIC_BASE_URL`:

```bash
docker compose up -d --force-recreate worker
docker compose exec worker printenv PUBLIC_BASE_URL
```

When one business event matches both a global rule and a project rule, each notification channel still receives only one delivery. Create separate channels when the same event must be sent to multiple destinations.

## Rules

Rules must select at least one channel. Supported failure events:

- `build.failed`
- `release.failed`
- `hook.failed`
- `gateway.apply_failed`

Delivery failures are recorded in the delivery list. An HTTP 2xx Webhook response is treated as a successful send; the platform then marks the delivery as `succeeded` and updates the channel's latest successful delivery time. Template rendering errors, invalid channel configuration, and Webhook platform 4xx responses except 429 are marked as failed without retrying. Network errors, 429, and 5xx responses still follow the queue retry policy. Notification tasks allow five retries, so a continuously failing delivery can be attempted up to six times including the initial send.

Filter JSON example:

```json
{
  "severities": ["error"],
  "projectIds": ["prj_xxx"],
  "applicationIds": [],
  "deploymentTargetIds": []
}
```

An empty array means that dimension is not filtered. The current UI manages global administrator rules first; project-space rules can be added later.

## SMTP example

Channel config JSON:

```json
{
  "host": "smtp.example.com",
  "port": 587,
  "security": "starttls",
  "username": "notice@example.com",
  "from": "Luna DevOps <notice@example.com>",
  "to": ["ops@example.com"],
  "timeoutSeconds": 15
}
```

Secret key-values:

```text
password=email or SMTP password
```

The password is stored in Secret Store. You do not need to fill it again when editing; enter `password=...` only when replacing it.

## Verification

Suggested verification flow:

1. Create a Webhook or SMTP channel.
2. Click the test action, review the template variable note in the confirmation dialog, then send the test message.
3. Create or confirm a notification template.
4. Create a rule, select failure events and a channel.
5. Trigger a build failure or hook failure intentionally.
6. Check delivery records for status, attempts, errors, and the redacted request snapshot.
