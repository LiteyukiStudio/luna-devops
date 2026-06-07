import type { ReactNode } from 'react'
import type { StatusTone } from '@/components/common/status-tone'
import { useTranslation } from 'react-i18next'
import { statusToneFor } from '@/components/common/status-tone'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

/**
 * 资源状态的统一小徽标。
 * 用于角色、状态、scope 等短文本标签；涉及健康、任务、启停、校验等状态时传入 tone。
 * 复杂状态说明或带操作的提示请使用 Alert/Card 组合。
 */
export function StatusBadge({ children, className, tone }: { children: ReactNode, className?: string, tone?: StatusTone }) {
  return <Badge className={cn(tone ? statusToneClassName(tone) : undefined, className)}>{children}</Badge>
}

/**
 * 按状态值自动着色的徽标。
 * 用于集群健康状态、构建/部署任务状态、Webhook/DNS/证书/镜像扫描等状态列。
 * 翻译优先读取 labelKeyPrefix.value；未提供时读取 common.value；缺失时回退原始值。
 */
export function StatusValueBadge({
  label,
  labelKeyPrefix,
  value,
}: {
  label?: ReactNode
  labelKeyPrefix?: string
  value: string
}) {
  const { t } = useTranslation()
  const normalized = value.trim()
  const commonLabelKey = `common.${statusI18nKey(normalized)}`
  const translated = label ?? (labelKeyPrefix
    ? t(`${labelKeyPrefix}.${normalized}`, { defaultValue: t(commonLabelKey, { defaultValue: normalized }) })
    : t(commonLabelKey, { defaultValue: normalized }))

  return <StatusBadge tone={statusToneFor(normalized)}>{translated}</StatusBadge>
}

function statusI18nKey(value: string) {
  return value.replace(/-([a-z])/g, (_, char: string) => char.toUpperCase())
}

function statusToneClassName(tone: StatusTone) {
  switch (tone) {
    case 'success':
      return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-300'
    case 'warning':
      return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-300'
    case 'danger':
      return 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-900/60 dark:bg-rose-950/40 dark:text-rose-300'
    case 'info':
      return 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900/60 dark:bg-sky-950/40 dark:text-sky-300'
    case 'neutral':
      return 'border-zinc-200 bg-zinc-50 text-zinc-700 dark:border-zinc-800 dark:bg-zinc-900/60 dark:text-zinc-300'
  }
}
