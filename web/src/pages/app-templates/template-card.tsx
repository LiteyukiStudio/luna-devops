import type { ReactNode } from 'react'
import type { AppTemplate, AppTemplateSummary } from '@/api'
import { ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { cn } from '@/lib/utils'
import { isMultiComponentTemplate, isSystemComponentTemplate } from './install-form'
import { templateCategoryLabel, templateDescription } from './template-text'

const FALLBACK_ICON = '/app-templates/icons/fallback.svg'

/** 模板卡整卡是一个按钮，打开模板详情与安装抽屉。 */
export function TemplateCard({ template, onOpen }: { template: AppTemplateSummary, onOpen: () => void }) {
  const { t } = useTranslation()
  const systemComponent = isSystemComponentTemplate(template)
  const multiComponent = isMultiComponentTemplate(template)
  return (
    <button
      className="flex min-h-40 min-w-0 flex-col rounded-container border border-border bg-surface-raised p-4 text-left transition-colors duration-standard ease-standard hover:border-primary-border hover:bg-surface-subtle focus-visible:border-primary focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-primary/50"
      type="button"
      onClick={onOpen}
    >
      <span className="flex min-w-0 items-start gap-3">
        <TemplateIcon icon={template.icon} />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold text-foreground">{template.name}</span>
          <span className="mt-0.5 block truncate text-xs text-muted-foreground">{templateCategoryLabel(template.category, t)}</span>
        </span>
        {systemComponent && <StatusBadge className="shrink-0" tone="neutral">{t('appTemplatesPage.platformComponent')}</StatusBadge>}
      </span>
      <span className="mt-3 line-clamp-2 text-sm leading-5 text-muted-foreground">
        {templateDescription(template, t)}
      </span>
      <span className="mt-auto flex min-w-0 items-center gap-3 pt-3 text-xs text-muted-foreground">
        {template.version && <span className="truncate font-mono">{template.version}</span>}
        {multiComponent && <span className="shrink-0">{t('appTemplatesPage.componentCount', { count: template.components.length })}</span>}
      </span>
    </button>
  )
}

export function TemplateIcon({ className, icon }: { className?: string, icon: string }) {
  return (
    <span className={cn('flex size-10 shrink-0 items-center justify-center rounded-container border border-border bg-surface-inset', className)}>
      <img
        alt=""
        className="size-6 object-contain"
        src={icon || FALLBACK_ICON}
        onError={(event) => {
          event.currentTarget.src = FALLBACK_ICON
        }}
      />
    </span>
  )
}

export function TemplateSourceLinks({ template }: { template: Pick<AppTemplate, 'officialRepository' | 'officialWebsite'> }) {
  const { t } = useTranslation()
  return (
    <span className="flex shrink-0 items-center gap-3">
      <TemplateSourceLink
        href={template.officialWebsite}
        icon={<ExternalLink className="size-3.5" />}
        label={t('appTemplatesPage.officialWebsite')}
      />
      <TemplateSourceLink
        href={template.officialRepository}
        icon={<GithubMark className="size-3.5" />}
        label={t('appTemplatesPage.officialRepository')}
      />
    </span>
  )
}

function TemplateSourceLink({ href, icon, label }: { href: string, icon: ReactNode, label: string }) {
  if (!/^https?:\/\//i.test(href))
    return null
  return (
    <a
      className="inline-flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-primary-text focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      href={href}
      rel="noopener noreferrer"
      target="_blank"
    >
      {icon}
      {label}
    </a>
  )
}

function GithubMark({ className }: { className?: string }) {
  return (
    <svg
      aria-hidden="true"
      className={className}
      fill="currentColor"
      viewBox="0 0 24 24"
    >
      <path d="M12 2C6.48 2 2 6.58 2 12.25c0 4.53 2.87 8.37 6.84 9.72.5.09.68-.22.68-.49v-1.9c-2.78.62-3.37-1.22-3.37-1.22-.45-1.19-1.11-1.5-1.11-1.5-.91-.64.07-.63.07-.63 1 .07 1.53 1.06 1.53 1.06.89 1.56 2.34 1.11 2.91.85.09-.66.35-1.11.63-1.37-2.22-.26-4.56-1.14-4.56-5.06 0-1.12.39-2.03 1.03-2.75-.1-.26-.45-1.3.1-2.71 0 0 .84-.28 2.75 1.05A9.3 9.3 0 0 1 12 6.98c.85 0 1.7.12 2.5.34 1.91-1.33 2.75-1.05 2.75-1.05.55 1.41.2 2.45.1 2.71.64.72 1.03 1.63 1.03 2.75 0 3.93-2.34 4.79-4.57 5.05.36.32.68.95.68 1.91v2.79c0 .27.18.59.69.49A10.12 10.12 0 0 0 22 12.25C22 6.58 17.52 2 12 2Z" />
    </svg>
  )
}
