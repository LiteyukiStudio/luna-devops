import { useQuery } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

/**
 * 防跳板提示：项目空间持有平台授权的内网出站例外时，添加成员或开放访问入口都会扩大可触达内网的范围。
 * 只提示不拦截；读取失败时不显示，避免以空状态冒充“没有例外”之外的结论。
 */
export function EgressExceptionWarning({ context, projectId }: { context: 'member' | 'route', projectId: string }) {
  const { t } = useTranslation()
  const policy = useQuery({
    queryKey: ['project-network-policy', projectId],
    queryFn: ({ signal }) => api.getProjectNetworkPolicy(projectId, signal),
    enabled: Boolean(projectId),
  })
  const count = policy.data?.egressExceptions.length ?? 0
  if (count === 0)
    return null
  return (
    <Alert variant="warning">
      <ShieldAlert />
      <AlertTitle>{t('common.egressExceptionWarning.title', { count })}</AlertTitle>
      <AlertDescription>{t(`common.egressExceptionWarning.${context}`)}</AlertDescription>
    </Alert>
  )
}
