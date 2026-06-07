import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Card } from '@/components/ui/card'

/**
 * 全屏无权限页面。
 * 用于路由级 403 或后端确认用户无访问权限的页面兜底；按钮级权限隐藏和局部提示不要跳转到这里。
 */
export function ForbiddenPage() {
  const { t } = useTranslation()
  return (
    <div className="grid min-h-screen place-items-center bg-background px-4 text-foreground">
      <Card className="w-full max-w-md">
        <h1 className="text-xl font-semibold">{t('common.forbiddenTitle')}</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          {t('common.forbiddenDescription')}
        </p>
        <Link className="mt-5 inline-flex h-9 items-center justify-center rounded-full bg-primary px-4 text-sm font-medium text-primary-foreground transition hover:bg-primary/90" to="/projects">
          {t('backToProjectSpaces')}
        </Link>
      </Card>
    </div>
  )
}
