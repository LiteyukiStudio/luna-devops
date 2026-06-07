import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'

/**
 * 全屏认证错误页。
 * 用于登录、OIDC 回调、会话恢复等认证流程失败后的兜底页面；不要放在普通表单字段或局部卡片错误里。
 */
export function AuthErrorPage({ title, description }: { title: string, description: string }) {
  const { t } = useTranslation()
  return (
    <div className="grid min-h-screen place-items-center bg-background px-4 text-foreground">
      <Card className="w-full max-w-md">
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="mt-2 text-sm text-muted-foreground">{description}</p>
        <Button className="mt-5">
          <Link to="/login">{t('auth.backToLogin')}</Link>
        </Button>
      </Card>
    </div>
  )
}
