import type { Application } from '@/api'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ApplicationIcon } from '@/components/common/application-icon-picker'
import { DeploymentReplicaBadge } from '@/components/common/deployment-replica-badge'
import { HoverText } from '@/components/common/hover-text'
import { StatusValueBadge } from '@/components/common/status-badge'

export function ApplicationSummary({ application, projectId }: { application: Application, projectId: string }) {
  const { t } = useTranslation()
  const deleting = application.deleteStatus === 'deleting'
  const deleteFailedMessage = application.deleteStatus === 'delete_failed' ? application.deleteMessage?.trim() : ''
  const deploymentTargets = application.deploymentSummary?.targets ?? []
  return (
    <div className="flex min-w-0 items-center gap-3">
      <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <ApplicationIcon name={application.icon} />
      </span>
      <div className="min-w-0 w-full">
        <div className="flex flex-wrap items-center gap-2">
          <Link className={`min-w-0 truncate font-medium transition hover:text-primary-text ${deleting ? 'pointer-events-none opacity-60' : ''}`} to={`/projects/${projectId}/apps/${application.id}`}>
            {application.name}
          </Link>
          {application.deleteStatus && application.deleteStatus !== 'active' && (
            <StatusValueBadge labelKeyPrefix="apps.deleteStatuses" value={application.deleteStatus} />
          )}
          {deleteFailedMessage && (
            <HoverText className="flex-1 text-xs text-muted-foreground" value={deleteFailedMessage} />
          )}
          {application.deleteStatus !== 'deleting' && application.deploymentSummary && (
            <span className="flex flex-wrap items-center gap-1.5">
              {deploymentTargets.length === 0
                ? (
                    <DeploymentReplicaBadge deployed={false} desiredReplicas={0} readyReplicas={0} status="not-deployed" />
                  )
                : deploymentTargets.map(target => (
                    <DeploymentReplicaBadge
                      key={target.id}
                      desiredReplicas={target.desiredReplicas}
                      prefix={t(`deploymentsPage.stageLabels.${target.stage}`, { defaultValue: target.stage })}
                      readyReplicas={target.readyReplicas}
                      status={target.status}
                    />
                  ))}
            </span>
          )}
        </div>
        <p className="truncate text-sm text-muted-foreground">
          {application.identifier}
        </p>
      </div>
    </div>
  )
}
