import type { AIInternalRouteName } from './internal-routes'
import type { ToolCallBlock } from './tool-call'
import type { AIUIAction } from '@/api'
import { LoaderCircle, Navigation } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getAIUIActionTargetPath } from './actions'

const routeLabelKeys: Record<AIInternalRouteName, string> = {
  'dashboard': 'dashboard',
  'projects': 'projects',
  'project.workspace': 'projectWorkspace',
  'application.detail': 'applicationDetail',
  'events': 'events',
  'code-repositories': 'codeRepositories',
  'registries': 'registries',
  'clusters': 'clusters',
  'app-templates': 'appTemplates',
  'billing': 'billing',
  'settings.account': 'account',
  'settings.auth-providers': 'authProviders',
  'settings.notifications': 'notifications',
  'settings.operations': 'operations',
  'settings.site': 'siteSettings',
  'settings.users': 'users',
}

export function AINavigationEvent({ block, onAction }: { block: ToolCallBlock, onAction: (action: AIUIAction) => Promise<boolean> }) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const action = block.uiActions.find(candidate => candidate.type === 'navigate')
  const routeName = action?.type === 'navigate' ? action.payload.routeName as AIInternalRouteName : undefined
  const target = routeName && routeLabelKeys[routeName]
    ? t(`aiAssistant.navigation.routes.${routeLabelKeys[routeName]}`)
    : t('aiAssistant.navigation.unknownTarget')
  const targetPath = action ? getAIUIActionTargetPath(action) : null

  const reopen = async () => {
    if (!action || pending)
      return
    try {
      setPending(true)
      if (!await onAction(action))
        toast.error(t('aiAssistant.actions.unavailable'))
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('aiAssistant.actions.unavailable'))
    }
    finally {
      setPending(false)
    }
  }

  return (
    <button
      aria-label={t('aiAssistant.navigation.reopen', { target })}
      className="inline-flex h-6 max-w-full w-fit items-center gap-1.5 rounded-full bg-primary-subtle px-2.5 text-[11px] font-medium text-primary-text transition-colors hover:bg-primary-subtle-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-70"
      data-ai-navigation-event
      disabled={!action || pending}
      title={targetPath ?? undefined}
      type="button"
      onClick={() => void reopen()}
    >
      {pending ? <LoaderCircle aria-hidden="true" className="size-3 animate-spin motion-reduce:animate-pulse" /> : <Navigation aria-hidden="true" className="size-3" />}
      <span className="truncate">{t('aiAssistant.navigation.completed', { target })}</span>
    </button>
  )
}
