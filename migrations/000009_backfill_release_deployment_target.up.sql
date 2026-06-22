UPDATE releases AS rel
SET deployment_target_id = target.id
FROM deployment_targets AS target
WHERE rel.deployment_target_id = ''
  AND rel.project_id = target.project_id
  AND rel.application_id = target.application_id
  AND rel.environment_id = target.environment_id
  AND target.enabled = true
  AND target.delete_status = 'active'
  AND (
    SELECT COUNT(*)
    FROM deployment_targets AS candidate
    WHERE candidate.project_id = rel.project_id
      AND candidate.application_id = rel.application_id
      AND candidate.environment_id = rel.environment_id
      AND candidate.enabled = true
      AND candidate.delete_status = 'active'
  ) = 1;
