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
	DatasetWorkerTaskEvents: {
		dataset: catalog[2],
		queries: []querySpec{{
			countSQL: `SELECT COUNT(*) AS count
FROM worker_task_events
WHERE created_at >= ? AND created_at < ?`,
			deleteSQL: `WITH victims AS (
    SELECT id
    FROM worker_task_events
    WHERE created_at >= ? AND created_at < ?
    ORDER BY created_at, id
    LIMIT 1000
)
DELETE FROM worker_task_events AS target
USING victims
WHERE target.id = victims.id`,
		}},
	},
	DatasetBuildLogs: {
		dataset: catalog[3],
		queries: []querySpec{{
			countSQL: `SELECT COUNT(*) AS count
FROM build_logs AS logs
JOIN build_runs AS parent ON parent.id = logs.build_run_id
WHERE parent.status IN ('succeeded', 'failed', 'canceled', 'lost', 'timeout')
  AND parent.finished_at >= ? AND parent.finished_at < ?`,
			deleteSQL: `WITH victims AS (
    SELECT logs.id
    FROM build_logs AS logs
    JOIN build_runs AS parent ON parent.id = logs.build_run_id
    WHERE parent.status IN ('succeeded', 'failed', 'canceled', 'lost', 'timeout')
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
		dataset: catalog[4],
		queries: []querySpec{{
			countSQL: `SELECT COUNT(*) AS count
FROM release_logs AS logs
JOIN releases AS parent ON parent.id = logs.release_id
WHERE parent.status IN ('succeeded', 'failed')
  AND parent.finished_at >= ? AND parent.finished_at < ?`,
			deleteSQL: `WITH victims AS (
    SELECT logs.id
    FROM release_logs AS logs
    JOIN releases AS parent ON parent.id = logs.release_id
    WHERE parent.status IN ('succeeded', 'failed')
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
		dataset: catalog[5],
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
		dataset: catalog[6],
		queries: []querySpec{
			{
				requireExpired: true,
				countSQL: `SELECT COUNT(*) AS count
FROM step_up_assertions
WHERE LEAST(idle_expires_at, absolute_expires_at) >= ?
  AND LEAST(idle_expires_at, absolute_expires_at) < ?
  AND LEAST(idle_expires_at, absolute_expires_at) <= ?`,
				deleteSQL: `WITH victims AS (
    SELECT id
    FROM step_up_assertions
    WHERE LEAST(idle_expires_at, absolute_expires_at) >= ?
      AND LEAST(idle_expires_at, absolute_expires_at) < ?
      AND LEAST(idle_expires_at, absolute_expires_at) <= ?
    ORDER BY LEAST(idle_expires_at, absolute_expires_at), id
    LIMIT 1000
)
DELETE FROM step_up_assertions AS target
USING victims
WHERE target.id = victims.id`,
			},
			{
				requireExpired: true,
				windowCount:    2,
				countSQL: `SELECT COUNT(*) AS count
FROM user_sessions AS session
WHERE session.expires_at >= ? AND session.expires_at < ? AND session.expires_at <= ?
  AND NOT EXISTS (
      SELECT 1
      FROM step_up_assertions AS assertion
      WHERE assertion.session_id = session.id
        AND NOT (
            LEAST(assertion.idle_expires_at, assertion.absolute_expires_at) >= ?
            AND LEAST(assertion.idle_expires_at, assertion.absolute_expires_at) < ?
            AND LEAST(assertion.idle_expires_at, assertion.absolute_expires_at) <= ?
        )
  )`,
				deleteSQL: `WITH victims AS (
    SELECT session.id
    FROM user_sessions AS session
    WHERE session.expires_at >= ? AND session.expires_at < ? AND session.expires_at <= ?
      AND NOT EXISTS (
          SELECT 1
          FROM step_up_assertions AS assertion
          WHERE assertion.session_id = session.id
            AND NOT (
                LEAST(assertion.idle_expires_at, assertion.absolute_expires_at) >= ?
                AND LEAST(assertion.idle_expires_at, assertion.absolute_expires_at) < ?
                AND LEAST(assertion.idle_expires_at, assertion.absolute_expires_at) <= ?
            )
      )
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
FROM user_remember_tokens
WHERE expires_at >= ? AND expires_at < ? AND expires_at <= ?`,
				deleteSQL: `WITH victims AS (
    SELECT id
    FROM user_remember_tokens
    WHERE expires_at >= ? AND expires_at < ? AND expires_at <= ?
    ORDER BY expires_at, id
    LIMIT 1000
)
DELETE FROM user_remember_tokens AS target
USING victims
WHERE target.id = victims.id`,
			},
		},
	},
}
