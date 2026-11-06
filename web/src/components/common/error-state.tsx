import { AlertTriangle, RotateCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

/**
 * 局部内容加载失败时的统一错误提示。
 * 用于列表、详情块、表单辅助数据等局部失败；整页 403 使用 ForbiddenPage，认证流程在对应页面处理整页错误。
 * 传入 onRetry 时提供重试按钮，retrying 期间禁用，避免重复请求。
 */
export function ErrorState({ description, onRetry, retrying = false, title }: {
  description?: string
  onRetry?: () => void
  retrying?: boolean
  title: string
}) {
  const { t } = useTranslation()
  return (
    <Alert variant="destructive">
      <AlertTriangle />
      <AlertTitle>{title}</AlertTitle>
      {(description || onRetry) && (
        <AlertDescription>
          {description && <p>{description}</p>}
          {onRetry && (
            <Button className="mt-2" disabled={retrying} size="sm" type="button" variant="outline" onClick={onRetry}>
              <RotateCw className={retrying ? 'size-4 animate-spin motion-reduce:animate-none' : 'size-4'} />
              {t('common.retry')}
            </Button>
          )}
        </AlertDescription>
      )}
    </Alert>
  )
}
