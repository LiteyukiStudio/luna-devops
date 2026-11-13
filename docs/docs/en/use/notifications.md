# Notifications

Luna DevOps separates notifications into three clear scopes:

- **Personal notifications** (**Account settings → Notifications**): Each user manages failure and recovery alerts sent to their own account email and personal Webhook bots.
- **Project-space notifications** (**Project space → Notifications** and sidebar **Integrations → Notification channels**): Project space **Owners** self-manage Webhook bots, their own SMTP sender accounts, alert rules, and cross-space channel bindings.
- **Platform global notifications** (**System management → Notifications**): Platform administrators manage global channels, custom templates, and platform-wide routing rules.

## Configure the platform mail service

A platform administrator configures the global SMTP service under **Site settings → Mail service**. It stores only the sending connection and sender identity for registration codes and personal notification emails; project-space rules cannot send arbitrary mail through the platform SMTP account.

Enter the host, port, security mode, username, sender address, and sender name. Passwords are encrypted in Secret Store and never echoed back; leave the password empty when editing to keep the existing value.

**Personal email aggregation cooldown** merges rapid personal failure emails for the same user: the first email sends immediately, and subsequent events within the cooldown window (`0`–`3600` seconds, default `60`, up to 20 events per digest) are combined. Each user receives an independent delivery to their own account email without exposing other users in To or CC.

## Configure my notifications

Open **Account settings → Notifications** after signing in:

1. Enable or disable email delivery (enabled by default).
2. Select the failure and recovery events to subscribe to.
3. Optionally add personal Feishu (interactive cards), Lark, WeCom, DingTalk, Slack, or Discord channels; provide an optional HMAC-SHA256 **Signing secret** when bot signature verification is enabled.
4. Test the channel after saving, and inspect duration, response snippets, or manual retries in your personal delivery history.

Supported personal subscription events:

- Runtime: `runtime.pod_oom_killed`, `runtime.pod_crash_loop`, `runtime.pod_start_failed`, `runtime.workload_degraded`, `runtime.recovered`
- Build & release: `build.failed`, `build.webhook_rejected`, `release.failed`, `hook.failed`
- Gateway & certificates: `gateway.apply_failed`, `certificate.failed`, `certificate.expired`

Personal deliveries target only the event's internal actor or the creator of the affected resource (deduplicated if identical) after verifying active project-space access. Each user may keep up to 10 personal channels; channel tests are rate-limited to 10 attempts per user per minute.

## Configure project-space notifications & cross-space reuse

**Prerequisite**: You must be an **Owner** of the target project space. Project **Admins** and **Developers** can view rules, bound channels, and delivery status without mutating credentials or viewing private recipient overrides.

### 1. Create or reuse a notification channel

Open **Project space → Notifications → Channels** or **Integrations → Notification channels**:

1. Click **Create channel** and choose a preset bot (Feishu interactive card, Lark, WeCom, DingTalk, Slack, Discord, etc.), a custom Webhook, or a **Custom SMTP email account**.
2. Select one or more project spaces where you hold the **Owner** role.
3. For a custom SMTP channel, enter your own SMTP host, port, encryption (`starttls` / `tls` / `none`), credentials, and sender address. **Default recipients are optional** — leave them empty to reuse one SMTP sender account across multiple spaces and specify recipients per rule.
4. Click **Test connection** in the dialog footer to verify connectivity before saving.

When a channel is bound to multiple project spaces, editing or deleting its credentials requires **Owner** permission on **all** bound spaces. An Owner of a single space can click **Unbind from space** to detach it locally (after removing it from that space's rules). Project and personal channels enforce public-egress SSRF protection, blocking cloud metadata, cluster-internal, and private network addresses.

### 2. Create a project-space notification rule

Under **Project space → Notifications → Rules**, click **Create rule**:

1. Enter a rule name and optionally filter specific applications or deployment targets.
2. Select the runtime, build, release, gateway/certificate, or hook events to match.
3. Select target channels (bound project channels or platform global channels). When an SMTP channel is selected, you can specify **Per-rule To / CC recipient overrides** (falling back to the channel's default recipients if left empty; at least one recipient must exist on the channel or rule).
4. Configure alert throttling, recovery notifications, and message time zone:
   - **Reminder interval while unresolved**: `0`–`10080` minutes, default `60` minutes (initial alert sends immediately upon confirmation).
   - **Max repeat reminders**: `0`–`100`, default `3`; set to `0` to alert once without repeat reminders.
   - **Notify when incident recovers**: Enabled by default; sends a recovery alert and resets reminder counters.
   - **Message time zone**: IANA time zone name, default `Asia/Shanghai`.

## Configure platform global notifications

**System management → Notifications** is restricted to platform administrators for managing global channels, custom Go `text/template` templates, and cross-project rules. Rules default to explicit project-space selection and broadcast platform-wide only when **All project spaces** is explicitly chosen.

Set the same browser-accessible `PUBLIC_BASE_URL` on both API and Worker to include direct console action buttons and links in emails and interactive cards.

## Inspect and retry deliveries

Open **Deliveries** in a project space, account settings, or system management to filter by status, event type, channel, or source event ID, and review queued/finished timestamps, duration, and attempts (up to `6` exponential-backoff attempts). Click any row to open the diagnostics drawer with the redacted request snapshot, remote response snippet (including Feishu/DingTalk application error codes), link to the source event, and **Redeliver** action.
