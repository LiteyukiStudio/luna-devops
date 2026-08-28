# Notifications

Luna DevOps separates team-wide notification routing from personal notifications. Platform administrators maintain shared routes, while each user controls only their own preferences and personal Webhook channels. Notifications do not use the project-resource **Related to me / All** visibility switch; personal and shared entries always follow their separate permission paths. Subscribe only to failures that require action to avoid unnecessary messages.

## Configure the platform mail service

A platform administrator first configures one global SMTP service under **Site settings → Mail service**. The service stores the sending connection and sender identity, not fixed recipients. Registration codes and personal email notifications reuse this service.

Enter the host, port, security mode, account, sender address, and sender name. When authentication is used, also enter the password. The password is stored only in Secret Store and is never shown again; leave it empty during a later edit to keep the current value.

**Personal email aggregation cooldown** applies only to personal failure-event email. The first message is sent immediately; later events for the same user during the cooldown are combined and sent when it ends. Each digest contains at most 20 events; additional events remain queued and continue in later cooldown cycles. The default is 60 seconds, and the accepted range is `0`–`3600`; `0` means no waiting. Registration codes, account recovery, administrator test messages, and shared SMTP channels are unaffected.

After saving, enter an authorized test recipient and send a test email. If it fails, verify that the port matches the security mode, the account permits SMTP login, and the API and worker networks can reach the mail server.

Personal notifications never combine multiple users in one message's To, CC, or BCC fields. The platform resolves one account email from an internal user ID and creates an independent delivery for that user, so other users' addresses are not exposed.

## Configure my notifications

Open **Account settings → Notifications** after signing in:

1. Choose whether to receive email. It is enabled by default for a first-time user.
2. Select the failure events you want to receive.
3. Optionally add personal Feishu, Lark, WeCom, DingTalk, Slack, or Discord channels from the platform presets.
4. Send a test after saving a channel, then check your personal delivery history.

Personal email always goes to the account email. A user cannot enter arbitrary To, CC, or BCC recipients and does not provide personal SMTP credentials. Personal event messages use an HTML digest with a plain-text fallback; each event shows its type, project-space, application, deployment-target context, and relevant type-specific details. Tokens and keys for personal Webhooks are stored only in Secret Store and are not shown again. Targets remain subject to the platform outbound security policy and cannot access protected internal addresses.

Personal subscriptions currently support these failure events:

- `build.failed`
- `release.failed`
- `hook.failed`
- `gateway.apply_failed`
- `certificate.failed`
- `certificate.expired`

Personal deliveries go only to the event's internal actor and the creator of the affected resource; when those IDs are the same, only one delivery is created. At creation time and again before sending, Luna DevOps verifies that the recipient still has access to the project space, the account remains active, and the event remains subscribed. Project membership or platform-administrator status alone never creates a personal delivery. Email addresses asserted by external payloads such as Git Webhooks are never trusted for recipient resolution.

Personal channels accept only built-in presets and their explicitly declared Secret fields. Arbitrary URLs, headers, and JSON configuration are not accepted, which keeps Webhook tokens out of business data.

Each user can keep at most 10 personal channels. A personal notification configuration request cannot exceed 64 KiB, a channel name cannot exceed 160 characters, and each preset-declared Secret cannot exceed 4,096 characters; undeclared fields are rejected. Personal channel tests are limited to 10 attempts per user per minute.

## Configure shared team notifications

The **Notification integrations** page is restricted to platform administrators and manages shared destinations and event routes:

- **Shared channels** are team chat groups, fixed Webhooks, or fixed email groups.
- **Routing rules** select which platform events are sent to which shared channels. Every rule must explicitly select one or more project spaces; it can match platform-wide events only when an administrator actively selects **All projects**. An empty scope never means all.
- **Message templates** are an advanced option for custom content.
- **Delivery records** show shared delivery status, attempts, and error summaries.

Shared SMTP channels remain appropriate for a fixed email group and can contain To, CC, and BCC recipients. They are different from the platform mail service: the platform service sends personal messages to account emails, while an administrator rule sends through a shared SMTP channel to its preconfigured recipients.

Prefer a built-in integration preset when creating a Webhook channel and enter only the token, key, or address required by the destination. A custom Webhook needs a URL, method, headers, and JSON message template. Send a test after saving, then create the route.

To include links back to Luna DevOps, configure the same `PUBLIC_BASE_URL` for the API and worker. Each event in a personal HTML message then includes both a **View details** button and a copyable absolute URL. Notifications still send without this setting, but do not include platform links.

## Verify notifications

### Personal notifications

1. Confirm that the email toggle and event selection under **Account settings → Notifications** are saved.
2. Send a test through each personal Webhook channel.
3. In a test environment, trigger a subscribed failure as your own account.
4. Confirm the final email and Webhook states in your personal delivery history.

### Shared notifications

1. Create a shared Webhook or SMTP channel and send a test.
2. Create a rule and explicitly select its project-space scope, events, and shared channels. Select **All projects** only for an intentional platform-wide broadcast.
3. Trigger a matching event in a test environment.
4. Confirm its state and error details in the administrator delivery records.

Do not introduce a production failure merely to test notifications.

## FAQ

### I did not receive a personal email

Confirm that the global mail test succeeds, the account email is correct, personal email is enabled, the event is selected, and the current user is either the event's internal actor or the creator of the affected resource with current access to the project space. If the event occurred during the aggregation cooldown after the previous personal email, wait until the cooldown ends before checking the next message.

### A Webhook test succeeds but a real event does not send

For a personal channel, confirm that the event is selected in your preferences and the channel is enabled. For a shared channel, check that its rule is enabled, the event and filters match, and a delivery record was created.

### Deliveries keep failing

Check credentials, message templates, destination URLs, and destination availability. Network errors, rate limits, and server failures may retry automatically; configuration and template errors usually require a correction and another test.

### Notifications have no detail link

Confirm that the API and worker use the same browser-accessible `PUBLIC_BASE_URL`.
