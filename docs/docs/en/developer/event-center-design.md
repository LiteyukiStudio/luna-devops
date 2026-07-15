# Event Center Design

The event center answers a simple question: what just happened in the platform?

Builds, releases, hooks, routes, and certificates already have their own status pages. During an incident, however, users often do not know which page to open first. The event center puts those changes on one timeline, so users can understand the sequence first and then follow a link to logs or resource details.

## Where it belongs

Events should be a top-level page in the DevOps navigation, immediately after Project Spaces. It is available to every signed-in user rather than being hidden under system administration.

- Regular users see events from project spaces they can access.
- Platform administrators start with “Related to me” and can switch to all events when needed.
- A project-space overview shows recent events and links to the event center with the project filter applied.
- Build, deployment, and route pages keep their local status views and can link to the event center with an application filter.

The first version should not add another full Events tab inside each project space. A summary and a filtered link avoid maintaining two copies of the same list.

## How it differs from existing records

The platform already stores several kinds of records, but each answers a different question.

| Data | Question it answers | Shown directly in the event center |
| --- | --- | --- |
| `PlatformEvent` | What changed in the product | Yes; this is the primary data source |
| `NotificationDelivery` | Where an event was sent and whether delivery succeeded | Linked from event details when relevant |
| `AuditLog` | Who performed a sensitive action and whether it succeeded | Only a user-safe summary; the original record remains a security log |
| `WorkerTaskEvent` | When an async task queued, started, retried, or finished | Linked as technical detail instead of filling the timeline |
| Kubernetes Event | What a cluster controller reported | Read from the cluster during diagnosis; not copied into the business event table |

The notification service currently stores an event snapshot only after a rule matches and a delivery is created. A business change with no notification rule leaves no complete event history. The new order must be “store the event first, then evaluate notification rules.”

## Core flow

```text
A build, release, hook, route, or certificate changes state
                         |
                         v
                Store PlatformEvent
                   /              \
                  v                v
          Event center query   Match notification rules
                                     |
                                     v
                           NotificationDelivery
```

Business modules emit structured events and never call Feishu, SMTP, or another channel directly. Notifications consume the same stored event and decide whether it should be sent.

## Implemented data model

Migration `000031_platform_events` creates the append-only `platform_events` table with the following fields:

| Field | Purpose |
| --- | --- |
| `id` | Event ID and the ID referenced by notification deliveries |
| `type` | Stable event type, such as `build.failed` |
| `category` | build, release, hook, gateway, certificate, security, and similar groups |
| `severity` | info, warning, or error |
| `project_id` | Project space; empty for platform-level events |
| `application_id` | Application, when applicable |
| `deployment_target_id` | Deployment target, when applicable |
| `resource_type` / `resource_id` | Primary resource type and ID |
| `actor_id` | User who triggered the change; empty for system automation |
| `summary_key` | Stable frontend i18n key |
| `message` | Raw external or runtime summary after redaction |
| `detail_json` | Event-specific structured data |
| `links_json` | Links to project, application, and business detail pages |
| `correlation_id` | Groups one build, release, or automation sequence |
| `trace_id` | Optional tracing ID |
| `occurred_at` | Time the event actually happened |
| `created_at` | Time the platform stored it |

Frequently filtered values need real columns and indexes. `detail_json` is for event-specific values such as an image reference, Git SHA, certificate hostname, or failed stage.

## Event types

Use `<resource>.<action-or-result>` and keep names stable after release.

The first implemented set includes:

| Category | Event types |
| --- | --- |
| Build | `build.started`, `build.succeeded`, `build.failed` |
| Release | `release.started`, `release.succeeded`, `release.failed` |
| Hook | `hook.started`, `hook.succeeded`, `hook.failed` |
| Route | `gateway.applied`, `gateway.apply_failed` |
| Certificate | `certificate.pending`, `certificate.issued`, `certificate.renewed`, `certificate.failed`, `certificate.expired` |

A second pass can add registry connections, Git webhooks, unhealthy replicas, shared configuration changes, and platform operations. Successful and in-progress events belong in the event center, but notifications should default to failures, expiry, and other actionable conditions.

Status reconcilers must compare the previous and next state. A certificate may be checked every minute, but `certificate.issued` must be emitted only when the state changes.

## Event page

Use compact filters at the top:

- Time: today, last 7 days, last 30 days, or a custom range.
- Project space, application, and deployment target support multiple selections. Application options come from the selected project spaces, and deployment-target options come from the selected applications.
- Category, event type, and severity support multiple selections. Selecting categories narrows the event-type options to those categories.
- Result supports multiple selections, such as viewing in-progress and failed events together.

The list is ordered newest first. Each row shows a summary, resource, status, and time. A details panel shows:

- Exact time and actor.
- Project space, application, deployment target, and primary resource.
- Failure summary and structured details.
- Direct links to the build log, Release, hook, route, or certificate.
- Related notification deliveries.
- Earlier and later events with the same `correlation_id`.

Do not add unread counts or “mark all as read” in the first version. This is an activity and diagnostic history, not a message inbox. If actionable acknowledgements become necessary later, model them separately.

## Authorization

The backend performs the final resource-scope check:

- Project members can read events only for project spaces they can access.
- Viewers can read events but cannot reveal secrets or gain privileged actions through event details.
- Platform administrators can read platform-level events and explicitly switch to all project spaces.
- User-scoped events are visible only to that user and platform administrators.

Events must never store or return passwords, tokens, secret values, full kubeconfigs, cookies, Authorization headers, or raw terminal commands. External errors must pass through the existing redaction layer before they enter `message`.

## API

Implemented endpoints:

```text
GET /api/v1/events
GET /api/v1/events/:eventId
GET /api/v1/events/catalog
```

The list endpoint supports pagination, sorting, and these filters:

```text
page, pageSize, sortOrder
projectIds, applicationIds, deploymentTargetIds
categories, types, severities
statuses, dateFrom, dateTo
```

Plural filters can be repeated, for example `projectIds=prj_a&projectIds=prj_b`. The API still accepts singular parameters such as `projectId` and `applicationId` so existing links from project overviews remain valid.

`catalog` is shared by frontend filters and notification rules. It returns event type, category, default severity, whether notification is recommended, and available detail fields. The frontend localizes stable keys rather than hardcoding display names.

## Notification changes

Keep the existing channels, templates, and adapters. Change only the event entry point:

1. A business module asks the event service to store a `PlatformEvent`.
2. After the event is stored, notification matching creates `NotificationDelivery` records for enabled rules.
3. The worker performs external delivery asynchronously, so business event creation does not wait for a remote channel.
4. A delivery keeps the snapshot needed for sending and the delivery result; it no longer doubles as the event history.

Every event type may be selected by a notification rule, but the catalog can mark recommended types. Defaults should include failures, expiry, and security alerts, not every successful operation.

## Retention and cleanup

Events are retained for 90 days by default. An independent daily worker task removes expired events in batches of at most 1,000 rows, so cleanup failures do not block status synchronization. Platform administrators can change the retention period or preview and remove a selected time range. Notification deliveries use their own retention period, while audit logs are excluded from general cleanup.

## What appears after an upgrade

The migration does not reconstruct old build or release history. After the updated API and worker start, new build, release, hook, access-route, and certificate state changes appear in the event center. If the page is empty immediately after an upgrade, trigger one build or release to verify the complete flow.

If an old event is deleted while a delivery still references it, the delivery keeps its own event snapshot, so historical delivery details remain readable.

Old test data or an interrupted write may contain JSON `null` in `detail_json` or `links_json`. The event API normalizes these values and malformed objects to `{}`, and the frontend repeats that normalization at its API boundary. Missing details or links therefore hide only those sections instead of crashing the event list or details page.

## Delivery order

1. Add the `PlatformEvent` model, migration, catalog, and write service.
2. Make notification matching consume stored events while preserving current failure notifications.
3. Emit events for builds, releases, hooks, routes, and certificate state changes.
4. Add list/detail APIs, authorization filters, and the frontend event page.
5. Add filtered links from project overviews and application pages.
6. Add retention cleanup, metrics, and deduplication tests.

## Acceptance criteria

- A failed build or release appears even when no notification rule exists.
- A business state change creates one event; retries and periodic reconciliation do not flood the timeline.
- Regular members cannot read events from another project space or platform-level events.
- Event details link directly to the relevant build, release, hook, route, or certificate page.
- Notification deliveries reference the same event ID and are visible from event details.
- Disabling every notification channel does not stop event recording.
- Sensitive values never enter the event table, API response, or notification template variables.
