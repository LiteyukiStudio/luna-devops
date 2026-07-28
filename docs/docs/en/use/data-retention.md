# Data Retention and Cleanup

Events, notification deliveries, worker task traces, and build or deployment logs keep growing as the platform runs. Platform administrators can configure automatic retention under **Site Settings > Data retention**, or preview and remove data from a specific time range.

## Automatic retention

The worker runs an independent retention task every 24 hours. Each dataset has its own retention period:

| Dataset | Default | Cleanup boundary |
| --- | ---: | --- |
| Platform events | 90 days | Uses the event occurrence time |
| Notification deliveries | 90 days | Removes only succeeded or failed deliveries |
| Worker task events | 30 days | Removes historical task traces |
| Build logs | 30 days | Removes logs from completed builds while preserving build records |
| Release logs | 90 days | Removes logs from completed releases while preserving releases and rollback history |
| Hook logs | 90 days | Removes logs from completed hooks while preserving run results |
| Expired authentication data | 30 days | Removes only expired sessions, remember tokens, and Step-up assertions |

Set a value to `0` to disable automatic cleanup for that dataset. Saving a policy affects future retention runs and does not immediately delete existing data.

Cleanup uses a fixed dataset catalog and small batches. Logs attached to active builds, releases, or hooks are protected. A future end time also cannot remove authentication records that are still valid.

## Manual cleanup

Manual cleanup is available only to platform administrators. When the platform Step-up MFA policy is enabled, the administrator must also complete the corresponding verification before cleanup:

1. Select one or more datasets.
2. Select a start and end time. The interval is `[startAt, endAt)`.
3. Preview the matching row count for every dataset.
4. Confirm the irreversible cleanup action.

Previewing does not change the database. Changing the datasets or time range invalidates the previous preview. The platform writes a summary to the audit log after cleanup, without copying removed log content into the audit record.

## Protected data

The following data is not part of the retention catalog and cannot be removed through the manual cleanup API:

- audit logs;
- billing usage, wallets, and immutable ledger entries;
- build records, build-job metadata, and image records;
- release records, revisions, and rollback relationships;
- hook results and script snapshots;
- secrets, tokens, registry credentials, and identity-provider settings;
- Kubernetes PVCs and application runtime data.

Build and release entries remain visible after their old log body has been removed. Connect an external log system or archive storage before cleanup when long-term log access is required.

## Replicas and failures

Asynq schedules automatic retention, and one worker consumes each task after it is enqueued. An accidental duplicate trigger from multiple replicas cannot expand the cleanup range. Deletion runs in fixed-size batches; if a run fails halfway through, completed batches remain deleted and the next run continues with the remaining rows.
