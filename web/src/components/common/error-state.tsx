import { AlertTriangle } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

/**
 * 局部内容加载失败时的统一错误提示。
 * 用于列表、详情块、表单辅助数据等局部失败；整页 403 使用 ForbiddenPage，认证失败使用 AuthErrorPage。
 */
export function ErrorState({ title, description }: { title: string, description?: string }) {
  return (
    <Alert variant="destructive">
      <AlertTriangle />
      <AlertTitle>{title}</AlertTitle>
      {description && <AlertDescription>{description}</AlertDescription>}
    </Alert>
  )
}
