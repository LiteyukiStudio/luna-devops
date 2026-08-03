# Notifications

Notifications send build, release, hook, and route events to collaboration tools or email. Start with failure events to avoid excessive messages for successful operations.

## Create a channel

Supported channel types are:

- **Webhook** for Feishu, Lark, WeCom, DingTalk, Slack, Discord, Gotify, and custom alert endpoints.
- **SMTP** for email through standard SMTP, STARTTLS, or TLS.

Prefer a built-in preset and enter only the token, key, or address it requests. For a custom webhook, configure the URL, method, headers, and JSON message template.

Channel secrets are stored securely and are not shown again. Webhook targets are checked by the platform's outbound security policy and cannot be used to access protected internal addresses.

After saving a channel, send a test message before creating a rule.

## Create a rule

A rule selects at least one channel and can filter by event type, severity, project, application, and deployment target. The main failure events are:

- `build.failed`
- `release.failed`
- `hook.failed`
- `gateway.apply_failed`

One event can be sent to multiple channels. Delivery records show status, attempt count, and an error summary for troubleshooting templates, credentials, network access, and destination services.

## Message templates

Built-in presets already provide a suitable message format. Edit templates only when custom content is required.

Templates can use event type, severity, resource names, actor, and detail links. Missing or invalid variables cause delivery to fail, so send another test after editing. Secrets are injected only during delivery and do not appear in previews or API responses.

To include links back to Luna DevOps, configure the same `PUBLIC_BASE_URL` for the API and worker. Notifications still send without it, but do not include platform links.

## SMTP

Enter the host, port, security mode, account, sender, recipients, and password. Leaving the password empty during later edits keeps the existing value.

If the test fails, verify that the port matches the security mode, the account permits SMTP login, and the network can reach the mail server.

## Verify notifications

1. Create a webhook or SMTP channel and send a test.
2. Create a rule for a failure event and select the channel.
3. Trigger a failure in a test environment.
4. Confirm the result and error details in delivery records.

Do not introduce a production failure merely to test notifications.

## FAQ

### The webhook test succeeds but real events do not send

Check that the rule is enabled, its event and filters match, and a delivery record was created.

### Deliveries keep failing

Check credentials, template variables, destination URL, and destination availability. Network errors, rate limits, and server failures may retry automatically; configuration and template errors usually require a correction.

### Notifications have no detail link

Confirm that the API and worker use the same browser-accessible `PUBLIC_BASE_URL`.
