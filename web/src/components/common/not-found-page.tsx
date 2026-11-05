import { FileQuestion } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { EmptyState } from '@/components/common/empty-state'
import { PageChromeHeading } from '@/components/common/page-chrome'
import { PageShell } from '@/components/common/page-shell'
import { Button } from '@/components/ui/button'

/** 工作台内未匹配任何路由时的兜底页面，保留侧栏与顶栏，让用户能直接继续导航。 */
export function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <PageShell width="content">
      <PageChromeHeading title={t('common.notFoundTitle')} />
      <EmptyState
        actions={(
          <Button asChild>
            <Link to="/dashboard">{t('common.backToDashboard')}</Link>
          </Button>
        )}
        description={t('common.notFoundDescription')}
        icon={<FileQuestion className="size-5" />}
        title={t('common.notFoundTitle')}
      />
    </PageShell>
  )
}
