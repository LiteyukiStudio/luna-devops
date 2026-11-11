import { useQuery } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

/**
 * 防跳板提示：项目空间持有平台授权的内网出站例外时，添加成员、开放访问入口或跨项目共享服务都会扩大可触达内网的范围。
 * 只提示不拦截；读取失败时不显示，避免以空状态冒充“没有例外”之外的结论。
 * 跨项目共享只在部署配置所在集群持有例外时需要批准，因此共享表单按所选部署配置的集群过滤。
 */
export function EgressExceptionWarning({ clusterId, context, projectId }: { clusterId?: string, context: 'member' | 'route' | 'share', projectId: string }) {
  const { t } = useTranslation()
  const policy = useQuery({
    queryKey: ['project-network-policy', projectId],
    queryFn: ({ signal }) => api.getProjectNetworkPolicy(projectId, signal),
    enabled: Boolean(projectId),
  })
  // clusterId 只统计该集群上的例外；未指定（含使用默认集群的部署配置）时按全部例外提示，宁可多提示。
  const count = (policy.data?.egressExceptions ?? []).filter(exception => !clusterId || exception.runtimeClusterId === clusterId).length
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
