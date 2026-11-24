package retention

var plans = map[string]datasetPlan{
	DatasetPlatformEvents: {
		dataset: catalog[0],
		queries: []querySpec{{
			countSQL: `SELECT COUNT(*) AS count
FROM platform_events
WHERE occurred_at >= ? AND occurred_at < ?`,
			deleteSQL: `WITH victims AS (
    SELECT id
    FROM platform_events
    WHERE occurred_at >= ? AND occurred_at < ?
    ORDER BY occurred_at, id
    LIMIT 1000
)
DELETE FROM platform_events AS target
USING victims
WHERE target.id = victims.id`,
		}},
	},
	DatasetNotificationDeliveries: {
		dataset: catalog[1],
		queries: []querySpec{{
			countSQL: `SELECT COUNT(*) AS count
FROM notification_deliveries
WHERE status IN ('succeeded', 'failed')
  AND finished_at >= ? AND finished_at < ?`,
			deleteSQL: `WITH victims AS (
    SELECT id
    FROM notification_deliveries
    WHERE status IN ('succeeded', 'failed')
      AND finished_at >= ? AND finished_at < ?
    ORDER BY finished_at, id
    LIMIT 1000
)
DELETE FROM notification_deliveries AS target
USING victims
WHERE target.id = victims.id`,
		}},
	},
	DatasetBuildLogs: {
		dataset: catalog[2],
		queries: []querySpec{{
			countSQL: `SELECT COUNT(*) AS count
FROM build_logs AS logs
JOIN build_runs AS parent ON parent.id = logs.build_run_id
WHERE parent.status IN ('succeeded', 'failed', 'canceled', 'timeout')
  AND parent.finished_at >= ? AND parent.finished_at < ?`,
			deleteSQL: `WITH victims AS (
    SELECT logs.id
    FROM build_logs AS logs
    JOIN build_runs AS parent ON parent.id = logs.build_run_id
    WHERE parent.status IN ('succeeded', 'failed', 'canceled', 'timeout')
      AND parent.finished_at >= ? AND parent.finished_at < ?
    ORDER BY parent.finished_at, logs.id
    LIMIT 1000
)
DELETE FROM build_logs AS target
USING victims
WHERE target.id = victims.id`,
		}},
	},
	DatasetReleaseLogs: {
		dataset: catalog[3],
		queries: []querySpec{{
			countSQL: `SELECT COUNT(*) AS count
FROM release_logs AS logs
JOIN releases AS parent ON parent.id = logs.release_id
WHERE parent.status IN ('succeeded', 'failed', 'canceled')
  AND parent.finished_at >= ? AND parent.finished_at < ?`,
			deleteSQL: `WITH victims AS (
    SELECT logs.id
    FROM release_logs AS logs
    JOIN releases AS parent ON parent.id = logs.release_id
    WHERE parent.status IN ('succeeded', 'failed', 'canceled')
      AND parent.finished_at >= ? AND parent.finished_at < ?
    ORDER BY parent.finished_at, logs.id
    LIMIT 1000
)
DELETE FROM release_logs AS target
USING victims
WHERE target.id = victims.id`,
		}},
	},
	DatasetHookRunLogs: {
		dataset: catalog[4],
		queries: []querySpec{{
			countSQL: `SELECT COUNT(*) AS count
FROM hook_run_logs AS logs
JOIN hook_runs AS parent ON parent.id = logs.hook_run_id
WHERE parent.status IN ('succeeded', 'failed')
  AND parent.finished_at >= ? AND parent.finished_at < ?`,
			deleteSQL: `WITH victims AS (
    SELECT logs.id
    FROM hook_run_logs AS logs
    JOIN hook_runs AS parent ON parent.id = logs.hook_run_id
    WHERE parent.status IN ('succeeded', 'failed')
      AND parent.finished_at >= ? AND parent.finished_at < ?
    ORDER BY parent.finished_at, logs.id
    LIMIT 1000
)
DELETE FROM hook_run_logs AS target
USING victims
WHERE target.id = victims.id`,
		}},
	},
	DatasetExpiredAuthData: {
		dataset: catalog[5],
		queries: []querySpec{
			{
				requireExpired: true,
				countSQL: `SELECT COUNT(*) AS count
FROM user_sessions AS session
WHERE session.expires_at >= ? AND session.expires_at < ? AND session.expires_at <= ?`,
				deleteSQL: `WITH victims AS (
    SELECT session.id
    FROM user_sessions AS session
    WHERE session.expires_at >= ? AND session.expires_at < ? AND session.expires_at <= ?
    ORDER BY session.expires_at, session.id
    LIMIT 1000
)
DELETE FROM user_sessions AS target
USING victims
WHERE target.id = victims.id`,
			},
			{
				requireExpired: true,
				countSQL: `SELECT COUNT(*) AS count
FROM email_registration_challenges
WHERE expires_at >= ? AND expires_at < ? AND expires_at <= ?`,
				deleteSQL: `WITH victims AS (
    SELECT id
    FROM email_registration_challenges
    WHERE expires_at >= ? AND expires_at < ? AND expires_at <= ?
    ORDER BY expires_at, id
    LIMIT 1000
)
DELETE FROM email_registration_challenges AS target
USING victims
WHERE target.id = victims.id`,
			},
			{
				requireExpired: true,
				countSQL: `SELECT COUNT(*) AS count
FROM oauth_authorization_codes
WHERE expires_at >= ? AND expires_at < ? AND expires_at <= ?`,
				deleteSQL: `WITH victims AS (
    SELECT id
    FROM oauth_authorization_codes
    WHERE expires_at >= ? AND expires_at < ? AND expires_at <= ?
    ORDER BY expires_at, id
    LIMIT 1000
)
DELETE FROM oauth_authorization_codes AS target
USING victims
WHERE target.id = victims.id`,
			},
			{
				requireExpired: true,
				countSQL: `SELECT COUNT(*) AS count
FROM oauth_refresh_tokens
WHERE expires_at >= ? AND expires_at < ? AND expires_at <= ?`,
				deleteSQL: `WITH victims AS (
    SELECT id
    FROM oauth_refresh_tokens
    WHERE expires_at >= ? AND expires_at < ? AND expires_at <= ?
    ORDER BY expires_at, id
    LIMIT 1000
)
DELETE FROM oauth_refresh_tokens AS target
USING victims
WHERE target.id = victims.id`,
			},
			{
				requireExpired: true,
				countSQL: `SELECT COUNT(*) AS count
FROM oauth_applications AS app
WHERE app.registration_source = 'dynamic'
  AND app.created_at >= ? AND app.created_at < ?
  AND app.created_at <= (?::timestamptz - interval '24 hours')
  AND NOT EXISTS (SELECT 1 FROM oauth_grants AS g WHERE g.application_id = app.id)
  AND NOT EXISTS (SELECT 1 FROM access_tokens AS t WHERE t.oauth_application_id = app.id)
  AND NOT EXISTS (SELECT 1 FROM oauth_refresh_tokens AS r WHERE r.application_id = app.id)
  AND NOT EXISTS (SELECT 1 FROM oauth_authorization_codes AS c WHERE c.application_id = app.id)`,
				deleteSQL: `WITH victims AS (
    SELECT app.id
    FROM oauth_applications AS app
    WHERE app.registration_source = 'dynamic'
      AND app.created_at >= ? AND app.created_at < ?
      AND app.created_at <= (?::timestamptz - interval '24 hours')
      AND NOT EXISTS (SELECT 1 FROM oauth_grants AS g WHERE g.application_id = app.id)
      AND NOT EXISTS (SELECT 1 FROM access_tokens AS t WHERE t.oauth_application_id = app.id)
      AND NOT EXISTS (SELECT 1 FROM oauth_refresh_tokens AS r WHERE r.application_id = app.id)
      AND NOT EXISTS (SELECT 1 FROM oauth_authorization_codes AS c WHERE c.application_id = app.id)
    ORDER BY app.created_at, app.id
    LIMIT 1000
)
DELETE FROM oauth_applications AS target
USING victims
WHERE target.id = victims.id`,
			},
		},
	},
	DatasetRuntimeObservations: {
		dataset: catalog[6],
		queries: []querySpec{{
			requireExpired: true,
			countSQL: `SELECT COUNT(*) AS count
FROM runtime_observations
WHERE period_start >= ? AND period_start < ?
  AND period_start <= (?::timestamptz - interval '24 hours')`,
			deleteSQL: `WITH victims AS (
    SELECT id
    FROM runtime_observations
    WHERE period_start >= ? AND period_start < ?
      AND period_start <= (?::timestamptz - interval '24 hours')
    ORDER BY period_start, id
    LIMIT 1000
)
DELETE FROM runtime_observations AS target
USING victims
WHERE target.id = victims.id`,
		}},
	},
	DatasetBillingRecords: {
		dataset: catalog[7],
		queries: []querySpec{
			{
				requireExpired: true,
				countSQL: `SELECT COUNT(*) AS count
FROM billing_ledger_entries AS ledger
LEFT JOIN billing_usage_records AS usage ON usage.id = ledger.usage_record_id
WHERE ledger.type = 'debit'
  AND (ledger.usage_record_id = '' OR usage.id IS NULL OR usage.status = 'settled')
  AND COALESCE(usage.period_start, ledger.created_at) >= ?
  AND COALESCE(usage.period_start, ledger.created_at) < ?
  AND COALESCE(usage.period_start, ledger.created_at) <= (?::timestamptz - interval '24 hours')
  AND NOT (
      COALESCE(usage.resource_type, ledger.resource_type) = 'volume_transfer'
      AND EXISTS (
          SELECT 1 FROM volume_transfers AS vt
          WHERE vt.id = COALESCE(usage.resource_id, ledger.resource_id)
      )
  )`,
				deleteSQL: `WITH victims AS (
    SELECT ledger.id
    FROM billing_ledger_entries AS ledger
    LEFT JOIN billing_usage_records AS usage ON usage.id = ledger.usage_record_id
    WHERE ledger.type = 'debit'
      AND (ledger.usage_record_id = '' OR usage.id IS NULL OR usage.status = 'settled')
      AND COALESCE(usage.period_start, ledger.created_at) >= ?
      AND COALESCE(usage.period_start, ledger.created_at) < ?
      AND COALESCE(usage.period_start, ledger.created_at) <= (?::timestamptz - interval '24 hours')
      AND NOT (
          COALESCE(usage.resource_type, ledger.resource_type) = 'volume_transfer'
          AND EXISTS (
              SELECT 1 FROM volume_transfers AS vt
              WHERE vt.id = COALESCE(usage.resource_id, ledger.resource_id)
          )
      )
    ORDER BY ledger.created_at, ledger.id
    LIMIT 1000
)
DELETE FROM billing_ledger_entries AS target
USING victims
WHERE target.id = victims.id`,
			},
			{
				requireExpired: true,
				countSQL: `SELECT COUNT(*) AS count
FROM billing_usage_records AS usage
WHERE usage.status = 'settled'
  AND usage.period_start >= ? AND usage.period_start < ?
  AND usage.period_start <= (?::timestamptz - interval '24 hours')
  AND NOT EXISTS (
      SELECT 1 FROM billing_ledger_entries AS l
      WHERE l.usage_record_id = usage.id AND l.type <> 'debit'
  )
  AND NOT (
      usage.resource_type = 'volume_transfer'
      AND EXISTS (
          SELECT 1 FROM volume_transfers AS vt
          WHERE vt.id = usage.resource_id
      )
  )`,
				deleteSQL: `WITH victims AS (
    SELECT usage.id
    FROM billing_usage_records AS usage
    WHERE usage.status = 'settled'
      AND usage.period_start >= ? AND usage.period_start < ?
      AND usage.period_start <= (?::timestamptz - interval '24 hours')
      AND NOT EXISTS (
          SELECT 1 FROM billing_ledger_entries AS l
          WHERE l.usage_record_id = usage.id
      )
      AND NOT (
          usage.resource_type = 'volume_transfer'
          AND EXISTS (
              SELECT 1 FROM volume_transfers AS vt
              WHERE vt.id = usage.resource_id
          )
      )
    ORDER BY usage.period_start, usage.id
    LIMIT 1000
)
DELETE FROM billing_usage_records AS target
USING victims
WHERE target.id = victims.id`,
			},
		},
	},
	DatasetAIRunEvents: {
		dataset: catalog[8],
		queries: []querySpec{{
			requireExpired: true,
			countSQL: `SELECT COUNT(*) AS count
FROM ai.run_events AS events
JOIN ai.runs AS parent ON parent.id = events.run_id
WHERE parent.status IN ('completed', 'failed', 'canceled', 'expired', 'interrupted')
  AND COALESCE(parent.completed_at, parent.created_at) >= ?
  AND COALESCE(parent.completed_at, parent.created_at) < ?
  AND COALESCE(parent.completed_at, parent.created_at) <= (?::timestamptz - interval '1 hour')`,
			deleteSQL: `WITH victims AS (
    SELECT events.id
    FROM ai.run_events AS events
    JOIN ai.runs AS parent ON parent.id = events.run_id
    WHERE parent.status IN ('completed', 'failed', 'canceled', 'expired', 'interrupted')
      AND COALESCE(parent.completed_at, parent.created_at) >= ?
      AND COALESCE(parent.completed_at, parent.created_at) < ?
      AND COALESCE(parent.completed_at, parent.created_at) <= (?::timestamptz - interval '1 hour')
    ORDER BY COALESCE(parent.completed_at, parent.created_at), parent.id, events.event_sequence
    LIMIT 1000
)
DELETE FROM ai.run_events
WHERE id IN (SELECT id FROM victims)`,
		}},
	},
}
