# Data Retention and Cleanup

Administrators can configure automatic retention under **Site Settings → Data Retention**, or preview and clean data from a selected period.

## Automatic retention

| Data | Default | Cleanup scope |
| --- | ---: | --- |
| Platform events | 90 days | Expired events |
| Notification deliveries | 90 days | Finished deliveries |
| Worker task events | 30 days | Historical task traces |
| Build logs | 30 days | Logs from finished builds; build records remain |
| Release logs | 90 days | Logs from finished releases; release records remain |
| Hook logs | 90 days | Logs from finished hooks; results remain |
| Expired authentication data | 30 days | Expired sessions and verification data |

Set a value to `0` to disable automatic cleanup for that category. Changes affect later cleanup runs and do not delete data immediately. Active task logs and valid sessions are not removed.

## Manual cleanup

Only administrators can run manual cleanup. If step-up verification is enabled, MFA is also required.

1. Select data categories and a time range.
2. Preview the number of matching records.
3. Verify the scope and confirm cleanup.

Preview does not change data. Changing the selection requires a new preview. Cleanup cannot be undone and its result is audited; export anything that requires long-term retention first.

## Data that is not removed

This feature does not remove audit logs, billing ledgers, build and release records, hook results, secrets and identity configuration, Kubernetes PVCs, or application data.

After log cleanup, build and release records remain, but their expired log text is no longer available. Use an external log or archive system when longer retention is required.
