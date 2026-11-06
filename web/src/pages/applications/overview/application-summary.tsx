import type { Application } from '@/api'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ApplicationIcon } from '@/components/common/application-icon-picker'
import { ResourceDeletionStatus } from '@/components/common/resource-deletion-status'
import { ReplicaStatusIndicator } from '@/pages/applications/deployments/operations/application-deployment-runtime'
import { applicationDeleteFailureMessage } from './application-delete-failure'

/** 应用名称单元：图标 + 名称链接 + 标识符。名称是进入详情的唯一入口。 */
export function ApplicationSummary({ application, projectId }: { application: Application, projectId: string }) {
  const { t } = useTranslation()
  const deleting = application.deleteStatus === 'deleting'
  return (
    <div className="flex min-w-0 items-center gap-3">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-control bg-muted text-muted-foreground">
        <ApplicationIcon name={application.icon} />
      </span>
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Link
            aria-disabled={deleting}
            className={`min-w-0 truncate rounded-sm font-medium transition hover:text-primary-text hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${deleting ? 'pointer-events-none opacity-60' : ''}`}
            tabIndex={deleting ? -1 : undefined}
            to={`/projects/${projectId}/apps/${application.id}`}
          >
            {application.name}
          </Link>
          <ResourceDeletionStatus message={applicationDeleteFailureMessage(t, application)} status={application.deleteStatus} />
        </div>
        <p className="truncate font-mono text-xs text-muted-foreground">{application.identifier}</p>
      </div>
    </div>
  )
}

/** 应用运行状态：每个部署环境一行“环境 + 圆点 + 就绪副本”；没有部署配置时显示未部署。 */
export function ApplicationRuntimeStatus({ application }: { application: Application }) {
  const { t } = useTranslation()
  const summary = application.deploymentSummary
  if (!summary || application.deleteStatus === 'deleting')
    return <span className="text-sm text-muted-foreground">-</span>
  if (summary.targets.length === 0)
    return <ReplicaStatusIndicator deployed={false} desiredReplicas={0} readyReplicas={0} status="not-deployed" />
  return (
    <ul className="grid min-w-0 gap-1">
      {summary.targets.map(target => (
        <li key={target.id} className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 text-xs text-muted-foreground">{t(`deploymentsPage.stageLabels.${target.stage}`, { defaultValue: target.stage })}</span>
          <ReplicaStatusIndicator desiredReplicas={target.desiredReplicas} readyReplicas={target.readyReplicas} status={target.status} />
        </li>
      ))}
    </ul>
  )
}
