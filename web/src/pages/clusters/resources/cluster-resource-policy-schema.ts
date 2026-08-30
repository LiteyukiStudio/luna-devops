import { z } from 'zod'

export function clusterResourcePolicySchema(t: (key: string) => string) {
  const percent = z.number().int().min(0, t('clustersPage.resourcePercentRange')).max(100, t('clustersPage.resourcePercentRange'))
  return z.object({
    cpuRequestPercent: percent,
    memoryRequestPercent: percent,
    cpuLimitPercent: percent,
    memoryLimitPercent: percent,
  }).superRefine((values, context) => {
    if (values.cpuLimitPercent > 0 && values.cpuRequestPercent > values.cpuLimitPercent)
      context.addIssue({ code: 'custom', message: t('clustersPage.requestExceedsLimit'), path: ['cpuRequestPercent'] })
    if (values.memoryLimitPercent > 0 && values.memoryRequestPercent > values.memoryLimitPercent)
      context.addIssue({ code: 'custom', message: t('clustersPage.requestExceedsLimit'), path: ['memoryRequestPercent'] })
  })
}
