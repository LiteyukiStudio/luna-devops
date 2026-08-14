import type { ErrorInfo, ReactNode } from 'react'
import { Component } from 'react'
import { useTranslation } from 'react-i18next'
import { recordInteractionCardRenderError } from '@/lib/telemetry'

type InteractionCardRenderScope = 'group' | 'card' | 'content' | 'field' | 'action'

interface InteractionCardErrorBoundaryProps {
  children: ReactNode
  resetKey: string
  scope: InteractionCardRenderScope
}

interface InteractionCardRenderBoundaryProps {
  children: ReactNode
  fallback: ReactNode
  scope: InteractionCardRenderScope
}

export function InteractionCardErrorBoundary({ children, resetKey, scope }: InteractionCardErrorBoundaryProps) {
  const { t } = useTranslation()
  return (
    <InteractionCardRenderBoundary
      key={resetKey}
      fallback={(
        <div className="rounded-control bg-warning-subtle px-2.5 py-2 text-[10px] text-warning" data-ai-card-render-error={scope} role="status">
          {t(`aiAssistant.cards.renderError.${scope}`)}
        </div>
      )}
      scope={scope}
    >
      {children}
    </InteractionCardRenderBoundary>
  )
}

class InteractionCardRenderBoundary extends Component<InteractionCardRenderBoundaryProps, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(error: unknown, _info: ErrorInfo) {
    recordInteractionCardRenderError(this.props.scope, error)
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children
  }
}
